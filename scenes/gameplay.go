package scenes

import (
	"fmt"
	"github.com/Anykey-Nomad/Open-Kaurava/loc"
	"log"
	"math"
	"strings"

	"github.com/Anykey-Nomad/Open-Kaurava/attrib"
	"github.com/Anykey-Nomad/Open-Kaurava/entities"
	"github.com/Anykey-Nomad/Open-Kaurava/mods"
	"github.com/Anykey-Nomad/Open-Kaurava/scene"
	"github.com/Anykey-Nomad/Open-Kaurava/screenui"
	"github.com/Anykey-Nomad/Open-Kaurava/ui"
)

// GameplayHUD — экранная часть интерфейса игровой сцены.
type GameplayHUD struct {
	t          float64
	paused     bool
	factory    *entities.Factory
	ownPlayer  entities.PlayerID
	selectedID string
	// selectedIDs — выделение целиком: отряд выбирается одним кликом по
	// любому бойцу, поэтому в списке все его бойцы, а selectedID — главный
	// (его данные показывает панель свойств).
	selectedIDs []string
	// squadOf — состав отряда по ID бойца (игровая сцена знает отряды,
	// нанятые в очередь).
	command      string
	topButtons   []item
	orderButtons []item
	// orderActions, orderMode и pendingOrder — панель приказов: действие
	// каждой клетки, её режим (приказы или строительство) и приказ, ждущий
	// цель левым кликом. Сетка и правила — в command_panel.go.
	orderActions []string
	orderMode    string
	pendingOrder string
	// orderPage — страница подменю (строительство, найм): в сетку влезает
	// десять записей, дальше листается кнопкой «дальше».
	orderPage int
	// placeBlueprint, placeLabel — постройка, выбранная в подменю и ждущая
	// места на карте (см. construction.go).
	placeBlueprint string
	placeLabel     string
	// orderAt — выдача приказа в мир: панель только выбирает приказ,
	// исполняет его игровая сцена.
	orderAt        func(order string, c *scene.Ctx, x, y int)
	library        *mods.Library
	screen         *screenui.Screen
	screenError    string
	textureRegions map[string]ui.Glyph
	textureLoaded  map[string]bool
	// textureBands — вертикальная доля непрозрачного арта в кадре: у панелей
	// ремастера кадр выше .screen-прямоугольника (1580×512 протива 1580×395), и
	// над панелью лежит прозрачная надставка (см. artPlacement).
	textureBands map[string][2]float32
	// stopMusic — сцена останавливает музыку боя, когда игрок выходит в меню.
	stopMusic func()
	// openMenu — клик по «МЕНЮ» ставит бой на паузу и открывает меню паузы
	// (scenes/pause_menu.go); музыка при этом продолжает играть.
	openMenu  func()
	pickWorld func(x, y, width, height int) (string, bool)
	// projectToScreen — проекция точки мира в пиксели: по ней рамка
	// выделения решает, какие отряды попали внутрь.
	projectToScreen func(point [3]float64, width, height int) (float64, float64, bool)
	// multiCards — карточки отрядов в панели мультивыделения: по их
	// прямоугольникам клик делает отряд главным или убирает его.
	multiCards []selectionCard
	// trainAt — заказ найма в очередь постройки, queueOf — очередь для
	// панели производства (train_panel.go).
	trainAt func(buildingID, squadPath string) string
	queueOf func(buildingID string) []trainItem
	// squadOf — бойцы отряда, в котором состоит юнит (nil — юнит одиночный).
	squadOf func(id string) []string
	// reinforceAt — начать пополнение отряда бойца; reinforceState — его
	// прогресс по SquadID (см. reinforce.go и upgrades_panel.go).
	reinforceAt    func(memberID string) string
	reinforceState func(squadKey string) (elapsed, seconds float64, active bool)
	// resurrectAt, summonAt — способности: воскрешение тел вокруг лорда и
	// призыв отряда томб-спайдером; harvestCounts — сколько тел каждого
	// чертежа он уже собрал (см. corpses.go).
	resurrectAt   func(unitID, abilityPath string) string
	summonAt      func(unitID, slot string) string
	harvestCounts func(unitID string) map[string]int
	// attribData — чертежи ebps/sbps, прочитанные при загрузке сценария.
	attribData *attribData
	// raceCaps — лимиты и валюты расы игрока: по ним панель ресурсов
	// решает, какие строки показывать.
	raceCaps *attrib.RaceCaps
	// blueprint — чертёж объекта по ID сущности: панель свойств показывает
	// данные чертежа, а не имя из сценария.
	blueprint map[string]string
	// minimapPath — сценарий загруженной карты: по нему ищется TGA-миникарта
	// (<карта>_mm.tga). minimapMin/max — границы играбельной площади
	// (DATAIMAP), minimapGroundMin/max — вся земля карты: по ним пиксели
	// снимка переводятся в метры (см. minimap.go).
	// uiScale — ужатие раскладки HUD на текущем окне (screenui.Scale).
	// Рамки панелей считаются в нём же, а шрифты и отступы внутри панелей
	// заданы в пикселях — их ужимают uiPx/uiFont.
	uiScale          float32
	minimapPath      string
	minimapMin       [2]float32
	minimapMax       [2]float32
	minimapGroundMin [2]float32
	minimapGroundMax [2]float32
}

