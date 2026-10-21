package vulkan

import "unsafe"

// Ресурсы графического пайплайна UI: память, буферы, картины (атласы),
// шейдерные модули, пайплайн, дескрипторы и сэмплер.
//
// Здесь только тонкие обёртки над уже загруженными точками входа уровня
// device (см. API.loadDevice). Высокоуровневая логика — в пакете ui.

// MemoryProperties возвращает свойства памяти физического устройства.
func (a *API) MemoryProperties() PhysicalDeviceMemoryProperties {
	var props PhysicalDeviceMemoryProperties
	if a.pd != 0 {
		a.vkGetPhysicalDeviceMemoryProperties(a.pd, &props)
	}
	return props
}

// FindMemoryTypeIndex подбирает индекс типа памяти: бит из требований
// ресурса И требуемые свойства (host-visible и т.п.).
func (a *API) FindMemoryTypeIndex(typeBits, wanted uint32) (uint32, error) {
	props := a.MemoryProperties()
	for i := uint32(0); i < props.MemoryTypeCount; i++ {
		if typeBits&(1<<i) != 0 && props.MemoryTypes[i].PropertyFlags&wanted == wanted {
			return i, nil
		}
	}
	return 0, &Error{Op: "FindMemoryTypeIndex", Code: ResultErrorInitializationFailed}
}

// BufferMemory — буфер вместе с выделенной под него памятью.
type BufferMemory struct {
	Buffer Buffer
	Memory DeviceMemory
	Size   uint64
}

// CreateBuffer создаёт буфер и память под него (size выравнивается до требований).
func (a *API) CreateBuffer(dev Device, size uint64, usage, memProps uint32) (BufferMemory, error) {
	info := BufferCreateInfo{
		Type:        StructureTypeBufferCreateInfo,
		Size:        size,
		Usage:       usage,
		SharingMode: SharingModeExclusive,
	}
	var out BufferMemory
	if err := check("vkCreateBuffer", a.vkCreateBuffer(dev, &info, nil, &out.Buffer)); err != nil {
		return out, err
	}
	out.Size = size

	var req MemoryRequirements
	a.vkGetBufferMemoryRequirements(dev, out.Buffer, &req)

	idx, err := a.FindMemoryTypeIndex(req.MemoryTypeBits, memProps)
	if err != nil {
		a.vkDestroyBuffer(dev, out.Buffer, nil)
		out.Buffer = 0
		return out, err
	}
	alloc := MemoryAllocateInfo{
		Type:            StructureTypeMemoryAllocateInfo,
		AllocationSize:  req.Size,
		MemoryTypeIndex: idx,
	}
	if err := check("vkAllocateMemory", a.vkAllocateMemory(dev, &alloc, nil, &out.Memory)); err != nil {
		a.vkDestroyBuffer(dev, out.Buffer, nil)
		out.Buffer = 0
		return out, err
	}
	if err := check("vkBindBufferMemory", a.vkBindBufferMemory(dev, out.Buffer, out.Memory, 0)); err != nil {
		a.vkFreeMemory(dev, out.Memory, nil)
		a.vkDestroyBuffer(dev, out.Buffer, nil)
		out.Buffer, out.Memory = 0, 0
		return out, err
	}
	return out, nil
}

// Destroy освобождает буфер и его память.
func (a *API) DestroyBuffer(dev Device, b *BufferMemory) {
	if b == nil {
		return
	}
	if b.Buffer != 0 {
		a.vkDestroyBuffer(dev, b.Buffer, nil)
		b.Buffer = 0
	}
	if b.Memory != 0 {
		a.vkFreeMemory(dev, b.Memory, nil)
		b.Memory = 0
	}
}

// WriteBuffer записывает данные в host-visible буфер (map → copy → unmap).
func (a *API) WriteBuffer(dev Device, b BufferMemory, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	var ptr unsafe.Pointer
	if err := check("vkMapMemory", a.vkMapMemory(dev, b.Memory, 0, uint64(len(data)), 0, &ptr)); err != nil {
		return err
	}
	dst := unsafe.Slice((*byte)(ptr), len(data))
	copy(dst, data)
	a.vkUnmapMemory(dev, b.Memory)
	return nil
}

