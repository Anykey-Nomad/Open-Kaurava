package audio

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// Поддержка звуков в формате FDA (Relic Chunky + проприетарный DCT-кодек),
// как они играются в dow-mod-studio-go при воспроизведении FX-таймлайнов:
// контейнер и декодер перенесены оттуда (internal/fda недоступен для
// импорта), а результат приводится к формату устройства через наш
// ResamplePCM16 — дальше сэмпл идёт в Play на общих основаниях.
//
// Формат файла (см. Readme кодека): заголовок «Relic Chunky», чанки FBIF
// (кто и когда записал), INFO (каналы, частота, битрейт, точки цикла) и
// DATA (сжатые кадры, по кадру на канал: [ch0][ch1][ch0][ch1]…).

const (
	fdaSignature    = "Relic Chunky"
	fdaSignServ     = 0x001a0a0d
	fdaChunkIDFBIF  = "FBIF"
	fdaChunkIDFDA   = "FDA "
	fdaChunkIDINFO  = "INFO"
	fdaChunkIDDATA  = "DATA"
	fdaHeaderSize   = 24
	fdaChunkHdrSize = 20
)

// FDAFile — распарсенный FDA-файл.
type FDAFile struct {
	Header  FDAHeader
	FBIF    FBIFChunk
	Info    INFOChunk
	RawData []byte
}

// FDAHeader — заголовок файла (24 байта).
type FDAHeader struct {
	Signature [12]byte
	SignServ  [4]byte
	MajorVer  uint32
	MinorVer  uint32
}

// FBIFChunk — чанк с информацией о записи плагина.
type FBIFChunk struct {
	PluginName string
	PluginVer  uint32
	UserName   string
	BurnTime   string
}

// INFOChunk — чанк с аудио-метаданными (28 байт).
type INFOChunk struct {
	Channels     uint32
	SampleSize   uint32
	BlockBitrate uint32
	SampleRate   uint32
	BeginLoop    uint32
	EndLoop      uint32
	StartOffset  uint32
}

// fixed part of the chunk header (without variable-length ChunkName)
type fdaChunkHdrFixed struct {
	ChunkType    [4]byte
	ChunkID      [4]byte
	ChunkVer     uint32
	ChunkSize    uint32
	LenChunkName uint32
}

// DATAChunk — заголовок чанка с телом звука.
type DATAChunk struct {
	DataSize uint32
}

// ParseFDA читает FDA-файл с диска.
func ParseFDA(filename string) (*FDAFile, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()
	return ParseFDAReader(f)
}

// ParseFDABytes парсит FDA-образ из памяти — так приходят файлы,
// извлечённые из SGA-архивов мода.
func ParseFDABytes(data []byte) (*FDAFile, error) {
	return ParseFDAReader(bytes.NewReader(data))
}

