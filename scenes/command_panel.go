package scenes

import (
	"fmt"
	"github.com/Anykey-Nomad/Open-Kaurava/loc"
	"sort"
	"strings"

	"github.com/Anykey-Nomad/Open-Kaurava/entities"
	"github.com/Anykey-Nomad/Open-Kaurava/scene"
	"github.com/Anykey-Nomad/Open-Kaurava/screenui"
)

// Панель приказов — сетка из game-screen. В gamescreen.screen DoWDE у группы
// grpCommandIcons двенадцать кнопок CommandIcon01..CommandIcon12 в четыре
// столбца и три ряда; их позиции и размеры берутся оттуда же, так что
// разметка панели в коде не дублируется.
//
// Верхний ряд — приказы отряда: движение и атака (цель указывается следующим
// левым кликом), отмена приказа, захват точки. Второй ряд подменяет приказы:
// у отряда «Строить» — постройки расы из structure_buildable_ext, у постройки
// «Найм» — отряды из spawner_ext/squad_table. Последняя клетка подменю
// возвращает к приказам, предпоследняя листает страницы.
const (
	// Режимы панели: приказы, постройки, найм.
	orderModeRoot  = ""
	orderModeBuild = "build"
	orderModeTrain = "train"

	// Приказы, которые ждут цель левым кликом по карте.
	orderNone    = ""
	orderMove    = "move"
	orderAttack  = "attack"
	orderCapture = "capture"

	// Клетки, которые не ждут цели.
	commandStop  = "stop"
	commandBuild = "build"
	commandTrain = "train"
	commandBack  = "back"
	commandNext  = "next"

	// Начало действий подменю: buildPrefix + путь ebps, trainPrefix + путь sbps.
	buildPrefix = "build:"
	trainPrefix = "train:"
	// Способности и призыв из тел: abilityPrefix + путь чертежа способности,
	// harvestPrefix + слот a/b/c из harvest_ext томб-спайдера.
	abilityPrefix = "ability:"
	harvestPrefix = "harvest:"
	pageNext      = "page:next"

	commandColumns = 4
	commandRows    = 3
	commandSlots   = commandColumns * commandRows

	// commandPageSize — сколько записей подменю влезает на страницу: из
	// двенадцати клеток две последние заняты «дальше» и «назад».
	commandPageSize = commandSlots - 2
)

// commandCell — клетка панели: подпись и действие. Пустое действие —
// свободный слот: в оригинале такие клетки занимают приказы, которые отряд
// ещё не получил.
type commandCell struct {
	label  string
	action string
}

// commandCells — клетки текущего режима панели.
func (g *GameplayHUD) commandCells() []commandCell {
	switch g.orderMode {
	case orderModeBuild:
		return g.submenuCells(g.buildOptions(), buildPrefix)
	case orderModeTrain:
		selected, _ := g.selectedEntity()
		return g.submenuCells(g.trainOptions(selected), trainPrefix)
	}
	selected, hasSelection := g.selectedEntity()
	cells := []commandCell{
		{label: loc.T("MOVE"), action: orderMove},
		{label: loc.T("ATTACK"), action: orderAttack},
		{label: loc.T("STOP"), action: commandStop},
		{label: loc.T("CAPTURE"), action: orderCapture},
	}
	// Второй ряд: у постройки — найм отрядов, у отряда — строительство.
	if hasSelection && selected.Kind == entities.KindBuilding {
		cells = append(cells, commandCell{label: loc.T("TRAIN"), action: commandTrain})
	} else {
		cells = append(cells, commandCell{label: loc.T("BUILD"), action: commandBuild})
	}
	// Дальше — способности выделенного юнита: воскрешение лорда и сбор тел
	// томб-спайдера. Клетки живут в той же сетке, поэтому счётчики тел в
	// их подписях обновляются вместе с кадром.
	if hasSelection {
		cells = append(cells, g.abilityCells(selected)...)
		if selected.Kind == entities.KindUnit {
			cells = append(cells, g.harvestCells(selected)...)
		}
	}
	return padCells(cells)
}

