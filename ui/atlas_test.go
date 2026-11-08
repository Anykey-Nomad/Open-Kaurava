package ui

import (
	"image"
	"image/color"
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
		if g.BY != float32(dr.Min.Y) || g.BX != float32(dr.Min.X) {
			t.Errorf("%q: смещение (%v,%v), ожидалось (%v,%v)", r, g.BX, g.BY, dr.Min.X, dr.Min.Y)
		}
		if g.W != dr.Dx() || g.H != dr.Dy() {
			t.Errorf("%q: размер %dx%d, ожидалось %dx%d", r, g.W, g.H, dr.Dx(), dr.Dy())
		}
	}
}

func TestGlyphInAtlas(t *testing.T) {
	f := testFont(t)
	a := NewAtlas()
	g, err := a.Glyph(f, 24, 'B')
	if err != nil {
		t.Fatalf("Glyph: %v", err)
	}
	if g.W <= 0 || g.H <= 0 || g.X < 0 || g.Y < 0 || g.X+g.W > a.W || g.Y+g.H > a.H {
		t.Fatalf("invalid glyph bounds: %+v", g)
	}
	ink := 0
	for y := g.Y; y < g.Y+g.H; y++ {
		for x := g.X; x < g.X+g.W; x++ {
			if a.Pix[(y*a.W+x)*4+3] > 0 {
				ink++
			}
		}
	}
	if ink == 0 {
		t.Error("в прямоугольнике глифа нет пикселей")
	}
	if a.Pix[(g.Y*a.W+g.X-1)*4+3] != 0 {
		t.Error("пиксель слева от глифа не пуст")
	}
}

func TestWhiteMarker(t *testing.T) {
	a := NewAtlas()
	if got := a.Pix[:4]; got[0] != 255 || got[1] != 255 || got[2] != 255 || got[3] != 255 {
		t.Errorf("white marker RGBA = %v", got)
	}
	u, v := a.WhiteUV()
	if u <= 0 || u >= 1 || v <= 0 || v >= 1 {
		t.Errorf("UV маркера вне атласа: %v, %v", u, v)
	}
}

func TestAtlasImageKeepsRGBAAndCachesKey(t *testing.T) {
	a := NewAtlas()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 210, G: 30, B: 70, A: 128})
	img.SetNRGBA(1, 0, color.NRGBA{R: 20, G: 190, B: 80, A: 255})
	region, err := a.Image("panel", img)
	if err != nil {
		t.Fatal(err)
	}
	again, err := a.Image("panel", image.NewRGBA(image.Rect(0, 0, 5, 5)))
	if err != nil {
		t.Fatal(err)
	}
	if again != region {
		t.Fatalf("cache returned different region: %+v vs %+v", region, again)
	}
	off := (region.Y*a.W + region.X) * 4
	if got := a.Pix[off : off+4]; got[0] != 210 || got[1] != 30 || got[2] != 70 || got[3] != 128 {
		t.Errorf("first pixel RGBA = %v", got)
	}
}

func TestTextWidth(t *testing.T) {
	f := testFont(t)
	d := NewDraw(NewAtlas(), f)
	const px = 20
	const s = "Привет, Vulkan"
	got := d.Text(s, 10, 40, px, [4]float32{1, 1, 1, 1})
	want := f.Measure(s, px)
	if diff := got - want; diff > 1.5 || diff < -1.5 {
		t.Errorf("ширина строки = %v, ожидалось %v", got, want)
	}
	if d.Len() == 0 {
		t.Error("в батче нет вершин")
	}
}

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
		if g.BX != float32(dr.Min.X) {
			t.Errorf("%q: BX=%v, ожидалось %v", r, g.BX, dr.Min.X)
		}
	}
	d.Text(s, 0, 0, px, [4]float32{1, 1, 1, 1})
	if d.Len() != 12 {
		t.Fatalf("в батче %d вершин, ожидалось 12", d.Len())
	}
	gA, err := a.Glyph(f, px, 'A')
	if err != nil {
		t.Fatalf("Glyph A: %v", err)
	}
	gB, err := a.Glyph(f, px, 'B')
	if err != nil {
		t.Fatalf("Glyph B: %v", err)
	}
	step := d.Verts[6].X - d.Verts[0].X
	want := gA.Advance + gB.BX - gA.BX
	if diff := step - want; diff > 0.01 || diff < -0.01 {
		t.Errorf("шаг пера = %v, ожидалось %v", step, want)
	}
}

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
