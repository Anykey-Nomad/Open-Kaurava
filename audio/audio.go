// Package audio — параллельное воспроизведение звуков игры.
//
// Устройство открывается один раз на процесс в фиксированном формате
// 48000 Гц, стерео, s16LE: это тот формат, который WASAPI принимает
// надёжно, а моно и «родные» 22050/44100 Гц звуков игры — нет (иначе
// oto уходит на WinMM, где частоту выбирает устройство и наложение
// звуков растягивается и падает по тону). Поэтому каждый звук приводится
// к формату устройства до воспроизведения — см. NewSample.
//
// Каждое звучание — отдельный Voice со своим oto.Player: звуки идут
// параллельно и смешиваются устройством, один не обрывает другой.
//
// Задел под объёмный звук: у каждого голоса своя громкость на ЛЕВОМ и
// ПРАВОМ канале (SetVolume), меняется в любой момент во время звучания.
// Позиционный расчёт будущего 3D-звука (дальность → затухание,
// направление → баланс каналов) снаружи подставит пару чисел каждый
// кадр — устройство, формат и микшер при этом не меняются.
//
// Схема воспроизведения (контекст, формат, параллельные плееры)
// перенесена из dow-mod-studio-go/internal/audioplayer и
// internal/fx/sound_play.go, где так играются звуки FX-таймлайнов.
package audio

import (
	"errors"
	"fmt"
	"sync"

	"github.com/ebitengine/oto/v3"
)

// Формат устройства процесса — см. комментарий к пакету.
const (
	SampleRate = 48000
	Channels   = 2
)

// frameBytes — кадр формата устройства: 2 канала × 2 байта (s16LE).
const frameBytes = Channels * 2

var (
	ctxOnce sync.Once
	ctx     *oto.Context
	ctxErr  error
)

// Context возвращает аудио-контекст процесса, создавая его при первом
// обращении. Аргументов нет намеренно: у контекста ровно один формат,
// а запрос другого — это приведение к этому (см. NewSample).
func Context() (*oto.Context, error) {
	ctxOnce.Do(func() {
		op := &oto.NewContextOptions{}
		op.SampleRate = SampleRate
		op.ChannelCount = Channels
		op.Format = oto.FormatSignedInt16LE
		var ready chan struct{}
		ctx, ready, ctxErr = oto.NewContext(op)
		if ctxErr == nil {
			<-ready
		}
	})
	if ctxErr != nil {
		return nil, ctxErr
	}
	return ctx, nil
}

// Play запускает звук параллельно с уже играющими: у sample своя
// громкость по каналам (left, right — множители, 1.0 полная, 0.0 тишина;
// отрицательные обнуляются). Вернётся ручка Voice — ею звук останавливается
// и громкость меняется на лету.
func Play(sample Sample, left, right float64) (*Voice, error) {
	if len(sample.PCM) == 0 {
		return nil, errors.New("audio: пустой сэмпл")
	}
	device, err := Context()
	if err != nil {
		return nil, fmt.Errorf("audio: устройство: %w", err)
	}
	reader := NewGainReader(sample.PCM, left, right)
	player := device.NewPlayer(reader)
	player.Play()
	voice := &Voice{reader: reader, player: player, stopCh: make(chan struct{})}
	// Плеер освобождается сам, когда сэмпл доигран: у игровых звуков
	// ручка не удерживается (см. Voice.retire).
	go voice.retire()
	return voice, nil
}