// newGameplayHUD создаёт общий интерактивный HUD без демонстрационных сущностей.
func newGameplayHUD(library *mods.Library) *GameplayHUD {
	return &GameplayHUD{
		library:        library,
		ownPlayer:      "blue",
		textureRegions: make(map[string]ui.Glyph),
		textureLoaded:  make(map[string]bool),
		blueprint:      make(map[string]string),
		command:        loc.T("Select your unit"),
		topButtons: []item{
			{label: loc.T("OBJECTIVES"), widgetName: "btnOBJ"},
			{label: loc.T("DIPLOMACY"), widgetName: "btnDIP"},
			{label: loc.T("MENU"), widgetName: "btnMENU"},
		},
	}
}

// updateHUD обновляет кнопки и состояние интерфейса игровой сцены.
func (g *GameplayHUD) updateHUD(c *scene.Ctx) {
	g.loadScreen()
	g.layout(c)
	for i := range g.topButtons {
		updateHUDButton(&g.topButtons[i], c)
	}
	for i := range g.orderButtons {
		updateHUDButton(&g.orderButtons[i], c)
	}
	if len(g.topButtons) > 2 && g.topButtons[2].clicked {
		// МЕНЮ: пауза и меню поверх боя. Если сцена не подключила меню
		// паузы (тесты), остаётся прямой выход в главное меню.
		if g.openMenu != nil {
			g.openMenu()
		} else {
			if g.stopMusic != nil {
				g.stopMusic()
			}
			c.Switch("menu")
		}
		return
	}
	if len(g.topButtons) > 0 && g.topButtons[0].clicked {
		g.command = loc.T("Objectives panel: SCAR integration is not implemented yet")
	}
	if len(g.topButtons) > 1 && g.topButtons[1].clicked {
		g.command = loc.T("Diplomacy: resource trading is not implemented yet")
	}
	if g.canIssueOrders() {
		for i := range g.orderButtons {
			if !g.orderButtons[i].clicked || i >= len(g.orderActions) {
				continue
			}
			g.applyCommand(g.orderActions[i])
		}
	}
	// Рамка и карточки отрядов разбираются до кликов по карте: клик по
	// карточке не должен доходить до 3D-сцены.
	g.selectByDrag(c)
	g.selectCardAtClick(c)
	g.selectAtMapClick(c)
	g.updateCaptureStatus()
}

// updateCaptureStatus — пока выбранный боец стоит на точке и захватывает
// её, строка состояния показывает ход захвата вместо последнего приказа.
func (g *GameplayHUD) updateCaptureStatus() {
	if g.selectedID == "" || g.factory == nil {
		return
	}
	entity, ok := g.factory.Get(g.selectedID)
	if !ok || !entity.Capturing {
		return
	}
	g.command = fmt.Sprintf(loc.T("Capturing point: %d%%"), int(entity.CaptureProgress*100+0.5))
}

func (g *GameplayHUD) loadScreen() {
	if g.screen != nil || g.library == nil {
		return
	}
	data, err := g.library.ReadFile(`Data\art\ui\screens\gamescreen.screen`)
	if err != nil {
		return
	}
	screen, err := screenui.Parse(data)
	if err != nil {
		g.screenError = err.Error()
		return
	}
	g.screen = screen
}

func (g *GameplayHUD) screenRect(name string, viewport screenui.Rect, fallback screenui.Rect) screenui.Rect {
	if g.screen != nil {
		if rect, ok := g.screen.Layout(name, viewport); ok && rect.W > 0 && rect.H > 0 {
			return rect
		}
	}
	return fallback
}

func (g *GameplayHUD) drawScreenGroup(c *scene.Ctx, name, title, detail string) {
	if g.screen == nil || !g.screenWidgetVisible(name) {
		return
	}
	r, ok := g.screen.Layout(name, screenui.Rect{W: float32(c.W), H: float32(c.H)})
	if !ok {
		return
	}
	r = clipScreenRect(r, screenui.Rect{W: float32(c.W), H: float32(c.H)})
	if r.W <= 0 || r.H <= 0 {
		return
	}
	// The .screen group rectangles describe layout bounds, not opaque panel
	// backgrounds. Keep placeholder panels translucent so they don't hide the map.
	alpha := float32(0.48)
	if name == "grpResourceInfo" {
		alpha = 0.22
	}
	c.Rect(r.X, r.Y, r.W, r.H, [4]float32{0.045, 0.055, 0.075, alpha})
	if name != "grpResourceInfo" {
		c.Rect(r.X, r.Y, r.W, 2, ColAccent)
	}
	c.Text(title, max(r.X+8, 4), max(r.Y+18, 14), 11, ColTextDim)
	c.Text(detail, max(r.X+8, 4), max(r.Y+34, 28), 11, ColText)
}

