#!/usr/bin/env python3
"""Независимая проверка раскладок структур пакета vulkan.

Go-структуры в vulkan/structs.go обязаны совпадать по раскладке с
vulkan_core.h. Ожидаемые значения в vulkan/layout_test.go выведены из
C-заголовка вручную; чтобы не доверяться одной только ручной работе, здесь
те же структуры описаны заново на ctypes (правила выравнивания считает
другая реализация C-ABI) и печатаются в том же формате:

    VULKAN_LAYOUT_PRINT=1 go test ./vulkan -run TestLayoutPrint > go.txt
    python3 tools/layout_check.py > py.txt
    diff go.txt py.txt

Расхождение означает, что одна из двух раскладок неверна, — это гарантированный
фейл при передаче структуры в драйвер Vulkan.

Примечание: PhysicalDeviceFeatures перечисляет 56 полей VkBool32 заново —
это одновременно независимый пересчёт числа флагов в VkPhysicalDeviceFeatures.
PhysicalDeviceProperties сознательно описана с буфером `rest`, как в Go:
хвост (limits/sparseProperties) базовому слою не нужен и перебуферен.
"""

import ctypes as C
import sys

PTR = C.c_void_p  # uintptr/указатель: 8 байт на 64-битных платформах


class ApplicationInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("ApplicationName", PTR),
        ("ApplicationVersion", C.c_uint32),
        ("EngineName", PTR),
        ("EngineVersion", C.c_uint32),
        ("APIVersion", C.c_uint32),
    ]


class InstanceCreateInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("ApplicationInfo", PTR),
        ("EnabledLayerCount", C.c_uint32),
        ("EnabledLayerNames", PTR),
        ("EnabledExtensionCount", C.c_uint32),
        ("EnabledExtensionNames", PTR),
    ]


class DeviceQueueCreateInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("QueueFamilyIndex", C.c_uint32),
        ("QueueCount", C.c_uint32),
        ("QueuePriorities", PTR),
    ]


class DeviceCreateInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("QueueCreateInfoCount", C.c_uint32),
        ("QueueCreateInfos", PTR),
        ("EnabledLayerCount", C.c_uint32),
        ("EnabledLayerNames", PTR),
        ("EnabledExtensionCount", C.c_uint32),
        ("EnabledExtensionNames", PTR),
        ("EnabledFeatures", PTR),
    ]


_FEATURE_FIELDS = [
    "RobustBufferAccess", "FullDrawIndexUint32", "ImageCubeArray", "IndependentBlend",
    "GeometryShader", "TessellationShader", "SampleRateShading", "DualSrcBlend",
    "LogicOp", "MultiDrawIndirect", "DrawIndirectFirstInstance", "DepthClamp",
    "DepthBiasClamp", "FillModeNonSolid", "DepthBounds", "WideLines", "LargePoints",
    "AlphaToOne", "MultiViewport", "SamplerAnisotropy", "TextureCompressionETC2",
    "TextureCompressionASTC_LDR", "TextureCompressionBC", "OcclusionQueryPrecise",
    "PipelineStatisticsQuery", "VertexPipelineStoresAndAtomics",
    "FragmentStoresAndAtomics", "ShaderTessellationAndGeometryPointSize",
    "ShaderImageGatherExtended", "ShaderStorageImageExtendedFormats",
    "ShaderStorageImageMultisample", "ShaderStorageImageReadWithoutFormat",
    "ShaderStorageImageWriteWithoutFormat", "ShaderUniformBufferArrayDynamicIndexing",
    "ShaderSampledImageArrayDynamicIndexing", "ShaderStorageBufferArrayDynamicIndexing",
    "ShaderStorageImageArrayDynamicIndexing", "ShaderClipDistance", "ShaderCullDistance",
    "ShaderFloat64", "ShaderInt64", "ShaderInt16", "ShaderResourceResidency",
    "ShaderResourceMinLod", "SparseBinding", "SparseResidencyBuffer",
    "SparseResidencyImage2D", "SparseResidencyImage3D", "SparseResidency2Samples",
    "SparseResidency3Samples", "SparseResidency4Samples", "SparseResidency8Samples",
    "SparseResidency16Samples", "SparseResidencyAliased", "VariableMultisampleRate",
    "InheritedQueries",
]


