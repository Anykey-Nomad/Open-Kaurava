package sfx

import (
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"dow-mod-studio/modapi"

	"github.com/Anykey-Nomad/Open-Kaurava/audio"
	"github.com/Anykey-Nomad/Open-Kaurava/settings"
)

// Плейлисты музыки игры: Data\soundplaylist*.lua — те же таблицы Lua, что
// читает движок. В геймплее играет soundplaylistmusic.lua, в переднем меню —
// soundplaylistfe*.lua, фоновая подложка — soundplaylistambient.lua. Формат:
//
//	playlist =
//	{
//		tracks = { "Ambient-Prototype-Battle BG", "ambient_desert", … },
//		silence_min = 3.0,
//		silence_max = 10.0,
//		order = false,
//	}
//
// Имена в tracks записаны без расширения или с .wav, хотя в архивах треки
// лежат как .fda. Поэтому имя сопоставляется с настоящими файлами папки
// музыки по основе без расширения — расширение не подставляется наугад, а
// берётся у того файла, который действительно есть.
//
// silence_min и silence_max — пауза тишины между треками: движок держит её
// после каждого трека, поэтому она есть и здесь.

// musicPlaylistFiles — папка музыки и папка плейлистов в дереве мода.
const musicPlaylistDir = `Data`

// Playlist — разобранный плейлист музыки.
type Playlist struct {
	Tracks     []string
	SilenceMin float64
	SilenceMax float64
	Order      bool
}

// ParsePlaylist читает таблицу плейлиста. Разбор ручной: файл — данные, а не
// код, и состоит из списка строк и трёх чисел; полноценный Lua-интерпретатор
// здесь не нужен.
func ParsePlaylist(data []byte) Playlist {
	text := string(data)
	pl := Playlist{}
	if body, ok := tableBody(text, "tracks"); ok {
		for {
			open := strings.IndexByte(body, '"')
			if open < 0 {
				break
			}
			body = body[open+1:]
			end := strings.IndexByte(body, '"')
			if end < 0 {
				break
			}
			if name := strings.TrimSpace(body[:end]); name != "" {
				pl.Tracks = append(pl.Tracks, name)
			}
			body = body[end+1:]
		}
	}
	pl.SilenceMin = luaNumber(text, "silence_min")
	pl.SilenceMax = luaNumber(text, "silence_max")
	pl.Order = luaFlag(text, "order")
	if pl.SilenceMax < pl.SilenceMin {
		pl.SilenceMax = pl.SilenceMin
	}
	return pl
}

// tableBody — содержимое фигурных скобок после «ключ =».
func tableBody(text, key string) (string, bool) {
	index := strings.Index(text, key)
	if index < 0 {
		return "", false
	}
	rest := text[index+len(key):]
	open := strings.IndexByte(rest, '{')
	if open < 0 {
		return "", false
	}
	rest = rest[open+1:]
	end := strings.IndexByte(rest, '}')
	if end < 0 {
		return rest, true
	}
	return rest[:end], true
}

// luaNumber — число после «ключ =» (поле плейлиста).
func luaNumber(text, key string) float64 {
	index := strings.Index(text, key)
	if index < 0 {
		return 0
	}
	rest := text[index+len(key):]
	equal := strings.IndexByte(rest, '=')
	if equal < 0 {
		return 0
	}
	rest = strings.TrimSpace(rest[equal+1:])
	end := 0
	for end < len(rest) && (rest[end] == '.' || rest[end] == '-' || (rest[end] >= '0' && rest[end] <= '9')) {
		end++
	}
	value, err := strconv.ParseFloat(rest[:end], 64)
	if err != nil {
		return 0
	}
	return value
}

// luaFlag — true/false после «ключ =».
func luaFlag(text, key string) bool {
	index := strings.Index(text, key)
	if index < 0 {
		return false
	}
	rest := text[index+len(key):]
	equal := strings.IndexByte(rest, '=')
	if equal < 0 {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(rest[equal+1:]), "true")
}

// musicFiles — файлы папки музыки по основе имени без расширения, в нижнем
// регистре. .fda приоритетнее: если рядом лежит и .wav, играется сжатый.
func musicFiles(tree *modapi.ModTree) map[string]string {
	files := map[string]string{}
	for _, file := range modapi.FilesInFolder(tree, musicFolder) {
		ext := pathExt(file.Name)
		if ext != ".fda" && ext != ".wav" && ext != ".ogg" {
			continue
		}
		stem := strings.ToLower(strings.TrimSuffix(file.Name, ext))
		if old, seen := files[stem]; seen && strings.EqualFold(pathExt(old), ".fda") {
			continue
		}
		files[stem] = file.FullPath
	}
	return files
}

// resolveTrack — файл трека по имени из плейлиста: имя сравнивается без
// расширения (.wav в плейлисте, .fda в архиве — это один и тот же трек).
func resolveTrack(files map[string]string, name string) (string, bool) {
	stem := strings.ToLower(strings.TrimSpace(name))
	if i := strings.LastIndex(stem, "."); i > 0 {
		stem = stem[:i]
	}
	path, ok := files[stem]
	return path, ok
}

