package vulkan

import "unsafe"

// Структуры Vulkan.
//
// ГЛАВНОЕ ПРАВИЛО: порядок полей обязан совпадать с заголовком vulkan_core.h
// буквально. Выравнивание и «дырки» Go рассчитывает так же, как C, поэтому
// явные padding-поля не нужны — наоборот, лишний _ uint32 сломает раскладку.
// Поля-указатели объявлены как Go-указатели (*T, unsafe.Pointer), а не uintptr:
// тогда GC видит ссылку и память под параметры переживёт вызов Vulkan.
//
// Проверка раскладки: см. layout_test.go и tools/layout_check.py.

// ApplicationInfo — VkApplicationInfo (48 байт).
type ApplicationInfo struct {
	Type               StructureType  // sType
	Next               unsafe.Pointer // pNext
	ApplicationName    *byte          // const char*
	ApplicationVersion uint32
	EngineName         *byte // const char*
	EngineVersion      uint32
	APIVersion         uint32
}

// InstanceCreateInfo — VkInstanceCreateInfo (64 байта).
type InstanceCreateInfo struct {
	Type                  StructureType
	Next                  unsafe.Pointer
	Flags                 uint32
	ApplicationInfo       *ApplicationInfo
	EnabledLayerCount     uint32
	EnabledLayerNames     **byte // const char* const*
	EnabledExtensionCount uint32
	EnabledExtensionNames **byte // const char* const*
}

// DeviceQueueCreateInfo — VkDeviceQueueCreateInfo (40 байт).
type DeviceQueueCreateInfo struct {
	Type             StructureType
	Next             unsafe.Pointer
	Flags            uint32
	QueueFamilyIndex uint32
	QueueCount       uint32
	QueuePriorities  *float32 // const float*
}

// DeviceCreateInfo — VkDeviceCreateInfo (72 байта).
//
// enabledLayerCount / enabledLayerNames — legacy-поля из заголовка,
// их место в раскладке обязано быть сохранено, хотя мы всегда передаём 0/nil.
type DeviceCreateInfo struct {
	Type                  StructureType
	Next                  unsafe.Pointer
	Flags                 uint32
	QueueCreateInfoCount  uint32
	QueueCreateInfos      *DeviceQueueCreateInfo
	EnabledLayerCount     uint32
	EnabledLayerNames     **byte
	EnabledExtensionCount uint32
	EnabledExtensionNames **byte
	EnabledFeatures       unsafe.Pointer // *PhysicalDeviceFeatures; в базе не используется (nil)
}

// PhysicalDeviceFeatures — VkPhysicalDeviceFeatures: 56 флагов VkBool32 (224 байта).
// Определён для корректной раскладки; в базовом слое фичи не включаются.
type PhysicalDeviceFeatures struct {
	RobustBufferAccess                      uint32
	FullDrawIndexUint32                     uint32
	ImageCubeArray                          uint32
	IndependentBlend                        uint32
	GeometryShader                          uint32
	TessellationShader                      uint32
	SampleRateShading                       uint32
	DualSrcBlend                            uint32
	LogicOp                                 uint32
	MultiDrawIndirect                       uint32
	DrawIndirectFirstInstance               uint32
	DepthClamp                              uint32
	DepthBiasClamp                          uint32
	FillModeNonSolid                        uint32
	DepthBounds                             uint32
	WideLines                               uint32
	LargePoints                             uint32
	AlphaToOne                              uint32
	MultiViewport                           uint32
	SamplerAnisotropy                       uint32
	TextureCompressionETC2                  uint32
	TextureCompressionASTC_LDR              uint32
	TextureCompressionBC                    uint32
	OcclusionQueryPrecise                   uint32
	PipelineStatisticsQuery                 uint32
	VertexPipelineStoresAndAtomics          uint32
	FragmentStoresAndAtomics                uint32
	ShaderTessellationAndGeometryPointSize  uint32
	ShaderImageGatherExtended               uint32
	ShaderStorageImageExtendedFormats       uint32
	ShaderStorageImageMultisample           uint32
	ShaderStorageImageReadWithoutFormat     uint32
	ShaderStorageImageWriteWithoutFormat    uint32
	ShaderUniformBufferArrayDynamicIndexing uint32
	ShaderSampledImageArrayDynamicIndexing  uint32
	ShaderStorageBufferArrayDynamicIndexing uint32
	ShaderStorageImageArrayDynamicIndexing  uint32
	ShaderClipDistance                      uint32
	ShaderCullDistance                      uint32
	ShaderFloat64                           uint32
	ShaderInt64                             uint32
	ShaderInt16                             uint32
	ShaderResourceResidency                 uint32
	ShaderResourceMinLod                    uint32
	SparseBinding                           uint32
	SparseResidencyBuffer                   uint32
	SparseResidencyImage2D                  uint32
	SparseResidencyImage3D                  uint32
	SparseResidency2Samples                 uint32
	SparseResidency3Samples                 uint32
	SparseResidency4Samples                 uint32
	SparseResidency8Samples                 uint32
	SparseResidency16Samples                uint32
	SparseResidencyAliased                  uint32
	VariableMultisampleRate                 uint32
	InheritedQueries                        uint32
}

