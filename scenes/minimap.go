package scenes

import (
	"image"
	"image/color"
	"math"
	"strings"

	"github.com/Anykey-Nomad/Open-Kaurava/entities"
	"github.com/Anykey-Nomad/Open-Kaurava/scene"
	"github.com/Anykey-Nomad/Open-Kaurava/screenui"
	"github.com/Anykey-Nomad/Open-Kaurava/ui"
)

// Миникарта в левой панели.
//
// Подложка — TGA-карта сценария: рядом с каждым .sgb лежит <карта>_mm.tga,
// тот самый снимок карты сверху, который игра кладёт под точки. Поверх него
// рисуются квадратные точки: свои отряды и постройки — зелёным, вражеские —
// красным, союзные — светло-синим, точки захвата — по типу, выделенное —
// белым.
//
// Игра показывает карту ромбом и только в границах играбельной площади:
// снимок поворачивается на 45° против часовой стрелки, а из него
// вырезается DATAIMAP — за её краем нельзя ни строить, ни ходить (там
// только декорация, которую видно с камеры). Точки поворачиваются тем же
// преобразованием, что и картинка, — иначе они съедут с объектов.
// minimapAngle — на сколько карта повёрнута в панели: на 45° (ромбом, как
// в оригинале). Тем же углом поворачиваются точки (см. minimapPoint),
// поэтому картинка и точки всегда ходят вместе: направление задаётся
// знаком угла.
const minimapAngle = -math.Pi / 4

var (
	// Свои отряды и постройки.
	minimapOwnColor = [4]float32{0.32, 0.84, 0.36, 1}
	// Вражеские отряды и постройки.
	minimapEnemyColor = [4]float32{0.90, 0.24, 0.22, 1}
	// Союзники: пока в модели игроков нет команд, ветка не срабатывает,
	// но цвет уже на месте.
	minimapAllyColor = [4]float32{0.45, 0.78, 1, 1}
	// Реликтовые точки — светло-оранжевый, почти жёлтый.
	minimapRelicColor = [4]float32{1, 0.78, 0.30, 1}
	// Стратегические позиции — тёмно-фиолетовый.
	minimapPointColor = [4]float32{0.42, 0.20, 0.56, 1}
	// Strategic objectives — тёмно-оранжевый.
	minimapObjectiveColor = [4]float32{0.85, 0.44, 0.10, 1}
	// Slag deposits — светло-фиолетовый.
	minimapSlagColor = [4]float32{0.74, 0.55, 0.96, 1}
	// Выделенное сейчас — белым поверх остальных точек.
	minimapSelectedColor = [4]float32{1, 1, 1, 1}
)

// minimapDot — одна точка на миникарте: пиксели панели и цвет.
type minimapDot struct {
	x, y  float32
	color [4]float32
	// selected — точка выделенного отряда: рисуется последней, белым.
	selected bool
}

// drawMinimap рисует карту и точки поверх неё в слоте ctmMinimap внутри
// grpMinimap. Сам grpMinimap — это вся левая панель вместе с рамкой: если
// заливать её целиком, карта занимает вдвое больше места, чем в игре.
func (g *GameplayHUD) drawMinimap(c *scene.Ctx) {
	if g.screen == nil || c.UI == nil || !g.screenWidgetVisible(screenMinimap) {
		return
	}
	viewport := screenui.Rect{W: float32(c.W), H: float32(c.H)}
	r, ok := g.screen.Layout(screenMinimap, viewport)
	if !ok {
		return
	}
	// Слот карты — ctmMinimap: в .screen именно он отведён под снимок,
	// а grpMinimap шире него на поля и арт рамки.
	if group := g.screen.Find(screenMinimap); group != nil {
		if slot := group.Find("ctmMinimap"); slot != nil {
			if slotRect := slot.Layout(viewport); slotRect.W > 0 && slotRect.H > 0 {
				r = slotRect
			}
		}
	}
	r = clipScreenRect(r, viewport)
	if r.W <= 0 || r.H <= 0 {
		return
	}
	area := g.minimapImageRect(r)
	if region, ok := g.loadMinimapTexture(c.UI); ok {
		c.UI.Image(region, area.X, area.Y, area.W, area.H, [4]float32{1, 1, 1, 1})
	}
	g.drawMinimapDots(c, area)
}

// minimapImageRect — квадрат панели под карту: карта повёрнута на 45° и
// занимает ромб, вписанный в квадрат со стороной min(ширина, высота).
func (g *GameplayHUD) minimapImageRect(area screenui.Rect) screenui.Rect {
	side := float32(math.Min(float64(area.W), float64(area.H)))
	return screenui.Rect{X: area.X + (area.W-side)/2, Y: area.Y + (area.H-side)/2, W: side, H: side}
}

