package vulkan

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unsafe"
)

// Проверка того, что раскладка Go-структур совпадает с vulkan_core.h.
//
// Ожидаемые значения выведены из определений C-заголовка по правилам выравнивания
// C: порядок полей идентичен, padding рассчитывается одинаково. Независимая
// реализация тех же правил — ctypes — лежит в tools/layout_check.py; она печатает
// строки того же формата, и два вывода сравниваются диффом (см. README):
//
//	VULKAN_LAYOUT_PRINT=1 go test ./vulkan -run TestLayoutPrint -v > go.txt
//	python3 tools/layout_check.py > py.txt
//	diff go.txt py.txt

type layoutCase struct {
	name    string
	size    uintptr
	offsets map[string]uintptr
}

// types возвращает нулевые значения структур для reflect.
func types() map[string]any {
	return map[string]any{
		"ApplicationInfo":                  ApplicationInfo{},
		"InstanceCreateInfo":               InstanceCreateInfo{},
		"DeviceQueueCreateInfo":            DeviceQueueCreateInfo{},
		"DeviceCreateInfo":                 DeviceCreateInfo{},
		"PhysicalDeviceFeatures":           PhysicalDeviceFeatures{},
		"AllocationCallbacks":              AllocationCallbacks{},
		"Extent2D":                         Extent2D{},
		"Extent3D":                         Extent3D{},
		"Offset2D":                         Offset2D{},
		"Rect2D":                           Rect2D{},
		"SurfaceFormatKHR":                 SurfaceFormatKHR{},
		"SurfaceCapabilitiesKHR":           SurfaceCapabilitiesKHR{},
		"SwapchainCreateInfoKHR":           SwapchainCreateInfoKHR{},
		"ComponentMapping":                 ComponentMapping{},
		"ImageSubresourceRange":            ImageSubresourceRange{},
		"ImageViewCreateInfo":              ImageViewCreateInfo{},
		"AttachmentDescription":            AttachmentDescription{},
		"AttachmentReference":              AttachmentReference{},
		"SubpassDescription":               SubpassDescription{},
		"SubpassDependency":                SubpassDependency{},
		"RenderPassCreateInfo":             RenderPassCreateInfo{},
		"FramebufferCreateInfo":            FramebufferCreateInfo{},
		"CommandPoolCreateInfo":            CommandPoolCreateInfo{},
		"CommandBufferAllocateInfo":        CommandBufferAllocateInfo{},
		"CommandBufferBeginInfo":           CommandBufferBeginInfo{},
		"ClearColorValue":                  ClearColorValue{},
		"ClearValue":                       ClearValue{},
		"ClearAttachment":                  ClearAttachment{},
		"ClearRect":                        ClearRect{},
		"RenderPassBeginInfo":              RenderPassBeginInfo{},
		"SemaphoreCreateInfo":              SemaphoreCreateInfo{},
		"FenceCreateInfo":                  FenceCreateInfo{},
		"SubmitInfo":                       SubmitInfo{},
		"PresentInfoKHR":                   PresentInfoKHR{},
		"ExtensionProperties":              ExtensionProperties{},
		"LayerProperties":                  LayerProperties{},
		"QueueFamilyProperties":            QueueFamilyProperties{},
		"PhysicalDeviceProperties":         PhysicalDeviceProperties{},
		"DebugUtilsMessengerCreateInfoEXT": DebugUtilsMessengerCreateInfoEXT{},
		"debugUtilsCallbackDataEXT":        debugUtilsCallbackDataEXT{},
		"Win32SurfaceCreateInfoKHR":        Win32SurfaceCreateInfoKHR{},
		"XlibSurfaceCreateInfoKHR":         XlibSurfaceCreateInfoKHR{},
		"WaylandSurfaceCreateInfoKHR":      WaylandSurfaceCreateInfoKHR{},

		// Память/буферы/картины.
		"MemoryAllocateInfo":             MemoryAllocateInfo{},
		"MemoryRequirements":             MemoryRequirements{},
		"MemoryType":                     MemoryType{},
		"MemoryHeap":                     MemoryHeap{},
		"PhysicalDeviceMemoryProperties": PhysicalDeviceMemoryProperties{},
		"BufferCreateInfo":               BufferCreateInfo{},
		"ImageCreateInfo":                ImageCreateInfo{},
		"ImageSubresourceLayers":         ImageSubresourceLayers{},
		"BufferImageCopy":                BufferImageCopy{},
		"Offset3D":                       Offset3D{},
		"ImageMemoryBarrier":             ImageMemoryBarrier{},

		// Пайплайн UI.
		"ShaderModuleCreateInfo":               ShaderModuleCreateInfo{},
		"PipelineShaderStageCreateInfo":        PipelineShaderStageCreateInfo{},
		"VertexInputBindingDescription":        VertexInputBindingDescription{},
		"VertexInputAttributeDescription":      VertexInputAttributeDescription{},
		"PipelineVertexInputStateCreateInfo":   PipelineVertexInputStateCreateInfo{},
		"PipelineInputAssemblyStateCreateInfo": PipelineInputAssemblyStateCreateInfo{},
		"Viewport":                             Viewport{},
		"PipelineViewportStateCreateInfo":      PipelineViewportStateCreateInfo{},
		"PipelineRasterizationStateCreateInfo": PipelineRasterizationStateCreateInfo{},
		"StencilOpState":                       StencilOpState{},
		"PipelineDepthStencilStateCreateInfo":  PipelineDepthStencilStateCreateInfo{},
		"PipelineMultisampleStateCreateInfo":   PipelineMultisampleStateCreateInfo{},
		"PipelineColorBlendAttachmentState":    PipelineColorBlendAttachmentState{},
		"PipelineColorBlendStateCreateInfo":    PipelineColorBlendStateCreateInfo{},
		"PipelineDynamicStateCreateInfo":       PipelineDynamicStateCreateInfo{},
		"PushConstantRange":                    PushConstantRange{},
		"PipelineLayoutCreateInfo":             PipelineLayoutCreateInfo{},
		"GraphicsPipelineCreateInfo":           GraphicsPipelineCreateInfo{},

		// Дескрипторы и сэмплер.
		"DescriptorSetLayoutBinding":    DescriptorSetLayoutBinding{},
		"DescriptorSetLayoutCreateInfo": DescriptorSetLayoutCreateInfo{},
		"DescriptorPoolSize":            DescriptorPoolSize{},
		"DescriptorPoolCreateInfo":      DescriptorPoolCreateInfo{},
		"DescriptorSetAllocateInfo":     DescriptorSetAllocateInfo{},
		"DescriptorImageInfo":           DescriptorImageInfo{},
		"WriteDescriptorSet":            WriteDescriptorSet{},
		"SamplerCreateInfo":             SamplerCreateInfo{},
	}
}

