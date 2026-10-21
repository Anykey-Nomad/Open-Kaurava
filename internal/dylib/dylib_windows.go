//go:build windows

package dylib

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// Open — LoadLibraryW. Ищет библиотеку по PATH/каталогам системы,
// как и dlopen ищет по ld.so. Понимает и абсолютные пути.
func Open(path string) (uintptr, error) {
	h, err := windows.LoadLibrary(path)
	if err != nil {
		return 0, fmt.Errorf("dylib: не удалось загрузить %q: %w", path, err)
	}
	return uintptr(h), nil
}

// Symbol — GetProcAddress. Тот же контракт, что и у purego.Dlsym на Unix.
func Symbol(handle uintptr, name string) (uintptr, error) {
	addr, err := windows.GetProcAddress(windows.Handle(handle), name)
	if err != nil {
		return 0, fmt.Errorf("dylib: символ %q не найден: %w", name, err)
	}
	return addr, nil
}

// Close — FreeLibrary. Не обязателен: библиотеки держатся всю жизнь процесса.
func Close(handle uintptr) error {
	return windows.FreeLibrary(windows.Handle(handle))
}
