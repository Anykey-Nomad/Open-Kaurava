// Package scene — менеджер сцен: интерфейс сцены, контекст кадра и
// переходы между сценами (в том числе через экран загрузки).
//
// Движок (main) держит окно, Vulkan и рендерер UI, а сцены получают
// готовый батчер для отрисовки и снимок ввода за кадр.
package scene

import (
	"github.com/Anykey-Nomad/Open-Kaurava/ui"
	"github.com/Anykey-Nomad/Open-Kaurava/window"
)

// Click — клик мыши, зафиксированный в этом кадре.
type Click struct {
	Button int
	X, Y   int
}

// OnUIClick — звук щелчка по элементу интерфейса. Вызывается один раз
// за кадр, на первом попавшем клике ЛКМ внутри прямоугольника кнопки
// (см. ClickedAt). Ставится приложением (пакет scenes — sfx.UIClick);
// nil в тестах и когда звуки не подключены.
var OnUIClick func()

// Input — снимок ввода за кадр.
type Input struct {
	// MouseX, MouseY — текущая позиция курсора в пикселях кадра.
	MouseX, MouseY int
	// Clicks — клики, случившиеся в этом кадре.
	Clicks []Click
	// Keys — клавиши, нажатые в этом кадре.
	Keys []window.Key
	// Scroll — прокрутка колесом мыши за кадр: > 0 — вверх, < 0 — вниз.
	Scroll float64
	// MoveX, MoveY — смещение мыши за кадр (относительно прошлого кадра).
	// Нужно камере: по нему считается вращение и сдвиг вида.
	MoveX, MoveY int
	// Buttons — зажатые кнопки мыши: бит N означает кнопку N
	// (1 — левая, 2 — средняя, 3 — правая).
	Buttons int
	// Held — клавиши, зажатые в данный момент (в Keys — только нажатия).
	Held []window.Key
	// Dragging — идёт перетаскивание; рамка в DragX..DragH.
	Dragging                   bool
	DragX, DragY, DragW, DragH int
	// DragEnded — перетаскивание завершено в этом кадре; DragButton — его
	// кнопка, DragFromX/Y — точка нажатия, а DragX..DragH — итоговая рамка.
	DragEnded            bool
	DragButton           int
	DragFromX, DragFromY int
	// DragCurrentButton — кнопка идущего перетаскивания: рамку выделения
	// рисует только левая, камера крутится правой.
	DragCurrentButton int
}

// Ctx — контекст кадра, доступный сцене.
type Ctx struct {
	// UI — батчер отрисовки (прямоугольники и текст).
	UI *ui.Draw
	// W, H — размер кадра в пикселях.
	W, H int
	// Input — ввод за кадр.
	Input Input
	// DT — длительность кадра в секундах.
	DT float64
	// Clear — цвет очистки кадра (движок подставляет свой, если сцена
	// его не меняет). Читается после Draw.
	Clear [4]float32
	// Switch — запросить переход на другую сцену (менеджер выполнит
	// переход на следующем кадре, при необходимости через загрузку).
	Switch func(name string)
	// Quit — попросить приложение закрыться.
	Quit func()
	// uiClicked — щелчок по интерфейсу уже озвучен в этом кадре
	// (контекст создаётся заново каждый кадр, см. main).
	uiClicked bool
}

// Rect рисует сплошной прямоугольник.
func (c *Ctx) Rect(x, y, w, h float32, col [4]float32) {
	if c.UI != nil {
		c.UI.Rect(x, y, w, h, col)
	}
}

// Text рисует строку; y — базовая линия. Возвращает ширину строки.
func (c *Ctx) Text(s string, x, y float32, px int, col [4]float32) float32 {
	if c.UI == nil {
		return 0
	}
	return c.UI.Text(s, x, y, px, col)
}

// Measure возвращает ширину строки в пикселях.
func (c *Ctx) Measure(s string, px int) float32 {
	if c.UI == nil {
		return 0
	}
	return c.UI.Measure(s, px)
}

// CenterText рисует строку по центру кадра. y — базовая линия.
func (c *Ctx) CenterText(s string, y float32, px int, col [4]float32) float32 {
	w := c.Measure(s, px)
	return c.Text(s, (float32(c.W)-w)/2, y, px, col)
}

// Hover сообщает, что курсор внутри прямоугольника.
func (c *Ctx) Hover(x, y, w, h float32) bool {
	mx, my := float32(c.Input.MouseX), float32(c.Input.MouseY)
	return mx >= x && mx < x+w && my >= y && my < y+h
}

// ClickedAt сообщает, что в этом кадре был клик внутри прямоугольника.
// Первый клик ЛКМ в прямоугольнике озвучивается через OnUIClick — так
// щелчок звучит один раз, даже если прямоугольник проверяют несколько
// кнопок. Правая кнопка звук не даёт: ею отдают приказы.
func (c *Ctx) ClickedAt(x, y, w, h float32) bool {
	for _, cl := range c.Input.Clicks {
		mx, my := float32(cl.X), float32(cl.Y)
		if mx >= x && mx < x+w && my >= y && my < y+h {
			if cl.Button == 1 && OnUIClick != nil && !c.uiClicked {
				c.uiClicked = true
				OnUIClick()
			}
			return true
		}
	}
	return false
}

// Key сообщает, что клавиша была нажата в этом кадре.
func (c *Ctx) Key(k window.Key) bool {
	for _, key := range c.Input.Keys {
		if key == k {
			return true
		}
	}
	return false
}

// Scene — сцена движка.
//
// Update получает ввод и время, Draw — батчер для отрисовки. Сцены
// регистрируются в менеджере по имени; переходы выполняет менеджер.
type Scene interface {
	// Name — уникальное имя сцены.
	Name() string
	// Update — логика кадра (ввод, анимация, переходы).
	Update(c *Ctx, dt float64)
	// Draw — отрисовка кадра.
	Draw(c *Ctx)
}
