package scenes

import (
	"fmt"
	"strings"
	"time"

	"dow-mod-studio/sgbview"
	"github.com/Anykey-Nomad/Open-Kaurava/gfx3d"
	"github.com/Anykey-Nomad/Open-Kaurava/mods"
	"github.com/Anykey-Nomad/Open-Kaurava/scene"
	"github.com/Anykey-Nomad/Open-Kaurava/vulkan"
	"github.com/Anykey-Nomad/Open-Kaurava/window"
)

// Preview — 3D-просмотр выбранного сценария: рельеф карты с текстурой
// земли и объекты, расставленные на нём.
//
// Сценарий читается в фоне: разбор .sgb, сборка текстуры земли и загрузка
// моделей объектов занимают секунды, а окно не должно замирать. Готовая
// сцена попадает в видеопамять в главном потоке (см. updateLoad).
type Preview struct {
	lib   *mods.Library
	world *gfx3d.Renderer
	cam   gfx3d.Camera

	// want — путь сценария, который грузится (или уже загружен).
	want string
	// loaded — путь сценария, который уже в видеопамяти.
	loaded string
	// builds — подпись выбора игроков: по ней видно, что постройки
	// изменились и сцену надо пересобрать.
	builds string
	// pending — канал с результатом фоновой загрузки (nil — загрузки нет).
	pending chan *previewResult
	// loading — идёт загрузка.
	loading bool
	// started — когда началась текущая загрузка.
	started time.Time
	// err — ошибка последней загрузки.
	err error
	// info — сводка по загруженной сцене.
	info string
	// warn — что не удалось показать: пропущенные объекты, обрезанная сцена.
	warn string
}

// previewResult — результат фоновой загрузки сценария.
type previewResult struct {
	scene *sgbview.Scene
	err   error
}

// NewPreview создаёт сцену 3D-просмотра. world — рендерер, в который
// кладётся собранная сцена сценария.
func NewPreview(lib *mods.Library, world *gfx3d.Renderer) *Preview {
	return &Preview{
		lib:   lib,
		world: world,
		cam:   gfx3d.NewCamera(),
	}
}

// Name — имя сцены.
func (p *Preview) Name() string { return "preview" }

// Update обрабатывает загрузку сценария и управление камерой.
func (p *Preview) Update(c *scene.Ctx, dt float64) {
	p.updateLoad(c)
	p.updateCamera(c, dt)
}

// updateLoad забирает готовый результат и запускает загрузку нового
// сценария. Результат обрабатывается в главном потоке: загрузка в
// видеопамять — это команды GPU, их нельзя разносить по потокам.
func (p *Preview) updateLoad(c *scene.Ctx) {
	if p.pending != nil {
		select {
		case res := <-p.pending:
			p.pending = nil
			p.loading = false
			if res.err != nil {
				p.err = res.err
				return
			}
			if p.world == nil {
				p.err = fmt.Errorf("3D-рендерер не создан")
				return
			}
			if err := p.world.UploadScene(res.scene); err != nil {
				p.err = err
				return
			}
			p.loaded = p.want
			p.err = nil
			p.cam.FitToScene(res.scene.GroundMin, res.scene.GroundMax,
				res.scene.CenterHeight, res.scene.CellSize)
			p.info = fmt.Sprintf("объектов: %d, вершин: %d, ячеек: %g м",
				len(res.scene.Objects), countVertices(res.scene), res.scene.CellSize)
			p.warn = sceneWarnings(res.scene)
		default:
		}
	}

	// Сценарий или выбор игроков изменились — грузим заново.
	if p.want != SelectedPath || p.builds != Setup.Key() {
		p.want = SelectedPath
		p.builds = Setup.Key()
		p.err = nil
		p.info = ""
		p.warn = ""
		p.startLoad()
	}
}

// startLoad запускает фоновую загрузку выбранного сценария.
func (p *Preview) startLoad() {
	p.loading = false
	p.pending = nil
	if p.lib == nil || SelectedPath == "" {
		return
	}
	tree := p.lib.Tree()
	if tree == nil {
		return // дерево модуля ещё строится — вернёмся к нему позже
	}
	path := SelectedPath
	ch := make(chan *previewResult, 1)
	p.pending = ch
	p.loading = true
	p.started = time.Now()
	go func() {
		// Стартовые постройки игроков ставятся на их позиции: без
		// них карта выглядит пустой, хотя в игре на старте у каждого
		// игрока есть база.
		sc, err := sgbview.LoadWith(tree, path, sgbview.DefaultLimits(), Setup.Builds)
		ch <- &previewResult{scene: sc, err: err}
	}()
}

// updateCamera вращает и двигает камеру, читает клавиши.
func (p *Preview) updateCamera(c *scene.Ctx, dt float64) {
	// Левая кнопка — вращение вокруг цели, правая — сдвиг точки обзора.
	if c.Input.Buttons&(1<<1) != 0 {
		p.cam.Orbit(float64(c.Input.MoveX), float64(c.Input.MoveY))
	}
	if c.Input.Buttons&(1<<3) != 0 {
		p.cam.Pan(float64(c.Input.MoveX), float64(c.Input.MoveY), c.H)
	}
	if c.Input.Scroll != 0 {
		p.cam.Zoom(c.Input.Scroll)
	}

	// WASD — движение по карте.
	var forward, strafe float64
	for _, k := range c.Input.Held {
		switch k {
		case window.KeyW:
			forward++
		case window.KeyS:
			forward--
		case window.KeyA:
			strafe--
		case window.KeyD:
			strafe++
		}
	}
	p.cam.Move(forward, strafe, dt)

	if c.Key(window.KeyEscape) {
		c.Switch("players")
	}
}