// MusicPlaylist — играющая музыка: треки плейлиста по кругу, с паузами
// тишины между ними.
type MusicPlaylist struct {
	tree   *modapi.ModTree
	tracks []MusicTrack

	silenceMin float64
	silenceMax float64
	order      bool

	next      int
	current   *audio.Music
	remaining float64
	gap       float64
	volume    float64
}

// OpenMusicPlaylist собирает плейлист из Data\<name>.lua (например
// "soundplaylistmusic") и разрешает его треки в файлы папки музыки. Треки,
// которых в моде нет, пропускаются.
func OpenMusicPlaylist(tree *modapi.ModTree, name string) (*MusicPlaylist, error) {
	if tree == nil {
		return nil, errors.New("sfx: нет дерева мода")
	}
	data, err := tree.ReadFile(musicPlaylistDir + `\` + name + ".lua")
	if err != nil {
		return nil, fmt.Errorf("sfx: плейлист %s: %w", name, err)
	}
	parsed := ParsePlaylist(data)
	if len(parsed.Tracks) == 0 {
		return nil, fmt.Errorf("sfx: в плейлисте %s нет треков", name)
	}
	files := musicFiles(tree)
	list := &MusicPlaylist{
		tree:       tree,
		silenceMin: parsed.SilenceMin,
		silenceMax: parsed.SilenceMax,
		order:      parsed.Order,
		volume:     MusicLevel(),
	}
	for _, track := range parsed.Tracks {
		path, ok := resolveTrack(files, track)
		if !ok {
			continue
		}
		list.tracks = append(list.tracks, MusicTrack{Path: path, Volume: trackVolume(tree, path)})
	}
	if len(list.tracks) == 0 {
		return nil, fmt.Errorf("sfx: ни один трек плейлиста %s не найден в %s", name, musicFolder)
	}
	if !list.order {
		list.shuffle()
	}
	return list, nil
}

// shuffle перемешивает порядок треков: order = false в плейлисте значит
// случайный выбор, а не порядок строк.
func (pl *MusicPlaylist) shuffle() {
	rand.Shuffle(len(pl.tracks), func(i, j int) {
		pl.tracks[i], pl.tracks[j] = pl.tracks[j], pl.tracks[i]
	})
}

// Len — сколько треков плейлиста нашлось в моде.
func (pl *MusicPlaylist) Len() int {
	if pl == nil {
		return 0
	}
	return len(pl.tracks)
}

// MusicLevel — общая громкость музыки: базовая громкость боя вместе с
// пользовательской настройкой (ползунок «Музыка» в меню паузы).
func MusicLevel() float64 {
	return musicGain * settings.Music()
}

// SetVolume задаёт общую громкость музыки поверх громкости трека из .rat.
func (pl *MusicPlaylist) SetVolume(v float64) {
	if pl == nil {
		return
	}
	pl.volume = v
	if pl.current != nil {
		pl.current.SetVolume(v * pl.Track().Volume)
	}
}

// Track — играющий трек (пустой, пока идёт тишина).
func (pl *MusicPlaylist) Track() MusicTrack {
	if pl == nil || pl.current == nil || len(pl.tracks) == 0 {
		return MusicTrack{}
	}
	return pl.tracks[(pl.next+len(pl.tracks)-1)%len(pl.tracks)]
}

// Update двигает плейлист: трек доигрывает, затем выдерживается пауза
// тишины из плейлиста, затем начинается следующий.
func (pl *MusicPlaylist) Update(dt float64) {
	if pl == nil || dt <= 0 {
		return
	}
	if pl.current != nil {
		pl.remaining -= dt
		if pl.remaining > 0 {
			return
		}
		pl.current.Stop()
		pl.current = nil
		pl.gap = pl.silence()
		return
	}
	if pl.gap > 0 {
		pl.gap -= dt
		if pl.gap > 0 {
			return
		}
		pl.gap = 0
	}
	pl.startNext()
}

// silence — пауза тишины между треками из плейлиста (0 — играть подряд).
func (pl *MusicPlaylist) silence() float64 {
	if pl.silenceMax <= 0 {
		return 0
	}
	if pl.silenceMax <= pl.silenceMin {
		return pl.silenceMin
	}
	return pl.silenceMin + rand.Float64()*(pl.silenceMax-pl.silenceMin)
}

// startNext запускает следующий трек; если он не прочитался — пробует
// следующие, чтобы битый файл не останавливал музыку.
func (pl *MusicPlaylist) startNext() {
	for i := 0; i < len(pl.tracks); i++ {
		track := pl.tracks[pl.next%len(pl.tracks)]
		pl.next++
		data, err := pl.tree.ReadFile(track.Path)
		if err != nil {
			continue
		}
		stream, err := audio.NewFDAStreamBytes(data, false)
		if err != nil {
			continue
		}
		music, err := audio.PlayMusic(stream, pl.volume*track.Volume)
		if err != nil {
			return
		}
		pl.current = music
		pl.remaining = music.Duration()
		return
	}
}

// Stop останавливает музыку и забывает текущий трек.
func (pl *MusicPlaylist) Stop() {
	if pl == nil {
		return
	}
	if pl.current != nil {
		pl.current.Stop()
		pl.current = nil
	}
	pl.gap = 0
}
