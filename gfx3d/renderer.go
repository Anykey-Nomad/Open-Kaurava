package gfx3d

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"math"
	"path"
	"runtime"
	"strings"
	"unsafe"

	"dow-mod-studio/imgdec"
	"dow-mod-studio/modapi"
	"dow-mod-studio/sgbview"
	"github.com/Anykey-Nomad/Open-Kaurava/fx"
	"github.com/Anykey-Nomad/Open-Kaurava/internal/shaders"
	"github.com/Anykey-Nomad/Open-Kaurava/vulkan"
)

const vertexStride = 64 // Pos(12) + Normal(12) + UV(8) + Bones(16) + Weights(16)

// fxStride — вершина частицы: Pos(12) + UV(8) + Color(16).
const fxStride = 36

// fxMaxQuads — предел квадов эффектов в кадре: 8192 по четыре вершины.
//
// Один эффект из DATA:ART/FX — это не пара спрайтов: вспышка тяжёлого
// болтера отдаёт в кадр под пять сотен квадов (ленты и кольца разбиты на
// сегменты), а стреляет целый отряд. С прежними 2048 вспышка одного бойца
// съедала четверть бюджета, и хвостовые группы карт просто не рисовались.
const fxMaxQuads = 8192

// fxSpriteSize — размер запасного спрайта частицы: встроенный мягкий круг
// с прозрачным краем. Он рисуется только там, где у эффекта нет своей
// карты в DATA:ART/FX (или файл не нашёлся).
const fxSpriteSize = 32

// paletteReserve — сколько костей палитры остаётся в запасе под объекты,
// которые появляются по ходу игры (постройки). Без запаса добавление
// объекта требовало бы пересоздания палитры и всех дескриптор-сетов сцены.
const paletteReserve = 1024

// pushData — push-константы 3D-шейдеров (112 байт).
type pushData struct {
	ViewProj [16]float32
	Tint     [4]float32
	Params   [4]float32 // x = alphaRef, y = ambient, z = diffuse, w = unused
	// BoneBase — база костей объекта в палитре кадра: в шейдере это
	// ivec4 info (offset 96), x — база. Три поля padding — чтобы
	// структура была ровно 112 байт, как раскладка push-констант GLSL.
	BoneBase int32
	pad      [3]int32
}

// animEntry — анимируемый объект сцены: его клип и текущий сдвиг
// от места появления (геометрия остаётся в точке спавна, сдвиг
// накладывается матрицей в палитре — WritePalette).
type animEntry struct {
	anim *sgbview.Anim
	move [3]float64
}

type gpuTexture struct {
	img  vulkan.Image
	mem  vulkan.DeviceMemory
	view vulkan.ImageView
	set  vulkan.DescriptorSet
}

// fxBatch — группа частиц с одной текстурой: first — первый индекс в
// общем индексном буфере, count — сколько индексов нарисовать.
type fxBatch struct {
	tex   *gpuTexture
	first uint32
	count uint32
	// additive — батч рисуется сложением: вспышки, трассеры, свечение
	// (Blending=2 в файле эффекта).
	additive bool
}

type gpuMesh struct {
	vbuf        vulkan.BufferMemory
	ibuf        vulkan.BufferMemory
	count       uint32
	tex         *gpuTexture
	tint        [4]float32
	alphaRef    float32
	translucent bool

	// name — имя меша (оно же у всех частей одного объекта): по нему
	// объект находится для сдвига при движении.
	name string
	// boneBase — база костей объекта в палитре кадра; для статики 0
	// (веса нулевые, шейдер палитру не читает).
	boneBase int32
	// base — копия исходных вершин и центр геометрии. Заполняется только
	// для мешей, помеченных SetDynamicObjects: сдвиг переписывает буфер
	// из этой копии, поэтому память дублируется только под движущиеся
	// объекты, а не под всю сцену.
	base       []byte
	baseCenter [3]float32
	// hidden — меш не рисуется: так убираются тела уничтоженных и
	// «спящие» постройки, чьё место занято пробуждённым юнитом.
	hidden bool
	// alpha — прозрачность меша (1 — обычная): по ней трупы плавно
	// исчезают. Меш, который и так рисовался полупрозрачным (вода),
	// остаётся таким при любой альфе.
	alpha float32
	// part — имя меша модели («guard_banner»), partHidden — часть скрыта
	// отдельно от объекта: так на точке захвата виден только флаг
	// захватившей расы, а остальные лежат в видеопамяти.
	part       string
	partHidden bool
}

// Renderer — Vulkan-рендерер 3D-сцены сценария.
type Renderer struct {
	api    *vulkan.API
	dev    vulkan.Device
	swap   *vulkan.Swapchain
	queue  vulkan.Queue
	family uint32

	cmdPool    vulkan.CommandPool
	setLayout  vulkan.DescriptorSetLayout
	pipeLayout vulkan.PipelineLayout
	sampler    vulkan.Sampler
	opaquePipe vulkan.Pipeline
	blendPipe  vulkan.Pipeline
	// fadePipe — тела, исчезающие по альфе: смешивание с записью глубины,
	// иначе труп, нарисованный раньше стоящей впереди постройки, был бы
	// виден поверх неё.
	fadePipe vulkan.Pipeline

	// FX — частицы: два пайплайна смешивания (по альфе и сложением),
	// запасной спрайт и общие буферы, которые переписываются каждый кадр.
	fxPipe    vulkan.Pipeline
	fxAddPipe vulkan.Pipeline
	fxTexture *gpuTexture
	fxVB      vulkan.BufferMemory
	fxIB      vulkan.BufferMemory
	fxVerts   []byte
	fxQuads   uint32
	// fxBatches — частицы кадра, разложенные по текстурам эффекта: каждый
	// батч рисуется своим дескриптор-сетом и диапазоном индексов.
	fxBatches []fxBatch
	// fxTree — дерево модуля для поиска карт эффектов (Data/art/fx/...).
	// Без него (или если карта не нашлась) остаётся встроенный круг.
	fxTree     *modapi.ModTree
	fxTextures map[string]*gpuTexture
	fxMissing  map[string]bool

	descPool vulkan.DescriptorPool
	// dynamic — имена мешей, которые могут двигаться: их вершины
	// копируются при загрузке (см. SetDynamicObjects, MoveObject).
	dynamic  map[string]bool
	whiteTex *gpuTexture
	textures map[string]*gpuTexture
	meshes   []gpuMesh

	// палитра костей анимируемых объектов: общий на кадр storage-буфер
	// и его CPU-копия, которую переписывает StepAnimations.
	palette    vulkan.BufferMemory
	paletteCPU []float32
	// anims — анимируемые объекты по имени меша (все части одного
	// объекта носят одно имя): сюда приходят приказы и сдвиги.
	anims map[string]*animEntry
	// boneNext — сколько костей палитры уже занято сценой и добавленными
	// по ходу игры объектами.
	boneNext int

	// texFormat — формат картинок сцены. Если кадр в sRGB, то и текстуры
	// читаются как sRGB: иначе цвет «выцветает» (значения sRGB считаются
	// линейными и снова кодируются при выводе).
	texFormat vulkan.Format
}

