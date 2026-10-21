package gfx3d

import (
	"fmt"
	"runtime"
	"unsafe"

	"dow-mod-studio/sgbview"
	"github.com/Anykey-Nomad/Open-Kaurava/internal/shaders"
	"github.com/Anykey-Nomad/Open-Kaurava/vulkan"
)

const vertexStride = 32 // Pos(12) + Normal(12) + UV(8)

// pushData — push-константы 3D-шейдеров (96 байт).
type pushData struct {
	ViewProj [16]float32
	Tint     [4]float32
	Params   [4]float32 // x = alphaRef, y = ambient, z = diffuse, w = unused
}

type gpuTexture struct {
	img  vulkan.Image
	mem  vulkan.DeviceMemory
	view vulkan.ImageView
	set  vulkan.DescriptorSet
}

type gpuMesh struct {
	vbuf      vulkan.BufferMemory
	ibuf      vulkan.BufferMemory
	count     uint32
	tex       *gpuTexture
	tint      [4]float32
	alphaRef  float32
	translucent bool
}

// Renderer — Vulkan-рендерер 3D-сцены сценария.
type Renderer struct {
	api    *vulkan.API
	dev    vulkan.Device
	swap   *vulkan.Swapchain
	queue  vulkan.Queue
	family uint32

	cmdPool     vulkan.CommandPool
	setLayout   vulkan.DescriptorSetLayout
	pipeLayout  vulkan.PipelineLayout
	sampler     vulkan.Sampler
	opaquePipe  vulkan.Pipeline
	blendPipe   vulkan.Pipeline

	descPool vulkan.DescriptorPool
	whiteTex *gpuTexture
	textures map[string]*gpuTexture
	meshes   []gpuMesh

	// texFormat — формат картинок сцены. Если кадр в sRGB, то и текстуры
	// читаются как sRGB: иначе цвет «выцветает» (значения sRGB считаются
	// линейными и снова кодируются при выводе).
	texFormat vulkan.Format
}

// NewRenderer создаёт 3D-рендерер.
func NewRenderer(api *vulkan.API, dev vulkan.Device, swap *vulkan.Swapchain, queue vulkan.Queue, family uint32) (*Renderer, error) {
	r := &Renderer{
		api:       api,
		dev:       dev,
		swap:      swap,
		queue:     queue,
		family:    family,
		textures:  map[string]*gpuTexture{},
		texFormat: vulkan.FormatR8G8B8A8UNORM,
	}
	if swap != nil && isSRGBFormat(swap.Format) {
		r.texFormat = vulkan.FormatR8G8B8A8SRGB
	}

	pool, err := api.CreateCommandPool(dev, family, 0)
	if err != nil {
		return nil, fmt.Errorf("gfx3d: пул команд: %w", err)
	}
	r.cmdPool = pool

	binding := vulkan.DescriptorSetLayoutBinding{
		Binding:         0,
		DescriptorType:  vulkan.DescriptorTypeCombinedImageSampler,
		DescriptorCount: 1,
		StageFlags:      vulkan.ShaderStageFragmentBit,
	}
	setLayout, err := api.CreateDescriptorSetLayout(dev, []vulkan.DescriptorSetLayoutBinding{binding})
	if err != nil {
		r.Destroy()
		return nil, fmt.Errorf("gfx3d: layout дескрипторов: %w", err)
	}
	r.setLayout = setLayout

	sampler, err := api.CreateSampler(dev, vulkan.SamplerCreateInfo{
		MagFilter:    vulkan.FilterLinear,
		MinFilter:    vulkan.FilterLinear,
		MipmapMode:   vulkan.SamplerMipmapModeLinear,
		AddressModeU: vulkan.SamplerAddressModeRepeat,
		AddressModeV: vulkan.SamplerAddressModeRepeat,
		AddressModeW: vulkan.SamplerAddressModeRepeat,
		MaxLod:       1.0,
	})
	if err != nil {
		r.Destroy()
		return nil, fmt.Errorf("gfx3d: сэмплер: %w", err)
	}
	r.sampler = sampler

	push := []vulkan.PushConstantRange{
		{
			StageFlags: vulkan.ShaderStageVertexBit | vulkan.ShaderStageFragmentBit,
			Offset:     0,
			Size:       uint32(unsafe.Sizeof(pushData{})),
		},
	}
	pipeLayout, err := api.CreatePipelineLayout(dev, []vulkan.DescriptorSetLayout{setLayout}, push)
	if err != nil {
		r.Destroy()
		return nil, fmt.Errorf("gfx3d: layout пайплайна: %w", err)
	}
	r.pipeLayout = pipeLayout

	if err := r.createPipelines(); err != nil {
		r.Destroy()
		return nil, err
	}

	if err := r.resetDescriptorPool(64); err != nil {
		r.Destroy()
		return nil, err
	}

	return r, nil
}

