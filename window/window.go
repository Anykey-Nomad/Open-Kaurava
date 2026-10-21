// Package window — создание окна ОС и события ввода без CGO.
//
// Внутри пакета — три слоя:
//
//	window.go              общий API: New, PollEvents, CreateSurface, ...
//	window_windows.go      Win32 через purego (user32.dll)
//	window_x11.go          X11 через purego (libX11.so.6), Linux по умолчанию
//	window_unsupported.go  заглушка для остальных ОС
//
// Слой владеет только нативным окном. Vulkan-поверхность создаётся методом
// CreateSurface: окно отдаёт своё расширение (VK_KHR_win32_surface /
// VK_KHR_xlib_surface) и нативные хэндлы, а весь вызов Vulkan остаётся
// кросс-платформенным кодом пакета vulkan.
//
// ВАЖНО: окно создаётся и опрашивается в том же потоке. В main вызывайте
// runtime.LockOSThread() до window.New — иначе Go может перенести горутину
// в другой поток, и сообщения окна будут теряться.
package window

import (
	"fmt"

	"github.com/Anykey-Nomad/Open-Kaurava/vulkan"
)

// Config — параметры окна.
type Config struct {
	Title  string
	Width  int
	Height int
}

// Window — окно ОС.
//
// Поля потокобезопасны только при работе из одного потока (см. LockOSThread).
type Window struct {
	width, height int
	shouldClose   bool
	pending       []Event
	native        nativeState

	// Состояние мыши обновляется из событий (см. push) и доступно
	// без разбора очереди — для рисования курсора и опроса кнопок.
	mouseX, mouseY int
	buttons        uint32 // бит N — кнопка N зажата (1=левая, 2=средняя, 3=правая)
	cursorVisible  bool
	captured       bool // курсор захвачен окном (см. CaptureMouse)
}

// New создаёт и показывает окно.
func New(cfg Config) (*Window, error) {
	if cfg.Width <= 0 {
		cfg.Width = 1280
	}
	if cfg.Height <= 0 {
		cfg.Height = 720
	}
	if cfg.Title == "" {
		cfg.Title = "Open-Kaurava"
	}
	w := &Window{
		width: cfg.Width, height: cfg.Height,
		// до первого движения мыши курсор рисуется в центре окна
		mouseX: cfg.Width / 2, mouseY: cfg.Height / 2,
		cursorVisible: true,
	}
	if err := w.native.init(w, cfg); err != nil {
		return nil, err
	}
	return w, nil
}

// PollEvents разбирает очередь сообщений ОС (не блокируется) и возвращает
// события, накопленные с прошлого вызова. Обновляет ShouldClose и Size.
func (w *Window) PollEvents() []Event {
	w.native.poll(w)
	events := w.pending
	w.pending = nil
	return events
}

// ShouldClose становится true, когда пользователь закрыл окно.
func (w *Window) ShouldClose() bool { return w.shouldClose }

// Close помечает окно закрытым (следующий PollEvents завершит цикл).
func (w *Window) Close() { w.shouldClose = true }

// Size возвращает клиентский размер окна в пикселях.
func (w *Window) Size() (width, height int) { return w.width, w.height }

// SetSize меняет клиентский размер окна. Размер приходит обратно событием
// EventResize; из цикла отрисовки это повод пересоздать swapchain.
func (w *Window) SetSize(width, height int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("window: недопустимый размер %dx%d", width, height)
	}
	return w.native.setSize(width, height)
}

// MousePos возвращает текущую позицию курсора в клиентских координатах.
// Значение обновляется из событий движения и нажатия.
func (w *Window) MousePos() (x, y int) { return w.mouseX, w.mouseY }

// MouseDown сообщает, зажата ли кнопка мыши (1=левая, 2=средняя, 3=правая).
func (w *Window) MouseDown(button int) bool {
	if button < 1 || button > 31 {
		return false
	}
	return w.buttons&(1<<button) != 0
}

// CaptureMouse захватывает/освобождает курсор для окна.
//
// При захвате события мыши продолжают приходить, даже когда курсор вышел
// за границы окна (необходимо для драгов). На X11 действует до отпускания
// кнопки; на Win32 — до ReleaseMouse или потери захвата системой.
func (w *Window) CaptureMouse(capture bool) error {
	if capture == w.captured {
		return nil
	}
	if err := w.native.captureMouse(capture); err != nil {
		return err
	}
	w.captured = capture
	return nil
}

// SetCursorVisible показывает или прячет системный курсор над окном.
//
// Скрытие нужно, когда курсор рисует движок сам (см. пример в main.go).
func (w *Window) SetCursorVisible(visible bool) {
	if visible == w.cursorVisible {
		return
	}
	w.cursorVisible = visible
	w.native.setCursorVisible(visible)
}

// Backend возвращает имя бэкенда: "win32", "x11" или "unsupported".
func (w *Window) Backend() string { return w.native.backend() }

// SurfaceExtension возвращает имя расширения Vulkan для создания поверхности.
func (w *Window) SurfaceExtension() string { return w.native.surfaceExtension() }

