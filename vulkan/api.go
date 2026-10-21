package vulkan

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/Anykey-Nomad/Open-Kaurava/internal/dylib"
)

// API — открытая библиотека Vulkan вместе со всеми указателями на функции.
//
// Уровни загрузки повторяют сам Vulkan:
//
//	Load()          — vkGetInstanceProcAddr + глобальные функции (vkCreateInstance и пр.)
//	CreateInstance  — точки входа уровня instance (устройства, поверхности, ...)
//	CreateDevice    — точки входа уровня device (swapchain, командные буферы, ...)
//
// Один API соответствует одному VkInstance и одному VkDevice — так работает
// подавляющее большинство движков. Для нескольких устройств создайте два API.
type API struct {
	lib     uintptr
	libPath string

	inst Instance
	dev  Device
	pd   PhysicalDevice // запоминается в CreateDevice: нужен для памяти

	// apiVersion — версия API, запрошенная при создании инстанса.
	apiVersion uint32

	// --- глобальный уровень (из самой библиотеки) ---
	vkGetInstanceProcAddr func(instance Instance, name string) uintptr

	vkCreateInstance                       func(pCreateInfo *InstanceCreateInfo, pAllocator *AllocationCallbacks, pInstance *Instance) Result
	vkEnumerateInstanceExtensionProperties func(pLayerName *byte, pPropertyCount *uint32, pProperties *ExtensionProperties) Result
	vkEnumerateInstanceLayerProperties     func(pPropertyCount *uint32, pProperties *LayerProperties) Result
	vkEnumerateInstanceVersion             func(pApiVersion *uint32) Result // опционально (Vulkan 1.1+)

	// --- уровень instance ---
	vkDestroyInstance func(instance Instance, pAllocator *AllocationCallbacks)

	vkEnumeratePhysicalDevices               func(instance Instance, pCount *uint32, pDevices *PhysicalDevice) Result
	vkGetPhysicalDeviceProperties            func(physicalDevice PhysicalDevice, pProperties *PhysicalDeviceProperties)
	vkGetPhysicalDeviceQueueFamilyProperties func(physicalDevice PhysicalDevice, pCount *uint32, pProperties *QueueFamilyProperties)
	vkEnumerateDeviceExtensionProperties     func(physicalDevice PhysicalDevice, pLayerName *byte, pCount *uint32, pProperties *ExtensionProperties) Result

	vkGetDeviceProcAddr func(instance Instance, name string) uintptr
	vkCreateDevice      func(physicalDevice PhysicalDevice, pCreateInfo *DeviceCreateInfo, pAllocator *AllocationCallbacks, pDevice *Device) Result

	vkDestroySurfaceKHR                       func(instance Instance, surface SurfaceKHR, pAllocator *AllocationCallbacks)
	vkGetPhysicalDeviceSurfaceSupportKHR      func(physicalDevice PhysicalDevice, queueFamilyIndex uint32, surface SurfaceKHR, pSupported *uint32) Result
	vkGetPhysicalDeviceSurfaceCapabilitiesKHR func(physicalDevice PhysicalDevice, surface SurfaceKHR, pCapabilities *SurfaceCapabilitiesKHR) Result
	vkGetPhysicalDeviceSurfaceFormatsKHR      func(physicalDevice PhysicalDevice, surface SurfaceKHR, pCount *uint32, pFormats *SurfaceFormatKHR) Result
	vkGetPhysicalDeviceSurfacePresentModesKHR func(physicalDevice PhysicalDevice, surface SurfaceKHR, pCount *uint32, pModes *PresentModeKHR) Result

	// Платформенные создания поверхностей — опциональные (нужно расширение).
	vkCreateWin32SurfaceKHR   func(instance Instance, pCreateInfo *Win32SurfaceCreateInfoKHR, pAllocator *AllocationCallbacks, pSurface *SurfaceKHR) Result
	vkCreateXlibSurfaceKHR    func(instance Instance, pCreateInfo *XlibSurfaceCreateInfoKHR, pAllocator *AllocationCallbacks, pSurface *SurfaceKHR) Result
	vkCreateWaylandSurfaceKHR func(instance Instance, pCreateInfo *WaylandSurfaceCreateInfoKHR, pAllocator *AllocationCallbacks, pSurface *SurfaceKHR) Result

	// Debug — опциональные (VK_EXT_debug_utils).
	vkCreateDebugUtilsMessengerEXT  func(instance Instance, pCreateInfo *DebugUtilsMessengerCreateInfoEXT, pAllocator *AllocationCallbacks, pMessenger *DebugUtilsMessengerEXT) Result
	vkDestroyDebugUtilsMessengerEXT func(instance Instance, messenger DebugUtilsMessengerEXT, pAllocator *AllocationCallbacks)

	// --- уровень device ---
	vkDestroyDevice  func(device Device, pAllocator *AllocationCallbacks)
	vkGetDeviceQueue func(device Device, queueFamilyIndex, queueIndex uint32, pQueue *Queue)
	vkDeviceWaitIdle func(device Device) Result
	vkQueueWaitIdle  func(queue Queue) Result
	vkQueueSubmit    func(queue Queue, submitCount uint32, pSubmits *SubmitInfo, fence Fence) Result

	vkCreateSwapchainKHR    func(device Device, pCreateInfo *SwapchainCreateInfoKHR, pAllocator *AllocationCallbacks, pSwapchain *SwapchainKHR) Result
	vkDestroySwapchainKHR   func(device Device, swapchain SwapchainKHR, pAllocator *AllocationCallbacks)
	vkGetSwapchainImagesKHR func(device Device, swapchain SwapchainKHR, pCount *uint32, pImages *Image) Result
	vkAcquireNextImageKHR   func(device Device, swapchain SwapchainKHR, timeout uint64, semaphore Semaphore, fence Fence, pImageIndex *uint32) Result
	vkQueuePresentKHR       func(queue Queue, pPresentInfo *PresentInfoKHR) Result

	vkCreateImageView    func(device Device, pCreateInfo *ImageViewCreateInfo, pAllocator *AllocationCallbacks, pView *ImageView) Result
	vkDestroyImageView   func(device Device, view ImageView, pAllocator *AllocationCallbacks)
	vkCreateRenderPass   func(device Device, pCreateInfo *RenderPassCreateInfo, pAllocator *AllocationCallbacks, pRenderPass *RenderPass) Result
	vkDestroyRenderPass  func(device Device, renderPass RenderPass, pAllocator *AllocationCallbacks)
	vkCreateFramebuffer  func(device Device, pCreateInfo *FramebufferCreateInfo, pAllocator *AllocationCallbacks, pFramebuffer *Framebuffer) Result
	vkDestroyFramebuffer func(device Device, framebuffer Framebuffer, pAllocator *AllocationCallbacks)

	vkCreateCommandPool      func(device Device, pCreateInfo *CommandPoolCreateInfo, pAllocator *AllocationCallbacks, pPool *CommandPool) Result
	vkDestroyCommandPool     func(device Device, pool CommandPool, pAllocator *AllocationCallbacks)
	vkResetCommandPool       func(device Device, pool CommandPool, flags uint32) Result
	vkResetCommandBuffer     func(commandBuffer CommandBuffer, flags uint32) Result
	vkAllocateCommandBuffers func(device Device, pAllocateInfo *CommandBufferAllocateInfo, pBuffers *CommandBuffer) Result
	vkBeginCommandBuffer     func(buffer CommandBuffer, pBeginInfo *CommandBufferBeginInfo) Result
	vkEndCommandBuffer       func(buffer CommandBuffer) Result

	vkCmdBeginRenderPass  func(buffer CommandBuffer, pRenderPassBegin *RenderPassBeginInfo, contents SubpassContents)
	vkCmdEndRenderPass    func(buffer CommandBuffer)
	vkCmdClearAttachments func(buffer CommandBuffer, attachmentCount uint32, pAttachments *ClearAttachment, rectCount uint32, pRects *ClearRect)

	vkCreateSemaphore  func(device Device, pCreateInfo *SemaphoreCreateInfo, pAllocator *AllocationCallbacks, pSemaphore *Semaphore) Result
	vkDestroySemaphore func(device Device, semaphore Semaphore, pAllocator *AllocationCallbacks)
	vkCreateFence      func(device Device, pCreateInfo *FenceCreateInfo, pAllocator *AllocationCallbacks, pFence *Fence) Result
	vkDestroyFence     func(device Device, fence Fence, pAllocator *AllocationCallbacks)
	vkWaitForFences    func(device Device, fenceCount uint32, pFences *Fence, waitAll uint32, timeout uint64) Result
	vkResetFences      func(device Device, fenceCount uint32, pFences *Fence) Result

	// --- память, буферы, картины (текстуры UI) ---
	vkGetPhysicalDeviceMemoryProperties func(physicalDevice PhysicalDevice, pMemoryProperties *PhysicalDeviceMemoryProperties)
	vkAllocateMemory                    func(device Device, pAllocateInfo *MemoryAllocateInfo, pAllocator *AllocationCallbacks, pMemory *DeviceMemory) Result
	vkFreeMemory                        func(device Device, memory DeviceMemory, pAllocator *AllocationCallbacks)
	vkMapMemory                         func(device Device, memory DeviceMemory, offset, size uint64, flags uint32, ppData *unsafe.Pointer) Result
	vkUnmapMemory                       func(device Device, memory DeviceMemory)
	vkCreateBuffer                      func(device Device, pCreateInfo *BufferCreateInfo, pAllocator *AllocationCallbacks, pBuffer *Buffer) Result
	vkDestroyBuffer                     func(device Device, buffer Buffer, pAllocator *AllocationCallbacks)
	vkGetBufferMemoryRequirements       func(device Device, buffer Buffer, pMemoryRequirements *MemoryRequirements)
	vkBindBufferMemory                  func(device Device, buffer Buffer, memory DeviceMemory, memoryOffset uint64) Result
	vkCreateImage                       func(device Device, pCreateInfo *ImageCreateInfo, pAllocator *AllocationCallbacks, pImage *Image) Result
	vkDestroyImage                      func(device Device, image Image, pAllocator *AllocationCallbacks)
	vkGetImageMemoryRequirements        func(device Device, image Image, pMemoryRequirements *MemoryRequirements)
	vkBindImageMemory                   func(device Device, image Image, memory DeviceMemory, memoryOffset uint64) Result
	vkCmdPipelineBarrier                func(buffer CommandBuffer, srcStageMask, dstStageMask uint32, dependencyFlags uint32, memoryBarrierCount uint32, pMemoryBarriers unsafe.Pointer, bufferMemoryBarrierCount uint32, pBufferMemoryBarriers unsafe.Pointer, imageMemoryBarrierCount uint32, pImageMemoryBarriers *ImageMemoryBarrier)
	vkCmdCopyBufferToImage              func(buffer CommandBuffer, srcBuffer Buffer, dstImage Image, dstImageLayout ImageLayout, regionCount uint32, pRegions *BufferImageCopy)

	// --- шейдерный модуль и графический пайплайн ---
	vkCreateShaderModule      func(device Device, pCreateInfo *ShaderModuleCreateInfo, pAllocator *AllocationCallbacks, pShaderModule *ShaderModule) Result
	vkDestroyShaderModule     func(device Device, shaderModule ShaderModule, pAllocator *AllocationCallbacks)
	vkCreatePipelineLayout    func(device Device, pCreateInfo *PipelineLayoutCreateInfo, pAllocator *AllocationCallbacks, pPipelineLayout *PipelineLayout) Result
	vkDestroyPipelineLayout   func(device Device, pipelineLayout PipelineLayout, pAllocator *AllocationCallbacks)
	vkCreateGraphicsPipelines func(device Device, pipelineCache uintptr, createInfoCount uint32, pCreateInfos *GraphicsPipelineCreateInfo, pAllocator *AllocationCallbacks, pPipelines *Pipeline) Result
	vkDestroyPipeline         func(device Device, pipeline Pipeline, pAllocator *AllocationCallbacks)

	// --- дескрипторы и сэмплер ---
	vkCreateDescriptorSetLayout  func(device Device, pCreateInfo *DescriptorSetLayoutCreateInfo, pAllocator *AllocationCallbacks, pSetLayout *DescriptorSetLayout) Result
	vkDestroyDescriptorSetLayout func(device Device, setLayout DescriptorSetLayout, pAllocator *AllocationCallbacks)
	vkCreateDescriptorPool       func(device Device, pCreateInfo *DescriptorPoolCreateInfo, pAllocator *AllocationCallbacks, pDescriptorPool *DescriptorPool) Result
	vkDestroyDescriptorPool      func(device Device, descriptorPool DescriptorPool, pAllocator *AllocationCallbacks)
	vkAllocateDescriptorSets     func(device Device, pAllocateInfo *DescriptorSetAllocateInfo, pDescriptorSets *DescriptorSet) Result
	vkUpdateDescriptorSets       func(device Device, descriptorWriteCount uint32, pDescriptorWrites *WriteDescriptorSet, descriptorCopyCount uint32, pDescriptorCopies unsafe.Pointer)
	vkCreateSampler              func(device Device, pCreateInfo *SamplerCreateInfo, pAllocator *AllocationCallbacks, pSampler *Sampler) Result
	vkDestroySampler             func(device Device, sampler Sampler, pAllocator *AllocationCallbacks)

	// --- команды рендера UI / 3D ---
	vkCmdBindPipeline       func(buffer CommandBuffer, pipelineBindPoint PipelineBindPoint, pipeline Pipeline)
	vkCmdBindVertexBuffers  func(buffer CommandBuffer, firstBinding uint32, bindingCount uint32, pBuffers *Buffer, pOffsets *uint64)
	vkCmdBindIndexBuffer    func(buffer CommandBuffer, bufferIdx Buffer, offset uint64, indexType IndexType)
	vkCmdBindDescriptorSets func(buffer CommandBuffer, pipelineBindPoint PipelineBindPoint, layout PipelineLayout, firstSet uint32, descriptorSetCount uint32, pDescriptorSets *DescriptorSet, dynamicOffsetCount uint32, pDynamicOffsets *uint32)
	vkCmdPushConstants      func(buffer CommandBuffer, layout PipelineLayout, stageFlags uint32, offset uint32, size uint32, pValues unsafe.Pointer)
	vkCmdSetViewport        func(buffer CommandBuffer, firstViewport uint32, viewportCount uint32, pViewports *Viewport)
	vkCmdSetScissor         func(buffer CommandBuffer, firstScissor uint32, scissorCount uint32, pScissors *Rect2D)
	vkCmdDraw               func(buffer CommandBuffer, vertexCount, instanceCount, firstVertex, firstInstance uint32)
	vkCmdDrawIndexed        func(buffer CommandBuffer, indexCount, instanceCount, firstIndex uint32, vertexOffset int32, firstInstance uint32)
}

