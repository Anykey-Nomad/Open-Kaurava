package screenui

import (
	"math"
	"testing"
)

// TestButtonWithoutSizeInheritsSiblingSize — у кнопки, описанной без size
// (btnDIP в gamescreen.screen), размер берётся у соседа по родителю, а
// позиция остаётся своя. Иначе арт состояний считается от вьюпорта, и
// menu_button_1 растягивается плитой на весь экран.
func TestButtonWithoutSizeInheritsSiblingSize(t *testing.T) {
	screen, err := Parse([]byte(`Screen = {
	version = 4,
	Widgets = { type = "Group", name = "root", position = {0, 0}, size = {1, 1}, Children = {
		{ type = "Group", name = "grpMenuButtons", position = {0.7, 0}, size = {0.3, 0.03}, Children = {
			{ type = "Button", name = "btnOBJ", position = {0.1, 0}, size = {0.083, 0.027} },
			{ type = "Button", name = "btnDIP", position = {0.2, 0} },
		} },
	} },
}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	button := screen.Find("btnDIP")
	if button == nil {
		t.Fatal("кнопка не найдена")
	}
	if !button.HasSize {
		t.Fatal("кнопка без size не получила размер соседа")
	}
	if button.Size != (Vec2{0.083, 0.027}) {
		t.Fatalf("размер кнопки %v, ожидался размер соседа", button.Size)
	}
	// Позиция своя: 0.7 родителя + 0.2 кнопки, верх — по родителю.
	rect := button.Layout(Rect{W: 1600, H: 870})
	want := Rect{X: 0.9 * 1600, Y: 0, W: 0.083 * 1600, H: 0.027 * 870}
	for _, pair := range [][2]float32{{rect.X, want.X}, {rect.Y, want.Y}, {rect.W, want.W}, {rect.H, want.H}} {
		if math.Abs(float64(pair[0]-pair[1])) > 0.5 {
			t.Fatalf("рамка кнопки %v, ожидалась %v", rect, want)
		}
	}
}
