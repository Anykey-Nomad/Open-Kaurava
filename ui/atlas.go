package ui

import (
	"fmt"
	"image"

	"golang.org/x/image/math/fixed"
)

// Атлас глифов: одноканальная (A8) картинка, в которой каждый символ
// растеризован один раз и лежит в своей ячейке.
//
// Размер фиксирован: перелокация не нужна, поэтому GPU-картина создаётся
// один раз и перезаливается только при появлении новых глифов.
const (
	// AtlasSize — сторона атласа в пикселях.
	AtlasSize = 2048
	// atlasPad — запас вокруг глифа (px): нужен линейной фильтрации,
	// иначе соседние ячейки «протекают» в край глифа.
	atlasPad = 2
	// atlasFirstX — первая свободная колонка: пиксель (0,0) занят
	// белым маркером для сплошных прямоугольников UI.
	atlasFirstX = 4
)

// Glyph — квадрат глифа в атласе и его метрики.
type Glyph struct {
	// X, Y, W, H — прямоугольник глифа в атласе (пиксели).
	X, Y, W, H int
	// BX, BY — смещение левого верхнего угла квада относительно точки
	// постановки (начало строки, базовая линия), в пикселях.
	BX, BY float32
	// Advance — ширина символа (px), на которую сдвигается перо.
	Advance float32
}

// glyphKey — ключ кэша: шрифт + размер + символ.
type glyphKey struct {
	f  *Font
	px int
	r  rune
}

// Atlas — A8-атлас глифов: байты пикселей + кэш метрик.
type Atlas struct {
	W, H int
	// Pix — данные картинки (W*H байт, один канал покрытия).
	Pix []byte

	glyphs map[glyphKey]Glyph
	dirty  bool

	// Укладка по полкам: x — следующая колонка, y — верх текущей полки,
	// rowH — высота полки.
	x, y, rowH int
}

// NewAtlas создаёт пустой атлас.
//
// Пиксель (0,0) — белый маркер: им рисуются сплошные прямоугольники UI
// (см. Draw.WhiteUV), поэтому он заранее заполнен единицей.
func NewAtlas() *Atlas {
	pix := make([]byte, AtlasSize*AtlasSize)
	pix[0] = 0xFF
	return &Atlas{
		W:      AtlasSize,
		H:      AtlasSize,
		Pix:    pix,
		glyphs: map[glyphKey]Glyph{},
		x:      atlasFirstX,
	}
}

// Dirty сообщает, что в атласе появились новые глифы и его надо перезалить на GPU.
func (a *Atlas) Dirty() bool { return a.dirty }

// ClearDirty снимает признак «нужно перезалить».
func (a *Atlas) ClearDirty() { a.dirty = false }

// WhiteUV — UV белого пикселя (0,0): им рисуются сплошные прямоугольники.
func (a *Atlas) WhiteUV() (u, v float32) {
	return 0.5 / float32(a.W), 0.5 / float32(a.H)
}

// Glyph возвращает метрики глифа, растеризуя его при первом обращении.
//
// Растеризация идёт с dot в начале координат, поэтому положение глифа
// не зависит от того, где он стоит в строке.
func (a *Atlas) Glyph(f *Font, px int, r rune) (Glyph, error) {
	key := glyphKey{f: f, px: px, r: r}
	if g, ok := a.glyphs[key]; ok {
		return g, nil
	}

	face, err := f.Face(px)
	if err != nil {
		return Glyph{}, err
	}
	adv, ok := face.GlyphAdvance(r)
	if !ok {
		if r != '?' {
			return a.Glyph(f, px, '?')
		}
		return Glyph{}, fmt.Errorf("глиф %q отсутствует в шрифте %s", r, f.Path)
	}
	g := Glyph{Advance: float32(adv) / 64}

	dr, mask, maskp, _, ok := face.Glyph(fixed.Point26_6{}, r)
	if !ok {
		return g, fmt.Errorf("глиф %q не растеризован", r)
	}
	if dr.Empty() {
		// Пробел и прочие пустые символы: места в атласе не занимают.
		a.glyphs[key] = g
		return g, nil
	}

	w, h := dr.Dx(), dr.Dy()
	x, y, err := a.pack(w, h)
	if err != nil {
		return g, err
	}
	// Глиф кладём в ячейку с отступом (atlasPad) — там же, откуда его потом
	// читают UV: иначе картинка и её координаты разойдутся на pad пикселей.
	a.blit(x+atlasPad, y+atlasPad, w, h, mask, maskp)

	g.X, g.Y, g.W, g.H = x+atlasPad, y+atlasPad, w, h
	// dr — это image.Rectangle, то есть ЦЕЛЫЕ пиксели относительно точки
	// постановки (базовой линии): dr.Min.Y — верх глифа (отрицателен),
	// dr.Max.Y — низ. Делить на 64 нельзя (это не fixed point) — иначе
	// глифы садятся не на базовую линию, а верхними краями.
	g.BX, g.BY = float32(dr.Min.X), float32(dr.Min.Y)
	a.glyphs[key] = g
	a.dirty = true
	return g, nil
}

// pack резервирует место под глиф w×h и возвращает левый верхний угол
// ячейки (с учётом запаса atlasPad).
func (a *Atlas) pack(w, h int) (int, int, error) {
	cw, ch := w+2*atlasPad, h+2*atlasPad
	if cw > a.W || ch > a.H {
		return 0, 0, fmt.Errorf("глиф %dx%d не помещается в атлас %dx%d", w, h, a.W, a.H)
	}
	if a.x+cw > a.W {
		// Текущая полка кончилась — начинаем новую.
		a.x = 0
		a.y += a.rowH
		a.rowH = 0
	}
	if a.y+ch > a.H {
		return 0, 0, fmt.Errorf("атлас глифов переполнен (%dx%d)", a.W, a.H)
	}
	x, y := a.x, a.y
	a.x += cw
	if ch > a.rowH {
		a.rowH = ch
	}
	return x, y, nil
}

// blit копирует покрытие глифа в атлас.
//
// mask — маска, нарисованная шрифтом; maskp — её пиксель, соответствующий
// левому верхнему углу прямоугольника глифа.
func (a *Atlas) blit(x, y, w, h int, mask image.Image, maskp image.Point) {
	dst := y*a.W + x

	switch m := mask.(type) {
	case *image.Alpha:
		// Быстрый путь: у альфа-маски пиксели идут подряд.
		b := m.Rect.Min
		for j := 0; j < h; j++ {
			sy := maskp.Y + j - b.Y
			sx := maskp.X - b.X
			if sy < 0 || sy >= m.Rect.Dy() {
				continue
			}
			off := sy*m.Stride + sx
			if sx < 0 || off < 0 || off+w > len(m.Pix) {
				continue
			}
			copy(a.Pix[dst+j*a.W:dst+j*a.W+w], m.Pix[off:off+w])
		}
	default:
		for j := 0; j < h; j++ {
			for i := 0; i < w; i++ {
				_, _, _, alpha := mask.At(maskp.X+i, maskp.Y+j).RGBA()
				a.Pix[dst+j*a.W+i] = byte(alpha >> 8)
			}
		}
	}
}
