package scenes

import (
	"testing"

	"github.com/Anykey-Nomad/Open-Kaurava/scene"
)

// TestSliderDrags проверяет перетаскивание ползунка: клик ставит значение по
// курсору, движение с зажатой левой кнопкой его меняет, отпускание — нет.
func TestSliderDrags(t *testing.T) {
	s := slider{x: 100, y: 200, w: 200, h: 20}
	ctx := scene.Ctx{W: 640, H: 480}

	// Клик по левой четверти дорожки.
	ctx.Input.MouseX, ctx.Input.MouseY = 150, 210
	ctx.Input.Buttons = 1 << 1
	ctx.Input.Clicks = []scene.Click{{Button: 1, X: 150, Y: 210}}
	s.update(&ctx)
	if s.value < 0.24 || s.value > 0.26 {
		t.Fatalf("после клика значение %v, ожидалось ~0.25", s.value)
	}

	// Ведём мышь вправо, кнопка всё ещё зажата.
	ctx.Input.Clicks = nil
	ctx.Input.MouseX = 250
	s.update(&ctx)
	if s.value < 0.74 || s.value > 0.76 {
		t.Fatalf("после перетаскивания значение %v, ожидалось ~0.75", s.value)
	}

	// Кнопку отпустили: дальнейшее движение мыши значение не меняет.
	ctx.Input.Buttons = 0
	ctx.Input.MouseX = 100
	s.update(&ctx)
	if s.value < 0.74 || s.value > 0.76 {
		t.Fatalf("после отпускания значение %v, ожидалось ~0.75", s.value)
	}
}

// TestSliderPressWithoutClickEvent — ползунок реагирует на зажатую кнопку,
// даже если событие клика в кадре не пришло (реальный путь ввода окна).
func TestSliderPressWithoutClickEvent(t *testing.T) {
	s := slider{x: 100, y: 200, w: 200, h: 20}
	ctx := scene.Ctx{W: 640, H: 480}
	ctx.Input.MouseX, ctx.Input.MouseY = 180, 210
	ctx.Input.Buttons = 1 << 1
	s.update(&ctx)
	if s.value < 0.39 || s.value > 0.41 {
		t.Fatalf("после нажатия без клика значение %v, ожидалось ~0.4", s.value)
	}
}

// TestSliderWheel — колесо над ползунком меняет громкость шагом 5 %.
func TestSliderWheel(t *testing.T) {
	s := slider{x: 100, y: 200, w: 200, h: 20, value: 0.5}
	ctx := scene.Ctx{W: 640, H: 480}
	ctx.Input.MouseX, ctx.Input.MouseY = 150, 210
	ctx.Input.Scroll = -1
	s.update(&ctx)
	if s.value < 0.44 || s.value > 0.46 {
		t.Fatalf("колесо вниз: значение %v, ожидалось ~0.45", s.value)
	}
	ctx.Input.Scroll = 1
	s.update(&ctx)
	if s.value < 0.49 || s.value > 0.51 {
		t.Fatalf("колесо вверх: значение %v, ожидалось ~0.5", s.value)
	}
}

// TestPauseMenuSliderClick — клик по ползунку в открытых настройках меню
// паузы доходит до ползунка и меняет значение.
func TestPauseMenuSliderClick(t *testing.T) {
	game := NewGameplay(nil, nil)
	game.pause.open = true
	game.pause.settings = true
	ctx := scene.Ctx{W: 1280, H: 720}
	game.pause.layout(&ctx)
	s := &game.pause.sliders[2]
	ctx.Input.MouseX, ctx.Input.MouseY = int(s.x+s.w*0.8), int(s.y+5)
	ctx.Input.Buttons = 1 << 1
	ctx.Input.Clicks = []scene.Click{{Button: 1, X: ctx.Input.MouseX, Y: ctx.Input.MouseY}}
	game.updatePauseMenu(&ctx)
	if s.value < 0.79 || s.value > 0.81 {
		t.Fatalf("значение музыки после клика %v, ожидалось ~0.8", s.value)
	}
}
