package vulkan

import (
	"runtime"
)

// NewInstance упрощённо создаёт VkInstance: application info + расширения + слои.
//
// Проверить доступность расширений заранее можно через EnumerateInstanceExtensionProperties.
func (a *API) NewInstance(appName string, extensions []string, layers []string) (Instance, error) {
	apiVersion := APIVersion1_0
	if a.vkEnumerateInstanceVersion != nil {
		var v uint32
		if a.vkEnumerateInstanceVersion(&v).OK() && v != 0 {
			apiVersion = v
		}
	}

	appInfo := &ApplicationInfo{
		Type:               StructureTypeApplicationInfo,
		ApplicationName:    CString(appName),
		ApplicationVersion: 1,
		EngineName:         CString("Open-Kaurava"),
		EngineVersion:      1,
		APIVersion:         apiVersion,
	}
	runtime.KeepAlive(appInfo.ApplicationName)
	runtime.KeepAlive(appInfo.EngineName)

	extNames, extKeep := CStringSlice(extensions)
	layerNames, layerKeep := CStringSlice(layers)

	info := &InstanceCreateInfo{
		Type:                  StructureTypeInstanceCreateInfo,
		ApplicationInfo:       appInfo,
		EnabledLayerCount:     uint32(len(layers)),
		EnabledLayerNames:     layerNames,
		EnabledExtensionCount: uint32(len(extensions)),
		EnabledExtensionNames: extNames,
	}

	var inst Instance
	if err := check("vkCreateInstance", a.vkCreateInstance(info, nil, &inst)); err != nil {
		runtime.KeepAlive(extKeep)
		runtime.KeepAlive(layerKeep)
		return 0, err
	}
	runtime.KeepAlive(extKeep)
	runtime.KeepAlive(layerKeep)

	a.inst = inst
	a.apiVersion = apiVersion
	if err := a.loadInstance(); err != nil {
		a.vkDestroyInstance(inst, nil)
		a.inst = 0
		return 0, err
	}
	return inst, nil
}

// loadInstance загружает точки входа уровня instance.
// Вызывается автоматически из NewInstance.
func (a *API) loadInstance() error {
	inst := a.inst
	p := func(name string, dst any) error {
		return must(dst, a.proc(inst, name), name)
	}

	if err := p("vkDestroyInstance", &a.vkDestroyInstance); err != nil {
		return err
	}
	if err := p("vkEnumeratePhysicalDevices", &a.vkEnumeratePhysicalDevices); err != nil {
		return err
	}
	if err := p("vkGetPhysicalDeviceProperties", &a.vkGetPhysicalDeviceProperties); err != nil {
		return err
	}
	if err := p("vkGetPhysicalDeviceQueueFamilyProperties", &a.vkGetPhysicalDeviceQueueFamilyProperties); err != nil {
		return err
	}
	if err := p("vkEnumerateDeviceExtensionProperties", &a.vkEnumerateDeviceExtensionProperties); err != nil {
		return err
	}
	if err := p("vkCreateDevice", &a.vkCreateDevice); err != nil {
		return err
	}
	if err := p("vkGetDeviceProcAddr", &a.vkGetDeviceProcAddr); err != nil {
		return err
	}

	// Поверхности опциональны: без расширения VK_KHR_surface инстанс жив,
	// но создать surface нельзя (методы вернут внятную ошибку).
	try(&a.vkDestroySurfaceKHR, a.proc(inst, "vkDestroySurfaceKHR"))
	try(&a.vkGetPhysicalDeviceSurfaceSupportKHR, a.proc(inst, "vkGetPhysicalDeviceSurfaceSupportKHR"))
	try(&a.vkGetPhysicalDeviceSurfaceCapabilitiesKHR, a.proc(inst, "vkGetPhysicalDeviceSurfaceCapabilitiesKHR"))
	try(&a.vkGetPhysicalDeviceSurfaceFormatsKHR, a.proc(inst, "vkGetPhysicalDeviceSurfaceFormatsKHR"))
	try(&a.vkGetPhysicalDeviceSurfacePresentModesKHR, a.proc(inst, "vkGetPhysicalDeviceSurfacePresentModesKHR"))

	// Платформенные и debug-функции опциональны: активны при наличии расширения.
	try(&a.vkCreateWin32SurfaceKHR, a.proc(inst, "vkCreateWin32SurfaceKHR"))
	try(&a.vkCreateXlibSurfaceKHR, a.proc(inst, "vkCreateXlibSurfaceKHR"))
	try(&a.vkCreateWaylandSurfaceKHR, a.proc(inst, "vkCreateWaylandSurfaceKHR"))
	try(&a.vkCreateDebugUtilsMessengerEXT, a.proc(inst, "vkCreateDebugUtilsMessengerEXT"))
	try(&a.vkDestroyDebugUtilsMessengerEXT, a.proc(inst, "vkDestroyDebugUtilsMessengerEXT"))

	return nil
}