func (r *Renderer) createPipelines() error {
	a := r.api

	vert, err := a.CreateShaderModule(r.dev, shaders.Mesh3DVert)
	if err != nil {
		return fmt.Errorf("gfx3d: вершинный шейдер: %w", err)
	}
	defer a.DestroyShaderModule(r.dev, vert)

	frag, err := a.CreateShaderModule(r.dev, shaders.Mesh3DFrag)
	if err != nil {
		return fmt.Errorf("gfx3d: фрагментный шейдер: %w", err)
	}
	defer a.DestroyShaderModule(r.dev, frag)

	name := append([]byte("main"), 0)
	stages := []vulkan.PipelineShaderStageCreateInfo{
		{Type: vulkan.StructureTypePipelineShaderStageCreateInfo, Stage: vulkan.ShaderStageFlags(vulkan.ShaderStageVertexBit), Module: vert, Name: &name[0]},
		{Type: vulkan.StructureTypePipelineShaderStageCreateInfo, Stage: vulkan.ShaderStageFlags(vulkan.ShaderStageFragmentBit), Module: frag, Name: &name[0]},
	}

	binding := vulkan.VertexInputBindingDescription{
		Binding:   0,
		Stride:    vertexStride,
		InputRate: vulkan.VertexInputRateVertex,
	}
	attrs := []vulkan.VertexInputAttributeDescription{
		{Location: 0, Binding: 0, Format: vulkan.FormatR32G32B32SFloat, Offset: 0},
		{Location: 1, Binding: 0, Format: vulkan.FormatR32G32B32SFloat, Offset: 12},
		{Location: 2, Binding: 0, Format: vulkan.FormatR32G32SFloat, Offset: 24},
	}
	vertexInput := vulkan.PipelineVertexInputStateCreateInfo{
		Type:                            vulkan.StructureTypePipelineVertexInputStateCreateInfo,
		VertexBindingDescriptionCount:   1,
		VertexBindingDescriptions:       &binding,
		VertexAttributeDescriptionCount: uint32(len(attrs)),
		VertexAttributeDescriptions:     &attrs[0],
	}

	inputAssembly := vulkan.PipelineInputAssemblyStateCreateInfo{
		Type:     vulkan.StructureTypePipelineInputAssemblyStateCreateInfo,
		Topology: vulkan.PrimitiveTopologyTriangleList,
	}

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

	dynStates := []vulkan.DynamicState{vulkan.DynamicStateViewport, vulkan.DynamicStateScissor}
	dynamic := vulkan.PipelineDynamicStateCreateInfo{
		Type:              vulkan.StructureTypePipelineDynamicStateCreateInfo,
		DynamicStateCount: uint32(len(dynStates)),
		DynamicStates:     &dynStates[0],
	}

	// Непрозрачный пайплайн: тест и запись глубины включены.
	depthOpaque := vulkan.PipelineDepthStencilStateCreateInfo{
		Type:             vulkan.StructureTypePipelineDepthStencilStateCreateInfo,
		DepthTestEnable:  1,
		DepthWriteEnable: 1,
		DepthCompareOp:   vulkan.CompareOpLessOrEqual,
	}
	blendOpaque := vulkan.PipelineColorBlendAttachmentState{
		BlendEnable: 0,
		ColorWriteMask: vulkan.ColorComponentRBit | vulkan.ColorComponentGBit |
			vulkan.ColorComponentBBit | vulkan.ColorComponentABit,
	}
	colorBlendOpaque := vulkan.PipelineColorBlendStateCreateInfo{
		Type:            vulkan.StructureTypePipelineColorBlendStateCreateInfo,
		AttachmentCount: 1,
		Attachments:     &blendOpaque,
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
		DepthStencilState:  unsafe.Pointer(&depthOpaque),
		ColorBlendState:    &colorBlendOpaque,
		DynamicState:       &dynamic,
		Layout:             r.pipeLayout,
		RenderPass:         r.swap.RenderPass,
		Subpass:            0,
		BasePipelineIndex:  -1,
	}
	opaquePipe, err := a.CreateGraphicsPipeline(r.dev, info)
	runtime.KeepAlive(name)
	runtime.KeepAlive(depthOpaque)
	if err != nil {
		return fmt.Errorf("gfx3d: непрозрачный пайплайн: %w", err)
	}
	r.opaquePipe = opaquePipe

	// Полупрозрачный пайплайн (для воды): тест глубины включён, запись выключена.
	depthBlend := vulkan.PipelineDepthStencilStateCreateInfo{
		Type:             vulkan.StructureTypePipelineDepthStencilStateCreateInfo,
		DepthTestEnable:  1,
		DepthWriteEnable: 0,
		DepthCompareOp:   vulkan.CompareOpLessOrEqual,
	}
	blendAlpha := vulkan.PipelineColorBlendAttachmentState{
		BlendEnable:         1,
		SrcColorBlendFactor: vulkan.BlendFactorSrcAlpha,
		DstColorBlendFactor: vulkan.BlendFactorOneMinusSrcAlpha,
		ColorBlendOp:        vulkan.BlendOpAdd,
		SrcAlphaBlendFactor: vulkan.BlendFactorOne,
		DstAlphaBlendFactor: vulkan.BlendFactorOneMinusSrcAlpha,
		AlphaBlendOp:        vulkan.BlendOpAdd,
		ColorWriteMask: vulkan.ColorComponentRBit | vulkan.ColorComponentGBit |
			vulkan.ColorComponentBBit | vulkan.ColorComponentABit,
	}
	colorBlendAlpha := vulkan.PipelineColorBlendStateCreateInfo{
		Type:            vulkan.StructureTypePipelineColorBlendStateCreateInfo,
		AttachmentCount: 1,
		Attachments:     &blendAlpha,
	}
	info.DepthStencilState = unsafe.Pointer(&depthBlend)
	info.ColorBlendState = &colorBlendAlpha
	blendPipe, err := a.CreateGraphicsPipeline(r.dev, info)
	runtime.KeepAlive(name)
	runtime.KeepAlive(depthBlend)
	if err != nil {
		a.DestroyPipeline(r.dev, r.opaquePipe)
		r.opaquePipe = 0
		return fmt.Errorf("gfx3d: полупрозрачный пайплайн: %w", err)
	}
	r.blendPipe = blendPipe
	return nil
}

