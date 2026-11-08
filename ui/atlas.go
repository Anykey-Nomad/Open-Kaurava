package ui

import (
	"fmt"
	"image"
	"image/color"

	"golang.org/x/image/math/fixed"
)

// Атлас RGBA: каждый глиф и UI-ресурс занимает отдельный участок изображения.
//
// Размер фиксирован: перелокация не нужна, поэтому GPU-картина создаётся
// один раз и перезаливается только при появлении новых глифов.
const (
	// AtlasSize — сторона атласа в пикселях.
	AtlasSize = 2048
	// atlasPad — запас между ячейками для линейной фильтрации.
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

// Atlas — RGBA-атлас глифов и UI-изображений.
type Atlas struct {
	W, H int
	// Pix — плотные RGBA8-пиксели, построчно сверху вниз.
	Pix []byte

	glyphs map[glyphKey]Glyph
	images map[string]Glyph
	dirty  bool

	// Укладка по полкам: x — следующая колонка, y — верх текущей полки,
	// rowH — высота полки.
	x, y, rowH int
}

// NewAtlas создаёт пустой атлас.
//
// Пиксель (0,0) — непрозрачный белый маркер для сплошных прямоугольников.
func NewAtlas() *Atlas {
	pix := make([]byte, AtlasSize*AtlasSize*4)
	pix[0], pix[1], pix[2], pix[3] = 0xFF, 0xFF, 0xFF, 0xFF
	return &Atlas{
		W:      AtlasSize,
		H:      AtlasSize,
		Pix:    pix,
		glyphs: map[glyphKey]Glyph{},
		images: map[string]Glyph{},
		x:      atlasFirstX,
	}
}

// Dirty сообщает, что в атласе появились новые глифы и его надо перезалить на GPU.
func (a *Atlas) Dirty() bool { return a.dirty }

// LookupImage возвращает ранее загруженный регион без изменения атласа.
func (a *Atlas) LookupImage(key string) (Glyph, bool) {
	region, ok := a.images[key]
	return region, ok
}

// ClearDirty снимает признак «нужно перезалить».
func (a *Atlas) ClearDirty() { a.dirty = false }

// MipLevel — один уровень мип-цепочки атласа.
type MipLevel struct {
	W, H int
	// Pix — плотные RGBA8-пиксели уровня, построчно сверху вниз.
	Pix []byte
}

// Levels возвращает число уровней полной мип-цепочки атласа.
func (a *Atlas) Levels() int {
	side := a.W
	if a.H > side {
		side = a.H
	}
	levels := 1
	for side > 1 {
		side /= 2
		levels++
	}
	return levels
}

// MipChain строит мип-уровни атласа для GPU.
//
// Нулевой уровень — сам Pix (без копии). Каждый следующий — усреднение 2×2,
// причём цвет считается предумноженным на альфу: иначе прозрачные пиксели
// подмешивали бы чёрный и на уменьшенном арте HUD появлялась бы тёмная
// кайма (те самые чёрные точки).
func (a *Atlas) MipChain() []MipLevel {
	levels := make([]MipLevel, 0, a.Levels())
	levels = append(levels, MipLevel{W: a.W, H: a.H, Pix: a.Pix})
	src, w, h := a.Pix, a.W, a.H
	for w > 1 || h > 1 {
		nw, nh := w/2, h/2
		if nw < 1 {
			nw = 1
		}
		if nh < 1 {
			nh = 1
		}
		dst := make([]byte, nw*nh*4)
		for y := 0; y < nh; y++ {
			for x := 0; x < nw; x++ {
				var r, g, b, alpha, count uint32
				for dy := 0; dy < 2; dy++ {
					sy := y*2 + dy
					if sy >= h {
						continue
					}
					for dx := 0; dx < 2; dx++ {
						sx := x*2 + dx
						if sx >= w {
							continue
						}
						off := (sy*w + sx) * 4
						pa := uint32(src[off+3])
						r += uint32(src[off]) * pa / 255
						g += uint32(src[off+1]) * pa / 255
						b += uint32(src[off+2]) * pa / 255
						alpha += pa
						count++
					}
				}
				off := (y*nw + x) * 4
				if count == 0 {
					continue
				}
				if alpha > 0 {
					dst[off+0] = byte(min(255, r*255/alpha))
					dst[off+1] = byte(min(255, g*255/alpha))
					dst[off+2] = byte(min(255, b*255/alpha))
				}
				dst[off+3] = byte(min(255, alpha/count))
			}
		}
		levels = append(levels, MipLevel{W: nw, H: nh, Pix: dst})
		src, w, h = dst, nw, nh
	}
	return levels
}

// WhiteUV — UV белого пикселя (0,0): им рисуются сплошные прямоугольники.
func (a *Atlas) WhiteUV() (u, v float32) {
	return 0.5 / float32(a.W), 0.5 / float32(a.H)
}

// Image добавляет картинку в атлас один раз на ключ и возвращает её область.
func (a *Atlas) Image(key string, src image.Image) (Glyph, error) {
	if region, ok := a.images[key]; ok {
		return region, nil
	}
	if src == nil || key == "" {
		return Glyph{}, fmt.Errorf("пустая UI-картинка")
	}
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return Glyph{}, fmt.Errorf("пустой размер UI-картинки %dx%d", w, h)
	}
	x, y, err := a.pack(w, h+atlasPad*2)
	if err != nil {
		return Glyph{}, err
	}
	x += atlasPad
	y += atlasPad
	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			pixel := color.NRGBAModel.Convert(src.At(bounds.Min.X+col, bounds.Min.Y+row)).(color.NRGBA)
			off := ((y+row)*a.W + x + col) * 4
			a.Pix[off+0] = pixel.R
			a.Pix[off+1] = pixel.G
			a.Pix[off+2] = pixel.B
			a.Pix[off+3] = pixel.A
		}
	}
	region := Glyph{X: x, Y: y, W: w, H: h}
	a.images[key] = region
	a.dirty = true
	return region, nil
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
	// Глиф кладём с отступом: он отделяет UV от соседних ячеек.
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

// blit копирует маску глифа в RGBA-атлас.
func (a *Atlas) blit(x, y, w, h int, mask image.Image, maskp image.Point) {
	for j := 0; j < h; j++ {
		for i := 0; i < w; i++ {
			_, _, _, alpha := mask.At(maskp.X+i, maskp.Y+j).RGBA()
			off := ((y+j)*a.W + x + i) * 4
			a.Pix[off+0], a.Pix[off+1], a.Pix[off+2], a.Pix[off+3] = 0xFF, 0xFF, 0xFF, byte(alpha>>8)
		}
	}
}
