package ui

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/Anykey-Nomad/Open-Kaurava/internal/shaders"
	"github.com/Anykey-Nomad/Open-Kaurava/vulkan"
)

// Options — параметры рендерера UI.
type Options struct {
	// Font — шрифт по умолчанию (обязателен).
	Font *Font
	// FramesInFlight — сколько вершинных буферов держать (по одному на кадр).
	FramesInFlight int
	// MaxVerts — максимум вершин в кадре.
	MaxVerts int
}

// Renderer — рисует батч квадов (текст и прямоугольники) поверх сцены.
//
// Внутри: SPIR-V шейдеры, A8-картина атласа глифов, сэмплер, набор
// дескрипторов, графический пайплайн и по одному вершинному буферу на кадр.
type Renderer struct {
	api    *vulkan.API
	dev    vulkan.Device
	swap   *vulkan.Swapchain
	queue  vulkan.Queue
	family uint32

	// Font — текущий шрифт (сцены могут рисовать разными).
	Font *Font
	// Atlas — атлас глифов (CPU-сторона).
	Atlas *Atlas

	setLayout  vulkan.DescriptorSetLayout
	pool       vulkan.DescriptorPool
	set        vulkan.DescriptorSet
	pipeLayout vulkan.PipelineLayout
	sampler    vulkan.Sampler
	img        vulkan.Image
	imgMem     vulkan.DeviceMemory
	view       vulkan.ImageView
	pipeline   vulkan.Pipeline

	cmdPool  vulkan.CommandPool
	vert     []vulkan.BufferMemory
	frames   int
	maxVerts int

	draw *Draw
}

// NewRenderer создаёт рендерер UI: шейдеры, атлас, пайплайн, буферы.
func NewRenderer(api *vulkan.API, dev vulkan.Device, swap *vulkan.Swapchain, queue vulkan.Queue, family uint32, opts Options) (*Renderer, error) {
	if opts.Font == nil {
		return nil, fmt.Errorf("ui: нужен шрифт")
	}
	if opts.FramesInFlight <= 0 {
		opts.FramesInFlight = 1
	}
	if opts.MaxVerts <= 0 {
		opts.MaxVerts = 32768
	}

	r := &Renderer{
		api:      api,
		dev:      dev,
		swap:     swap,
		queue:    queue,
		family:   family,
		Font:     opts.Font,
		Atlas:    NewAtlas(),
		frames:   opts.FramesInFlight,
		maxVerts: opts.MaxVerts,
	}
	r.draw = NewDraw(r.Atlas, opts.Font)

	// Пул команд для служебных отправок (заливка атласа).
	pool, err := api.CreateCommandPool(dev, family, 0)
	if err != nil {
		return nil, fmt.Errorf("ui: пул команд: %w", err)
	}
	r.cmdPool = pool

	if err := r.createResources(); err != nil {
		r.Destroy()
		return nil, err
	}
	return r, nil
}