func (g *GameplayHUD) drawSelectionDetails(c *scene.Ctx, selected entities.Entity, viewport screenui.Rect) {
	if g.screen == nil {
		return
	}
	group := g.screen.Find(g.visibleSelectionWidget(selected))
	if group == nil {
		return
	}
	primary := group.Find("grpPrimaryInfo")
	stats := group.Find("grpStats")
	info := g.selectionInfo(selected)
	if primary == nil && stats == nil {
		// Постройки: в .screen у grpStructureSelection нет дочерних
		// шаблонов, поэтому панель рисуется по прямоугольнику группы — с
		// настоящей иконкой из ebps.
		r := group.Layout(viewport)
		c.Rect(r.X, r.Y, r.W, r.H, [4]float32{0.06, 0.07, 0.10, 0.9})
		// Отступы и кегль — в пикселях авторской раскладки: на широком окне
		// они ужимаются вместе с рамкой панели.
		iconSize := min(max(r.H-g.uiPx(16), g.uiPx(24)), g.uiPx(96))
		iconRect := screenui.Rect{X: r.X + g.uiPx(8), Y: r.Y + g.uiPx(8), W: min(iconSize, max(r.W*0.3, g.uiPx(32))), H: iconSize}
		g.drawIcon(c, g.iconFor(selected), iconRect)
		textX := iconRect.X + iconRect.W + g.uiPx(10)
		c.Text(selected.Name, textX, r.Y+g.uiPx(24), g.uiFont(14), ColText)
		c.Text(g.ownerLabel(selected.Owner), textX, r.Y+g.uiPx(42), g.uiFont(10), ColTextDim)
		g.drawSelectionInfo(c, info, textX, r.Y+g.uiPx(56), g.uiFont(9))
		g.drawSelectedStats(c, selected, textX, r.Y+r.H-g.uiPx(40), max(r.X+r.W-textX-g.uiPx(8), g.uiPx(30)))
		return
	}
	if primary != nil {
		r := primary.Layout(viewport)
		c.Rect(r.X, r.Y, r.W, r.H, [4]float32{0.09, 0.10, 0.13, 0.86})
		// Полоски берутся из .screen (healthSingleSquad/moraleSingleSquad),
		// поэтому они стоят там же, где их рисует игра.
		healthRect := screenui.Rect{X: r.X + g.uiPx(8), Y: r.Y + r.H - g.uiPx(42), W: max(r.W-g.uiPx(16), 1), H: max(2, g.uiPx(6))}
		moraleRect := screenui.Rect{X: healthRect.X, Y: healthRect.Y + g.uiPx(22), W: healthRect.W, H: healthRect.H}
		if widget := primary.Find("healthSingleSquad"); widget != nil {
			if rect := widget.Layout(viewport); rect.W > 0 {
				healthRect = rect
			}
		}
		if widget := primary.Find("moraleSingleSquad"); widget != nil {
			if rect := widget.Layout(viewport); rect.W > 0 {
				moraleRect = rect
			}
		}
		// Иконка над полосками, квадратная и по центру панели.
		iconH := max(healthRect.Y-r.Y-g.uiPx(28), g.uiPx(24))
		iconW := min(max(r.W-g.uiPx(16), 1), iconH)
		iconRect := screenui.Rect{X: r.X + (r.W-iconW)/2, Y: r.Y + g.uiPx(6), W: iconW, H: iconH}
		g.drawIcon(c, g.iconFor(selected), iconRect)
		drawBarValue(c, healthRect, selected.Health.Current, selected.Health.Max, ColHealth, true)
		if selected.MoraleMax > 0 {
			drawBarValue(c, moraleRect, selected.Morale, selected.MoraleMax, ColMorale, false)
		}
	}
	if stats != nil {
		r := stats.Layout(viewport)
		c.Rect(r.X, r.Y, r.W, r.H, [4]float32{0.06, 0.07, 0.10, 0.82})
		nameWidget := group.Find("txtSquadName")
		if nameWidget == nil {
			nameWidget = group.Find("txtStructureName")
		}
		if nameWidget != nil {
			nameRect := nameWidget.Layout(viewport)
			c.Rect(nameRect.X, nameRect.Y, nameRect.W, nameRect.H, [4]float32{0.11, 0.13, 0.17, 1})
			c.Text(selected.Name, nameRect.X+g.uiPx(4), nameRect.Y+nameRect.H-g.uiPx(4), g.uiFont(12), ColText)
		} else {
			c.Text(selected.Name, r.X+g.uiPx(6), r.Y+g.uiPx(20), g.uiFont(13), ColText)
		}
		c.Text(g.ownerLabel(selected.Owner), r.X+g.uiPx(6), r.Y+g.uiPx(38), g.uiFont(10), ColTextDim)
		g.drawSelectionInfo(c, info, r.X+g.uiPx(6), r.Y+g.uiPx(52), g.uiFont(9))
	}
}

// drawSelectionInfo печатает строки панели свойств; пустой список —
// строка о том, что чертёж прочитать не удалось.
func (g *GameplayHUD) drawSelectionInfo(c *scene.Ctx, lines []string, x, y float32, px int) {
	if len(lines) == 0 {
		c.Text(loc.T("Blueprint data (ebps/sbps) not read"), x, y, px, ColTextWarn)
		return
	}
	for i, line := range lines {
		c.Text(line, x, y+float32(i)*(float32(px)+g.uiPx(4)), px, ColText)
	}
}

