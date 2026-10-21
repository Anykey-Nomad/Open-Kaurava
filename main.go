// Пример базового кадра на purego-биндингах Vulkan: окно ОС + инстанс Vulkan +
// поверхность + swapchain + отрисовка (заливка цветом) — и всё это без CGO
// на Windows и Linux из одного кода.
//
// Запуск:
//
//	go run .            # окно, рисует анимированную заливку, пока не закрыто
//	go run . -frames 300 # отрисовать 300 кадров и выйти (удобно для проверок)
//	go run . -validation # включить слои валидации, если они установлены
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/Anykey-Nomad/Open-Kaurava/gfx3d"
	"github.com/Anykey-Nomad/Open-Kaurava/mods"
	"github.com/Anykey-Nomad/Open-Kaurava/scene"
	"github.com/Anykey-Nomad/Open-Kaurava/scenes"
	"github.com/Anykey-Nomad/Open-Kaurava/ui"
	"github.com/Anykey-Nomad/Open-Kaurava/vulkan"
	"github.com/Anykey-Nomad/Open-Kaurava/window"
)

// Число кадров в полёте: command buffer + семафор + fence на каждый.
const framesInFlight = 2

func main() {
	// Окно создаётся и опрашивается в одном потоке: Win32 требует, чтобы
	// сообщения приходили в поток, создавший окно, а Xlib не потокобезопасен.
	runtime.LockOSThread()

	var (
		width      = flag.Int("width", 1280, "ширина окна, px")
		height     = flag.Int("height", 720, "высота окна, px")
		title      = flag.String("title", "Open-Kaurava — purego Vulkan", "заголовок окна")
		frames     = flag.Int("frames", 0, "сколько кадров отрисовать (0 — пока окно не закрыто)")
		validation = flag.Bool("validation", false, "включить VK_LAYER_KHRONOS_validation, если установлен")
		vulkanLib  = flag.String("vulkan", "", "путь к библиотеке Vulkan (по умолчанию — стандартная для ОС)")
		logEvents  = flag.Bool("log-events", false, "печатать события окна")
		present    = flag.String("present", "", "режим представления: fifo|mailbox|immediate (по умолчанию auto)")
		verbose    = flag.Bool("v", false, "подробный вывод")
		resizeTest = flag.Bool("resize-test", false, "сам менять размер окна и проверять пересоздание swapchain")
		osCursor   = flag.Bool("os-cursor", false, "оставить системный курсор вместо рисуемого крестика")
		fontPath   = flag.String("font", "", "путь к шрифту TTF/OTF (по умолчанию — системный)")
		startScene = flag.String("scene", "menu", "сцена на старте: menu|scenarios|players|preview|gameplay")
		dowPath    = flag.String("dow", "", "путь к установленной Dawn of War (по умолчанию — ищется сама)")
		scenario   = flag.String("scenario", "", "сценарий для 3D-просмотра (имя файла .sgb)")
	)
	flag.Parse()

	if *resizeTest && *frames == 0 {
		*frames = 150 // тест должен завершиться сам
	}

	if err := run(args{
		width: *width, height: *height, title: *title, maxFrames: *frames,
		validation: *validation, vulkanLib: *vulkanLib, logEvents: *logEvents,
		present: *present, verbose: *verbose, resizeTest: *resizeTest, osCursor: *osCursor,
		font: *fontPath, scene: *startScene, dow: *dowPath, scenario: *scenario,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "ошибка:", err)
		os.Exit(1)
	}
}

// args — параметры командной строки примера.
type args struct {
	width, height int
	title         string
	maxFrames     int
	validation    bool
	vulkanLib     string
	logEvents     bool
	present       string
	verbose       bool
	resizeTest    bool
	osCursor      bool
	font          string
	scene         string
	dow           string
	scenario      string
}

