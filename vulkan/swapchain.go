package vulkan

import (
	"fmt"
	"runtime"
)

// SwapchainOptions — параметры создания цепочки кадров.
type SwapchainOptions struct {
	// Width/Height — желаемый размер в пикселях (из окна).
	// Фактический extent берётся из возможностей surface и подрезается под него.
	Width, Height int
	// PreferredFormat — предпочтительный формат; FormatUndefined = первый доступный.
	PreferredFormat Format
	// PreferredPresentMode — предпочтительный режим; nil = FIFO (гарантирован spec'ом).
	PreferredPresentMode *PresentModeKHR
	// FramesInFlight — количество кадров в полёте (>=1); влияет только на
	// рекомендуемое число изображений (minImageCount + FramesInFlight).
	FramesInFlight int
	// GraphicsFamily / PresentFamily — семейства очередей.
	GraphicsFamily uint32
	PresentFamily  uint32
	// PhysicalDevice — физическое устройство, к которому относится surface.
	// Нужно для запросов возможностей поверхности внутри NewSwapchain/Recreate.
	PhysicalDevice PhysicalDevice
}

// Swapchain — цепочка кадров вместе со всем, что нужно для отрисовки в неё:
// изображения, image views, render pass (clear → present) и фреймбуферы.
//
// Владелец ресурсов: вызывающий. Destroy() освобождает всё разом.
type Swapchain struct {
	Handle       SwapchainKHR
	Images       []Image
	Views        []ImageView
	Format       Format
	ColorSpace   ColorSpaceKHR
	Extent       Extent2D
	RenderPass   RenderPass
	Framebuffers []Framebuffer
	DepthImage   Image
	DepthMemory  DeviceMemory
	DepthView    ImageView
	// RenderFinished — семафор ожидания present'а, по одному на изображение.
	RenderFinished []Semaphore

	api     *API
	device  Device
	surface SurfaceKHR
	pd      PhysicalDevice
	opts    SwapchainOptions
	present PresentModeKHR
}

// NewSwapchain создаёт swapchain и всю связанную с ним графику.
func NewSwapchain(api *API, device Device, surface SurfaceKHR, opts SwapchainOptions) (*Swapchain, error) {
	if api.vkCreateSwapchainKHR == nil {
		return nil, absent("vkCreateSwapchainKHR", ExtSwapchain)
	}
	if opts.FramesInFlight <= 0 {
		opts.FramesInFlight = 2
	}
	s := &Swapchain{
		api: api, device: device, surface: surface, opts: opts,
		pd: opts.PhysicalDevice,
	}
	if err := s.create(0); err != nil {
		return nil, err
	}
	return s, nil
}