// abilityCells — кнопки способностей выделенного юнита. Пока показываются
// только те, что поднимают тела (массовое воскрешение лорда некронов):
// остальные способности сцена ещё не исполняет.
func (g *GameplayHUD) abilityCells(selected entities.Entity) []commandCell {
	if g.attribData == nil {
		return nil
	}
	unit, ok := g.attribData.unitFor(g.blueprintOf(selected))
	if !ok || len(unit.Abilities) == 0 {
		return nil
	}
	var cells []commandCell
	for _, path := range unit.Abilities {
		ability, ok := g.attribData.abilityFor(path)
		if !ok || !ability.Resurrects() {
			continue
		}
		label := g.attribData.localName(ability.ScreenNameID)
		if label == "" {
			label = loc.T("RESURRECT")
		}
		cells = append(cells, commandCell{label: strings.ToUpper(label), action: abilityPrefix + path})
	}
	return cells
}

// harvestCells — кнопки призыва томб-спайдера: в подписи видно, сколько тел
// собрано и сколько нужно для отряда.
func (g *GameplayHUD) harvestCells(selected entities.Entity) []commandCell {
	if g.attribData == nil || g.harvestCounts == nil {
		return nil
	}
	unit, ok := g.attribData.unitFor(g.blueprintOf(selected))
	if !ok || unit.Harvest == nil {
		return nil
	}
	bodies := g.harvestCounts(selected.ID)
	cells := make([]commandCell, 0, len(unit.Harvest.Slots))
	for _, slot := range unit.Harvest.Slots {
		title := slot.Entity
		if squad, ok := g.attribData.squadByName(slot.Squad); ok {
			title = g.attribData.squadTitle(squad)
		}
		label := fmt.Sprintf("%s %d/%d", strings.ToUpper(title), bodies[strings.ToLower(slot.Entity)], slot.Bodies)
		cells = append(cells, commandCell{label: label, action: harvestPrefix + slot.Slot})
	}
	return cells
}

// buildOptions — меню строительства: постройки расы игрока из
// structure_buildable_ext, в порядке build_menu_priority. Номенклатуру
// задают чертежи ebps, поэтому список расы появляется целиком.
func (g *GameplayHUD) buildOptions() []commandCell {
	if g.attribData == nil {
		return nil
	}
	buildings := g.attribData.buildablesFor(raceFolderOf(Setup.Own))
	cells := make([]commandCell, 0, len(buildings))
	for _, building := range buildings {
		label := g.attribData.localName(building.ScreenNameID)
		if label == "" {
			label = building.Name
		}
		cells = append(cells, commandCell{label: label, action: building.Path})
	}
	return cells
}

// trainOptions — меню найма выбранной постройки: отряды из
// spawner_ext/squad_table её чертежа.
func (g *GameplayHUD) trainOptions(selected entities.Entity) []commandCell {
	if g.attribData == nil {
		return nil
	}
	unit, ok := g.attribData.unitFor(g.blueprintOf(selected))
	if !ok || len(unit.Spawns) == 0 {
		return nil
	}
	cells := make([]commandCell, 0, len(unit.Spawns))
	for _, path := range unit.Spawns {
		cells = append(cells, commandCell{label: g.attribData.squadLabel(path), action: path})
	}
	return cells
}

// submenuCells раскладывает записи подменю по страницам: на странице
// commandPageSize записей, затем «дальше» (если есть следующая страница) и
// «назад» в последней клетке сетки.
func (g *GameplayHUD) submenuCells(entries []commandCell, prefix string) []commandCell {
	cells := padCells(nil)
	start := g.orderPage * commandPageSize
	if start >= len(entries) {
		g.orderPage, start = 0, 0
	}
	end := min(start+commandPageSize, len(entries))
	for i, entry := range entries[start:end] {
		cells[i] = commandCell{label: entry.label, action: prefix + entry.action}
	}
	if end < len(entries) {
		cells[commandSlots-2] = commandCell{label: commandNext, action: pageNext}
	}
	cells[commandSlots-1] = commandCell{label: commandBack, action: commandBack}
	return cells
}