// AllocationCallbacks — VkAllocationCallbacks (48 байт).
// Базовый слой всегда передаёт nil: используется аллокатор Vulkan по умолчанию.
type AllocationCallbacks struct {
	PfnAllocation         uintptr
	PfnReallocation       uintptr
	PfnFree               uintptr
	PfnInternalAllocation uintptr
	PfnInternalFree       uintptr
	UserData              unsafe.Pointer
}

// Extent2D / Extent3D / Offset2D / Rect2D — геометрические типы (8 / 12 / 8 / 16 байт).
type (
	Extent2D struct {
		Width  uint32
		Height uint32
	}
	Extent3D struct {
		Width  uint32
		Height uint32
		Depth  uint32
	}
	Offset2D struct {
		X int32
		Y int32
	}
	Rect2D struct {
		Offset Offset2D
		Extent Extent2D
	}
)

// SurfaceFormatKHR — VkSurfaceFormatKHR (8 байт).
type SurfaceFormatKHR struct {
	Format     Format
	ColorSpace ColorSpaceKHR
}

// SurfaceCapabilitiesKHR — VkSurfaceCapabilitiesKHR (56 байт).
type SurfaceCapabilitiesKHR struct {
	MinImageCount           uint32
	MaxImageCount           uint32
	CurrentExtent           Extent2D
	MinImageExtent          Extent2D
	MaxImageExtent          Extent2D
	MaxImageArrayLayers     uint32
	SupportedTransforms     uint32 // VkSurfaceTransformFlagsKHR
	CurrentTransform        uint32 // VkSurfaceTransformFlagBitsKHR
	SupportedCompositeAlpha uint32
	SupportedUsageFlags     uint32
}

// SwapchainCreateInfoKHR — VkSwapchainCreateInfoKHR (104 байта).
type SwapchainCreateInfoKHR struct {
	Type                  StructureType
	Next                  unsafe.Pointer
	Flags                 uint32
	Surface               SurfaceKHR
	MinImageCount         uint32
	ImageFormat           Format
	ImageColorSpace       ColorSpaceKHR
	ImageExtent           Extent2D
	ImageArrayLayers      uint32
	ImageUsage            uint32
	ImageSharingMode      SharingMode
	QueueFamilyIndexCount uint32
	QueueFamilyIndices    *uint32
	PreTransform          uint32
	CompositeAlpha        uint32
	PresentMode           PresentModeKHR
	Clipped               uint32 // VkBool32
	OldSwapchain          SwapchainKHR
}

// ComponentMapping — VkComponentMapping (16 байт).
type ComponentMapping struct {
	R ComponentSwizzle
	G ComponentSwizzle
	B ComponentSwizzle
	A ComponentSwizzle
}