class PhysicalDeviceFeatures(C.Structure):
    _fields_ = [(name, C.c_uint32) for name in _FEATURE_FIELDS]


class AllocationCallbacks(C.Structure):
    _fields_ = [
        ("PfnAllocation", PTR),
        ("PfnReallocation", PTR),
        ("PfnFree", PTR),
        ("PfnInternalAllocation", PTR),
        ("PfnInternalFree", PTR),
        ("UserData", PTR),
    ]


class Extent2D(C.Structure):
    _fields_ = [("Width", C.c_uint32), ("Height", C.c_uint32)]


class Extent3D(C.Structure):
    _fields_ = [("Width", C.c_uint32), ("Height", C.c_uint32), ("Depth", C.c_uint32)]


class Offset2D(C.Structure):
    _fields_ = [("X", C.c_int32), ("Y", C.c_int32)]


class Rect2D(C.Structure):
    _fields_ = [("Offset", Offset2D), ("Extent", Extent2D)]


class SurfaceFormatKHR(C.Structure):
    _fields_ = [("Format", C.c_int32), ("ColorSpace", C.c_int32)]


class SurfaceCapabilitiesKHR(C.Structure):
    _fields_ = [
        ("MinImageCount", C.c_uint32),
        ("MaxImageCount", C.c_uint32),
        ("CurrentExtent", Extent2D),
        ("MinImageExtent", Extent2D),
        ("MaxImageExtent", Extent2D),
        ("MaxImageArrayLayers", C.c_uint32),
        ("SupportedTransforms", C.c_uint32),
        ("CurrentTransform", C.c_uint32),
        ("SupportedCompositeAlpha", C.c_uint32),
        ("SupportedUsageFlags", C.c_uint32),
    ]


class SwapchainCreateInfoKHR(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("Surface", C.c_uint64),          # VkSurfaceKHR (non-dispatchable)
        ("MinImageCount", C.c_uint32),
        ("ImageFormat", C.c_int32),
        ("ImageColorSpace", C.c_int32),
        ("ImageExtent", Extent2D),
        ("ImageArrayLayers", C.c_uint32),
        ("ImageUsage", C.c_uint32),
        ("ImageSharingMode", C.c_int32),
        ("QueueFamilyIndexCount", C.c_uint32),
        ("QueueFamilyIndices", PTR),
        ("PreTransform", C.c_uint32),
        ("CompositeAlpha", C.c_uint32),
        ("PresentMode", C.c_int32),
        ("Clipped", C.c_uint32),
        ("OldSwapchain", C.c_uint64),
    ]


class ComponentMapping(C.Structure):
    _fields_ = [
        ("R", C.c_int32), ("G", C.c_int32), ("B", C.c_int32), ("A", C.c_int32),
    ]


class ImageSubresourceRange(C.Structure):
    _fields_ = [
        ("AspectMask", C.c_uint32),
        ("BaseMipLevel", C.c_uint32),
        ("LevelCount", C.c_uint32),
        ("BaseArrayLayer", C.c_uint32),
        ("LayerCount", C.c_uint32),
    ]


class ImageViewCreateInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("Image", C.c_uint64),      # VkImage — non-dispatchable
        ("ViewType", C.c_int32),
        ("Format", C.c_int32),
        ("Components", ComponentMapping),
        ("SubresourceRange", ImageSubresourceRange),
    ]


class AttachmentDescription(C.Structure):
    _fields_ = [
        ("Flags", C.c_uint32),
        ("Format", C.c_int32),
        ("Samples", C.c_int32),
        ("LoadOp", C.c_int32),
        ("StoreOp", C.c_int32),
        ("StencilLoadOp", C.c_int32),
        ("StencilStoreOp", C.c_int32),
        ("InitialLayout", C.c_int32),
        ("FinalLayout", C.c_int32),
    ]