func (g *GameplayHUD) screenWidgetVisible(name string) bool {
	if g.screen == nil {
		return false
	}
	widget := g.screen.Find(name)
	if widget == nil {
		return false
	}
	if name == "grpSingleSquadSelection" || name == "grpStructureSelection" || name == "grpMultiSquadSelection" || name == screenProduction || name == screenUpgrades || name == screenCommand {
		selected, ok := g.selectedEntity()
		if !ok {
			return false
		}
		if name == screenProduction {
			return g.isOwnEntity(selected) && selected.Kind == entities.KindBuilding
		}
		if name == screenUpgrades {
			return g.isOwnEntity(selected) && selected.Kind != entities.KindBuilding
		}
		if name == screenCommand {
			return g.isOwnEntity(selected)
		}
		if name == "grpMultiSquadSelection" || (name == "grpStructureSelection") != (selected.Kind == entities.KindBuilding) {
			return false
		}
		// Screen visibility here is the initial game state. The selection model
		// activates the matching alternative; only ancestor visibility applies.
		for parent := widget.Parent; parent != nil; parent = parent.Parent {
			if parent.Visible != nil && !*parent.Visible {
				return false
			}
		}
		return true
	}
	return widget.IsVisible()
}

func (g *GameplayHUD) selectedEntity() (entities.Entity, bool) {
	if g.factory == nil || g.selectedID == "" {
		return entities.Entity{}, false
	}
	entity, ok := g.factory.Get(g.selectedID)
	if !ok || !entity.Alive || entity.Dormant {
		// Труп и спящая постройка не выделяются: панель приказов не
		// открывается ни у мёртвого, ни у занятого монолитом.
		return entities.Entity{}, false
	}
	return entity, true
}

// selectedEntities — выделение списком: первый объект главный, остальные —
// товарищи по отряду. Приказы раздаются всем.
func (g *GameplayHUD) selectedEntities() []entities.Entity {
	if g.factory == nil {
		return nil
	}
	ids := g.selectedIDs
	if len(ids) == 0 && g.selectedID != "" {
		ids = []string{g.selectedID}
	}
	out := make([]entities.Entity, 0, len(ids))
	for _, id := range ids {
		if entity, ok := g.factory.Get(id); ok && entity.Alive && !entity.Dormant {
			out = append(out, entity)
		}
	}
	return out
}

// selectionSet — выделение как множество ID: индикаторы в мире рисуются для
// всех выбранных бойцов.
func (g *GameplayHUD) selectionSet() map[string]bool {
	set := make(map[string]bool, len(g.selectedIDs)+1)
	mark := func(id string) {
		if entity, ok := g.factory.Get(id); ok && entity.Alive && !entity.Dormant {
			set[id] = true
		}
	}
	for _, id := range g.selectedIDs {
		mark(id)
	}
	if g.selectedID != "" {
		mark(g.selectedID)
	}
	return set
}

func (g *GameplayHUD) canIssueOrders() bool {
	selected, ok := g.selectedEntity()
	return ok && g.isOwnEntity(selected)
}

func (g *GameplayHUD) isOwnEntity(selected entities.Entity) bool {
	return g.ownPlayer != "" && selected.Owner == g.ownPlayer
}

// selectCommand — статус после выбора объекта: свой, нейтральный (не враг —
// доступен захват) или пусто. У вражеского объекта статус печатает сама
// панель свойств, поэтому command он не получает.
func (g *GameplayHUD) selectCommand(selected entities.Entity) string {
	switch {
	case g.isOwnEntity(selected):
		return loc.T("Orders available")
	case selected.Owner == entities.NeutralPlayer:
		return loc.T("Neutral object — capture available")
	default:
		return ""
	}
}

func (g *GameplayHUD) visibleSelectionWidget(selected entities.Entity) string {
	if selected.Kind == entities.KindBuilding {
		return "grpStructureSelection"
	}
	return "grpSingleSquadSelection"
}

func (g *GameplayHUD) ownerLabel(owner entities.PlayerID) string {
	switch {
	case owner == g.ownPlayer && owner != "":
		return loc.T("Ally")
	case owner == entities.NeutralPlayer:
		return loc.T("Neutral object")
	case owner != "":
		return loc.T("Enemy")
	default:
		return loc.T("No owner assigned")
	}
}

func (g *GameplayHUD) selectAtMapClick(c *scene.Ctx) {
	if g.factory == nil {
		return
	}
	// Режим размещения постройки: клик по карте ставит площадку, а не меняет
	// выделение (см. Gameplay.updatePlacement).
	if g.placeBlueprint != "" {
		return
	}
	for _, click := range c.Input.Clicks {
		if click.Button != 1 || g.pointInHUD(click.X, click.Y, c) {
			continue
		}
		// Выбранный кнопкой приказ ждёт цель: клик по карте отдаёт его, а не
		// меняет выделение — так приказ и указывается в оригинале.
		if g.pendingOrder != orderNone && g.canIssueOrders() {
			if g.orderAt != nil {
				g.orderAt(g.pendingOrder, c, click.X, click.Y)
			}
			g.pendingOrder = orderNone
			continue
		}
		g.orderMode, g.pendingOrder = orderModeRoot, orderNone
		if g.pickWorld != nil {
			selectedID, ok := g.pickWorld(click.X, click.Y, c.W, c.H)
			if !ok {
				g.setSelection("")
				g.command = loc.T("Select your unit")
				continue
			}
			selected, found := g.factory.Get(selectedID)
			if !found {
				g.setSelection("")
				g.command = loc.T("Select your unit")
				continue
			}
			own := selected.Owner == g.ownPlayer && !selected.Dormant
			switch {
			case ctrlHeld(c) && own:
				// Ctrl — переключение отряда: выбранный убирается, новый
				// добавляется к остальным и становится главным.
				g.toggleSquad(selectedID)
			case own && g.squadSelected(selectedID):
				// Отряд уже в выделении: клик только делает его главным,
				// не сбрасывая остальные отряды.
				g.focusSquad(selectedID)
			default:
				g.setSelection(selectedID)
			}
			g.selectVoice()
			g.command = g.selectCommand(selected)
			continue
		}
		selected, err := g.factory.SelectAt(entities.Position{X: float64(click.X), Y: float64(click.Y)}, 28)
		if err != nil {
			g.setSelection("")
			g.command = loc.T("Select your unit")
			continue
		}
		g.setSelection(selected.ID)
		g.selectVoice()
		g.command = g.selectCommand(selected)
	}
}

