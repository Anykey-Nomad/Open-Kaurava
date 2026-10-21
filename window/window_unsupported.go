//go:build !windows && !linux

package window

import (
	"fmt"
	"runtime"

	"github.com/Anykey-Nomad/Open-Kaurava/vulkan"
)

// Заглушка для платформ без бэкенда (macOS, BSD, js, ...).
//
// Пакет window компилируется везде, но New возвращает внятную ошибку.
// Биндинги Vulkan при этом остаются рабочими: на macOS нужен MoltenVK плюс
// бэкенд окна поверх Cocoa — его здесь сознательно нет.

type nativeState struct{}

func (n *nativeState) init(w *Window, cfg Config) error {
	return fmt.Errorf("window: бэкенд окна не реализован для %s (поддержаны windows и linux)", runtime.GOOS)
}

func (n *nativeState) poll(w *Window)    {}
func (n *nativeState) destroy(w *Window) {}

func (n *nativeState) setSize(width, height int) error {
	return fmt.Errorf("window: SetSize не поддерживается на %s", runtime.GOOS)
}
func (n *nativeState) backend() string { return "unsupported:" + runtime.GOOS }

func (n *nativeState) captureMouse(capture bool) error {
	return fmt.Errorf("window: захват курсора не поддерживается на %s", runtime.GOOS)
}

func (n *nativeState) setCursorVisible(visible bool) {}

func (n *nativeState) surfaceExtension() string { return "" }

func (n *nativeState) createSurface(api *vulkan.API, instance vulkan.Instance) (vulkan.SurfaceKHR, error) {
	return 0, fmt.Errorf("window: создание поверхности не поддерживается на %s", runtime.GOOS)
}
