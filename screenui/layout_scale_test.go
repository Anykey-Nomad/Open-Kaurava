package screenui

import (
	"math"
	"testing"
)

// TestLayoutShrinksAuthoredAspectToWideScreen — раскладка .screen нарисована
// под 4:3, а на 16:9 движок ужимает её на отношение аспектов (0.75) и
// прижимает блоки к ближайшему краю. По этому правилу панели в игре
// компактнее: консоль 1235×291 при 1600×900 превращается в 926×226 у
// правого нижнего угла, как в DoWDE.
func TestLayoutShrinksAuthoredAspectToWideScreen(t *testing.T) {
	screen, err := Parse([]byte(`Screen = {
	AspectRatio = 1.33333,
	Widgets = { type = "Group", name = "root", position = {0, 0}, size = {1, 1}, Children = {
		{ type = "ArtLabel", name = "taskbarRight", position = {0.2486, 0.6640}, size = {0.7719, 0.3347} },
		{ type = "ArtLabel", name = "taskbarLeft", position = {-0.0149, 0.6650}, size = {0.2959, 0.3347} },
		{ type = "ArtLabel", name = "menubarRight", position = {0.5037, 0.0000}, size = {0.5000, 0.1667} },
	} },
}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	viewport := Rect{W: 1600, H: 900}
	cases := []struct {
		name string
		want Rect
	}{
		// Правый нижний угол: ужатие от правого края и от низа.
		{"taskbarRight", Rect{X: 698.3, Y: 673.2, W: 926.3, H: 225.9}},
		// Левый нижний угол: от левого края и от низа.
		{"taskbarLeft", Rect{X: -17.9, Y: 673.9, W: 355.1, H: 225.9}},
		// Верхняя полоса: правый край, верх остаётся у нуля.
		{"menubarRight", Rect{X: 1004.0, Y: 0, W: 600.0, H: 112.5}},
	}
	for _, c := range cases {
		got := screen.Find(c.name).Layout(viewport)
		if !rectNear(got, c.want, 1) {
			t.Errorf("%s: рамка %v, ожидалась %v", c.name, got, c.want)
		}
	}
}

// TestLayoutKeepsAuthoredAspectUnscaled — если раскладка авторская под тот же
// аспект, что и окно (16:9), ужимать нечего: рамки остаются как в .screen.
func TestLayoutKeepsAuthoredAspectUnscaled(t *testing.T) {
	screen, err := Parse([]byte(`Screen = {
	AspectRatio = 1.77778,
	Widgets = { type = "Group", name = "root", position = {0, 0}, size = {1, 1}, Children = {
		{ type = "ArtLabel", name = "taskbarRight", position = {0.2486, 0.6640}, size = {0.7719, 0.3347} },
	} },
}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := screen.Find("taskbarRight").Layout(Rect{W: 1600, H: 900})
	want := Rect{X: 397.8, Y: 597.6, W: 1235, H: 301.2}
	if !rectNear(got, want, 1) {
		t.Fatalf("рамка %v, ожидалась %v", got, want)
	}
}

// TestChildFollowsBlockAnchor — внутренняя группа консоли (панель
// выделения) должна ехать вместе с её артом: край наследуется от блока,
// иначе при ужатии панель остаётся на старом месте и уезжает левее
// консоли, на карту.
func TestChildFollowsBlockAnchor(t *testing.T) {
	screen, err := Parse([]byte(`Screen = {
	AspectRatio = 1.33333,
	Widgets = { type = "Group", name = "root", position = {0, 0}, size = {1, 1}, Children = {
		{ type = "Group", name = "grpTaskbarRight", position = {0, 0}, size = {1, 1}, Children = {
			{ type = "ArtLabel", name = "artTaskbarRight", position = {0.2486, 0.6640}, size = {0.7719, 0.3347},
			  Presentation = { Art = {
				{ type = "Graphic", texture = "generic:art/ui/textures/taskbar/taskbar_share/taskbar_separated.tga" },
			} } },
			{ type = "Group", name = "grpSelection", position = {0.2816, 0.8150}, size = {0.3439, 0.1800} },
		} },
	} },
}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	viewport := Rect{W: 1600, H: 900}
	art := screen.Find("artTaskbarRight").Layout(viewport)
	selection := screen.Find("grpSelection").Layout(viewport)
	if !rectNear(selection, Rect{X: 737.9, Y: 775.1, W: 412.7, H: 121.5}, 1) {
		t.Fatalf("панель выделения %v, ожидалась внутри консоли", selection)
	}
	if selection.X < art.X || selection.X+selection.W > art.X+art.W+1 {
		t.Fatalf("панель выделения %v вышла за арт консоли %v", selection, art)
	}
	if selection.Y < art.Y || selection.Y+selection.H > art.Y+art.H+1 {
		t.Fatalf("панель выделения %v вышла за арт консоли по высоте %v", selection, art)
	}
}

func rectNear(got, want Rect, tolerance float32) bool {
	for _, pair := range [][2]float32{{got.X, want.X}, {got.Y, want.Y}, {got.W, want.W}, {got.H, want.H}} {
		if math.Abs(float64(pair[0]-pair[1])) > float64(tolerance) {
			return false
		}
	}
	return true
}