// ReadBuffer читает данные из host-visible буфера (map → copy → unmap).
func (a *API) ReadBuffer(dev Device, b BufferMemory, dst []byte) error {
	if len(dst) == 0 {
		return nil
	}
	var ptr unsafe.Pointer
	if err := check("vkMapMemory", a.vkMapMemory(dev, b.Memory, 0, uint64(len(dst)), 0, &ptr)); err != nil {
		return err
	}
	copy(dst, unsafe.Slice((*byte)(ptr), len(dst)))
	a.vkUnmapMemory(dev, b.Memory)
	return nil
}

// CreateImage2D создаёт 2D-картину (атлас глифов) и память под неё.
func (a *API) CreateImage2D(dev Device, format Format, width, height int, usage uint32) (Image, DeviceMemory, error) {
	info := ImageCreateInfo{
		Type:          StructureTypeImageCreateInfo,
		ImageType:     ImageType2D,
		Format:        format,
		Extent:        Extent3D{Width: uint32(width), Height: uint32(height), Depth: 1},
		MipLevels:     1,
		ArrayLayers:   1,
		Samples:       SampleCount1Bit,
		Tiling:        ImageTilingOptimal,
		Usage:         usage,
		SharingMode:   SharingModeExclusive,
		InitialLayout: ImageLayoutUndefined,
	}
	var img Image
	if err := check("vkCreateImage", a.vkCreateImage(dev, &info, nil, &img)); err != nil {
		return 0, 0, err
	}

	var req MemoryRequirements
	a.vkGetImageMemoryRequirements(dev, img, &req)
	idx, err := a.FindMemoryTypeIndex(req.MemoryTypeBits, MemoryPropertyDeviceLocalBit)
	if err != nil {
		a.vkDestroyImage(dev, img, nil)
		return 0, 0, err
	}
	alloc := MemoryAllocateInfo{
		Type:            StructureTypeMemoryAllocateInfo,
		AllocationSize:  req.Size,
		MemoryTypeIndex: idx,
	}
	var mem DeviceMemory
	if err := check("vkAllocateMemory", a.vkAllocateMemory(dev, &alloc, nil, &mem)); err != nil {
		a.vkDestroyImage(dev, img, nil)
		return 0, 0, err
	}
	if err := check("vkBindImageMemory", a.vkBindImageMemory(dev, img, mem, 0)); err != nil {
		a.vkFreeMemory(dev, mem, nil)
		a.vkDestroyImage(dev, img, nil)
		return 0, 0, err
	}
	return img, mem, nil
}

// DestroyImage освобождает картину и её память.
func (a *API) DestroyImage(dev Device, img Image, mem DeviceMemory) {
	if img != 0 {
		a.vkDestroyImage(dev, img, nil)
	}
	if mem != 0 {
		a.vkFreeMemory(dev, mem, nil)
	}
}

// TransitionImageLayout переводит картину между layout (барьер в командном буфере).
func (a *API) TransitionImageLayout(cmd CommandBuffer, img Image, old, new ImageLayout, srcAccess, dstAccess uint32) {
	barrier := ImageMemoryBarrier{
		Type:                StructureTypeImageMemoryBarrier,
		SrcAccessMask:       srcAccess,
		DstAccessMask:       dstAccess,
		OldLayout:           old,
		NewLayout:           new,
		SrcQueueFamilyIndex: QueueFamilyIgnored,
		DstQueueFamilyIndex: QueueFamilyIgnored,
		Image:               img,
		SubresourceRange: ImageSubresourceRange{
			AspectMask:     ImageAspectColorBit,
			BaseMipLevel:   0,
			LevelCount:     1,
			BaseArrayLayer: 0,
			LayerCount:     1,
		},
	}
	a.vkCmdPipelineBarrier(cmd,
		PipelineStageTopOfPipeBit, PipelineStageTransferBit, 0,
		0, nil, 0, nil, 1, &barrier)
}

// CmdCopyBufferToImage копирует содержимое буфера в 2D-картину
// (изображение должно быть в TransferDstOptimal).
func (a *API) CmdCopyBufferToImage(cmd CommandBuffer, buf Buffer, img Image, width, height int) {
	region := BufferImageCopy{
		BufferOffset: 0,
		ImageSubresource: ImageSubresourceLayers{
			AspectMask: ImageAspectColorBit,
			LayerCount: 1,
		},
		ImageExtent: Extent3D{Width: uint32(width), Height: uint32(height), Depth: 1},
	}
	a.vkCmdCopyBufferToImage(cmd, buf, img, ImageLayoutTransferDstOptimal, 1, &region)
}