// ImageSubresourceRange — VkImageSubresourceRange (20 байт).
type ImageSubresourceRange struct {
	AspectMask     uint32
	BaseMipLevel   uint32
	LevelCount     uint32
	BaseArrayLayer uint32
	LayerCount     uint32
}

// ImageViewCreateInfo — VkImageViewCreateInfo (80 байт).
type ImageViewCreateInfo struct {
	Type             StructureType
	Next             unsafe.Pointer
	Flags            uint32
	Image            Image
	ViewType         ImageViewType
	Format           Format
	Components       ComponentMapping
	SubresourceRange ImageSubresourceRange
}

// AttachmentDescription — VkAttachmentDescription (36 байт).
type AttachmentDescription struct {
	Flags          uint32
	Format         Format
	Samples        SampleCount
	LoadOp         AttachmentLoadOp
	StoreOp        AttachmentStoreOp
	StencilLoadOp  AttachmentLoadOp
	StencilStoreOp AttachmentStoreOp
	InitialLayout  ImageLayout
	FinalLayout    ImageLayout
}

// AttachmentReference — VkAttachmentReference (8 байт).
type AttachmentReference struct {
	Attachment uint32
	Layout     ImageLayout
}

// SubpassDescription — VkSubpassDescription (72 байта).
type SubpassDescription struct {
	Flags                   uint32
	PipelineBindPoint       PipelineBindPoint
	InputAttachmentCount    uint32
	InputAttachments        *AttachmentReference
	ColorAttachmentCount    uint32
	ColorAttachments        *AttachmentReference
	ResolveAttachments      *AttachmentReference
	DepthStencilAttachment  *AttachmentReference
	PreserveAttachmentCount uint32
	PreserveAttachments     *uint32
}

// SubpassDependency — VkSubpassDependency (28 байт).
type SubpassDependency struct {
	SrcSubpass      uint32
	DstSubpass      uint32
	SrcStageMask    uint32
	DstStageMask    uint32
	SrcAccessMask   uint32
	DstAccessMask   uint32
	DependencyFlags uint32
}

// RenderPassCreateInfo — VkRenderPassCreateInfo (72 байта).
type RenderPassCreateInfo struct {
	Type            StructureType
	Next            unsafe.Pointer
	Flags           uint32
	AttachmentCount uint32
	Attachments     *AttachmentDescription
	SubpassCount    uint32
	Subpasses       *SubpassDescription
	DependencyCount uint32
	Dependencies    *SubpassDependency
}

// FramebufferCreateInfo — VkFramebufferCreateInfo (64 байта).
type FramebufferCreateInfo struct {
	Type            StructureType
	Next            unsafe.Pointer
	Flags           uint32
	RenderPass      RenderPass
	AttachmentCount uint32
	Attachments     *ImageView
	Width           uint32
	Height          uint32
	Layers          uint32
}

// CommandPoolCreateInfo — VkCommandPoolCreateInfo (24 байта).
type CommandPoolCreateInfo struct {
	Type             StructureType
	Next             unsafe.Pointer
	Flags            uint32
	QueueFamilyIndex uint32
}

// CommandBufferAllocateInfo — VkCommandBufferAllocateInfo (32 байта).
type CommandBufferAllocateInfo struct {
	Type               StructureType
	Next               unsafe.Pointer
	CommandPool        CommandPool
	Level              CommandBufferLevel
	CommandBufferCount uint32
}

// CommandBufferBeginInfo — VkCommandBufferBeginInfo (32 байта).
type CommandBufferBeginInfo struct {
	Type            StructureType
	Next            unsafe.Pointer
	Flags           uint32
	InheritanceInfo unsafe.Pointer // *CommandBufferInheritanceInfo; для primary всегда nil
}

// ClearColorValue — VkClearColorValue (16 байт, union).
type ClearColorValue struct {
	Float32 [4]float32
}

// ClearValue — VkClearValue (16 байт, union: color | depthStencil).
type ClearValue struct {
	Color ClearColorValue
}

