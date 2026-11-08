package audio

import (
	"encoding/binary"
	"fmt"
	"io"
	"sync/atomic"
)

// FDAStream — поток из .fda в формате устройства (48000 Гц, стерео, s16LE).
//
// Музыкальные треки весят 5–11 МБ сжатых, а это около четырёх минут звука:
// декодировать их целиком в PCM — это десятки мегабайт памяти на трек, а
// играть музыку положено непрерывно и с зацикливанием. Поэтому поток
// декодирует по одному блоку кодека (512 сэмплов на канал), приводит частоту
// к 48000 линейной интерполяцией с переносом состояния между блоками и
// отдаёт oto ровно столько байт, сколько он просит. Память при этом
// ограничена парой блоков, а не длиной трека.
type FDAStream struct {
	fda       *FDAFile
	rate      int
	channels  int
	frameSize int
	bitrate   int
	blocks    int
	loop      bool

	dec   *relicHandle
	block int

	// src — уже декодированные кадры в формате источника, но сведённые к
	// стерео (чередование L, R); pos — положение чтения в кадрах, step —
	// сколько кадров источника приходится на один кадр устройства.
	src  []float32
	pos  float64
	step float64

	buf     []byte
	samples []int16

	volume atomic.Uint32
}

// NewFDAStream собирает поток из .fda. loop зацикливает трек: конец файла
// продолжается его началом, декодер при этом перезапускается, чтобы не
// тянуть состояние прошлого круга.
func NewFDAStream(fda *FDAFile, loop bool) (*FDAStream, error) {
	if fda == nil {
		return nil, fmt.Errorf("audio: пустой FDA")
	}
	channels := int(fda.Info.Channels)
	if channels <= 0 || channels > relicMaxChannels {
		return nil, fmt.Errorf("audio: FDA: неверное число каналов %d", channels)
	}
	rate := SoundPlaybackRate(int(fda.Info.SampleRate))
	if rate <= 0 {
		return nil, fmt.Errorf("audio: FDA: неверная частота %d", fda.Info.SampleRate)
	}
	bitrate := int(fda.Info.BlockBitrate)
	if bitrate < relicMinBitrate || bitrate > relicMaxBitrate {
		return nil, fmt.Errorf("audio: FDA: неверный битрейт %d", bitrate)
	}
	dec := relicInit(channels, bitrate, rate)
	if dec == nil {
		return nil, fmt.Errorf("audio: FDA: декодер не инициализировался (%d Гц, %d каналов)", rate, channels)
	}
	frameSize := relicGetFrameSize(dec)
	if frameSize <= 0 {
		return nil, fmt.Errorf("audio: FDA: неверный размер кадра")
	}
	stream := &FDAStream{
		fda:       fda,
		rate:      rate,
		channels:  channels,
		frameSize: frameSize,
		bitrate:   bitrate,
		blocks:    len(fda.RawData) / (frameSize * channels),
		loop:      loop,
		dec:       dec,
		step:      float64(rate) / float64(SampleRate),
		buf:       make([]byte, relicBufferSize),
		samples:   make([]int16, relicSamplesPerFrame*channels),
	}
	stream.SetVolume(1)
	return stream, nil
}

// NewFDAStreamBytes — поток прямо из образа файла (так приходят треки из SGA).
func NewFDAStreamBytes(data []byte, loop bool) (*FDAStream, error) {
	fda, err := ParseFDABytes(data)
	if err != nil {
		return nil, err
	}
	return NewFDAStream(fda, loop)
}

// SetVolume задаёт громкость потока на лету: 1.0 — как в файле, 0 — тишина.
func (s *FDAStream) SetVolume(v float64) {
	if v < 0 {
		v = 0
	}
	if v > 4 {
		v = 4
	}
	s.volume.Store(float32bits(float32(v)))
}

// Rate — частота, на которой декодируется трек (до приведения к устройству).
func (s *FDAStream) Rate() int { return s.rate }

// Channels — число каналов в файле.
func (s *FDAStream) Channels() int { return s.channels }

// Duration — длительность трека: столько секунд звучит один проход до
// зацикливания.
func (s *FDAStream) Duration() float64 {
	return float64(s.blocks) * relicSamplesPerFrame / float64(s.rate)
}

// Read отдаёт плееру кадры в формате устройства. Поток бесконечен, если
// включено зацикливание; иначе по концу трека возвращается io.EOF.
func (s *FDAStream) Read(p []byte) (int, error) {
	usable := len(p) - len(p)%frameBytes
	written := 0
	volume := float64(float32frombits(s.volume.Load()))
	for written+frameBytes <= usable {
		// Для интерполяции нужен кадр после текущего.
		for s.pos+1 >= float64(len(s.src)/2) && s.fill() {
		}
		if s.pos+1 >= float64(len(s.src)/2) {
			break
		}
		frame := int(s.pos)
		frac := float32(s.pos - float64(frame))
		l := s.lerp(frame, 0, frac) * volume
		r := s.lerp(frame, 1, frac) * volume
		binary.LittleEndian.PutUint16(p[written:], uint16(clamp16f(l)))
		binary.LittleEndian.PutUint16(p[written+2:], uint16(clamp16f(r)))
		written += frameBytes
		s.pos += s.step
	}
	// Прочитанное выбрасываем, чтобы буфер не рос: остаётся хвост в один кадр.
	if drop := int(s.pos); drop > 0 {
		copy(s.src, s.src[drop*2:])
		s.src = s.src[:len(s.src)-drop*2]
		s.pos -= float64(drop)
	}
	if written == 0 {
		return 0, io.EOF
	}
	return written, nil
}

// lerp — значение канала между двумя соседними кадрами источника.
func (s *FDAStream) lerp(frame, channel int, frac float32) float64 {
	a := s.src[frame*2+channel]
	b := s.src[(frame+1)*2+channel]
	return float64(a + (b-a)*frac)
}

// fill декодирует следующий блок кодека в буфер источника. Ложь — блоков
// больше нет (и зацикливание выключено).
func (s *FDAStream) fill() bool {
	if s.block >= s.blocks {
		if !s.loop {
			return false
		}
		s.dec = relicInit(s.channels, s.bitrate, s.rate)
		if s.dec == nil {
			return false
		}
		s.block = 0
	}
	total := len(s.fda.RawData)
	decoded := false
	for ch := 0; ch < s.channels; ch++ {
		offset := (s.block*s.channels + ch) * s.frameSize
		for j := 0; j < relicBufferSize; j++ {
			if j < s.frameSize && offset+j < total {
				s.buf[j] = s.fda.RawData[offset+j]
			} else {
				s.buf[j] = 0
			}
		}
		if relicDecodeFrame(s.dec, s.buf, ch) {
			decoded = true
		}
	}
	relicGetPCM16(s.dec, s.samples)
	for f := 0; f < relicSamplesPerFrame; f++ {
		left := float32(s.samples[f*s.channels]) / 32768
		right := left
		if s.channels == 2 {
			right = float32(s.samples[f*2+1]) / 32768
		}
		s.src = append(s.src, left, right)
	}
	s.block++
	return decoded || s.block < s.blocks || s.loop
}

// clamp16f — float -1..1 в int16 с насыщением.
func clamp16f(v float64) int16 {
	switch {
	case v >= 1:
		return 32767
	case v <= -1:
		return -32768
	}
	return int16(v * 32767)
}
