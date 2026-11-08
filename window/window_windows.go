//go:build windows

package window

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"golang.org/x/sys/windows"

	"github.com/Anykey-Nomad/Open-Kaurava/internal/dylib"
	"github.com/Anykey-Nomad/Open-Kaurava/vulkan"
)

// Бэкенд Win32. Весь GUI вызывается через purego (user32.dll / kernel32.dll),
// из x/sys/windows нужен только LoadLibrary (см. internal/dylib).
//
// CGO не используется; сообщения разбираются неблокирующим PeekMessageW,
// поэтому цикл отрисовки не зависает, когда окно простаивает.

// Структуры Win32 (раскладка x64).
type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type rect struct {
	Left, Top, Right, Bottom int32
}

func (r rect) Width() int  { return int(r.Right - r.Left) }
func (r rect) Height() int { return int(r.Bottom - r.Top) }

type point struct{ X, Y int32 }

type msg struct {
	Hwnd    uintptr
	Message uint32
	_       uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
	Private uint32
}

// Константы Win32.
const (
	wsOverlappedWindow = 0x00CF0000
	cwUseDefault       = int32(-2147483648) // 0x80000000
	swShow             = 5
	pmRemove           = 0x0001
	idcArrow           = uintptr(32512)

	wmDestroy     = 0x0002
	wmSize        = 0x0005
	wmClose       = 0x0010
	wmEraseBkgnd  = 0x0014
	wmQuit        = 0x0012
	wmPaint       = 0x000F
	wmMouseMove   = 0x0200
	wmLButtonDown = 0x0201
	wmLButtonUp   = 0x0202
	wmRButtonDown = 0x0204
	wmRButtonUp   = 0x0205
	wmMButtonDown = 0x0207
	wmMButtonUp   = 0x0208
	wmMouseWheel  = 0x020A
	wmSetCursor   = 0x0020
	wmKeyDown     = 0x0100
	wmKeyUp       = 0x0101

	htClient = 1

	sizeMinimized = 1
)