// CreateImageView создаёт 2D-view картину (формат = формат картины).
func (a *API) CreateImageView(dev Device, img Image, format Format, aspect uint32) (ImageView, error) {
	info := ImageViewCreateInfo{
		Type:     StructureTypeImageViewCreateInfo,
		Image:    img,
		ViewType: ImageViewType2D,
		Format:   format,
		Components: ComponentMapping{R: ComponentSwizzleIdentity, G: ComponentSwizzleIdentity,
			B: ComponentSwizzleIdentity, A: ComponentSwizzleIdentity},
		SubresourceRange: ImageSubresourceRange{
			AspectMask:     aspect,
			BaseMipLevel:   0,
			LevelCount:     1,
			BaseArrayLayer: 0,
			LayerCount:     1,
		},
	}
	var v ImageView
	if err := check("vkCreateImageView", a.vkCreateImageView(dev, &info, nil, &v)); err != nil {
		return 0, err
	}
	return v, nil
}

// CreateFence создаёт fence (по умолчанию — несигнальный).
func (a *API) CreateFence(dev Device) (Fence, error) {
	info := FenceCreateInfo{Type: StructureTypeFenceCreateInfo}
	var f Fence
	if err := check("vkCreateFence", a.vkCreateFence(dev, &info, nil, &f)); err != nil {
		return 0, err
	}
	return f, nil
}

// DestroyFence освобождает fence.
func (a *API) DestroyFence(dev Device, f Fence) {
	if f != 0 {
		a.vkDestroyFence(dev, f, nil)
	}
}

// CmdImageBarrier записывает барьер памяти картины в командный буфер.
func (a *API) CmdImageBarrier(cmd CommandBuffer, b *ImageMemoryBarrier, srcStage, dstStage uint32) {
	a.vkCmdPipelineBarrier(cmd, srcStage, dstStage, 0, 0, nil, 0, nil, 1, b)
}

// --- Шейдерные модули ---

// SPIR-V должен быть выровнен по 4 байта: байты переводим в uint32.
func spirvWords(code []byte) []uint32 {
	if len(code)%4 != 0 {
		return nil
	}
	words := make([]uint32, len(code)/4)
	for i := range words {
		words[i] = uint32(code[4*i]) |
			uint32(code[4*i+1])<<8 |
			uint32(code[4*i+2])<<16 |
			uint32(code[4*i+3])<<24
	}
	return words
}

// CreateShaderModule создаёт шейдерный модуль из SPIR-V байтов.
func (a *API) CreateShaderModule(dev Device, spirv []byte) (ShaderModule, error) {
	words := spirvWords(spirv)
	if len(words) == 0 {
		return 0, &Error{Op: "vkCreateShaderModule", Code: ResultErrorValidationFailedEXT}
	}
	info := ShaderModuleCreateInfo{
		Type:     StructureTypeShaderModuleCreateInfo,
		CodeSize: uintptr(len(words)) * 4,
		Code:     &words[0],
	}
	var mod ShaderModule
	if err := check("vkCreateShaderModule", a.vkCreateShaderModule(dev, &info, nil, &mod)); err != nil {
		return 0, err
	}
	return mod, nil
}

// DestroyShaderModule освобождает модуль (после создания пайплайна можно и раньше).
func (a *API) DestroyShaderModule(dev Device, mod ShaderModule) {
	if mod != 0 {
		a.vkDestroyShaderModule(dev, mod, nil)
	}
}

// --- Пайплайн ---

// CreatePipelineLayout создаёт layout пайплайна (наборы дескрипторов + push-константы).
func (a *API) CreatePipelineLayout(dev Device, setLayouts []DescriptorSetLayout, push []PushConstantRange) (PipelineLayout, error) {
	info := PipelineLayoutCreateInfo{
		Type: StructureTypePipelineLayoutCreateInfo,
	}
	if len(setLayouts) > 0 {
		info.SetLayoutCount = uint32(len(setLayouts))
		info.SetLayouts = &setLayouts[0]
	}
	if len(push) > 0 {
		info.PushConstantRangeCount = uint32(len(push))
		info.PushConstantRanges = &push[0]
	}
	var layout PipelineLayout
	if err := check("vkCreatePipelineLayout", a.vkCreatePipelineLayout(dev, &info, nil, &layout)); err != nil {
		return 0, err
	}
	return layout, nil
}