func layoutCases() []layoutCase {
	return []layoutCase{
		{"ApplicationInfo", 48, map[string]uintptr{
			"Type": 0, "Next": 8, "ApplicationName": 16, "ApplicationVersion": 24,
			"EngineName": 32, "EngineVersion": 40, "APIVersion": 44,
		}},
		{"InstanceCreateInfo", 64, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16, "ApplicationInfo": 24,
			"EnabledLayerCount": 32, "EnabledLayerNames": 40,
			"EnabledExtensionCount": 48, "EnabledExtensionNames": 56,
		}},
		{"DeviceQueueCreateInfo", 40, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16, "QueueFamilyIndex": 20,
			"QueueCount": 24, "QueuePriorities": 32,
		}},
		{"DeviceCreateInfo", 72, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16, "QueueCreateInfoCount": 20,
			"QueueCreateInfos": 24, "EnabledLayerCount": 32, "EnabledLayerNames": 40,
			"EnabledExtensionCount": 48, "EnabledExtensionNames": 56, "EnabledFeatures": 64,
		}},
		{"PhysicalDeviceFeatures", 224, map[string]uintptr{}},
		{"AllocationCallbacks", 48, map[string]uintptr{"PfnAllocation": 0, "UserData": 40}},
		{"Extent2D", 8, map[string]uintptr{"Width": 0, "Height": 4}},
		{"Extent3D", 12, map[string]uintptr{"Width": 0, "Height": 4, "Depth": 8}},
		{"Offset2D", 8, map[string]uintptr{"X": 0, "Y": 4}},
		{"Rect2D", 16, map[string]uintptr{"Offset": 0, "Extent": 8}},
		{"SurfaceFormatKHR", 8, map[string]uintptr{"Format": 0, "ColorSpace": 4}},
		{"SurfaceCapabilitiesKHR", 52, map[string]uintptr{
			"MinImageCount": 0, "MaxImageCount": 4, "CurrentExtent": 8,
			"MinImageExtent": 16, "MaxImageExtent": 24, "MaxImageArrayLayers": 32,
			"SupportedTransforms": 36, "CurrentTransform": 40,
			"SupportedCompositeAlpha": 44, "SupportedUsageFlags": 48,
		}},
		{"SwapchainCreateInfoKHR", 104, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16, "Surface": 24, "MinImageCount": 32,
			"ImageFormat": 36, "ImageColorSpace": 40, "ImageExtent": 44,
			"ImageArrayLayers": 52, "ImageUsage": 56, "ImageSharingMode": 60,
			"QueueFamilyIndexCount": 64, "QueueFamilyIndices": 72, "PreTransform": 80,
			"CompositeAlpha": 84, "PresentMode": 88, "Clipped": 92, "OldSwapchain": 96,
		}},
		{"ComponentMapping", 16, map[string]uintptr{"R": 0, "G": 4, "B": 8, "A": 12}},
		{"ImageSubresourceRange", 20, map[string]uintptr{
			"AspectMask": 0, "BaseMipLevel": 4, "LevelCount": 8,
			"BaseArrayLayer": 12, "LayerCount": 16,
		}},
		{"ImageViewCreateInfo", 80, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16, "Image": 24, "ViewType": 32,
			"Format": 36, "Components": 40, "SubresourceRange": 56,
		}},
		{"AttachmentDescription", 36, map[string]uintptr{
			"Flags": 0, "Format": 4, "Samples": 8, "LoadOp": 12, "StoreOp": 16,
			"StencilLoadOp": 20, "StencilStoreOp": 24, "InitialLayout": 28, "FinalLayout": 32,
		}},
		{"AttachmentReference", 8, map[string]uintptr{"Attachment": 0, "Layout": 4}},
		{"SubpassDescription", 72, map[string]uintptr{
			"Flags": 0, "PipelineBindPoint": 4, "InputAttachmentCount": 8,
			"InputAttachments": 16, "ColorAttachmentCount": 24, "ColorAttachments": 32,
			"ResolveAttachments": 40, "DepthStencilAttachment": 48,
			"PreserveAttachmentCount": 56, "PreserveAttachments": 64,
		}},
		{"SubpassDependency", 28, map[string]uintptr{
			"SrcSubpass": 0, "DstSubpass": 4, "SrcStageMask": 8, "DstStageMask": 12,
			"SrcAccessMask": 16, "DstAccessMask": 20, "DependencyFlags": 24,
		}},
		{"RenderPassCreateInfo", 64, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16, "AttachmentCount": 20,
			"Attachments": 24, "SubpassCount": 32, "Subpasses": 40,
			"DependencyCount": 48, "Dependencies": 56,
		}},
		{"FramebufferCreateInfo", 64, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16, "RenderPass": 24,
			"AttachmentCount": 32, "Attachments": 40, "Width": 48, "Height": 52, "Layers": 56,
		}},
		{"CommandPoolCreateInfo", 24, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16, "QueueFamilyIndex": 20,
		}},
		{"CommandBufferAllocateInfo", 32, map[string]uintptr{
			"Type": 0, "Next": 8, "CommandPool": 16, "Level": 24, "CommandBufferCount": 28,
		}},
		{"CommandBufferBeginInfo", 32, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16, "InheritanceInfo": 24,
		}},
		{"ClearColorValue", 16, map[string]uintptr{}},
		{"ClearValue", 16, map[string]uintptr{"Color": 0}},
		{"ClearAttachment", 24, map[string]uintptr{
			"AspectMask": 0, "ColorAttachment": 4, "ClearValue": 8,
		}},
		{"ClearRect", 24, map[string]uintptr{
			"Rect": 0, "BaseArrayLayer": 16, "LayerCount": 20,
		}},
		{"RenderPassBeginInfo", 64, map[string]uintptr{
			"Type": 0, "Next": 8, "RenderPass": 16, "Framebuffer": 24,
			"RenderArea": 32, "ClearValueCount": 48, "ClearValues": 56,
		}},
		{"SemaphoreCreateInfo", 24, map[string]uintptr{"Type": 0, "Next": 8, "Flags": 16}},
		{"FenceCreateInfo", 24, map[string]uintptr{"Type": 0, "Next": 8, "Flags": 16}},
		{"SubmitInfo", 72, map[string]uintptr{
			"Type": 0, "Next": 8, "WaitSemaphoreCount": 16, "WaitSemaphores": 24,
			"WaitDstStageMask": 32, "CommandBufferCount": 40, "CommandBuffers": 48,
			"SignalSemaphoreCount": 56, "SignalSemaphores": 64,
		}},
		{"PresentInfoKHR", 64, map[string]uintptr{
			"Type": 0, "Next": 8, "WaitSemaphoreCount": 16, "WaitSemaphores": 24,
			"SwapchainCount": 32, "Swapchains": 40, "ImageIndices": 48, "Results": 56,
		}},
		{"ExtensionProperties", 260, map[string]uintptr{"ExtensionName": 0, "SpecVersion": 256}},
		{"LayerProperties", 520, map[string]uintptr{
			"LayerName": 0, "SpecVersion": 256, "ImplementationVersion": 260, "Description": 264,
		}},
		{"QueueFamilyProperties", 24, map[string]uintptr{
			"QueueFlags": 0, "QueueCount": 4, "TimestampValidBits": 8,
			"MinImageTransferGranularity": 12,
		}},
		// Хвост структуры перебуферен намеренно (см. PhysicalDeviceProperties):
		// важны только поля до limits.
		{"PhysicalDeviceProperties", 4392, map[string]uintptr{
			"APIVersion": 0, "DriverVersion": 4, "VendorID": 8, "DeviceID": 12,
			"DeviceType": 16, "DeviceName": 20, "PipelineCacheUUID": 276,
		}},
		{"DebugUtilsMessengerCreateInfoEXT", 48, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16, "MessageSeverity": 20,
			"MessageType": 24, "UserCallback": 32, "UserData": 40,
		}},
		{"debugUtilsCallbackDataEXT", 96, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16, "MessageIDName": 24,
			"MessageIDNumber": 32, "Message": 40, "QueueLabelCount": 48,
			"QueueLabels": 56, "CmdBufLabelCount": 64, "CmdBufLabels": 72,
			"ObjectCount": 80, "Objects": 88,
		}},
		{"Win32SurfaceCreateInfoKHR", 40, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16, "HInstance": 24, "Hwnd": 32,
		}},
		// Память/буферы/картины.
		{"MemoryAllocateInfo", 32, map[string]uintptr{
			"Type": 0, "Next": 8, "AllocationSize": 16, "MemoryTypeIndex": 24,
		}},
		{"MemoryRequirements", 24, map[string]uintptr{
			"Size": 0, "Alignment": 8, "MemoryTypeBits": 16,
		}},
		{"MemoryType", 8, map[string]uintptr{"PropertyFlags": 0, "HeapIndex": 4}},
		{"MemoryHeap", 16, map[string]uintptr{"Size": 0, "Flags": 8}},
		// Полная VkPhysicalDeviceMemoryProperties: 4 + 32×8 + 4 + pad4 + 16×16 = 520.
		{"PhysicalDeviceMemoryProperties", 520, map[string]uintptr{
			"MemoryTypeCount": 0, "MemoryTypes": 4,
			"MemoryHeapCount": 260, "MemoryHeaps": 264,
		}},
		{"BufferCreateInfo", 56, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16, "Size": 24, "Usage": 32,
			"SharingMode": 36, "QueueFamilyIndexCount": 40, "QueueFamilyIndices": 48,
		}},
		{"ImageCreateInfo", 88, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16, "ImageType": 20, "Format": 24,
			"Extent": 28, "MipLevels": 40, "ArrayLayers": 44, "Samples": 48,
			"Tiling": 52, "Usage": 56, "SharingMode": 60,
			"QueueFamilyIndexCount": 64, "QueueFamilyIndices": 72, "InitialLayout": 80,
		}},
		{"ImageSubresourceLayers", 16, map[string]uintptr{
			"AspectMask": 0, "MipLevel": 4, "BaseArrayLayer": 8, "LayerCount": 12,
		}},
		{"Offset3D", 12, map[string]uintptr{"X": 0, "Y": 4, "Z": 8}},
		{"BufferImageCopy", 56, map[string]uintptr{
			"BufferOffset": 0, "BufferRowLength": 8, "BufferImageHeight": 12,
			"ImageSubresource": 16, "ImageOffset": 32, "ImageExtent": 44,
		}},
		{"ImageMemoryBarrier", 72, map[string]uintptr{
			"Type": 0, "Next": 8, "SrcAccessMask": 16, "DstAccessMask": 20,
			"OldLayout": 24, "NewLayout": 28, "SrcQueueFamilyIndex": 32,
			"DstQueueFamilyIndex": 36, "Image": 40, "SubresourceRange": 48,
		}},
		// Пайплайн UI.
		{"ShaderModuleCreateInfo", 40, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16, "CodeSize": 24, "Code": 32,
		}},
		{"PipelineShaderStageCreateInfo", 48, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16, "Stage": 20, "Module": 24,
			"Name": 32, "Specialization": 40,
		}},
		{"VertexInputBindingDescription", 12, map[string]uintptr{
			"Binding": 0, "Stride": 4, "InputRate": 8,
		}},
		{"VertexInputAttributeDescription", 16, map[string]uintptr{
			"Location": 0, "Binding": 4, "Format": 8, "Offset": 12,
		}},
		{"PipelineVertexInputStateCreateInfo", 48, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16,
			"VertexBindingDescriptionCount": 20, "VertexBindingDescriptions": 24,
			"VertexAttributeDescriptionCount": 32, "VertexAttributeDescriptions": 40,
		}},
		{"PipelineInputAssemblyStateCreateInfo", 32, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16, "Topology": 20, "PrimitiveRestartEnable": 24,
		}},
		{"Viewport", 24, map[string]uintptr{
			"X": 0, "Y": 4, "Width": 8, "Height": 12, "MinDepth": 16, "MaxDepth": 20,
		}},
		{"PipelineViewportStateCreateInfo", 48, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16, "ViewportCount": 20, "Viewports": 24,
			"ScissorCount": 32, "Scissors": 40,
		}},
		// Выравнивание 8 (pNext): 60 округляется до 64 и в C, и в Go.
		{"PipelineRasterizationStateCreateInfo", 64, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16, "DepthClampEnable": 20,
			"RasterizerDiscardEnable": 24, "PolygonMode": 28, "CullMode": 32,
			"FrontFace": 36, "DepthBiasEnable": 40, "DepthBiasConstantFactor": 44,
			"DepthBiasClamp": 48, "DepthBiasSlopeFactor": 52, "LineWidth": 56,
		}},
		{"PipelineMultisampleStateCreateInfo", 48, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16, "RasterizationSamples": 20,
			"SampleShadingEnable": 24, "MinSampleShading": 28, "SampleMask": 32,
			"AlphaToCoverageEnable": 40, "AlphaToOneEnable": 44,
		}},
		{"StencilOpState", 28, map[string]uintptr{
			"FailOp": 0, "PassOp": 4, "DepthFailOp": 8, "CompareOp": 12,
			"CompareMask": 16, "WriteMask": 20, "Reference": 24,
		}},
		{"PipelineDepthStencilStateCreateInfo", 104, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16, "DepthTestEnable": 20,
			"DepthWriteEnable": 24, "DepthCompareOp": 28,
			"DepthBoundsTestEnable": 32, "StencilTestEnable": 36,
			"Front": 40, "Back": 68, "MinDepthBounds": 96, "MaxDepthBounds": 100,
		}},
		{"PipelineColorBlendAttachmentState", 32, map[string]uintptr{
			"BlendEnable": 0, "SrcColorBlendFactor": 4, "DstColorBlendFactor": 8,
			"ColorBlendOp": 12, "SrcAlphaBlendFactor": 16, "DstAlphaBlendFactor": 20,
			"AlphaBlendOp": 24, "ColorWriteMask": 28,
		}},
		{"PipelineColorBlendStateCreateInfo", 56, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16, "LogicOpEnable": 20, "LogicOp": 24,
			"AttachmentCount": 28, "Attachments": 32, "BlendConstants": 40,
		}},
		{"PipelineDynamicStateCreateInfo", 32, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16, "DynamicStateCount": 20, "DynamicStates": 24,
		}},
		{"PushConstantRange", 12, map[string]uintptr{
			"StageFlags": 0, "Offset": 4, "Size": 8,
		}},
		{"PipelineLayoutCreateInfo", 48, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16, "SetLayoutCount": 20, "SetLayouts": 24,
			"PushConstantRangeCount": 32, "PushConstantRanges": 40,
		}},
		{"GraphicsPipelineCreateInfo", 144, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16, "StageCount": 20, "Stages": 24,
			"VertexInputState": 32, "InputAssemblyState": 40, "TessellationState": 48,
			"ViewportState": 56, "RasterizationState": 64, "MultisampleState": 72,
			"DepthStencilState": 80, "ColorBlendState": 88, "DynamicState": 96,
			"Layout": 104, "RenderPass": 112, "Subpass": 120,
			"BasePipelineHandle": 128, "BasePipelineIndex": 136,
		}},
		// Дескрипторы и сэмплер.
		{"DescriptorSetLayoutBinding", 24, map[string]uintptr{
			"Binding": 0, "DescriptorType": 4, "DescriptorCount": 8,
			"StageFlags": 12, "ImmutableSamplers": 16,
		}},
		{"DescriptorSetLayoutCreateInfo", 32, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16, "BindingCount": 20, "Bindings": 24,
		}},
		{"DescriptorPoolSize", 8, map[string]uintptr{"Type": 0, "Count": 4}},
		{"DescriptorPoolCreateInfo", 40, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16, "MaxSets": 20,
			"PoolSizeCount": 24, "PoolSizes": 32,
		}},
		{"DescriptorSetAllocateInfo", 40, map[string]uintptr{
			"Type": 0, "Next": 8, "DescriptorPool": 16,
			"DescriptorSetCount": 24, "SetLayouts": 32,
		}},
		{"DescriptorImageInfo", 24, map[string]uintptr{
			"Sampler": 0, "ImageView": 8, "ImageLayout": 16,
		}},
		{"WriteDescriptorSet", 64, map[string]uintptr{
			"Type": 0, "Next": 8, "DstSet": 16, "DstBinding": 24,
			"DstArrayElement": 28, "DescriptorCount": 32, "DescriptorType": 36,
			"ImageInfo": 40, "BufferInfo": 48, "TexelBufferView": 56,
		}},
		{"SamplerCreateInfo", 80, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16, "MagFilter": 20, "MinFilter": 24,
			"MipmapMode": 28, "AddressModeU": 32, "AddressModeV": 36, "AddressModeW": 40,
			"MipLodBias": 44, "AnisotropyEnable": 48, "MaxAnisotropy": 52,
			"CompareEnable": 56, "CompareOp": 60, "MinLod": 64, "MaxLod": 68,
			"BorderColor": 72, "UnnormalizedCoordinates": 76,
		}},
		{"XlibSurfaceCreateInfoKHR", 40, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16, "Dpy": 24, "Window": 32,
		}},
		{"WaylandSurfaceCreateInfoKHR", 40, map[string]uintptr{
			"Type": 0, "Next": 8, "Flags": 16, "Display": 24, "Surface": 32,
		}},
	}
}