// padCells дополняет сетку пустыми клетками до 4×3.
func padCells(cells []commandCell) []commandCell {
	for len(cells) < commandSlots {
		cells = append(cells, commandCell{})
	}
	return cells
}

// applyCommand исполняет нажатие клетки панели приказов.
func (g *GameplayHUD) applyCommand(action string) {
	switch {
	case action == "":
		return
	case action == pageNext:
		g.orderPage++
	case action == commandBack:
		g.orderMode, g.orderPage = orderModeRoot, 0
		g.command = loc.T("Orders: choose an order")
	case action == commandBuild:
		g.orderMode, g.orderPage = orderModeBuild, 0
		g.pendingOrder = orderNone
		g.command = g.submenuStatus(g.buildOptions(), loc.T("Construction"))
	case action == commandTrain:
		selected, _ := g.selectedEntity()
		g.orderMode, g.orderPage = orderModeTrain, 0
		g.pendingOrder = orderNone
		g.command = g.submenuStatus(g.trainOptions(selected), loc.T("Train"))
	case strings.HasPrefix(action, buildPrefix):
		path := strings.TrimPrefix(action, buildPrefix)
		g.orderMode, g.orderPage = orderModeRoot, 0
		g.pendingOrder = orderNone
		// Постройка выбрана, место под неё указывается на карте: «призрак»
		// основания считает игровая сцена (см. construction.go).
		g.placeBlueprint = path
		g.placeLabel = g.attribData.displayName(path)
		g.command = loc.T("Building site: ") + g.placeLabel + loc.T(" (RMB — cancel)")
	case strings.HasPrefix(action, trainPrefix):
		path := strings.TrimPrefix(action, trainPrefix)
		g.orderMode, g.orderPage = orderModeRoot, 0
		selected, ok := g.selectedEntity()
		if !ok || g.trainAt == nil {
			g.command = loc.T("Train: ") + g.attribData.squadLabel(path)
			return
		}
		g.command = g.trainAt(selected.ID, path)
	case strings.HasPrefix(action, abilityPrefix):
		// Способность выделенного юнита: воскрешение тел вокруг лорда.
		selected, ok := g.selectedEntity()
		if !ok || g.resurrectAt == nil {
			g.command = loc.T("Ability: no target")
			return
		}
		g.orderMode, g.orderPage = orderModeRoot, 0
		g.pendingOrder = orderNone
		g.command = g.resurrectAt(selected.ID, strings.TrimPrefix(action, abilityPrefix))
	case strings.HasPrefix(action, harvestPrefix):
		// Призыв томб-спайдера из собранных тел.
		selected, ok := g.selectedEntity()
		if !ok || g.summonAt == nil {
			g.command = loc.T("Summon: no target")
			return
		}
		g.orderMode, g.orderPage = orderModeRoot, 0
		g.pendingOrder = orderNone
		g.command = g.summonAt(selected.ID, strings.TrimPrefix(action, harvestPrefix))
	case action == commandStop:
		g.pendingOrder = orderNone
		g.clearPlacement()
		if selected, ok := g.selectedEntity(); ok && g.factory != nil {
			_ = g.factory.ClearOrder(selected.ID)
		}
		g.command = loc.T("Order cancelled")
	default:
		// Движение, атака и захват ждут цель: следующий левый клик по карте
		// отдаёт приказ (см. selectAtMapClick).
		g.pendingOrder = action
		g.command = loc.T("Target with left click: ") + action
	}
}

