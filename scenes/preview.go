package scenes

import (
	"fmt"
	"github.com/Anykey-Nomad/Open-Kaurava/loc"
	"sort"
	"strings"
	"time"

	"dow-mod-studio/sgbview"
	"github.com/Anykey-Nomad/Open-Kaurava/fx"
	"github.com/Anykey-Nomad/Open-Kaurava/gfx3d"
	"github.com/Anykey-Nomad/Open-Kaurava/mods"
	"github.com/Anykey-Nomad/Open-Kaurava/rgdmeta"
	"github.com/Anykey-Nomad/Open-Kaurava/scene"
	"github.com/Anykey-Nomad/Open-Kaurava/sfx"
	"github.com/Anykey-Nomad/Open-Kaurava/vulkan"
	"github.com/Anykey-Nomad/Open-Kaurava/window"
)

// Gameplay — игровая сцена: рельеф карты с текстурой земли и
// объекты выбранного сценария.
//
// Сценарий читается в фоне: разбор .sgb, сборка текстуры земли и загрузка
// моделей объектов занимают секунды, а окно не должно замирать. Готовая
// сцена попадает в видеопамять в главном потоке (см. updateLoad).
type Gameplay struct {
	lib   *mods.Library
	world *gfx3d.Renderer
	cam   gfx3d.Camera
	hud   *GameplayHUD
	// music — играющий плейлист музыки боя (scenes/music.go).
	music *sfx.MusicPlaylist
	// pause — меню паузы поверх боя: затемнение, пункты и настройки звука
	// (scenes/pause_menu.go).
	pause pauseMenu

	// want — путь сценария, который грузится (или уже загружен).
	want string
	// loaded — путь сценария, который уже в видеопамяти.
	loaded string
	// builds — подпись выбора игроков: по ней видно, что постройки
	// изменились и сцену надо пересобрать.
	builds string
	// pending — канал с результатом фоновой загрузки (nil — загрузки нет).
	pending chan *sceneLoadResult
	// loading — идёт загрузка.
	loading bool
	// started — когда началась текущая загрузка.
	started time.Time
	// err — ошибка последней загрузки.
	err error
	// info — сводка по загруженной сцене.
	info     string
	heights  [][]float32
	heightXs []float64
	heightYs []float64
	objects  []gameplayObject
	// sites — заложенные площадки: по ним работают строители, и они же
	// рисуются коробкой, пока меша новой постройки нет (construction.go).
	sites map[string]*constructionSite
	// place — «призрак» постройки при выборе места.
	place placementState
	// siteModels — фоновая загрузка моделей заложенных построек: пока
	// модель не пришла, площадка рисуется коробкой (site_models.go).
	siteModels chan siteModelResult
	// unitModels — то же для нанятых бойцов; modelSerial — номер загрузки:
	// поднятое тело читает модель заново, и её имя меша не должно совпасть
	// со старым (иначе рендерер двигал бы и скрытый труп).
	unitModels  chan unitModelResult
	modelSerial int
	// queues — очереди найма по ID постройки: первая запись нанимается,
	// остальные (до шести) ждут (training.go).
	queues map[string]*trainQueue
	// squads — состав нанятых отрядов (ключ → ID бойцов), entitySquad —
	// обратная связь по бойцу: по ним выделение берёт отряд целиком.
	squads      map[string][]string
	entitySquad map[string]string
	nextSquad   int
	// harvest — собранные тела по ID томб-спайдера, cooldowns — остатки
	// перезарядки способностей по ключу "ID|чертёж" (corpses.go).
	harvest   map[string]*harvestState
	cooldowns map[string]float64
	// capture — фаза анимации каждой точки захвата (capture.go).
	capture map[string]*captureVisual
	// reinforce — идущее пополнение отрядов по SquadID (reinforce.go).
	reinforce map[string]*reinforceTask
	// fx — частицы эффектов (вспышки, дым, пыль); fxShots — последние
	// увиденные счётчики выстрелов по объектам, fxClock — таймер
	// дымовых проверок (см. fx.go).
	fx      *fx.System
	fxShots map[string]uint64
	// shotFX — играющие сейчас эффекты выстрелов из настоящих файлов
	// DATA:ART/FX: каждый живёт от своего выстрела до конца клипа.
	shotFX []activeShotFX
	// shotDefs — эффекты выстрела по имени меша объекта, fxFiles — уже
	// разобранные файлы эффектов: один файл обслуживает много выстрелов.
	shotDefs map[string][]sgbview.ShotFX
	fxFiles  map[string]*fx.File
	fxClock  float64
	// pass — карта проходимости загруженного сценария (DATAIMAP):
	// по ней строится маршрут приказов движения (см. path.go).
	pass *sgbview.PassMap
	// warn — что не удалось показать: пропущенные объекты, обрезанная сцена.
	warn string
}

