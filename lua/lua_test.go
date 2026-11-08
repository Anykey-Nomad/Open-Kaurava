package lua

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLua51Semantics — ключевые черты языка Lua 5.1: таблицы, метатаблицы,
// vararg, замыкания и _VERSION.
func TestLua51Semantics(t *testing.T) {
	vm := New()
	defer vm.Close()

	const src = `
		assert(_VERSION == "Lua 5.1", "не 5.1: ".._VERSION)

		-- таблицы и метатаблицы
		local t = setmetatable({1, 2, 3}, {__index = function(_, k) return "нет:"..k end})
		assert(#t == 3, "длина массива")
		assert(t[1] + t[2] + t[3] == 6, "сумма массива")
		assert(t.no_such_key == "нет:no_such_key", "__index")

		-- замыкания и счётчик
		local function counter()
			local n = 0
			return function() n = n + 1 return n end
		end
		local next1, next2 = counter(), counter()
		next1(); next1()
		assert(next1() == 3 and next2() == 1, "замыкания независимы")

		-- vararg
		local function sum(...)
			local s = 0
			for i = 1, select("#", ...) do s = s + select(i, ...) end
			return s
		end
		assert(sum(1, 2, 3, 4) == 10, "vararg")

		-- строковые библиотеки 5.1: паттерны, gsub
		assert(("hello world"):gsub("o", "0") == "hell0 w0rld", "gsub")
		assert(string.match("key=42", "(%w+)=(%d+)") == "key", "match")

		result = "ok"
	`
	if err := vm.DoString(src); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	if got := String(vm.GetGlobal("result")); got != "ok" {
		t.Fatalf("result = %q, ожидалось \"ok\"", got)
	}
}

// TestGoFunction — вызов Go-функции из Lua и чтение результата обратно.
func TestGoFunction(t *testing.T) {
	vm := New()
	defer vm.Close()

	vm.Register("double", func(L *LState) int {
		L.Push(LNumber(Number(L.CheckNumber(1)) * 2))
		return 1
	})
	if err := vm.DoString(`answer = double(21)`); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	if got := Number(vm.GetGlobal("answer")); got != 42 {
		t.Fatalf("answer = %v, ожидалось 42", got)
	}
}

// TestDoFile — выполнение скрипта из файла.
func TestDoFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.lua")
	if err := os.WriteFile(path, []byte(`greeting = "привет, ".._VERSION`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	vm := New()
	defer vm.Close()
	if err := vm.DoFile(path); err != nil {
		t.Fatalf("DoFile: %v", err)
	}
	if got := String(vm.GetGlobal("greeting")); got != "привет, Lua 5.1" {
		t.Fatalf("greeting = %q", got)
	}
}

// TestErrors — ошибки компиляции и выполнения доходят до Go с позицией.
func TestErrors(t *testing.T) {
	vm := New()
	defer vm.Close()

	if err := vm.DoString(`if then`); err == nil || !strings.Contains(err.Error(), "line:1") {
		t.Fatalf("синтаксическая ошибка не поймана: %v", err)
	}
	if err := vm.DoString("local x = 1\nerror('деление на ноль')"); err == nil ||
		!strings.Contains(err.Error(), "деление на ноль") {
		t.Fatalf("ошибка времени выполнения не поймана: %v", err)
	}
}

// TestConvert — преобразование значений Lua в Go и обратно.
func TestConvert(t *testing.T) {
	vm := New()
	defer vm.Close()

	vm.SetGlobal("in_number", LNumber(3.5))
	vm.SetGlobal("in_string", LString("s"))
	vm.SetGlobal("in_bool", LBool(true))
	if err := vm.DoString(`
		out_number = in_number * 2
		out_missing = no_such_global == nil
	`); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	if got := Number(vm.GetGlobal("out_number")); got != 7 {
		t.Fatalf("out_number = %v, ожидалось 7", got)
	}
	if !Bool(vm.GetGlobal("out_missing")) {
		t.Fatal("отсутствующая глобальная переменная не nil")
	}
	if got := vm.GetGlobal("in_string"); String(got) != "s" {
		t.Fatalf("in_string = %v", got)
	}
	if got := vm.GetGlobal("no_such_global"); got != LNil {
		t.Fatalf("no_such_global = %v, ожидалось LNil", got)
	}
}