func (g *GameplayHUD) pointInHUD(x, y int, c *scene.Ctx) bool {
	px, py := float32(x), float32(y)
	buttons := append(append([]item(nil), g.topButtons...), g.orderButtons...)
	for _, button := range buttons {
		if inRect(px, py, screenui.Rect{X: button.x, Y: button.y, W: button.w, H: button.h}) {
			return true
		}
	}
	viewport := screenui.Rect{W: float32(c.W), H: float32(c.H)}
	if g.screen == nil {
		// Без .screen панели рисуются fallback-прямоугольниками — их и
		// принимаем за HUD.
		return inRect(px, py, screenui.Rect{X: 16, Y: max(float32(c.H)-172, 92), W: 300, H: 156})
	}
	// Геометрия HUD берётся из самого .screen: клик внутри любой видимой
	// панели не уходит в карту. Полноэкранные группы (.screen держит
	// виртуальную широкую «подложку») панелями не считаются.
	for _, widget := range g.screen.All() {
		if !widget.IsVisible() || !g.hudWidget(widget) {
			continue
		}
		rect := clipScreenRect(widget.Layout(viewport), viewport)
		if rect.W <= 0 || rect.H <= 0 {
			continue
		}
		if rect.W >= viewport.W*0.9 && rect.H >= viewport.H*0.9 {
			continue
		}
		if inRect(px, py, rect) {
			return true
		}
	}
	margin := float32(16)
	return py >= margin && py < margin+64 && px >= margin && px < float32(c.W)-margin
}

// hudWidget — виджет, который HUD рисует сам и который поэтому ловит клики:
// кнопка, панель с текстурами или группа, в которую HUD пишет содержимое.
func (g *GameplayHUD) hudWidget(widget *screenui.Widget) bool {
	switch widget.Name {
	case screenMinimap, screenProduction, screenUpgrades, screenCommand:
		return true
	}
	return widget.Type == "Button" || widget.TexturePath() != "" || len(widget.ArtNodes) > 0
}

func inRect(px, py float32, r screenui.Rect) bool {
	return px >= r.X && px < r.X+r.W && py >= r.Y && py < r.Y+r.H
}

func (g *GameplayHUD) drawEntities(c *scene.Ctx) {
	if g.factory == nil {
		return
	}
	for _, entity := range g.factory.All() {
		if entity.ID != g.selectedID {
			continue
		}
		g.drawSelectionIndicator(c, entity)
	}
}

func (g *GameplayHUD) drawSelectionIndicator(c *scene.Ctx, entity entities.Entity) {
	rx, ry := float32(math.Abs(entity.SelectionUI.Scale[0])), float32(math.Abs(entity.SelectionUI.Scale[1]))
	if rx < 10 {
		rx = 18
	}
	if ry < 10 {
		ry = 18
	}
	cx := float32(entity.Position.X + entity.SelectionUI.Offset[0])
	cy := float32(entity.Position.Y + entity.SelectionUI.Offset[1])
	color := [4]float32{0.12, 0.95, 0.22, 0.95}
	if entity.SelectionUI.DisplayType == 1 {
		c.Rect(cx-rx, cy-ry, rx*2, 2, color)
		c.Rect(cx-rx, cy+ry, rx*2, 2, color)
		c.Rect(cx-rx, cy-ry, 2, ry*2, color)
		c.Rect(cx+rx, cy-ry, 2, ry*2, color)
		return
	}
	const segments = 48
	for i := 0; i < segments; i++ {
		a := 2 * math.Pi * float64(i) / segments
		x := cx + float32(math.Cos(a))*rx
		y := cy + float32(math.Sin(a))*ry
		c.Rect(x-1, y-1, 3, 3, color)
	}
}

