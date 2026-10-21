package vulkan

import (
	"bytes"
	"errors"
	"fmt"
)

var resultNames = map[Result]string{
	ResultSuccess:                     "VK_SUCCESS",
	ResultNotReady:                    "VK_NOT_READY",
	ResultTimeout:                     "VK_TIMEOUT",
	ResultIncomplete:                  "VK_INCOMPLETE",
	ResultErrorOutOfHostMemory:        "VK_ERROR_OUT_OF_HOST_MEMORY",
	ResultErrorOutOfDeviceMemory:      "VK_ERROR_OUT_OF_DEVICE_MEMORY",
	ResultErrorInitializationFailed:   "VK_ERROR_INITIALIZATION_FAILED",
	ResultErrorDeviceLost:             "VK_ERROR_DEVICE_LOST",
	ResultErrorMemoryMapFailed:        "VK_ERROR_MEMORY_MAP_FAILED",
	ResultErrorLayerNotPresent:        "VK_ERROR_LAYER_NOT_PRESENT",
	ResultErrorExtensionNotPresent:    "VK_ERROR_EXTENSION_NOT_PRESENT",
	ResultErrorFeatureNotPresent:      "VK_ERROR_FEATURE_NOT_PRESENT",
	ResultErrorIncompatibleDriver:     "VK_ERROR_INCOMPATIBLE_DRIVER",
	ResultErrorTooManyObjects:         "VK_ERROR_TOO_MANY_OBJECTS",
	ResultErrorFormatNotSupported:     "VK_ERROR_FORMAT_NOT_SUPPORTED",
	ResultErrorUnknown:                "VK_ERROR_UNKNOWN",
	ResultErrorOutOfPoolMemory:        "VK_ERROR_OUT_OF_POOL_MEMORY",
	ResultErrorFragmentation:          "VK_ERROR_FRAGMENTATION",
	ResultErrorSurfaceLostKHR:         "VK_ERROR_SURFACE_LOST_KHR",
	ResultErrorNativeWindowInUseKHR:   "VK_ERROR_NATIVE_WINDOW_IN_USE_KHR",
	ResultErrorOutOfDateKHR:           "VK_ERROR_OUT_OF_DATE_KHR",
	ResultErrorIncompatibleDisplayKHR: "VK_ERROR_INCOMPATIBLE_DISPLAY_KHR",
	ResultErrorValidationFailedEXT:    "VK_ERROR_VALIDATION_FAILED_EXT",
	ResultSuboptimalKHR:               "VK_SUBOPTIMAL_KHR",
}

// String — имя константы VkResult.
func (r Result) String() string {
	if name, ok := resultNames[r]; ok {
		return name
	}
	return fmt.Sprintf("VkResult(%d)", int32(r))
}

// OK возвращает true для любых успехов (VK_SUCCESS, VK_SUBOPTIMAL_KHR, ...).
func (r Result) OK() bool { return r >= 0 }

// Error делает Result значением ошибки, чтобы работало:
//
//	errors.Is(err, vulkan.ResultErrorOutOfDateKHR)
func (r Result) Error() string { return "vulkan: " + r.String() }

// Error — ошибка Vulkan: какая операция и какой VkResult вернулся.
type Error struct {
	Op   string
	Code Result
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Op, e.Code)
}

// Is implements errors.Is для сравнения с кодом: errors.Is(err, vulkan.ResultErrorOutOfDateKHR).
func (e *Error) Is(target error) bool {
	var r Result
	if errors.As(target, &r) {
		return e.Code == r
	}
	return false
}

// check превращает VkResult в error, если это не успех.
func check(op string, r Result) error {
	if r.OK() {
		return nil
	}
	return &Error{Op: op, Code: r}
}

// IsOutOfDate сообщает, что swapchain устарел (окно изменилось) и его нужно пересоздать.
func IsOutOfDate(err error) bool {
	var e *Error
	return errors.As(err, &e) && (e.Code == ResultErrorOutOfDateKHR || e.Code == ResultSuboptimalKHR ||
		e.Code == ResultErrorSurfaceLostKHR)
}

// CString — нуль-терминированный указатель на строку для полей create-info.
// Память удерживается GC, пока живёт указатель.
func CString(s string) *byte {
	b := make([]byte, len(s)+1)
	copy(b, s)
	return &b[0]
}

// CStringSlice собирает C-массив строк (const char* const*).
// Второе возвращаемое значение нужно держать живым на время вызова Vulkan
// (runtime.KeepAlive) — либо хранить указатель в Go-структуре.
func CStringSlice(list []string) (**byte, []*byte) {
	if len(list) == 0 {
		return nil, nil
	}
	refs := make([]*byte, len(list))
	for i, s := range list {
		refs[i] = CString(s)
	}
	return &refs[0], refs
}

// cstrToString превращает нуль-терминированный буфер в строку.
func cstrToString(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}
