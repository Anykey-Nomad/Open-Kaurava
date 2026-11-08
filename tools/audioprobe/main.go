// audioprobe — офлайн-проверка пакета audio: конвертация в формат
// устройства, применение пары громкостей L/R и (с флагом -play)
// фактическое параллельное воспроизведение двух звуков.
//
//	go run ./tools/audioprobe            # без вывода на устройство
//	go run ./tools/audioprobe -play      # + игра два звука параллельно
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"math"
	"os"
	"time"

	"dow-mod-studio/modapi"

	"github.com/Anykey-Nomad/Open-Kaurava/attrib"
	"github.com/Anykey-Nomad/Open-Kaurava/audio"
	"github.com/Anykey-Nomad/Open-Kaurava/sfx"
)

// dowRoot — установленная Dawn of War (как в tools/rgddump).
const dowRoot = `C:\Program Files\Steam\steamapps\common\Dawn of War Definitive Edition`

const (
	testRate     = 22050 // «родная» частота звуков игры
	toneHz       = 440.0
	toneSeconds  = 0.5
	deviceFrames = int(audio.SampleRate * toneSeconds)
	checkEps     = 0.02 // допуск на длину/частоту
	almostSilent = 200  // порог «тихо» для среднеквадратического
)

var fail bool

func check(name string, ok bool, detail string) {
	status := "OK  "
	if !ok {
		status = "FAIL"
		fail = true
	}
	fmt.Printf("[%s] %-48s %s\n", status, name, detail)
}

func main() {
	play := flag.Bool("play", false, "фактически проиграть тестовые звуки")
	fdaPath := flag.String("fda", "", "путь к FDA-файлу: декодировать и проверить")
	ack := flag.String("ack", "", "speech_directory юнита: разрешить отклик приказа")
	unitPath := flag.String("unit", "", "путь чертежа ebps: прочитать speech_directory и разрешить отклик")
	capture := flag.Bool("capture", false, "в режиме отклика: приказ захвата (иначе движение)")
	module := flag.String("module", "DoWDE", "модуль для дерева файлов")
	root := flag.String("root", dowRoot, "путь к установленной Dawn of War")
	flag.Parse()

	if *ack != "" || *unitPath != "" {
		checkEmbedded()
		speech := *ack
		if *unitPath != "" {
			speech = checkUnitSpeech(*root, *module, *unitPath)
		}
		checkAck(*root, *module, speech, *capture, *play)
		if fail {
			os.Exit(1)
		}
		return
	}

	if *fdaPath == "" {
		checkSinePipeline()
		checkWAVPipeline()
		checkGainPair()
	} else {
		checkFDA(*fdaPath, *play)
		if fail {
			os.Exit(1)
		}
		return
	}

	if *play {
		playParallel()
	}

	if fail {
		os.Exit(1)
	}
}

// checkEmbedded: встроенные движковые звуки интерфейса и сигнала приказа.
func checkEmbedded() {
	for _, name := range []string{"sfxdata/taskbar_button_click.fda", "sfxdata/ping_cycle.fda"} {
		s, err := sfx.EmbeddedSample(name)
		if err != nil {
			check("встроенный звук "+name, false, err.Error())
			continue
		}
		decoded := bytesToInt16(s.PCM)
		dur := float64(s.Frames()) / audio.SampleRate
		check("встроенный звук "+name, dur > 0.05 && rms(decoded, 0, 2) > almostSilent,
			fmt.Sprintf("%.2f с, RMS L=%.0f", dur, rms(decoded, 0, 2)))
	}
}

// checkUnitSpeech читает чертёж ebps и печатает его speech_directory.
func checkUnitSpeech(root, module, unitPath string) string {
	tree := openTree(root, module)
	unit, err := attrib.ReadUnit(tree, unitPath)
	if err != nil {
		check("чтение чертежа ebps", false, err.Error())
		return ""
	}
	check("чтение speech_directory из ebps", unit.Speech != "", unit.Speech)
	return unit.Speech
}