class AttachmentReference(C.Structure):
    _fields_ = [("Attachment", C.c_uint32), ("Layout", C.c_int32)]


class SubpassDescription(C.Structure):
    _fields_ = [
        ("Flags", C.c_uint32),
        ("PipelineBindPoint", C.c_int32),
        ("InputAttachmentCount", C.c_uint32),
        ("InputAttachments", PTR),
        ("ColorAttachmentCount", C.c_uint32),
        ("ColorAttachments", PTR),
        ("ResolveAttachments", PTR),
        ("DepthStencilAttachment", PTR),
        ("PreserveAttachmentCount", C.c_uint32),
        ("PreserveAttachments", PTR),
    ]


class SubpassDependency(C.Structure):
    _fields_ = [
        ("SrcSubpass", C.c_uint32),
        ("DstSubpass", C.c_uint32),
        ("SrcStageMask", C.c_uint32),
        ("DstStageMask", C.c_uint32),
        ("SrcAccessMask", C.c_uint32),
        ("DstAccessMask", C.c_uint32),
        ("DependencyFlags", C.c_uint32),
    ]


class RenderPassCreateInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("AttachmentCount", C.c_uint32),
        ("Attachments", PTR),
        ("SubpassCount", C.c_uint32),
        ("Subpasses", PTR),
        ("DependencyCount", C.c_uint32),
        ("Dependencies", PTR),
    ]


class FramebufferCreateInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("RenderPass", C.c_uint64),
        ("AttachmentCount", C.c_uint32),
        ("Attachments", PTR),
        ("Width", C.c_uint32),
        ("Height", C.c_uint32),
        ("Layers", C.c_uint32),
    ]


class CommandPoolCreateInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("QueueFamilyIndex", C.c_uint32),
    ]


class CommandBufferAllocateInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("CommandPool", C.c_uint64),
        ("Level", C.c_int32),
        ("CommandBufferCount", C.c_uint32),
    ]


class CommandBufferBeginInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("InheritanceInfo", PTR),
    ]


class ClearColorValue(C.Structure):
    _fields_ = [("Float32", C.c_float * 4)]


class ClearValue(C.Structure):
    _fields_ = [("Color", ClearColorValue)]


class ClearAttachment(C.Structure):
    _fields_ = [
        ("AspectMask", C.c_uint32),
        ("ColorAttachment", C.c_uint32),
        ("ClearValue", ClearValue),
    ]


class ClearRect(C.Structure):
    _fields_ = [
        ("Rect", Rect2D),
        ("BaseArrayLayer", C.c_uint32),
        ("LayerCount", C.c_uint32),
    ]


class RenderPassBeginInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("RenderPass", C.c_uint64),
        ("Framebuffer", C.c_uint64),
        ("RenderArea", Rect2D),
        ("ClearValueCount", C.c_uint32),
        ("ClearValues", PTR),
    ]


class SemaphoreCreateInfo(C.Structure):
    _fields_ = [("Type", C.c_int32), ("Next", PTR), ("Flags", C.c_uint32)]


class FenceCreateInfo(C.Structure):
    _fields_ = [("Type", C.c_int32), ("Next", PTR), ("Flags", C.c_uint32)]


class SubmitInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("WaitSemaphoreCount", C.c_uint32),
        ("WaitSemaphores", PTR),
        ("WaitDstStageMask", PTR),
        ("CommandBufferCount", C.c_uint32),
        ("CommandBuffers", PTR),
        ("SignalSemaphoreCount", C.c_uint32),
        ("SignalSemaphores", PTR),
    ]


class PresentInfoKHR(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("WaitSemaphoreCount", C.c_uint32),
        ("WaitSemaphores", PTR),
        ("SwapchainCount", C.c_uint32),
        ("Swapchains", PTR),
        ("ImageIndices", PTR),
        ("Results", PTR),
    ]


