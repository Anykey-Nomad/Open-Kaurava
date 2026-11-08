//go:build linux

package window

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/Anykey-Nomad/Open-Kaurava/internal/dylib"
	"github.com/Anykey-Nomad/Open-Kaurava/vulkan"
)

// Бэкенд X11 (libX11.so.6) через purego.
//
// Выбран основным бэкендом Linux: X11 доступен и в чистых сессиях, и под
// XWayland в любом Wayland-окружении (GNOME, KDE, WSLg). Нативный Wayland
// требует дескрипторов интерфейсов xdg-shell и вынесен в отдельную задачу.
//
// Xlib не потокобезопасен без XInitThreads, поэтому все вызовы идут из одного
// потока (см. runtime.LockOSThread в main).

// Функции Xlib. Сигнатуры повторяют заголовок Xlib.h для LP64:
// Window/Atom/unsigned long = uintptr, long event_mask = int64.
var (
	libx11 uintptr

	xOpenDisplay        func(name *byte) uintptr
	xDefaultScreen      func(dpy uintptr) int32
	xRootWindow         func(dpy uintptr, screen int32) uintptr
	xBlackPixel         func(dpy uintptr, screen int32) uintptr
	xCreateSimpleWindow func(dpy, parent uintptr, x, y, w, h int32, borderWidth uint32, border, background uintptr) uintptr
	xStoreName          func(dpy, win uintptr, name string) int32
	xSelectInput        func(dpy, win uintptr, mask int64) int32
	xInternAtom         func(dpy uintptr, name string, onlyIfExists int32) uintptr
	xSetWMProtocols     func(dpy, win uintptr, atoms *uintptr, count int32) int32
	xMapWindow          func(dpy, win uintptr) int32
	xResizeWindow       func(dpy, win uintptr, w, h uint32) int32
	xFlush              func(dpy uintptr) int32
	xPending            func(dpy uintptr) int32
	xNextEvent          func(dpy uintptr, ev unsafe.Pointer) int32
	xLookupString       func(ev unsafe.Pointer, buf *byte, bufsize int32, keysym *uintptr, status unsafe.Pointer) int32
	xCloseDisplay       func(dpy uintptr) int32
	xDestroyWindow      func(dpy, win uintptr) int32

	// Курсор и захват указателя.
	xGrabPointer          func(dpy, grabWin uintptr, ownerEvents int32, eventMask uint32, pointerMode, keyboardMode int32, confineTo, cursor, time uintptr) int32
	xUngrabPointer        func(dpy, time uintptr) int32
	xCreateBitmapFromData func(dpy, drawable uintptr, data string, w, h uint32) uintptr
	xCreatePixmapCursor   func(dpy, source, mask uintptr, fg, bg *xColor, x, y int32) uintptr
	xDefineCursor         func(dpy, win, cursor uintptr) int32
	xUndefineCursor       func(dpy, win uintptr) int32
	xFreeCursor           func(dpy, cursor uintptr) int32
	xFreePixmap           func(dpy, pixmap uintptr) int32
)

// nativeState — нативные хэндлы X11.
type nativeState struct {
	display     uintptr
	window      uintptr
	wmDelete    uintptr
	keySymBuf   [32]byte // рабочий буфер XLookupString
	blankCursor uintptr  // невидимый курсор (создаётся лениво)
}

// xColor — XColor из Xlib.h (LP64: pixel 8 + 3×2 + flags + pad = 16 байт).
type xColor struct {
	Pixel            uintptr
	Red, Green, Blue uint16
	Flags            byte
	_                byte
}

const (
	// Маски событий (X.h).
	keyPressMask        = 1 << 0
	keyReleaseMask      = 1 << 1
	buttonPressMask     = 1 << 2
	buttonReleaseMask   = 1 << 3
	pointerMotionMask   = 1 << 6
	exposureMask        = 1 << 15
	structureNotifyMask = 1 << 17

	// Типы событий (X.h).
	evExpose          = 12
	evDestroyNotify   = 17
	evConfigureNotify = 22
	evClientMessage   = 33
	evKeyPress        = 2
	evKeyRelease      = 3
	evButtonPress     = 4
	evButtonRelease   = 5
	evMotionNotify    = 6

	// Размер буфера XEvent (long pad[24] на LP64).
	xEventSize = 192
)

var (
	x11Mu    sync.Mutex
	x11ByWin = map[uintptr]*Window{}
)