// minimapPoint переводит мировые метры в пиксели карты. Карта повёрнута
// на 45° против часовой стрелки, поэтому точка поворачивается тем же
// преобразованием, что и картинка: иначе точки съехали бы с объектов.
func (g *GameplayHUD) minimapPoint(area screenui.Rect, x, y float64) (float32, float32, bool) {
	width := float64(g.minimapMax[0] - g.minimapMin[0])
	height := float64(g.minimapMax[1] - g.minimapMin[1])
	if width <= 0 || height <= 0 || area.W <= 0 || area.H <= 0 {
		return 0, 0, false
	}
	u := (x - float64(g.minimapMin[0])) / width
	v := 1 - (y-float64(g.minimapMin[1]))/height
	if u < 0 || u > 1 || v < 0 || v > 1 {
		return 0, 0, false
	}
	// Оси мира сначала нормируются по своей длине: ромб миникарты один и
	// тот же для любой карты, иначе у прямоугольной (absolute_zero)
	// остаются чёрные углы, а у квадратной — нет.
	dx := u - 0.5
	dy := v - 0.5
	cos, sin := math.Cos(minimapAngle), math.Sin(minimapAngle)
	rx := dx*cos + dy*sin
	ry := -dx*sin + dy*cos
	half := (math.Abs(cos) + math.Abs(sin)) / 2
	cu := 0.5 + rx/(2*half)
	cv := 0.5 + ry/(2*half)
	return area.X + float32(cu)*area.W, area.Y + float32(cv)*area.H, true
}

// drawMinimapDots собирает точки объектов и рисует их: сначала обычные,
// затем выделенные — белое не должно теряться под соседями.
func (g *GameplayHUD) drawMinimapDots(c *scene.Ctx, area screenui.Rect) {
	if g.factory == nil {
		return
	}
	size := float32(math.Max(2.5, math.Min(float64(area.W), float64(area.H))/56))
	selected := make(map[string]bool, len(g.selectedIDs)+1)
	for _, entity := range g.selectedEntities() {
		selected[entity.ID] = true
		if entity.SquadID != "" {
			selected[entity.SquadID] = true
		}
	}
	// Отряд — одна точка в центре бойцов: восемь точек на одном месте
	// сливались бы в пятно.
	type squadDot struct {
		x, y     float64
		count    int
		color    [4]float32
		selected bool
	}
	squads := map[string]*squadDot{}
	dots := make([]minimapDot, 0, 64)
	add := func(entity entities.Entity, color [4]float32, isSelected bool) {
		x, y, ok := g.minimapPoint(area, entity.Position.X, entity.Position.Y)
		if !ok {
			return
		}
		dots = append(dots, minimapDot{x: x, y: y, color: color, selected: isSelected})
	}
	for _, entity := range g.factory.All() {
		if !entity.Alive || entity.Dormant {
			continue
		}
		blueprint := strings.ToLower(g.blueprint[entity.ID])
		if color, ok := minimapCaptureColor(blueprint); ok {
			// Точки захвата — по типу, чьи бы они ни были.
			add(entity, color, selected[entity.ID])
			continue
		}
		if entity.Owner == "" || entity.Owner == entities.NeutralPlayer {
			continue
		}
		if entity.Kind != entities.KindUnit && entity.Kind != entities.KindBuilding {
			continue
		}
		color, ok := g.minimapOwnerColor(entity.Owner)
		if !ok {
			continue
		}
		if entity.Kind == entities.KindUnit && entity.SquadID != "" {
			dot := squads[entity.SquadID]
			if dot == nil {
				dot = &squadDot{color: color}
				squads[entity.SquadID] = dot
			}
			dot.x += entity.Position.X
			dot.y += entity.Position.Y
			dot.count++
			dot.selected = dot.selected || selected[entity.SquadID]
			continue
		}
		add(entity, color, selected[entity.ID])
	}
	for _, dot := range squads {
		if dot.count == 0 {
			continue
		}
		x, y, ok := g.minimapPoint(area, dot.x/float64(dot.count), dot.y/float64(dot.count))
		if !ok {
			continue
		}
		color := dot.color
		if color == ([4]float32{}) {
			color = minimapOwnColor
		}
		dots = append(dots, minimapDot{x: x, y: y, color: color, selected: dot.selected})
	}
	for pass := 0; pass < 2; pass++ {
		for _, dot := range dots {
			if dot.selected != (pass == 1) {
				continue
			}
			color := dot.color
			if dot.selected {
				color = minimapSelectedColor
			}
			c.Rect(dot.x-size/2, dot.y-size/2, size, size, color)
		}
	}
}

