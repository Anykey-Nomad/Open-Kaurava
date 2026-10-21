package vulkan

// Хэндлы Vulkan.
//
// Dispatchable-хэндлы (Instance, Device, ...) — указатели, non-dispatchable
// (SurfaceKHR, SwapchainKHR, ...) — на 64-битных платформах uint64. uintptr
// покрывает оба случая и на amd64, и на arm64 (Windows и Linux).
type (
	Instance               uintptr
	PhysicalDevice         uintptr
	Device                 uintptr
	Queue                  uintptr
	CommandBuffer          uintptr
	SurfaceKHR             uintptr
	SwapchainKHR           uintptr
	Image                  uintptr
	ImageView              uintptr
	RenderPass             uintptr
	Framebuffer            uintptr
	CommandPool            uintptr
	Semaphore              uintptr
	Fence                  uintptr
	DebugUtilsMessengerEXT uintptr

	// Ресурсы графического пайплайна (текст/UI).
	DeviceMemory        uintptr
	Buffer              uintptr
	ShaderModule        uintptr
	Pipeline            uintptr
	PipelineLayout      uintptr
	DescriptorSetLayout uintptr
	DescriptorPool      uintptr
	DescriptorSet       uintptr
	Sampler             uintptr
)

// StructureType — VkStructureType. Значения взяты из vulkan_core.h.
type StructureType int32

const (
	StructureTypeApplicationInfo                  StructureType = 0
	StructureTypeInstanceCreateInfo               StructureType = 1
	StructureTypeDeviceQueueCreateInfo            StructureType = 2
	StructureTypeDeviceCreateInfo                 StructureType = 3
	StructureTypeSubmitInfo                       StructureType = 4
	StructureTypeFenceCreateInfo                  StructureType = 8
	StructureTypeSemaphoreCreateInfo              StructureType = 9
	StructureTypeImageViewCreateInfo              StructureType = 15
	StructureTypeFramebufferCreateInfo            StructureType = 37
	StructureTypeRenderPassCreateInfo             StructureType = 38
	StructureTypeCommandPoolCreateInfo            StructureType = 39
	StructureTypeCommandBufferAllocateInfo        StructureType = 40
	StructureTypeCommandBufferBeginInfo           StructureType = 42
	StructureTypeRenderPassBeginInfo              StructureType = 43
	StructureTypeSwapchainCreateInfoKHR           StructureType = 1000001000
	StructureTypePresentInfoKHR                   StructureType = 1000001001
	StructureTypeXlibSurfaceCreateInfoKHR         StructureType = 1000004000
	StructureTypeWaylandSurfaceCreateInfoKHR      StructureType = 1000006000
	StructureTypeWin32SurfaceCreateInfoKHR        StructureType = 1000009000
	StructureTypeDebugUtilsMessengerCreateInfoEXT StructureType = 1000128004

	// Пайплайн и ресурсы.
	StructureTypeMemoryAllocateInfo                   StructureType = 5
	StructureTypeBufferCreateInfo                     StructureType = 12
	StructureTypeImageCreateInfo                      StructureType = 14
	StructureTypeShaderModuleCreateInfo               StructureType = 16
	StructureTypePipelineShaderStageCreateInfo        StructureType = 18
	StructureTypePipelineVertexInputStateCreateInfo   StructureType = 19
	StructureTypePipelineInputAssemblyStateCreateInfo StructureType = 20
	StructureTypePipelineViewportStateCreateInfo      StructureType = 22
	StructureTypePipelineRasterizationStateCreateInfo StructureType = 23
	StructureTypePipelineMultisampleStateCreateInfo   StructureType = 24
	StructureTypePipelineDepthStencilStateCreateInfo  StructureType = 25
	StructureTypePipelineColorBlendStateCreateInfo    StructureType = 26
	StructureTypePipelineDynamicStateCreateInfo       StructureType = 27
	StructureTypeGraphicsPipelineCreateInfo           StructureType = 28
	StructureTypePipelineLayoutCreateInfo             StructureType = 30
	StructureTypeSamplerCreateInfo                    StructureType = 31
	StructureTypeDescriptorSetLayoutCreateInfo        StructureType = 32
	StructureTypeDescriptorPoolCreateInfo             StructureType = 33
	StructureTypeDescriptorSetAllocateInfo            StructureType = 34
	StructureTypeWriteDescriptorSet                   StructureType = 35
	StructureTypeImageMemoryBarrier                   StructureType = 45
)

