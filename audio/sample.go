package audio

import (
	"encoding/binary"
	"fmt"
)

// Sample — PCM в формате устройства: 48000 Гц, стерео, s16LE,
// кадры подряд (L, R, L, R…). Такой сэмпл играется без всякой
// дальнейшей обработки — только умножение на громкости каналов в
// GainReader перед каждым кадром.
type Sample struct {
	PCM []byte
}

// Frames — сколько кадров в сэмпле.
func (s Sample) Frames() int { return len(s.PCM) / frameBytes }

// NewSample приводит произвольный PCM16 к формату устройства: другая
// частота (22050/44100 у звуков игры) пересемплируется, моно
// разворачивается в стерео. Конвертация делается один раз при загрузке,
// не в кадр воспроизведения.
func NewSample(pcm []byte, rate, channels int) (Sample, error) {
	if rate <= 0 || channels <= 0 {
		return Sample{}, fmt.Errorf("audio: неверный формат: %d Гц, %d каналов", rate, channels)
	}
	if len(pcm)%(channels*2) != 0 {
		return Sample{}, fmt.Errorf("audio: PCM не выровнен по кадрам (%d байт, %d каналов)", len(pcm), channels)
	}
	if len(pcm) == 0 {
		return Sample{}, nil
	}
	samples := bytesToInt16(pcm)
	out, err := ResamplePCM16(samples, rate, channels, SampleRate, Channels)
	if err != nil {
		return Sample{}, err
	}
	return Sample{PCM: int16ToBytes(out)}, nil
}

// LoadWAV разбирает PCM16 WAV и приводит его к формату устройства.
func LoadWAV(data []byte) (Sample, error) {
	pcm, rate, channels, err := parseWAV(data)
	if err != nil {
		return Sample{}, err
	}
	return NewSample(pcm, rate, channels)
}

// parseWAV читает контейнер RIFF/WAVE: только целочисленный 16-битный PCM,
// каким кодируются звуки игры. Возвращает сырые данные, частоту и каналы.
func parseWAV(data []byte) (pcm []byte, rate, channels int, err error) {
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, 0, 0, fmt.Errorf("audio: не RIFF/WAVE")
	}
	pos := 12
	for pos+8 <= len(data) {
		id := string(data[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		pos += 8
		if size < 0 || pos+size > len(data) {
			return nil, 0, 0, fmt.Errorf("audio: обрезанный WAV-чанк %q", id)
		}
		payload := data[pos : pos+size]
		switch id {
		case "fmt ":
			if len(payload) < 16 {
				return nil, 0, 0, fmt.Errorf("audio: обрезанный fmt-чанк")
			}
			if format := binary.LittleEndian.Uint16(payload[0:2]); format != 1 {
				return nil, 0, 0, fmt.Errorf("audio: поддерживается только PCM (формат %d)", format)
			}
			channels = int(binary.LittleEndian.Uint16(payload[2:4]))
			rate = int(binary.LittleEndian.Uint32(payload[4:8]))
			if bits := binary.LittleEndian.Uint16(payload[14:16]); bits != 16 {
				return nil, 0, 0, fmt.Errorf("audio: только 16-битный PCM (битов: %d)", bits)
			}
		case "data":
			pcm = append([]byte(nil), payload...)
		}
		pos += size
		if size%2 != 0 { // чанки RIFF выравниваются по границе слова.
			pos++
		}
	}
	if rate <= 0 || channels <= 0 || pcm == nil {
		return nil, 0, 0, fmt.Errorf("audio: в WAV нет fmt или data")
	}
	if len(pcm)%(channels*2) != 0 {
		return nil, 0, 0, fmt.Errorf("audio: данные WAV не выровнены по кадрам")
	}
	return pcm, rate, channels, nil
}

// bytesToInt16 и int16ToBytes — перекладывание little-endian PCM между
// байтами и сэмплами (ресемплер считает в сэмплах).
func bytesToInt16(pcm []byte) []int16 {
	out := make([]int16, len(pcm)/2)
	for i := range out {
		out[i] = int16(binary.LittleEndian.Uint16(pcm[i*2:]))
	}
	return out
}

func int16ToBytes(samples []int16) []byte {
	out := make([]byte, len(samples)*2)
	for i, v := range samples {
		binary.LittleEndian.PutUint16(out[i*2:], uint16(v))
	}
	return out
}