// Recreate пересоздаёт пайплайны после пересоздания swapchain.
func (r *Renderer) Recreate() error {
	a := r.api
	if r.opaquePipe != 0 {
		a.DestroyPipeline(r.dev, r.opaquePipe)
		r.opaquePipe = 0
	}
	if r.blendPipe != 0 {
		a.DestroyPipeline(r.dev, r.blendPipe)
		r.blendPipe = 0
	}
	return r.createPipelines()
}

func (r *Renderer) resetDescriptorPool(maxSets uint32) error {
	a := r.api
	if maxSets < 16 {
		maxSets = 16
	}
	if r.whiteTex != nil {
		r.destroyTexture(r.whiteTex)
		r.whiteTex = nil
	}
	for _, t := range r.textures {
		r.destroyTexture(t)
	}
	r.textures = map[string]*gpuTexture{}
	if r.descPool != 0 {
		a.DestroyDescriptorPool(r.dev, r.descPool)
		r.descPool = 0
	}

	pool, err := a.CreateDescriptorPool(r.dev, []vulkan.DescriptorPoolSize{
		{Type: vulkan.DescriptorTypeCombinedImageSampler, Count: maxSets},
	}, maxSets)
	if err != nil {
		return fmt.Errorf("gfx3d: пул дескрипторов: %w", err)
	}
	r.descPool = pool

	white, err := r.uploadTexture(1, 1, []byte{0xff, 0xff, 0xff, 0xff})
	if err != nil {
		return fmt.Errorf("gfx3d: белая текстура: %w", err)
	}
	r.whiteTex = white
	return nil
}

// ClearScene освобождает буферы и текстуры текущей сцены.
func (r *Renderer) ClearScene() {
	if r == nil || r.api == nil {
		return
	}
	_ = r.api.DeviceWaitIdle(r.dev)
	for i := range r.meshes {
		r.api.DestroyBuffer(r.dev, &r.meshes[i].vbuf)
		r.api.DestroyBuffer(r.dev, &r.meshes[i].ibuf)
	}
	r.meshes = nil
}