// DestroyPipelineLayout освобождает layout.
func (a *API) DestroyPipelineLayout(dev Device, l PipelineLayout) {
	if l != 0 {
		a.vkDestroyPipelineLayout(dev, l, nil)
	}
}

// CreateGraphicsPipeline создаёт графический пайплайн (один create-info).
func (a *API) CreateGraphicsPipeline(dev Device, info GraphicsPipelineCreateInfo) (Pipeline, error) {
	var p Pipeline
	if err := check("vkCreateGraphicsPipelines",
		a.vkCreateGraphicsPipelines(dev, 0, 1, &info, nil, &p)); err != nil {
		return 0, err
	}
	return p, nil
}

// DestroyPipeline освобождает пайплайн.
func (a *API) DestroyPipeline(dev Device, p Pipeline) {
	if p != 0 {
		a.vkDestroyPipeline(dev, p, nil)
	}
}

// --- Дескрипторы и сэмплер ---

// CreateDescriptorSetLayout создаёт layout набора (для UI — один биндинг: атлас).
func (a *API) CreateDescriptorSetLayout(dev Device, bindings []DescriptorSetLayoutBinding) (DescriptorSetLayout, error) {
	info := DescriptorSetLayoutCreateInfo{
		Type: StructureTypeDescriptorSetLayoutCreateInfo,
	}
	if len(bindings) > 0 {
		info.BindingCount = uint32(len(bindings))
		info.Bindings = &bindings[0]
	}
	var layout DescriptorSetLayout
	if err := check("vkCreateDescriptorSetLayout",
		a.vkCreateDescriptorSetLayout(dev, &info, nil, &layout)); err != nil {
		return 0, err
	}
	return layout, nil
}

// DestroyDescriptorSetLayout освобождает layout набора.
func (a *API) DestroyDescriptorSetLayout(dev Device, l DescriptorSetLayout) {
	if l != 0 {
		a.vkDestroyDescriptorSetLayout(dev, l, nil)
	}
}

// CreateDescriptorPool создаёт пул дескрипторов.
func (a *API) CreateDescriptorPool(dev Device, sizes []DescriptorPoolSize, maxSets uint32) (DescriptorPool, error) {
	info := DescriptorPoolCreateInfo{
		Type:    StructureTypeDescriptorPoolCreateInfo,
		MaxSets: maxSets,
	}
	if len(sizes) > 0 {
		info.PoolSizeCount = uint32(len(sizes))
		info.PoolSizes = &sizes[0]
	}
	var pool DescriptorPool
	if err := check("vkCreateDescriptorPool", a.vkCreateDescriptorPool(dev, &info, nil, &pool)); err != nil {
		return 0, err
	}
	return pool, nil
}

// DestroyDescriptorPool освобождает пул (вместе с наборами из него).
func (a *API) DestroyDescriptorPool(dev Device, p DescriptorPool) {
	if p != 0 {
		a.vkDestroyDescriptorPool(dev, p, nil)
	}
}

// AllocateDescriptorSet выделяет набор дескрипторов.
func (a *API) AllocateDescriptorSet(dev Device, pool DescriptorPool, layout DescriptorSetLayout) (DescriptorSet, error) {
	info := DescriptorSetAllocateInfo{
		Type:               StructureTypeDescriptorSetAllocateInfo,
		DescriptorPool:     pool,
		DescriptorSetCount: 1,
		SetLayouts:         &layout,
	}
	var set DescriptorSet
	if err := check("vkAllocateDescriptorSets", a.vkAllocateDescriptorSets(dev, &info, &set)); err != nil {
		return 0, err
	}
	return set, nil
}

// UpdateDescriptorSets применяет записи (wrapper над vkUpdateDescriptorSets).
func (a *API) UpdateDescriptorSets(dev Device, writes []WriteDescriptorSet) {
	if len(writes) == 0 {
		return
	}
	a.vkUpdateDescriptorSets(dev, uint32(len(writes)), &writes[0], 0, nil)
}

// WriteDescriptorImage собирает WriteDescriptorSet для картин-дескриптора.
func WriteDescriptorImage(set DescriptorSet, binding uint32, sampler Sampler, view ImageView, layout ImageLayout) WriteDescriptorSet {
	return WriteDescriptorSet{
		Type:            StructureTypeWriteDescriptorSet,
		DstSet:          set,
		DstBinding:      binding,
		DescriptorCount: 1,
		DescriptorType:  DescriptorTypeCombinedImageSampler,
		ImageInfo: &DescriptorImageInfo{
			Sampler:     sampler,
			ImageView:   view,
			ImageLayout: layout,
		},
	}
}