// Result — VkResult.
type Result int32

const (
	ResultSuccess                     Result = 0
	ResultNotReady                    Result = 1
	ResultTimeout                     Result = 2
	ResultIncomplete                  Result = 5
	ResultErrorOutOfHostMemory        Result = -1
	ResultErrorOutOfDeviceMemory      Result = -2
	ResultErrorInitializationFailed   Result = -3
	ResultErrorDeviceLost             Result = -4
	ResultErrorMemoryMapFailed        Result = -5
	ResultErrorLayerNotPresent        Result = -6
	ResultErrorExtensionNotPresent    Result = -7
	ResultErrorFeatureNotPresent      Result = -8
	ResultErrorIncompatibleDriver     Result = -9
	ResultErrorTooManyObjects         Result = -10
	ResultErrorFormatNotSupported     Result = -11
	ResultErrorUnknown                Result = -13
	ResultErrorOutOfPoolMemory        Result = -1000069000
	ResultErrorFragmentation          Result = -1000161000
	ResultErrorSurfaceLostKHR         Result = -1000000000
	ResultErrorNativeWindowInUseKHR   Result = -1000000001
	ResultErrorOutOfDateKHR           Result = -1000001004
	ResultErrorIncompatibleDisplayKHR Result = -1000003001
	ResultErrorValidationFailedEXT    Result = -1000011001
	ResultSuboptimalKHR               Result = 1000001003
)

// Format — VkFormat (используемые значения).
type Format int32

const (
	FormatUndefined        Format = 0
	FormatR8UNorm          Format = 9
	FormatR8G8B8A8UNORM    Format = 37
	FormatR8G8B8A8SRGB     Format = 43
	FormatB8G8R8A8UNORM    Format = 44
	FormatB8G8R8A8SRGB     Format = 50
	FormatR32G32SFloat     Format = 103
	FormatR32G32B32SFloat  Format = 106
	FormatR32G32B32ASFloat Format = 109
	FormatD32SFloat        Format = 126
)

// ColorSpaceKHR — VkColorSpaceKHR.
type ColorSpaceKHR int32

const ColorSpaceSRGBNonlinearKHR ColorSpaceKHR = 0

// PresentModeKHR — VkPresentModeKHR.
type PresentModeKHR int32

const (
	PresentModeImmediateKHR PresentModeKHR = 0
	PresentModeMailboxKHR   PresentModeKHR = 1
	PresentModeFIFOKHR      PresentModeKHR = 2
)

// ImageLayout — VkImageLayout.
type ImageLayout int32

const (
	ImageLayoutUndefined                        ImageLayout = 0
	ImageLayoutColorAttachmentOptimal           ImageLayout = 2
	ImageLayoutDepthStencilAttachmentOptimal    ImageLayout = 3
	ImageLayoutShaderReadOnlyOptimal            ImageLayout = 5
	ImageLayoutTransferDstOptimal               ImageLayout = 7
	ImageLayoutPresentSrcKHR                    ImageLayout = 1000001002
)

// SharingMode — VkSharingMode.
type SharingMode int32

const (
	SharingModeExclusive  SharingMode = 0
	SharingModeConcurrent SharingMode = 1
)

// AttachmentLoadOp / AttachmentStoreOp — VkAttachmentLoadOp / VkAttachmentStoreOp.
type (
	AttachmentLoadOp  int32
	AttachmentStoreOp int32
)

const (
	AttachmentLoadOpClear     AttachmentLoadOp  = 1
	AttachmentLoadOpDontCare  AttachmentLoadOp  = 2
	AttachmentStoreOpStore    AttachmentStoreOp = 0
	AttachmentStoreOpDontCare AttachmentStoreOp = 1
)

// PipelineBindPoint — VkPipelineBindPoint.
type PipelineBindPoint int32

const PipelineBindPointGraphics PipelineBindPoint = 0

// CommandBufferLevel — VkCommandBufferLevel.
type CommandBufferLevel int32

const CommandBufferLevelPrimary CommandBufferLevel = 0

// SubpassContents — VkSubpassContents.
type SubpassContents int32

const SubpassContentsInline SubpassContents = 0

