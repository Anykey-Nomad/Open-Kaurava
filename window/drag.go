package window

// Трекер кликов и перетаскиваний.
//
// Отличает «клик» (нажатие и отпускание без заметного смещения) от
// «перетаскивания» (смещение за порогом до отпускания) и накапливает
// результаты за кадр. Чистая логика поверх потока событий: работает с
// любым бэкендом и покрывается юнит-тестами без реального окна.

// Click — завершённое нажатие кнопки (отпустили, не уведя за порог).
type Click struct {
	Button int
	X, Y   int
}

// DragRect — завершённое перетаскивание: от точки нажатия до точки отпускания.
type DragRect struct {
	Button int
	X0, Y0 int // точка нажатия
	X1, Y1 int // точка отпускания
}

// Rect нормализует прямоугольник: W/H неотрицательные.
func (d DragRect) Rect() (x, y, w, h int) {
	x, y = min(d.X0, d.X1), min(d.Y0, d.Y1)
	return x, y, abs(d.X1 - d.X0), abs(d.Y1 - d.Y0)
}

// Drag — машина состояний мыши поверх событий PollEvents.
//
//	for _, ev := range win.PollEvents() {
//	    drag.Feed(ev)
//	}
//	// в кадре: drag.Pos() для курсора, drag.TakeClicks()/TakeDrags() — результаты,
//	// drag.Rect() — рамка текущего выделения, пока она тянется.
//
// Трекер однопнопочный: первая зажатая кнопка держит жест до отпускания,
// остальные кнопки в этот момент игнорируются.
type Drag struct {
	// Threshold — смещение в пикселях, после которого нажатие становится
	// перетаскиванием. Значение ≤0 означает 5 px по умолчанию.
	Threshold int

	// --- состояние текущего жеста ---
	down     bool
	button   int
	startX   int
	startY   int
	curX     int
	curY     int
	dragging bool // порог пройден — рамка активна

	// --- результаты, накопленные с прошлого Take* ---
	clicks []Click
	drags  []DragRect
}

func (d *Drag) threshold() int {
	if d.Threshold <= 0 {
		return 5
	}
	return d.Threshold
}

// Feed прогоняет событие окна через машину состояний.
func (d *Drag) Feed(ev Event) {
	switch ev.Kind {
	case EventMouseDown:
		if d.down {
			return // жест уже идёт — вторую кнопку не подхватываем
		}
		d.down, d.button = true, ev.Button
		d.startX, d.startY = ev.X, ev.Y
		d.curX, d.curY = ev.X, ev.Y
		d.dragging = false
	case EventMouseMove:
		if !d.down {
			d.curX, d.curY = ev.X, ev.Y
			return
		}
		d.curX, d.curY = ev.X, ev.Y
		if !d.dragging {
			t := d.threshold()
			if abs(d.curX-d.startX) >= t || abs(d.curY-d.startY) >= t {
				d.dragging = true
			}
		}
	case EventMouseUp:
		if !d.down || ev.Button != d.button {
			return
		}
		d.curX, d.curY = ev.X, ev.Y
		if d.dragging {
			d.drags = append(d.drags, DragRect{
				Button: d.button, X0: d.startX, Y0: d.startY, X1: d.curX, Y1: d.curY,
			})
		} else {
			d.clicks = append(d.clicks, Click{Button: d.button, X: d.curX, Y: d.curY})
		}
		d.down, d.dragging = false, false
	}
}

// Pos возвращает текущую позицию курсора (последнее известное событие).
func (d *Drag) Pos() (x, y int) { return d.curX, d.curY }

// Down сообщает, зажата ли сейчас кнопка жеста.
func (d *Drag) Down() bool { return d.down }

// Dragging сообщает, что идёт перетаскивание (порог пройден, кнопка зажата).
func (d *Drag) Dragging() bool { return d.dragging }

// Button возвращает кнопку текущего жеста (0 — жеста нет).
func (d *Drag) Button() int {
	if !d.down {
		return 0
	}
	return d.button
}

// Rect возвращает рамку текущего перетаскивания; ok=false, если её нет.
func (d *Drag) Rect() (x, y, w, h int, ok bool) {
	if !d.dragging {
		return 0, 0, 0, 0, false
	}
	x, y, w, h = d.rect()
	return x, y, w, h, true
}

func (d *Drag) rect() (x, y, w, h int) {
	x, y = min(d.startX, d.curX), min(d.startY, d.curY)
	return x, y, abs(d.curX - d.startX), abs(d.curY - d.startY)
}

// TakeClicks возвращает клики, накопленные с прошлого вызова, и очищает список.
func (d *Drag) TakeClicks() []Click {
	out := d.clicks
	d.clicks = nil
	return out
}

// TakeDrags возвращает завершённые перетаскивания и очищает список.
func (d *Drag) TakeDrags() []DragRect {
	out := d.drags
	d.drags = nil
	return out
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