// createResources создаёт картину атласа, сэмплер, дескрипторы и пайплайн.
func (r *Renderer) createResources() error {
	a := r.api

	// Картина атласа: одноканальная, в неё грузим покрытие глифов.
	img, mem, err := a.CreateImage2D(r.dev, vulkan.FormatR8UNorm, r.Atlas.W, r.Atlas.H,
		vulkan.ImageUsageTransferDstBit|vulkan.ImageUsageSampledBit)
	if err != nil {
		return fmt.Errorf("ui: картина атласа: %w", err)
	}
	r.img, r.imgMem = img, mem

	view, err := a.CreateImageView(r.dev, img, vulkan.FormatR8UNorm, vulkan.ImageAspectColorBit)
	if err != nil {
		return fmt.Errorf("ui: view атласа: %w", err)
	}
	r.view = view

	sampler, err := a.DefaultSampler(r.dev)
	if err != nil {
		return fmt.Errorf("ui: сэмплер: %w", err)
	}
	r.sampler = sampler

	// Набор дескрипторов: одна текстура во fragment-шейдере.
	binding := vulkan.DescriptorSetLayoutBinding{
		Binding:         0,
		DescriptorType:  vulkan.DescriptorTypeCombinedImageSampler,
		DescriptorCount: 1,
		StageFlags:      vulkan.ShaderStageFragmentBit,
	}
	setLayout, err := a.CreateDescriptorSetLayout(r.dev, []vulkan.DescriptorSetLayoutBinding{binding})
	if err != nil {
		return fmt.Errorf("ui: layout дескрипторов: %w", err)
	}
	r.setLayout = setLayout

	pool, err := a.CreateDescriptorPool(r.dev, []vulkan.DescriptorPoolSize{
		{Type: vulkan.DescriptorTypeCombinedImageSampler, Count: 1},
	}, 1)
	if err != nil {
		return fmt.Errorf("ui: пул дескрипторов: %w", err)
	}
	r.pool = pool

	set, err := a.AllocateDescriptorSet(r.dev, pool, setLayout)
	if err != nil {
		return fmt.Errorf("ui: набор дескрипторов: %w", err)
	}
	r.set = set

	write := vulkan.WriteDescriptorImage(r.set, 0, r.sampler, r.view, vulkan.ImageLayoutShaderReadOnlyOptimal)
	a.UpdateDescriptorSets(r.dev, []vulkan.WriteDescriptorSet{write})

	// Вершинные буферы: по одному на кадр в полёте.
	for i := 0; i < r.frames; i++ {
		buf, err := a.CreateBuffer(r.dev, uint64(r.maxVerts*VertexSize),
			vulkan.BufferUsageVertexBufferBit,
			vulkan.MemoryPropertyHostVisibleBit|vulkan.MemoryPropertyHostCoherentBit)
		if err != nil {
			return fmt.Errorf("ui: вершинный буфер: %w", err)
		}
		r.vert = append(r.vert, buf)
	}

	return r.createPipeline()
}