func init() {
	var err error
	if libx11, err = dylib.Open("libX11.so.6"); err != nil {
		panic("window: libX11.so.6 недоступна (нужен X11 или XWayland): " + err.Error())
	}

	mustReg := func(dst any, name string) {
		addr, err := dylib.Symbol(libx11, name)
		if err != nil {
			panic("window: символ X11 " + name + " не найден: " + err.Error())
		}
		purego.RegisterFunc(dst, addr)
	}

	mustReg(&xOpenDisplay, "XOpenDisplay")
	mustReg(&xDefaultScreen, "XDefaultScreen")
	mustReg(&xRootWindow, "XRootWindow")
	mustReg(&xBlackPixel, "XBlackPixel")
	mustReg(&xCreateSimpleWindow, "XCreateSimpleWindow")
	mustReg(&xStoreName, "XStoreName")
	mustReg(&xSelectInput, "XSelectInput")
	mustReg(&xInternAtom, "XInternAtom")
	mustReg(&xSetWMProtocols, "XSetWMProtocols")
	mustReg(&xMapWindow, "XMapWindow")
	mustReg(&xResizeWindow, "XResizeWindow")
	mustReg(&xFlush, "XFlush")
	mustReg(&xPending, "XPending")
	mustReg(&xNextEvent, "XNextEvent")
	mustReg(&xLookupString, "XLookupString")
	mustReg(&xCloseDisplay, "XCloseDisplay")
	mustReg(&xDestroyWindow, "XDestroyWindow")
	mustReg(&xGrabPointer, "XGrabPointer")
	mustReg(&xUngrabPointer, "XUngrabPointer")
	mustReg(&xCreateBitmapFromData, "XCreateBitmapFromData")
	mustReg(&xCreatePixmapCursor, "XCreatePixmapCursor")
	mustReg(&xDefineCursor, "XDefineCursor")
	mustReg(&xUndefineCursor, "XUndefineCursor")
	mustReg(&xFreeCursor, "XFreeCursor")
	mustReg(&xFreePixmap, "XFreePixmap")
}

func (n *nativeState) init(w *Window, cfg Config) error {
	dpy := xOpenDisplay(nil)
	if dpy == 0 {
		return fmt.Errorf("window: не удалось подключиться к X-дисплею (проверьте переменную DISPLAY)")
	}

	screen := xDefaultScreen(dpy)
	root := xRootWindow(dpy, screen)
	black := xBlackPixel(dpy, screen)

	win := xCreateSimpleWindow(dpy, root, 0, 0, int32(cfg.Width), int32(cfg.Height), 0, black, black)
	if win == 0 {
		xCloseDisplay(dpy)
		return fmt.Errorf("window: XCreateSimpleWindow не удалось создать окно")
	}

	xStoreName(dpy, win, cfg.Title)
	xSelectInput(dpy, win, keyPressMask|keyReleaseMask|buttonPressMask|buttonReleaseMask|
		pointerMotionMask|exposureMask|structureNotifyMask)

	// Корректное закрытие крестиком: WM_DELETE_WINDOW.
	atom := xInternAtom(dpy, "WM_DELETE_WINDOW", 0)
	if atom != 0 {
		xSetWMProtocols(dpy, win, &atom, 1)
	}

	n.display, n.window, n.wmDelete = dpy, win, atom

	x11Mu.Lock()
	x11ByWin[win] = w
	x11Mu.Unlock()

	xMapWindow(dpy, win)
	xFlush(dpy)
	return nil
}

func (n *nativeState) poll(w *Window) {
	dpy := n.display
	if dpy == 0 {
		return
	}
	// XPending сначала читает сокет, если в буфере пусто: вызов не блокирует.
	for xPending(dpy) > 0 {
		var buf xEvent
		xNextEvent(dpy, unsafe.Pointer(&buf.data[0]))
		n.handle(w, &buf)
	}
	xFlush(dpy)
}

// setSize меняет клиентский размер окна (XResizeWindow → ConfigureNotify).
func (n *nativeState) setSize(width, height int) error {
	if n.display == 0 || n.window == 0 {
		return fmt.Errorf("window: окно уже уничтожено")
	}
	if xResizeWindow(n.display, n.window, uint32(width), uint32(height)) != 0 {
		return fmt.Errorf("window: XResizeWindow вернул ошибку")
	}
	xFlush(n.display)
	return nil
}

func (n *nativeState) destroy(w *Window) {
	if n.display == 0 {
		return
	}
	if n.window != 0 {
		x11Mu.Lock()
		delete(x11ByWin, n.window)
		x11Mu.Unlock()

		xDestroyWindow(n.display, n.window)
		n.window = 0
	}
	if n.blankCursor != 0 {
		xFreeCursor(n.display, n.blankCursor)
		n.blankCursor = 0
	}
	xCloseDisplay(n.display)
	n.display = 0
}

