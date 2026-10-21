//go:build darwin || freebsd || linux || netbsd

package dylib

import (
	"fmt"

	"github.com/ebitengine/purego"
)

// Open — purego.Dlopen. RTLD_NOW: символы резолвятся сразу при загрузке.
func Open(path string) (uintptr, error) {
	return purego.Dlopen(path, purego.RTLD_NOW)
}

// Symbol — purego.Dlsym.
func Symbol(handle uintptr, name string) (uintptr, error) {
	addr, err := purego.Dlsym(handle, name)
	if err != nil {
		return 0, err
	}
	if addr == 0 {
		return 0, fmt.Errorf("dylib: символ %q не найден", name)
	}
	return addr, nil
}

// Close — purego.Dlclose. Не обязателен: библиотеки держатся всю жизнь процесса.
func Close(handle uintptr) error {
	return purego.Dlclose(handle)
}