// ClearAttachment — VkClearAttachment (24 байта).
type ClearAttachment struct {
	AspectMask      uint32
	ColorAttachment uint32
	ClearValue      ClearValue
}

// ClearRect — VkClearRect (24 байта).
type ClearRect struct {
	Rect           Rect2D
	BaseArrayLayer uint32
	LayerCount     uint32
}

// --- Память, буферы, картины (для текстур UI) ---

// MemoryAllocateInfo — VkMemoryAllocateInfo (32 байта).
type MemoryAllocateInfo struct {
	Type            StructureType
	Next            unsafe.Pointer
	AllocationSize  uint64
	MemoryTypeIndex uint32
}

// MemoryRequirements — VkMemoryRequirements (24 байта).
type MemoryRequirements struct {
	Size           uint64
	Alignment      uint64
	MemoryTypeBits uint32
}

// MemoryType / MemoryHeap / PhysicalDeviceMemoryProperties — VkPhysicalDeviceMemoryProperties.
//
// Полная раскладка: массивы фиксированной длины (32 типа, 16 куч), хвост
// не нужен — структура компактная (520 байт), поля важные целиком.
type MemoryType struct {
	PropertyFlags uint32
	HeapIndex     uint32
}

type MemoryHeap struct {
	Size  uint64
	Flags uint32
}

type PhysicalDeviceMemoryProperties struct {
	MemoryTypeCount uint32
	MemoryTypes     [32]MemoryType
	MemoryHeapCount uint32
	MemoryHeaps     [16]MemoryHeap
}

// BufferCreateInfo — VkBufferCreateInfo (56 байт).
type BufferCreateInfo struct {
	Type                  StructureType
	Next                  unsafe.Pointer
	Flags                 uint32
	Size                  uint64
	Usage                 uint32
	SharingMode           SharingMode
	QueueFamilyIndexCount uint32
	QueueFamilyIndices    *uint32
}

// ImageCreateInfo — VkImageCreateInfo (88 байт).
type ImageCreateInfo struct {
	Type                  StructureType
	Next                  unsafe.Pointer
	Flags                 uint32
	ImageType             ImageType
	Format                Format
	Extent                Extent3D
	MipLevels             uint32
	ArrayLayers           uint32
	Samples               SampleCount
	Tiling                ImageTiling
	Usage                 uint32
	SharingMode           SharingMode
	QueueFamilyIndexCount uint32
	QueueFamilyIndices    *uint32
	InitialLayout         ImageLayout
}

// ImageSubresourceLayers — VkImageSubresourceLayers (16 байт).
type ImageSubresourceLayers struct {
	AspectMask     uint32
	MipLevel       uint32
	BaseArrayLayer uint32
	LayerCount     uint32
}

// BufferImageCopy — VkBufferImageCopy (56 байт) — загрузка текстуры из буфера.
type BufferImageCopy struct {
	BufferOffset      uint64
	BufferRowLength   uint32
	BufferImageHeight uint32
	ImageSubresource  ImageSubresourceLayers
	ImageOffset       Offset3D
	ImageExtent       Extent3D
}

// Offset3D — VkOffset3D (12 байт).
type Offset3D struct {
	X, Y, Z int32
}

// ImageMemoryBarrier — VkImageMemoryBarrier (72 байта) — переход layout текстуры.
type ImageMemoryBarrier struct {
	Type                StructureType
	Next                unsafe.Pointer
	SrcAccessMask       uint32
	DstAccessMask       uint32
	OldLayout           ImageLayout
	NewLayout           ImageLayout
	SrcQueueFamilyIndex uint32
	DstQueueFamilyIndex uint32
	Image               Image
	SubresourceRange    ImageSubresourceRange
}

// --- Шейдерный модуль и графический пайплайн ---

// ShaderModuleCreateInfo — VkShaderModuleCreateInfo (40 байт).
type ShaderModuleCreateInfo struct {
	Type     StructureType
	Next     unsafe.Pointer
	Flags    uint32
	CodeSize uintptr // size_t
	Code     *uint32
}

