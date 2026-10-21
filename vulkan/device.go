package vulkan

import (
	"fmt"
	"runtime"
)

// DeviceInfo — результат подбора физического устройства под surface.
type DeviceInfo struct {
	PhysicalDevice PhysicalDevice
	Properties     PhysicalDeviceProperties
	// GraphicsFamily — семейство с очередью графики.
	GraphicsFamily uint32
	// PresentFamily — семейство, поддерживающее представление в surface.
	// Может совпадать с GraphicsFamily (обычный случай).
	PresentFamily uint32
}

// Name возвращает имя устройства.
func (d DeviceInfo) Name() string { return d.Properties.Name() }

// SeparateFamilies сообщает, что очередь представления отличается от графической
// (тогда swapchain создаётся в режиме VK_SHARING_MODE_CONCURRENT).
func (d DeviceInfo) SeparateFamilies() bool { return d.GraphicsFamily != d.PresentFamily }

// EnumeratePhysicalDevices возвращает все физические устройства.
func (a *API) EnumeratePhysicalDevices(inst Instance) ([]PhysicalDevice, error) {
	var count uint32
	if err := check("vkEnumeratePhysicalDevices", a.vkEnumeratePhysicalDevices(inst, &count, nil)); err != nil {
		return nil, err
	}
	devices := make([]PhysicalDevice, count)
	if count > 0 {
		if err := check("vkEnumeratePhysicalDevices", a.vkEnumeratePhysicalDevices(inst, &count, &devices[0])); err != nil {
			return nil, err
		}
	}
	return devices[:count], nil
}

// GetPhysicalDeviceProperties возвращает свойства устройства.
//
// Реальная VkPhysicalDeviceProperties больше Go-структуры (в ней есть limits),
// поэтому хвост в PhysicalDeviceProperties намеренно перебуферен: драйвер пишет
// гарантированно внутрь (см. комментарий к типу и layout_test.go).
func (a *API) GetPhysicalDeviceProperties(p PhysicalDevice) PhysicalDeviceProperties {
	var props PhysicalDeviceProperties
	a.vkGetPhysicalDeviceProperties(p, &props)
	return props
}

// GetPhysicalDeviceQueueFamilyProperties возвращает семейства очередей.
func (a *API) GetPhysicalDeviceQueueFamilyProperties(p PhysicalDevice) []QueueFamilyProperties {
	var count uint32
	a.vkGetPhysicalDeviceQueueFamilyProperties(p, &count, nil)
	out := make([]QueueFamilyProperties, count)
	if count > 0 {
		a.vkGetPhysicalDeviceQueueFamilyProperties(p, &count, &out[0])
	}
	return out[:count]
}

// EnumerateDeviceExtensionProperties возвращает расширения физического устройства.
func (a *API) EnumerateDeviceExtensionProperties(p PhysicalDevice) ([]ExtensionProperties, error) {
	var count uint32
	if err := check("vkEnumerateDeviceExtensionProperties",
		a.vkEnumerateDeviceExtensionProperties(p, nil, &count, nil)); err != nil {
		return nil, err
	}
	props := make([]ExtensionProperties, count)
	if count > 0 {
		if err := check("vkEnumerateDeviceExtensionProperties",
			a.vkEnumerateDeviceExtensionProperties(p, nil, &count, &props[0])); err != nil {
			return nil, err
		}
	}
	return props[:count], nil
}