// CreateSurface создаёт VkSurfaceKHR из нативного окна.
//
// extension (VK_KHR_win32_surface / VK_KHR_xlib_surface / VK_KHR_wayland_surface)
// должна быть включена в instance — см. Window.SurfaceExtension().
func (w *Window) CreateSurface(api *vulkan.API, instance vulkan.Instance) (vulkan.SurfaceKHR, error) {
	return w.native.createSurface(api, instance)
}

// Destroy закрывает окно и освобождает ресурсы ОС.
func (w *Window) Destroy() {
	if w == nil {
		return
	}
	w.native.destroy(w)
}

func (w *Window) push(ev Event) {
	switch ev.Kind {
	case EventMouseMove:
		w.mouseX, w.mouseY = ev.X, ev.Y
	case EventMouseDown:
		w.mouseX, w.mouseY = ev.X, ev.Y
		if ev.Button >= 1 && ev.Button <= 31 {
			w.buttons |= 1 << ev.Button
		}
	case EventMouseUp:
		w.mouseX, w.mouseY = ev.X, ev.Y
		if ev.Button >= 1 && ev.Button <= 31 {
			w.buttons &^= 1 << ev.Button
		}
	}
	w.pending = append(w.pending, ev)
}

func (w *Window) resize(width, height int) {
	if width <= 0 || height <= 0 || (width == w.width && height == w.height) {
		return
	}
	w.width, w.height = width, height
	w.push(Event{Kind: EventResize, Width: width, Height: height})
}

func (w *Window) close() { w.shouldClose = true }

// EventKind — тип события.
type EventKind uint8

const (
	EventNone EventKind = iota
	// EventClose — окно закрыто пользователем (после него ShouldClose == true).
	EventClose
	// EventResize — размер изменён (Width/Height).
	EventResize
	// EventExpose — окно нужно перерисовать.
	EventExpose
	EventKeyDown
	EventKeyUp
	EventMouseMove
	EventMouseDown
	EventMouseUp
	// EventScroll — прокрутка; Scroll > 0 вверх, < 0 вниз.
	EventScroll
)

// String — имя события (для логов).
func (k EventKind) String() string {
	switch k {
	case EventClose:
		return "close"
	case EventResize:
		return "resize"
	case EventExpose:
		return "expose"
	case EventKeyDown:
		return "keydown"
	case EventKeyUp:
		return "keyup"
	case EventMouseMove:
		return "mousemove"
	case EventMouseDown:
		return "mousedown"
	case EventMouseUp:
		return "mouseup"
	case EventScroll:
		return "scroll"
	default:
		return "none"
	}
}

// Event — событие окна. Поля заполняются в зависимости от Kind.
type Event struct {
	Kind EventKind

	// Resize / Expose.
	Width, Height int

	// Клавиатура.
	Key      Key
	Scancode int
	Repeat   bool

	// Мышь: клиентские координаты, кнопка 1=левая, 2=средняя, 3=правая.
	X, Y   int
	Button int
	Scroll float64
}

// Key — портируемый код клавиши. Источник: Win32 VK_* или X11 KeySym,
// оба приведены к одному набору.
type Key uint16

const (
	KeyUnknown Key = iota
	KeyA
	KeyB
	KeyC
	KeyD
	KeyE
	KeyF
	KeyG
	KeyH
	KeyI
	KeyJ
	KeyK
	KeyL
	KeyM
	KeyN
	KeyO
	KeyP
	KeyQ
	KeyR
	KeyS
	KeyT
	KeyU
	KeyV
	KeyW
	KeyX
	KeyY
	KeyZ
	Key0
	Key1
	Key2
	Key3
	Key4
	Key5
	Key6
	Key7
	Key8
	Key9
	KeyEscape
	KeySpace
	KeyEnter
	KeyTab
	KeyBackspace
	KeyLeft
	KeyRight
	KeyUp
	KeyDown
	KeyLeftShift
	KeyRightShift
	KeyLeftCtrl
	KeyRightCtrl
	KeyLeftAlt
	KeyRightAlt
	KeyF1
	KeyF2
	KeyF3
	KeyF4
	KeyF5
	KeyF6
	KeyF7
	KeyF8
	KeyF9
	KeyF10
	KeyF11
	KeyF12
)

var keyNames = map[Key]string{
	KeyEscape: "Escape", KeySpace: "Space", KeyEnter: "Enter", KeyTab: "Tab",
	KeyBackspace: "Backspace",
	KeyLeft:      "Left", KeyRight: "Right", KeyUp: "Up", KeyDown: "Down",
	KeyLeftShift: "LShift", KeyRightShift: "RShift",
	KeyLeftCtrl: "LCtrl", KeyRightCtrl: "RCtrl",
	KeyLeftAlt: "LAlt", KeyRightAlt: "RAlt",
}

// String — имя клавиши ("A", "5", "F1", "Escape", ...).
func (k Key) String() string {
	if k >= KeyA && k <= KeyZ {
		return string(rune('A' + (k - KeyA)))
	}
	if k >= Key0 && k <= Key9 {
		return string(rune('0' + (k - Key0)))
	}
	if k >= KeyF1 && k <= KeyF12 {
		return fmt.Sprintf("F%d", 1+k-KeyF1)
	}
	if name, ok := keyNames[k]; ok {
		return name
	}
	return "Unknown"
}