// PipelineShaderStageCreateInfo — VkPipelineShaderStageCreateInfo (48 байт).
type PipelineShaderStageCreateInfo struct {
	Type           StructureType
	Next           unsafe.Pointer
	Flags          uint32
	Stage          ShaderStageFlags
	Module         ShaderModule
	Name           *byte          // «main", NUL-терминированная
	Specialization unsafe.Pointer // *SpecializationInfo, обычно nil
}

// VertexInputBindingDescription — VkVertexInputBindingDescription (12 байт).
type VertexInputBindingDescription struct {
	Binding   uint32
	Stride    uint32
	InputRate VertexInputRate
}

// VertexInputAttributeDescription — VkVertexInputAttributeDescription (16 байт).
type VertexInputAttributeDescription struct {
	Location uint32
	Binding  uint32
	Format   Format
	Offset   uint32
}

// PipelineVertexInputStateCreateInfo — VkPipelineVertexInputStateCreateInfo (48 байт).
type PipelineVertexInputStateCreateInfo struct {
	Type                            StructureType
	Next                            unsafe.Pointer
	Flags                           uint32
	VertexBindingDescriptionCount   uint32
	VertexBindingDescriptions       *VertexInputBindingDescription
	VertexAttributeDescriptionCount uint32
	VertexAttributeDescriptions     *VertexInputAttributeDescription
}

// PipelineInputAssemblyStateCreateInfo — VkPipelineInputAssemblyStateCreateInfo (32 байта).
type PipelineInputAssemblyStateCreateInfo struct {
	Type                   StructureType
	Next                   unsafe.Pointer
	Flags                  uint32
	Topology               PrimitiveTopology
	PrimitiveRestartEnable uint32 // VkBool32
}

// Viewport — VkViewport (24 байта).
type Viewport struct {
	X, Y, Width, Height float32
	MinDepth, MaxDepth  float32
}

// PipelineViewportStateCreateInfo — VkPipelineViewportStateCreateInfo (48 байт).
type PipelineViewportStateCreateInfo struct {
	Type          StructureType
	Next          unsafe.Pointer
	Flags         uint32
	ViewportCount uint32
	Viewports     *Viewport
	ScissorCount  uint32
	Scissors      *Rect2D
}

// PipelineRasterizationStateCreateInfo — VkPipelineRasterizationStateCreateInfo (60 байт).
type PipelineRasterizationStateCreateInfo struct {
	Type                    StructureType
	Next                    unsafe.Pointer
	Flags                   uint32
	DepthClampEnable        uint32
	RasterizerDiscardEnable uint32
	PolygonMode             PolygonMode
	CullMode                CullModeFlags
	FrontFace               FrontFace
	DepthBiasEnable         uint32
	DepthBiasConstantFactor float32
	DepthBiasClamp          float32
	DepthBiasSlopeFactor    float32
	LineWidth               float32
}

// PipelineMultisampleStateCreateInfo — VkPipelineMultisampleStateCreateInfo (48 байт).
type PipelineMultisampleStateCreateInfo struct {
	Type                  StructureType
	Next                  unsafe.Pointer
	Flags                 uint32
	RasterizationSamples  SampleCount
	SampleShadingEnable   uint32
	MinSampleShading      float32
	SampleMask            *uint32
	AlphaToCoverageEnable uint32
	AlphaToOneEnable      uint32
}

// StencilOpState — VkStencilOpState (28 байт).
type StencilOpState struct {
	FailOp      StencilOp
	PassOp      StencilOp
	DepthFailOp StencilOp
	CompareOp   CompareOp
	CompareMask uint32
	WriteMask   uint32
	Reference   uint32
}

// PipelineDepthStencilStateCreateInfo — VkPipelineDepthStencilStateCreateInfo (104 байта).
type PipelineDepthStencilStateCreateInfo struct {
	Type                  StructureType
	Next                  unsafe.Pointer
	Flags                 uint32
	DepthTestEnable       uint32
	DepthWriteEnable      uint32
	DepthCompareOp        CompareOp
	DepthBoundsTestEnable uint32
	StencilTestEnable     uint32
	Front                 StencilOpState
	Back                  StencilOpState
	MinDepthBounds        float32
	MaxDepthBounds        float32
}

