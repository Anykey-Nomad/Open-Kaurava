// Package lua — интерпретатор Lua 5.1 для скриптов движка, без CGO.
//
// Реализация — GopherLua (https://github.com/yuin/gopher-lua, лицензия MIT):
// компилятор и VM языка Lua 5.1 целиком на Go. Эталонная реализация Lua
// написана на C и требует CGO; здесь та же семантика 5.1 (таблицы,
// метатаблицы, замыкания, корутины, vararg, библиотеки string/table/math/
// io/os/debug) выполнена на чистом Go и собирается вместе с движком при
// CGO_ENABLED=0.
//
//	vm := lua.New()
//	defer vm.Close()
//	if err := vm.DoString(`answer = 6 * 7`); err != nil {
//	        return err
//	}
//	fmt.Println(lua.Number(vm.GetGlobal("answer"))) // 42
//
// Go-функции отдаются скриптам через Register. Один VM — одна песочница:
// из нескольких горутин его использовать нельзя.
package lua

import (
	glua "github.com/yuin/gopher-lua"
)

// Типы значений Lua — алиасы GopherLua: скриптовый слой движка работает
// с ними, не импортируя реализацию напрямую.
type (
	LState     = glua.LState
	LValue     = glua.LValue
	LBool      = glua.LBool
	LNumber    = glua.LNumber
	LString    = glua.LString
	LTable     = glua.LTable
	LFunction  = glua.LFunction
	LGFunction = glua.LGFunction
)

// LNil — «пустое» значение Lua.
var LNil = glua.LNil

// Number вытаскивает из значения Lua число; 0 — если это не число.
func Number(v LValue) float64 { return float64(glua.LVAsNumber(v)) }

// String вытаскивает из значения Lua строку; "" — если это не строка.
func String(v LValue) string { return glua.LVAsString(v) }

// Bool вытаскивает из значения Lua булево; false — если это не boolean.
func Bool(v LValue) bool { return glua.LVAsBool(v) }

// VM — состояние интерпретатора: глобальное окружение скриптов, стек и
// стандартные библиотеки Lua 5.1.
type VM struct {
	L *LState
}

// New поднимает VM со стандартными библиотеками (base, string, table, math,
// io, os, debug, package).
func New() *VM {
	return &VM{L: glua.NewState()}
}

// Close освобождает VM.
func (v *VM) Close() { v.L.Close() }

// DoString компилирует и выполняет исходник Lua.
func (v *VM) DoString(src string) error { return v.L.DoString(src) }

// DoFile компилирует и выполняет файл с исходником Lua.
func (v *VM) DoFile(path string) error { return v.L.DoFile(path) }

// SetGlobal записывает значение в глобальную переменную скрипта.
func (v *VM) SetGlobal(name string, val LValue) { v.L.SetGlobal(name, val) }

// GetGlobal читает глобальную переменную скрипта; LNil, если её нет.
func (v *VM) GetGlobal(name string) LValue { return v.L.GetGlobal(name) }

// Register объявляет Go-функцию глобальной функцией Lua под именем name.
func (v *VM) Register(name string, fn LGFunction) {
	v.L.SetGlobal(name, v.L.NewFunction(fn))
}