// createPipeline собирает графический пайплайн UI.
func (r *Renderer) createPipeline() error {
	a := r.api

	vert, err := a.CreateShaderModule(r.dev, shaders.QuadVert)
	if err != nil {
		return fmt.Errorf("ui: вершинный шейдер: %w", err)
	}
	defer a.DestroyShaderModule(r.dev, vert)

	frag, err := a.CreateShaderModule(r.dev, shaders.QuadFrag)
	if err != nil {
		return fmt.Errorf("ui: фрагментный шейдер: %w", err)
	}
	defer a.DestroyShaderModule(r.dev, frag)

	// Имя точки входа — "main" в обоих шейдерах.
	name := append([]byte("main"), 0)

	stages := []vulkan.PipelineShaderStageCreateInfo{
		{Type: vulkan.StructureTypePipelineShaderStageCreateInfo, Stage: vulkan.ShaderStageFlags(vulkan.ShaderStageVertexBit), Module: vert, Name: &name[0]},
		{Type: vulkan.StructureTypePipelineShaderStageCreateInfo, Stage: vulkan.ShaderStageFlags(vulkan.ShaderStageFragmentBit), Module: frag, Name: &name[0]},
	}

	// Вершина: pos(vec2) | uv(vec2) | color(vec4).
	binding := vulkan.VertexInputBindingDescription{
		Binding:   0,
		Stride:    VertexSize,
		InputRate: vulkan.VertexInputRateVertex,
	}
	attrs := []vulkan.VertexInputAttributeDescription{
		{Location: 0, Binding: 0, Format: vulkan.FormatR32G32SFloat, Offset: 0},
		{Location: 1, Binding: 0, Format: vulkan.FormatR32G32SFloat, Offset: 8},
		{Location: 2, Binding: 0, Format: vulkan.FormatR32G32B32ASFloat, Offset: 16},
	}
	vertexInput := vulkan.PipelineVertexInputStateCreateInfo{
		Type:                            vulkan.StructureTypePipelineVertexInputStateCreateInfo,
		VertexBindingDescriptionCount:   1,
		VertexBindingDescriptions:       &binding,
		VertexAttributeDescriptionCount: uint32(len(attrs)),
		VertexAttributeDescriptions:     &attrs[0],
	}

	inputAssembly := vulkan.PipelineInputAssemblyStateCreateInfo{
		Type:                   vulkan.StructureTypePipelineInputAssemblyStateCreateInfo,
		Topology:               vulkan.PrimitiveTopologyTriangleList,
		PrimitiveRestartEnable: 0,
	}

	// Вьюпорт и ножницы задаются динамически (размер меняется при resize).
	vp := vulkan.Viewport{}
	sc := vulkan.Rect2D{}
	viewportState := vulkan.PipelineViewportStateCreateInfo{
		Type:          vulkan.StructureTypePipelineViewportStateCreateInfo,
		ViewportCount: 1,
		Viewports:     &vp,
		ScissorCount:  1,
		Scissors:      &sc,
	}

	raster := vulkan.PipelineRasterizationStateCreateInfo{
		Type:        vulkan.StructureTypePipelineRasterizationStateCreateInfo,
		PolygonMode: vulkan.PolygonModeFill,
		CullMode:    vulkan.CullModeNone,
		FrontFace:   vulkan.FrontFaceCounterClockwise,
		LineWidth:   1.0,
	}

	multisample := vulkan.PipelineMultisampleStateCreateInfo{
		Type:                 vulkan.StructureTypePipelineMultisampleStateCreateInfo,
		RasterizationSamples: vulkan.SampleCount1Bit,
	}

	// Обычное альфа-смешивание: цвет — src*alpha + dst*(1-alpha).
	blend := vulkan.PipelineColorBlendAttachmentState{
		BlendEnable:         1,
		SrcColorBlendFactor: vulkan.BlendFactorSrcAlpha,
		DstColorBlendFactor: vulkan.BlendFactorOneMinusSrcAlpha,
		ColorBlendOp:        vulkan.BlendOpAdd,
		SrcAlphaBlendFactor: vulkan.BlendFactorSrcAlpha,
		DstAlphaBlendFactor: vulkan.BlendFactorOneMinusSrcAlpha,
		AlphaBlendOp:        vulkan.BlendOpAdd,
		ColorWriteMask: vulkan.ColorComponentRBit | vulkan.ColorComponentGBit |
			vulkan.ColorComponentBBit | vulkan.ColorComponentABit,
	}
	colorBlend := vulkan.PipelineColorBlendStateCreateInfo{
		Type:            vulkan.StructureTypePipelineColorBlendStateCreateInfo,
		AttachmentCount: 1,
		Attachments:     &blend,
	}

	dynStates := []vulkan.DynamicState{vulkan.DynamicStateViewport, vulkan.DynamicStateScissor}
	dynamic := vulkan.PipelineDynamicStateCreateInfo{
		Type:              vulkan.StructureTypePipelineDynamicStateCreateInfo,
		DynamicStateCount: uint32(len(dynStates)),
		DynamicStates:     &dynStates[0],
	}

	// Push constant: размер кадра в пикселях (для перевода в NDC).
	push := []vulkan.PushConstantRange{
		{StageFlags: vulkan.ShaderStageVertexBit, Offset: 0, Size: 8},
	}
	pipeLayout, err := a.CreatePipelineLayout(r.dev, []vulkan.DescriptorSetLayout{r.setLayout}, push)
	if err != nil {
		return fmt.Errorf("ui: layout пайплайна: %w", err)
	}
	r.pipeLayout = pipeLayout

	depthStencil := vulkan.PipelineDepthStencilStateCreateInfo{
		Type:            vulkan.StructureTypePipelineDepthStencilStateCreateInfo,
		DepthTestEnable: 0,
		DepthCompareOp:  vulkan.CompareOpAlways,
	}

	info := vulkan.GraphicsPipelineCreateInfo{
		Type:               vulkan.StructureTypeGraphicsPipelineCreateInfo,
		StageCount:         uint32(len(stages)),
		Stages:             &stages[0],
		VertexInputState:   &vertexInput,
		InputAssemblyState: &inputAssembly,
		ViewportState:      &viewportState,
		RasterizationState: &raster,
		MultisampleState:   &multisample,
		DepthStencilState:  unsafe.Pointer(&depthStencil),
		ColorBlendState:    &colorBlend,
		DynamicState:       &dynamic,
		Layout:             r.pipeLayout,
		RenderPass:         r.swap.RenderPass,
		Subpass:            0,
		BasePipelineIndex:  -1,
	}
	pipeline, err := a.CreateGraphicsPipeline(r.dev, info)
	runtime.KeepAlive(name)
	runtime.KeepAlive(depthStencil)
	if err != nil {
		return fmt.Errorf("ui: пайплайн: %w", err)
	}
	r.pipeline = pipeline
	return nil
}