// DestroyInstance уничтожает VkInstance.
func (a *API) DestroyInstance(inst Instance) {
	if inst != 0 && a.vkDestroyInstance != nil {
		a.vkDestroyInstance(inst, nil)
	}
	if a.inst == inst {
		a.inst = 0
	}
}

// EnumerateInstanceExtensionProperties возвращает расширения (layer == "" — глобальные).
func (a *API) EnumerateInstanceExtensionProperties(layer string) ([]ExtensionProperties, error) {
	var layerName *byte
	if layer != "" {
		layerName = CString(layer)
		defer runtime.KeepAlive(layerName)
	}
	var count uint32
	if err := check("vkEnumerateInstanceExtensionProperties",
		a.vkEnumerateInstanceExtensionProperties(layerName, &count, nil)); err != nil {
		return nil, err
	}
	props := make([]ExtensionProperties, count)
	if count > 0 {
		if err := check("vkEnumerateInstanceExtensionProperties",
			a.vkEnumerateInstanceExtensionProperties(layerName, &count, &props[0])); err != nil {
			return nil, err
		}
	}
	return props[:count], nil
}

// EnumerateInstanceLayerProperties возвращает установленные слои валидации и т.п.
func (a *API) EnumerateInstanceLayerProperties() ([]LayerProperties, error) {
	var count uint32
	if err := check("vkEnumerateInstanceLayerProperties", a.vkEnumerateInstanceLayerProperties(&count, nil)); err != nil {
		return nil, err
	}
	props := make([]LayerProperties, count)
	if count > 0 {
		if err := check("vkEnumerateInstanceLayerProperties", a.vkEnumerateInstanceLayerProperties(&count, &props[0])); err != nil {
			return nil, err
		}
	}
	return props[:count], nil
}

// HasInstanceExtension проверяет, есть ли расширение у загрузчика Vulkan.
func (a *API) HasInstanceExtension(name string) (bool, error) {
	props, err := a.EnumerateInstanceExtensionProperties("")
	if err != nil {
		return false, err
	}
	return ExtensionList(props).Contains(name), nil
}

// HasLayer проверяет, установлен ли слой.
func (a *API) HasLayer(name string) (bool, error) {
	props, err := a.EnumerateInstanceLayerProperties()
	if err != nil {
		return false, err
	}
	for _, p := range props {
		if cstrToString(p.LayerName[:]) == name {
			return true, nil
		}
	}
	return false, nil
}

// ExtensionList — удобный поиск по списку расширений.
type ExtensionList []ExtensionProperties

// Contains сообщает, есть ли расширение с данным именем.
func (l ExtensionList) Contains(name string) bool {
	for _, p := range l {
		if cstrToString(p.ExtensionName[:]) == name {
			return true
		}
	}
	return false
}

// String — имена расширений через запятую (для логов).
func (l ExtensionList) String() string {
	s := ""
	for i, p := range l {
		if i > 0 {
			s += ", "
		}
		s += cstrToString(p.ExtensionName[:])
	}
	return s
}