// create (пере)создаёт swapchain и связанные ресурсы.
// old — предыдущий swapchain (0 при первом создании); после успешного
// пересоздания он уничтожается вызывающим кодом.
func (s *Swapchain) create(old SwapchainKHR) error {
	api := s.api
	pd, err := s.physicalDevice()
	if err != nil {
		return err
	}

	caps, err := api.GetPhysicalDeviceSurfaceCapabilities(pd, s.surface)
	if err != nil {
		return err
	}
	formats, err := api.GetPhysicalDeviceSurfaceFormats(pd, s.surface)
	if err != nil {
		return err
	}
	modes, err := api.GetPhysicalDeviceSurfacePresentModes(pd, s.surface)
	if err != nil {
		return err
	}

	format := pickFormat(formats, s.opts.PreferredFormat)
	presentMode := pickPresentMode(modes, s.opts)
	extent := pickExtent(caps, s.opts.Width, s.opts.Height)

	imageCount := caps.MinImageCount + uint32(s.opts.FramesInFlight-1)
	if imageCount < caps.MinImageCount+1 {
		imageCount = caps.MinImageCount + 1
	}
	if caps.MaxImageCount != 0 && imageCount > caps.MaxImageCount {
		imageCount = caps.MaxImageCount
	}

	// Композитная альфа: предпочитаем OPAQUE, иначе первый доступный бит.
	compositeAlpha := uint32(CompositeAlphaOpaqueBitKHR)
	if caps.SupportedCompositeAlpha&compositeAlpha == 0 {
		compositeAlpha = 1
		for compositeAlpha != 0 && caps.SupportedCompositeAlpha&compositeAlpha == 0 {
			compositeAlpha <<= 1
		}
	}

	usage := uint32(ImageUsageColorAttachmentBit)
	if caps.SupportedUsageFlags&usage == 0 {
		return fmt.Errorf("vulkan: поверхность не поддерживает VK_IMAGE_USAGE_COLOR_ATTACHMENT_BIT")
	}

	// Форматы и режимы представлены не пустыми — проверено в ChoosePhysicalDevice.

	info := SwapchainCreateInfoKHR{
		Type:             StructureTypeSwapchainCreateInfoKHR,
		Surface:          s.surface,
		MinImageCount:    imageCount,
		ImageFormat:      format.Format,
		ImageColorSpace:  format.ColorSpace,
		ImageExtent:      extent,
		ImageArrayLayers: 1,
		ImageUsage:       usage,
		PreTransform:     caps.CurrentTransform,
		CompositeAlpha:   compositeAlpha,
		PresentMode:      presentMode,
		Clipped:          1,
		OldSwapchain:     old,
	}

	var indices []uint32
	if s.opts.GraphicsFamily != s.opts.PresentFamily {
		indices = []uint32{s.opts.GraphicsFamily, s.opts.PresentFamily}
		info.ImageSharingMode = SharingModeConcurrent
		info.QueueFamilyIndexCount = uint32(len(indices))
		info.QueueFamilyIndices = &indices[0]
	} else {
		info.ImageSharingMode = SharingModeExclusive
	}

	var handle SwapchainKHR
	if err := check("vkCreateSwapchainKHR", api.vkCreateSwapchainKHR(s.device, &info, nil, &handle)); err != nil {
		runtime.KeepAlive(indices)
		return err
	}
	runtime.KeepAlive(indices)

	s.Handle = handle
	s.Format = format.Format
	s.ColorSpace = format.ColorSpace
	s.Extent = extent
	s.present = presentMode
	s.opts.Width = int(extent.Width)
	s.opts.Height = int(extent.Height)

	if err := s.loadImages(); err != nil {
		return err
	}
	if err := s.createRenderFinished(); err != nil {
		return err
	}
	if err := s.createImageViews(); err != nil {
		return err
	}
	if err := s.createDepthBuffer(); err != nil {
		return err
	}
	if err := s.createRenderPass(); err != nil {
		return err
	}
	if err := s.createFramebuffers(); err != nil {
		return err
	}
	return nil
}

// Recreate пересоздаёт swapchain под новый размер окна.
// Вызывать после vkDeviceWaitIdle.
func (s *Swapchain) Recreate(width, height int) error {
	old := s.Handle
	s.opts.Width, s.opts.Height = width, height
	s.destroyResources()
	if err := s.create(old); err != nil {
		// старый swapchain уже не используется — освобождаем, чтобы не течь
		s.api.vkDestroySwapchainKHR(s.device, old, nil)
		s.Handle = 0
		return err
	}
	s.api.vkDestroySwapchainKHR(s.device, old, nil)
	return nil
}

// Extent возвращает текущий размер изображений в пикселях.
func (s *Swapchain) Extent2D() Extent2D { return s.Extent }

// PresentMode возвращает выбранный режим представления (FIFO, MAILBOX, ...).
func (s *Swapchain) PresentMode() PresentModeKHR { return s.present }

// physicalDevice возвращает физическое устройство, которому принадлежит surface.
// Определяется по одному из семейств очередей.
func (s *Swapchain) physicalDevice() (PhysicalDevice, error) {
	// API хранит выбранное устройство неявно: surface уже привязан к своему
	// физическому устройству, поэтому достаточно того, которое создавал swapchain.
	// Для базового слоя запоминаем pd в API при выборе — см. SetPhysicalDevice.
	if s.pd == 0 {
		return 0, fmt.Errorf("vulkan: не задано физическое устройство для swapchain (вызовите api.SetPhysicalDevice)")
	}
	return s.pd, nil
}