// submenuStatus — строка состояния при открытии подменю: сколько записей
// нашлось в чертежах.
func (g *GameplayHUD) submenuStatus(entries []commandCell, title string) string {
	if len(entries) == 0 {
		return title + loc.T(": nothing found in the race blueprint")
	}
	return title + loc.T(": choose an item")
}

// layoutCommandPanel раскладывает клетки панели по сетке grpCommandIcons.
// Геометрия берётся из .screen; расчётная сетка нужна только без файла.
func (g *GameplayHUD) layoutCommandPanel(panel, viewport screenui.Rect) {
	cells := g.commandCells()
	g.orderButtons = make([]item, len(cells))
	g.orderActions = make([]string, len(cells))
	rects := g.screenCommandSlots(viewport)
	for i, cell := range cells {
		rect := commandGridRect(panel, i)
		if len(rects) == commandSlots {
			rect = rects[i]
		}
		g.orderButtons[i] = item{label: cell.label, x: rect.X, y: rect.Y, w: rect.W, h: rect.H}
		g.orderActions[i] = cell.action
	}
}

// screenCommandSlots — прямоугольники клеток из .screen: кнопки
// CommandIcon01..12 внутри grpCommandIcons. У части кнопок size не задан —
// тогда берётся размер соседней клетки, как это делает и сама игра.
func (g *GameplayHUD) screenCommandSlots(viewport screenui.Rect) []screenui.Rect {
	if g.screen == nil {
		return nil
	}
	group := g.screen.Find(screenCommand)
	if group == nil {
		return nil
	}
	var cells []*screenui.Widget
	for _, child := range group.Children {
		if isCommandSlotNamed(child.Name) {
			cells = append(cells, child)
		}
	}
	if len(cells) != commandSlots {
		return nil
	}
	sort.Slice(cells, func(i, j int) bool { return cells[i].Name < cells[j].Name })
	refW, refH := float32(0), float32(0)
	for _, cell := range cells {
		if cell.HasSize && cell.Size[0] > 0 && cell.Size[1] > 0 {
			rect := cell.Layout(viewport)
			refW, refH = rect.W, rect.H
			break
		}
	}
	rects := make([]screenui.Rect, 0, len(cells))
	for _, cell := range cells {
		rect := cell.Layout(viewport)
		if refW > 0 && (!cell.HasSize || rect.W > refW*1.5 || rect.H > refH*1.5) {
			rect.W, rect.H = refW, refH
		}
		rects = append(rects, rect)
	}
	return rects
}

// isCommandSlotNamed — имя клетки сетки: CommandIcon01..12. Рядом в .screen
// лежат CommandIconNNHotkey, но это подписи, а не клетки.
func isCommandSlotNamed(name string) bool {
	lower := strings.ToLower(name)
	if !strings.HasPrefix(lower, "commandicon") || len(lower) < 4 {
		return false
	}
	tail := lower[len(lower)-2:]
	return tail[0] >= '0' && tail[0] <= '9' && tail[1] >= '0' && tail[1] <= '9'
}

// commandGridRect — расчётная клетка сетки 4×3 на случай, когда .screen не
// загружен: доли сняты с CommandIcon03 (55×39 px при панели 230×129, шаг
// 57×40, первая клетка почти в углу панели).
func commandGridRect(panel screenui.Rect, index int) screenui.Rect {
	const (
		cellW  = 0.0434 / 0.17983
		cellH  = 0.0545 / 0.18
		stepX  = 0.0448 / 0.17983
		stepY  = 0.0562 / 0.18
		insetX = -0.0002 / 0.17983
		insetY = 0.0031 / 0.18
	)
	col, row := index%commandColumns, index/commandColumns
	return screenui.Rect{
		X: panel.X + panel.W*insetX + float32(col)*panel.W*stepX,
		Y: panel.Y + panel.H*insetY + float32(row)*panel.H*stepY,
		W: panel.W * cellW,
		H: panel.H * cellH,
	}
}

