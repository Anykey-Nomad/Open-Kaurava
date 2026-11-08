package ui

import "testing"

// TestAtlasMipChainHalves проверяет, что цепочка получает вдвое меньшие
// уровни: арт HUD рисуется уменьшенным, и без мипов он алиасит.
func TestAtlasMipChainHalves(t *testing.T) {
	a := NewAtlas()
	chain := a.MipChain()
	if len(chain) != a.Levels() {
		t.Fatalf("уровней в цепочке %d, ожидалось %d", len(chain), a.Levels())
	}
	if chain[0].W != a.W || chain[0].H != a.H {
		t.Fatalf("нулевой уровень %dx%d, ожидался %dx%d", chain[0].W, chain[0].H, a.W, a.H)
	}
	for i := 1; i < len(chain); i++ {
		wantW := max(1, chain[i-1].W/2)
		wantH := max(1, chain[i-1].H/2)
		if chain[i].W != wantW || chain[i].H != wantH {
			t.Fatalf("уровень %d: %dx%d, ожидалось %dx%d", i, chain[i].W, chain[i].H, wantW, wantH)
		}
		if len(chain[i].Pix) != chain[i].W*chain[i].H*4 {
			t.Fatalf("уровень %d: %d байт на %dx%d", i, len(chain[i].Pix), chain[i].W, chain[i].H)
		}
	}
	last := chain[len(chain)-1]
	if last.W != 1 || last.H != 1 {
		t.Fatalf("последний уровень %dx%d, ожидался 1x1", last.W, last.H)
	}
}

// TestAtlasMipChainIgnoresTransparentBlack проверяет предумножение: чёрные
// прозрачные пиксели не должны темнить непрозрачные — именно из-за этого
// на уменьшенных панелях появлялись чёрные точки.
func TestAtlasMipChainIgnoresTransparentBlack(t *testing.T) {
	a := NewAtlas()
	// Атлас пуст (всё прозрачно-чёрное), кроме одного непрозрачного белого
	// пикселя: в уровне 1 цвет должен остаться белым, а не потемнеть до
	// четверти — иначе на уменьшенном арте HUD появляются чёрные точки.
	off := (a.W*10 + 10) * 4
	a.Pix[off], a.Pix[off+1], a.Pix[off+2], a.Pix[off+3] = 255, 255, 255, 255

	level := a.MipChain()[1]
	x, y := 10/2, 10/2
	at := (y*level.W + x) * 4
	// Альфа — среднее по четырём текселям блока: 255/4 = 63.
	if level.Pix[at+3] != 63 {
		t.Fatalf("альфа %d, ожидалась 63 (255/4)", level.Pix[at+3])
	}
	for i := 0; i < 3; i++ {
		if level.Pix[at+i] != 255 {
			t.Fatalf("канал %d = %d, ожидалось 255: прозрачный чёрный подмешался", i, level.Pix[at+i])
		}
	}
	// Соседняя ячейка осталась полностью прозрачной.
	empty := ((y*level.W + x + 1) * 4) + 3
	if level.Pix[empty] != 0 {
		t.Fatalf("альфа пустой ячейки %d, ожидался 0", level.Pix[empty])
	}
}