// ImageViewType — VkImageViewType.
type ImageViewType int32

const ImageViewType2D ImageViewType = 1

// ComponentSwizzle — VkComponentSwizzle.
type ComponentSwizzle int32

const ComponentSwizzleIdentity ComponentSwizzle = 0

// SampleCount — VkSampleCountFlagBits.
type SampleCount int32

const SampleCount1Bit SampleCount = 1

// Пайплайн UI: перечисления (VkDescriptorType, VkPrimitiveTopology и т.д.).
type (
	DescriptorType     int32
	ShaderStageFlags   uint32
	VertexInputRate    int32
	PrimitiveTopology  int32
	PolygonMode        int32
	CullModeFlags      int32
	FrontFace          int32
	BlendFactor        int32
	BlendOp            int32
	DynamicState       int32
	Filter             int32
	SamplerMipmapMode  int32
	SamplerAddressMode int32
	CompareOp          int32
	StencilOp          int32
	IndexType          int32
	BorderColor        int32
	ImageType          int32
	ImageTiling        int32
)

const (
	DescriptorTypeCombinedImageSampler DescriptorType = 1
)

const (
	VertexInputRateVertex         VertexInputRate   = 0
	PrimitiveTopologyTriangleList PrimitiveTopology = 3
	PolygonModeFill               PolygonMode       = 0
	CullModeNone                  CullModeFlags     = 0
	CullModeFrontBit              CullModeFlags     = 1
	CullModeBackBit               CullModeFlags     = 2
	FrontFaceCounterClockwise     FrontFace         = 0
	FrontFaceClockwise            FrontFace         = 1
)

const (
	BlendFactorZero             BlendFactor = 0
	BlendFactorOne              BlendFactor = 1
	BlendFactorSrcAlpha         BlendFactor = 6
	BlendFactorOneMinusSrcAlpha BlendFactor = 7
	BlendOpAdd                  BlendOp     = 0
)

const (
	DynamicStateViewport DynamicState = 0
	DynamicStateScissor  DynamicState = 1
)

const (
	FilterNearest                    Filter             = 0
	FilterLinear                     Filter             = 1
	SamplerMipmapModeLinear          SamplerMipmapMode  = 1
	SamplerAddressModeRepeat         SamplerAddressMode = 0
	SamplerAddressModeClampToEdge    SamplerAddressMode = 2
	CompareOpLess                    CompareOp          = 1
	CompareOpLessOrEqual             CompareOp          = 3
	CompareOpAlways                  CompareOp          = 7
	StencilOpKeep                    StencilOp          = 0
	IndexTypeUint16                  IndexType          = 0
	IndexTypeUint32                  IndexType          = 1
	BorderColorFloatTransparentBlack BorderColor        = 0
)

const (
	ImageType2D        ImageType   = 1
	ImageTilingOptimal ImageTiling = 0
)

// PhysicalDeviceType — VkPhysicalDeviceType.
type PhysicalDeviceType int32

const (
	PhysicalDeviceTypeOther         PhysicalDeviceType = 0
	PhysicalDeviceTypeIntegratedGPU PhysicalDeviceType = 1
	PhysicalDeviceTypeDiscreteGPU   PhysicalDeviceType = 2
	PhysicalDeviceTypeVirtualGPU    PhysicalDeviceType = 3
	PhysicalDeviceTypeCPU           PhysicalDeviceType = 4
)