func (g *GameplayHUD) layout(c *scene.Ctx) {
	const margin = float32(16)
	topW, topH := max(float32(c.W)-2*margin, 0), float32(64)
	btnW, btnH, gap := float32(96), float32(34), float32(6)
	startX := margin + topW - 3*btnW - 2*gap - 12
	for i := range g.topButtons {
		g.topButtons[i].x = startX + float32(i)*(btnW+gap)
		g.topButtons[i].y = margin + (topH-btnH)/2
		g.topButtons[i].w, g.topButtons[i].h = btnW, btnH
		if g.screen == nil || g.topButtons[i].widgetName == "" {
			continue
		}
		// Кнопки без size в .screen получают размер соседа при разборе
		// (screenui.inheritButtonGeometry), поэтому здесь готовая рамка.
		if rect, ok := g.screen.Layout(g.topButtons[i].widgetName, screenui.Rect{W: float32(c.W), H: float32(c.H)}); ok {
			if rect.W > 0 && rect.W <= float32(c.W)*0.2 && rect.H > 0 && rect.H <= float32(c.H)*0.1 {
				g.topButtons[i].x, g.topButtons[i].y = rect.X, rect.Y
				g.topButtons[i].w, g.topButtons[i].h = rect.W, rect.H
			}
		}
	}

	viewport := screenui.Rect{W: float32(c.W), H: float32(c.H)}
	g.uiScale = 1
	if g.screen != nil {
		g.uiScale = g.screen.Scale(viewport)
	}
	fallback := screenui.Rect{X: float32(c.W) - 250, Y: float32(c.H) - 150, W: 230, H: 130}
	g.layoutCommandPanel(g.screenRect(screenCommand, viewport, fallback), viewport)
}

// uiPx переводит пиксель авторской раскладки в пиксель окна: отступы внутри
// панелей ужимаются тем же коэффициентом, что и рамки из .screen.
func (g *GameplayHUD) uiPx(v float32) float32 {
	if g.uiScale <= 0 || g.uiScale >= 1 {
		return v
	}
	return v * g.uiScale
}

// uiFont — то же для кегля: шрифт ужимается, но не мельче 6 px.
func (g *GameplayHUD) uiFont(v int) int {
	px := int(math.Round(float64(float32(v) * g.uiScale)))
	if g.uiScale <= 0 || px >= v {
		return v
	}
	if px < 6 {
		return 6
	}
	return px
}

func newDemoGameplayHUD() *GameplayHUD {
	g := newGameplayHUD(nil)
	g.factory = entities.NewFactory()
	allKinds := []entities.Kind{entities.KindUnit, entities.KindBuilding, entities.KindSummoned, entities.KindGameplay}
	for _, player := range []entities.Player{
		{ID: "blue", Name: loc.T("Ally"), AllowedProduction: allKinds},
		{ID: "red", Name: loc.T("Enemy"), AllowedProduction: allKinds},
	} {
		_ = g.factory.RegisterPlayer(player)
	}
	friendly, _ := g.factory.Spawn(entities.Spec{
		Name: loc.T("Infantryman"), Kind: entities.KindUnit, ProducedBy: "blue", Selectable: true,
		SelectionUI: entities.SelectionUI{DisplayType: 0, Scale: [2]float64{18, 18}},
	}, entities.Position{X: 260, Y: 230})
	_, _ = g.factory.Spawn(entities.Spec{
		Name: loc.T("Command post"), Kind: entities.KindBuilding, ProducedBy: "blue", Selectable: true,
		SelectionUI: entities.SelectionUI{DisplayType: 1, Scale: [2]float64{48, 34}},
	}, entities.Position{X: 490, Y: 330})
	_, _ = g.factory.Spawn(entities.Spec{
		Name: loc.T("Scout"), Kind: entities.KindUnit, ProducedBy: "red", Selectable: true,
		SelectionUI: entities.SelectionUI{DisplayType: 0, Scale: [2]float64{20, 20}},
	}, entities.Position{X: 740, Y: 250})
	g.selectedID = friendly.ID
	return g
}

func updateHUDButton(it *item, c *scene.Ctx) {
	it.hover = c.Hover(it.x, it.y, it.w, it.h)
	it.clicked = false
	for _, click := range c.Input.Clicks {
		if click.Button == 1 && float32(click.X) >= it.x && float32(click.X) < it.x+it.w &&
			float32(click.Y) >= it.y && float32(click.Y) < it.y+it.h {
			it.clicked = true
			return
		}
	}
}

func (g *GameplayHUD) drawTopPanel(c *scene.Ctx) {
	const margin = float32(16)
	viewport := screenui.Rect{W: float32(c.W), H: float32(c.H)}
	leftFallback := screenui.Rect{X: margin, Y: margin, W: float32(c.W) * 0.5, H: 64}
	left := g.screenRect("artMenubarLeft", viewport, leftFallback)
	if g.screen == nil {
		c.Rect(left.X, left.Y, left.W, left.H, [4]float32{0.03, 0.04, 0.07, 0.92})
		c.Rect(left.X, left.Y, left.W, 3, ColAccent)
		g.drawResources(c)
	}
	if g.screen != nil {
		// Арт верхней панели (artMenubar* вместе с разделёнными половинками)
		// рисует общий обход .screen — см. drawScreenArt.
		g.drawResources(c)
	}
	if g.screen == nil {
		c.Text(loc.T("Scenario: ")+Selected, margin+12, margin+39, 13, ColTextDim)
		c.Text(loc.T("OBJECTIVES  •  DIPLOMACY  •  MENU"), float32(c.W)-420, margin+39, 13, ColTextDim)
	}
	for i := range g.topButtons {
		g.topButtons[i].draw(c, 12)
	}
}