// NewRenderer создаёт 3D-рендерер.
func NewRenderer(api *vulkan.API, dev vulkan.Device, swap *vulkan.Swapchain, queue vulkan.Queue, family uint32) (*Renderer, error) {
	r := &Renderer{
		api:        api,
		dev:        dev,
		swap:       swap,
		queue:      queue,
		family:     family,
		textures:   map[string]*gpuTexture{},
		fxTextures: map[string]*gpuTexture{},
		fxMissing:  map[string]bool{},
		texFormat:  vulkan.FormatR8G8B8A8UNORM,
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
	// Палитра костей — storage-буфер, читается вершинным шейдером скиннинга.
	boneBinding := vulkan.DescriptorSetLayoutBinding{
		Binding:         1,
		DescriptorType:  vulkan.DescriptorTypeStorageBuffer,
		DescriptorCount: 1,
		StageFlags:      vulkan.ShaderStageVertexBit,
	}
	setLayout, err := api.CreateDescriptorSetLayout(dev, []vulkan.DescriptorSetLayoutBinding{binding, boneBinding})
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
		{Location: 3, Binding: 0, Format: vulkan.FormatR32G32B32ASInt, Offset: 32},
		{Location: 4, Binding: 0, Format: vulkan.FormatR32G32B32ASFloat, Offset: 48},
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

	// Пайплайн исчезающих тел: альфа-смешивание с записью глубины. Общий
	// полупрозрачный пайплайн (вода) глубину не пишет, и труп, нарисованный
	// раньше стоящего впереди здания, оказался бы поверх него.
	depthFade := depthBlend
	depthFade.DepthWriteEnable = 1
	info.DepthStencilState = unsafe.Pointer(&depthFade)
	info.ColorBlendState = &colorBlendAlpha
	fadePipe, fadeErr := a.CreateGraphicsPipeline(r.dev, info)
	runtime.KeepAlive(name)
	runtime.KeepAlive(depthFade)
	if fadeErr != nil {
		// Без своего пайплайна трупы исчезают через полупрозрачный: хуже
		// по порядку отрисовки, но сцена работает.
		r.fadePipe = 0
	} else {
		r.fadePipe = fadePipe
	}
	// Пайплайн частиц не критичен: если он не создался, сцена рисуется без
	// эффектов, а не отказывает вовсе. Аддитивный — надстройка над обычным:
	// без него вспышки идут альфой, но не пропадают.
	if pipe, err := r.createFXPipeline(false); err == nil {
		r.fxPipe = pipe
	} else {
		r.fxPipe = 0
	}
	if pipe, err := r.createFXPipeline(true); err == nil {
		r.fxAddPipe = pipe
	} else {
		r.fxAddPipe = 0
	}
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
	if r.fadePipe != 0 {
		a.DestroyPipeline(r.dev, r.fadePipe)
		r.fadePipe = 0
	}
	if r.fxPipe != 0 {
		a.DestroyPipeline(r.dev, r.fxPipe)
		r.fxPipe = 0
	}
	if r.fxAddPipe != 0 {
		a.DestroyPipeline(r.dev, r.fxAddPipe)
		r.fxAddPipe = 0
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
	if r.fxTexture != nil {
		r.destroyTexture(r.fxTexture)
		r.fxTexture = nil
	}
	for _, t := range r.textures {
		r.destroyTexture(t)
	}
	r.textures = map[string]*gpuTexture{}
	for _, t := range r.fxTextures {
		r.destroyTexture(t)
	}
	r.fxTextures = map[string]*gpuTexture{}
	r.fxMissing = map[string]bool{}
	if r.descPool != 0 {
		a.DestroyDescriptorPool(r.dev, r.descPool)
		r.descPool = 0
	}

	pool, err := a.CreateDescriptorPool(r.dev, []vulkan.DescriptorPoolSize{
		{Type: vulkan.DescriptorTypeCombinedImageSampler, Count: maxSets},
		{Type: vulkan.DescriptorTypeStorageBuffer, Count: maxSets},
	}, maxSets)
	if err != nil {
		return fmt.Errorf("gfx3d: пул дескрипторов: %w", err)
	}
	r.descPool = pool

	// Палитра костей нужна уже белой текстуре: дескриптор-сет пишет оба
	// биндинга. Сцена позже пересоздаст её под реальное число костей.
	if r.palette.Buffer == 0 {
		if err := r.createPalette(1); err != nil {
			return err
		}
	}

	white, err := r.uploadTexture(1, 1, []byte{0xff, 0xff, 0xff, 0xff})
	if err != nil {
		return fmt.Errorf("gfx3d: белая текстура: %w", err)
	}
	r.whiteTex = white
	// Спрайт частицы тоже не критичен: без него FX просто не рисуются.
	if particle, err := r.uploadTexture(fxSpriteSize, fxSpriteSize, fxSprite(fxSpriteSize)); err == nil {
		r.fxTexture = particle
	}
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
	if r.palette.Buffer != 0 {
		r.api.DestroyBuffer(r.dev, &r.palette)
		r.palette = vulkan.BufferMemory{}
	}
	r.paletteCPU = nil
	r.anims = nil
}

// SetVisible показывает или скрывает меш объекта. Скрытый меш остаётся в
// видеопамяти: так пробуждённый монолит убирает свою постройку и
// возвращает её на место, не перезагружая сцену.
func (r *Renderer) SetVisible(name string, visible bool) bool {
	if r == nil || name == "" {
		return false
	}
	found := false
	for i := range r.meshes {
		if r.meshes[i].name != name {
			continue
		}
		r.meshes[i].hidden = !visible
		found = true
	}
	return found
}

// SetObjectAlpha задаёт прозрачность мешей объекта: 1 — обычная, 0 —
// невидим. Так трупы гаснут плавно, а не пропадают за один кадр. false —
// объекта нет; мёртвому телу, каким его оставила геометрия сцены, тоже
// можно задать альфу — меш ищется по имени, без привязки к анимации.
func (r *Renderer) SetObjectAlpha(name string, alpha float32) bool {
	if r == nil || name == "" {
		return false
	}
	if alpha < 0 {
		alpha = 0
	}
	if alpha > 1 {
		alpha = 1
	}
	found := false
	for i := range r.meshes {
		if r.meshes[i].name != name {
			continue
		}
		r.meshes[i].alpha = alpha
		found = true
	}
	return found
}

// SetObjectFrames ставит объект на кадры клипов, заданных извне: так точка
// захвата связывает подъём флага с процентом захвата. false — объекта нет,
// у него нет анимации или ни одного из клипов.
func (r *Renderer) SetObjectFrames(name string, frames []sgbview.ClipFrame) bool {
	if r == nil || name == "" {
		return false
	}
	entry := r.anims[name]
	if entry == nil || entry.anim == nil {
		return false
	}
	return entry.anim.SetClipFrames(frames)
}

// HasObjectClip сообщает, есть ли у объекта клип с таким именем: по нему
// точка захвата выбирает, как назван клип у её модели (у реликвий — с
// приставкой ol_).
func (r *Renderer) HasObjectClip(name, clip string) bool {
	if r == nil || name == "" || clip == "" {
		return false
	}
	entry := r.anims[name]
	return entry != nil && entry.anim != nil && entry.anim.HasClip(clip)
}

// ObjectClipDuration — длительность клипа объекта в секундах (0 — клипа
// нет): по ней точка захвата проигрывает uncaptured_open до конца.
func (r *Renderer) ObjectClipDuration(name, clip string) float64 {
	if r == nil || name == "" || clip == "" {
		return 0
	}
	entry := r.anims[name]
	if entry == nil || entry.anim == nil {
		return 0
	}
	return entry.anim.ClipDuration(clip)
}

// SetObjectParts показывает части объекта по карте видимости (ключи — имена
// мешей модели в нижнем регистре): всё, чего в карте нет, скрывается. Так
// на точке захвата виден только флаг захватившей расы. nil — снова видно всё.
func (r *Renderer) SetObjectParts(name string, visible map[string]bool) bool {
	if r == nil || name == "" {
		return false
	}
	found := false
	for i := range r.meshes {
		m := &r.meshes[i]
		if m.name != name || m.part == "" {
			continue
		}
		found = true
		m.partHidden = visible != nil && !visible[strings.ToLower(m.part)]
	}
	return found
}

// ShowObjectParts показывает части объекта по набору клипов: видимость
// берётся из треков vis_* модели — так на точке захвата остов остаётся, а
// флаг виден только у захватившей расы.
func (r *Renderer) ShowObjectParts(name string, clips []string) bool {
	if r == nil || name == "" || len(clips) == 0 {
		return false
	}
	entry := r.anims[name]
	if entry == nil || entry.anim == nil {
		return false
	}
	visible := entry.anim.VisibleWith(clips...)
	if visible == nil {
		return false
	}
	return r.SetObjectParts(name, visible)
}

// SetDynamicObjects помечает имена мешей объектов, которые могут
// двигаться по карте. Их вершины копируются при загрузке сцены, чтобы
// MoveObject мог сдвигать меш без повторной загрузки всей сцены.
//
// Вызывается до UploadScene; отметка переживает перезагрузку сцены.
func (r *Renderer) SetDynamicObjects(names map[string]bool) {
	if r == nil {
		return
	}
	r.dynamic = names
}

// MoveObject сдвигает все мешы объекта так, чтобы его геометрический центр
// оказался в center (мировые координаты, Z вверх). Работает только для
// мешей, помеченных SetDynamicObjects; для остальных — тихое бездействие.
func (r *Renderer) MoveObject(name string, center [3]float32) error {
	if r == nil || name == "" {
		return nil
	}
	a := r.api
	for i := range r.meshes {
		m := &r.meshes[i]
		if m.name != name || m.base == nil {
			continue
		}
		dx := center[0] - m.baseCenter[0]
		dy := center[1] - m.baseCenter[1]
		dz := center[2] - m.baseCenter[2]
		buf := append([]byte(nil), m.base...)
		for off := 0; off+12 <= len(buf); off += vertexStride {
			for axis, delta := range [3]float32{dx, dy, dz} {
				bits := binary.LittleEndian.Uint32(buf[off+axis*4:])
				value := math.Float32frombits(bits) + delta
				binary.LittleEndian.PutUint32(buf[off+axis*4:], math.Float32bits(value))
			}
		}
		if err := a.WriteBuffer(r.dev, m.vbuf, buf); err != nil {
			return fmt.Errorf("gfx3d: сдвиг меша %s: %w", name, err)
		}
	}
	return nil
}

// UploadScene загружает собранную сцену сценария в память GPU.
func (r *Renderer) UploadScene(sc *sgbview.Scene) error {
	r.ClearScene()
	if sc == nil {
		return nil
	}

	// Палитра костей: база каждого анимируемого объекта и её размер.
	// Геометрия объекта лежит в своём мире, поэтому индексы костей в
	// вершинах локальны — база общая только для палитры и шейдера.
	bones := 0
	r.anims = map[string]*animEntry{}
	for _, obj := range sc.Objects {
		if obj.Anim == nil {
			continue
		}
		obj.Anim.SetBase(bones)
		for _, p := range obj.Parts {
			if p.Mesh != nil && p.Mesh.Name != "" {
				r.anims[p.Mesh.Name] = &animEntry{anim: obj.Anim}
				break
			}
		}
		bones += obj.Anim.BoneCount()
	}
	r.boneNext = bones
	if err := r.createPalette(bones + paletteReserve); err != nil {
		return err
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
	// Запас с лихвой: постройка, добавленная по ходу игры, приносит до трёх
	// десятков своих текстур (у штаба их 28), и каждая занимает свой
	// дескриптор-сет — без запаса она ушла бы в белую текстуру.
	if err := r.resetDescriptorPool(uint32(len(uniqueTex) + 128)); err != nil {
		return err
	}

	getTex := func(t *sgbview.Texture) *gpuTexture { return r.textureFor(t) }

	white := [4]float32{1, 1, 1, 1}

	// 1. Рельеф.
	if sc.Terrain != nil {
		tex := getTex(sc.TerrainTexture)
		if err := r.addMesh(sc.Terrain, tex, white, 0.0, false, 0); err != nil {
			return err
		}
	}

	// 2. Объекты карты.
	for _, obj := range sc.Objects {
		var base int32
		if obj.Anim != nil {
			base = int32(obj.Anim.Base())
		}
		for _, p := range obj.Parts {
			if p.Mesh == nil {
				continue
			}
			tex := getTex(p.Texture)
			if err := r.addMesh(p.Mesh, tex, white, 0.5, false, base); err != nil {
				return err
			}
		}
	}

	// 3. Вода (рисуется последней с альфа-смешиванием).
	if sc.Water != nil {
		wc := sgbview.WaterColor
		// Вода полупрозрачная: сценарий хранит её альфу 255 (непрозрачно),
		// поэтому оттенок берём из данных, а прозрачность задаём сами —
		// сквозь воду должен быть виден рельеф. Меньшая альфа в данных
		// (если мод её задал) сохраняется.
		const waterAlpha = 0.55
		alpha := float32(wc[3]) / 255
		if alpha <= 0 || alpha > waterAlpha {
			alpha = waterAlpha
		}
		waterTint := [4]float32{
			float32(wc[0]) / 255,
			float32(wc[1]) / 255,
			float32(wc[2]) / 255,
			alpha,
		}
		if err := r.addMesh(sc.Water, r.whiteTex, waterTint, 0.0, true, 0); err != nil {
			return err
		}
	}

	return nil
}

// createFXPipeline — пайплайн частиц: та же раскладка push-констант и тот
// же лейаут, что у мешей, но вершина — точка, UV и цвет. Глубина
// проверяется без записи: частицы рисуются после воды.
//
// additive=false — обычное смешивание по альфе (дым, пыль, копоть).
// additive=true — сложение: так велят эффекты с Blending=2 (вспышки,
// трассеры, плазма, молнии) — их цвет добавляется к кадру и светится.
func (r *Renderer) createFXPipeline(additive bool) (vulkan.Pipeline, error) {
	a := r.api
	vert, err := a.CreateShaderModule(r.dev, shaders.FXVert)
	if err != nil {
		return 0, fmt.Errorf("gfx3d: вершинный шейдер частиц: %w", err)
	}
	defer a.DestroyShaderModule(r.dev, vert)
	frag, err := a.CreateShaderModule(r.dev, shaders.FXFrag)
	if err != nil {
		return 0, fmt.Errorf("gfx3d: фрагментный шейдер частиц: %w", err)
	}
	defer a.DestroyShaderModule(r.dev, frag)

	name := append([]byte("main"), 0)
	stages := []vulkan.PipelineShaderStageCreateInfo{
		{Type: vulkan.StructureTypePipelineShaderStageCreateInfo, Stage: vulkan.ShaderStageFlags(vulkan.ShaderStageVertexBit), Module: vert, Name: &name[0]},
		{Type: vulkan.StructureTypePipelineShaderStageCreateInfo, Stage: vulkan.ShaderStageFlags(vulkan.ShaderStageFragmentBit), Module: frag, Name: &name[0]},
	}
	binding := vulkan.VertexInputBindingDescription{
		Binding:   0,
		Stride:    fxStride,
		InputRate: vulkan.VertexInputRateVertex,
	}
	attrs := []vulkan.VertexInputAttributeDescription{
		{Location: 0, Binding: 0, Format: vulkan.FormatR32G32B32SFloat, Offset: 0},
		{Location: 1, Binding: 0, Format: vulkan.FormatR32G32SFloat, Offset: 12},
		{Location: 2, Binding: 0, Format: vulkan.FormatR32G32B32ASFloat, Offset: 20},
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
		ViewportCount: 1, Viewports: &vp,
		ScissorCount: 1, Scissors: &sc,
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
	depth := vulkan.PipelineDepthStencilStateCreateInfo{
		Type:             vulkan.StructureTypePipelineDepthStencilStateCreateInfo,
		DepthTestEnable:  1,
		DepthWriteEnable: 0,
		DepthCompareOp:   vulkan.CompareOpLessOrEqual,
	}
	blend := vulkan.PipelineColorBlendAttachmentState{
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
	if additive {
		// Сложение: цвет спрайта добавляется к кадру, не затирая фон. Альфу
		// тоже складываем — искра не гасит то, что за ней.
		blend.DstColorBlendFactor = vulkan.BlendFactorOne
		blend.SrcAlphaBlendFactor = vulkan.BlendFactorOne
		blend.DstAlphaBlendFactor = vulkan.BlendFactorOne
	}
	colorBlend := vulkan.PipelineColorBlendStateCreateInfo{
		Type:            vulkan.StructureTypePipelineColorBlendStateCreateInfo,
		AttachmentCount: 1,
		Attachments:     &blend,
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
		DepthStencilState:  unsafe.Pointer(&depth),
		ColorBlendState:    &colorBlend,
		DynamicState:       &dynamic,
		Layout:             r.pipeLayout,
		RenderPass:         r.swap.RenderPass,
		Subpass:            0,
		BasePipelineIndex:  -1,
	}
	pipe, err := a.CreateGraphicsPipeline(r.dev, info)
	runtime.KeepAlive(name)
	runtime.KeepAlive(depth)
	if err != nil {
		return 0, fmt.Errorf("gfx3d: пайплайн частиц: %w", err)
	}
	return pipe, nil
}

// fxSprite — мягкий круг с прозрачным краем: им рисуются все частицы, а
// цвет и размер даёт вершина.
func fxSprite(size int) []byte {
	pixels := make([]byte, size*size*4)
	center := float64(size-1) / 2
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			distance := math.Hypot((float64(x)-center)/center, (float64(y)-center)/center)
			alpha := 0.0
			if distance < 1 {
				alpha = math.Pow(1-distance, 1.5)
			}
			offset := (y*size + x) * 4
			pixels[offset] = 255
			pixels[offset+1] = 255
			pixels[offset+2] = 255
			pixels[offset+3] = byte(alpha * 255)
		}
	}
	return pixels
}

// createFXBuffers создаёт буферы частиц один раз: вершины переписываются
// каждый кадр, индексы постоянны (два треугольника на квад).
func (r *Renderer) createFXBuffers() error {
	a := r.api
	vbuf, err := a.CreateBuffer(r.dev, uint64(fxMaxQuads*4*fxStride),
		vulkan.BufferUsageVertexBufferBit,
		vulkan.MemoryPropertyHostVisibleBit|vulkan.MemoryPropertyHostCoherentBit)
	if err != nil {
		return fmt.Errorf("gfx3d: буфер частиц: %w", err)
	}
	indices := make([]uint32, 0, fxMaxQuads*6)
	for quad := uint32(0); quad < fxMaxQuads; quad++ {
		base := quad * 4
		indices = append(indices, base, base+1, base+2, base, base+2, base+3)
	}
	ibytes := unsafe.Slice((*byte)(unsafe.Pointer(&indices[0])), len(indices)*4)
	ibuf, err := a.CreateBuffer(r.dev, uint64(len(ibytes)),
		vulkan.BufferUsageIndexBufferBit,
		vulkan.MemoryPropertyHostVisibleBit|vulkan.MemoryPropertyHostCoherentBit)
	if err != nil {
		a.DestroyBuffer(r.dev, &vbuf)
		return fmt.Errorf("gfx3d: индексный буфер частиц: %w", err)
	}
	if err := a.WriteBuffer(r.dev, ibuf, ibytes); err != nil {
		a.DestroyBuffer(r.dev, &vbuf)
		a.DestroyBuffer(r.dev, &ibuf)
		return err
	}
	r.fxVB, r.fxIB = vbuf, ibuf
	r.fxVerts = make([]byte, 0, fxMaxQuads*4*fxStride)
	return nil
}

// SetParticles раскладывает частицы в квады, развёрнутые к камере, и
// переписывает их вершины. Вызывается перед Record каждый кадр; пустой
// список просто выключает отрисовку.
func (r *Renderer) SetParticles(particles []fx.Particle, eye [3]float64) {
	r.SetFX(particles, nil, eye)
}

// SetFX — весь эффектный слой кадра: частицы-билборды и готовые
// четырёхугольники лент/колец. И те и другие группируются по карте эффекта:
// частицы берут её из свойства Texture (DATA:ART/FX/...), ленты несут её на
// узле. Батчи рисуются одним буфером — по диапазону индексов на текстуру.
// Частицы без карты (старые искры и дым) остаются встроенным кругом.
func (r *Renderer) SetFX(particles []fx.Particle, quads []fx.Quad, eye [3]float64) {
	if r == nil {
		return
	}
	r.fxQuads = 0
	r.fxBatches = r.fxBatches[:0]
	if (len(particles) == 0 && len(quads) == 0) || r.fxPipe == 0 {
		return
	}
	if r.fxVB.Buffer == 0 {
		if err := r.createFXBuffers(); err != nil {
			return
		}
	}
	// Порядок первого появления текстуры сохраняется: кадр не мерцает от
	// перестановки батчей между кадрами. Ленты идут первыми — вспышка
	// должна читаться поверх луча, а не под ним.
	// Группа — одна текстура и один способ смешивания: аддитивные и
	// обычные частицы одной карты всё равно рисуются разными пайплайнами.
	type fxGroup struct {
		key      string
		additive bool
		quads    []fx.Quad
		items    []fx.Particle
	}
	textureKey := func(name string) string { return strings.ToLower(strings.TrimSpace(name)) }
	groups := make([]fxGroup, 0, 4)
	index := make(map[string]int, 4)
	group := func(name string, additive bool) *fxGroup {
		key := textureKey(name)
		if additive {
			key += "|add"
		}
		if at, ok := index[key]; ok {
			return &groups[at]
		}
		index[key] = len(groups)
		groups = append(groups, fxGroup{key: textureKey(name), additive: additive})
		return &groups[len(groups)-1]
	}
	for _, q := range quads {
		g := group(q.Texture, q.Additive)
		g.quads = append(g.quads, q)
	}
	for _, p := range particles {
		g := group(p.Texture, p.Additive)
		g.items = append(g.items, p)
	}

	frame := r.fxVerts[:0]
	count := uint32(0)
	for _, g := range groups {
		tex := r.fxTextureFor(g.key)
		if tex == nil {
			continue
		}
		first := count
		for _, q := range g.quads {
			if count >= fxMaxQuads {
				break
			}
			color := [4]float32{float32(q.R), float32(q.G), float32(q.B), float32(q.A)}
			for i := 0; i < 4; i++ {
				frame = appendF32(frame, float32(q.P[i].X))
				frame = appendF32(frame, float32(q.P[i].Y))
				frame = appendF32(frame, float32(q.P[i].Z))
				frame = appendF32(frame, float32(q.UV[i][0]))
				frame = appendF32(frame, float32(q.UV[i][1]))
				frame = appendF32(frame, color[0])
				frame = appendF32(frame, color[1])
				frame = appendF32(frame, color[2])
				frame = appendF32(frame, color[3])
			}
			count++
		}
		for _, p := range g.items {
			if count >= fxMaxQuads {
				break
			}
			size := p.SizeAt() * 0.5
			if size <= 0 {
				continue
			}
			toEye := [3]float64{eye[0] - p.Pos[0], eye[1] - p.Pos[1], eye[2] - p.Pos[2]}
			length := math.Sqrt(toEye[0]*toEye[0] + toEye[1]*toEye[1] + toEye[2]*toEye[2])
			if length < 1e-6 {
				continue
			}
			forward := [3]float64{toEye[0] / length, toEye[1] / length, toEye[2] / length}
			// right = cross(up, forward), up = (0, 0, 1).
			right := [3]float64{-forward[1], forward[0], 0}
			rightLength := math.Hypot(right[0], right[1])
			if rightLength < 1e-6 {
				right = [3]float64{1, 0, 0}
				rightLength = 1
			}
			right = [3]float64{right[0] / rightLength, right[1] / rightLength, 0}
			up := [3]float64{
				forward[1]*right[2] - forward[2]*right[1],
				forward[2]*right[0] - forward[0]*right[2],
				forward[0]*right[1] - forward[1]*right[0],
			}
			color := p.ColorAt()
			corners := [4][2]float64{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}}
			uvs := [4][2]float32{{0, 1}, {1, 1}, {1, 0}, {0, 0}}
			for i := 0; i < 4; i++ {
				frame = appendF32(frame, float32(p.Pos[0]+right[0]*corners[i][0]*size+up[0]*corners[i][1]*size))
				frame = appendF32(frame, float32(p.Pos[1]+right[1]*corners[i][0]*size+up[1]*corners[i][1]*size))
				frame = appendF32(frame, float32(p.Pos[2]+right[2]*corners[i][0]*size+up[2]*corners[i][1]*size))
				frame = appendF32(frame, uvs[i][0])
				frame = appendF32(frame, uvs[i][1])
				frame = appendF32(frame, color[0])
				frame = appendF32(frame, color[1])
				frame = appendF32(frame, color[2])
				frame = appendF32(frame, color[3])
			}
			count++
		}
		if count > first {
			r.fxBatches = append(r.fxBatches, fxBatch{
				tex:      tex,
				first:    first * 6,
				count:    (count - first) * 6,
				additive: g.additive,
			})
		}
	}
	// Свечение рисуем последним: вспышка и трассер должны ложиться поверх
	// дыма и копоти, как бы ни легли группы по первому появлению карты.
	if len(r.fxBatches) > 1 {
		ordered := make([]fxBatch, 0, len(r.fxBatches))
		for _, batch := range r.fxBatches {
			if !batch.additive {
				ordered = append(ordered, batch)
			}
		}
		for _, batch := range r.fxBatches {
			if batch.additive {
				ordered = append(ordered, batch)
			}
		}
		r.fxBatches = append(r.fxBatches[:0], ordered...)
	}
	if count == 0 {
		return
	}
	if err := r.api.WriteBuffer(r.dev, r.fxVB, frame); err != nil {
		return
	}
	r.fxVerts = frame
	r.fxQuads = count
}

// DeathDuration — длина клипа смерти объекта: после неё обломки можно
// убирать. 0 — клипа нет (или объект не анимируется), и сцена решает, что
// показывать нечего.
func (r *Renderer) DeathDuration(name string) float64 {
	if r == nil || name == "" {
		return 0
	}
	entry := r.anims[name]
	if entry == nil || entry.anim == nil {
		return 0
	}
	return entry.anim.DeathDuration()
}

// ShotEffects — эффекты выстрела объекта по имени меша: их сцена играет из
// .whe, а не рисует процедурную вспышку.
func (r *Renderer) ShotEffects(name string) []sgbview.ShotFX {
	if r == nil || name == "" {
		return nil
	}
	entry := r.anims[name]
	if entry == nil || entry.anim == nil {
		return nil
	}
	return entry.anim.ShotFX()
}

// DeathEffects — эффекты гибели объекта по имени меша: их .whe модели
// привязывает к таймлайнам смерти (кровь, искры, вспышки). Сцена запускает
// их, когда сущность перестаёт быть живой.
func (r *Renderer) DeathEffects(name string) []sgbview.FXEvent {
	if r == nil || name == "" {
		return nil
	}
	entry := r.anims[name]
	if entry == nil || entry.anim == nil {
		return nil
	}
	return entry.anim.DeathFX()
}

// MarkerPoint — мировая точка маркера объекта (ствол, дуло) с учётом его
// текущей позы и сдвига по карте. Второй конец эффекта сцена считает сама.
func (r *Renderer) MarkerPoint(name, marker string) ([3]float64, bool) {
	if r == nil || name == "" || marker == "" {
		return [3]float64{}, false
	}
	entry := r.anims[name]
	if entry == nil || entry.anim == nil {
		return [3]float64{}, false
	}
	point, ok := entry.anim.MarkerPoint(marker)
	if !ok {
		return [3]float64{}, false
	}
	return [3]float64{point[0] + entry.move[0], point[1] + entry.move[1], point[2] + entry.move[2]}, true
}

// SetTextureTree задаёт дерево модуля, из которого рендерер берёт карты
// эффектов (Data/art/fx). Вызывается при загрузке сценария: без дерева
// частицы рисуются встроенным кругом.
func (r *Renderer) SetTextureTree(tree *modapi.ModTree) {
	if r == nil || r.fxTree == tree {
		return
	}
	r.fxTree = tree
	r.fxMissing = map[string]bool{}
}

// fxTextureFor отдаёт GPU-текстуру карты эффекта. Пустое имя и ненайденная
// карта — запасной круг: эффект без текстуры лучше, чем пропавшие частицы.
func (r *Renderer) fxTextureFor(name string) *gpuTexture {
	if name == "" || r.fxTree == nil {
		return r.fxTexture
	}
	if tex, ok := r.fxTextures[name]; ok {
		return tex
	}
	if r.fxMissing[name] {
		return r.fxTexture
	}
	img, ok := r.loadFxImage(name)
	if !ok {
		r.fxMissing[name] = true
		return r.fxTexture
	}
	width, height, pixels := fxPixels(img)
	tex, err := r.uploadTexture(width, height, pixels)
	if err != nil {
		r.fxMissing[name] = true
		return r.fxTexture
	}
	r.fxTextures[name] = tex
	return tex
}

// loadFxImage читает и декодирует карту эффекта: путь перебирается от
// DATA:ART/FX до папки модуля, расширения — .dds/.rtx/.tga. Игра пишет в
// эффекте .tga, а в архивах лежит одноимённый .dds или .rtx, поэтому имя
// ищется и без расширения — по стему файла.
func (r *Renderer) loadFxImage(name string) (image.Image, bool) {
	if r.fxTree == nil || name == "" {
		return nil, false
	}
	for _, candidate := range fxTextureCandidates(name) {
		ext := strings.TrimPrefix(strings.ToLower(path.Ext(candidate)), ".")
		if data, err := r.fxTree.ReadFile(candidate); err == nil && len(data) > 0 {
			if img := decodeFxTexture(data, ext); img != nil {
				return img, true
			}
		}
		base := strings.ToLower(path.Base(candidate))
		stem := strings.TrimSuffix(base, path.Ext(base))
		for _, fileName := range []string{base, stem} {
			if fileName == "" {
				continue
			}
			file := r.fxTree.FindByName(fileName)
			if file == nil {
				continue
			}
			data, err := r.fxTree.ReadTreeFile(file)
			if err != nil || len(data) == 0 {
				continue
			}
			if img := decodeFxTexture(data, ext); img != nil {
				return img, true
			}
		}
	}
	return nil, false
}

// fxTextureCandidates перечисляет пути, под которыми ищется карта эффекта.
// Имя в свойстве Texture бывает полным data-путём, путём от папки эффектов
// или голым именем файла; расширение .tga из редактора часто не совпадает
// с .dds/.rtx в архиве, поэтому пробуются все три.
func fxTextureCandidates(name string) []string {
	p := strings.ReplaceAll(strings.TrimSpace(name), `\`, "/")
	for _, prefix := range []string{"data:", "data/", "./"} {
		if len(p) >= len(prefix) && strings.EqualFold(p[:len(prefix)], prefix) {
			p = p[len(prefix):]
		}
	}
	p = strings.TrimLeft(p, "/")
	if p == "" {
		return nil
	}
	if path.Ext(p) == "" {
		p += ".tga"
	}
	if !strings.HasPrefix(strings.ToLower(p), "art/fx/") {
		p = "art/fx/" + p
	}
	stem := strings.TrimSuffix(p, path.Ext(p))
	exts := []string{path.Ext(p), ".rtx", ".dds", ".tga"}
	out := make([]string, 0, 16)
	seen := map[string]bool{}
	for _, ext := range exts {
		if ext == "" {
			continue
		}
		rel := stem + ext
		for _, candidate := range []string{"Data/" + rel, rel, strings.ToLower("Data/" + rel), strings.ToLower(rel)} {
			if seen[candidate] {
				continue
			}
			seen[candidate] = true
			out = append(out, candidate)
		}
	}
	return out
}

// decodeFxTexture декодирует карту по magic-байтам, а если формат не узнан —
// перебирает игровые декодеры. Так .dds из DATA:ART/FX читается без того,
// чтобы путь в эффекте был правильным.
func decodeFxTexture(data []byte, ext string) image.Image {
	formats := make([]string, 0, 5)
	switch {
	case bytes.HasPrefix(data, []byte("Relic Chunky")):
		formats = append(formats, "rtx")
	case len(data) >= 4 && string(data[:4]) == "DDS ":
		formats = append(formats, "dds")
	}
	if ext != "" {
		formats = append(formats, ext)
	}
	formats = append(formats, "rtx", "dds", "tga")
	seen := map[string]bool{}
	for _, format := range formats {
		if seen[format] {
			continue
		}
		seen[format] = true
		img, err := imgdec.Decode(format, data)
		if err == nil && img != nil {
			return img
		}
	}
	return nil
}

// fxPixels переводит карту в RGBA с непремноженной альфой — в таком виде её
// ждёт пайплайн частиц (SrcAlpha/OneMinusSrcAlpha).
func fxPixels(img image.Image) (int, int, []byte) {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	pixels := make([]byte, width*height*4)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			pixel := color.NRGBAModel.Convert(img.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
			offset := (y*width + x) * 4
			pixels[offset+0] = pixel.R
			pixels[offset+1] = pixel.G
			pixels[offset+2] = pixel.B
			pixels[offset+3] = pixel.A
		}
	}
	return width, height, pixels
}

// appendF32 добавляет float32 в little-endian — так вершина ложится в
// буфер без промежуточной структуры.
func appendF32(buf []byte, value float32) []byte {
	var bits [4]byte
	binary.LittleEndian.PutUint32(bits[:], math.Float32bits(value))
	return append(buf, bits[:]...)
}

func (r *Renderer) addMesh(m *sgbview.Mesh, tex *gpuTexture, tint [4]float32, alphaRef float32, translucent bool, boneBase int32) error {
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
	mesh := gpuMesh{
		vbuf:        vbuf,
		ibuf:        ibuf,
		count:       uint32(len(m.Indices)),
		tex:         tex,
		tint:        tint,
		alphaRef:    alphaRef,
		translucent: translucent,
		alpha:       1,
		name:        m.Name,
		part:        m.Part,
		partHidden:  m.Hidden,
		boneBase:    boneBase,
	}
	// Движущиеся объекты: копия вершин и центр для сдвига по приказу.
	if r.dynamic[m.Name] && len(m.Vertices) > 0 {
		mesh.base = append([]byte(nil), vbytes...)
		var min, max [3]float32
		for i, v := range m.Vertices {
			if i == 0 {
				min, max = v.Pos, v.Pos
				continue
			}
			for axis := range v.Pos {
				if v.Pos[axis] < min[axis] {
					min[axis] = v.Pos[axis]
				}
				if v.Pos[axis] > max[axis] {
					max[axis] = v.Pos[axis]
				}
			}
		}
		for axis := range mesh.baseCenter {
			mesh.baseCenter[axis] = (min[axis] + max[axis]) * 0.5
		}
	}
	r.meshes = append(r.meshes, mesh)
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
	writes := []vulkan.WriteDescriptorSet{vulkan.WriteDescriptorImage(set, 0, r.sampler, view, vulkan.ImageLayoutShaderReadOnlyOptimal)}
	if r.palette.Buffer != 0 {
		writes = append(writes, vulkan.WriteDescriptorBuffer(set, 1,
			vulkan.DescriptorTypeStorageBuffer, r.palette.Buffer, 0, r.palette.Size))
	}
	a.UpdateDescriptorSets(r.dev, writes)

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

	currentPipe := r.opaquePipe
	a.CmdBindPipeline(cmd, r.opaquePipe)

	for i := range r.meshes {
		m := &r.meshes[i]
		if m.hidden || m.partHidden || m.alpha <= 0.001 {
			continue
		}
		// Исчезающий меш рисуется своим пайплайном (альфа + запись
		// глубины), вода — полупрозрачным, остальное — непрозрачным.
		want := r.opaquePipe
		switch {
		case m.alpha < 1:
			if r.fadePipe != 0 {
				want = r.fadePipe
			} else {
				want = r.blendPipe
			}
		case m.translucent:
			want = r.blendPipe
		}
		if want != currentPipe {
			currentPipe = want
			a.CmdBindPipeline(cmd, want)
		}
		tint := m.tint
		tint[3] *= m.alpha
		pc := pushData{
			ViewProj: vp,
			Tint:     tint,
			Params:   [4]float32{m.alphaRef, ambient, diffuse, 0},
			BoneBase: m.boneBase,
		}
		a.CmdPushConstants(cmd, r.pipeLayout, stages, unsafe.Pointer(&pc), uint32(unsafe.Sizeof(pc)))
		a.CmdBindDescriptorSet(cmd, r.pipeLayout, m.tex.set)
		a.CmdBindVertexBuffer(cmd, m.vbuf.Buffer, 0)
		a.CmdBindIndexBuffer(cmd, m.ibuf.Buffer, 0, vulkan.IndexTypeUint32)
		a.CmdDrawIndexed(cmd, m.count)
	}

	// Частицы — поверх воды: общие вершины кадра, но своя карта у каждого
	// батча (DATA:ART/FX). Частицы без карты идут запасным кругом.
	if r.fxQuads > 0 && r.fxPipe != 0 && len(r.fxBatches) > 0 {
		currentPipe := r.fxPipe
		a.CmdBindPipeline(cmd, currentPipe)
		pc := pushData{
			ViewProj: vp,
			Tint:     [4]float32{1, 1, 1, 1},
			Params:   [4]float32{0, 1, 1, 0},
		}
		a.CmdPushConstants(cmd, r.pipeLayout, stages, unsafe.Pointer(&pc), uint32(unsafe.Sizeof(pc)))
		a.CmdBindVertexBuffer(cmd, r.fxVB.Buffer, 0)
		a.CmdBindIndexBuffer(cmd, r.fxIB.Buffer, 0, vulkan.IndexTypeUint32)
		for _, batch := range r.fxBatches {
			tex := batch.tex
			if tex == nil {
				tex = r.fxTexture
			}
			if tex == nil {
				continue
			}
			// Батчи уже разложены: сначала альфа, потом сложение, так что
			// пайплайн меняется один раз за кадр.
			want := r.fxPipe
			if batch.additive && r.fxAddPipe != 0 {
				want = r.fxAddPipe
			}
			if want != currentPipe {
				currentPipe = want
				a.CmdBindPipeline(cmd, currentPipe)
			}
			a.CmdBindDescriptorSet(cmd, r.pipeLayout, tex.set)
			a.CmdDrawIndexedRange(cmd, batch.count, batch.first)
		}
	}
	return nil
}

// textureFor отдаёт GPU-текстуру сцены: одинаковая картинка (Key) грузится
// один раз, а её дескриптор-сет сразу указывает и на палитру костей.
func (r *Renderer) textureFor(t *sgbview.Texture) *gpuTexture {
	if t == nil || t.Width <= 0 || t.Height <= 0 || len(t.Pixels) < t.Width*t.Height*4 {
		return r.whiteTex
	}
	if existing, ok := r.textures[t.Key]; ok && t.Key != "" {
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

// AddObject добавляет в загруженную сцену объект, появившийся по ходу игры
// (постройка или нанятый боец): геометрию с текстурами, палитру костей и
// анимацию. Палитра создана с запасом, поэтому сцена не перезагружается.
// dynamic=true помечает объект перемещаемым: рендерер копирует его вершины,
// чтобы MoveObject мог его сдвигать.
func (r *Renderer) AddObject(obj sgbview.Object, dynamic bool) error {
	if r == nil || r.palette.Buffer == 0 {
		return fmt.Errorf("gfx3d: сцена ещё не загружена")
	}
	if len(obj.Parts) == 0 {
		return fmt.Errorf("gfx3d: у объекта нет геометрии")
	}
	var base int32
	if obj.Anim != nil {
		bones := obj.Anim.BoneCount()
		if (r.boneNext+bones)*16 > len(r.paletteCPU) {
			return fmt.Errorf("gfx3d: палитра костей переполнена: %d костей", r.boneNext+bones)
		}
		obj.Anim.SetBase(r.boneNext)
		base = int32(r.boneNext)
		r.boneNext += bones
	}
	if dynamic {
		if r.dynamic == nil {
			r.dynamic = map[string]bool{}
		}
		r.dynamic[obj.MeshName()] = true
	}
	white := [4]float32{1, 1, 1, 1}
	for _, part := range obj.Parts {
		if part.Mesh == nil {
			continue
		}
		if err := r.addMesh(part.Mesh, r.textureFor(part.Texture), white, 0.5, false, base); err != nil {
			return err
		}
	}
	if obj.Anim != nil {
		if name := obj.MeshName(); name != "" {
			if r.anims == nil {
				r.anims = map[string]*animEntry{}
			}
			r.anims[name] = &animEntry{anim: obj.Anim}
		}
	}
	return nil
}

// SetConstruction ставит объект на кадр стройки по доле готовности: 0 —
// начало, 1 — последний кадр последней стадии. false — объекта нет или ему
// нечего показывать (у модели нет клипа стройки).
func (r *Renderer) SetConstruction(name string, progress float64) bool {
	if r == nil {
		return false
	}
	entry := r.anims[name]
	if entry == nil || !entry.anim.HasBuildAnimation() {
		return false
	}
	entry.anim.SetProgress(progress)
	return true
}

// SetFacing задаёт направление взгляда объекта: угол в мировых радианах,
// 0 — вдоль +X. false — объекта нет.
func (r *Renderer) SetFacing(name string, yaw float64) bool {
	if r == nil {
		return false
	}
	entry := r.anims[name]
	if entry == nil {
		return false
	}
	entry.anim.SetFacing(yaw)
	return true
}

// SetAnim задаёт сдвиг от места появления и состояние анимации объекта
// (по имени меша). false — объект статичен (нет скелета или клипа idle):
// его двигает MoveObject, а не палитра.
func (r *Renderer) SetAnim(name string, move [3]float64, state sgbview.AnimState) bool {
	if r == nil {
		return false
	}
	e := r.anims[name]
	if e == nil {
		return false
	}
	e.move = move
	e.anim.SetState(state)
	return true
}

// SetAttack включает боевую стойку объекта: melee — ближний бой, иначе
// стрельба. false — объекта нет или у модели нет клипов боя.
func (r *Renderer) SetAttack(name string, melee bool) bool {
	if r == nil {
		return false
	}
	e := r.anims[name]
	if e == nil || !e.anim.HasAttack() {
		return false
	}
	e.anim.SetAttack(melee)
	return true
}

// SetAim задаёт направление на цель: bearing — угол в мире относительно
// взгляда бойца (0 — прямо перед ним), pitch — подъём в радианах. Кадр
// прицела выбирается по этому углу, поэтому движущаяся цель не уходит
// из-под ствола.
func (r *Renderer) SetAim(name string, bearing, pitch float64) bool {
	if r == nil {
		return false
	}
	e := r.anims[name]
	if e == nil {
		return false
	}
	e.anim.SetAim(bearing, pitch)
	return true
}

// StepAnimations двигает время анимаций на dt секунд (0 — заморозка,
// как на паузе: клип не идёт, но палитра перезаливается — состояние
// могло смениться приказом) и записывает обновлённую палитру костей
// в GPU-буфер.
func (r *Renderer) StepAnimations(dt float64) error {
	if r == nil || r.palette.Buffer == 0 || len(r.anims) == 0 || len(r.paletteCPU) == 0 {
		return nil
	}
	for _, e := range r.anims {
		e.anim.Step(dt)
		e.anim.WritePalette(r.paletteCPU, e.move)
	}
	bytes := unsafe.Slice((*byte)(unsafe.Pointer(&r.paletteCPU[0])), len(r.paletteCPU)*4)
	return r.api.WriteBuffer(r.dev, r.palette, bytes)
}

// createPalette создаёт палитру костей кадра на n костей (минимум одна):
// буфер хост-видимый, CPU-копия заполняется единичными матрицами — до
// первого шага анимации покой скелета даёт исходные вершины.
func (r *Renderer) createPalette(n int) error {
	if n < 1 {
		n = 1
	}
	r.paletteCPU = make([]float32, n*16)
	for i := 0; i < n; i++ {
		b := i * 16
		r.paletteCPU[b+0], r.paletteCPU[b+5] = 1, 1
		r.paletteCPU[b+10], r.paletteCPU[b+15] = 1, 1
	}
	bytes := unsafe.Slice((*byte)(unsafe.Pointer(&r.paletteCPU[0])), len(r.paletteCPU)*4)
	buf, err := r.api.CreateBuffer(r.dev, uint64(len(bytes)),
		vulkan.BufferUsageStorageBufferBit,
		vulkan.MemoryPropertyHostVisibleBit|vulkan.MemoryPropertyHostCoherentBit)
	if err != nil {
		return fmt.Errorf("gfx3d: палитра костей: %w", err)
	}
	if err := r.api.WriteBuffer(r.dev, buf, bytes); err != nil {
		r.api.DestroyBuffer(r.dev, &buf)
		return fmt.Errorf("gfx3d: заливка палитры: %w", err)
	}
	r.palette = buf
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
	if r.fxTexture != nil {
		r.destroyTexture(r.fxTexture)
		r.fxTexture = nil
	}
	for _, t := range r.fxTextures {
		r.destroyTexture(t)
	}
	r.fxTextures = nil
	if r.opaquePipe != 0 {
		a.DestroyPipeline(r.dev, r.opaquePipe)
		r.opaquePipe = 0
	}
	if r.blendPipe != 0 {
		a.DestroyPipeline(r.dev, r.blendPipe)
		r.blendPipe = 0
	}
	if r.fadePipe != 0 {
		a.DestroyPipeline(r.dev, r.fadePipe)
		r.fadePipe = 0
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