// PipelineColorBlendAttachmentState — VkPipelineColorBlendAttachmentState (32 байта).
type PipelineColorBlendAttachmentState struct {
	BlendEnable         uint32
	SrcColorBlendFactor BlendFactor
	DstColorBlendFactor BlendFactor
	ColorBlendOp        BlendOp
	SrcAlphaBlendFactor BlendFactor
	DstAlphaBlendFactor BlendFactor
	AlphaBlendOp        BlendOp
	ColorWriteMask      uint32
}

// PipelineColorBlendStateCreateInfo — VkPipelineColorBlendStateCreateInfo (56 байт).
type PipelineColorBlendStateCreateInfo struct {
	Type            StructureType
	Next            unsafe.Pointer
	Flags           uint32
	LogicOpEnable   uint32
	LogicOp         uint32
	AttachmentCount uint32
	Attachments     *PipelineColorBlendAttachmentState
	BlendConstants  [4]float32
}

// PipelineDynamicStateCreateInfo — VkPipelineDynamicStateCreateInfo (32 байта).
type PipelineDynamicStateCreateInfo struct {
	Type              StructureType
	Next              unsafe.Pointer
	Flags             uint32
	DynamicStateCount uint32
	DynamicStates     *DynamicState
}

// PushConstantRange — VkPushConstantRange (12 байт).
type PushConstantRange struct {
	StageFlags uint32
	Offset     uint32
	Size       uint32
}

// PipelineLayoutCreateInfo — VkPipelineLayoutCreateInfo (48 байт).
type PipelineLayoutCreateInfo struct {
	Type                   StructureType
	Next                   unsafe.Pointer
	Flags                  uint32
	SetLayoutCount         uint32
	SetLayouts             *DescriptorSetLayout
	PushConstantRangeCount uint32
	PushConstantRanges     *PushConstantRange
}

// GraphicsPipelineCreateInfo — VkGraphicsPipelineCreateInfo (144 байта).
type GraphicsPipelineCreateInfo struct {
	Type               StructureType
	Next               unsafe.Pointer
	Flags              uint32
	StageCount         uint32
	Stages             *PipelineShaderStageCreateInfo
	VertexInputState   *PipelineVertexInputStateCreateInfo
	InputAssemblyState *PipelineInputAssemblyStateCreateInfo
	TessellationState  unsafe.Pointer // не используется
	ViewportState      *PipelineViewportStateCreateInfo
	RasterizationState *PipelineRasterizationStateCreateInfo
	MultisampleState   *PipelineMultisampleStateCreateInfo
	DepthStencilState  unsafe.Pointer // без depth-attachment — nil
	ColorBlendState    *PipelineColorBlendStateCreateInfo
	DynamicState       *PipelineDynamicStateCreateInfo
	Layout             PipelineLayout
	RenderPass         RenderPass
	Subpass            uint32
	BasePipelineHandle Pipeline
	BasePipelineIndex  int32
}

// --- Дескрипторы и сэмплер ---

// DescriptorSetLayoutBinding — VkDescriptorSetLayoutBinding (24 байта).
type DescriptorSetLayoutBinding struct {
	Binding           uint32
	DescriptorType    DescriptorType
	DescriptorCount   uint32
	StageFlags        uint32
	ImmutableSamplers *Sampler
}

// DescriptorSetLayoutCreateInfo — VkDescriptorSetLayoutCreateInfo (32 байта).
type DescriptorSetLayoutCreateInfo struct {
	Type         StructureType
	Next         unsafe.Pointer
	Flags        uint32
	BindingCount uint32
	Bindings     *DescriptorSetLayoutBinding
}

// DescriptorPoolSize — VkDescriptorPoolSize (8 байт).
type DescriptorPoolSize struct {
	Type  DescriptorType
	Count uint32
}

