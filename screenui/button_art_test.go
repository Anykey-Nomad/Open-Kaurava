package screenui

import "testing"

// TestButtonArtFitsButtonAndTakesOneState — у кнопки арт состояния ложится в
// её собственный прямоугольник и рисуется только одно состояние. В .screen
// координаты Graphic заданы долями кнопки из стиля: если применять их к
// экрану, кнопки меню превращаются в плиту на пол-экрана.
func TestButtonArtFitsButtonAndTakesOneState(t *testing.T) {
	screen, err := Parse([]byte(`Screen = {
	version = 4,
	Widgets = { type = "Group", name = "root", position = {0, 0}, size = {1, 1}, Children = {
		{ type = "Group", name = "grpMenu", position = {0.5, 0}, size = {1, 0.1}, Children = {
			{ type = "Button", name = "btnDIP", position = {0.1, 0}, size = {0.2, 0.5},
			  Presentation = { Art = {
				{ type = "Graphic", position = {0.05, 0.12}, size = {0.9, 0.72},
				  texture = "generic:art/ui/textures/taskbar/taskbar_share/menu_button_1.tga",
				  states = { "Disabled" } },
				{ type = "Graphic", position = {0.05, 0.12}, size = {0.9, 0.72},
				  texture = "generic:art/ui/textures/taskbar/taskbar_share/menu_button_1.tga",
				  states = { "Normal" } },
			} } },
		} },
	} },
}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	viewport := Rect{W: 1600, H: 900}
	button := screen.Find("btnDIP")
	if button == nil {
		t.Fatal("кнопка не найдена")
	}
	if !button.IsButton() {
		t.Fatalf("тип %q не распознан как кнопка", button.Type)
	}
	images := button.TextureRects(viewport)
	if len(images) != 1 {
		t.Fatalf("арта кнопки %d, ожидалось 1 (одно состояние)", len(images))
	}
	want := button.Layout(viewport)
	if images[0].Rect != want {
		t.Fatalf("арт кнопки (%v), ожидался прямоугольник кнопки (%v)", images[0].Rect, want)
	}
}

// TestArtWithoutStatesKeepsOwnGeometry — арт без состояний (иконки, рамки)
// рисуется по своим координатам внутри владельца, как и раньше.
func TestArtWithoutStatesKeepsOwnGeometry(t *testing.T) {
	screen, err := Parse([]byte(`Screen = {
	Widgets = { type = "Group", name = "root", position = {0, 0}, size = {1, 1}, Children = {
		{ type = "Group", name = "grpInfo", position = {0, 0}, size = {0.5, 0.5},
		  Presentation = { Art = {
			{ type = "Graphic", position = {0.5, 0.5}, size = {0.5, 0.5},
			  texture = "generic:art/ui/textures/art_black_coverup.tga" },
		} } },
	} },
}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	viewport := Rect{W: 1600, H: 900}
	images := screen.Find("grpInfo").TextureRects(viewport)
	if len(images) != 1 {
		t.Fatalf("арта %d, ожидался 1", len(images))
	}
	want := Rect{X: 400, Y: 225, W: 400, H: 225}
	if images[0].Rect != want {
		t.Fatalf("арт (%v), ожидался %v", images[0].Rect, want)
	}
}
