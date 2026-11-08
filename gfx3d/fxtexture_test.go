package gfx3d

import "testing"

// Карта эффекта приходит из свойства Texture в разном виде: полным data-путём,
// путём от art/fx или голым именем файла. Пути ниже должны разрешаться в один
// и тот же набор, а расширение .tga из редактора — уступать .dds/.rtx.
func TestFxTextureCandidates(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"DATA:ART/FX/GLOW.TGA", "Data/ART/FX/GLOW.TGA"},
		{"art/fx/necron/glow.tga", "Data/art/fx/necron/glow.tga"},
		{"TEXTURES_NEW/NECRON/CRYSTAL_TOP_CTR.TGA", "Data/art/fx/TEXTURES_NEW/NECRON/CRYSTAL_TOP_CTR.TGA"},
		{"SMITE_DROP_GLOW.DDS", "Data/art/fx/SMITE_DROP_GLOW.DDS"},
	}
	for _, tc := range cases {
		got := fxTextureCandidates(tc.name)
		if len(got) == 0 || got[0] != tc.want {
			t.Fatalf("fxTextureCandidates(%q)[0] = %q, ожидался %q", tc.name, got, tc.want)
		}
		// Запасные расширения: тот же путь в нижнем регистре и .dds.
		wantDDS := "data/" + lowerStem(tc.want) + ".dds"
		found := false
		for _, candidate := range got {
			if candidate == wantDDS {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("fxTextureCandidates(%q) не пробует %q: %v", tc.name, wantDDS, got)
		}
	}
}

// lowerStem — путь без расширения в нижнем регистре и без префикса Data/.
func lowerStem(path string) string {
	lower := ""
	for i := 0; i < len(path); i++ {
		c := path[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		lower += string(c)
	}
	lower = lower[len("data/"):]
	for i := len(lower) - 1; i >= 0; i-- {
		if lower[i] == '.' {
			return lower[:i]
		}
	}
	return lower
}