// ParseFDAReader разбирает FDA-образ из читателя.
func ParseFDAReader(f io.Reader) (*FDAFile, error) {
	file := &FDAFile{}

	// Read header
	if err := binary.Read(f, binary.LittleEndian, &file.Header); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	// Validate signature
	if string(file.Header.Signature[:]) != fdaSignature {
		return nil, fmt.Errorf("invalid signature: got %q, want %q", string(file.Header.Signature[:]), fdaSignature)
	}

	// Validate service signature
	signServ := binary.LittleEndian.Uint32(file.Header.SignServ[:])
	if signServ != fdaSignServ {
		return nil, fmt.Errorf("invalid service signature: got 0x%08x, want 0x%08x", signServ, fdaSignServ)
	}

	// Read chunks sequentially
	for {
		var fixed fdaChunkHdrFixed
		if err := binary.Read(f, binary.LittleEndian, &fixed); err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("read chunk header: %w", err)
		}
		if fixed.LenChunkName > 0 {
			name := make([]byte, fixed.LenChunkName)
			if _, err := io.ReadFull(f, name); err != nil {
				return nil, fmt.Errorf("read chunk name: %w", err)
			}
		}

		switch string(fixed.ChunkID[:]) {
		case fdaChunkIDFBIF:
			if err := readFBIFChunk(f, &file.FBIF); err != nil {
				return nil, fmt.Errorf("read FBIF: %w", err)
			}

		case fdaChunkIDFDA:
			// Контейнерный чанк — тело не читаем, его содержимое
			// приходит следующими чанками.

		case fdaChunkIDINFO:
			if err := binary.Read(f, binary.LittleEndian, &file.Info); err != nil {
				return nil, fmt.Errorf("read INFO: %w", err)
			}

		case fdaChunkIDDATA:
			var dc DATAChunk
			if err := binary.Read(f, binary.LittleEndian, &dc); err != nil {
				return nil, fmt.Errorf("read DATA header: %w", err)
			}
			file.RawData = make([]byte, dc.DataSize)
			if _, err := io.ReadFull(f, file.RawData); err != nil {
				return nil, fmt.Errorf("read DATA payload: %w", err)
			}

		default:
			// Unknown chunk - skip
			if fixed.ChunkSize > 0 {
				if _, err := io.CopyN(io.Discard, f, int64(fixed.ChunkSize)); err != nil {
					return nil, fmt.Errorf("skip unknown chunk: %w", err)
				}
			}
		}
	}

	return file, nil
}

func readFBIFChunk(r io.Reader, fbif *FBIFChunk) error {
	var lenPluginName uint32
	if err := binary.Read(r, binary.LittleEndian, &lenPluginName); err != nil {
		return err
	}
	pluginName := make([]byte, lenPluginName)
	if _, err := io.ReadFull(r, pluginName); err != nil {
		return err
	}
	fbif.PluginName = string(pluginName)

	if err := binary.Read(r, binary.LittleEndian, &fbif.PluginVer); err != nil {
		return err
	}

	var lenUserName uint32
	if err := binary.Read(r, binary.LittleEndian, &lenUserName); err != nil {
		return err
	}
	userName := make([]byte, lenUserName)
	if _, err := io.ReadFull(r, userName); err != nil {
		return err
	}
	fbif.UserName = string(userName)

	var lenBurnTime uint32
	if err := binary.Read(r, binary.LittleEndian, &lenBurnTime); err != nil {
		return err
	}
	burnTime := make([]byte, lenBurnTime)
	if _, err := io.ReadFull(r, burnTime); err != nil {
		return err
	}
	fbif.BurnTime = string(burnTime)

	return nil
}

// DecodeFDA декодирует FDA-образ в «родном» виде: PCM16 на частоте файла,
// число каналов — из INFO. Дальше путь обычный: NewSample → Play.
func DecodeFDA(data []byte) (pcm []int16, rate, channels int, err error) {
	fda, err := ParseFDABytes(data)
	if err != nil {
		return nil, 0, 0, err
	}
	// Частота из INFO — только метка, и она бывает неверной: такому файлу
	// верить нельзя (см. SoundPlaybackRate), и декодировать его надо уже на
	// исправленной — от неё зависит таблица частот кодека.
	rate = SoundPlaybackRate(int(fda.Info.SampleRate))
	pcm, failed, err := decodeFDAAtRate(fda, rate)
	if err != nil {
		return nil, 0, 0, err
	}
	if failed > 0 {
		// Битовые данные кадров не влезают в частотную таблицу — обычно
		// это означает объявленную в INFO частоту не ту (выбор таблицы
		// кодека упирается ровно в 22050 Гц).
		return nil, 0, 0, fmt.Errorf("FDA: кадров не распаковано %d из %d (проверьте частоту %d Гц)",
			failed, len(fda.RawData)/frameSizeOf(fda)/max(1, int(fda.Info.Channels)), rate)
	}
	return pcm, rate, int(fda.Info.Channels), nil
}

