package ui

import (
	"image"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// testFont находит любой доступный шрифт (системный или проектный).
// Если шрифтов нет — тест пропускается, а не падает.
func testFont(t *testing.T) *Font {
	t.Helper()
	path, err := FindFont("")
	if err != nil {
		t.Skipf("шрифт не найден: %v", err)
	}
	f, err := LoadFont(path)
	if err != nil {
		t.Fatalf("LoadFont(%s): %v", path, err)
	}
	return f
}

// TestGlyphBaseline проверяет главное: глиф садится на базовую линию.
//
// dr из opentype — это image.Rectangle в ЦЕЛЫХ пикселях относительно точки
// постановки: dr.Min.Y — верх глифа (выше базовой линии, т.е. отрицательный),
// dr.Max.Y — низ. Поэтому BX/BY должны быть в пикселях, а не в 1/64 пикселя.
func TestGlyphBaseline(t *testing.T) {
	f := testFont(t)
	const px = 32
	face, err := f.Face(px)
	if err != nil {
		t.Fatalf("Face(%d): %v", px, err)
	}
	a := NewAtlas()

	for _, r := range []rune{'A', 'o', 'g', 'у', 'W'} {
		dr, _, _, _, ok := face.Glyph(fixed.Point26_6{}, r)
		if !ok {
			t.Fatalf("глиф %q не растеризован", r)
		}
		g, err := a.Glyph(f, px, r)
		if err != nil {
			t.Fatalf("Glyph(%q): %v", r, err)
		}
		if g.BY != float32(dr.Min.Y) {
			t.Errorf("%q: BY = %v, ожидалось %v (dr.Min.Y)", r, g.BY, float32(dr.Min.Y))
		}
		if g.BX != float32(dr.Min.X) {
			t.Errorf("%q: BX = %v, ожидалось %v (dr.Min.X)", r, g.BX, float32(dr.Min.X))
		}
		// Верх глифа выше базовой линии, низ — на ней или ниже (для выносных).
		if g.BY >= 0 {
			t.Errorf("%q: верх глифа %v не выше базовой линии", r, g.BY)
		}
		if bottom := g.BY + float32(g.H); bottom != float32(dr.Max.Y) {
			t.Errorf("%q: низ глифа %v, ожидалось %v (dr.Max.Y)", r, bottom, float32(dr.Max.Y))
		}
		if g.W != dr.Dx() || g.H != dr.Dy() {
			t.Errorf("%q: размер %dx%d, ожидалось %dx%d", r, g.W, g.H, dr.Dx(), dr.Dy())
		}
	}
}

// TestGlyphInAtlas проверяет, что прямоугольник глифа в атласе совпадает
// с реально записанными пикселями: запас (atlasPad) не должен смещать
// картинку относительно UV-координат.
func TestGlyphInAtlas(t *testing.T) {
	f := testFont(t)
	const px = 24
	a := NewAtlas()

	g, err := a.Glyph(f, px, 'B')
	if err != nil {
		t.Fatalf("Glyph: %v", err)
	}
	if g.W <= 0 || g.H <= 0 {
		t.Fatalf("глиф пустой: %dx%d", g.W, g.H)
	}
	if g.X < 0 || g.Y < 0 || g.X+g.W > a.W || g.Y+g.H > a.H {
		t.Fatalf("глиф %d,%d %dx%d выходит за атлас %dx%d", g.X, g.Y, g.W, g.H, a.W, a.H)
	}

	// В прямоугольнике глифа должно быть что-то нарисованное.
	ink := 0
	for y := g.Y; y < g.Y+g.H; y++ {
		for x := g.X; x < g.X+g.W; x++ {
			if a.Pix[y*a.W+x] > 0 {
				ink++
			}
		}
	}
	if ink == 0 {
		t.Error("в прямоугольнике глифа в атласе нет ни одного закрашенного пикселя")
	}

	// Левый столбец ячейки — это запас: он обязан быть пустым.
	if a.Pix[g.Y*a.W+g.X-1] != 0 {
		t.Error("пиксель слева от глифа не пуст: картинка и UV-координаты разошлись")
	}
}

// TestWhiteMarker проверяет маркер сплошной заливки: он должен быть белым
// и лежать в левом верхнем углу атласа.
func TestWhiteMarker(t *testing.T) {
	a := NewAtlas()
	if a.Pix[0] != 0xFF {
		t.Errorf("маркер в (0,0) = %d, ожидалось 255", a.Pix[0])
	}
	u, v := a.WhiteUV()
	if u <= 0 || u >= 1 || v <= 0 || v >= 1 {
		t.Errorf("UV маркера вне атласа: %v, %v", u, v)
	}
}

// TestTextWidth сверяет ширину строки с замером шрифта.
func TestTextWidth(t *testing.T) {
	f := testFont(t)
	a := NewAtlas()
	d := NewDraw(a, f)

	const px = 20
	const s = "Привет, Vulkan"
	got := d.Text(s, 10, 40, px, [4]float32{1, 1, 1, 1})
	want := f.Measure(s, px)
	// Разница в единицу — накопление округлений float32 по перу.
	if diff := got - want; diff > 1.5 || diff < -1.5 {
		t.Errorf("ширина строки = %v, ожидалось %v", got, want)
	}
	if d.Len() == 0 {
		t.Error("в батче нет вершин")
	}
}

// TestNoKerningGap проверяет, что между глифами нет разрывов из-за
// потерянного левого выноса (BX) — символы идут вплотную по перу.
func TestNoKerningGap(t *testing.T) {
	f := testFont(t)
	const px = 32
	face, err := f.Face(px)
	if err != nil {
		t.Fatalf("Face: %v", err)
	}
	a := NewAtlas()
	d := NewDraw(a, f)

	const s = "AB"
	for _, r := range s {
		dr, _, _, _, ok := face.Glyph(fixed.Point26_6{}, r)
		if !ok {
			t.Fatalf("глиф %q не растеризован", r)
		}
		g, err := a.Glyph(f, px, r)
		if err != nil {
			t.Fatalf("Glyph: %v", err)
		}
		// Левый вынос символа — целое число пикселей (не 1/64 пикселя).
		if diff := g.BX - float32(dr.Min.X); diff != 0 {
			t.Errorf("%q: BX=%v, ожидалось %v", r, g.BX, float32(dr.Min.X))
		}
	}

	// Перо сдвигается ровно на ширину символа: между квадами не должно
	// быть «дырки» из-за потерянного левого выноса.
	d.Text(s, 0, 0, px, [4]float32{1, 1, 1, 1})
	if d.Len() != 12 { // два квада по шесть вершин
		t.Fatalf("в батче %d вершин, ожидалось 12", d.Len())
	}
	gA, err := a.Glyph(f, px, 'A')
	if err != nil {
		t.Fatalf("Glyph: %v", err)
	}
	gB, err := a.Glyph(f, px, 'B')
	if err != nil {
		t.Fatalf("Glyph: %v", err)
	}
	// Левый край квада B = перо после A + левый вынос B.
	step := d.Verts[6].X - d.Verts[0].X
	want := gA.Advance + gB.BX - gA.BX
	if diff := step - want; diff > 0.01 || diff < -0.01 {
		t.Errorf("шаг пера = %v, ожидалось %v", step, want)
	}
}

// TestFontMetricsSmoke — короткая проверка, что метрики шрифта читаются.
func TestFontMetricsSmoke(t *testing.T) {
	f := testFont(t)
	face, err := f.Face(16)
	if err != nil {
		t.Fatalf("Face: %v", err)
	}
	m := face.Metrics()
	if m.Ascent <= 0 || m.Descent <= 0 {
		t.Errorf("подозрительные метрики: ascent=%d descent=%d", m.Ascent, m.Descent)
	}
	if f.Ascent(16) <= 0 || f.Descent(16) <= 0 {
		t.Error("метрики шрифта в пакете ui нечитаемы")
	}
	var _ image.Image = (*image.Alpha)(nil)
	_ = font.HintingFull
}
