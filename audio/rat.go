package audio

import (
	"strconv"
	"strings"
)

// RAT — настройки воспроизведения звука из файла .rat (Relic Audio Tool).
// Файл текстовый: строки «ключ = значение» и комментарии «--». Он лежит рядом
// с .fda и тем же именем; без него игра берёт _default.rat своей папки.
type RAT struct {
	Version      int
	Volume       float64
	HasVolume    bool
	Priority     int
	MaxPolyphony int
	Source       int
}

// ParseRAT читает .rat. Файл маленький и текстовый, поэтому парсер без
// внешних зависимостей: неизвестные ключи пропускаются, числа в десятичной
// записи.
func ParseRAT(data []byte) RAT {
	rat := RAT{Volume: 1, Priority: 100, MaxPolyphony: 1}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "version":
			rat.Version = parseInt(value)
		case "volume":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				rat.Volume, rat.HasVolume = v, true
			}
		case "priority":
			rat.Priority = parseInt(value)
		case "maxpolyphony":
			rat.MaxPolyphony = parseInt(value)
		case "source":
			rat.Source = parseInt(value)
		}
	}
	return rat
}

// parseInt — целое из поля .rat: мусор считается нулём, а не ошибкой.
func parseInt(s string) int {
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return v
}