// checkAck: разрешение отклика приказа голосом юнита через дерево мода —
// путь категории в озвучке и декодирование реплики.
func checkAck(root, module, speech string, isCapture, playIt bool) {
	tree := openTree(root, module)
	kind := "движение"
	if isCapture {
		kind = "захват"
	}
	s, source, ok := sfx.Resolve(tree, speech, isCapture)
	if !ok {
		check("разрешение отклика ("+kind+")", false, "ничего не найдено")
		return
	}
	dur := float64(s.Frames()) / audio.SampleRate
	decoded := bytesToInt16(s.PCM)
	check("разрешение отклика ("+kind+")", dur > 0.05 && rms(decoded, 0, 2) > almostSilent,
		fmt.Sprintf("%s — %.2f с, RMS L=%.0f", source, dur, rms(decoded, 0, 2)))
	if playIt && ok {
		voice, err := audio.Play(s, 0.9, 0.9)
		if err != nil {
			check("play ack", false, err.Error())
			return
		}
		check("play ack", true, fmt.Sprintf("%.2f с на динамиках", dur))
		time.Sleep(time.Duration(dur*1000+300) * time.Millisecond)
		voice.Stop()
	}
}

// openTree строит merged-дерево модуля (свои файлы + обязательные моды).
func openTree(root, module string) *modapi.ModTree {
	modules, err := modapi.DiscoverModules(root)
	if err != nil {
		check("поиск модулей", false, err.Error())
		return nil
	}
	mod := modapi.FindModule(modules, module)
	if mod == nil {
		check("поиск модуля "+module, false, "не найден")
		return nil
	}
	tree, err := modapi.BuildModTree(mod.Path, "English")
	if err != nil {
		check("дерево мода", false, err.Error())
		return nil
	}
	return tree
}

// checkFDA: декодирование реального FDA мода в формат устройства —
// то, что делают FX-таймлайны в dow-mod-studio-go.
func checkFDA(path string, playIt bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		check("чтение файла", false, err.Error())
		return
	}
	fda, err := audio.ParseFDABytes(data)
	if err != nil {
		check("парсинг контейнера", false, err.Error())
		return
	}
	check("парсинг контейнера", true,
		fmt.Sprintf("%d Гц, %d канал(а), битрейт %d, Plugin %q",
			fda.Info.SampleRate, fda.Info.Channels, fda.Info.BlockBitrate, fda.FBIF.PluginName))

	pcm, rate, channels, err := audio.DecodeFDA(data)
	if err != nil {
		check("декодирование кодека Relic", false, err.Error())
		return
	}
	dur := float64(len(pcm)/channels) / float64(rate)
	check("декодирование кодека Relic", len(pcm) > 0,
		fmt.Sprintf("%d сэмплов, %.2f с", len(pcm)/channels, dur))

	sample, err := audio.NewSample(toBytes(pcm), rate, channels)
	if err != nil {
		check("приведение к формату устройства", false, err.Error())
		return
	}
	gotDur := float64(sample.Frames()) / audio.SampleRate
	check("приведение к формату устройства",
		math.Abs(gotDur-dur) <= 0.05,
		fmt.Sprintf("%.2f с на %d Гц стерео", gotDur, audio.SampleRate))

	decoded := bytesToInt16(sample.PCM)
	l := rms(decoded, 0, 2)
	check("в сигнале есть звук (не тишина/не мусор)", l > almostSilent,
		fmt.Sprintf("RMS L=%.0f", l))

	if playIt {
		voice, err := audio.Play(sample, 0.8, 0.8)
		if err != nil {
			check("play FDA", false, err.Error())
			return
		}
		check("play FDA", true, fmt.Sprintf("%.2f с на динамиках", gotDur))
		time.Sleep(time.Duration(gotDur*1000+300) * time.Millisecond)
		voice.Stop()
	}
}

// sinePCM16 — моно-синус заданной частоты на testRate, как выглядит
// «сырой» игровой звук до приведения к формату устройства.
func sinePCM16(freq float64) []int16 {
	n := int(testRate * toneSeconds)
	out := make([]int16, n)
	for i := range out {
		v := math.Sin(2 * math.Pi * freq * float64(i) / testRate)
		out[i] = int16(v * 16000)
	}
	return out
}

