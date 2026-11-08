package lua

// Хуки игровой логики: обновление кадра и события ввода.
//
// Диспетчер регистрирует в VM через Register две функции подписки:
//
//	on_update(function(dt) ... end)  — зовётся каждый кадр, dt — секунды
//	on_input(function(ev) ... end)   — зовётся на каждое событие ввода
//
// и раздаёт события всем подписчикам в порядке подписки:
//
//	hooks := lua.NewHooks(vm)
//	_ = vm.DoFile("game.lua")
//	hooks.Update(dt)                                // раз в кадр
//	_ = hooks.Input(lua.Event{Kind: "key_down", Key: "Escape"})
//
// Ошибка одного хука не мешает остальным: ошибки собираются и возвращаются
// одной (errors.Join), кадр при этом не падает.

import (
	"errors"
	"fmt"

	glua "github.com/yuin/gopher-lua"
)

// Event — событие ввода, передаваемое хуку on_input. Поля доходят до скрипта
// таблицей с теми же именами.
type Event struct {
	// Kind — имя события ввода (как у window.EventKind.String()):
	// "keydown", "keyup", "mousemove", "mousedown", "mouseup", "scroll".
	Kind string
	// Key — имя клавиши ("A", "5", "F1", "Escape", ...); только для клавишных
	// событий.
	Key string
	// IsRepeat — автоповтор нажатия.
	IsRepeat bool
	// X, Y — позиция курсора в пикселях (события мыши и scroll).
	X, Y int
	// Button — кнопка мыши: 1=левая, 2=средняя, 3=правая.
	Button int
	// Scroll — прокрутка колесом: > 0 — вверх, < 0 — вниз.
	Scroll float64
}

// Hooks — диспетчер хуков игровой логики поверх VM.
// Как и VM, он не потокобезопасен: хуки зовутся из одного потока.
type Hooks struct {
	vm     *VM
	update []*LFunction
	input  []*LFunction
}

// NewHooks регистрирует функции подписки on_update и on_input в VM (через
// Register) и возвращает диспетчер.
func NewHooks(vm *VM) *Hooks {
	h := &Hooks{vm: vm}
	vm.Register("on_update", h.subscribe(&h.update))
	vm.Register("on_input", h.subscribe(&h.input))
	return h
}

// subscribe — Lua-функция подписки: первый аргумент — обработчик, он
// запоминается в указанном списке.
func (h *Hooks) subscribe(dst *[]*LFunction) LGFunction {
	return func(L *LState) int {
		*dst = append(*dst, L.CheckFunction(1))
		return 0
	}
}

// Update зовёт хуки обновления кадра; dt — длительность кадра в секундах.
func (h *Hooks) Update(dt float64) error {
	return h.call("on_update", h.update, LNumber(dt))
}

// Input зовёт хуки событий ввода; ev доходит до скрипта таблицей с полями
// kind, key, is_repeat, x, y, button, scroll.
func (h *Hooks) Input(ev Event) error {
	if len(h.input) == 0 {
		return nil
	}
	t := h.vm.L.NewTable()
	t.RawSetString("kind", LString(ev.Kind))
	t.RawSetString("key", LString(ev.Key))
	t.RawSetString("is_repeat", LBool(ev.IsRepeat))
	t.RawSetString("x", LNumber(ev.X))
	t.RawSetString("y", LNumber(ev.Y))
	t.RawSetString("button", LNumber(ev.Button))
	t.RawSetString("scroll", LNumber(ev.Scroll))
	return h.call("on_input", h.input, t)
}

// call зовёт каждого обработчика с одним аргументом и собирает ошибки.
func (h *Hooks) call(name string, fns []*LFunction, arg LValue) error {
	var errs []error
	for i, fn := range fns {
		if err := h.vm.L.CallByParam(glua.P{Fn: fn, NRet: 0, Protect: true}, arg); err != nil {
			errs = append(errs, fmt.Errorf("%s #%d: %w", name, i+1, err))
		}
	}
	return errors.Join(errs...)
}