// Load открывает библиотеку Vulkan и загружает глобальные точки входа.
//
// path == "" — подбирается стандартное имя для ОС ("vulkan-1.dll",
// "libvulkan.so.1", "libvulkan.1.dylib"). Явный путь полезен, если драйвер
// установлен вне стандартных каталогов.
func Load(path string) (*API, error) {
	paths := []string{path}
	if path == "" {
		paths = defaultLibraries()
	}
	var lastErr error
	for _, p := range paths {
		api, err := open(p)
		if err == nil {
			return api, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("vulkan: не удалось загрузить библиотеку: %w", lastErr)
}

// defaultLibraries — кандидаты имён библиотеки Vulkan для текущей ОС.
func defaultLibraries() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{"vulkan-1.dll"}
	case "darwin":
		return []string{"libvulkan.1.dylib", "libvulkan.dylib", "libMoltenVK.dylib"}
	default:
		return []string{"libvulkan.so.1", "libvulkan.so"}
	}
}

func open(path string) (*API, error) {
	lib, err := dylib.Open(path)
	if err != nil {
		return nil, err
	}
	a := &API{lib: lib, libPath: path}

	// vkGetInstanceProcAddr — единственная функция, известная напрямую из библиотеки.
	purego.RegisterLibFunc(&a.vkGetInstanceProcAddr, lib, "vkGetInstanceProcAddr")
	if a.vkGetInstanceProcAddr(0, "vkCreateInstance") == 0 {
		return nil, fmt.Errorf("vulkan: %s не предоставляет vkCreateInstance — это не библиотека Vulkan", path)
	}

	// Глобальные функции доступны с instance == 0.
	if err := must(&a.vkCreateInstance, a.proc(0, "vkCreateInstance"), "vkCreateInstance"); err != nil {
		return nil, err
	}
	if err := must(&a.vkEnumerateInstanceExtensionProperties, a.proc(0, "vkEnumerateInstanceExtensionProperties"), "vkEnumerateInstanceExtensionProperties"); err != nil {
		return nil, err
	}
	if err := must(&a.vkEnumerateInstanceLayerProperties, a.proc(0, "vkEnumerateInstanceLayerProperties"), "vkEnumerateInstanceLayerProperties"); err != nil {
		return nil, err
	}
	try(&a.vkEnumerateInstanceVersion, a.proc(0, "vkEnumerateInstanceVersion"))

	return a, nil
}

