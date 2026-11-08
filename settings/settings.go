// Package settings — пользовательские настройки, общие для сцен и звука.
//
// Пока это только громкости: речь юнитов, звуки действий и эффектов и
// музыка. Значения — доли от 1; их умножают на свою базовую громкость
// пакеты sfx и scenes, поэтому настройка не зависит от того, какой трек
// или реплика играет.
package settings

import (
	"math"
	"sync/atomic"
)

var (
	speech  atomic.Uint64
	effects atomic.Uint64
	music   atomic.Uint64
)

func init() {
	SetSpeech(1)
	SetEffects(1)
	SetMusic(1)
}

// clamp держит громкость в диапазоне 0..1.
func clamp(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	return math.Max(0, math.Min(1, v))
}

// Speech — громкость речи юнитов.
func Speech() float64 { return math.Float64frombits(speech.Load()) }

// SetSpeech задаёт громкость речи.
func SetSpeech(v float64) { speech.Store(math.Float64bits(clamp(v))) }

// Effects — громкость звуков действий и эффектов (щелчки, сигналы).
func Effects() float64 { return math.Float64frombits(effects.Load()) }

// SetEffects задаёт громкость звуков действий и эффектов.
func SetEffects(v float64) { effects.Store(math.Float64bits(clamp(v))) }

// Music — громкость музыки.
func Music() float64 { return math.Float64frombits(music.Load()) }

// SetMusic задаёт громкость музыки.
func SetMusic(v float64) { music.Store(math.Float64bits(clamp(v))) }
