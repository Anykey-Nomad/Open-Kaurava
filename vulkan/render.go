package vulkan

// Командные буферы и кадровая синхронизация.
//
// Схема базового кадра (классика Vulkan, без шейдеров):
//
//	imageAvailable ← acquire      (семафор на кадр)
//	_fence.Wait / Reset
//	командный буфер: render pass с loadOp=CLEAR → layout → PRESENT_SRC
//	queueSubmit: wait imageAvailable @ COLOR_ATTACHMENT_OUTPUT,
//	             signal renderFinished[imageIndex], fence
//	queuePresent: wait renderFinished[imageIndex]
//
// renderFinished живёт по числу изображений swapchain (а не кадров):
// present ждёт его ровно для своего изображения, поэтому переиспользовать
// семафор чужого кадра нельзя.

// CreateCommandPool создаёт пул командных буферов.
func (a *API) CreateCommandPool(dev Device, family, flags uint32) (CommandPool, error) {
	if flags == 0 {
		flags = CommandPoolCreateResetCommandBufferBit
	}
	info := CommandPoolCreateInfo{
		Type:             StructureTypeCommandPoolCreateInfo,
		Flags:            flags,
		QueueFamilyIndex: family,
	}
	var pool CommandPool
	if err := check("vkCreateCommandPool", a.vkCreateCommandPool(dev, &info, nil, &pool)); err != nil {
		return 0, err
	}
	return pool, nil
}

// DestroyCommandPool освобождает пул (вместе с его буферами команд).
func (a *API) DestroyCommandPool(dev Device, pool CommandPool) {
	if pool != 0 {
		a.vkDestroyCommandPool(dev, pool, nil)
	}
}

// AllocateCommandBuffers выделяет буферы команд из пула.
func (a *API) AllocateCommandBuffers(dev Device, pool CommandPool, count int) ([]CommandBuffer, error) {
	if count <= 0 {
		return nil, nil
	}
	info := CommandBufferAllocateInfo{
		Type:               StructureTypeCommandBufferAllocateInfo,
		CommandPool:        pool,
		Level:              CommandBufferLevelPrimary,
		CommandBufferCount: uint32(count),
	}
	bufs := make([]CommandBuffer, count)
	if err := check("vkAllocateCommandBuffers", a.vkAllocateCommandBuffers(dev, &info, &bufs[0])); err != nil {
		return nil, err
	}
	return bufs, nil
}

// ResetCommandPool возвращает все буферы пула в исходное состояние.
//
// ВНИМАНИЕ: сбрасывать общий пул нельзя, пока в работе чужой буфер
// команд (кадров в полёте несколько). Для одного кадра — ResetCommandBuffer.
func (a *API) ResetCommandPool(dev Device, pool CommandPool) error {
	return check("vkResetCommandPool", a.vkResetCommandPool(dev, pool, 0))
}

// ResetCommandBuffer возвращает один буфер команд в исходное состояние.
//
// Так можно готовить кадр, не задевая другие буферы того же пула: сброс
// пула запрещён, пока какой-либо его буфер ещё выполняется на GPU.
func (a *API) ResetCommandBuffer(cmd CommandBuffer) error {
	return check("vkResetCommandBuffer", a.vkResetCommandBuffer(cmd, 0))
}

// BeginCommandBuffer открывает буфер команд на запись.
func (a *API) BeginCommandBuffer(cmd CommandBuffer, flags uint32) error {
	if flags == 0 {
		flags = CommandBufferUsageOneTimeSubmitBit
	}
	info := CommandBufferBeginInfo{
		Type:  StructureTypeCommandBufferBeginInfo,
		Flags: flags,
	}
	return check("vkBeginCommandBuffer", a.vkBeginCommandBuffer(cmd, &info))
}

// EndCommandBuffer завершает запись.
func (a *API) EndCommandBuffer(cmd CommandBuffer) error {
	return check("vkEndCommandBuffer", a.vkEndCommandBuffer(cmd))
}

// Frame — ресурсы одного кадра в полёте.
//
// Cmd, ImageAvailable и InFlight живут по числу кадров в полёте.
// RenderFinished живёт в Swapchain — по числу изображений (см. выше).
type Frame struct {
	Cmd            CommandBuffer
	ImageAvailable Semaphore
	InFlight       Fence
}

// NewFrameSet создаёт count кадров синхронизации: командные буферы из pool +
// семафор ожидания изображения + fence (создаётся сигнальным).
func (a *API) NewFrameSet(dev Device, pool CommandPool, count int) ([]Frame, error) {
	cmds, err := a.AllocateCommandBuffers(dev, pool, count)
	if err != nil {
		return nil, err
	}
	frames := make([]Frame, count)
	for i, cmd := range cmds {
		frames[i].Cmd = cmd

		semInfo := SemaphoreCreateInfo{Type: StructureTypeSemaphoreCreateInfo}
		if err := check("vkCreateSemaphore", a.vkCreateSemaphore(dev, &semInfo, nil, &frames[i].ImageAvailable)); err != nil {
			return nil, err
		}

		fenceInfo := FenceCreateInfo{
			Type:  StructureTypeFenceCreateInfo,
			Flags: FenceCreateSignaledBit, // первый кадр не должен ждать
		}
		if err := check("vkCreateFence", a.vkCreateFence(dev, &fenceInfo, nil, &frames[i].InFlight)); err != nil {
			return nil, err
		}
	}
	return frames, nil
}

