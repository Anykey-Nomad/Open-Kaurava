package vulkan

// Платформенные поверхности.
//
// Функции создания лежат в одном кросс-платформенном пакете: различается только
// структура create-info, а её раскладка одинакова на всех ОС. Какую из них
// вызвать, решает слой window — он знает нативные хэндлы своего окна.
// Вся ветвление по ОС остаётся в window/*_windows.go и window/*_linux.go.

// CreateWin32Surface — vkCreateWin32SurfaceKHR (нужно расширение VK_KHR_win32_surface).
func (a *API) CreateWin32Surface(inst Instance, hinstance, hwnd uintptr) (SurfaceKHR, error) {
	if a.vkCreateWin32SurfaceKHR == nil {
		return 0, absent("vkCreateWin32SurfaceKHR", ExtWin32Surface)
	}
	info := Win32SurfaceCreateInfoKHR{
		Type:      StructureTypeWin32SurfaceCreateInfoKHR,
		HInstance: hinstance,
		Hwnd:      hwnd,
	}
	var s SurfaceKHR
	if err := check("vkCreateWin32SurfaceKHR", a.vkCreateWin32SurfaceKHR(inst, &info, nil, &s)); err != nil {
		return 0, err
	}
	return s, nil
}

// CreateXlibSurface — vkCreateXlibSurfaceKHR (нужно расширение VK_KHR_xlib_surface).
func (a *API) CreateXlibSurface(inst Instance, display, window uintptr) (SurfaceKHR, error) {
	if a.vkCreateXlibSurfaceKHR == nil {
		return 0, absent("vkCreateXlibSurfaceKHR", ExtXlibSurface)
	}
	info := XlibSurfaceCreateInfoKHR{
		Type:   StructureTypeXlibSurfaceCreateInfoKHR,
		Dpy:    display,
		Window: window,
	}
	var s SurfaceKHR
	if err := check("vkCreateXlibSurfaceKHR", a.vkCreateXlibSurfaceKHR(inst, &info, nil, &s)); err != nil {
		return 0, err
	}
	return s, nil
}

// CreateWaylandSurface — vkCreateWaylandSurfaceKHR (нужно расширение VK_KHR_wayland_surface).
func (a *API) CreateWaylandSurface(inst Instance, display, surface uintptr) (SurfaceKHR, error) {
	if a.vkCreateWaylandSurfaceKHR == nil {
		return 0, absent("vkCreateWaylandSurfaceKHR", ExtWaylandSurface)
	}
	info := WaylandSurfaceCreateInfoKHR{
		Type:    StructureTypeWaylandSurfaceCreateInfoKHR,
		Display: display,
		Surface: surface,
	}
	var s SurfaceKHR
	if err := check("vkCreateWaylandSurfaceKHR", a.vkCreateWaylandSurfaceKHR(inst, &info, nil, &s)); err != nil {
		return 0, err
	}
	return s, nil
}

// DestroySurface — vkDestroySurfaceKHR.
func (a *API) DestroySurface(inst Instance, s SurfaceKHR) {
	if s != 0 && a.vkDestroySurfaceKHR != nil {
		a.vkDestroySurfaceKHR(inst, s, nil)
	}
}

// GetPhysicalDeviceSurfaceSupport сообщает, поддерживает ли семейство очередь представление.
func (a *API) GetPhysicalDeviceSurfaceSupport(pd PhysicalDevice, family uint32, surface SurfaceKHR) (bool, error) {
	if a.vkGetPhysicalDeviceSurfaceSupportKHR == nil {
		return false, absent("vkGetPhysicalDeviceSurfaceSupportKHR", ExtSurface)
	}
	var supported uint32
	if err := check("vkGetPhysicalDeviceSurfaceSupportKHR",
		a.vkGetPhysicalDeviceSurfaceSupportKHR(pd, family, surface, &supported)); err != nil {
		return false, err
	}
	return supported != 0, nil
}

// GetPhysicalDeviceSurfaceCapabilities возвращает возможности поверхности.
func (a *API) GetPhysicalDeviceSurfaceCapabilities(pd PhysicalDevice, surface SurfaceKHR) (SurfaceCapabilitiesKHR, error) {
	if a.vkGetPhysicalDeviceSurfaceCapabilitiesKHR == nil {
		return SurfaceCapabilitiesKHR{}, absent("vkGetPhysicalDeviceSurfaceCapabilitiesKHR", ExtSurface)
	}
	var caps SurfaceCapabilitiesKHR
	if err := check("vkGetPhysicalDeviceSurfaceCapabilitiesKHR",
		a.vkGetPhysicalDeviceSurfaceCapabilitiesKHR(pd, surface, &caps)); err != nil {
		return SurfaceCapabilitiesKHR{}, err
	}
	return caps, nil
}

// GetPhysicalDeviceSurfaceFormats возвращает доступные форматы.
func (a *API) GetPhysicalDeviceSurfaceFormats(pd PhysicalDevice, surface SurfaceKHR) ([]SurfaceFormatKHR, error) {
	if a.vkGetPhysicalDeviceSurfaceFormatsKHR == nil {
		return nil, absent("vkGetPhysicalDeviceSurfaceFormatsKHR", ExtSurface)
	}
	var count uint32
	if err := check("vkGetPhysicalDeviceSurfaceFormatsKHR",
		a.vkGetPhysicalDeviceSurfaceFormatsKHR(pd, surface, &count, nil)); err != nil {
		return nil, err
	}
	// count == 0 означает «любой формат» → возвращаем специальную пару.
	if count == 0 {
		return []SurfaceFormatKHR{{Format: FormatUndefined, ColorSpace: ColorSpaceSRGBNonlinearKHR}}, nil
	}
	formats := make([]SurfaceFormatKHR, count)
	if err := check("vkGetPhysicalDeviceSurfaceFormatsKHR",
		a.vkGetPhysicalDeviceSurfaceFormatsKHR(pd, surface, &count, &formats[0])); err != nil {
		return nil, err
	}
	return formats[:count], nil
}

// GetPhysicalDeviceSurfacePresentModes возвращает доступные режимы представления.
func (a *API) GetPhysicalDeviceSurfacePresentModes(pd PhysicalDevice, surface SurfaceKHR) ([]PresentModeKHR, error) {
	if a.vkGetPhysicalDeviceSurfacePresentModesKHR == nil {
		return nil, absent("vkGetPhysicalDeviceSurfacePresentModesKHR", ExtSurface)
	}
	var count uint32
	if err := check("vkGetPhysicalDeviceSurfacePresentModesKHR",
		a.vkGetPhysicalDeviceSurfacePresentModesKHR(pd, surface, &count, nil)); err != nil {
		return nil, err
	}
	modes := make([]PresentModeKHR, count)
	if count > 0 {
		if err := check("vkGetPhysicalDeviceSurfacePresentModesKHR",
			a.vkGetPhysicalDeviceSurfacePresentModesKHR(pd, surface, &count, &modes[0])); err != nil {
			return nil, err
		}
	}
	return modes[:count], nil
}