// SetPhysicalDevice задаёт физическое устройство после создания
// (если оно не было указано в SwapchainOptions.PhysicalDevice).
func (s *Swapchain) SetPhysicalDevice(pd PhysicalDevice) { s.pd = pd }

func (s *Swapchain) loadImages() error {
	var count uint32
	if err := check("vkGetSwapchainImagesKHR",
		s.api.vkGetSwapchainImagesKHR(s.device, s.Handle, &count, nil)); err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("vkGetSwapchainImagesKHR: swapchain не содержит изображений")
	}
	images := make([]Image, count)
	if err := check("vkGetSwapchainImagesKHR",
		s.api.vkGetSwapchainImagesKHR(s.device, s.Handle, &count, &images[0])); err != nil {
		return err
	}
	s.Images = images[:count]
	return nil
}

// createRenderFinished создаёт семафоры present'а — по одному на изображение.
func (s *Swapchain) createRenderFinished() error {
	s.RenderFinished = make([]Semaphore, len(s.Images))
	info := SemaphoreCreateInfo{Type: StructureTypeSemaphoreCreateInfo}
	for i := range s.RenderFinished {
		if err := check("vkCreateSemaphore",
			s.api.vkCreateSemaphore(s.device, &info, nil, &s.RenderFinished[i])); err != nil {
			return err
		}
	}
	return nil
}

func (s *Swapchain) createImageViews() error {
	s.Views = make([]ImageView, len(s.Images))
	for i, img := range s.Images {
		info := ImageViewCreateInfo{
			Type:     StructureTypeImageViewCreateInfo,
			Image:    img,
			ViewType: ImageViewType2D,
			Format:   s.Format,
			Components: ComponentMapping{
				R: ComponentSwizzleIdentity,
				G: ComponentSwizzleIdentity,
				B: ComponentSwizzleIdentity,
				A: ComponentSwizzleIdentity,
			},
			SubresourceRange: ImageSubresourceRange{
				AspectMask:     ImageAspectColorBit,
				BaseMipLevel:   0,
				LevelCount:     1,
				BaseArrayLayer: 0,
				LayerCount:     1,
			},
		}
		var view ImageView
		if err := check("vkCreateImageView", s.api.vkCreateImageView(s.device, &info, nil, &view)); err != nil {
			return err
		}
		s.Views[i] = view
	}
	return nil
}

func (s *Swapchain) createDepthBuffer() error {
	img, mem, err := s.api.CreateImage2D(s.device, FormatD32SFloat,
		int(s.Extent.Width), int(s.Extent.Height), ImageUsageDepthStencilAttachmentBit)
	if err != nil {
		return err
	}
	view, err := s.api.CreateImageView(s.device, img, FormatD32SFloat, ImageAspectDepthBit)
	if err != nil {
		s.api.DestroyImage(s.device, img, mem)
		return err
	}
	s.DepthImage = img
	s.DepthMemory = mem
	s.DepthView = view
	return nil
}

