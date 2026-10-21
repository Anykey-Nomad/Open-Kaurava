package scenes

import (
	"math"

	"github.com/Anykey-Nomad/Open-Kaurava/mods"
	"github.com/Anykey-Nomad/Open-Kaurava/scene"
)

// Menu — главное меню: заголовок и кнопки переходов.
type Menu struct {
	items []item
	t     float64
	lib   *mods.Library
}

// NewMenu создаёт главное меню.
func NewMenu(lib *mods.Library) *Menu {
	return &Menu{
		items: []item{
			{label: "Начать"},
			{label: "Сценарии"},
			{label: "Выход"},
		},
		lib: lib,
	}
}

// Name — имя сцены.
func (m *Menu) Name() string { return "menu" }

// Update обрабатывает наведение и клики по кнопкам.
func (m *Menu) Update(c *scene.Ctx, dt float64) {
	m.t += dt

	// Пункты главного меню живут в правой части экрана: колонка прижата
	// к правому краю с отступом, по центру по вертикали.
	btnW, btnH := float32(320), float32(54)
	x := float32(c.W) - btnW - menuMargin
	y := float32(c.H)/2 - 40
	layout(m.items, btnW, btnH, x, y, 16)

	for i := range m.items {
		m.items[i].update(c)
	}

	switch {
	case m.items[0].clicked: // Начать
		c.Switch("gameplay")
	case m.items[1].clicked: // Сценарии
		c.Switch("scenarios")
	case m.items[2].clicked: // Выход
		c.Quit()
	}
}

// Draw рисует фон, заголовок и кнопки.
func (m *Menu) Draw(c *scene.Ctx) {
	c.Rect(0, 0, float32(c.W), float32(c.H), ColBG)

	// Заголовок с лёгкой пульсацией подсветки.
	pulse := 0.85 + 0.15*float32(math.Sin(m.t*2))
	col := [4]float32{ColAccent[0] * pulse, ColAccent[1] * pulse, ColAccent[2] * pulse, 1}
	c.CenterText("Open-Kaurava", 150, 56, col)
	c.CenterText("движок на чистом Go: Vulkan без CGO, шрифт из TTF, сцены", 186, 20, ColTextDim)

	for i := range m.items {
		m.items[i].draw(c, 26)
	}

	c.CenterText("сцена: главное меню", float32(c.H)-36, 18, ColTextDim)
	c.CenterText(m.moduleStatus(), float32(c.H)-62, 18, ColTextDim)
}

// moduleStatus — строка о состоянии модуля игры.
func (m *Menu) moduleStatus() string {
	if m.lib == nil {
		return "модуль: не загружен"
	}
	state, err, _ := m.lib.Status()
	switch state {
	case mods.StateLoading:
		return "модуль: загрузка…"
	case mods.StateFailed:
		if err != nil {
			return "модуль: ошибка — " + err.Error()
		}
		return "модуль: ошибка"
	}
	mod := m.lib.Module()
	if mod == nil {
		return "модуль: не найден"
	}
	return "модуль: " + mod.Name + " (" + mod.UIName + ")"
}
