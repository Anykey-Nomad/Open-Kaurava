package audio

import (
	"errors"
	"fmt"
	"sync"

	"github.com/ebitengine/oto/v3"
)

// Music — играющий музыкальный трек: поток FDA, подключённый к устройству.
// В отличие от игровых звуков трек живёт минутами и зациклен, поэтому у него
// есть ручка остановки, а громкость меняется на лету.
type Music struct {
	stream *FDAStream
	player *oto.Player

	mu     sync.Mutex
	closed bool
}

// PlayMusic запускает поток музыки параллельно звукам игры. volume — общая
// громкость поверх той, что записана в .rat (1.0 — как в файле).
func PlayMusic(stream *FDAStream, volume float64) (*Music, error) {
	if stream == nil {
		return nil, errors.New("audio: нет потока музыки")
	}
	device, err := Context()
	if err != nil {
		return nil, fmt.Errorf("audio: устройство: %w", err)
	}
	stream.SetVolume(volume)
	player := device.NewPlayer(stream)
	player.Play()
	return &Music{stream: stream, player: player}, nil
}

// Duration — длительность трека в секундах.
func (m *Music) Duration() float64 {
	if m == nil {
		return 0
	}
	return m.stream.Duration()
}

// SetVolume меняет громкость трека на лету.
func (m *Music) SetVolume(v float64) {
	if m == nil {
		return
	}
	m.stream.SetVolume(v)
}

// Volume — текущая громкость трека.
func (m *Music) Volume() float64 {
	if m == nil {
		return 0
	}
	return float64(float32frombits(m.stream.volume.Load()))
}

// Stop останавливает трек и освобождает плеер. Повторный вызов безопасен.
func (m *Music) Stop() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	m.closed = true
	m.player.Pause()
	m.player.Close()
}