class ExtensionProperties(C.Structure):
    _fields_ = [("ExtensionName", C.c_char * 256), ("SpecVersion", C.c_uint32)]


class LayerProperties(C.Structure):
    _fields_ = [
        ("LayerName", C.c_char * 256),
        ("SpecVersion", C.c_uint32),
        ("ImplementationVersion", C.c_uint32),
        ("Description", C.c_char * 256),
    ]


class QueueFamilyProperties(C.Structure):
    _fields_ = [
        ("QueueFlags", C.c_uint32),
        ("QueueCount", C.c_uint32),
        ("TimestampValidBits", C.c_uint32),
        ("MinImageTransferGranularity", Extent3D),
    ]


class PhysicalDeviceProperties(C.Structure):
    _fields_ = [
        ("APIVersion", C.c_uint32),
        ("DriverVersion", C.c_uint32),
        ("VendorID", C.c_uint32),
        ("DeviceID", C.c_uint32),
        ("DeviceType", C.c_int32),
        ("DeviceName", C.c_char * 256),
        ("PipelineCacheUUID", C.c_uint8 * 16),
        ("LimitsAlign", C.c_uint32),  # выравнивание limits до 8, как в C
        ("rest", C.c_byte * 4096),  # limits + sparseProperties, как в Go
    ]


class DebugUtilsMessengerCreateInfoEXT(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("MessageSeverity", C.c_uint32),
        ("MessageType", C.c_uint32),
        ("UserCallback", PTR),
        ("UserData", PTR),
    ]


class DebugUtilsCallbackDataEXT(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("MessageIDName", PTR),
        ("MessageIDNumber", C.c_int32),
        ("Message", PTR),
        ("QueueLabelCount", C.c_uint32),
        ("QueueLabels", PTR),
        ("CmdBufLabelCount", C.c_uint32),
        ("CmdBufLabels", PTR),
        ("ObjectCount", C.c_uint32),
        ("Objects", PTR),
    ]


class Win32SurfaceCreateInfoKHR(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("HInstance", PTR),
        ("Hwnd", PTR),
    ]


class XlibSurfaceCreateInfoKHR(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("Dpy", PTR),
        ("Window", C.c_uint64),  # XID: unsigned long на LP64
    ]


class WaylandSurfaceCreateInfoKHR(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("Display", PTR),
        ("Surface", PTR),
    ]


# --- Память/буферы/картины ---

class MemoryAllocateInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("AllocationSize", C.c_uint64),
        ("MemoryTypeIndex", C.c_uint32),
    ]


class MemoryRequirements(C.Structure):
    _fields_ = [
        ("Size", C.c_uint64),
        ("Alignment", C.c_uint64),
        ("MemoryTypeBits", C.c_uint32),
    ]


class MemoryType(C.Structure):
    _fields_ = [
        ("PropertyFlags", C.c_uint32),
        ("HeapIndex", C.c_uint32),
    ]


class MemoryHeap(C.Structure):
    _fields_ = [
        ("Size", C.c_uint64),
        ("Flags", C.c_uint32),
    ]


class PhysicalDeviceMemoryProperties(C.Structure):
    _fields_ = [
        ("MemoryTypeCount", C.c_uint32),
        ("MemoryTypes", MemoryType * 32),
        ("MemoryHeapCount", C.c_uint32),
        ("MemoryHeaps", MemoryHeap * 16),
    ]


class BufferCreateInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("Size", C.c_uint64),
        ("Usage", C.c_uint32),
        ("SharingMode", C.c_int32),
        ("QueueFamilyIndexCount", C.c_uint32),
        ("QueueFamilyIndices", PTR),
    ]


class ImageCreateInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("ImageType", C.c_int32),
        ("Format", C.c_int32),
        ("Extent", Extent3D),
        ("MipLevels", C.c_uint32),
        ("ArrayLayers", C.c_uint32),
        ("Samples", C.c_int32),
        ("Tiling", C.c_int32),
        ("Usage", C.c_uint32),
        ("SharingMode", C.c_int32),
        ("QueueFamilyIndexCount", C.c_uint32),
        ("QueueFamilyIndices", PTR),
        ("InitialLayout", C.c_int32),
    ]


