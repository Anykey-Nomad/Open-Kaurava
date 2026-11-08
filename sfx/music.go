package sfx

import (
	"errors"
	"fmt"
	"math/rand"
	"strings"

	"dow-mod-studio/modapi"

	"github.com/Anykey-Nomad/Open-Kaurava/audio"
	"github.com/Anykey-Nomad/Open-Kaurava/settings"
)

// Музыка игрового процесса.
//
// Треки лежат в Data\sound\music и устроены так же, как остальные звуки игры:
// рядом с .fda стоит .rat из Relic Audio Tool (там громкость, приоритет и
// полифония трека), а пустые .con-файлы помечают папку как папку со звуком —
// это и есть «плейлист» в терминах инструментария Relic: набор файлов в
// папке плюс настройки рядом с каждым из них. Имена треков задают их роль:
// ambient_ingame_* — фоновая подложка боя, battle_ingame_* — боевая тема.
const (
	musicFolder = `Data\sound\music`

	// musicGain — общая громкость музыки поверх громкости из .rat: она
	// играет под голосами и выстрелами и не должна их заглушать.
	musicGain = 0.5
)

// gameplayTrackPrefixes — наборы треков, которые играют в бою.
var gameplayTrackPrefixes = []string{"ambient_ingame", "battle_ingame"}

// MusicTrack — трек игрового процесса и его громкость из .rat.
type MusicTrack struct {
	Path   string
	Volume float64
}

// GameplayTracks перечисляет музыку боя: .fda из папки музыки, чьё имя
// принадлежит одному из игровых наборов. Громкость берётся из одноимённого
// .rat, а без него — из _default.rat той же папки; нет и его — 1.0.
func GameplayTracks(tree *modapi.ModTree) []MusicTrack {
	if tree == nil {
		return nil
	}
	var tracks []MusicTrack
	for _, file := range modapi.FilesInFolder(tree, musicFolder) {
		if !strings.EqualFold(pathExt(file.Name), ".fda") {
			continue
		}
		name := strings.TrimSuffix(strings.ToLower(file.Name), pathExt(file.Name))
		if !gameplayTrack(name) {
			continue
		}
		tracks = append(tracks, MusicTrack{
			Path:   file.FullPath,
			Volume: trackVolume(tree, name),
		})
	}
	return tracks
}

// gameplayTrack — имя трека принадлежит игровому набору.
func gameplayTrack(name string) bool {
	for _, prefix := range gameplayTrackPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// trackVolume — громкость трека из его .rat, иначе из _default.rat папки.
func trackVolume(tree *modapi.ModTree, name string) float64 {
	paths := []string{
		musicFolder + `\` + name + ".rat",
		musicFolder + `\_default.rat`,
	}
	for _, path := range paths {
		data, err := tree.ReadFile(path)
		if err != nil || len(data) == 0 {
			continue
		}
		if rat := audio.ParseRAT(data); rat.HasVolume {
			return rat.Volume
		}
	}
	return 1
}

// StartGameplayMusic запускает музыку боя: случайный трек игрового набора,
// зацикленный, с громкостью из его .rat. Возвращает ручку трека (ею музыка
// останавливается) и путь — по нему видно, что играет.
func StartGameplayMusic(tree *modapi.ModTree) (*audio.Music, string, error) {
	tracks := GameplayTracks(tree)
	if len(tracks) == 0 {
		return nil, "", fmt.Errorf("sfx: в %s нет музыки игрового процесса", musicFolder)
	}
	start := rand.Intn(len(tracks))
	var firstErr error
	for i := 0; i < len(tracks); i++ {
		track := tracks[(start+i)%len(tracks)]
		data, err := tree.ReadFile(track.Path)
		if err != nil {
			firstErr = err
			continue
		}
		stream, err := audio.NewFDAStreamBytes(data, true)
		if err != nil {
			firstErr = err
			continue
		}
		music, err := audio.PlayMusic(stream, musicGain*track.Volume*settings.Music())
		if err != nil {
			return nil, "", err
		}
		return music, track.Path, nil
	}
	if firstErr == nil {
		firstErr = errors.New("sfx: музыкальные треки не прочитаны")
	}
	return nil, "", firstErr
}

// pathExt — расширение имени файла вместе с точкой, в нижнем регистре.
func pathExt(name string) string {
	i := strings.LastIndex(name, ".")
	if i < 0 {
		return ""
	}
	return strings.ToLower(name[i:])
}