// Функции user32/kernel32.
var (
	user32, kernel32 uintptr

	registerClassEx    func(c *wndClassEx) uint16
	createWindowEx     func(exStyle uint32, className *uint16, windowName *uint16, style uint32, x, y, w, h int32, parent, menu, hinst, param uintptr) uintptr
	defWindowProc      func(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr
	showWindow         func(hwnd uintptr, cmd int32) int32
	updateWindow       func(hwnd uintptr) int32
	peekMessage        func(m *msg, hwnd uintptr, min, max, filter uint32) int32
	translateMessage   func(m *msg) int32
	dispatchMessage    func(m *msg) uintptr
	postQuitMessage    func(code int32)
	destroyWindow      func(hwnd uintptr) int32
	getClientRect      func(hwnd uintptr, r *rect) int32
	setWindowPos       func(hwnd, insertAfter uintptr, x, y, cx, cy int32, flags uint32) int32
	adjustWindowRectEx func(r *rect, style uint32, hasMenu int32, exStyle uint32) int32
	loadCursor         func(hinst, name uintptr) uintptr
	setCursor          func(hCursor uintptr) uintptr
	setCapture         func(hwnd uintptr) uintptr
	releaseCapture     func() int32
	getModuleHandleW   func(name *uint16) uintptr
	setProcessDPIAware func() int32 // опционально (может отсутствовать)

	wndProcCallback uintptr
)

// nativeState — нативные хэндлы окна Win32.
type nativeState struct {
	hinstance uintptr
	hwnd      uintptr
}

var (
	windowsMu   sync.Mutex
	windowsByID = map[uintptr]*Window{}

	// Класс окна регистрируется один раз на процесс.
	classOnce sync.Once
	classAtom uint16
	classErr  error
	className *uint16 // живёт всю жизнь процесса: имя класса в WNDCLASEX
)

func init() {
	var err error
	if user32, err = dylib.Open("user32.dll"); err != nil {
		panic("window: user32.dll недоступна: " + err.Error())
	}
	if kernel32, err = dylib.Open("kernel32.dll"); err != nil {
		panic("window: kernel32.dll недоступна: " + err.Error())
	}

	mustReg := func(dst any, lib uintptr, name string) {
		addr, err := dylib.Symbol(lib, name)
		if err != nil {
			panic("window: " + name + " не найдена: " + err.Error())
		}
		purego.RegisterFunc(dst, addr)
	}
	tryReg := func(dst any, lib uintptr, name string) bool {
		addr, err := dylib.Symbol(lib, name)
		if err != nil {
			return false
		}
		purego.RegisterFunc(dst, addr)
		return true
	}

	mustReg(&registerClassEx, user32, "RegisterClassExW")
	mustReg(&createWindowEx, user32, "CreateWindowExW")
	mustReg(&defWindowProc, user32, "DefWindowProcW")
	mustReg(&showWindow, user32, "ShowWindow")
	mustReg(&updateWindow, user32, "UpdateWindow")
	mustReg(&peekMessage, user32, "PeekMessageW")
	mustReg(&translateMessage, user32, "TranslateMessage")
	mustReg(&dispatchMessage, user32, "DispatchMessageW")
	mustReg(&postQuitMessage, user32, "PostQuitMessage")
	mustReg(&destroyWindow, user32, "DestroyWindow")
	mustReg(&getClientRect, user32, "GetClientRect")
	mustReg(&setWindowPos, user32, "SetWindowPos")
	mustReg(&adjustWindowRectEx, user32, "AdjustWindowRectEx")
	mustReg(&loadCursor, user32, "LoadCursorW")
	mustReg(&setCursor, user32, "SetCursor")
	mustReg(&setCapture, user32, "SetCapture")
	mustReg(&releaseCapture, user32, "ReleaseCapture")
	mustReg(&getModuleHandleW, kernel32, "GetModuleHandleW")
	tryReg(&setProcessDPIAware, user32, "SetProcessDPIAware")

	// До создания окна: иначе Windows будет масштабировать клиентскую область.
	if setProcessDPIAware != nil {
		setProcessDPIAware()
	}

	wndProcCallback = purego.NewCallback(windowProcedure)
}

func (n *nativeState) init(w *Window, cfg Config) error {
	hinst := getModuleHandleW(nil)
	if hinst == 0 {
		return fmt.Errorf("window: GetModuleHandleW не вернул handle модуля")
	}

	if err := registerWindowClass(hinst); err != nil {
		return err
	}

	title, err := windows.UTF16PtrFromString(cfg.Title)
	if err != nil {
		return err
	}

	frame := rect{Left: 0, Top: 0, Right: int32(cfg.Width), Bottom: int32(cfg.Height)}
	adjustWindowRectEx(&frame, wsOverlappedWindow, 0, 0)

	hwnd := createWindowEx(
		0, className, title, wsOverlappedWindow,
		cwUseDefault, cwUseDefault,
		int32(frame.Width()), int32(frame.Height()),
		0, 0, hinst, 0,
	)
	if hwnd == 0 {
		return fmt.Errorf("window: CreateWindowExW не удалось создать окно (ошибка %d)", windows.GetLastError())
	}

	n.hinstance, n.hwnd = hinst, hwnd

	windowsMu.Lock()
	windowsByID[hwnd] = w
	windowsMu.Unlock()

	var client rect
	getClientRect(hwnd, &client)
	w.resize(client.Width(), client.Height())

	showWindow(hwnd, swShow)
	updateWindow(hwnd)
	return nil
}

// registerWindowClass регистрирует класс окна один раз на процесс.
func registerWindowClass(hinst uintptr) error {
	classOnce.Do(func() {
		className, classErr = windows.UTF16PtrFromString("Open-KauravaWindowClass")
		if classErr != nil {
			return
		}
		wc := wndClassEx{
			Size:      uint32(unsafe.Sizeof(wndClassEx{})),
			WndProc:   wndProcCallback,
			Instance:  hinst,
			Cursor:    loadCursor(0, idcArrow),
			ClassName: className,
		}
		classAtom = registerClassEx(&wc)
		if classAtom == 0 {
			classErr = fmt.Errorf("window: RegisterClassExW: %v", windows.GetLastError())
		}
	})
	return classErr
}

func (n *nativeState) poll(w *Window) {
	var m msg
	for {
		if peekMessage(&m, 0, 0, 0, pmRemove) == 0 {
			return
		}
		if m.Message == wmQuit {
			w.close()
			return
		}
		translateMessage(&m)
		dispatchMessage(&m)
	}
}

func (n *nativeState) destroy(w *Window) {
	if n.hwnd == 0 {
		return
	}
	windowsMu.Lock()
	delete(windowsByID, n.hwnd)
	windowsMu.Unlock()

	destroyWindow(n.hwnd)
	n.hwnd = 0
}

const (
	swpNoMove   = 0x0002
	swpNoZOrder = 0x0004
)

// setSize меняет клиентский размер, не двигая окно на экране.
func (n *nativeState) setSize(width, height int) error {
	if n.hwnd == 0 {
		return fmt.Errorf("window: окно уже уничтожено")
	}
	r := rect{Right: int32(width), Bottom: int32(height)}
	adjustWindowRectEx(&r, wsOverlappedWindow, 0, 0)
	if setWindowPos(n.hwnd, 0, 0, 0, int32(r.Width()), int32(r.Height()), swpNoMove|swpNoZOrder) == 0 {
		return fmt.Errorf("window: SetWindowPos не удалось изменить размер (ошибка %d)", windows.GetLastError())
	}
	return nil
}

func (n *nativeState) backend() string { return "win32" }

// captureMouse захватывает/освобождает курсор (SetCapture — события идут в
// окно, даже когда курсор вышел за его границы).
func (n *nativeState) captureMouse(capture bool) error {
	if n.hwnd == 0 {
		return fmt.Errorf("window: окно уже уничтожено")
	}
	if capture {
		setCapture(n.hwnd)
	} else {
		releaseCapture()
	}
	return nil
}

// setCursorVisible прячет/показывает системный курсор: обработка wmSetCursor
// держит его скрытым, пока курсор над клиентской областью.
func (n *nativeState) setCursorVisible(visible bool) {
	if visible {
		setCursor(loadCursor(0, idcArrow))
	} else {
		setCursor(0)
	}
}

func (n *nativeState) surfaceExtension() string { return vulkan.ExtWin32Surface }

func (n *nativeState) createSurface(api *vulkan.API, instance vulkan.Instance) (vulkan.SurfaceKHR, error) {
	return api.CreateWin32Surface(instance, n.hinstance, n.hwnd)
}

// windowProcedure — оконная процедура (вызывается из потока окна).
func windowProcedure(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	windowsMu.Lock()
	w := windowsByID[hwnd]
	windowsMu.Unlock()

	if w != nil {
		switch msg {
		case wmClose:
			// DefWindowProc вызовет DestroyWindow → получим wmDestroy.
		case wmDestroy:
			w.close()
			postQuitMessage(0)
		case wmEraseBkgnd:
			// Кадры рисует Vulkan: стирать фон не нужно, так меньше мерцания.
			return 1
		case wmSize:
			if int(wparam) != sizeMinimized {
				width := int(int16(uint16(lparam & 0xFFFF)))
				height := int(int16(uint16(lparam >> 16)))
				w.resize(width, height)
			}
		case wmPaint:
			// Валидация области рисунка без перерисовки — DefWindowProc.
		case wmKeyDown, wmKeyUp:
			kind := EventKeyDown
			if msg == wmKeyUp {
				kind = EventKeyUp
			}
			w.push(Event{
				Kind:     kind,
				Key:      keyFromVK(wparam),
				Scancode: int((lparam >> 16) & 0xFF),
				Repeat:   (lparam>>30)&1 != 0 && msg == wmKeyDown,
			})
		case wmMouseMove:
			w.push(Event{
				Kind: EventMouseMove,
				X:    int(int16(uint16(lparam & 0xFFFF))),
				Y:    int(int16(uint16(lparam >> 16))),
			})
		case wmSetCursor:
			// Прятать курсор, пока он над клиентской областью (см. SetCursorVisible).
			if !w.cursorVisible && uint16(lparam&0xFFFF) == htClient {
				setCursor(0)
				return 1
			}
		case wmLButtonDown, wmLButtonUp, wmRButtonDown, wmRButtonUp, wmMButtonDown, wmMButtonUp:
			kind := EventMouseDown
			if msg == wmLButtonUp || msg == wmRButtonUp || msg == wmMButtonUp {
				kind = EventMouseUp
			}
			w.push(Event{
				Kind:   kind,
				Button: mouseButton(msg),
				X:      int(int16(uint16(lparam & 0xFFFF))),
				Y:      int(int16(uint16(lparam >> 16))),
			})
		case wmMouseWheel:
			delta := int16(uint16(wparam >> 16))
			// Координаты колеса приходят в экранных — их здесь не подставляем.
			w.push(Event{Kind: EventScroll, Scroll: float64(delta) / 120})
		}
	}
	return defWindowProc(hwnd, msg, wparam, lparam)
}

func mouseButton(msg uint32) int {
	switch msg {
	case wmLButtonDown, wmLButtonUp:
		return 1
	case wmMButtonDown, wmMButtonUp:
		return 2
	default:
		return 3
	}
}

// keyFromVK приводит Win32-код клавиши к портируемому Key.
func keyFromVK(vk uintptr) Key {
	switch {
	case vk >= 'A' && vk <= 'Z':
		return KeyA + Key(vk-'A')
	case vk >= '0' && vk <= '9':
		return Key0 + Key(vk-'0')
	case vk >= 0x70 && vk <= 0x7B: // VK_F1..VK_F12
		return KeyF1 + Key(vk-0x70)
	}
	switch vk {
	case 0x1B:
		return KeyEscape
	case 0x20:
		return KeySpace
	case 0x0D:
		return KeyEnter
	case 0x09:
		return KeyTab
	case 0x08:
		return KeyBackspace
	case 0x2E:
		return KeyDelete
	case 0x25:
		return KeyLeft
	case 0x27:
		return KeyRight
	case 0x26:
		return KeyUp
	case 0x28:
		return KeyDown
	case 0x10, 0xA0, 0xA1:
		return KeyLeftShift
	case 0x11, 0xA2, 0xA3:
		return KeyLeftCtrl
	case 0x12, 0xA4, 0xA5:
		return KeyLeftAlt
	}
	return KeyUnknown
}