// Record3D рисует 3D-сцену (вызывается движком до интерфейса).
func (p *Preview) Record3D(cmd vulkan.CommandBuffer, width, height float32) error {
	if p.world == nil || !p.world.HasMeshes() {
		return nil
	}
	return p.world.Record(cmd, &p.cam, width, height)
}

// Draw рисует интерфейс поверх 3D-сцены.
func (p *Preview) Draw(c *scene.Ctx) {
	// Фон кадра: тёмное небо, пока карта не загружена.
	c.Clear = [4]float32{0.05, 0.06, 0.09, 1}

	if p.world == nil || !p.world.HasMeshes() {
		c.Rect(0, 0, float32(c.W), float32(c.H), c.Clear)
		if p.err != nil {
			c.CenterText("не удалось загрузить сценарий", float32(c.H)/2-20, 26, ColTextWarn)
			c.CenterText(p.err.Error(), float32(c.H)/2+16, 18, ColTextDim)
		} else {
			c.CenterText("загрузка сценария…", float32(c.H)/2, 26, ColText)
			if p.loading {
				c.CenterText(fmt.Sprintf("%.1f с", time.Since(p.started).Seconds()),
					float32(c.H)/2+36, 18, ColTextDim)
			}
		}
		c.CenterText("ESC — к выбору игроков", float32(c.H)-36, 18, ColTextDim)
		return
	}

	p.drawInfo(c)
	p.drawPlayers(c)
	p.drawHelp(c)
}

// drawPlayers — панель со списком игроков: кто за кем играет и какие
// стартовые постройки на карте.
func (p *Preview) drawPlayers(c *scene.Ctx) {
	slots := Setup.SortedSlots()
	if len(slots) == 0 {
		return
	}
	const w = 460
	rows := len(slots)
	if rows > 8 {
		rows = 8
	}
	h := float32(46 + rows*22)
	y := float32(16 + 104 + 12)
	if p.warn != "" {
		y += 24
	}
	c.Rect(16, y, w, h, [4]float32{0, 0, 0, 0.45})
	c.Rect(16, y, 3, h, ColAccent)
	c.Text("Игроки", 32, y+28, 20, ColText)
	for i := 0; i < rows; i++ {
		s := slots[i]
		label := fmt.Sprintf("%d — %s", s.Owner, Setup.RaceLabel(s.Owner))
		col := ColTextDim
		if s.Owner == Setup.Own {
			label = "★ " + label
			col = ColTextWarn
		}
		c.Text(label, 32, y+50+float32(i)*22, 16, col)
	}
}

// drawInfo — панель со сведениями о сценарии (слева сверху).
func (p *Preview) drawInfo(c *scene.Ctx) {
	const w = 460
	h := float32(104)
	if p.warn != "" {
		h = 128
	}
	c.Rect(16, 16, w, h, [4]float32{0, 0, 0, 0.45})
	c.Rect(16, 16, 3, h, ColAccent)

	name := Selected
	if name == "" {
		name = "не выбран"
	}
	c.Text(name, 32, 44, 24, ColText)
	if SelectedPath != "" {
		c.Text(SelectedPath, 32, 70, 16, ColTextDim)
	}
	if p.info != "" {
		c.Text(p.info, 32, 94, 16, ColTextDim)
	}
	if p.warn != "" {
		c.Text(p.warn, 32, 118, 16, ColTextWarn)
	}
}

// sceneWarnings собирает строку о том, что сцена показана не вся: часть
// объектов не загрузилась или не поместилась в лимиты.
func sceneWarnings(sc *sgbview.Scene) string {
	if sc == nil {
		return ""
	}
	var parts []string
	if sc.Dropped > 0 {
		parts = append(parts, fmt.Sprintf("не показано объектов: %d", sc.Dropped))
	}
	if len(sc.Skipped) > 0 {
		parts = append(parts, fmt.Sprintf("не загружено объектов: %d", len(sc.Skipped)))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ", ")
}

// drawHelp — подсказка по управлению и положение камеры.
func (p *Preview) drawHelp(c *scene.Ctx) {
	const w = 460
	h := float32(76)
	y := float32(c.H) - h - 16
	c.Rect(16, y, w, h, [4]float32{0, 0, 0, 0.45})
	c.Text("ЛКМ — вращение, ПКМ — сдвиг, колесо — зум, WASD — ходить, ESC — назад",
		32, y+28, 16, ColTextDim)
	c.Text(fmt.Sprintf("азимут %.0f°, наклон %.0f°, дальность %.0f м",
		radToDeg(p.cam.Yaw), radToDeg(p.cam.Pitch), p.cam.Distance),
		32, y+54, 16, ColTextDim)
}

// countVertices считает вершины во всех мешах сцены.
func countVertices(sc *sgbview.Scene) int {
	if sc == nil {
		return 0
	}
	n := 0
	if sc.Terrain != nil {
		n += len(sc.Terrain.Vertices)
	}
	if sc.Water != nil {
		n += len(sc.Water.Vertices)
	}
	for _, o := range sc.Objects {
		for _, p := range o.Parts {
			if p.Mesh != nil {
				n += len(p.Mesh.Vertices)
			}
		}
	}
	return n
}

// radToDeg переводит радианы в градусы.
func radToDeg(r float64) float64 { return r * 180 / 3.141592653589793 }
