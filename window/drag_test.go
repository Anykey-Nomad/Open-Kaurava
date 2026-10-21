package window

import "testing"

// Юнит-тесты трекера кликов/драгов: чистая логика, окно не нужно.

func feed(d *Drag, kind EventKind, x, y, button int) {
	d.Feed(Event{Kind: kind, X: x, Y: y, Button: button})
}

func TestDragClick(t *testing.T) {
	var d Drag
	feed(&d, EventMouseDown, 100, 100, 1)
	if !d.Down() || d.Dragging() {
		t.Fatalf("после нажатия: Down=%v Dragging=%v", d.Down(), d.Dragging())
	}
	// смещение меньше порога — это всё ещё клик
	feed(&d, EventMouseMove, 102, 101, 0)
	feed(&d, EventMouseUp, 102, 101, 1)

	clicks := d.TakeClicks()
	if len(clicks) != 1 {
		t.Fatalf("кликов: got %d, want 1", len(clicks))
	}
	if clicks[0].Button != 1 || clicks[0].X != 102 || clicks[0].Y != 101 {
		t.Errorf("клик: got %+v, want {1 102 101}", clicks[0])
	}
	if len(d.TakeDrags()) != 0 {
		t.Error("кликом не должно регистрироваться перетаскивание")
	}
	// Take очищает
	if len(d.TakeClicks()) != 0 {
		t.Error("TakeClicks не очистил список")
	}
}

func TestDragDrag(t *testing.T) {
	var d Drag
	feed(&d, EventMouseDown, 100, 100, 1)
	feed(&d, EventMouseMove, 130, 140, 0)
	if !d.Dragging() {
		t.Fatal("после смещения 30px порог не пройден")
	}
	x, y, w, h, ok := d.Rect()
	if !ok || x != 100 || y != 100 || w != 30 || h != 40 {
		t.Fatalf("рамка: got (%d,%d %dx%d ok=%v), want (100,100 30x40)", x, y, w, h, ok)
	}
	// движение в обратную сторону — рамка нормализуется
	feed(&d, EventMouseMove, 70, 90, 0)
	x, y, w, h, _ = d.Rect()
	if x != 70 || y != 90 || w != 30 || h != 10 {
		t.Errorf("рамка назад: got (%d,%d %dx%d), want (70,90 30x10)", x, y, w, h)
	}
	feed(&d, EventMouseUp, 70, 90, 1)

	drags := d.TakeDrags()
	if len(drags) != 1 {
		t.Fatalf("драгов: got %d, want 1", len(drags))
	}
	if rx, ry, rw, rh := drags[0].Rect(); rx != 70 || ry != 90 || rw != 30 || rh != 10 {
		t.Errorf("итоговый rect: got (%d,%d %dx%d), want (70,90 30x10)", rx, ry, rw, rh)
	}
	if len(d.TakeClicks()) != 0 {
		t.Error("драгом не должно регистрироваться нажатие")
	}
	if d.Down() || d.Dragging() {
		t.Error("после отпускания жест должен завершиться")
	}
}

func TestDragButtonMismatch(t *testing.T) {
	// отпустили другую кнопку — жест не завершается
	var d Drag
	feed(&d, EventMouseDown, 10, 10, 1)
	feed(&d, EventMouseUp, 10, 10, 3)
	if !d.Down() {
		t.Fatal("отпускание чужой кнопки завершило жест")
	}
	feed(&d, EventMouseUp, 10, 10, 1)
	if len(d.TakeClicks()) != 1 {
		t.Error("клик своей кнопкой должен завершить жест")
	}
}

func TestDragSecondButtonDownIgnored(t *testing.T) {
	var d Drag
	feed(&d, EventMouseDown, 10, 10, 1)
	feed(&d, EventMouseDown, 50, 50, 3) // игнорируется
	if d.Button() != 1 {
		t.Fatalf("кнопка жеста: got %d, want 1", d.Button())
	}
	// первая точка не сместилась
	feed(&d, EventMouseMove, 12, 10, 0)
	feed(&d, EventMouseUp, 12, 10, 1)
	if clicks := d.TakeClicks(); len(clicks) != 1 || clicks[0].X != 12 {
		t.Errorf("клики: got %+v, want один в (12,10)", clicks)
	}
}

func TestDragMouseMoveWithoutButton(t *testing.T) {
	// движение без нажатия обновляет позицию, но не создаёт жестов
	var d Drag
	feed(&d, EventMouseMove, 300, 200, 0)
	if x, y := d.Pos(); x != 300 || y != 200 {
		t.Errorf("позиция: got (%d,%d), want (300,200)", x, y)
	}
	if d.Down() || d.Dragging() {
		t.Error("движение без кнопки не должно начинать жест")
	}
	feed(&d, EventMouseUp, 300, 200, 1)
	if len(d.TakeClicks()) != 0 {
		t.Error("MouseUp без нажатия — не клик")
	}
}

func TestDragCustomThreshold(t *testing.T) {
	d := Drag{Threshold: 20}
	feed(&d, EventMouseDown, 0, 0, 1)
	feed(&d, EventMouseMove, 19, 0, 0)
	if d.Dragging() {
		t.Error("19px при пороге 20 — ещё не драг")
	}
	feed(&d, EventMouseMove, 20, 0, 0)
	if !d.Dragging() {
		t.Error("20px при пороге 20 — драг")
	}
}