func run(a args) error {
	// --- 1. Окно -------------------------------------------------------------
	win, err := window.New(window.Config{Title: a.title, Width: a.width, Height: a.height})
	if err != nil {
		return err
	}
	defer win.Destroy()

	// --- 2. Загрузка Vulkan (purego: Dlopen/LoadLibrary) ----------------------
	api, err := vulkan.Load(a.vulkanLib)
	if err != nil {
		return err
	}
	defer api.Close()

	// --- 3. Расширения -------------------------------------------------------
	required := []string{vulkan.ExtSurface, win.SurfaceExtension()}
	avail, err := api.EnumerateInstanceExtensionProperties("")
	if err != nil {
		return err
	}
	for _, name := range required {
		if !vulkan.ExtensionList(avail).Contains(name) {
			return fmt.Errorf("расширение %s недоступно; доступны: %s", name, vulkan.ExtensionList(avail))
		}
	}

	var layers []string
	if a.validation {
		ok, _ := api.HasLayer(vulkan.LayerKhronosValidation)
		hasDebug, _ := api.HasInstanceExtension(vulkan.ExtDebugUtils)
		if ok && hasDebug {
			layers = append(layers, vulkan.LayerKhronosValidation)
			required = append(required, vulkan.ExtDebugUtils)
		} else {
			log.Printf("предупреждение: слой валидации недоступен (слой=%v, debug_utils=%v)", ok, hasDebug)
			a.validation = false
		}
	}

	// --- 4. VkInstance -------------------------------------------------------
	instance, err := api.NewInstance("Open-Kaurava", required, layers)
	if err != nil {
		return err
	}
	defer api.DestroyInstance(instance)

	var messenger vulkan.DebugUtilsMessengerEXT
	if a.validation {
		severity := vulkan.DebugUtilsMessageSeverityWarningBitEXT | vulkan.DebugUtilsMessageSeverityErrorBitEXT
		types := vulkan.DebugUtilsMessageTypeGeneralBitEXT |
			vulkan.DebugUtilsMessageTypeValidationBitEXT |
			vulkan.DebugUtilsMessageTypePerformanceBitEXT
		messenger, err = api.CreateDebugMessenger(instance, severity, types,
			func(sev uint32, msg string) { log.Printf("[vulkan] %s", msg) })
		if err != nil {
			log.Printf("предупреждение: %s", vulkan.DebugMessengerHint(err))
		} else {
			defer api.DestroyDebugMessenger(instance, messenger)
		}
	}

	// --- 5. Поверхность (хэндлы отдаёт окно) ---------------------------------
	surface, err := win.CreateSurface(api, instance)
	if err != nil {
		return fmt.Errorf("создание поверхности: %w", err)
	}
	defer api.DestroySurface(instance, surface)

	// --- 6. Физическое устройство и логическое устройство -------------------
	deviceInfo, err := api.ChoosePhysicalDevice(instance, surface, []string{vulkan.ExtSwapchain})
	if err != nil {
		return err
	}
	if a.verbose {
		log.Printf("устройство: %s (семейства: graphics=%d present=%d)",
			deviceInfo.Name(), deviceInfo.GraphicsFamily, deviceInfo.PresentFamily)
	}

	queueFamilies := []uint32{deviceInfo.GraphicsFamily, deviceInfo.PresentFamily}
	device, err := api.CreateDevice(deviceInfo, queueFamilies, []string{vulkan.ExtSwapchain})
	if err != nil {
		return err
	}
	defer api.DestroyDevice(device)

	queue := api.GetDeviceQueue(device, deviceInfo.GraphicsFamily, 0)

	// --- 7. Swapchain --------------------------------------------------------
	w, h := win.Size()
	opts := vulkan.SwapchainOptions{
		Width:           w,
		Height:          h,
		FramesInFlight:  framesInFlight,
		GraphicsFamily:  deviceInfo.GraphicsFamily,
		PresentFamily:   deviceInfo.PresentFamily,
		PhysicalDevice:  deviceInfo.PhysicalDevice,
		PreferredFormat: vulkan.FormatUndefined,
	}
	switch a.present {
	case "fifo":
		m := vulkan.PresentModeFIFOKHR
		opts.PreferredPresentMode = &m
	case "mailbox":
		m := vulkan.PresentModeMailboxKHR
		opts.PreferredPresentMode = &m
	case "immediate":
		m := vulkan.PresentModeImmediateKHR
		opts.PreferredPresentMode = &m
	}

	swapchain, err := vulkan.NewSwapchain(api, device, surface, opts)
	if err != nil {
		return err
	}
	defer swapchain.Destroy()

	if a.verbose {
		log.Printf("swapchain: %s, %dx%d, изображений=%d, present=%d",
			formatName(swapchain.Format), swapchain.Extent.Width, swapchain.Extent.Height,
			len(swapchain.Images), int(swapchain.PresentMode()))
	}

	// --- 8. Командный пул и кадры синхронизации ------------------------------
	pool, err := api.CreateCommandPool(device, deviceInfo.GraphicsFamily, 0)
	if err != nil {
		return err
	}
	defer api.DestroyCommandPool(device, pool)

	frames, err := api.NewFrameSet(device, pool, framesInFlight)
	if err != nil {
		return err
	}
	defer func() {
		for i := range frames {
			api.DestroyFrame(device, &frames[i])
		}
	}()

	// --- 9. Рендерер UI: шрифт, атлас глифов, пайплайн -----------------------
	fontPath, err := ui.FindFont(a.font)
	if err != nil {
		return err
	}
	font, err := ui.LoadFont(fontPath)
	if err != nil {
		return err
	}
	if a.verbose {
		log.Printf("шрифт: %s", fontPath)
	}
	renderer, err := ui.NewRenderer(api, device, swapchain, queue, deviceInfo.GraphicsFamily, ui.Options{
		Font:           font,
		FramesInFlight: framesInFlight,
	})
	if err != nil {
		return err
	}
	defer renderer.Destroy()

	// --- 9a. Рендерер 3D-сцены сценария --------------------------------------
	// Тот же render pass, что и у интерфейса: карта рисуется первой, UI —
	// поверх неё (у UI-пайплайна проверка глубины выключена).
	world, err := gfx3d.NewRenderer(api, device, swapchain, queue, deviceInfo.GraphicsFamily)
	if err != nil {
		return err
	}
	defer world.Destroy()

	// --- 10. Модули игры и сцены -------------------------------------------
	// Модуль (файлы .module и дерево файлов) грузится в фоне: дерево
	// читает архивы игры, и ждать его на старте окна незачем.
	library := mods.New(a.dow)

	manager := scene.New(0.7) // 0.7 с — экран загрузки между сценами
	manager.Register(scenes.NewMenu(library))
	manager.Register(scenes.NewScenarios(library))
	manager.Register(scenes.NewPlayers(library))
	manager.Register(scenes.NewPreview(library, world))
	manager.Register(scenes.NewGameplay())
	if err := manager.SwitchNow(a.scene); err != nil {
		return err
	}
	if a.verbose {
		log.Printf("сцены: %v, старт: %s", manager.Names(), manager.Current())
	}

	// --- 11. Цикл кадров -----------------------------------------------------
	log.Printf("окно: backend=%s %dx%d; Vulkan: %s; библиотека: %s",
		win.Backend(), w, h, formatAPIVersion(api.APIVersion()), api.Path())

	var (
		frameIndex   int
		rendered     int
		recreations  int
		needRecreate bool
		start        = time.Now()
		last         = time.Now()

		// Мышиные жесты: трекер превращает события в клики/драги,
		// захват курсора держит драг живым за границами окна.
		drag window.Drag

		// Ввод текущего кадра: клики, клавиши, позиция мыши.
		input  scene.Input
		keys   []window.Key
		scroll float64
	)

	// Клавиши, зажатые в данный момент: нужны камере для ходьбы.
	held := map[window.Key]bool{}
	// Предыдущая позиция мыши — чтобы посчитать смещение за кадр.
	prevX, prevY := win.MousePos()

	// Сценарий, заданный в командной строке: ждём дерево модуля и
	// открываем 3D-просмотр этого сценария.
	pendingScenario := a.scenario

	// Курсор рисует движок поверх сцены (крестик из прямоугольников),
	// поэтому системный можно спрятать — без -os-cursor.
	if !a.osCursor {
		win.SetCursorVisible(false)
	}

	// Пересоздание swapchain под текущий размер окна.
	recreate := func() error {
		ww, hh := win.Size()
		if ww <= 0 || hh <= 0 {
			return nil // свёрнутое окно — ждём разворота
		}
		if err := api.DeviceWaitIdle(device); err != nil {
			return err
		}
		if err := swapchain.Recreate(ww, hh); err != nil {
			return err
		}
		// Пайплайн UI ссылался на старый render pass — пересоздаём.
		if err := renderer.Recreate(); err != nil {
			return err
		}
		// То же самое для 3D-пайплайнов.
		if err := world.Recreate(); err != nil {
			return err
		}
		recreations++
		if a.verbose {
			log.Printf("swapchain пересоздан: %dx%d", swapchain.Extent.Width, swapchain.Extent.Height)
		}
		return nil
	}

	for !win.ShouldClose() {
		wasDragging := drag.Dragging()
		keys = keys[:0]
		scroll = 0
		for _, ev := range win.PollEvents() {
			drag.Feed(ev)
			switch ev.Kind {
			case window.EventResize:
				needRecreate = true
				if a.verbose {
					log.Printf("событие: resize %dx%d", ev.Width, ev.Height)
				}
			case window.EventClose:
				needRecreate = true
			case window.EventKeyDown:
				keys = append(keys, ev.Key)
				held[ev.Key] = true
			case window.EventKeyUp:
				delete(held, ev.Key)
			case window.EventScroll:
				scroll += ev.Scroll
			default:
				if a.logEvents {
					log.Printf("событие: %s key=%s button=%d x=%d y=%d",
						ev.Kind, ev.Key, ev.Button, ev.X, ev.Y)
				}
			}
		}

		// Драг начался — захватываем курсор, чтобы рамка не рвалась у края окна.
		if drag.Dragging() && !wasDragging {
			if err := win.CaptureMouse(true); err != nil && a.verbose {
				log.Printf("захват курсора: %v", err)
			}
		}
		if !drag.Dragging() && wasDragging {
			win.CaptureMouse(false)
		}

		// Клики и завершённые драги — то, что отработал трекер.
		input = scene.Input{Keys: keys, Scroll: scroll}
		for _, c := range drag.TakeClicks() {
			input.Clicks = append(input.Clicks, scene.Click{Button: c.Button, X: c.X, Y: c.Y})
			log.Printf("клик: кнопка=%d в (%d, %d)", c.Button, c.X, c.Y)
		}
		for _, d := range drag.TakeDrags() {
			rx, ry, rw, rh := d.Rect()
			log.Printf("драг: кнопка=%d рамка (%d, %d) %dx%d", d.Button, rx, ry, rw, rh)
		}
		input.MouseX, input.MouseY = win.MousePos()
		input.MoveX, input.MoveY = input.MouseX-prevX, input.MouseY-prevY
		prevX, prevY = input.MouseX, input.MouseY
		for b := 1; b <= 3; b++ {
			if win.MouseDown(b) {
				input.Buttons |= 1 << b
			}
		}
		input.Held = heldKeys(held)
		if x, y, w, h, ok := drag.Rect(); ok {
			input.Dragging = true
			input.DragX, input.DragY, input.DragW, input.DragH = x, y, w, h
		}

		if win.ShouldClose() {
			break
		}

		// Размер окна мог измениться без события (страховка).
		ww, hh := win.Size()
		if ww != int(swapchain.Extent.Width) || hh != int(swapchain.Extent.Height) {
			needRecreate = true
		}

		frame := frames[frameIndex]

		// Ждём окончания кадра: его семафор и буфер команд нельзя
		// переиспользовать, пока GPU не закончил предыдущий кадр.
		if err := api.WaitForFence(frame.InFlight); err != nil {
			return err
		}

		// Изображение забираем после ожидания: к этому моменту семафор
		// кадра свободен (предыдущий submit с ним завершён).
		imageIndex, err := swapchain.Acquire(frame.ImageAvailable)
		if err != nil {
			if vulkan.IsOutOfDate(err) {
				needRecreate = true
				if err := recreate(); err != nil {
					return err
				}
				needRecreate = false
				continue
			}
			return err
		}

		// Fence сбрасываем только после успешного acquire: иначе при
		// пропущенном кадре ждать его было бы уже нечего.
		if err := api.ResetFence(frame.InFlight); err != nil {
			return err
		}
		// Сбрасываем только свой буфер команд: общий пул трогать нельзя,
		// пока другие кадры в полёте.
		if err := api.ResetCommandBuffer(frame.Cmd); err != nil {
			return err
		}

		// Сценарий из командной строки: как только дерево модуля готово,
		// выбираем его и открываем нужную сцену — 3D-просмотр или, если она
		// запрошена, экран выбора игроков.
		if pendingScenario != "" {
			if sc, ok := library.FindScenario(pendingScenario); ok {
				scenes.Select(sc.Name, sc.Path)
				pendingScenario = ""
				target := "preview"
				if a.scene == "players" {
					target = "players"
				}
				if manager.Current() != target {
					_ = manager.SwitchNow(target)
				}
				log.Printf("сценарий выбран: %s (%s)", sc.Name, sc.Path)
			}
		}

		// Сцена: ввод → обновление → отрисовка в батчер UI.
		now := time.Now()
		dt := now.Sub(last).Seconds()
		last = now

		ctx := scene.Ctx{
			UI:    renderer.NewFrame(),
			W:     int(swapchain.Extent.Width),
			H:     int(swapchain.Extent.Height),
			Input: input,
			DT:    dt,
			// Анимация цвета: доказывает, что кадры реально идут.
			Clear: animateColor(time.Since(start)),
		}
		manager.Update(&ctx, dt)
		manager.Draw(&ctx)
		color := ctx.Clear

		// Курсор рисует движок (системный спрятан) — как и в прошлой версии.
		if !a.osCursor {
			drawCursor(ctx.UI, input.MouseX, input.MouseY)
		}
		if err := renderer.Prepare(frameIndex); err != nil {
			return err
		}

		if err := swapchain.Record(frame.Cmd, imageIndex, color, nil,
			func(cmd vulkan.CommandBuffer) error {
				// Сначала 3D-мир, поверх него — интерфейс.
				if w3d := manager.World3D(); w3d != nil {
					if err := w3d.Record3D(cmd,
						float32(swapchain.Extent.Width), float32(swapchain.Extent.Height)); err != nil {
						return err
					}
				}
				return renderer.Record(cmd, frameIndex,
					float32(swapchain.Extent.Width), float32(swapchain.Extent.Height))
			}); err != nil {
			return err
		}

		err = api.QueueSubmit(queue, frame.Cmd, frame.ImageAvailable,
			swapchain.RenderFinished[imageIndex], vulkan.PipelineStageColorAttachmentOutputBit, frame.InFlight)
		if err != nil {
			return err
		}

		if err := swapchain.Present(queue, imageIndex, swapchain.RenderFinished[imageIndex]); err != nil {
			if vulkan.IsOutOfDate(err) {
				needRecreate = true
			} else {
				return err
			}
		}

		frameIndex = (frameIndex + 1) % len(frames)
		rendered++

		// Сцена попросила закрыть приложение.
		if manager.Quit {
			win.Close()
			break
		}

		// Автоматический тест ресайза: три смены размера за прогон.
		if a.resizeTest {
			switch rendered {
			case 30:
				resizeTo(win, 900, 600)
			case 60:
				resizeTo(win, 1280, 720)
			case 90:
				resizeTo(win, 1024, 576)
			}
		}

		if needRecreate {
			if err := recreate(); err != nil {
				return err
			}
			needRecreate = false
		}

		if a.maxFrames > 0 && rendered >= a.maxFrames {
			break
		}
	}

	elapsed := time.Since(start)
	if rendered > 0 {
		log.Printf("готово: кадров=%d за %s (%.1f fps), пересозданий swapchain=%d",
			rendered, elapsed.Round(time.Millisecond), float64(rendered)/elapsed.Seconds(), recreations)
	} else {
		log.Printf("кадров не отрисовано (окно закрыто слишком рано)")
	}
	if a.resizeTest && recreations < 2 {
		return fmt.Errorf("resize-test: пересозданий swapchain=%d, ожидалось минимум 2", recreations)
	}
	return api.DeviceWaitIdle(device)
}