// UploadScene загружает собранную сцену сценария в память GPU.
func (r *Renderer) UploadScene(sc *sgbview.Scene) error {
	r.ClearScene()
	if sc == nil {
		return nil
	}

	// Считаем сколько уникальных текстур понадобится.
	uniqueTex := map[string]bool{}
	if sc.TerrainTexture != nil && sc.TerrainTexture.Key != "" {
		uniqueTex[sc.TerrainTexture.Key] = true
	}
	for _, obj := range sc.Objects {
		for _, p := range obj.Parts {
			if p.Texture != nil && p.Texture.Key != "" {
				uniqueTex[p.Texture.Key] = true
			}
		}
	}
	if err := r.resetDescriptorPool(uint32(len(uniqueTex) + 8)); err != nil {
		return err
	}

	getTex := func(t *sgbview.Texture) *gpuTexture {
		if t == nil || t.Width <= 0 || t.Height <= 0 || len(t.Pixels) < t.Width*t.Height*4 {
			return r.whiteTex
		}
		if existing, ok := r.textures[t.Key]; ok {
			return existing
		}
		gt, err := r.uploadTexture(t.Width, t.Height, t.Pixels)
		if err != nil {
			return r.whiteTex
		}
		if t.Key != "" {
			r.textures[t.Key] = gt
		}
		return gt
	}

	white := [4]float32{1, 1, 1, 1}

	// 1. Рельеф.
	if sc.Terrain != nil {
		tex := getTex(sc.TerrainTexture)
		if err := r.addMesh(sc.Terrain, tex, white, 0.0, false); err != nil {
			return err
		}
	}

	// 2. Объекты карты.
	for _, obj := range sc.Objects {
		for _, p := range obj.Parts {
			if p.Mesh == nil {
				continue
			}
			tex := getTex(p.Texture)
			if err := r.addMesh(p.Mesh, tex, white, 0.5, false); err != nil {
				return err
			}
		}
	}

	// 3. Вода (рисуется последней с альфа-смешиванием).
	if sc.Water != nil {
		wc := sgbview.WaterColor
		waterTint := [4]float32{
			float32(wc[0]) / 255,
			float32(wc[1]) / 255,
			float32(wc[2]) / 255,
			float32(wc[3]) / 255,
		}
		if err := r.addMesh(sc.Water, r.whiteTex, waterTint, 0.0, true); err != nil {
			return err
		}
	}

	return nil
}

func (r *Renderer) addMesh(m *sgbview.Mesh, tex *gpuTexture, tint [4]float32, alphaRef float32, translucent bool) error {
	if m == nil || len(m.Vertices) == 0 || len(m.Indices) < 3 {
		return nil
	}
	a := r.api

	vbytes := unsafe.Slice((*byte)(unsafe.Pointer(&m.Vertices[0])), len(m.Vertices)*vertexStride)
	vbuf, err := a.CreateBuffer(r.dev, uint64(len(vbytes)),
		vulkan.BufferUsageVertexBufferBit,
		vulkan.MemoryPropertyHostVisibleBit|vulkan.MemoryPropertyHostCoherentBit)
	if err != nil {
		return fmt.Errorf("gfx3d: вершинный буфер: %w", err)
	}
	if err := a.WriteBuffer(r.dev, vbuf, vbytes); err != nil {
		a.DestroyBuffer(r.dev, &vbuf)
		return err
	}

	ibytes := unsafe.Slice((*byte)(unsafe.Pointer(&m.Indices[0])), len(m.Indices)*4)
	ibuf, err := a.CreateBuffer(r.dev, uint64(len(ibytes)),
		vulkan.BufferUsageIndexBufferBit,
		vulkan.MemoryPropertyHostVisibleBit|vulkan.MemoryPropertyHostCoherentBit)
	if err != nil {
		a.DestroyBuffer(r.dev, &vbuf)
		return fmt.Errorf("gfx3d: индексный буфер: %w", err)
	}
	if err := a.WriteBuffer(r.dev, ibuf, ibytes); err != nil {
		a.DestroyBuffer(r.dev, &ibuf)
		a.DestroyBuffer(r.dev, &vbuf)
		return err
	}

	if tex == nil {
		tex = r.whiteTex
	}
	r.meshes = append(r.meshes, gpuMesh{
		vbuf:        vbuf,
		ibuf:        ibuf,
		count:       uint32(len(m.Indices)),
		tex:         tex,
		tint:        tint,
		alphaRef:    alphaRef,
		translucent: translucent,
	})
	return nil
}