// sizeOf — unsafe.Sizeof структуры по имени; ^uintptr(0), если имя неизвестно.
func sizeOf(name string) uintptr {
	zero, ok := types()[name]
	if !ok {
		return ^uintptr(0)
	}
	return reflect.TypeOf(zero).Size()
}

// offsetOf — unsafe.Offsetof поля по имени структуры и поля.
func offsetOf(name, field string) (uintptr, bool) {
	zero, ok := types()[name]
	if !ok {
		return 0, false
	}
	f, ok := reflect.TypeOf(zero).FieldByName(field)
	if !ok {
		return 0, false
	}
	return f.Offset, true
}

// TestStructLayouts сверяет размеры и критичные смещения с ожидаемыми значениями.
func TestStructLayouts(t *testing.T) {
	for _, tc := range layoutCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if actual := sizeOf(tc.name); actual != tc.size {
				t.Errorf("размер %s: got %d, want %d", tc.name, actual, tc.size)
			}
			for field, want := range tc.offsets {
				got, ok := offsetOf(tc.name, field)
				if !ok {
					t.Errorf("%s.%s: поле не найдено", tc.name, field)
					continue
				}
				if got != want {
					t.Errorf("смещение %s.%s: got %d, want %d", tc.name, field, got, want)
				}
			}
		})
	}
}