// sceneLoadResult — результат фоновой загрузки сценария.
type sceneLoadResult struct {
	scene *sgbview.Scene
	// collide — sim_entity_ext/is_collide из чертежей объектов: от него
	// зависит, можно ли выделить объект (ключи — rgdmeta.Key).
	collide map[string]bool
	// attrib — чертежи ebps/sbps объектов сценария и лимиты рас; читаются
	// там же, в фоне.
	attrib *attribData
	err    error
}

// NewGameplay создаёт игровую сцену. world — рендерер, в который кладётся
// собранная карта и её объекты.
func NewGameplay(lib *mods.Library, world *gfx3d.Renderer) *Gameplay {
	return &Gameplay{
		lib:         lib,
		world:       world,
		cam:         gfx3d.NewCamera(),
		hud:         newGameplayHUD(lib),
		sites:       map[string]*constructionSite{},
		queues:      map[string]*trainQueue{},
		squads:      map[string][]string{},
		entitySquad: map[string]string{},
		harvest:     map[string]*harvestState{},
		cooldowns:   map[string]float64{},
		capture:     map[string]*captureVisual{},
		fx:          fx.New(),
		fxShots:     map[string]uint64{},
		shotDefs:    map[string][]sgbview.ShotFX{},
		fxFiles:     map[string]*fx.File{},
	}
}

// Name — имя сцены.
func (p *Gameplay) Name() string { return "gameplay" }

// Update обрабатывает загрузку сценария и управление камерой.
func (p *Gameplay) Update(c *scene.Ctx, dt float64) {
	p.updateLoad(c)
	// Музыка идёт своим ходом: трек доигрывает, затем пауза из плейлиста.
	p.updateMusic(dt)
	// Меню паузы забирает ввод целиком: пока оно открыто, камера и HUD
	// кадр не обрабатывают.
	if p.updatePauseMenu(c) {
		return
	}
	p.updateCamera(c, dt)
	if p.hud != nil {
		p.hud.pickWorld = p.pickWorldObject
		p.hud.stopMusic = p.stopMusic
		p.hud.openMenu = p.openPauseMenu
		p.hud.projectToScreen = p.cam.Project
		p.hud.orderAt = p.issueCommandOrder
		p.hud.trainAt = p.trainSquad
		p.hud.queueOf = p.queueOf
		p.hud.squadOf = p.squadMembers
		p.hud.resurrectAt = p.resurrectAt
		p.hud.summonAt = p.summonAt
		p.hud.harvestCounts = p.harvestCounts
		p.hud.reinforceAt = p.reinforceAt
		p.hud.reinforceState = p.reinforceState
		// Ресурсы копятся по тику кадра, пока сцена не на паузе.
		if !p.hud.paused && p.hud.factory != nil {
			p.hud.factory.Tick(dt)
		}
		p.hud.updateHUD(c)
		// Кнопка МЕНЮ могла открыть паузу — остальной кадр не обрабатываем.
		if p.pause.open {
			return
		}
		// Размещение постройки идёт до приказов: в этом режиме клик по карте
		// ставит площадку, а не меняет выделение.
		p.updatePlacement(c)
		// Приказы правой кнопкой — при выделенном юните; после движения
		// фабрики сдвинувшиеся мешы переносятся в новые координаты.
		p.updateOrders(c)
		// Модели построек, дочитанные в фоне, входят в сцену до расчёта
		// прогресса: первый же кадр ставит клип стройки на нужную стадию.
		p.updateSiteModels()
		p.updateUnitModels()
		p.updateTraining(dt)
		p.updateReinforce(dt)
		p.updateConstruction(dt)
		// Сбор тел томб-спайдерами — до переноса мешей: забранное тело должно
		// исчезнуть в этом же кадре.
		p.updateHarvest(dt)
		p.updateAbilityCooldowns(dt)
		p.syncMovedObjects(dt)
		// Точки захвата рисуются клипами флага: процент захвата задаёт кадр.
		p.updateCapturePoints(dt)
		p.updateFX(dt)
		// Анимации объектов: на паузе клипы стоят (dt = 0), но палитра
		// всё равно перезаливается — приказ мог сменить состояние.
		if p.world != nil {
			animDT := dt
			if p.hud.paused {
				animDT = 0
			}
			_ = p.world.StepAnimations(animDT)
		}
	}
}