func (r *Renderer) uploadTexture(width, height int, pixels []byte) (*gpuTexture, error) {
	a := r.api
	img, mem, err := a.CreateImage2D(r.dev, r.texFormat, width, height,
		vulkan.ImageUsageTransferDstBit|vulkan.ImageUsageSampledBit)
	if err != nil {
		return nil, err
	}

	staging, err := a.CreateBuffer(r.dev, uint64(len(pixels)),
		vulkan.BufferUsageTransferSrcBit,
		vulkan.MemoryPropertyHostVisibleBit|vulkan.MemoryPropertyHostCoherentBit)
	if err != nil {
		a.DestroyImage(r.dev, img, mem)
		return nil, err
	}
	defer a.DestroyBuffer(r.dev, &staging)

	if err := a.WriteBuffer(r.dev, staging, pixels); err != nil {
		a.DestroyImage(r.dev, img, mem)
		return nil, err
	}

	cmds, err := a.AllocateCommandBuffers(r.dev, r.cmdPool, 1)
	if err != nil {
		a.DestroyImage(r.dev, img, mem)
		return nil, err
	}
	cmd := cmds[0]
	if err := a.BeginCommandBuffer(cmd, 0); err != nil {
		a.DestroyImage(r.dev, img, mem)
		return nil, err
	}

	barrier := vulkan.ImageMemoryBarrier{
		Type:                vulkan.StructureTypeImageMemoryBarrier,
		SrcAccessMask:       0,
		DstAccessMask:       vulkan.AccessTransferWriteBit,
		OldLayout:           vulkan.ImageLayoutUndefined,
		NewLayout:           vulkan.ImageLayoutTransferDstOptimal,
		SrcQueueFamilyIndex: vulkan.QueueFamilyIgnored,
		DstQueueFamilyIndex: vulkan.QueueFamilyIgnored,
		Image:               img,
		SubresourceRange: vulkan.ImageSubresourceRange{
			AspectMask:     vulkan.ImageAspectColorBit,
			BaseMipLevel:   0,
			LevelCount:     1,
			BaseArrayLayer: 0,
			LayerCount:     1,
		},
	}
	a.CmdImageBarrier(cmd, &barrier, vulkan.PipelineStageTopOfPipeBit, vulkan.PipelineStageTransferBit)
	a.CmdCopyBufferToImage(cmd, staging.Buffer, img, width, height)

	barrier.SrcAccessMask = vulkan.AccessTransferWriteBit
	barrier.DstAccessMask = vulkan.AccessShaderReadBit
	barrier.OldLayout = vulkan.ImageLayoutTransferDstOptimal
	barrier.NewLayout = vulkan.ImageLayoutShaderReadOnlyOptimal
	a.CmdImageBarrier(cmd, &barrier, vulkan.PipelineStageTransferBit, vulkan.PipelineStageFragmentShaderBit)

	if err := a.EndCommandBuffer(cmd); err != nil {
		a.DestroyImage(r.dev, img, mem)
		return nil, err
	}

	fence, err := a.CreateFence(r.dev)
	if err != nil {
		a.DestroyImage(r.dev, img, mem)
		return nil, err
	}
	defer a.DestroyFence(r.dev, fence)

	if err := a.QueueSubmit(r.queue, cmd, 0, 0, 0, fence); err != nil {
		a.DestroyImage(r.dev, img, mem)
		return nil, err
	}
	if err := a.WaitForFence(fence); err != nil {
		a.DestroyImage(r.dev, img, mem)
		return nil, err
	}

	view, err := a.CreateImageView(r.dev, img, r.texFormat, vulkan.ImageAspectColorBit)
	if err != nil {
		a.DestroyImage(r.dev, img, mem)
		return nil, err
	}

	set, err := a.AllocateDescriptorSet(r.dev, r.descPool, r.setLayout)
	if err != nil {
		a.DestroyImageView(r.dev, view)
		a.DestroyImage(r.dev, img, mem)
		return nil, err
	}
	write := vulkan.WriteDescriptorImage(set, 0, r.sampler, view, vulkan.ImageLayoutShaderReadOnlyOptimal)
	a.UpdateDescriptorSets(r.dev, []vulkan.WriteDescriptorSet{write})

	return &gpuTexture{img: img, mem: mem, view: view, set: set}, nil
}