// Recreate пересоздаёт пайплайн: после пересоздания swapchain меняется
// render pass, а пайплайн обязан ссылаться на актуальный.
func (r *Renderer) Recreate() error {
	a := r.api
	if r.pipeline != 0 {
		a.DestroyPipeline(r.dev, r.pipeline)
		r.pipeline = 0
	}
	return r.createPipeline()
}

// NewFrame начинает новый кадр и возвращает батчер для сцен.
func (r *Renderer) NewFrame() *Draw {
	r.draw.Reset()
	return r.draw
}

// Drawer возвращает текущий батчер (для сцен, рисующих вне NewFrame).
func (r *Renderer) Drawer() *Draw { return r.draw }

// Prepare готовит кадр к записи: доливает новые глифы в картину атласа
// и копирует вершины в буфер кадра.
//
// Вызывать до записи команд (внутри render pass загрузка атласа недопустима).
func (r *Renderer) Prepare(frameIndex int) error {
	if r.Atlas.Dirty() {
		if err := r.uploadAtlas(); err != nil {
			return err
		}
	}
	if len(r.draw.Verts) == 0 {
		return nil
	}
	slot := frameIndex % len(r.vert)
	n := len(r.draw.Verts)
	if n > r.maxVerts {
		r.draw.Dropped = n - r.maxVerts
		n = r.maxVerts
	}
	data := unsafe.Slice((*byte)(unsafe.Pointer(&r.draw.Verts[0])), n*VertexSize)
	return r.api.WriteBuffer(r.dev, r.vert[slot], data)
}

// Record пишет команды отрисовки батча (вызывается внутри render pass).
func (r *Renderer) Record(cmd vulkan.CommandBuffer, frameIndex int, width, height float32) error {
	if len(r.draw.Verts) == 0 {
		return nil
	}
	n := len(r.draw.Verts)
	if n > r.maxVerts {
		n = r.maxVerts
	}
	slot := frameIndex % len(r.vert)

	a := r.api
	a.CmdBindPipeline(cmd, r.pipeline)
	a.CmdBindDescriptorSet(cmd, r.pipeLayout, r.set)

	screen := [2]float32{width, height}
	a.CmdPushConstants(cmd, r.pipeLayout, vulkan.ShaderStageVertexBit, unsafe.Pointer(&screen[0]), 8)

	a.CmdSetViewport(cmd, width, height)
	a.CmdSetScissor(cmd, uint32(width), uint32(height))
	a.CmdBindVertexBuffer(cmd, r.vert[slot].Buffer, 0)
	a.CmdDraw(cmd, uint32(n))
	return nil
}