// minimapOwnerColor — цвет владельца: свои зелёные, враги красные.
func (g *GameplayHUD) minimapOwnerColor(owner entities.PlayerID) ([4]float32, bool) {
	if owner == g.ownPlayer {
		return minimapOwnColor, true
	}
	if owner == entities.NeutralPlayer || owner == "" {
		return [4]float32{}, false
	}
	return minimapEnemyColor, true
}

// minimapCaptureColor — цвет точки захвата по её чертежу: у каждого типа
// свой оттенок, как в оригинальном интерфейсе.
func minimapCaptureColor(blueprint string) ([4]float32, bool) {
	switch {
	case strings.Contains(blueprint, "relic_struct"):
		return minimapRelicColor, true
	case strings.Contains(blueprint, "strategic_objective"):
		return minimapObjectiveColor, true
	case strings.Contains(blueprint, "slag_deposit"):
		return minimapSlagColor, true
	case strings.Contains(blueprint, "strategic_point"):
		return minimapPointColor, true
	}
	return [4]float32{}, false
}

// loadMinimapTexture читает TGA-карту сценария и кладёт её в атлас.
func (g *GameplayHUD) loadMinimapTexture(d *ui.Draw) (ui.Glyph, bool) {
	path := strings.TrimSpace(g.minimapPath)
	if path == "" || d == nil || g.library == nil {
		return ui.Glyph{}, false
	}
	key := "minimap:" + strings.ToLower(path)
	if region, ok := g.textureRegions[key]; ok {
		return region, true
	}
	if g.textureLoaded[key] {
		return ui.Glyph{}, false
	}
	g.textureLoaded[key] = true
	tree := g.library.Tree()
	if tree == nil {
		// Дерево ещё строится: попробуем на следующем кадре.
		delete(g.textureLoaded, key)
		return ui.Glyph{}, false
	}
	for _, candidate := range minimapCandidates(path) {
		data, err := tree.ReadFile(candidate)
		if err != nil || len(data) == 0 {
			continue
		}
		var region ui.Glyph
		// Снимок карты обрезается по играбельной площади и поворачивается
		// на 45°; если файл не TGA или не разобрался — карта берётся как
		// есть, без поворота: пусть лучше будет, чем пустая панель.
		if img, err := ui.DecodeTGA(data); err == nil && img != nil {
			if canvas := g.minimapCanvas(img); canvas != nil {
				region, err = d.Atlas.Image(key, canvas)
			}
		}
		if region.W == 0 {
			region, err = d.LoadImage(key, data)
		}
		if err != nil || region.W <= 0 {
			continue
		}
		g.textureRegions[key] = region
		return region, true
	}
	return ui.Glyph{}, false
}

// minimapCanvas готовит картинку для панели: обрезает снимок по границам
// проходимости и поворачивает результат на 45° против часовой стрелки.
//
// Игра показывает карту ромбом: снимок <карта>_mm.tga — вид сверху с осами
// мира по диагоналям кадра. Поэтому из него вырезается играбельная
// площадь (а не весь снимок с декоративной периферией, куда ставить
// нельзя) и пересэмплируется в квадрат со стороной hypot(w, h); углы
// квадрата прозрачны. Точки на панели считаются тем же преобразованием
// (см. minimapPoint).
func (g *GameplayHUD) minimapCanvas(src image.Image) *image.RGBA {
	bounds := src.Bounds()
	groundW := float64(g.minimapGroundMax[0] - g.minimapGroundMin[0])
	groundH := float64(g.minimapGroundMax[1] - g.minimapGroundMin[1])
	playW := float64(g.minimapMax[0] - g.minimapMin[0])
	playH := float64(g.minimapMax[1] - g.minimapMin[1])
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 || groundW <= 0 || groundH <= 0 || playW <= 0 || playH <= 0 {
		return nil
	}
	// Снимок покрывает всю землю карты: границы проходимости вырезаются
	// из него по долям от GroundMin..GroundMax. Верх снимка — север (+Y).
	scaleX := float64(bounds.Dx()) / groundW
	scaleY := float64(bounds.Dy()) / groundH
	cropW := playW * scaleX
	cropH := playH * scaleY
	x0 := float64(bounds.Min.X) + (float64(g.minimapMin[0])-float64(g.minimapGroundMin[0]))*scaleX
	y0 := float64(bounds.Min.Y) + (float64(g.minimapGroundMax[1])-float64(g.minimapMax[1]))*scaleY
	sidePx := math.Hypot(cropW, cropH)
	side := int(math.Round(sidePx))
	if side < 48 {
		return nil
	}
	if side > 512 {
		side = 512
	}
	cos, sin := math.Cos(minimapAngle), math.Sin(minimapAngle)
	half := (math.Abs(cos) + math.Abs(sin)) / 2
	center := float64(side) / 2
	out := image.NewRGBA(image.Rect(0, 0, side, side))
	for py := 0; py < side; py++ {
		for px := 0; px < side; px++ {
			// Обратный поворот к тому же ромбу: из пикселя канвы в
			// пиксель обрезанной карты. Оси нормированы по аспекту, как
			// и в minimapPoint, поэтому ромб заполнен при любой карте.
			x := float64(px) + 0.5 - center
			y := float64(py) + 0.5 - center
			rx := x * half * 2 / float64(side)
			ry := y * half * 2 / float64(side)
			lx := (0.5 + cos*rx - sin*ry) * cropW
			ly := (0.5 + sin*rx + cos*ry) * cropH
			if lx < 0 || ly < 0 || lx >= cropW || ly >= cropH {
				// За играбельной площадью — прозрачно: углы канвы
				// остаются панели, как и в игре.
				continue
			}
			r, g, b, a := minimapSample(src, x0+lx, y0+ly)
			if a == 0 {
				continue
			}
			out.SetRGBA(px, py, color.RGBA{R: r, G: g, B: b, A: a})
		}
	}
	return out
}