// ChoosePhysicalDevice подбирает лучшее устройство под surface: нужны все
// обязательные расширения, семейство с графической очередью, очередь
// представления и непустые списки форматов/режимов. DiscreteGPU
// предпочитается интегральному.
//
// extensions — обязательные расширения устройства (обычно ["VK_KHR_swapchain"]).
func (a *API) ChoosePhysicalDevice(inst Instance, surface SurfaceKHR, extensions []string) (DeviceInfo, error) {
	devices, err := a.EnumeratePhysicalDevices(inst)
	if err != nil {
		return DeviceInfo{}, err
	}
	if len(devices) == 0 {
		return DeviceInfo{}, fmt.Errorf("vulkan: не найдено ни одного физического устройства")
	}

	best := DeviceInfo{}
	bestRank := -1

	for _, pd := range devices {
		props := a.GetPhysicalDeviceProperties(pd)

		avail, err := a.EnumerateDeviceExtensionProperties(pd)
		if err != nil {
			continue
		}
		missing := false
		for _, need := range extensions {
			if !ExtensionList(avail).Contains(need) {
				missing = true
				break
			}
		}
		if missing {
			continue
		}

		graphics, present, ok := a.chooseQueueFamilies(pd, surface)
		if !ok {
			continue
		}

		formats, err := a.GetPhysicalDeviceSurfaceFormats(pd, surface)
		if err != nil || len(formats) == 0 {
			continue
		}
		modes, err := a.GetPhysicalDeviceSurfacePresentModes(pd, surface)
		if err != nil || len(modes) == 0 {
			continue
		}

		rank := 1
		if props.DeviceType == PhysicalDeviceTypeDiscreteGPU {
			rank = 2
		}
		if rank > bestRank {
			bestRank = rank
			best = DeviceInfo{
				PhysicalDevice: pd,
				Properties:     props,
				GraphicsFamily: graphics,
				PresentFamily:  present,
			}
		}
	}

	if bestRank < 0 {
		return DeviceInfo{}, fmt.Errorf("vulkan: нет устройства, подходящего для surface (нужны %v, графическая очередь и поддержка surface)", extensions)
	}
	return best, nil
}

// chooseQueueFamilies ищет семейство графики и семейство представления;
// предпочитается одно семейство, совмещающее обе роли.
func (a *API) chooseQueueFamilies(p PhysicalDevice, surface SurfaceKHR) (graphics, present uint32, ok bool) {
	const none = ^uint32(0)
	graphics, present = none, none

	families := a.GetPhysicalDeviceQueueFamilyProperties(p)
	for i, f := range families {
		if f.QueueCount == 0 {
			continue
		}
		hasGraphics := f.QueueFlags&QueueGraphicsBit != 0

		var supported uint32
		if err := check("vkGetPhysicalDeviceSurfaceSupportKHR",
			a.vkGetPhysicalDeviceSurfaceSupportKHR(p, uint32(i), surface, &supported)); err != nil {
			continue
		}
		hasPresent := supported != 0

		if hasGraphics && hasPresent {
			// идеальный случай: одно семейство подходит для всего
			return uint32(i), uint32(i), true
		}
		if hasGraphics && graphics == none {
			graphics = uint32(i)
		}
		if hasPresent && present == none {
			present = uint32(i)
		}
	}
	if graphics == none || present == none {
		return 0, 0, false
	}
	return graphics, present, true
}

// CreateDevice создаёт логическое устройство и загружает его точки входа.
//
// queueFamilies — семейства очередей (дубликаты отбрасываются), приоритет 1.0.
// extensions — обязательные расширения устройства (["VK_KHR_swapchain"] и др.).
func (a *API) CreateDevice(info DeviceInfo, queueFamilies []uint32, extensions []string) (Device, error) {
	families := dedupe(queueFamilies)
	if len(families) == 0 {
		families = []uint32{info.GraphicsFamily}
	}

	priority := float32(1.0)
	infos := make([]DeviceQueueCreateInfo, len(families))
	for i, idx := range families {
		infos[i] = DeviceQueueCreateInfo{
			Type:             StructureTypeDeviceQueueCreateInfo,
			QueueFamilyIndex: idx,
			QueueCount:       1,
			QueuePriorities:  &priority,
		}
	}

	extNames, extKeep := CStringSlice(extensions)
	createInfo := DeviceCreateInfo{
		Type:                  StructureTypeDeviceCreateInfo,
		QueueCreateInfoCount:  uint32(len(infos)),
		QueueCreateInfos:      &infos[0],
		EnabledExtensionCount: uint32(len(extensions)),
		EnabledExtensionNames: extNames,
	}

	var dev Device
	err := check("vkCreateDevice", a.vkCreateDevice(info.PhysicalDevice, &createInfo, nil, &dev))
	runtime.KeepAlive(extKeep)
	runtime.KeepAlive(infos)
	if err != nil {
		return 0, err
	}

	a.dev = dev
	a.pd = info.PhysicalDevice // для запросов свойств памяти
	if err := a.loadDevice(); err != nil {
		a.vkDestroyDevice(dev, nil)
		a.dev = 0
		return 0, err
	}
	return dev, nil
}

