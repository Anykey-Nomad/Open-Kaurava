package scenes

import (
	"log"

	"github.com/Anykey-Nomad/Open-Kaurava/sfx"
)

// Музыка боя: плейлист из дерева мода (Data\soundplaylistmusic.lua), треки —
// из Data\sound\music с громкостью из .rat, паузы тишины между ними — из
// silence_min/silence_max самого плейлиста. Разрешение имён и запуск треков
// живут в пакете sfx; здесь — запуск на загруженную карту, ход плейлиста по
// кадру и остановка при выходе в меню.

// musicPlaylists — какой плейлист играет в бою. Геймплейный — основной; если
// мод его не переопределил, играет фоновая подложка.
var musicPlaylists = []string{"soundplaylistmusic", "soundplaylistambient"}

// startMusic запускает музыку один раз на сессию: плейлист живёт до выхода в
// меню, а перезагрузка карты не должна начинать его заново.
func (p *Gameplay) startMusic() {
	if p == nil || p.music != nil || p.hud == nil || p.hud.library == nil {
		return
	}
	tree := p.hud.library.Tree()
	for _, name := range musicPlaylists {
		playlist, err := sfx.OpenMusicPlaylist(tree, name)
		if err != nil {
			log.Printf("музыка: %v", err)
			continue
		}
		playlist.SetVolume(sfx.MusicLevel())
		p.music = playlist
		log.Printf("музыка: плейлист %s, треков %d", name, playlist.Len())
		return
	}
}

// updateMusic двигает плейлист: трек доигрывает, затем выдерживается пауза
// тишины из плейлиста и начинается следующий трек.
func (p *Gameplay) updateMusic(dt float64) {
	if p == nil || p.music == nil || dt <= 0 {
		return
	}
	p.music.Update(dt)
}

// applyMusicVolume пересчитывает громкость играющего плейлиста после
// изменения настройки музыки.
func (p *Gameplay) applyMusicVolume() {
	if p == nil || p.music == nil {
		return
	}
	p.music.SetVolume(sfx.MusicLevel())
}

// stopMusic останавливает музыку боя — при выходе в меню.
func (p *Gameplay) stopMusic() {
	if p == nil || p.music == nil {
		return
	}
	p.music.Stop()
	p.music = nil
}