class ImageSubresourceLayers(C.Structure):
    _fields_ = [
        ("AspectMask", C.c_uint32),
        ("MipLevel", C.c_uint32),
        ("BaseArrayLayer", C.c_uint32),
        ("LayerCount", C.c_uint32),
    ]


class Offset3D(C.Structure):
    _fields_ = [("X", C.c_int32), ("Y", C.c_int32), ("Z", C.c_int32)]


class BufferImageCopy(C.Structure):
    _fields_ = [
        ("BufferOffset", C.c_uint64),
        ("BufferRowLength", C.c_uint32),
        ("BufferImageHeight", C.c_uint32),
        ("ImageSubresource", ImageSubresourceLayers),
        ("ImageOffset", Offset3D),
        ("ImageExtent", Extent3D),
    ]


class ImageMemoryBarrier(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("SrcAccessMask", C.c_uint32),
        ("DstAccessMask", C.c_uint32),
        ("OldLayout", C.c_int32),
        ("NewLayout", C.c_int32),
        ("SrcQueueFamilyIndex", C.c_uint32),
        ("DstQueueFamilyIndex", C.c_uint32),
        ("Image", C.c_uint64),
        ("SubresourceRange", ImageSubresourceRange),
    ]


# --- Пайплайн UI ---

class ShaderModuleCreateInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("CodeSize", C.c_size_t),
        ("Code", PTR),
    ]


class PipelineShaderStageCreateInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("Stage", C.c_uint32),
        ("Module", C.c_uint64),
        ("Name", PTR),
        ("Specialization", PTR),
    ]


class VertexInputBindingDescription(C.Structure):
    _fields_ = [
        ("Binding", C.c_uint32),
        ("Stride", C.c_uint32),
        ("InputRate", C.c_int32),
    ]


class VertexInputAttributeDescription(C.Structure):
    _fields_ = [
        ("Location", C.c_uint32),
        ("Binding", C.c_uint32),
        ("Format", C.c_int32),
        ("Offset", C.c_uint32),
    ]


class PipelineVertexInputStateCreateInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("VertexBindingDescriptionCount", C.c_uint32),
        ("VertexBindingDescriptions", PTR),
        ("VertexAttributeDescriptionCount", C.c_uint32),
        ("VertexAttributeDescriptions", PTR),
    ]


class PipelineInputAssemblyStateCreateInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("Topology", C.c_int32),
        ("PrimitiveRestartEnable", C.c_uint32),
    ]


class Viewport(C.Structure):
    _fields_ = [
        ("X", C.c_float), ("Y", C.c_float),
        ("Width", C.c_float), ("Height", C.c_float),
        ("MinDepth", C.c_float), ("MaxDepth", C.c_float),
    ]


class PipelineViewportStateCreateInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("ViewportCount", C.c_uint32),
        ("Viewports", PTR),
        ("ScissorCount", C.c_uint32),
        ("Scissors", PTR),
    ]


class PipelineRasterizationStateCreateInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("DepthClampEnable", C.c_uint32),
        ("RasterizerDiscardEnable", C.c_uint32),
        ("PolygonMode", C.c_int32),
        ("CullMode", C.c_int32),
        ("FrontFace", C.c_int32),
        ("DepthBiasEnable", C.c_uint32),
        ("DepthBiasConstantFactor", C.c_float),
        ("DepthBiasClamp", C.c_float),
        ("DepthBiasSlopeFactor", C.c_float),
        ("LineWidth", C.c_float),
    ]


class PipelineMultisampleStateCreateInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("RasterizationSamples", C.c_int32),
        ("SampleShadingEnable", C.c_uint32),
        ("MinSampleShading", C.c_float),
        ("SampleMask", PTR),
        ("AlphaToCoverageEnable", C.c_uint32),
        ("AlphaToOneEnable", C.c_uint32),
    ]