// CreateSampler создаёт сэмплер.
func (a *API) CreateSampler(dev Device, info SamplerCreateInfo) (Sampler, error) {
	info.Type = StructureTypeSamplerCreateInfo
	var s Sampler
	if err := check("vkCreateSampler", a.vkCreateSampler(dev, &info, nil, &s)); err != nil {
		return 0, err
	}
	return s, nil
}

// DefaultSampler — линейный сэмплер без повторов: стандарт для текстовых атласов.
func (a *API) DefaultSampler(dev Device) (Sampler, error) {
	return a.CreateSampler(dev, SamplerCreateInfo{
		MagFilter:    FilterLinear,
		MinFilter:    FilterLinear,
		MipmapMode:   SamplerMipmapModeLinear,
		AddressModeU: SamplerAddressModeClampToEdge,
		AddressModeV: SamplerAddressModeClampToEdge,
		AddressModeW: SamplerAddressModeClampToEdge,
		MaxLod:       1.0,
	})
}

// DestroySampler освобождает сэмплер.
func (a *API) DestroySampler(dev Device, s Sampler) {
	if s != 0 {
		a.vkDestroySampler(dev, s, nil)
	}
}

// DestroyImageView освобождает view картины.
func (a *API) DestroyImageView(dev Device, v ImageView) {
	if v != 0 {
		a.vkDestroyImageView(dev, v, nil)
	}
}

// --- Команды рендера UI ---

// CmdBindPipeline привязывает графический пайплайн.
func (a *API) CmdBindPipeline(cmd CommandBuffer, p Pipeline) {
	a.vkCmdBindPipeline(cmd, PipelineBindPointGraphics, p)
}

// CmdBindVertexBuffer привязывает один вершинный буфер.
func (a *API) CmdBindVertexBuffer(cmd CommandBuffer, b Buffer, offset uint64) {
	a.vkCmdBindVertexBuffers(cmd, 0, 1, &b, &offset)
}

// CmdBindIndexBuffer привязывает индексный буфер.
func (a *API) CmdBindIndexBuffer(cmd CommandBuffer, b Buffer, offset uint64, indexType IndexType) {
	a.vkCmdBindIndexBuffer(cmd, b, offset, indexType)
}

// CmdBindDescriptorSet привязывает набор дескрипторов (набор 0).
func (a *API) CmdBindDescriptorSet(cmd CommandBuffer, layout PipelineLayout, set DescriptorSet) {
	a.vkCmdBindDescriptorSets(cmd, PipelineBindPointGraphics, layout, 0, 1, &set, 0, nil)
}

// CmdPushConstants передаёт push-константы (например, rect в пикселях).
func (a *API) CmdPushConstants(cmd CommandBuffer, layout PipelineLayout, stages uint32, data unsafe.Pointer, size uint32) {
	a.vkCmdPushConstants(cmd, layout, stages, 0, size, data)
}

// CmdSetViewport задаёт вьюпорт (пиксели framebuffer, Y вниз).
func (a *API) CmdSetViewport(cmd CommandBuffer, w, h float32) {
	vp := Viewport{X: 0, Y: 0, Width: w, Height: h, MinDepth: 0, MaxDepth: 1}
	a.vkCmdSetViewport(cmd, 0, 1, &vp)
}

// CmdSetScissor задаёт ножницы (по умолчанию — весь кадр).
func (a *API) CmdSetScissor(cmd CommandBuffer, w, h uint32) {
	sc := Rect2D{Extent: Extent2D{Width: w, Height: h}}
	a.vkCmdSetScissor(cmd, 0, 1, &sc)
}

// CmdDraw рисует vertexCount вершин (1 instance).
func (a *API) CmdDraw(cmd CommandBuffer, vertexCount uint32) {
	a.vkCmdDraw(cmd, vertexCount, 1, 0, 0)
}

// CmdDrawIndexed рисует indexCount индексов (1 instance).
func (a *API) CmdDrawIndexed(cmd CommandBuffer, indexCount uint32) {
	a.vkCmdDrawIndexed(cmd, indexCount, 1, 0, 0, 0)
}