func (g *GameplayHUD) drawBottomPanels(c *scene.Ctx) {
	const margin = float32(16)
	viewport := screenui.Rect{W: float32(c.W), H: float32(c.H)}
	fallback := screenui.Rect{X: margin, Y: max(float32(c.H)-172, margin+64+12), W: 300, H: 156}
	left := g.screenRect("artTaskbarLeft", viewport, fallback)
	right := g.screenRect("artTaskbarRight", viewport, screenui.Rect{X: left.X + left.W, Y: left.Y, W: fallback.W, H: left.H})
	centerW := min(float32(c.W)*0.46, 660)
	center := g.screenRect("grpMultiSquadSelection", viewport, screenui.Rect{
		X: (float32(c.W) - centerW) / 2, Y: fallback.Y, W: centerW, H: fallback.H,
	})
	rightX, panelY := max(right.X+margin, margin), left.Y
	if g.screen == nil {
		for _, rect := range []screenui.Rect{left, center, right} {
			c.Rect(rect.X, rect.Y, rect.W, rect.H, [4]float32{0.03, 0.04, 0.07, 0.92})
			c.Rect(rect.X, rect.Y, rect.W, 3, ColAccent)
		}
	}
	if g.screen != nil {
		// Левая панель — миникарта: её прямоугольник и арт задаёт .screen
		// (grpMinimap внутри artTaskbarLeft).
		g.drawMinimap(c)
	}
	selected, hasSelection := g.selectedEntity()
	cards := g.selectionCards()
	if !hasSelection {
		g.multiCards = nil
		c.Text(loc.T("No object selected"), center.X+12, center.Y+29, 15, ColTextDim)
	} else {
		switch {
		case len(cards) > 1:
			// Мультивыделение: карточка каждого отряда со своими
			// полосками здоровья и морали; клик по карточке делает
			// отряд главным. Панель — центральная, grpMultiSquadSelection.
			rects := g.cardRects(c, len(cards))
			for i := range cards {
				if i < len(rects) {
					cards[i].rect = rects[i]
				}
			}
			g.multiCards = cards
			g.drawMultiSelection(c, cards, rects)
		case g.screen != nil:
			g.multiCards = nil
			g.drawSelectionDetails(c, selected, viewport)
		default:
			g.multiCards = nil
			g.drawFallbackSelection(c, selected, center)
		}
		if selected.UnderConstruction() {
			c.Text(fmt.Sprintf(loc.T("Construction: %.0f%%"), selected.ConstructionRatio()*100), center.X+12, center.Y+center.H-8, 12, ColTextWarn)
		}
	}
	if hasSelection && g.canIssueOrders() {
		if g.screen != nil {
			// Очередь найма: крупная клетка текущего заказа с прогрессом и
			// шесть клеток ожидания — разметка grpBuildQueue из .screen.
			g.drawTrainQueue(c, selected, viewport)
			// Панель grpUpgrades: кнопка Reinforce на своём месте из .screen.
			g.drawUpgradePanel(c, selected, viewport)
		}
		// Сетка приказов рисуется сама: клетки CommandIcon01..12, подсветка
		// выбранного приказа и строка статуса над панелью.
		g.drawCommandPanel(c)
	} else if hasSelection {
		status := loc.T("Enemy object: orders unavailable")
		if selected.Owner == entities.NeutralPlayer {
			status = loc.T("Neutral object: capture available")
		}
		c.Text(status, rightX+12, panelY+29, 13, ColTextDim)
	}
}

func clipScreenRect(r, viewport screenui.Rect) screenui.Rect {
	left := max(r.X, 0)
	top := max(r.Y, 0)
	right := min(r.X+r.W, viewport.W)
	bottom := min(r.Y+r.H, viewport.H)
	if right <= left || bottom <= top {
		return screenui.Rect{}
	}
	return screenui.Rect{X: left, Y: top, W: right - left, H: bottom - top}
}

func (g *GameplayHUD) loadScreenTexture(d *ui.Draw, name string) (ui.Glyph, bool) {
	if region, ok := g.textureRegions[name]; ok {
		return region, true
	}
	if g.textureLoaded[name] {
		return ui.Glyph{}, false
	}
	g.textureLoaded[name] = true
	if g.library == nil || g.screen == nil {
		return ui.Glyph{}, false
	}
	tree := g.library.Tree()
	if tree == nil {
		delete(g.textureLoaded, name)
		return ui.Glyph{}, false
	}
	for _, candidate := range screenTextureCandidates(name, raceSkin(g.raceFolder())) {
		data, err := tree.ReadFile(candidate)
		if err != nil {
			continue
		}
		if d == nil {
			return ui.Glyph{}, false
		}
		img, err := ui.DecodeImage(data)
		if err != nil {
			g.screenError = err.Error()
			log.Printf("ui texture: %s: декодер отказал: %v", name, err)
			return ui.Glyph{}, false
		}
		if band, shifted := artBand(img); shifted {
			if g.textureBands == nil {
				g.textureBands = map[string][2]float32{}
			}
			g.textureBands[name] = band
		}
		region, err := d.Atlas.Image(strings.ToLower(name), img)
		if err != nil {
			g.screenError = err.Error()
			log.Printf("ui texture: %s: атлас отказал: %v", name, err)
			return ui.Glyph{}, false
		}
		g.textureRegions[name] = region
		log.Printf("ui texture: %s -> %s", name, candidate)
		return region, true
	}
	log.Printf("ui texture: %s: файл не найден", name)
	return ui.Glyph{}, false
}