// drawCommandPanel рисует сетку приказов и строку статуса над панелью.
// Свободные клетки остаются пустыми, выбранный приказ подсвечен, пока ждёт
// цель.
func (g *GameplayHUD) drawCommandPanel(c *scene.Ctx) {
	if g.screen != nil && !g.screenWidgetVisible(screenCommand) {
		return
	}
	viewport := screenui.Rect{W: float32(c.W), H: float32(c.H)}
	fallback := screenui.Rect{X: float32(c.W) - 250, Y: float32(c.H) - 150, W: 230, H: 130}
	r := clipScreenRect(g.screenRect(screenCommand, viewport, fallback), viewport)
	if r.W <= 0 || r.H <= 0 {
		return
	}
	for i := range g.orderButtons {
		g.drawCommandCell(c, &g.orderButtons[i], g.orderActions[i])
	}
	status := g.command
	if g.pendingOrder != orderNone {
		status = loc.T("Target with left click: ") + g.pendingOrder
	}
	if status == "" {
		status = loc.T("Orders")
	}
	statusPx := g.uiFont(10)
	statusWidth := c.Measure(status, statusPx)
	c.Text(status, r.X+r.W-statusWidth, max(r.Y-g.uiPx(6), g.uiPx(12)), statusPx, ColTextDim)
}

// drawCommandCell рисует одну клетку: пустой слот, обычную кнопку или
// подсвеченный приказ, ждущий цель.
func (g *GameplayHUD) drawCommandCell(c *scene.Ctx, it *item, action string) {
	if action == "" {
		c.Rect(it.x, it.y, it.w, it.h, [4]float32{0.07, 0.08, 0.10, 0.35})
		return
	}
	armed := g.pendingOrder != orderNone && action == g.pendingOrder
	bg := [4]float32{0.10, 0.11, 0.15, 0.85}
	switch {
	case armed:
		bg = [4]float32{0.18, 0.38, 0.60, 0.95}
	case it.hover:
		bg = ColPanelHot
	}
	c.Rect(it.x, it.y, it.w, it.h, bg)
	if armed || it.hover {
		c.Rect(it.x, it.y, it.w, max(1, g.uiPx(2)), ColAccent)
	}
	g.drawCellLabel(c, it.label, it.x, it.y, it.w, it.h)
}

// drawCellLabel печатает подпись клетки по центру, перенося её по словам:
// клетка 55×39 px, длинное слово вроде «ДВИЖЕНИЕ» в одну строку не влезает.
func (g *GameplayHUD) drawCellLabel(c *scene.Ctx, label string, x, y, w, h float32) {
	px := g.uiFont(8)
	lines := wrapLabel(c, label, px, w-g.uiPx(4))
	if len(lines) == 0 {
		return
	}
	lineH := float32(px) + 1
	top := y + (h-float32(len(lines))*lineH)/2 + float32(px)*0.4
	for i, line := range lines {
		lineWidth := c.Measure(line, px)
		c.Text(line, x+(w-lineWidth)/2, top+float32(i)*lineH, px, ColText)
	}
}

// wrapLabel разбивает подпись на строки по ширине клетки.
func wrapLabel(c *scene.Ctx, label string, px int, width float32) []string {
	words := strings.Fields(label)
	if len(words) == 0 {
		return nil
	}
	lines := []string{words[0]}
	for _, word := range words[1:] {
		last := len(lines) - 1
		candidate := lines[last] + " " + word
		if c.Measure(candidate, px) <= width {
			lines[last] = candidate
			continue
		}
		lines = append(lines, word)
	}
	return lines
}

// clearPendingOrder снимает приказ, ждущий цели: новый выбор или правая
// кнопка его отменяют.
func (g *GameplayHUD) clearPendingOrder() {
	g.pendingOrder = orderNone
}

// clearPlacement снимает режим размещения постройки: отмена, правый клик или
// уже поставленная площадка.
func (g *GameplayHUD) clearPlacement() {
	g.placeBlueprint, g.placeLabel = "", ""
}