// heldKeys перечисляет зажатые клавиши (для камеры: WASD).
func heldKeys(held map[window.Key]bool) []window.Key {
	if len(held) == 0 {
		return nil
	}
	out := make([]window.Key, 0, len(held))
	for k := range held {
		out = append(out, k)
	}
	return out
}

// resizeTo меняет размер окна и печатает результат (для resize-test).
func resizeTo(win *window.Window, w, h int) {
	if err := win.SetSize(w, h); err != nil {
		log.Printf("resize-test: %v", err)
		return
	}
	log.Printf("resize-test: окно → %dx%d", w, h)
}

// drawCursor рисует крестик-курсор на позиции мыши.
//
// Канва: тёмная окантовка под белым крестиком — курсор виден на любом фоне.
// Прямоугольники идут через UI-пайплайн (белый пиксель атласа).
func drawCursor(d *ui.Draw, x, y int) {
	const (
		arm    = 8 // длина луча крестика, px
		thick  = 2 // толщина луча
		margin = 1 // чёрная окантовка, px
	)
	fx, fy := float32(x), float32(y)
	outline := [4]float32{0, 0, 0, 1}
	white := [4]float32{1, 1, 1, 1}

	d.Rect(fx-arm-margin, fy-thick/2-margin, 2*arm+2*margin+1, thick+2*margin, outline)
	d.Rect(fx-thick/2-margin, fy-arm-margin, thick+2*margin, 2*arm+2*margin+1, outline)
	d.Rect(fx-arm, fy-thick/2, 2*arm+1, thick, white)
	d.Rect(fx-thick/2, fy-arm, thick, 2*arm+1, white)
}