// DescriptorPoolCreateInfo — VkDescriptorPoolCreateInfo (40 байт).
type DescriptorPoolCreateInfo struct {
	Type          StructureType
	Next          unsafe.Pointer
	Flags         uint32
	MaxSets       uint32
	PoolSizeCount uint32
	PoolSizes     *DescriptorPoolSize
}

// DescriptorSetAllocateInfo — VkDescriptorSetAllocateInfo (40 байт).
type DescriptorSetAllocateInfo struct {
	Type               StructureType
	Next               unsafe.Pointer
	DescriptorPool     DescriptorPool
	DescriptorSetCount uint32
	SetLayouts         *DescriptorSetLayout
}

// DescriptorImageInfo — VkDescriptorImageInfo (24 байта).
type DescriptorImageInfo struct {
	Sampler     Sampler
	ImageView   ImageView
	ImageLayout ImageLayout
}

// WriteDescriptorSet — VkWriteDescriptorSet (64 байта).
type WriteDescriptorSet struct {
	Type            StructureType
	Next            unsafe.Pointer
	DstSet          DescriptorSet
	DstBinding      uint32
	DstArrayElement uint32
	DescriptorCount uint32
	DescriptorType  DescriptorType
	ImageInfo       *DescriptorImageInfo
	BufferInfo      unsafe.Pointer // не используется
	TexelBufferView unsafe.Pointer // не используется
}

// SamplerCreateInfo — VkSamplerCreateInfo (80 байт).
type SamplerCreateInfo struct {
	Type                    StructureType
	Next                    unsafe.Pointer
	Flags                   uint32
	MagFilter               Filter
	MinFilter               Filter
	MipmapMode              SamplerMipmapMode
	AddressModeU            SamplerAddressMode
	AddressModeV            SamplerAddressMode
	AddressModeW            SamplerAddressMode
	MipLodBias              float32
	AnisotropyEnable        uint32
	MaxAnisotropy           float32
	CompareEnable           uint32
	CompareOp               CompareOp
	MinLod                  float32
	MaxLod                  float32
	BorderColor             BorderColor
	UnnormalizedCoordinates uint32
}

// RenderPassBeginInfo — VkRenderPassBeginInfo (64 байта).
type RenderPassBeginInfo struct {
	Type            StructureType
	Next            unsafe.Pointer
	RenderPass      RenderPass
	Framebuffer     Framebuffer
	RenderArea      Rect2D
	ClearValueCount uint32
	ClearValues     *ClearValue
}

// SemaphoreCreateInfo — VkSemaphoreCreateInfo (16 байт).
type SemaphoreCreateInfo struct {
	Type  StructureType
	Next  unsafe.Pointer
	Flags uint32
}

// FenceCreateInfo — VkFenceCreateInfo (16 байт).
type FenceCreateInfo struct {
	Type  StructureType
	Next  unsafe.Pointer
	Flags uint32
}

// SubmitInfo — VkSubmitInfo (72 байта).
type SubmitInfo struct {
	Type                 StructureType
	Next                 unsafe.Pointer
	WaitSemaphoreCount   uint32
	WaitSemaphores       *Semaphore
	WaitDstStageMask     *uint32
	CommandBufferCount   uint32
	CommandBuffers       *CommandBuffer
	SignalSemaphoreCount uint32
	SignalSemaphores     *Semaphore
}

// PresentInfoKHR — VkPresentInfoKHR (64 байта).
type PresentInfoKHR struct {
	Type               StructureType
	Next               unsafe.Pointer
	WaitSemaphoreCount uint32
	WaitSemaphores     *Semaphore
	SwapchainCount     uint32
	Swapchains         *SwapchainKHR
	ImageIndices       *uint32
	Results            *Result
}

// ExtensionProperties — VkExtensionProperties (260 байт).
type ExtensionProperties struct {
	ExtensionName [256]byte
	SpecVersion   uint32
}

