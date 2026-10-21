// Package scenes — конкретные сцены движка: главное меню, выбор сценария,
// геймплей. Сцены добавляются в менеджер (см. scene.Manager) и могут
// запрашивать переход друг на друга.
package scenes

import "github.com/Anykey-Nomad/Open-Kaurava/scene"

// Отступы колонок меню от краёв экрана: пункты главного меню стоят
// справа, список сценариев — слева.
const (
	menuMargin     = 64.0 // отступ колонки главного меню от правого края
	scenarioMargin = 72.0 // отступ списка сценариев от левого края
)

// Палитра интерфейса.
var (
	ColBG       = [4]float32{0.06, 0.07, 0.10, 1}
	ColPanel    = [4]float32{0.12, 0.13, 0.18, 1}
	ColPanelHot = [4]float32{0.19, 0.23, 0.33, 1}
	ColAccent   = [4]float32{0.35, 0.72, 1.00, 1}
	ColText     = [4]float32{0.92, 0.94, 0.98, 1}
	ColTextDim  = [4]float32{0.58, 0.62, 0.70, 1}
	ColTextWarn = [4]float32{1.00, 0.78, 0.30, 1}
)

// item — кнопка меню: прямоугольник, подпись и состояние ввода.
//
// Состояние обновляется в Update (ввод), отрисовка — в Draw: так ввод
// и картинка не расходятся по кадрам.
type item struct {
	label      string
	x, y, w, h float32

	hover   bool
	clicked bool
}

// update пересчитывает состояние кнопки по вводу кадра.
func (it *item) update(c *scene.Ctx) {
	it.hover = c.Hover(it.x, it.y, it.w, it.h)
	it.clicked = it.hover && c.ClickedAt(it.x, it.y, it.w, it.h)
}

// draw рисует кнопку с подписью по центру.
func (it *item) draw(c *scene.Ctx, px int) {
	bg := ColPanel
	if it.hover {
		bg = ColPanelHot
	}
	c.Rect(it.x, it.y, it.w, it.h, bg)
	if it.hover {
		c.Rect(it.x, it.y, it.w, 3, ColAccent)
	}
	tw := c.Measure(it.label, px)
	c.Text(it.label, it.x+(it.w-tw)/2, it.y+it.h/2+float32(px)*0.35, px, ColText)
}

// layout раскладывает кнопки в колонку по центру экрана.
func layout(items []item, w, h float32, x, y, gap float32) {
	for i := range items {
		items[i].x = x
		items[i].y = y + float32(i)*(h+gap)
		items[i].w = w
		items[i].h = h
	}
}