class StencilOpState(C.Structure):
    _fields_ = [
        ("FailOp", C.c_int32),
        ("PassOp", C.c_int32),
        ("DepthFailOp", C.c_int32),
        ("CompareOp", C.c_int32),
        ("CompareMask", C.c_uint32),
        ("WriteMask", C.c_uint32),
        ("Reference", C.c_uint32),
    ]


class PipelineDepthStencilStateCreateInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("DepthTestEnable", C.c_uint32),
        ("DepthWriteEnable", C.c_uint32),
        ("DepthCompareOp", C.c_int32),
        ("DepthBoundsTestEnable", C.c_uint32),
        ("StencilTestEnable", C.c_uint32),
        ("Front", StencilOpState),
        ("Back", StencilOpState),
        ("MinDepthBounds", C.c_float),
        ("MaxDepthBounds", C.c_float),
    ]


class PipelineColorBlendAttachmentState(C.Structure):
    _fields_ = [
        ("BlendEnable", C.c_uint32),
        ("SrcColorBlendFactor", C.c_int32),
        ("DstColorBlendFactor", C.c_int32),
        ("ColorBlendOp", C.c_int32),
        ("SrcAlphaBlendFactor", C.c_int32),
        ("DstAlphaBlendFactor", C.c_int32),
        ("AlphaBlendOp", C.c_int32),
        ("ColorWriteMask", C.c_uint32),
    ]


class PipelineColorBlendStateCreateInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("LogicOpEnable", C.c_uint32),
        ("LogicOp", C.c_uint32),
        ("AttachmentCount", C.c_uint32),
        ("Attachments", PTR),
        ("BlendConstants", C.c_float * 4),
    ]


class PipelineDynamicStateCreateInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("DynamicStateCount", C.c_uint32),
        ("DynamicStates", PTR),
    ]


class PushConstantRange(C.Structure):
    _fields_ = [
        ("StageFlags", C.c_uint32),
        ("Offset", C.c_uint32),
        ("Size", C.c_uint32),
    ]


class PipelineLayoutCreateInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("SetLayoutCount", C.c_uint32),
        ("SetLayouts", PTR),
        ("PushConstantRangeCount", C.c_uint32),
        ("PushConstantRanges", PTR),
    ]


class GraphicsPipelineCreateInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("StageCount", C.c_uint32),
        ("Stages", PTR),
        ("VertexInputState", PTR),
        ("InputAssemblyState", PTR),
        ("TessellationState", PTR),
        ("ViewportState", PTR),
        ("RasterizationState", PTR),
        ("MultisampleState", PTR),
        ("DepthStencilState", PTR),
        ("ColorBlendState", PTR),
        ("DynamicState", PTR),
        ("Layout", C.c_uint64),
        ("RenderPass", C.c_uint64),
        ("Subpass", C.c_uint32),
        ("BasePipelineHandle", C.c_uint64),
        ("BasePipelineIndex", C.c_int32),
    ]


# --- Дескрипторы и сэмплер ---

class DescriptorSetLayoutBinding(C.Structure):
    _fields_ = [
        ("Binding", C.c_uint32),
        ("DescriptorType", C.c_int32),
        ("DescriptorCount", C.c_uint32),
        ("StageFlags", C.c_uint32),
        ("ImmutableSamplers", PTR),
    ]


class DescriptorSetLayoutCreateInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("BindingCount", C.c_uint32),
        ("Bindings", PTR),
    ]


class DescriptorPoolSize(C.Structure):
    _fields_ = [("Type", C.c_int32), ("Count", C.c_uint32)]


class DescriptorPoolCreateInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("MaxSets", C.c_uint32),
        ("PoolSizeCount", C.c_uint32),
        ("PoolSizes", PTR),
    ]


class DescriptorSetAllocateInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("DescriptorPool", C.c_uint64),
        ("DescriptorSetCount", C.c_uint32),
        ("SetLayouts", PTR),
    ]