// LayerProperties — VkLayerProperties (520 байт).
type LayerProperties struct {
	LayerName             [256]byte
	SpecVersion           uint32
	ImplementationVersion uint32
	Description           [256]byte
}

// QueueFamilyProperties — VkQueueFamilyProperties (24 байта).
type QueueFamilyProperties struct {
	QueueFlags                  uint32
	QueueCount                  uint32
	TimestampValidBits          uint32
	MinImageTransferGranularity Extent3D
}

// PhysicalDeviceProperties — VkPhysicalDeviceProperties.
//
// Поля limits и sparseProperties нам не нужны, но Vulkan пишет в структуру
// целиком, поэтому хвост закрыт заведомо большим буфером: это гарантирует, что
// драйвер не запишет за пределы Go-структуры. Реальный размер структуры
// (~1.5 КБ) буфер перекрывает с большим запасом; имя устройства, версии и тип
// читаются из началa, где раскладка известна точно (см. layout_test.go).
type PhysicalDeviceProperties struct {
	APIVersion        uint32             // 0
	DriverVersion     uint32             // 4
	VendorID          uint32             // 8
	DeviceID          uint32             // 12
	DeviceType        PhysicalDeviceType // 16
	DeviceName        [256]byte          // 20
	PipelineCacheUUID [16]byte           // 276
	// Выравнивание 8: VkPhysicalDeviceLimits в C начинается с 296-го байта.
	LimitsAlign uint32
	// VkPhysicalDeviceLimits + VkPhysicalDeviceSparseProperties целиком:
	// буфер заведомо больше реального хвоста структуры.
	rest [4096]byte
}

// Name возвращает имя устройства из DeviceName (нуль-терминированная строка).
func (p *PhysicalDeviceProperties) Name() string {
	return cstrToString(p.DeviceName[:])
}

// DebugUtilsMessengerCreateInfoEXT — VkDebugUtilsMessengerCreateInfoEXT (48 байт).
type DebugUtilsMessengerCreateInfoEXT struct {
	Type            StructureType
	Next            unsafe.Pointer
	Flags           uint32
	MessageSeverity uint32
	MessageType     uint32
	UserCallback    uintptr // PFN_vkDebugUtilsMessengerCallbackEXT
	UserData        unsafe.Pointer
}

// debugUtilsCallbackDataEXT — VkDebugUtilsMessengerCallbackDataEXT (96 байт).
// Внутренний тип: из него читается только pMessage.
type debugUtilsCallbackDataEXT struct {
	Type             StructureType
	Next             unsafe.Pointer
	Flags            uint32
	MessageIDName    *byte
	MessageIDNumber  int32
	Message          *byte
	QueueLabelCount  uint32
	QueueLabels      unsafe.Pointer
	CmdBufLabelCount uint32
	CmdBufLabels     unsafe.Pointer
	ObjectCount      uint32
	Objects          unsafe.Pointer
}

// Платформенные структуры поверхностей. Лежат в пакете vulkan без тегов
// сборки: раскладка одинаковая на всех ОС (только указатели), а вызывать
// соответствующую функцию — решение слоя window.
//
// Win32SurfaceCreateInfoKHR — 40 байт.
type Win32SurfaceCreateInfoKHR struct {
	Type      StructureType
	Next      unsafe.Pointer
	Flags     uint32
	HInstance uintptr // HINSTANCE
	Hwnd      uintptr // HWND
}

// XlibSurfaceCreateInfoKHR — 40 байт.
type XlibSurfaceCreateInfoKHR struct {
	Type   StructureType
	Next   unsafe.Pointer
	Flags  uint32
	Dpy    uintptr // Display*
	Window uintptr // Window (XID, unsigned long)
}

// WaylandSurfaceCreateInfoKHR — 40 байт.
type WaylandSurfaceCreateInfoKHR struct {
	Type    StructureType
	Next    unsafe.Pointer
	Flags   uint32
	Display uintptr // struct wl_display*
	Surface uintptr // struct wl_surface*
}
