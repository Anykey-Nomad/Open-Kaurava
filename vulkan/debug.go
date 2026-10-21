package vulkan

import (
	"fmt"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Отладочный вывод валидационных слоёв (VK_EXT_debug_utils).
//
// Колбэк собирается purego.NewCallback: C-сторона вызывает его по обычному
// соглашению вызовов, а мы читаем pMessage из VkDebugUtilsMessengerCallbackDataEXT.

// DebugCallback получает сообщение от слоёв валидации.
// severity — флаги DebugUtilsMessageSeverity*BitEXT.
type DebugCallback func(severity uint32, message string)

// CreateDebugMessenger создаёт мессенджер отладки.
//
// Пример:
//
//	m, err := api.CreateDebugMessenger(inst,
//	    vulkan.DebugUtilsMessageSeverityWarningBitEXT|vulkan.DebugUtilsMessageSeverityErrorBitEXT,
//	    vulkan.DebugUtilsMessageTypeGeneralBitEXT|vulkan.DebugUtilsMessageTypeValidationBitEXT,
//	    func(sev uint32, msg string) { log.Println(msg) })
func (a *API) CreateDebugMessenger(inst Instance, severity, types uint32, fn DebugCallback) (DebugUtilsMessengerEXT, error) {
	if a.vkCreateDebugUtilsMessengerEXT == nil {
		return 0, absent("vkCreateDebugUtilsMessengerEXT", ExtDebugUtils)
	}

	// Память под колбэк не освобождается никогда (так устроен purego.NewCallback),
	// поэтому отдельное удержание не требуется.
	callback := purego.NewCallback(func(severity, types, data, userData uintptr) uintptr {
		if fn != nil && data != 0 {
			// Идиома purego: uintptr из C-мира → unsafe.Pointer через адрес
			// локальной переменной (та же приёма, что в func.go у purego).
			ptr := *(*unsafe.Pointer)(unsafe.Pointer(&data))
			d := (*debugUtilsCallbackDataEXT)(ptr)
			fn(uint32(severity), goCString(d.Message))
		}
		return 0 // VK_FALSE: не подавлять сообщение
	})

	info := DebugUtilsMessengerCreateInfoEXT{
		Type:            StructureTypeDebugUtilsMessengerCreateInfoEXT,
		MessageSeverity: severity,
		MessageType:     types,
		UserCallback:    callback,
	}
	var m DebugUtilsMessengerEXT
	if err := check("vkCreateDebugUtilsMessengerEXT",
		a.vkCreateDebugUtilsMessengerEXT(inst, &info, nil, &m)); err != nil {
		return 0, err
	}
	return m, nil
}

// DestroyDebugMessenger уничтожает мессенджер.
func (a *API) DestroyDebugMessenger(inst Instance, m DebugUtilsMessengerEXT) {
	if m != 0 && a.vkDestroyDebugUtilsMessengerEXT != nil {
		a.vkDestroyDebugUtilsMessengerEXT(inst, m, nil)
	}
}

// goCString читает нуль-терминированную строку из C-памяти в Go-строку.
// Ограничение сверху защищает от зависания, если NUL не найден.
func goCString(p *byte) string {
	if p == nil {
		return ""
	}
	const maxLen = 1 << 16
	n := 0
	for n < maxLen && *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(p, n))
}

// String печатает ошибку создания мессенджера с подсказкой (для примера).
func DebugMessengerHint(err error) string {
	return fmt.Sprintf("отладочный мессенджер недоступен: %v (нужны слой %s и расширение %s)",
		err, LayerKhronosValidation, ExtDebugUtils)
}
