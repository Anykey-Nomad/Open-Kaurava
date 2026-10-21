package ui

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