// loadDevice загружает точки входа уровня device.
func (a *API) loadDevice() error {
	resolve := func(name string) uintptr {
		if a.vkGetDeviceProcAddr != nil {
			if addr := a.vkGetDeviceProcAddr(a.inst, name); addr != 0 {
				return addr
			}
		}
		return a.proc(a.inst, name) // запасной путь: device-функции через instance
	}
	p := func(name string, dst any) error { return must(dst, resolve(name), name) }

	for _, f := range []struct {
		name string
		dst  any
	}{
		{"vkDestroyDevice", &a.vkDestroyDevice},
		{"vkGetDeviceQueue", &a.vkGetDeviceQueue},
		{"vkDeviceWaitIdle", &a.vkDeviceWaitIdle},
		{"vkQueueWaitIdle", &a.vkQueueWaitIdle},
		{"vkQueueSubmit", &a.vkQueueSubmit},
		{"vkCreateImageView", &a.vkCreateImageView},
		{"vkDestroyImageView", &a.vkDestroyImageView},
		{"vkCreateRenderPass", &a.vkCreateRenderPass},
		{"vkDestroyRenderPass", &a.vkDestroyRenderPass},
		{"vkCreateFramebuffer", &a.vkCreateFramebuffer},
		{"vkDestroyFramebuffer", &a.vkDestroyFramebuffer},
		{"vkCreateCommandPool", &a.vkCreateCommandPool},
		{"vkDestroyCommandPool", &a.vkDestroyCommandPool},
		{"vkResetCommandPool", &a.vkResetCommandPool},
		{"vkResetCommandBuffer", &a.vkResetCommandBuffer},
		{"vkAllocateCommandBuffers", &a.vkAllocateCommandBuffers},
		{"vkBeginCommandBuffer", &a.vkBeginCommandBuffer},
		{"vkEndCommandBuffer", &a.vkEndCommandBuffer},
		{"vkCmdBeginRenderPass", &a.vkCmdBeginRenderPass},
		{"vkCmdEndRenderPass", &a.vkCmdEndRenderPass},
		{"vkCmdClearAttachments", &a.vkCmdClearAttachments},
		{"vkCreateSemaphore", &a.vkCreateSemaphore},
		{"vkDestroySemaphore", &a.vkDestroySemaphore},
		{"vkCreateFence", &a.vkCreateFence},
		{"vkDestroyFence", &a.vkDestroyFence},
		{"vkWaitForFences", &a.vkWaitForFences},
		{"vkResetFences", &a.vkResetFences},

		// Память/буферы/картины — для текстур UI.
		{"vkGetPhysicalDeviceMemoryProperties", &a.vkGetPhysicalDeviceMemoryProperties},
		{"vkAllocateMemory", &a.vkAllocateMemory},
		{"vkFreeMemory", &a.vkFreeMemory},
		{"vkMapMemory", &a.vkMapMemory},
		{"vkUnmapMemory", &a.vkUnmapMemory},
		{"vkCreateBuffer", &a.vkCreateBuffer},
		{"vkDestroyBuffer", &a.vkDestroyBuffer},
		{"vkGetBufferMemoryRequirements", &a.vkGetBufferMemoryRequirements},
		{"vkBindBufferMemory", &a.vkBindBufferMemory},
		{"vkCreateImage", &a.vkCreateImage},
		{"vkDestroyImage", &a.vkDestroyImage},
		{"vkGetImageMemoryRequirements", &a.vkGetImageMemoryRequirements},
		{"vkBindImageMemory", &a.vkBindImageMemory},
		{"vkCmdPipelineBarrier", &a.vkCmdPipelineBarrier},
		{"vkCmdCopyBufferToImage", &a.vkCmdCopyBufferToImage},

		// Графический пайплайн UI.
		{"vkCreateShaderModule", &a.vkCreateShaderModule},
		{"vkDestroyShaderModule", &a.vkDestroyShaderModule},
		{"vkCreatePipelineLayout", &a.vkCreatePipelineLayout},
		{"vkDestroyPipelineLayout", &a.vkDestroyPipelineLayout},
		{"vkCreateGraphicsPipelines", &a.vkCreateGraphicsPipelines},
		{"vkDestroyPipeline", &a.vkDestroyPipeline},

		// Дескрипторы и сэмплер.
		{"vkCreateDescriptorSetLayout", &a.vkCreateDescriptorSetLayout},
		{"vkDestroyDescriptorSetLayout", &a.vkDestroyDescriptorSetLayout},
		{"vkCreateDescriptorPool", &a.vkCreateDescriptorPool},
		{"vkDestroyDescriptorPool", &a.vkDestroyDescriptorPool},
		{"vkAllocateDescriptorSets", &a.vkAllocateDescriptorSets},
		{"vkUpdateDescriptorSets", &a.vkUpdateDescriptorSets},
		{"vkCreateSampler", &a.vkCreateSampler},
		{"vkDestroySampler", &a.vkDestroySampler},

		// Команды рендера UI / 3D.
		{"vkCmdBindPipeline", &a.vkCmdBindPipeline},
		{"vkCmdBindVertexBuffers", &a.vkCmdBindVertexBuffers},
		{"vkCmdBindIndexBuffer", &a.vkCmdBindIndexBuffer},
		{"vkCmdBindDescriptorSets", &a.vkCmdBindDescriptorSets},
		{"vkCmdPushConstants", &a.vkCmdPushConstants},
		{"vkCmdSetViewport", &a.vkCmdSetViewport},
		{"vkCmdSetScissor", &a.vkCmdSetScissor},
		{"vkCmdDraw", &a.vkCmdDraw},
		{"vkCmdDrawIndexed", &a.vkCmdDrawIndexed},
	} {
		if err := p(f.name, f.dst); err != nil {
			return err
		}
	}

	// Точки входа VK_KHR_swapchain: доступны только при подключённом расширении.
	// Если его нет — NewSwapchain вернёт внятную ошибку про расширение.
	for _, f := range []struct {
		name string
		dst  any
	}{
		{"vkCreateSwapchainKHR", &a.vkCreateSwapchainKHR},
		{"vkDestroySwapchainKHR", &a.vkDestroySwapchainKHR},
		{"vkGetSwapchainImagesKHR", &a.vkGetSwapchainImagesKHR},
		{"vkAcquireNextImageKHR", &a.vkAcquireNextImageKHR},
		{"vkQueuePresentKHR", &a.vkQueuePresentKHR},
	} {
		try(f.dst, resolve(f.name))
	}
	return nil
}