// LoadFDA — главная точка входа: декодирует FDA и приводит его к формату
// устройства (48000 Гц, стерео), после чего сэмпл готов к Play.
func LoadFDA(data []byte) (Sample, error) {
	pcm, rate, channels, err := DecodeFDA(data)
	if err != nil {
		return Sample{}, err
	}
	return NewSample(int16ToBytes(pcm), rate, channels)
}

// Частоты, между которыми путается INFO у части файлов DoWDE.
//
// Кодек Relic отдаёт одни и те же сэмплы, на какой бы частоте его ни вызвать:
// она решает только, как долго они звучат и насколько высоко, и восстановить её
// из самих сэмплов нельзя. Ложная частота в заголовке поэтому не лечится
// ресемплингом — растянутый, на октаву ниже звук остаётся растянутым. Зато
// ошибка у этих файлов одна и та же: в INFO стоит 22050, а записаны они были
// на 44100 (так же исправлен плеер FDA в dow-mod-studio-go).
const (
	bogusSoundRate = 22050
	realSoundRate  = 44100
)

// SoundPlaybackRate — частота, с которой декодированный звук надо играть.
// Совпадает с заявленной, кроме одной известной ошибки: 22050 в INFO значит
// 44100 на самом деле. Всё остальное берётся на слово.
func SoundPlaybackRate(declared int) int {
	if declared == bogusSoundRate {
		return realSoundRate
	}
	return declared
}

// frameSizeOf — размер кадра кодека в байтах (битрейт/8) для сообщений
// об ошибках; 0, если кодек не инициализировался.
func frameSizeOf(fda *FDAFile) int {
	bitrate := int(fda.Info.BlockBitrate)
	if bitrate < relicMinBitrate || bitrate > relicMaxBitrate {
		return 0
	}
	return bitrate / 8
}

// decodeFDAAtRate декодирует кадры на заданной частоте: от неё зависит,
// какой частотной таблицей (128/256/512) распаковывается кодек — ошибка
// в выборе таблицы не лечится последующим ресемплингом. Возвращает число
// кадров, которые кодек не смог распаковать: по нему видно, что частота
// угадана неверно.
func decodeFDAAtRate(fda *FDAFile, codecRate int) ([]int16, int, error) {
	// Init Relic decoder
	channels := int(fda.Info.Channels)
	bitrate := int(fda.Info.BlockBitrate)

	decoder := relicInit(channels, bitrate, codecRate)
	if decoder == nil {
		return nil, 0, fmt.Errorf("failed to init Relic decoder (channels=%d, bitrate=%d, rate=%d)", channels, bitrate, codecRate)
	}

	frameSize := relicGetFrameSize(decoder)
	if frameSize == 0 {
		return nil, 0, fmt.Errorf("invalid frame size")
	}

	// Calculate total samples
	// Frames are stored per-channel: [ch0_frame][ch1_frame][ch0_frame][ch1_frame]...
	totalBytes := len(fda.RawData)
	framesPerChannel := totalBytes / (frameSize * channels)
	totalSamples := framesPerChannel * relicSamplesPerFrame

	// Decode all frames
	pcmData := make([]int16, 0, totalSamples*channels)
	chBuf := make([]byte, relicBufferSize)
	failed := 0

	for i := 0; i < framesPerChannel; i++ {
		// Decode each channel from its own frame in the stream
		for ch := 0; ch < channels; ch++ {
			offset := (i*channels + ch) * frameSize
			// Copy frame data into buffer (padded to relicBufferSize)
			for j := 0; j < relicBufferSize; j++ {
				if j < frameSize && offset+j < totalBytes {
					chBuf[j] = fda.RawData[offset+j]
				} else {
					chBuf[j] = 0
				}
			}
			if !relicDecodeFrame(decoder, chBuf, ch) {
				failed++
			}
		}

		// Extract PCM samples for all channels
		samples := make([]int16, relicSamplesPerFrame*channels)
		relicGetPCM16(decoder, samples)
		pcmData = append(pcmData, samples...)
	}

	return pcmData, failed, nil
}