// TestLayoutPrint печатает раскладки в формате, дублируемом tools/layout_check.py.
// Печатаются ВСЕ поля каждой структуры (без таблицы ожиданий), чтобы вывод
// можно было байт-в-байт сравнить с независимым ctypes-расчётом.
// Включается переменной окружения, чтобы обычный go test не мусорил в stdout.
func TestLayoutPrint(t *testing.T) {
	if os.Getenv("VULKAN_LAYOUT_PRINT") == "" {
		t.Skip("установите VULKAN_LAYOUT_PRINT=1, чтобы напечатать раскладки")
	}
	names := make([]string, 0, len(types()))
	for n := range types() {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, name := range names {
		typ := reflect.TypeOf(types()[name])
		var b strings.Builder
		b.WriteString("LAYOUT " + name + " size=" + strconv.Itoa(int(typ.Size())))

		type fieldOff struct {
			name string
			off  uintptr
		}
		fields := make([]fieldOff, 0, typ.NumField())
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if f.Name == "_" {
				continue // явный padding: в выводе его нет, но и ctypes его не печатает под именем
			}
			fields = append(fields, fieldOff{f.Name, f.Offset})
		}
		sort.Slice(fields, func(i, j int) bool { return fields[i].name < fields[j].name })
		for _, f := range fields {
			b.WriteString(" " + f.name + "=" + strconv.Itoa(int(f.off)))
		}
		fmt.Fprintln(os.Stdout, b.String())
	}
}

