// Package dylib — единая загрузка динамических библиотек (.dll / .so / .dylib) без CGO.
//
// Это единственное место во всём проекте, где способ загрузки внешнего кода
// отличается между ОС: purego.Dlopen существует только на Unix (это обёртка над
// dlopen), поэтому на Windows используется LoadLibrary из golang.org/x/sys/windows.
//
// Важно: регистрация функций после получения handle — purego.RegisterLibFunc —
// работает одинаково везде: на Windows внутри она вызывает GetProcAddress,
// на Unix — dlsym. Поэтому весь остальной код биндингов платформо-независим:
//
//	handle, err := dylib.Open("vulkan-1.dll") // на Linux — "libvulkan.so.1"
//	purego.RegisterLibFunc(&vkCreateInstance, handle, "vkCreateInstance")
//
// Реализации: dylib_unix.go, dylib_windows.go, dylib_unsupported.go.
package dylib