// createRenderPass делает один универсальный pass: очистка цвета и глубины → кадр → present.
func (s *Swapchain) createRenderPass() error {
	attachments := [2]AttachmentDescription{
		{
			Format:         s.Format,
			Samples:        SampleCount1Bit,
			LoadOp:         AttachmentLoadOpClear,
			StoreOp:        AttachmentStoreOpStore,
			StencilLoadOp:  AttachmentLoadOpDontCare,
			StencilStoreOp: AttachmentStoreOpDontCare,
			InitialLayout:  ImageLayoutUndefined,
			FinalLayout:    ImageLayoutPresentSrcKHR,
		},
		{
			Format:         FormatD32SFloat,
			Samples:        SampleCount1Bit,
			LoadOp:         AttachmentLoadOpClear,
			StoreOp:        AttachmentStoreOpDontCare,
			StencilLoadOp:  AttachmentLoadOpDontCare,
			StencilStoreOp: AttachmentStoreOpDontCare,
			InitialLayout:  ImageLayoutUndefined,
			FinalLayout:    ImageLayoutDepthStencilAttachmentOptimal,
		},
	}
	colorRef := AttachmentReference{Attachment: 0, Layout: ImageLayoutColorAttachmentOptimal}
	depthRef := AttachmentReference{Attachment: 1, Layout: ImageLayoutDepthStencilAttachmentOptimal}
	subpass := SubpassDescription{
		PipelineBindPoint:      PipelineBindPointGraphics,
		ColorAttachmentCount:   1,
		ColorAttachments:       &colorRef,
		DepthStencilAttachment: &depthRef,
	}
	// Стандартная внешняя зависимость: ждём семафор acquire, пишем цвет и глубину.
	dependency := SubpassDependency{
		SrcSubpass:      ^uint32(0),
		DstSubpass:      0,
		SrcStageMask:    PipelineStageColorAttachmentOutputBit | PipelineStageEarlyFragmentTestsBit,
		DstStageMask:    PipelineStageColorAttachmentOutputBit | PipelineStageEarlyFragmentTestsBit,
		SrcAccessMask:   0,
		DstAccessMask:   AccessColorAttachmentWriteBit | AccessDepthStencilAttachmentWriteBit,
		DependencyFlags: 0,
	}
	info := RenderPassCreateInfo{
		Type:            StructureTypeRenderPassCreateInfo,
		AttachmentCount: uint32(len(attachments)),
		Attachments:     &attachments[0],
		SubpassCount:    1,
		Subpasses:       &subpass,
		DependencyCount: 1,
		Dependencies:    &dependency,
	}
	var rp RenderPass
	if err := check("vkCreateRenderPass", s.api.vkCreateRenderPass(s.device, &info, nil, &rp)); err != nil {
		return err
	}
	s.RenderPass = rp
	return nil
}

func (s *Swapchain) createFramebuffers() error {
	s.Framebuffers = make([]Framebuffer, len(s.Views))
	for i := range s.Views {
		atts := [2]ImageView{s.Views[i], s.DepthView}
		info := FramebufferCreateInfo{
			Type:            StructureTypeFramebufferCreateInfo,
			RenderPass:      s.RenderPass,
			AttachmentCount: uint32(len(atts)),
			Attachments:     &atts[0],
			Width:           s.Extent.Width,
			Height:          s.Extent.Height,
			Layers:          1,
		}
		var fb Framebuffer
		if err := check("vkCreateFramebuffer", s.api.vkCreateFramebuffer(s.device, &info, nil, &fb)); err != nil {
			return err
		}
		s.Framebuffers[i] = fb
	}
	return nil
}

// destroyResources освобождает view/render pass/framebuffers, но оставляет
// сам swapchain (нужен как oldSwapchain при пересоздании).
func (s *Swapchain) destroyResources() {
	api, dev := s.api, s.device
	api.vkDeviceWaitIdle(dev)

	for _, fb := range s.Framebuffers {
		if fb != 0 {
			api.vkDestroyFramebuffer(dev, fb, nil)
		}
	}
	s.Framebuffers = nil
	for _, v := range s.Views {
		if v != 0 {
			api.vkDestroyImageView(dev, v, nil)
		}
	}
	s.Views = nil
	if s.DepthView != 0 {
		api.vkDestroyImageView(dev, s.DepthView, nil)
		s.DepthView = 0
	}
	if s.DepthImage != 0 || s.DepthMemory != 0 {
		api.DestroyImage(dev, s.DepthImage, s.DepthMemory)
		s.DepthImage, s.DepthMemory = 0, 0
	}
	if s.RenderPass != 0 {
		api.vkDestroyRenderPass(dev, s.RenderPass, nil)
		s.RenderPass = 0
	}
	for _, sem := range s.RenderFinished {
		if sem != 0 {
			api.vkDestroySemaphore(dev, sem, nil)
		}
	}
	s.RenderFinished = nil
}

// Destroy освобождает swapchain и все связанные ресурсы.
func (s *Swapchain) Destroy() {
	if s == nil {
		return
	}
	s.destroyResources()
	if s.Handle != 0 {
		s.api.vkDestroySwapchainKHR(s.device, s.Handle, nil)
		s.Handle = 0
	}
}

