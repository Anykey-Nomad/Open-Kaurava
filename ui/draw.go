package ui

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"

	"dow-mod-studio/imgdec"
)

// Vertex — вершина UI-квадра.
//
// Раскладка обязана совпадать с internal/shaders/quad.vert:
// позиция в пикселях (2 float), UV в атласе (2 float), цвет (4 float) —
// 32 байта на вершину.
type Vertex struct {
	X, Y       float32
	U, V       float32
	R, G, B, A float32
}

// VertexSize — размер вершины в байтах (совпадает с sizeof(Vertex) в Go).
const VertexSize = 32

// Draw — батч кадра: прямоугольники и строки собираются в список вершин,
// который Renderer затем рисует одним вызовом vkCmdDraw.
//
// Система координат — пиксели кадра, начало в левом верхнем углу,
// Y вниз (как в оконных системах).
type Draw struct {
	// Atlas — источник глифов (обязателен для Rect: белый пиксель).
	Atlas *Atlas
	// Font — шрифт по умолчанию для Text.
	Font *Font

	// Verts — накопленные вершины кадра.
	Verts []Vertex
	// Dropped — сколько квадов не влезло в буфер (информация для лога).
	Dropped int
}

// NewDraw создаёт батчер.
func NewDraw(atlas *Atlas, f *Font) *Draw {
	return &Draw{Atlas: atlas, Font: f}
}

// Reset очищает батч перед новым кадром.
func (d *Draw) Reset() {
	d.Verts = d.Verts[:0]
	d.Dropped = 0
}

// Len возвращает число вершин в батче.
func (d *Draw) Len() int { return len(d.Verts) }

// quad добавляет прямоугольник с заданными UV.
func (d *Draw) quad(x, y, w, h, u0, v0, u1, v1 float32, c [4]float32) {
	x1, y1 := x+w, y+h
	d.Verts = append(d.Verts,
		Vertex{x, y, u0, v0, c[0], c[1], c[2], c[3]},
		Vertex{x1, y, u1, v0, c[0], c[1], c[2], c[3]},
		Vertex{x1, y1, u1, v1, c[0], c[1], c[2], c[3]},
		Vertex{x, y, u0, v0, c[0], c[1], c[2], c[3]},
		Vertex{x1, y1, u1, v1, c[0], c[1], c[2], c[3]},
		Vertex{x, y1, u0, v1, c[0], c[1], c[2], c[3]},
	)
}

// LoadImage decodes a UI texture (TGA/DDS/RTX/PNG) into the frame's shared atlas.
func (d *Draw) LoadImage(key string, data []byte) (Glyph, error) {
	if d == nil || d.Atlas == nil {
		return Glyph{}, fmt.Errorf("ui: frame has no atlas")
	}
	if region, ok := d.Atlas.LookupImage(key); ok {
		return region, nil
	}
	img, err := decodeImage(data)
	if err != nil {
		return Glyph{}, fmt.Errorf("ui: decode %s: %w", key, err)
	}
	return d.Atlas.Image(key, img)
}

// decodeImage декодирует файл текстуры UI. Игровые форматы определяются по
// magic-байтам: .rtx — контейнер "Relic Chunky" (скомпилированные текстуры
// ремастера), .dds — заголовок DDS. Затем пробуется TGA и кодеки image
// (PNG/JPEG).
func decodeImage(data []byte) (image.Image, error) {
	switch {
	case len(data) >= 12 && string(data[:12]) == "Relic Chunky":
		return imgdec.Decode("rtx", data)
	case len(data) >= 4 && string(data[:4]) == "DDS ":
		return imgdec.Decode("dds", data)
	}
	if img, err := DecodeTGA(data); err == nil {
		return img, nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}

// Image рисует текстурное изображение из атласа с заданными размерами.
func (d *Draw) Image(region Glyph, x, y, w, h float32, tint [4]float32) {
	if w <= 0 || h <= 0 || d.Atlas == nil || region.W <= 0 || region.H <= 0 {
		return
	}
	u0 := float32(region.X) / float32(d.Atlas.W)
	v0 := float32(region.Y) / float32(d.Atlas.H)
	u1 := float32(region.X+region.W) / float32(d.Atlas.W)
	v1 := float32(region.Y+region.H) / float32(d.Atlas.H)
	d.quad(x, y, w, h, u0, v0, u1, v1, tint)
}

// DecodeImage декодирует картинку так же, как LoadImage: TGA, а также
// .rtx/.dds ремастера. Нужен там, где важны пиксели, а не готовая область
// атласа (арт панелей: над панелью лежит прозрачная надставка кадра).
func DecodeImage(data []byte) (image.Image, error) {
	return decodeImage(data)
}

// Rect рисует сплошной прямоугольник (цвет берётся из атласа белый пиксель).
func (d *Draw) Rect(x, y, w, h float32, c [4]float32) {
	if w <= 0 || h <= 0 || d.Atlas == nil {
		return
	}
	u, v := d.Atlas.WhiteUV()
	d.quad(x, y, w, h, u, v, u, v, c)
}

// Text рисует строку шрифтом по умолчанию.
//
// y — базовая линия (не верх строки). Возвращает ширину строки в пикселях.
func (d *Draw) Text(s string, x, y float32, px int, c [4]float32) float32 {
	return d.TextFont(d.Font, s, x, y, px, c)
}

// TextFont рисует строку конкретным шрифтом. y — базовая линия.
func (d *Draw) TextFont(f *Font, s string, x, y float32, px int, c [4]float32) float32 {
	if f == nil || d.Atlas == nil || s == "" {
		return 0
	}
	pen := x
	for _, r := range s {
		g, err := d.Atlas.Glyph(f, px, r)
		if err != nil {
			// Атлас переполнен или нет глифа: рисуем пробел, не срывая кадр.
			pen += g.Advance
			continue
		}
		if g.W > 0 && g.H > 0 {
			u0 := float32(g.X) / float32(d.Atlas.W)
			v0 := float32(g.Y) / float32(d.Atlas.H)
			u1 := float32(g.X+g.W) / float32(d.Atlas.W)
			v1 := float32(g.Y+g.H) / float32(d.Atlas.H)
			d.quad(pen+g.BX, y+g.BY, float32(g.W), float32(g.H), u0, v0, u1, v1, c)
		}
		pen += g.Advance
	}
	return pen - x
}

// Measure возвращает ширину строки шрифтом по умолчанию.
func (d *Draw) Measure(s string, px int) float32 {
	if d.Font == nil {
		return 0
	}
	return d.Font.Measure(s, px)
}