func screenTexturePaths(name string) []string {
	name = strings.ReplaceAll(name, "/", `\`)
	name = strings.TrimLeft(name, `\`)
	if strings.HasPrefix(strings.ToLower(name), `data\`) {
		return []string{name}
	}
	return []string{`Data\` + name, name}
}

// screenTextureCandidates — перебор путей текстуры панели. Настоящая игра через
// Lua-биндинг race_image (game/taskbar.lua) подменяет папку taskbar_share на
// папку расового скина игрока (для некронов — necron), а ремастер хранит
// панели в .dds/.rtx, а не в .tga, как записано в .screen. Поэтому путь
// перебирается как «скин → папка из .screen» × «.rtx → .dds → .tga»: ремастер
// кладёт панели в .rtx, поэтому старая .tga берётся последней.
func screenTextureCandidates(path string, skin string) []string {
	base := screenTexturePaths(path)
	const marker = `textures\taskbar\`
	dir := ""
	for _, p := range base {
		seg, ok := taskbarDirSeg(p, marker)
		if !ok {
			continue
		}
		if j := strings.Index(p[seg:], `\`); j >= 0 {
			dir = p[seg : seg+j]
		}
		break
	}
	if dir == "" {
		return base
	}
	dirs := make([]string, 0, 2)
	if skin != "" && !strings.EqualFold(skin, dir) {
		dirs = append(dirs, skin)
	}
	dirs = append(dirs, dir)
	// Порядок как у движка ремастера: .screen требует .tga, а файл лежит в
	// .rtx — движок сам подменяет расширение.
	exts := []string{".rtx", ".dds", ".tga"}
	out := make([]string, 0, len(base)*len(dirs)*len(exts))
	for _, p := range base {
		seg, ok := taskbarDirSeg(p, marker)
		if !ok {
			out = append(out, p)
			continue
		}
		j := seg
		for j < len(p) && p[j] != '\\' {
			j++
		}
		rest := p[j:]
		stem := rest
		if dot := strings.LastIndex(rest, "."); dot >= 0 {
			stem = rest[:dot]
		}
		for _, d := range dirs {
			for _, ext := range exts {
				out = append(out, p[:seg]+d+stem+ext)
			}
		}
	}
	return out
}

// taskbarDirSeg — индекс начала папки скина внутри пути панели; ok — путь
// вообще относится к панелям taskbar.
func taskbarDirSeg(p, marker string) (int, bool) {
	i := strings.Index(strings.ToLower(p), marker)
	if i < 0 {
		return 0, false
	}
	return i + len(marker), true
}

// raceFolder — папка расы игрока для расового скина панелей. Сначала выбор
// с экрана игроков, затем — по чертежу своих отрядов: сценарий и демо
// запускаются без него, а скин всё равно должен быть расовым.
func (g *GameplayHUD) raceFolder() string {
	if folder := raceFolderOf(Setup.Own); folder != "" {
		return folder
	}
	if g.factory == nil {
		return ""
	}
	for _, entity := range g.factory.All() {
		if entity.Owner != g.ownPlayer {
			continue
		}
		if folder := raceFolderFromPath(g.blueprint[entity.ID]); folder != "" {
			return folder
		}
	}
	return ""
}

// raceFolderFromPath вытаскивает папку расы из пути чертежа: доля сразу
// после "races" ("races/necrons/..." → "necrons").
func raceFolderFromPath(blueprint string) string {
	path := strings.ReplaceAll(strings.TrimSpace(blueprint), `\`, "/")
	parts := strings.Split(path, "/")
	for idx, part := range parts {
		if strings.EqualFold(part, "races") && idx+1 < len(parts) {
			return strings.ToLower(parts[idx+1])
		}
	}
	return ""
}

// raceSkins — папки расовых скинов панелей в архивах игры
// (data/art/ui/textures/taskbar/<имя>).
var raceSkins = []string{"chaos", "dark_eldar", "eldar", "imperial_guard", "necron", "orks", "sisters", "space_marines", "tau"}

// raceSkinAliases — расхождения папок ebps/sbps рас с папками скинов:
// имперская гвардия лежит в "guard", некроны — в "necrons".
var raceSkinAliases = map[string]string{"guard": "imperial_guard", "necrons": "necron"}

// raceSkin — папка расового скина для папки расы; "", если скина нет —
// тогда остаётся путь из .screen.
func raceSkin(raceFolder string) string {
	folder := strings.ToLower(strings.TrimSpace(raceFolder))
	if folder == "" {
		return ""
	}
	if alias, ok := raceSkinAliases[folder]; ok {
		return alias
	}
	for _, skin := range raceSkins {
		if folder == skin {
			return folder
		}
	}
	if trimmed := strings.TrimSuffix(folder, "s"); trimmed != folder {
		for _, skin := range raceSkins {
			if trimmed == skin {
				return trimmed
			}
		}
	}
	return ""
}