// Флаги (VkFlags = uint32).
const (
	QueueGraphicsBit uint32 = 0x00000001
	QueueComputeBit  uint32 = 0x00000002
	QueueTransferBit uint32 = 0x00000004

	ImageUsageColorAttachmentBit        uint32 = 0x00000010
	ImageUsageDepthStencilAttachmentBit uint32 = 0x00000020
	ImageUsageTransferDstBit            uint32 = 0x00000002
	ImageUsageSampledBit                uint32 = 0x00000004

	ImageAspectColorBit uint32 = 0x00000001
	ImageAspectDepthBit uint32 = 0x00000002

	PipelineStageTopOfPipeBit             uint32 = 0x00000001
	PipelineStageVertexInputBit           uint32 = 0x00000004
	PipelineStageVertexShaderBit          uint32 = 0x00000008
	PipelineStageEarlyFragmentTestsBit    uint32 = 0x00000100
	PipelineStageLateFragmentTestsBit     uint32 = 0x00000200
	PipelineStageColorAttachmentOutputBit uint32 = 0x00000400
	PipelineStageTransferBit              uint32 = 0x00001000
	PipelineStageFragmentShaderBit        uint32 = 0x00000080
	PipelineStageHostBit                  uint32 = 0x00004000
	PipelineStageAllCommandsBit           uint32 = 0x00010000

	AccessColorAttachmentReadBit         uint32 = 0x00000080
	AccessColorAttachmentWriteBit        uint32 = 0x00000100
	AccessDepthStencilAttachmentReadBit  uint32 = 0x00000200
	AccessDepthStencilAttachmentWriteBit uint32 = 0x00000400
	AccessMemoryReadBit                  uint32 = 0x00008000
	AccessShaderReadBit           uint32 = 0x00000010
	AccessTransferReadBit         uint32 = 0x00000800
	AccessTransferWriteBit        uint32 = 0x00001000
	AccessHostWriteBit            uint32 = 0x00002000

	CommandPoolCreateTransientBit          uint32 = 0x00000001
	CommandPoolCreateResetCommandBufferBit uint32 = 0x00000002

	CommandBufferUsageOneTimeSubmitBit uint32 = 0x00000001

	FenceCreateSignaledBit uint32 = 0x00000001

	SurfaceTransformIdentityBitKHR uint32 = 0x00000001

	CompositeAlphaOpaqueBitKHR         uint32 = 0x00000001
	CompositeAlphaPreMultipliedBitKHR  uint32 = 0x00000002
	CompositeAlphaPostMultipliedBitKHR uint32 = 0x00000004
	CompositeAlphaInheritBitKHR        uint32 = 0x00000008

	BufferUsageTransferSrcBit  uint32 = 0x00000001
	BufferUsageTransferDstBit  uint32 = 0x00000002
	BufferUsageIndexBufferBit  uint32 = 0x00000040
	BufferUsageVertexBufferBit uint32 = 0x00000080

	MemoryPropertyDeviceLocalBit  uint32 = 0x00000001
	MemoryPropertyHostVisibleBit  uint32 = 0x00000002
	MemoryPropertyHostCoherentBit uint32 = 0x00000004

	ShaderStageVertexBit   uint32 = 0x00000001
	ShaderStageFragmentBit uint32 = 0x00000010

	// Цветовые компоненты (VkColorComponentFlagBits).
	ColorComponentRBit uint32 = 0x00000001
	ColorComponentGBit uint32 = 0x00000002
	ColorComponentBBit uint32 = 0x00000004
	ColorComponentABit uint32 = 0x00000008

	QueueFamilyIgnored uint32 = 0xFFFFFFFF

	DebugUtilsMessageSeverityVerboseBitEXT uint32 = 0x00000001
	DebugUtilsMessageSeverityInfoBitEXT    uint32 = 0x00000010
	DebugUtilsMessageSeverityWarningBitEXT uint32 = 0x00000100
	DebugUtilsMessageSeverityErrorBitEXT   uint32 = 0x00001000
	DebugUtilsMessageTypeGeneralBitEXT     uint32 = 0x00000001
	DebugUtilsMessageTypeValidationBitEXT  uint32 = 0x00000002
	DebugUtilsMessageTypePerformanceBitEXT uint32 = 0x00000004
)

// Имена расширений и слоёв.
const (
	ExtSurface        = "VK_KHR_surface"
	ExtSwapchain      = "VK_KHR_swapchain"
	ExtWin32Surface   = "VK_KHR_win32_surface"
	ExtXlibSurface    = "VK_KHR_xlib_surface"
	ExtWaylandSurface = "VK_KHR_wayland_surface"
	ExtDebugUtils     = "VK_EXT_debug_utils"

	LayerKhronosValidation = "VK_LAYER_KHRONOS_validation"
)

// VK_API_VERSION_1_0 — версия API, которую запрашиваем при создании instance.
const APIVersion1_0 uint32 = (1 << 22) | (0 << 12)

// APIVersion1_1/1_2/1_3 для запроса более новых версий.
const (
	APIVersion1_1 uint32 = (1 << 22) | (1 << 12)
	APIVersion1_2 uint32 = (1 << 22) | (2 << 12)
	APIVersion1_3 uint32 = (1 << 22) | (3 << 12)
)