// Acquire запрашивает следующее изображение (ждёт semaphore).
// Ошибка IsOutOfDate означает, что окно изменилось — нужен Recreate.
func (s *Swapchain) Acquire(semaphore Semaphore) (uint32, error) {
	var index uint32
	r := s.api.vkAcquireNextImageKHR(s.device, s.Handle, ^uint64(0), semaphore, 0, &index)
	if r == ResultErrorOutOfDateKHR || r == ResultSuboptimalKHR {
		return 0, &Error{Op: "vkAcquireNextImageKHR", Code: r}
	}
	if err := check("vkAcquireNextImageKHR", r); err != nil {
		return 0, err
	}
	return index, nil
}

// Present показывает изображение; semaphore — renderFinished.
func (s *Swapchain) Present(queue Queue, index uint32, semaphore Semaphore) error {
	sw := s.Handle
	info := PresentInfoKHR{
		Type:               StructureTypePresentInfoKHR,
		WaitSemaphoreCount: boolToCount(semaphore != 0),
		WaitSemaphores:     semaphorePtr(semaphore),
		SwapchainCount:     1,
		Swapchains:         &sw,
		ImageIndices:       &index,
	}
	r := s.api.vkQueuePresentKHR(queue, &info)
	if r == ResultErrorOutOfDateKHR || r == ResultSuboptimalKHR {
		return &Error{Op: "vkQueuePresentKHR", Code: r}
	}
	return check("vkQueuePresentKHR", r)
}

// pickFormat выбирает формат: предпочтительный, затем sRGB, затем первый.
func pickFormat(formats []SurfaceFormatKHR, preferred Format) SurfaceFormatKHR {
	if len(formats) == 1 && formats[0].Format == FormatUndefined {
		// «любой формат» → выбираем стандартный
		return SurfaceFormatKHR{Format: FormatB8G8R8A8SRGB, ColorSpace: ColorSpaceSRGBNonlinearKHR}
	}
	if preferred != FormatUndefined {
		for _, f := range formats {
			if f.Format == preferred {
				return f
			}
		}
	}
	for _, f := range formats {
		if f.Format == FormatB8G8R8A8SRGB {
			return f
		}
	}
	for _, f := range formats {
		if f.Format == FormatB8G8R8A8UNORM || f.Format == FormatR8G8B8A8UNORM {
			return f
		}
	}
	return formats[0]
}

// pickPresentMode выбирает режим: предпочтительный, затем MAILBOX, затем FIFO.
func pickPresentMode(modes []PresentModeKHR, opts SwapchainOptions) PresentModeKHR {
	if opts.PreferredPresentMode != nil {
		want := *opts.PreferredPresentMode
		for _, m := range modes {
			if m == want {
				return m
			}
		}
	}
	for _, m := range modes {
		if m == PresentModeMailboxKHR {
			return m
		}
	}
	// FIFO гарантирован спецификацией
	return PresentModeFIFOKHR
}

// pickExtent определяет размер изображений: fixed extent поверхности либо размер окна,
// зажатый между min и max.
func pickExtent(caps SurfaceCapabilitiesKHR, width, height int) Extent2D {
	if caps.CurrentExtent.Width != ^uint32(0) {
		return caps.CurrentExtent
	}
	e := Extent2D{Width: uint32(width), Height: uint32(height)}
	if e.Width < caps.MinImageExtent.Width {
		e.Width = caps.MinImageExtent.Width
	}
	if e.Width > caps.MaxImageExtent.Width {
		e.Width = caps.MaxImageExtent.Width
	}
	if e.Height < caps.MinImageExtent.Height {
		e.Height = caps.MinImageExtent.Height
	}
	if e.Height > caps.MaxImageExtent.Height {
		e.Height = caps.MaxImageExtent.Height
	}
	if e.Width == 0 {
		e.Width = 1
	}
	if e.Height == 0 {
		e.Height = 1
	}
	return e
}