// toBytes укладывает сэмплы в байты, как лежат в WAV-data (little-endian).
func toBytes(samples []int16) []byte {
	var buf bytes.Buffer
	for _, s := range samples {
		_ = binary.Write(&buf, binary.LittleEndian, s)
	}
	return buf.Bytes()
}

func bytesToInt16(pcm []byte) []int16 {
	out := make([]int16, len(pcm)/2)
	for i := range out {
		out[i] = int16(binary.LittleEndian.Uint16(pcm[i*2:]))
	}
	return out
}

// dominantFreq — частота с максимальной автокорреляцией (без FFT):
// сдвиг сигнала сам на себя по всем лагам, лучший лаг — период.
// На входе — один канал (через кадр не иначе: в перемежённом стерео
// период удваивается и автокорреляция даст половинную частоту).
func dominantFreq(samples []int16, rate int) float64 {
	bestLag, bestScore := 0, 0.0
	for lag := rate/1000 + 1; lag <= rate/50; lag++ { // 50..1000 Гц
		var sum float64
		for i := 0; i+lag < len(samples); i++ {
			sum += float64(samples[i]) * float64(samples[i+lag])
		}
		if sum > bestScore {
			bestScore, bestLag = sum, lag
		}
	}
	if bestLag == 0 {
		return 0
	}
	return float64(rate) / float64(bestLag)
}

// rms — среднеквадратическое значение одного канала в кадрах.
func rms(frames []int16, channel, channels int) float64 {
	var sum float64
	var n int
	for f := channel; f < len(frames); f += channels {
		sum += float64(frames[f]) * float64(frames[f])
		n++
	}
	if n == 0 {
		return 0
	}
	return math.Sqrt(sum / float64(n))
}

// checkSinePipeline: моно 22050 Гц → стерео 48000: длина и тон сохранены,
// оба канала заполнены одинаково (моно → стерео upmix).
func checkSinePipeline() {
	src := sinePCM16(toneHz)
	sample, err := audio.NewSample(toBytes(src), testRate, 1)
	if err != nil {
		check("конвертация 22050→48000 моно→стерео", false, err.Error())
		return
	}
	frames := sample.Frames()
	check("длина сэмпла после ресемпла",
		math.Abs(float64(frames-deviceFrames)) <= float64(deviceFrames)*checkEps,
		fmt.Sprintf("%d кадров (ожидалось ~%d)", frames, deviceFrames))
	check("стерео: данные кратны кадру", len(sample.PCM)%4 == 0,
		fmt.Sprintf("%d байт", len(sample.PCM)))

	decoded := bytesToInt16(sample.PCM)
	left := make([]int16, 0, frames)
	for f := 0; f+1 < len(decoded); f += 2 {
		left = append(left, decoded[f])
	}
	gotHz := dominantFreq(left, audio.SampleRate)
	check("тон после ресемпла (без фиксации)",
		math.Abs(gotHz-toneHz) <= toneHz*checkEps,
		fmt.Sprintf("%.1f Гц (ожидалось %.0f)", gotHz, toneHz))

	l := rms(decoded, 0, 2)
	r := rms(decoded, 1, 2)
	check("моно заполняет оба канала поровну",
		l > almostSilent && r > almostSilent && math.Abs(l-r)/l < 0.01,
		fmt.Sprintf("L=%.0f R=%.0f", l, r))
}

// checkWAVPipeline: WAV-контейнер разбирается и приводится к формату
// устройства, мусор отвергается.
func checkWAVPipeline() {
	src := sinePCM16(toneHz)
	var wav bytes.Buffer
	writeWAV(&wav, toBytes(src), testRate, 1)
	sample, err := audio.LoadWAV(wav.Bytes())
	if err != nil {
		check("LoadWAV", false, err.Error())
		return
	}
	check("LoadWAV: приведение к формату устройства",
		sample.Frames() > 0 && math.Abs(float64(sample.Frames()-deviceFrames)) <= float64(deviceFrames)*checkEps,
		fmt.Sprintf("%d кадров", sample.Frames()))
	_, err = audio.LoadWAV([]byte("not a wav"))
	check("LoadWAV отвергает мусор", err != nil, fmt.Sprint(err))
}