func (n *nativeState) backend() string { return "x11" }

const (
	grabModeAsync = 1
	grabSuccess   = 0
)

// captureMouse захватывает указатель (XGrabPointer): события продолжают
// приходить в наше окно, даже когда курсор за его пределами — нужно для драгов.
func (n *nativeState) captureMouse(capture bool) error {
	if n.display == 0 || n.window == 0 {
		return fmt.Errorf("window: окно уже уничтожено")
	}
	if capture {
		mask := uint32(pointerMotionMask | buttonPressMask | buttonReleaseMask)
		// owner_events=True: внутренние события приходят как обычно, всё
		// остальное (в том числе движение вне окна) — в наше окно.
		if r := xGrabPointer(n.display, n.window, 1, mask, grabModeAsync, grabModeAsync, 0, 0, 0); r != grabSuccess {
			return fmt.Errorf("window: XGrabPointer не удалось захватить курсор (код %d)", r)
		}
	} else {
		xUngrabPointer(n.display, 0)
	}
	xFlush(n.display)
	return nil
}

// setCursorVisible прячет/показывает системный курсор над окном: для скрытия
// ставится невидимый курсор (1×1 пустая маска), для показа — родительский.
func (n *nativeState) setCursorVisible(visible bool) {
	if n.display == 0 || n.window == 0 {
		return
	}
	if visible {
		xUndefineCursor(n.display, n.window)
	} else {
		if n.blankCursor == 0 {
			n.blankCursor = n.createBlankCursor()
		}
		if n.blankCursor != 0 {
			xDefineCursor(n.display, n.window, n.blankCursor)
		}
	}
	xFlush(n.display)
}

// createBlankCursor создаёт невидимый курсор: пустая 1×1 bitmap в роли
// и маски, и источника.
func (n *nativeState) createBlankCursor() uintptr {
	// Данные битмапа выровнены по 32 бита на строку: 1×1 → 4 байта нулей.
	bm := xCreateBitmapFromData(n.display, n.window, "\x00\x00\x00\x00", 1, 1)
	if bm == 0 {
		return 0
	}
	defer xFreePixmap(n.display, bm)
	var black xColor
	cur := xCreatePixmapCursor(n.display, bm, bm, &black, &black, 0, 0)
	return cur
}

func (n *nativeState) surfaceExtension() string { return vulkan.ExtXlibSurface }

func (n *nativeState) createSurface(api *vulkan.API, instance vulkan.Instance) (vulkan.SurfaceKHR, error) {
	return api.CreateXlibSurface(instance, n.display, n.window)
}

// handle разбирает одно событие X11.
func (n *nativeState) handle(w *Window, buf *xEvent) {
	switch buf.kind() {
	case evDestroyNotify:
		w.close()
	case evClientMessage:
		// Закрытие через WM: message_type == WM_DELETE_WINDOW.
		ev := buf.clientMessage()
		if ev.MessageType == n.wmDelete && n.wmDelete != 0 {
			w.close()
		}
	case evConfigureNotify:
		ev := buf.configure()
		w.resize(int(ev.Width), int(ev.Height))
	case evExpose:
		ev := buf.expose()
		w.push(Event{Kind: EventExpose, Width: int(ev.Width), Height: int(ev.Height)})
	case evKeyPress, evKeyRelease:
		ev := buf.input()
		kind := EventKeyDown
		if buf.kind() == evKeyRelease {
			kind = EventKeyUp
		}
		w.push(Event{Kind: kind, Key: n.decodeKey(unsafe.Pointer(ev), ev.Detail), Scancode: int(ev.Detail)})
	case evButtonPress, evButtonRelease:
		ev := buf.input()
		// Колесо мыши приходит как «кнопки» 4 (вверх) и 5 (вниз).
		if ev.Detail == 4 || ev.Detail == 5 {
			delta := 1.0
			if ev.Detail == 5 {
				delta = -1
			}
			w.push(Event{Kind: EventScroll, Scroll: delta, X: int(ev.X), Y: int(ev.Y)})
			return
		}
		kind := EventMouseDown
		if buf.kind() == evButtonRelease {
			kind = EventMouseUp
		}
		w.push(Event{Kind: kind, Button: int(ev.Detail), X: int(ev.X), Y: int(ev.Y)})
	case evMotionNotify:
		ev := buf.input()
		w.push(Event{Kind: EventMouseMove, X: int(ev.X), Y: int(ev.Y)})
	}
}

