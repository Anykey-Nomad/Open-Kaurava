package audio

import (
	"fmt"
	"math"
)

// Приведение декодированного PCM к формату процесса oto. Контекст создаётся
// один раз, поэтому звуки с другой частотой или числом каналов конвертируются
// заранее (в NewSample), а не при воспроизведении.
//
// Звуки игры приходят на 22050 и 44100 Гц, а контекст — 48000, так что
// почти каждый звук конвертируется по дороге к динамикам. Услышать
// правильность конвертации на звуковом эффекте можно по двум вещам:
// длительности сэмпла и частотам в нём. Звук правильной длины, но
// неправильного тона — провал, который в тесте незаметен, а на слух
// кричащий.
//
// Линейная интерполяция длину и тон держит правильно, но это самый
// слабый из интерполяторов, а для x2 — 22050 → 44100 — он хуже всего:
// его полоса пропускания проваливается к верхнему краю (звук теряет
// яркость), а образы выше частоты Найквиста не убираются — в полосу
// возвращается копия сигнала, измеренная на 5 kHz тоне на −14.5 дБ.
// Это тот тонкий металлический звон, из-за которого эффект звучит
// синтетически.
//
// Поэтому здесь оконная sinc-интерполяция — то, что делает идеальный
// ресемплер: фильтр на нижней из двух частот Найквиста и реконструкция
// сэмплов ядром, а не отрезком прямой.

// fxResampleTaps — сколько сэмплов по обе стороны от позиции выхода
// охватывает ядро. 16 на сторону — ядро в 32 отсчёта: в полосе
// пропускания плоскость в доли дБ и ниже −70 дБ на образах, то есть
// далеко за пределами того, что показывает звуковой эффект.
const fxResampleTaps = 16

// fxResamplePhases — размер таблицы ядра. Форма ядра для каждого выходного
// сэмпла одинакова и лишь сдвигается на дробную часть расстояния до
// ближайшего входного, поэтому таблица строится один раз на конвертацию
// и используется индексом. Сдвиг квантуется на 512-ю часть сэмпла — это
// далеко за пределами слышимости, зато внутренний цикл читает таблицу,
// а не считает синус.
const fxResamplePhases = 512

// fxResampleKernel — предвычисленные веса одной конвертации:
// индексируется фазой, затем номером тапа.
type fxResampleKernel struct {
	// taps[phase][j] — вес входного сэмпла j-fxResampleTaps относительно
	// позиции выхода для данной дробной фазы.
	taps [][]float64
	// cutoff — угол среза в циклах на входной сэмпл: половина меньшей
	// из двух частот, что и убирает образ из слышимой полосы.
	cutoff float64
}

// buildFxResampleKernel строит ядро для конвертации. cutoff — угол среза в
// циклах на сэмпл, 0.5 — собственная частота Найквиста входа.
func buildFxResampleKernel(cutoff float64) *fxResampleKernel {
	k := 2 * cutoff
	kern := &fxResampleKernel{cutoff: cutoff, taps: make([][]float64, fxResamplePhases)}
	for p := 0; p < fxResamplePhases; p++ {
		frac := float64(p) / float64(fxResamplePhases)
		row := make([]float64, 2*fxResampleTaps)
		var sum float64
		for j := 0; j < 2*fxResampleTaps; j++ {
			// Расстояние от позиции выхода до этого входного сэмпла в
			// сэмплах: собственный сдвиг тапа минус дробная часть.
			d := float64(j-fxResampleTaps) - frac
			w := fxSinc(k*d) * fxBlackman(d/fxResampleTaps)
			row[j] = w
			sum += w
		}
		// Нормировка каждой фазы к единичному усилению держит уровень
		// сигнала одинаковым через всю конвертацию: без неё провал полосы
		// пропускания приходит как тихий сэмпл, а не только глухой.
		if sum != 0 {
			for j := range row {
				row[j] /= sum
			}
		}
		kern.taps[p] = row
	}
	return kern
}

// fxSinc — нормированный синус: sin(pi x)/(pi x).
func fxSinc(x float64) float64 {
	if x == 0 {
		return 1
	}
	px := math.Pi * x
	return math.Sin(px) / px
}

// fxBlackman — окно Блэкмана на -1..1: оно же уводит собственный провал
// ядра вниз — голый sinc звенит и оставляет эхо перед резким переходом,
// а удар — это резкий переход.
func fxBlackman(u float64) float64 {
	if u <= -1 || u >= 1 {
		return 0
	}
	return 0.42 + 0.5*math.Cos(math.Pi*u) + 0.08*math.Cos(2*math.Pi*u)
}