// DestroyDevice уничтожает логическое устройство.
func (a *API) DestroyDevice(dev Device) {
	if dev != 0 && a.vkDestroyDevice != nil {
		a.vkDestroyDevice(dev, nil)
	}
	if a.dev == dev {
		a.dev = 0
		a.pd = 0
	}
}

// GetDeviceQueue возвращает очередь по семейству и индексу.
func (a *API) GetDeviceQueue(dev Device, family, index uint32) Queue {
	var q Queue
	a.vkGetDeviceQueue(dev, family, index, &q)
	return q
}

// DeviceWaitIdle ждёт завершения всех операций устройства.
func (a *API) DeviceWaitIdle(dev Device) error {
	return check("vkDeviceWaitIdle", a.vkDeviceWaitIdle(dev))
}

// QueueWaitIdle ждёт завершения операций очереди.
func (a *API) QueueWaitIdle(q Queue) error {
	return check("vkQueueWaitIdle", a.vkQueueWaitIdle(q))
}

// QueueSubmit отправляет буферы команд в очередь.
func (a *API) QueueSubmit(q Queue, cmd CommandBuffer, wait, signal Semaphore, waitStage uint32, fence Fence) error {
	submit := SubmitInfo{
		Type:                 StructureTypeSubmitInfo,
		WaitSemaphoreCount:   boolToCount(wait != 0),
		WaitSemaphores:       semaphorePtr(wait),
		WaitDstStageMask:     stagePtr(waitStage),
		CommandBufferCount:   1,
		CommandBuffers:       &cmd,
		SignalSemaphoreCount: boolToCount(signal != 0),
		SignalSemaphores:     semaphorePtr(signal),
	}
	return check("vkQueueSubmit", a.vkQueueSubmit(q, 1, &submit, fence))
}

// держим указатели в живых: семафоры/этапы — Go-поля, поэтому их жизнь
// привязана к submit (передаётся по указателю во время вызова).
func boolToCount(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}

func semaphorePtr(s Semaphore) *Semaphore {
	if s == 0 {
		return nil
	}
	return &s
}

func stagePtr(stage uint32) *uint32 {
	if stage == 0 {
		return nil
	}
	return &stage
}

func dedupe(list []uint32) []uint32 {
	seen := make(map[uint32]bool, len(list))
	out := make([]uint32, 0, len(list))
	for _, v := range list {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