// uploadAtlas перезаливает картину атласа (новые глифы).
func (r *Renderer) uploadAtlas() error {
	a := r.api
	if err := a.DeviceWaitIdle(r.dev); err != nil {
		return err
	}

	staging, err := a.CreateBuffer(r.dev, uint64(len(r.Atlas.Pix)),
		vulkan.BufferUsageTransferSrcBit,
		vulkan.MemoryPropertyHostVisibleBit|vulkan.MemoryPropertyHostCoherentBit)
	if err != nil {
		return fmt.Errorf("ui: staging атласа: %w", err)
	}
	defer a.DestroyBuffer(r.dev, &staging)

	if err := a.WriteBuffer(r.dev, staging, r.Atlas.Pix); err != nil {
		return err
	}

	cmds, err := a.AllocateCommandBuffers(r.dev, r.cmdPool, 1)
	if err != nil {
		return fmt.Errorf("ui: командный буфер заливки: %w", err)
	}
	cmd := cmds[0]
	if err := a.BeginCommandBuffer(cmd, 0); err != nil {
		return err
	}

	barrier := vulkan.ImageMemoryBarrier{
		Type:                vulkan.StructureTypeImageMemoryBarrier,
		SrcAccessMask:       0,
		DstAccessMask:       vulkan.AccessTransferWriteBit,
		OldLayout:           vulkan.ImageLayoutUndefined,
		NewLayout:           vulkan.ImageLayoutTransferDstOptimal,
		SrcQueueFamilyIndex: vulkan.QueueFamilyIgnored,
		DstQueueFamilyIndex: vulkan.QueueFamilyIgnored,
		Image:               r.img,
		SubresourceRange: vulkan.ImageSubresourceRange{
			AspectMask:     vulkan.ImageAspectColorBit,
			BaseMipLevel:   0,
			LevelCount:     1,
			BaseArrayLayer: 0,
			LayerCount:     1,
		},
	}
	a.CmdImageBarrier(cmd, &barrier, vulkan.PipelineStageTopOfPipeBit, vulkan.PipelineStageTransferBit)
	a.CmdCopyBufferToImage(cmd, staging.Buffer, r.img, r.Atlas.W, r.Atlas.H)

	barrier.SrcAccessMask = vulkan.AccessTransferWriteBit
	barrier.DstAccessMask = vulkan.AccessShaderReadBit
	barrier.OldLayout = vulkan.ImageLayoutTransferDstOptimal
	barrier.NewLayout = vulkan.ImageLayoutShaderReadOnlyOptimal
	a.CmdImageBarrier(cmd, &barrier, vulkan.PipelineStageTransferBit, vulkan.PipelineStageFragmentShaderBit)

	if err := a.EndCommandBuffer(cmd); err != nil {
		return err
	}

	fence, err := a.CreateFence(r.dev)
	if err != nil {
		return fmt.Errorf("ui: fence заливки: %w", err)
	}
	defer a.DestroyFence(r.dev, fence)

	if err := a.QueueSubmit(r.queue, cmd, 0, 0, 0, fence); err != nil {
		return fmt.Errorf("ui: отправка заливки: %w", err)
	}
	if err := a.WaitForFence(fence); err != nil {
		return err
	}

	r.Atlas.ClearDirty()
	return nil
}

// Destroy освобождает все ресурсы рендерера.
func (r *Renderer) Destroy() {
	a := r.api
	if a == nil {
		return
	}
	if r.dev != 0 {
		_ = a.DeviceWaitIdle(r.dev)
	}
	if r.pipeline != 0 {
		a.DestroyPipeline(r.dev, r.pipeline)
		r.pipeline = 0
	}
	if r.pipeLayout != 0 {
		a.DestroyPipelineLayout(r.dev, r.pipeLayout)
		r.pipeLayout = 0
	}
	if r.pool != 0 {
		a.DestroyDescriptorPool(r.dev, r.pool)
		r.pool = 0
	}
	if r.setLayout != 0 {
		a.DestroyDescriptorSetLayout(r.dev, r.setLayout)
		r.setLayout = 0
	}
	if r.sampler != 0 {
		a.DestroySampler(r.dev, r.sampler)
		r.sampler = 0
	}
	if r.view != 0 {
		a.DestroyImageView(r.dev, r.view)
		r.view = 0
	}
	if r.img != 0 {
		a.DestroyImage(r.dev, r.img, r.imgMem)
		r.img, r.imgMem = 0, 0
	}
	for i := range r.vert {
		a.DestroyBuffer(r.dev, &r.vert[i])
	}
	r.vert = nil
	if r.cmdPool != 0 {
		a.DestroyCommandPool(r.dev, r.cmdPool)
		r.cmdPool = 0
	}
}