// ResamplePCM16 приводит PCM к формату устройства oto (48000/2, см.
// Sample). Частота и число каналов любые в пределах 1–2.
func ResamplePCM16(samples []int16, inRate, inChannels, outRate, outChannels int) ([]int16, error) {
	if inRate <= 0 || outRate <= 0 || inChannels <= 0 || outChannels <= 0 {
		return nil, fmt.Errorf("audio: неверный формат PCM: %d Гц, %d → %d Гц, %d", inRate, inChannels, outRate, outChannels)
	}
	if (inChannels != 1 && inChannels != 2) || (outChannels != 1 && outChannels != 2) {
		return nil, fmt.Errorf("audio: конвертация каналов %d → %d не поддерживается", inChannels, outChannels)
	}
	if len(samples)%inChannels != 0 {
		return nil, fmt.Errorf("audio: число сэмплов %d не делится на %d каналов", len(samples), inChannels)
	}
	inFrames := len(samples) / inChannels
	if inFrames == 0 {
		return []int16{}, nil
	}
	// Конвертация, меняющая только число каналов, — это свёртка или
	// развёртка, а не фильтрация: сэмплы остаются сэмплами.
	if inRate == outRate {
		return remapPCM16Channels(samples, inChannels, outChannels), nil
	}
	outFrames := int((int64(inFrames)*int64(outRate) + int64(inRate)/2) / int64(inRate))
	if outFrames <= 0 {
		return []int16{}, nil
	}

	// Срез стоит на меньшей из двух частот Найквиста: апсемплинг
	// сохраняет всё, что было в источнике, даунсемплинг срезает то, что
	// целевая частота не несёт. В циклах на входной сэмпл.
	cutoff := 0.5
	if outRate < inRate {
		cutoff = 0.5 * float64(outRate) / float64(inRate)
	}
	kern := buildFxResampleKernel(cutoff)

	out := make([]int16, 0, outFrames*outChannels)
	// Позиция каждого выходного кадра во входных сэмплах считается точно,
	// а не накоплением: шаг по плавающей точке с округлением — как
	// конвертация уплывает, а уплывший звук обрывается раньше конца.
	step := float64(inRate) / float64(outRate)
	pos := 0.0
	for frame := 0; frame < outFrames; frame++ {
		i0 := int(math.Floor(pos))
		frac := pos - float64(i0)
		// Ближайшая фаза таблицы: для точного x2 это только 0 или
		// половина — вся конвертация читает две строки таблицы, а не
		// гуляет по все 512.
		p := int(frac*fxResamplePhases + 0.5)
		if p >= fxResamplePhases {
			p = fxResamplePhases - 1
		}
		row := kern.taps[p]
		for ch := 0; ch < outChannels; ch++ {
			var acc float64
			for j := 0; j < 2*fxResampleTaps; j++ {
				idx := i0 + j - fxResampleTaps
				// За краями держится ближайший настоящий сэмпл, а не
				// ноль: конвертация не уводит звук в тишину и не щёлкает
				// на границе.
				if idx < 0 {
					idx = 0
				} else if idx >= inFrames {
					idx = inFrames - 1
				}
				acc += row[j] * float64(fxSourceSample(samples, inChannels, outChannels, idx, ch))
			}
			out = append(out, clampPCM16(int64(math.Round(acc))))
		}
		pos += step
	}
	return out, nil
}

// remapPCM16Channels меняет число каналов без пересчёта во времени —
// именно это и есть конвертация при неизменной частоте.
func remapPCM16Channels(samples []int16, inChannels, outChannels int) []int16 {
	inFrames := len(samples) / inChannels
	out := make([]int16, 0, inFrames*outChannels)
	for f := 0; f < inFrames; f++ {
		for ch := 0; ch < outChannels; ch++ {
			out = append(out, clampPCM16(fxSourceSample(samples, inChannels, outChannels, f, ch)))
		}
	}
	return out
}

// fxSourceSample читает один входной кадр как один выходной канал: моно
// заполняет оба канала, стерео сворачивается в один средним.
func fxSourceSample(samples []int16, inChannels, outChannels int, frame, outChannel int) int64 {
	first, second := outChannel, outChannel
	switch {
	case inChannels == outChannels:
	case inChannels == 1:
		first, second = 0, 0
	default:
		first, second = 0, 1
	}
	v := int64(samples[frame*inChannels+first])
	if first != second {
		v = (v + int64(samples[frame*inChannels+second])) / 2
	}
	return v
}

func clampPCM16(v int64) int16 {
	if v > 32767 {
		return 32767
	}
	if v < -32768 {
		return -32768
	}
	return int16(v)
}