// animateColor — плавная смена цвета заливки (HSV → RGB).
func animateColor(t time.Duration) [4]float32 {
	const speed = 0.15 // оборотов в секунду
	phase := t.Seconds() * speed
	h := phase - float64(int(phase))
	return hsv(h, 0.65, 0.75)
}

func hsv(h, s, v float64) [4]float32 {
	i := int(h * 6)
	f := h*6 - float64(i)
	p := v * (1 - s)
	q := v * (1 - f*s)
	t := v * (1 - (1-f)*s)
	var r, g, b float64
	switch i % 6 {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	default:
		r, g, b = v, p, q
	}
	return [4]float32{float32(r), float32(g), float32(b), 1}
}

func formatName(f vulkan.Format) string {
	switch f {
	case vulkan.FormatB8G8R8A8SRGB:
		return "B8G8R8A8_SRGB"
	case vulkan.FormatB8G8R8A8UNORM:
		return "B8G8R8A8_UNORM"
	case vulkan.FormatR8G8B8A8SRGB:
		return "R8G8B8A8_SRGB"
	case vulkan.FormatR8G8B8A8UNORM:
		return "R8G8B8A8_UNORM"
	default:
		return fmt.Sprintf("format(%d)", int32(f))
	}
}

func formatAPIVersion(v uint32) string {
	return fmt.Sprintf("Vulkan %d.%d.%d", (v>>22)&0x7F, (v>>12)&0x3FF, v&0xFFF)
}