// checkGainPair: пара громкостей L/R гасит каналы пропорционально и
// читается/меняется на лету.
func checkGainPair() {
	src := sinePCM16(toneHz)
	full, err := audio.NewSample(toBytes(src), testRate, 1)
	if err != nil {
		check("сэмпл для gain-теста", false, err.Error())
		return
	}
	ref := bytesToInt16(full.PCM)
	refL := rms(ref, 0, 2)

	reader := audio.NewGainReader(full.PCM, 0.5, 1.0)
	buf := make([]byte, 4*1024)
	var got []int16
	for {
		n, _ := reader.Read(buf)
		if n == 0 {
			break
		}
		got = append(got, bytesToInt16(buf[:n])...)
	}
	l := rms(got, 0, 2)
	r := rms(got, 1, 2)
	check("L×0.5 примерно вдвое тише эталона",
		refL > almostSilent && math.Abs(l-refL*0.5)/(refL*0.5) < 0.05,
		fmt.Sprintf("%.0f против %.0f", l, refL*0.5))
	check("R при L×0.5 не тронут",
		math.Abs(r-refL)/refL < 0.05,
		fmt.Sprintf("%.0f (эталон %.0f)", r, refL))

	l2, r2 := reader.Volumes()
	check("громкости читаются обратно", l2 == 0.5 && r2 == 1.0,
		fmt.Sprintf("%.2f/%.2f", l2, r2))
	reader.SetVolumes(0.0, 0.25)
	l2, r2 = reader.Volumes()
	check("громкости меняются на лету", l2 == 0 && r2 == 0.25,
		fmt.Sprintf("%.2f/%.2f", l2, r2))
	reader.SetVolumes(-1, -1)
	l2, _ = reader.Volumes()
	check("отрицательные громкости обнуляются", l2 == 0, fmt.Sprintf("%.2f", l2))
}

// playParallel: два звука одновременно — 440 Гц с громкостью 0.5/0.5 и
// 880 Гц с 1.0/0.15 (заметно сдвинут влево) — затем останавливаются.
func playParallel() {
	a, err := audio.NewSample(toBytes(sinePCM16(440)), testRate, 1)
	if err != nil {
		check("play: сэмпл A", false, err.Error())
		return
	}
	b, err := audio.NewSample(toBytes(sinePCM16(880)), testRate, 1)
	if err != nil {
		check("play: сэмпл B", false, err.Error())
		return
	}
	voiceA, err := audio.Play(a, 0.5, 0.5)
	if err != nil {
		check("play: запуск A", false, err.Error())
		return
	}
	// Второй стартует уже поверх первого — они играют параллельно.
	voiceB, err := audio.Play(b, 1.0, 0.15)
	if err != nil {
		check("play: запуск B", false, err.Error())
		voiceA.Stop()
		return
	}
	check("play: два параллельных голоса запущены", true, "440+880 Гц, ~1.5 с")
	// Длина обоих ~0.5 с, даём запас на старт.
	time.Sleep(1500 * time.Millisecond)
	voiceA.Stop()
	voiceB.Stop()
	check("play: устройство приняло и остановило звук", true, "")
}

func writeWAV(buf *bytes.Buffer, pcm []byte, rate, channels int) {
	buf.WriteString("RIFF")
	_ = binary.Write(buf, binary.LittleEndian, uint32(36+len(pcm)))
	buf.WriteString("WAVEfmt ")
	_ = binary.Write(buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(buf, binary.LittleEndian, uint16(channels))
	_ = binary.Write(buf, binary.LittleEndian, uint32(rate))
	_ = binary.Write(buf, binary.LittleEndian, uint32(rate*channels*2))
	_ = binary.Write(buf, binary.LittleEndian, uint16(channels*2))
	_ = binary.Write(buf, binary.LittleEndian, uint16(16))
	buf.WriteString("data")
	_ = binary.Write(buf, binary.LittleEndian, uint32(len(pcm)))
	buf.Write(pcm)
}