// updateLoad забирает готовый результат и запускает загрузку нового
// сценария. Результат обрабатывается в главном потоке: загрузка в
// видеопамять — это команды GPU, их нельзя разносить по потокам.
func (p *Gameplay) updateLoad(c *scene.Ctx) {
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
				p.err = fmt.Errorf("%s", loc.T("3D renderer not created"))
				return
			}
			// Движущиеся объекты: рендерер копирует их вершины, чтобы
			// сдвигать меш по приказу без перезагрузки всей сцены.
			p.world.SetDynamicObjects(dynamicObjectNames(res.scene))
			if err := p.world.UploadScene(res.scene); err != nil {
				p.err = err
				return
			}
			p.loaded = p.want
			p.err = nil
			// Эффекты прошлой карты не должны переехать на новую.
			if p.fx != nil {
				p.fx.Clear()
			}
			p.fxShots = map[string]uint64{}
			// Музыка боя — по загруженной карте (см. scenes/music.go).
			p.startMusic()
			p.cam.FitToScene(res.scene.GroundMin, res.scene.GroundMax,
				res.scene.CenterHeight, res.scene.CellSize)
			p.heights, p.heightXs, p.heightYs = sceneHeightGrid(res.scene.Terrain)
			p.cam.SetGroundHeight(p.groundHeight)
			if start, ok := playerStartingPoint(res.scene.Players, Setup.Own); ok {
				// StartingPosition хранит координаты сценария: X/Z — плоскость
				// карты, Y — высота. Мир движка Z-up, поэтому целью камеры
				// становится (X, Z).
				p.cam.Target[0], p.cam.Target[1] = float64(start.X), float64(start.Z)
			}
			p.cam.UpdateTargetHeight()
			drawScene := sceneWithoutStartingMarkers(res.scene)
			if drawScene != res.scene {
				if err := p.world.UploadScene(drawScene); err != nil {
					p.err = err
					return
				}
			}
			p.hud.attribData = res.attrib
			p.pass = res.scene.Pass
			p.objects = p.buildGameplayObjects(res.scene, res.collide)
			// Миникарта: снимок лежит рядом со сценарием. Панель показывает
			// только играбельную площадь (DATAIMAP) — за её границами
			// ставить и ходить нельзя; границы земли нужны, чтобы вырезать
			// эту площадь из снимка.
			p.hud.minimapPath = p.loaded
			p.hud.minimapGroundMin = res.scene.GroundMin
			p.hud.minimapGroundMax = res.scene.GroundMax
			p.hud.minimapMin = res.scene.GroundMin
			p.hud.minimapMax = res.scene.GroundMax
			if pass := res.scene.Pass; pass != nil && pass.W > 0 && pass.H > 0 {
				p.hud.minimapMin = [2]float32{
					float32(pass.OriginX),
					float32(pass.OriginY),
				}
				p.hud.minimapMax = [2]float32{
					float32(pass.OriginX + float64(pass.W)*pass.CellX),
					float32(pass.OriginY + float64(pass.H)*pass.CellY),
				}
			}
			// Состояние точки захвата, сбор тел и перезарядки привязаны к ID
			// сущностей, а новая сцена начинает нумерацию заново: старое
			// состояние к ней не относится.
			p.capture = map[string]*captureVisual{}
			p.reinforce = map[string]*reinforceTask{}
			p.harvest = map[string]*harvestState{}
			p.cooldowns = map[string]float64{}
			p.info = fmt.Sprintf(loc.T("objects: %d, vertices: %d, cell: %g m"),
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
func (p *Gameplay) startLoad() {
	p.loading = false
	p.pending = nil
	if p.lib == nil || SelectedPath == "" {
		return
	}
	tree := p.lib.Tree()
	if tree == nil {
		return // дерево модуля ещё строится — вернёмся к нему позже
	}
	// Карты эффектов (Data/art/fx) рендерер читает из того же дерева, что
	// и сценарий: без него частицы падают обратно на встроенный круг.
	p.world.SetTextureTree(tree)
	path := SelectedPath
	builds := append([]sgbview.SlotBuild(nil), Setup.Builds...)
	// Кому какая раса досталась: папку расы слота считаем здесь, в основном
	// потоке (глобальный Setup), а чертежи читаем уже в фоне.
	var raceSlots []slotRace
	for _, slot := range Setup.SortedSlots() {
		raceSlots = append(raceSlots, slotRace{slot: slot.Owner, folder: raceFolderOf(slot.Owner)})
	}
	lib := p.lib
	ch := make(chan *sceneLoadResult, 1)
	p.pending = ch
	p.loading = true
	p.started = time.Now()
	go func() {
		// Стартовые постройки игроков ставятся на их позиции: без
		// них карта выглядит пустой, хотя в игре на старте у каждого
		// игрока есть база. Стартовые бойцы (обычно строитель, у
		// некронов — ещё «трупы» лорда и томб-спайдеров) встают
		// вокруг этой постройки.
		units := startingUnitsFor(tree, raceSlots)
		sc, err := sgbview.LoadWith(tree, path, sgbview.DefaultLimits(), builds, units...)
		var collide map[string]bool
		var data *attribData
		if err == nil && sc != nil {
			// is_collide читается из чертежей здесь же, в фоне: основной
			// поток не должен читать сотни файлов на кадр.
			names := make([]string, 0, len(sc.Objects))
			for _, object := range sc.Objects {
				names = append(names, object.Name)
			}
			collide = rgdmeta.CollideMap(tree, names)
			// Как и is_collide, чертежи ebps/sbps читаются здесь: панель
			// свойств показывает здоровье, размер отряда и право захвата.
			data = loadAttribData(tree, sc, lib)
		}
		ch <- &sceneLoadResult{scene: sc, collide: collide, attrib: data, err: err}
	}()
}

// updateCamera вращает камеру при зажатом Alt и панорамирует правой кнопкой.
func (p *Gameplay) updateCamera(c *scene.Ctx, dt float64) {
	altDown := false
	for _, key := range c.Input.Held {
		if key == window.KeyLeftAlt || key == window.KeyRightAlt {
			altDown = true
			break
		}
	}
	if altDown && (c.Input.MoveX != 0 || c.Input.MoveY != 0) {
		p.cam.Orbit(float64(c.Input.MoveX), float64(c.Input.MoveY))
	}
	// Панорамирование — средняя кнопка: правая отдана приказам движения.
	if !altDown && c.Input.Buttons&(1<<2) != 0 {
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

	p.cam.UpdateTargetHeight()
	if c.Key(window.KeyEscape) {
		c.Switch("players")
	}
}

// Record3D рисует 3D-сцену (вызывается движком до интерфейса).
func (p *Gameplay) Record3D(cmd vulkan.CommandBuffer, width, height float32) error {
	if p.world == nil || !p.world.HasMeshes() {
		return nil
	}
	// Частицы собираются в меши кадра здесь же: их вершины зависят от
	// камеры, поэтому их нельзя подготовить заранее.
	if p.fx != nil {
		eye := p.cam.Eye()
		// Сначала общие частицы сцены (дым, пыль, искры), затем эффекты
		// выстрелов из DATA:ART/FX со своими лентами и картами.
		particles := append([]fx.Particle(nil), p.fx.Particles()...)
		shotParticles, quads := p.shotEffectFrame(eye)
		particles = append(particles, shotParticles...)
		p.world.SetFX(particles, quads, eye)
	}
	return p.world.Record(cmd, &p.cam, width, height)
}

// Draw рисует интерфейс поверх 3D-сцены.
func (p *Gameplay) Draw(c *scene.Ctx) {
	// Фон кадра: тёмное небо, пока карта не загружена.
	c.Clear = [4]float32{0.05, 0.06, 0.09, 1}

	if p.world == nil || !p.world.HasMeshes() {
		c.Rect(0, 0, float32(c.W), float32(c.H), c.Clear)
		if p.err != nil {
			c.CenterText(loc.T("failed to load the scenario"), float32(c.H)/2-20, 26, ColTextWarn)
			c.CenterText(p.err.Error(), float32(c.H)/2+16, 18, ColTextDim)
		} else {
			c.CenterText(loc.T("loading scenario…"), float32(c.H)/2, 26, ColText)
			if p.loading {
				c.CenterText(fmt.Sprintf(loc.T("%.1f s"), time.Since(p.started).Seconds()),
					float32(c.H)/2+36, 18, ColTextDim)
			}
		}
		c.CenterText(loc.T("ESC — pause menu"), float32(c.H)-36, 18, ColTextDim)
		return
	}

	if p.hud != nil {
		drawGameplayObjects(c, &p.cam, p.objects, p.hud.selectionSet())
		// «Призрак» постройки под курсором и коробки заложенных площадок.
		p.drawPlacement(c)
		// Весь арт интерфейса берётся из .screen одним обходом: панели, меню
		// и рамки расставлены в самом файле.
		p.hud.drawScreenArt(c)
		p.hud.drawTopPanel(c)
		p.hud.drawBottomPanels(c)
		p.hud.drawDragBox(c)
	}
	// Меню паузы рисуется поверх всего интерфейса.
	p.drawPauseMenu(c)
}

// sceneWarnings собирает строку о том, что сцена показана не вся: часть
// объектов не загрузилась или не поместилась в лимиты.
func sceneWarnings(sc *sgbview.Scene) string {
	if sc == nil {
		return ""
	}
	var parts []string
	if sc.Dropped > 0 {
		parts = append(parts, fmt.Sprintf(loc.T("objects not shown: %d"), sc.Dropped))
	}
	if len(sc.Skipped) > 0 {
		parts = append(parts, fmt.Sprintf(loc.T("objects not loaded: %d"), len(sc.Skipped)))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ", ")
}

// drawHelp — подсказка по управлению и положение камеры.
func (p *Gameplay) drawHelp(c *scene.Ctx) {
	const w = 460
	h := float32(76)
	y := float32(c.H) - h - 16
	c.Rect(16, y, w, h, [4]float32{0, 0, 0, 0.45})
	c.Text(loc.T("Alt+mouse — rotate, RMB — pan, LMB — select, wheel — zoom, WASD — move"),
		32, y+28, 16, ColTextDim)
	c.Text(fmt.Sprintf(loc.T("azimuth %.0f°, pitch %.0f°, distance %.0f m"),
		radToDeg(p.cam.Yaw), radToDeg(p.cam.Pitch), p.cam.Distance),
		32, y+54, 16, ColTextDim)
}

// countVertices считает вершины во всех мешах сцены.
func sceneHeightGrid(terrain *sgbview.Mesh) ([][]float32, []float64, []float64) {
	if terrain == nil || len(terrain.Vertices) < 4 {
		return nil, nil, nil
	}
	xSet, ySet := make(map[float32]struct{}), make(map[float32]struct{})
	for _, vertex := range terrain.Vertices {
		xSet[vertex.Pos[0]] = struct{}{}
		ySet[vertex.Pos[1]] = struct{}{}
	}
	xs, ys := make([]float32, 0, len(xSet)), make([]float32, 0, len(ySet))
	for x := range xSet {
		xs = append(xs, x)
	}
	for y := range ySet {
		ys = append(ys, y)
	}
	sort.Slice(xs, func(i, j int) bool { return xs[i] < xs[j] })
	sort.Slice(ys, func(i, j int) bool { return ys[i] < ys[j] })
	if len(xs) < 2 || len(ys) < 2 || len(xs)*len(ys) > len(terrain.Vertices)*2 {
		return nil, nil, nil
	}
	xIndex, yIndex := make(map[float32]int, len(xs)), make(map[float32]int, len(ys))
	for i, x := range xs {
		xIndex[x] = i
	}
	for i, y := range ys {
		yIndex[y] = i
	}
	grid := make([][]float32, len(ys))
	for y := range grid {
		grid[y] = make([]float32, len(xs))
	}
	for _, vertex := range terrain.Vertices {
		grid[yIndex[vertex.Pos[1]]][xIndex[vertex.Pos[0]]] = vertex.Pos[2]
	}
	outX, outY := make([]float64, len(xs)), make([]float64, len(ys))
	for i, x := range xs {
		outX[i] = float64(x)
	}
	for i, y := range ys {
		outY[i] = float64(y)
	}
	return grid, outX, outY
}

func interpolateHeight(axis []float64, index int, fraction float64) (int, int, float64) {
	if index >= len(axis)-1 {
		return len(axis) - 1, len(axis) - 1, 0
	}
	span := axis[index+1] - axis[index]
	if span <= 0 {
		return index, index, 0
	}
	return index, index + 1, fraction / span
}

func playerStartingPoint(slots []sgbview.PlayerSlot, owner int) (sgbview.StartingPosition, bool) {
	for _, slot := range slots {
		if slot.Owner == owner && len(slot.Positions) > 0 {
			return slot.Positions[0], true
		}
	}
	return sgbview.StartingPosition{}, false
}

func (p *Gameplay) groundHeight(x, y float64) float64 {
	if len(p.heights) < 2 || len(p.heightXs) < 2 || len(p.heightYs) < 2 {
		return 0
	}
	locate := func(axis []float64, value float64) (int, float64) {
		if value <= axis[0] {
			return 0, 0
		}
		if value >= axis[len(axis)-1] {
			return len(axis) - 1, 0
		}
		index := sort.Search(len(axis), func(i int) bool { return axis[i] >= value }) - 1
		return index, value - axis[index]
	}
	xi, dx := locate(p.heightXs, x)
	yi, dy := locate(p.heightYs, y)
	x0, x1, tx := interpolateHeight(p.heightXs, xi, dx)
	y0, y1, ty := interpolateHeight(p.heightYs, yi, dy)
	a := float64(p.heights[y0][x0])*(1-tx) + float64(p.heights[y0][x1])*tx
	b := float64(p.heights[y1][x0])*(1-tx) + float64(p.heights[y1][x1])*tx
	return a*(1-ty) + b*ty
}

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