// TestPhysicalDevicePropertiesFits — проверка «в живую»: драйвер пишет в нашу
// структуру, а мы убеждаемся, что запись не вышла за её границы (канарейка)
// и что имя устройства читается корректно. Пропускается, если Vulkan недоступен.
func TestPhysicalDevicePropertiesFits(t *testing.T) {
	api, err := Load("")
	if err != nil {
		t.Skipf("Vulkan недоступен: %v", err)
	}
	defer api.Close()

	inst, err := api.NewInstance("layout-test", nil, nil)
	if err != nil {
		t.Skipf("не удалось создать инстанс: %v", err)
	}
	defer api.DestroyInstance(inst)

	devices, err := api.EnumeratePhysicalDevices(inst)
	if err != nil || len(devices) == 0 {
		t.Skipf("нет физических устройств: %v", err)
	}

	// Буфер: наша структура + запас за её спиной, весь залит канарейкой.
	type padded struct {
		props PhysicalDeviceProperties
		guard [16384]byte
	}
	var c padded
	raw := unsafe.Slice((*byte)(unsafe.Pointer(&c)), int(unsafe.Sizeof(c)))
	for i := range raw {
		raw[i] = 0xAA
	}

	api.vkGetPhysicalDeviceProperties(devices[0], &c.props)

	// Последний изменённый байт ≈ фактический размер VkPhysicalDeviceProperties.
	last := -1
	for i, b := range raw {
		if b != 0xAA {
			last = i
		}
	}
	if last < 0 {
		t.Fatal("драйвер не записал ничего в VkPhysicalDeviceProperties")
	}
	written := uintptr(last + 1)
	ours := unsafe.Sizeof(c.props)
	t.Logf("драйвер записал %d байт, наша структура = %d байт", written, ours)

	// Диагностика: первые 32 байта — как их записал драйвер.
	if written >= 32 {
		t.Logf("hex[0..32] = % x", raw[:32])
		t.Logf("apiVersion=%d(0x%x) driverVersion=%d vendorID=0x%x deviceID=0x%x deviceType=%d",
			c.props.APIVersion, c.props.APIVersion, c.props.DriverVersion,
			c.props.VendorID, c.props.DeviceID, c.props.DeviceType)
	}

	if written > ours {
		t.Errorf("драйвер записал %d байт, а Go-структура только %d — раскладка небезопасна", written, ours)
	}

	name := c.props.Name()
	if name == "" {
		t.Error("имя устройства пустое: DeviceName не совпал с раскладкой")
	} else {
		t.Logf("устройство: %q, API %d.%d.%d",
			name,
			(c.props.APIVersion>>22)&0x7F, (c.props.APIVersion>>12)&0x3FF, c.props.APIVersion&0xFFF)
	}
	if c.props.DeviceType < PhysicalDeviceTypeOther || c.props.DeviceType > PhysicalDeviceTypeCPU {
		t.Errorf("неожиданный deviceType = %d (возможна ошибка раскладки)", c.props.DeviceType)
	}
}
