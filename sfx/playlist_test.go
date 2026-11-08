package sfx

import (
	"math"
	"testing"
)

// TestParsePlaylist — таблица soundplaylistmusic.lua: имена с .wav и без
// расширения, пауза тишины, флаг порядка.
func TestParsePlaylist(t *testing.T) {
	data := []byte("playlist = \r\n{\r\n\ttracks = \r\n\t{\t\t\r\n\r\n" +
		"\t\t\"MU_IG_STR_Chaos_perc.wav\",\r\n" +
		"\t\t\"music_msd1\",\r\n" +
		"\t\t\"music_EldarThemev2\",\r\n" +
		"\t},\r\n\t\r\n\tsilence_min = 3.5,\r\n\tsilence_max = 12.0,\r\n\t\r\n\torder = true,\r\n}\r\n")
	pl := ParsePlaylist(data)
	if len(pl.Tracks) != 3 {
		t.Fatalf("треков %d, ждали 3: %v", len(pl.Tracks), pl.Tracks)
	}
	if pl.Tracks[0] != "MU_IG_STR_Chaos_perc.wav" || pl.Tracks[1] != "music_msd1" {
		t.Fatalf("имена треков прочитаны неверно: %v", pl.Tracks)
	}
	if math.Abs(pl.SilenceMin-3.5) > 0.001 || math.Abs(pl.SilenceMax-12) > 0.001 {
		t.Fatalf("пауза %g..%g, ждали 3.5..12", pl.SilenceMin, pl.SilenceMax)
	}
	if !pl.Order {
		t.Fatal("order = true должен читаться")
	}

	// Без silence и order — значения по умолчанию, треки на месте.
	bare := ParsePlaylist([]byte("playlist = { tracks = { \"only\" } }"))
	if len(bare.Tracks) != 1 || bare.SilenceMax != 0 || bare.Order {
		t.Fatalf("плейлист без настроек: %+v", bare)
	}
}

// TestResolveTrack — имя из плейлиста (.wav или без расширения) находит
// настоящий файл трека (.fda) в папке музыки.
func TestResolveTrack(t *testing.T) {
	files := map[string]string{
		"music_msd1":           `Data\sound\music\music_msd1.fda`,
		"mu_ig_str_chaos_perc": `Data\sound\music\mu_ig_str_chaos_perc.fda`,
		"music_eldarthemev2":   `Data\sound\music\music_EldarThemev2.fda`,
	}
	for _, name := range []string{"music_msd1.wav", "MUSIC_MSD1", " music_msd1 "} {
		if path, ok := resolveTrack(files, name); !ok || path != files["music_msd1"] {
			t.Fatalf("%q → %q (%v), ждали %q", name, path, ok, files["music_msd1"])
		}
	}
	if path, ok := resolveTrack(files, "music_eldarthemev2.wav"); !ok || path != files["music_eldarthemev2"] {
		t.Fatalf("имя без учёта регистра: %q (%v)", path, ok)
	}
	// Трека нет в моде — расширение не подставляется наугад.
	if path, ok := resolveTrack(files, "no_such_track.wav"); ok {
		t.Fatalf("несуществующий трек разрешён в %q", path)
	}
}