class DescriptorImageInfo(C.Structure):
    _fields_ = [
        ("Sampler", C.c_uint64),
        ("ImageView", C.c_uint64),
        ("ImageLayout", C.c_int32),
    ]


class WriteDescriptorSet(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("DstSet", C.c_uint64),
        ("DstBinding", C.c_uint32),
        ("DstArrayElement", C.c_uint32),
        ("DescriptorCount", C.c_uint32),
        ("DescriptorType", C.c_int32),
        ("ImageInfo", PTR),
        ("BufferInfo", PTR),
        ("TexelBufferView", PTR),
    ]


class SamplerCreateInfo(C.Structure):
    _fields_ = [
        ("Type", C.c_int32),
        ("Next", PTR),
        ("Flags", C.c_uint32),
        ("MagFilter", C.c_int32),
        ("MinFilter", C.c_int32),
        ("MipmapMode", C.c_int32),
        ("AddressModeU", C.c_int32),
        ("AddressModeV", C.c_int32),
        ("AddressModeW", C.c_int32),
        ("MipLodBias", C.c_float),
        ("AnisotropyEnable", C.c_uint32),
        ("MaxAnisotropy", C.c_float),
        ("CompareEnable", C.c_uint32),
        ("CompareOp", C.c_int32),
        ("MinLod", C.c_float),
        ("MaxLod", C.c_float),
        ("BorderColor", C.c_int32),
        ("UnnormalizedCoordinates", C.c_uint32),
    ]


