//go:build !(darwin || freebsd || linux || netbsd || windows)

package dylib

import "errors"

var errUnsupported = errors.New("dylib: эта платформа не поддерживается (нужны windows, linux, darwin, freebsd, netbsd)")

// Open — заглушка для неподдерживаемых платформ.
func Open(path string) (uintptr, error) { return 0, errUnsupported }

// Symbol — заглушка для неподдерживаемых платформ.
func Symbol(handle uintptr, name string) (uintptr, error) { return 0, errUnsupported }

// Close — заглушка для неподдерживаемых платформ.
func Close(handle uintptr) error { return errUnsupported }