func (r *Renderer) destroyTexture(t *gpuTexture) {
	if t == nil {
		return
	}
	if t.view != 0 {
		r.api.DestroyImageView(r.dev, t.view)
		t.view = 0
	}
	if t.img != 0 || t.mem != 0 {
		r.api.DestroyImage(r.dev, t.img, t.mem)
		t.img, t.mem = 0, 0
	}
}

// HasMeshes сообщает, загружена ли сцена для отрисовки.
func (r *Renderer) HasMeshes() bool {
	return r != nil && len(r.meshes) > 0
}

// isSRGBFormat сообщает, что формат кадра кодирует цвет в sRGB.
func isSRGBFormat(f vulkan.Format) bool {
	return f == vulkan.FormatB8G8R8A8SRGB || f == vulkan.FormatR8G8B8A8SRGB
}

// Record записывает команды отрисовки 3D-сцены в активный render pass.
func (r *Renderer) Record(cmd vulkan.CommandBuffer, cam *Camera, width, height float32) error {
	if r == nil || len(r.meshes) == 0 || cam == nil {
		return nil
	}
	a := r.api
	a.CmdSetViewport(cmd, width, height)
	a.CmdSetScissor(cmd, uint32(width), uint32(height))

	vp := cam.ViewProj(int(width), int(height))
	stages := vulkan.ShaderStageVertexBit | vulkan.ShaderStageFragmentBit

	// Освещение как в просмотрщике dow-mod-studio: мягкая полутень + основная яркость текстуры.
	const (
		ambient = 0.55
		diffuse = 0.55
	)

	currentBlend := false
	a.CmdBindPipeline(cmd, r.opaquePipe)

	for i := range r.meshes {
		m := &r.meshes[i]
		if m.translucent != currentBlend {
			currentBlend = m.translucent
			if currentBlend {
				a.CmdBindPipeline(cmd, r.blendPipe)
			} else {
				a.CmdBindPipeline(cmd, r.opaquePipe)
			}
		}
		pc := pushData{
			ViewProj: vp,
			Tint:     m.tint,
			Params:   [4]float32{m.alphaRef, ambient, diffuse, 0},
		}
		a.CmdPushConstants(cmd, r.pipeLayout, stages, unsafe.Pointer(&pc), uint32(unsafe.Sizeof(pc)))
		a.CmdBindDescriptorSet(cmd, r.pipeLayout, m.tex.set)
		a.CmdBindVertexBuffer(cmd, m.vbuf.Buffer, 0)
		a.CmdBindIndexBuffer(cmd, m.ibuf.Buffer, 0, vulkan.IndexTypeUint32)
		a.CmdDrawIndexed(cmd, m.count)
	}
	return nil
}

// Destroy освобождает все ресурсы 3D-рендерера.
func (r *Renderer) Destroy() {
	if r == nil || r.api == nil {
		return
	}
	a := r.api
	if r.dev != 0 {
		_ = a.DeviceWaitIdle(r.dev)
	}
	r.ClearScene()
	if r.whiteTex != nil {
		r.destroyTexture(r.whiteTex)
		r.whiteTex = nil
	}
	for _, t := range r.textures {
		r.destroyTexture(t)
	}
	r.textures = nil
	if r.opaquePipe != 0 {
		a.DestroyPipeline(r.dev, r.opaquePipe)
		r.opaquePipe = 0
	}
	if r.blendPipe != 0 {
		a.DestroyPipeline(r.dev, r.blendPipe)
		r.blendPipe = 0
	}
	if r.pipeLayout != 0 {
		a.DestroyPipelineLayout(r.dev, r.pipeLayout)
		r.pipeLayout = 0
	}
	if r.descPool != 0 {
		a.DestroyDescriptorPool(r.dev, r.descPool)
		r.descPool = 0
	}
	if r.setLayout != 0 {
		a.DestroyDescriptorSetLayout(r.dev, r.setLayout)
		r.setLayout = 0
	}
	if r.sampler != 0 {
		a.DestroySampler(r.dev, r.sampler)
		r.sampler = 0
	}
	if r.cmdPool != 0 {
		a.DestroyCommandPool(r.dev, r.cmdPool)
		r.cmdPool = 0
	}
}