// Lib возвращает handle загруженной библиотеки (для purego.RegisterLibFunc
// при самостоятельной загрузке расширений).
func (a *API) Lib() uintptr { return a.lib }

// Path возвращает имя/путь, по которому библиотека была загружена.
func (a *API) Path() string { return a.libPath }

// APIVersion возвращает версию API, под которую создан инстанс (0 — до создания).
func (a *API) APIVersion() uint32 { return a.apiVersion }

// Close освобождает библиотеку. Обычно не нужен: Vulkan держат до выхода.
func (a *API) Close() error {
	if a.lib == 0 {
		return nil
	}
	err := dylib.Close(a.lib)
	a.lib = 0
	return err
}

// proc — vkGetInstanceProcAddr(instance, name).
func (a *API) proc(inst Instance, name string) uintptr {
	return a.vkGetInstanceProcAddr(inst, name)
}

// Proc возвращает сырой адрес функции уровня instance/device — способ
// подключить любое расширение, которого нет в базовом слое:
//
//	addr := api.Proc("vkGetPhysicalDeviceProperties2")
//	if addr != 0 { purego.RegisterFunc(&myFn, addr) }
func (a *API) Proc(name string) uintptr {
	inst := a.inst
	if a.dev != 0 {
		// для device-функций корректнее vkGetDeviceProcAddr, но и instance-вариант работает
		if a.vkGetDeviceProcAddr != nil {
			if addr := a.vkGetDeviceProcAddr(a.inst, name); addr != 0 {
				return addr
			}
		}
	}
	return a.proc(inst, name)
}

// must регистрирует обязательную функцию; отсутствие точки входа — ошибка загрузки.
func must(dst any, addr uintptr, name string) error {
	if addr == 0 {
		return fmt.Errorf("vulkan: не найдена точка входа %s", name)
	}
	purego.RegisterFunc(dst, addr)
	return nil
}

// try регистрирует опциональную функцию (расширение/слой может отсутствовать).
func try(dst any, addr uintptr) bool {
	if addr == 0 {
		return false
	}
	purego.RegisterFunc(dst, addr)
	return true
}

// absent описывает нерабочую опциональную функцию.
func absent(name, ext string) error {
	return fmt.Errorf("vulkan: %s недоступна (нужно расширение %s)", name, ext)
}
