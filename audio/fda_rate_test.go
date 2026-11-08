package audio

import "testing"

// TestSoundPlaybackRate — файлы, которые врут о частоте: 22050 в INFO значит
// 44100 на самом деле, всё остальное играется как заявлено.
func TestSoundPlaybackRate(t *testing.T) {
	cases := []struct {
		declared int
		want     int
		why      string
	}{
		{22050, 44100, "заявлено 22050, а записано на 44100"},
		{44100, 44100, "верный заголовок не трогаем"},
		{48000, 48000, "48 кГц не трогаем"},
		{11025, 11025, "частота, которая не врала, остаётся как есть"},
		{0, 0, "пустой заголовок не угадываем"},
	}
	for _, c := range cases {
		if got := SoundPlaybackRate(c.declared); got != c.want {
			t.Errorf("SoundPlaybackRate(%d) = %d, ждали %d: %s", c.declared, got, c.want, c.why)
		}
	}
}