// Destroy освобождает ресурсы кадра.
func (a *API) DestroyFrame(dev Device, f *Frame) {
	if f.ImageAvailable != 0 {
		a.vkDestroySemaphore(dev, f.ImageAvailable, nil)
		f.ImageAvailable = 0
	}
	if f.InFlight != 0 {
		a.vkDestroyFence(dev, f.InFlight, nil)
		f.InFlight = 0
	}
	f.Cmd = 0
}

// WaitForFence ждёт завершения кадра (бесконечно).
func (a *API) WaitForFence(f Fence) error {
	return check("vkWaitForFences", a.vkWaitForFences(a.dev, 1, &f, 1, ^uint64(0)))
}

// ResetFence возвращает fence в сигнальное состояние после ожидания.
func (a *API) ResetFence(f Fence) error {
	return check("vkResetFences", a.vkResetFences(a.dev, 1, &f))
}

// Rect — залитый прямоугольник (пиксали framebuffer, левый верхний угол — начало).
//
// Используется для курсора, рамок выделения и прочей 2D-графики без шейдеров:
// заливка идёт через vkCmdClearAttachments внутри render pass.
type Rect struct {
	X, Y, W, H int32
	Color      [4]float32
}

// RecordClear записывает в буфер команд один кадр: начало → render pass с
// заливкой цветом → конец. Этого достаточно, чтобы прокрутить весь путь
// acquire → render → present без шейдеров и пайплайна.
//
// index — индекс изображения swapchain (для фреймбуфера).
func (s *Swapchain) RecordClear(cmd CommandBuffer, index uint32, color [4]float32) error {
	return s.Record(cmd, index, color, nil, nil)
}

// Record — RecordClear плюс набор залитых прямоугольников поверх фона
// (курсор, рамка выделения) и необязательный callback draw, вызываемого
// внутри render pass уже поверх заливки — им рисует пакет ui.Renderer
// (пайплайн, дескрипторы, vkCmdDraw). Прямоугольники обрезаются по краям
// кадра; пустые и полностью за пределами кадра пропускаются.
func (s *Swapchain) Record(cmd CommandBuffer, index uint32, color [4]float32, rects []Rect, draw func(CommandBuffer) error) error {
	if int(index) >= len(s.Framebuffers) {
		return &Error{Op: "Record", Code: ResultErrorOutOfDateKHR}
	}
	a := s.api
	if err := a.BeginCommandBuffer(cmd, 0); err != nil {
		return err
	}

	clears := [2]ClearValue{
		{Color: ClearColorValue{Float32: color}},
		{Color: ClearColorValue{Float32: [4]float32{1.0, 0, 0, 0}}},
	}
	begin := RenderPassBeginInfo{
		Type:            StructureTypeRenderPassBeginInfo,
		RenderPass:      s.RenderPass,
		Framebuffer:     s.Framebuffers[index],
		RenderArea:      Rect2D{Extent: s.Extent},
		ClearValueCount: uint32(len(clears)),
		ClearValues:     &clears[0],
	}
	a.vkCmdBeginRenderPass(cmd, &begin, SubpassContentsInline)

	for _, r := range rects {
		x0, y0, x1, y1 := clipRect(r, s.Extent)
		if x1 <= x0 || y1 <= y0 {
			continue
		}
		att := ClearAttachment{
			AspectMask:      ImageAspectColorBit,
			ColorAttachment: 0,
			ClearValue:      ClearValue{Color: ClearColorValue{Float32: r.Color}},
		}
		cr := ClearRect{
			Rect: Rect2D{
				Offset: Offset2D{X: x0, Y: y0},
				Extent: Extent2D{Width: uint32(x1 - x0), Height: uint32(y1 - y0)},
			},
			LayerCount: 1,
		}
		a.vkCmdClearAttachments(cmd, 1, &att, 1, &cr)
	}

	// Слой поверх заливки: пайплайн UI (шейдеры, атлас глифов).
	if draw != nil {
		if err := draw(cmd); err != nil {
			return err
		}
	}

	a.vkCmdEndRenderPass(cmd)
	return a.EndCommandBuffer(cmd)
}

// clipRect обрезает прямоугольник по границам кадра.
func clipRect(r Rect, extent Extent2D) (x0, y0, x1, y1 int32) {
	x0, y0 = r.X, r.Y
	x1, y1 = r.X+r.W, r.Y+r.H
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > int32(extent.Width) {
		x1 = int32(extent.Width)
	}
	if y1 > int32(extent.Height) {
		y1 = int32(extent.Height)
	}
	return
}