# Имя в Go → класс ctypes (имена полей обязаны совпадать с Go).
STRUCTS = {
    "ApplicationInfo": ApplicationInfo,
    "InstanceCreateInfo": InstanceCreateInfo,
    "DeviceQueueCreateInfo": DeviceQueueCreateInfo,
    "DeviceCreateInfo": DeviceCreateInfo,
    "PhysicalDeviceFeatures": PhysicalDeviceFeatures,
    "AllocationCallbacks": AllocationCallbacks,
    "Extent2D": Extent2D,
    "Extent3D": Extent3D,
    "Offset2D": Offset2D,
    "Rect2D": Rect2D,
    "SurfaceFormatKHR": SurfaceFormatKHR,
    "SurfaceCapabilitiesKHR": SurfaceCapabilitiesKHR,
    "SwapchainCreateInfoKHR": SwapchainCreateInfoKHR,
    "ComponentMapping": ComponentMapping,
    "ImageSubresourceRange": ImageSubresourceRange,
    "ImageViewCreateInfo": ImageViewCreateInfo,
    "AttachmentDescription": AttachmentDescription,
    "AttachmentReference": AttachmentReference,
    "SubpassDescription": SubpassDescription,
    "SubpassDependency": SubpassDependency,
    "RenderPassCreateInfo": RenderPassCreateInfo,
    "FramebufferCreateInfo": FramebufferCreateInfo,
    "CommandPoolCreateInfo": CommandPoolCreateInfo,
    "CommandBufferAllocateInfo": CommandBufferAllocateInfo,
    "CommandBufferBeginInfo": CommandBufferBeginInfo,
    "ClearColorValue": ClearColorValue,
    "ClearValue": ClearValue,
    "ClearAttachment": ClearAttachment,
    "ClearRect": ClearRect,
    "RenderPassBeginInfo": RenderPassBeginInfo,
    "SemaphoreCreateInfo": SemaphoreCreateInfo,
    "FenceCreateInfo": FenceCreateInfo,
    "SubmitInfo": SubmitInfo,
    "PresentInfoKHR": PresentInfoKHR,
    "ExtensionProperties": ExtensionProperties,
    "LayerProperties": LayerProperties,
    "QueueFamilyProperties": QueueFamilyProperties,
    "PhysicalDeviceProperties": PhysicalDeviceProperties,
    "DebugUtilsMessengerCreateInfoEXT": DebugUtilsMessengerCreateInfoEXT,
    "debugUtilsCallbackDataEXT": DebugUtilsCallbackDataEXT,
    "Win32SurfaceCreateInfoKHR": Win32SurfaceCreateInfoKHR,
    "XlibSurfaceCreateInfoKHR": XlibSurfaceCreateInfoKHR,
    "WaylandSurfaceCreateInfoKHR": WaylandSurfaceCreateInfoKHR,
    "MemoryAllocateInfo": MemoryAllocateInfo,
    "MemoryRequirements": MemoryRequirements,
    "MemoryType": MemoryType,
    "MemoryHeap": MemoryHeap,
    "PhysicalDeviceMemoryProperties": PhysicalDeviceMemoryProperties,
    "BufferCreateInfo": BufferCreateInfo,
    "ImageCreateInfo": ImageCreateInfo,
    "ImageSubresourceLayers": ImageSubresourceLayers,
    "BufferImageCopy": BufferImageCopy,
    "Offset3D": Offset3D,
    "ImageMemoryBarrier": ImageMemoryBarrier,
    "ShaderModuleCreateInfo": ShaderModuleCreateInfo,
    "PipelineShaderStageCreateInfo": PipelineShaderStageCreateInfo,
    "VertexInputBindingDescription": VertexInputBindingDescription,
    "VertexInputAttributeDescription": VertexInputAttributeDescription,
    "PipelineVertexInputStateCreateInfo": PipelineVertexInputStateCreateInfo,
    "PipelineInputAssemblyStateCreateInfo": PipelineInputAssemblyStateCreateInfo,
    "Viewport": Viewport,
    "PipelineViewportStateCreateInfo": PipelineViewportStateCreateInfo,
    "PipelineRasterizationStateCreateInfo": PipelineRasterizationStateCreateInfo,
    "PipelineMultisampleStateCreateInfo": PipelineMultisampleStateCreateInfo,
    "StencilOpState": StencilOpState,
    "PipelineDepthStencilStateCreateInfo": PipelineDepthStencilStateCreateInfo,
    "PipelineColorBlendAttachmentState": PipelineColorBlendAttachmentState,
    "PipelineColorBlendStateCreateInfo": PipelineColorBlendStateCreateInfo,
    "PipelineDynamicStateCreateInfo": PipelineDynamicStateCreateInfo,
    "PushConstantRange": PushConstantRange,
    "PipelineLayoutCreateInfo": PipelineLayoutCreateInfo,
    "GraphicsPipelineCreateInfo": GraphicsPipelineCreateInfo,
    "DescriptorSetLayoutBinding": DescriptorSetLayoutBinding,
    "DescriptorSetLayoutCreateInfo": DescriptorSetLayoutCreateInfo,
    "DescriptorPoolSize": DescriptorPoolSize,
    "DescriptorPoolCreateInfo": DescriptorPoolCreateInfo,
    "DescriptorSetAllocateInfo": DescriptorSetAllocateInfo,
    "DescriptorImageInfo": DescriptorImageInfo,
    "WriteDescriptorSet": WriteDescriptorSet,
    "SamplerCreateInfo": SamplerCreateInfo,
}


def main():
    if sys.maxsize < 2 ** 62:
        print("layout_check: нужна 64-битная Python (ptr=%d байт)" % C.sizeof(PTR),
              file=sys.stderr)
        return 1

    # Имя в выводе — имя Go-типа (у одного оно с маленькой буквы), сортировка
    # по нему же: вывод должен байт-в-байт совпадать с go test.
    pairs = []
    for py_name, cls in STRUCTS.items():
        go_name = "debugUtilsCallbackDataEXT" if py_name == "DebugUtilsCallbackDataEXT" else py_name
        pairs.append((go_name, cls))

    for go_name, cls in sorted(pairs, key=lambda p: p[0]):
        fields = sorted(f[0] for f in cls._fields_)
        parts = ["LAYOUT %s size=%d" % (go_name, C.sizeof(cls))]
        for field in fields:
            parts.append("%s=%d" % (field, getattr(cls, field).offset))
        print(" ".join(parts))
    return 0


if __name__ == "__main__":
    sys.exit(main())