// minimapSample — билинейная выборка из снимка карты; за его пределами
// пиксель прозрачный.
func minimapSample(src image.Image, x, y float64) (uint8, uint8, uint8, uint8) {
	bounds := src.Bounds()
	x -= 0.5
	y -= 0.5
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	fx, fy := x-float64(x0), y-float64(y0)
	at := func(px, py int) (float64, float64, float64, float64) {
		if px < bounds.Min.X || py < bounds.Min.Y || px >= bounds.Max.X || py >= bounds.Max.Y {
			return 0, 0, 0, 0
		}
		r, g, b, a := src.At(px, py).RGBA()
		// Цвета премножаются на альфу: иначе на прозрачных краях
		// появляется тёмная кайма.
		af := float64(a >> 8)
		return float64(r>>8) * af / 255, float64(g>>8) * af / 255, float64(b>>8) * af / 255, af
	}
	w00, w10, w01, w11 := (1-fx)*(1-fy), fx*(1-fy), (1-fx)*fy, fx*fy
	r00, g00, b00, a00 := at(x0, y0)
	r10, g10, b10, a10 := at(x0+1, y0)
	r01, g01, b01, a01 := at(x0, y0+1)
	r11, g11, b11, a11 := at(x0+1, y0+1)
	r := r00*w00 + r10*w10 + r01*w01 + r11*w11
	g := g00*w00 + g10*w10 + g01*w01 + g11*w11
	b := b00*w00 + b10*w10 + b01*w01 + b11*w11
	a := a00*w00 + a10*w10 + a01*w01 + a11*w11
	if a <= 0.5 {
		return 0, 0, 0, 0
	}
	return uint8(math.Min(255, r*255/a)), uint8(math.Min(255, g*255/a)), uint8(math.Min(255, b*255/a)), uint8(math.Min(255, a))
}

// minimapCandidates — файлы миникарты рядом со сценарием: <карта>_mm с
// расширениями ремастера (.tga в редакторе, .rtx/.dds в архиве).
func minimapCandidates(scenario string) []string {
	name := strings.ReplaceAll(strings.TrimSpace(scenario), `\`, "/")
	if name == "" {
		return nil
	}
	dir, file := "", name
	if slash := strings.LastIndex(name, "/"); slash >= 0 {
		dir, file = name[:slash+1], name[slash+1:]
	}
	if dot := strings.LastIndex(file, "."); dot >= 0 {
		file = file[:dot]
	}
	if file == "" {
		return nil
	}
	// Рядом с картой лежит <карта>_mm.tga — тот снимок, который игра
	// кладёт под точки; <карта>.tga — арт лобби, его трогать не надо.
	stems := []string{dir + file + "_mm"}
	rels := make([]string, 0, len(stems)*2)
	for _, stem := range stems {
		rels = append(rels, stem)
		if !strings.HasPrefix(strings.ToLower(stem), "data/") {
			rels = append(rels, "Data/"+stem)
		}
	}
	exts := []string{".tga", ".rtx", ".dds", ".png"}
	out := make([]string, 0, len(rels)*len(exts)*2)
	seen := map[string]bool{}
	for _, rel := range rels {
		for _, ext := range exts {
			for _, candidate := range []string{rel + ext, strings.ToLower(rel + ext)} {
				if seen[candidate] {
					continue
				}
				seen[candidate] = true
				out = append(out, candidate)
			}
		}
	}
	return out
}
