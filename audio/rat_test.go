package audio

import "testing"

// TestParseRAT — .rat из Relic Audio Tool: громкость, приоритет и полифония
// читаются, комментарии и неизвестные ключи пропускаются.
func TestParseRAT(t *testing.T) {
	data := []byte("-- This file is generated automatically by the Relic Audio Tool\r\n" +
		"\r\nversion = 9\r\n\r\npriority = 100\r\nvolume = 0.650000\r\n" +
		"source = 1\r\nmaxPolyphony = 2\r\npitch = 1.000000\r\n")
	rat := ParseRAT(data)
	if !rat.HasVolume || rat.Volume != 0.65 {
		t.Fatalf("громкость %v (есть: %v), ждали 0.65", rat.Volume, rat.HasVolume)
	}
	if rat.Priority != 100 || rat.MaxPolyphony != 2 || rat.Version != 9 {
		t.Fatalf("приоритет %d, полифония %d, версия %d", rat.Priority, rat.MaxPolyphony, rat.Version)
	}

	// Без volume остаётся значение по умолчанию и признак «не задано».
	plain := ParseRAT([]byte("version = 9\r\n\r\nvolumeRand = -1.000000\r\n"))
	if plain.HasVolume || plain.Volume != 1 {
		t.Fatalf("volumeRand — не volume: %v (есть: %v)", plain.Volume, plain.HasVolume)
	}
}

// TestFDAStreamRejectsBadData — поток не притворяется, что сможет играть
// мусор: без заголовка Relic Chunky он не собирается.
func TestFDAStreamRejectsBadData(t *testing.T) {
	if _, err := NewFDAStreamBytes([]byte("not an fda at all"), true); err == nil {
		t.Fatal("на мусоре поток не должен собираться")
	}
}