// decodeKey по реальному событию получает keysym (XLookupString) и переводит
// его в портируемый Key. Модификаторы не учитываются: Escape, буквы, цифры,
// стрелки и F-клавиши распознаются независимо от Shift/Ctrl.
func (n *nativeState) decodeKey(event unsafe.Pointer, _ uint32) Key {
	var keysym uintptr
	xLookupString(event, &n.keySymBuf[0], int32(len(n.keySymBuf)), &keysym, nil)
	return keyFromKeysym(uint64(keysym))
}

// keyFromKeysym приводит X11 KeySym к портируемому Key.
func keyFromKeysym(sym uint64) Key {
	switch {
	case sym >= 'a' && sym <= 'z':
		return KeyA + Key(sym-'a')
	case sym >= 'A' && sym <= 'Z':
		return KeyA + Key(sym-'A')
	case sym >= '0' && sym <= '9':
		return Key0 + Key(sym-'0')
	case sym >= 0xFFBE && sym <= 0xFFC9: // XK_F1..XK_F12
		return KeyF1 + Key(sym-0xFFBE)
	}
	switch sym {
	case 0xFF1B: // XK_Escape
		return KeyEscape
	case 0x0020: // XK_space
		return KeySpace
	case 0xFF0D: // XK_Return
		return KeyEnter
	case 0xFF09: // XK_Tab
		return KeyTab
	case 0xFF08: // XK_BackSpace
		return KeyBackspace
	case 0xFFFF: // XK_Delete
		return KeyDelete
	case 0xFF51: // XK_Left
		return KeyLeft
	case 0xFF53: // XK_Right
		return KeyRight
	case 0xFF52: // XK_Up
		return KeyUp
	case 0xFF54: // XK_Down
		return KeyDown
	case 0xFFE1, 0xFFE2: // XK_Shift_L/R
		return KeyLeftShift
	case 0xFFE3, 0xFFE4: // XK_Control_L/R
		return KeyLeftCtrl
	case 0xFFE9, 0xFFEA: // XK_Alt_L/R
		return KeyLeftAlt
	}
	return KeyUnknown
}

// xEvent — буфер под XEvent (192 байта). Первое поле uint64 гарантирует
// выравнивание 8 для накладных структур событий.
type xEvent struct {
	_    uint64
	data [xEventSize]byte
}

func (e *xEvent) kind() int32 {
	return *(*int32)(unsafe.Pointer(&e.data[0]))
}

// Общая преамбула XAnyEvent/XKeyEvent/XButtonEvent/XMotionEvent (смещения из Xlib.h).
type xInputEvent struct {
	Type       int32
	_          int32
	Serial     uint64
	SendEvent  int32
	_          int32
	Display    uintptr
	Window     uintptr
	Root       uintptr
	SubWindow  uintptr
	Time       uint64
	X          int32
	Y          int32
	XRoot      int32
	YRoot      int32
	State      uint32
	Detail     uint32 // keycode (клавиатура) либо button (мышь)
	SameScreen int32
	_          int32
}

func (e *xEvent) input() *xInputEvent { return (*xInputEvent)(unsafe.Pointer(&e.data[0])) }

type xConfigureEvent struct {
	Type             int32
	_                int32
	Serial           uint64
	SendEvent        int32
	_                int32
	Display          uintptr
	Event            uintptr
	Window           uintptr
	X, Y             int32
	Width, Height    int32
	BorderWidth      int32
	Above            uintptr
	OverrideRedirect int32
}

func (e *xEvent) configure() *xConfigureEvent {
	return (*xConfigureEvent)(unsafe.Pointer(&e.data[0]))
}

type xExposeEvent struct {
	Type          int32
	_             int32
	Serial        uint64
	SendEvent     int32
	_             int32
	Display       uintptr
	Window        uintptr
	X, Y          int32
	Width, Height int32
	Count         int32
}

func (e *xEvent) expose() *xExposeEvent { return (*xExposeEvent)(unsafe.Pointer(&e.data[0])) }

type xClientMessageEvent struct {
	Type        int32
	_           int32
	Serial      uint64
	SendEvent   int32
	_           int32
	Display     uintptr
	Window      uintptr
	MessageType uintptr
	Format      int32
	_           int32
	Data        [5]uint64 // union { char[20]; short[10]; long[5] }
}

func (e *xEvent) clientMessage() *xClientMessageEvent {
	return (*xClientMessageEvent)(unsafe.Pointer(&e.data[0]))
}
