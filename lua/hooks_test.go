package lua

import (
	"strings"
	"testing"
)

// TestHooksUpdate — подписка через on_update и вызов с dt.
func TestHooksUpdate(t *testing.T) {
	vm := New()
	defer vm.Close()
	hooks := NewHooks(vm)

	if err := vm.DoString(`
		calls = {}
		on_update(function(dt) table.insert(calls, dt) end)
		on_update(function(dt) table.insert(calls, dt * 2) end)
	`); err != nil {
		t.Fatalf("DoString: %v", err)
	}

	if err := hooks.Update(0.5); err != nil {
		t.Fatalf("Update: %v", err)
	}

	tbl, ok := vm.GetGlobal("calls").(*LTable)
	if !ok {
		t.Fatalf("calls не таблица: %v", vm.GetGlobal("calls"))
	}
	if tbl.Len() != 2 {
		t.Fatalf("вызовов %d, ожидалось 2", tbl.Len())
	}
	if got := Number(tbl.RawGetInt(1)); got != 0.5 {
		t.Errorf("calls[1] = %v, ожидалось 0.5", got)
	}
	if got := Number(tbl.RawGetInt(2)); got != 1 {
		t.Errorf("calls[2] = %v, ожидалось 1", got)
	}
}

// TestHooksInput — событие ввода доходит до скрипта таблицей полей.
func TestHooksInput(t *testing.T) {
	vm := New()
	defer vm.Close()
	hooks := NewHooks(vm)

	if err := vm.DoString(`
		on_input(function(ev)
			kind, key, rep = ev.kind, ev.key, ev.is_repeat
			x, y, button, scroll = ev.x, ev.y, ev.button, ev.scroll
		end)
	`); err != nil {
		t.Fatalf("DoString: %v", err)
	}

	if err := hooks.Input(Event{
		Kind: "key_down", Key: "Escape", IsRepeat: true,
		X: 10, Y: 20, Button: 1, Scroll: -3,
	}); err != nil {
		t.Fatalf("Input: %v", err)
	}

	for _, tc := range []struct {
		name string
		want LValue
	}{
		{"kind", LString("key_down")},
		{"key", LString("Escape")},
		{"rep", LBool(true)},
		{"x", LNumber(10)},
		{"y", LNumber(20)},
		{"button", LNumber(1)},
		{"scroll", LNumber(-3)},
	} {
		if got := vm.GetGlobal(tc.name); got != tc.want {
			t.Errorf("%s = %v, ожидалось %v", tc.name, got, tc.want)
		}
	}
}

// TestHooksMany — несколько обработчиков одного хука зовутся в порядке подписки.
func TestHooksMany(t *testing.T) {
	vm := New()
	defer vm.Close()
	hooks := NewHooks(vm)

	if err := vm.DoString(`
		order = ""
		on_update(function() order = order .. "a" end)
		on_update(function() order = order .. "b" end)
		on_update(function() order = order .. "c" end)
	`); err != nil {
		t.Fatalf("DoString: %v", err)
	}

	if err := hooks.Update(0.016); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := String(vm.GetGlobal("order")); got != "abc" {
		t.Fatalf("order = %q, ожидалось \"abc\"", got)
	}
}

// TestHooksError — ошибка одного хука не мешает остальным, но видна снаружи.
func TestHooksError(t *testing.T) {
	vm := New()
	defer vm.Close()
	hooks := NewHooks(vm)

	if err := vm.DoString(`
		ran = false
		on_update(function() error("упавший хук") end)
		on_update(function() ran = true end)
	`); err != nil {
		t.Fatalf("DoString: %v", err)
	}

	err := hooks.Update(0.016)
	if err == nil || !strings.Contains(err.Error(), "упавший хук") {
		t.Fatalf("ожидалась ошибка хука, получили: %v", err)
	}
	if !Bool(vm.GetGlobal("ran")) {
		t.Fatal("второй хук не выполнился после ошибки первого")
	}
}

// TestHooksNoSubscribers — без подписчиков вызовы — пустая операция.
func TestHooksNoSubscribers(t *testing.T) {
	vm := New()
	defer vm.Close()
	hooks := NewHooks(vm)

	if err := hooks.Update(0.016); err != nil {
		t.Fatalf("Update без подписчиков: %v", err)
	}
	if err := hooks.Input(Event{Kind: "key_down", Key: "A"}); err != nil {
		t.Fatalf("Input без подписчиков: %v", err)
	}
}

// TestHooksBadSubscriber — подписка не-функцией отклоняется с ошибкой Lua.
func TestHooksBadSubscriber(t *testing.T) {
	vm := New()
	defer vm.Close()
	NewHooks(vm)

	if err := vm.DoString(`on_update(42)`); err == nil ||
		!strings.Contains(err.Error(), "function") {
		t.Fatalf("ожидалась ошибка типа аргумента, получили: %v", err)
	}
}
