package scenes

import (
	"fmt"
	"math"

	"github.com/Anykey-Nomad/Open-Kaurava/scene"
	"github.com/Anykey-Nomad/Open-Kaurava/window"
)

// Gameplay — игровая сцена: HUD, рамка выделения и возврат в меню.
type Gameplay struct {
	t float64
}

// NewGameplay создаёт игровую сцену.
func NewGameplay() *Gameplay { return &Gameplay{} }

// Name — имя сцены.
func (g *Gameplay) Name() string { return "gameplay" }

// Update — анимация и обработка ввода.
func (g *Gameplay) Update(c *scene.Ctx, dt float64) {
	g.t += dt
	if c.Key(window.KeyEscape) {
		c.Switch("menu")
	}
}

// Draw рисует фон, «мир», HUD и рамку выделения.
func (g *Gameplay) Draw(c *scene.Ctx) {
	c.Rect(0, 0, float32(c.W), float32(c.H), [4]float32{0.08, 0.10, 0.13, 1})

	// Условный «мир»: сетка плит, плавно ползущая по времени.
	cell := float32(64)
	offset := float32(math.Sin(float64(g.t)*0.5)) * cell / 2
	for x := -cell; x < float32(c.W)+cell; x += cell {
		c.Rect(x+offset, 0, 1, float32(c.H), [4]float32{0.12, 0.14, 0.18, 1})
	}
	for y := -cell; y < float32(c.H)+cell; y += cell {
		c.Rect(0, y+offset, float32(c.W), 1, [4]float32{0.12, 0.14, 0.18, 1})
	}

	// Рамка выделения мышью.
	if c.Input.Dragging {
		x := float32(c.Input.DragX)
		y := float32(c.Input.DragY)
		w := float32(c.Input.DragW)
		h := float32(c.Input.DragH)
		c.Rect(x, y, w, 2, ColTextWarn)
		c.Rect(x, y+h-2, w, 2, ColTextWarn)
		c.Rect(x, y, 2, h, ColTextWarn)
		c.Rect(x+w-2, y, 2, h, ColTextWarn)
	}

	// HUD.
	c.Rect(16, 16, float32(c.W)-32, 64, [4]float32{0, 0, 0, 0.45})
	name := Selected
	if name == "" {
		name = "не выбран"
	}
	c.Text("Сценарий: "+name, 32, 48, 26, ColText)
	c.Text(fmt.Sprintf("Мышь: %d, %d", c.Input.MouseX, c.Input.MouseY), 32, 72, 18, ColTextDim)
	c.Text(fmt.Sprintf("Кадр: %.1f с", g.t), float32(c.W)-180, 48, 20, ColTextDim)
	if SelectedPath != "" {
		c.Text(SelectedPath, 32, 100, 18, ColTextDim)
	}
	c.Text("ESC — в главное меню", 32, float32(c.H)-28, 18, ColTextDim)
}
