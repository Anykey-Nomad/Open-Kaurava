package scenes

import (
	"fmt"
	"github.com/Anykey-Nomad/Open-Kaurava/loc"
	"sort"

	"github.com/Anykey-Nomad/Open-Kaurava/entities"
	"github.com/Anykey-Nomad/Open-Kaurava/scene"
	"github.com/Anykey-Nomad/Open-Kaurava/screenui"
	"github.com/Anykey-Nomad/Open-Kaurava/window"
)

// Выделение нескольких отрядов.
//
// В игре отряд — единая сущность, но в реестре он лежит отдельными бойцами,
// поэтому выделение хранится списком ID, а «отряд» для интерфейса — это
// группа из squadOf. selectedID при этом остаётся главным отрядом: панель
// приказов и свойства показывают именно его, а остальные выделены для
// приказов и полосок.

// selectionCard — карточка отряда в панели мультивыделения: по её
// прямоугольнику клик делает отряд главным (а с Ctrl — убирает из выбора).
type selectionCard struct {
	// focus — ID бойца, который становится главным при клике по карточке.
	focus string
	ids   []string
	rect  screenui.Rect
}

// squadMembers — бойцы отряда, в котором состоит юнит; одиночный юнит —
// сам себе отряд. Живые и не спящие: мёртвого товарища в выделение не
// возвращают.
func (g *GameplayHUD) squadMembers(id string) []string {
	if id == "" || g.factory == nil {
		return nil
	}
	members := []string{id}
	if g.squadOf != nil {
		if mates := g.squadOf(id); len(mates) > 0 {
			members = mates
		}
	}
	alive := make([]string, 0, len(members))
	for _, member := range members {
		if entity, ok := g.factory.Get(member); ok && entity.Alive && !entity.Dormant {
			alive = append(alive, member)
		}
	}
	if len(alive) == 0 {
		return []string{id}
	}
	return alive
}

// selectionHas — есть ли сущность в текущем выделении.
func (g *GameplayHUD) selectionHas(id string) bool {
	for _, selected := range g.selectedIDs {
		if selected == id {
			return true
		}
	}
	return false
}

// squadSelected — весь отряд уже в выделении.
func (g *GameplayHUD) squadSelected(id string) bool {
	members := g.squadMembers(id)
	if len(members) == 0 {
		return false
	}
	for _, member := range members {
		if !g.selectionHas(member) {
			return false
		}
	}
	return true
}

// setSelection заменяет выделение отрядом: клик по бойцу выбирает весь его
// отряд, а главным становится сам боец.
func (g *GameplayHUD) setSelection(id string) {
	g.selectedIDs = g.selectedIDs[:0]
	g.selectedID = id
	if id == "" {
		return
	}
	for _, member := range g.squadMembers(id) {
		if !g.selectionHas(member) {
			g.selectedIDs = append(g.selectedIDs, member)
		}
	}
	if len(g.selectedIDs) == 0 {
		g.selectedIDs = append(g.selectedIDs, id)
	}
}

// addSquad добавляет отряд в выделение, не снимая остальные.
func (g *GameplayHUD) addSquad(id string) {
	for _, member := range g.squadMembers(id) {
		if !g.selectionHas(member) {
			g.selectedIDs = append(g.selectedIDs, member)
		}
	}
}

// removeSquad убирает отряд из выделения. Если убирают главный, главным
// становится первый оставшийся.
func (g *GameplayHUD) removeSquad(id string) {
	drop := map[string]bool{}
	for _, member := range g.squadMembers(id) {
		drop[member] = true
	}
	kept := g.selectedIDs[:0]
	removingPrimary := false
	for _, selected := range g.selectedIDs {
		if drop[selected] {
			if selected == g.selectedID {
				removingPrimary = true
			}
			continue
		}
		kept = append(kept, selected)
	}
	g.selectedIDs = kept
	if removingPrimary {
		g.selectedID = ""
		if len(kept) > 0 {
			g.selectedID = kept[0]
		}
	}
}

// toggleSquad переключает отряд в выделении: уже выбранный — убирает,
// новый — добавляет и делает главным.
func (g *GameplayHUD) toggleSquad(id string) {
	if g.squadSelected(id) {
		g.removeSquad(id)
		return
	}
	g.addSquad(id)
	g.selectedID = id
}

// focusSquad делает отряд главным, не меняя состава выделения.
func (g *GameplayHUD) focusSquad(id string) {
	if id == "" {
		return
	}
	g.selectedID = id
	if !g.squadSelected(id) {
		g.addSquad(id)
	}
}

// selectionCards — отряды текущего выделения в порядке добавления. Каждая
// карточка несёт ID главного бойца и полный список своих бойцов.
func (g *GameplayHUD) selectionCards() []selectionCard {
	seen := map[string]bool{}
	cards := make([]selectionCard, 0, len(g.selectedIDs))
	for _, id := range g.selectedIDs {
		members := g.squadMembers(id)
		key := members[0]
		if seen[key] {
			continue
		}
		seen[key] = true
		focus := id
		if g.selectedID != "" && containsID(members, g.selectedID) {
			focus = g.selectedID
		}
		cards = append(cards, selectionCard{focus: focus, ids: members})
	}
	return cards
}

func containsID(ids []string, id string) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}

// ctrlHeld — зажат ли Ctrl: с ним клик добавляет или убирает отряд.
func ctrlHeld(c *scene.Ctx) bool {
	for _, key := range c.Input.Held {
		if key == window.KeyLeftCtrl || key == window.KeyRightCtrl {
			return true
		}
	}
	return false
}

// selectByDrag выбирает отряды рамкой. Если в рамку попал хоть один
// невыбранный отряд, рамка добавляет все попавшие; если все попавшие уже
// выбраны — снимает выделение с них.
func (g *GameplayHUD) selectByDrag(c *scene.Ctx) {
	if g.factory == nil || g.projectToScreen == nil {
		return
	}
	if g.placeBlueprint != "" || g.pendingOrder != orderNone {
		return
	}
	if !c.Input.DragEnded || c.Input.DragButton != 1 {
		return
	}
	// Рамка, начатая на интерфейсе, — не выделение.
	if g.pointInHUD(c.Input.DragFromX, c.Input.DragFromY, c) {
		return
	}
	x0, y0 := c.Input.DragX, c.Input.DragY
	x1, y1 := x0+c.Input.DragW, y0+c.Input.DragH
	hits := map[string][]string{}
	for _, entity := range g.factory.All() {
		if entity.Owner != g.ownPlayer || !entity.Alive || entity.Dormant || !entity.Selectable {
			continue
		}
		// Точка — центр тела на земле: рамка ловит отряд, если попал в
		// его положение, а не в висящий над землёй силуэт.
		x, y, ok := g.projectToScreen([3]float64{entity.Position.X, entity.Position.Y, 0}, c.W, c.H)
		if !ok || x < float64(x0) || x > float64(x1) || y < float64(y0) || y > float64(y1) {
			continue
		}
		members := g.squadMembers(entity.ID)
		hits[members[0]] = members
	}
	if len(hits) == 0 {
		g.setSelection("")
		g.command = loc.T("Select your unit")
		return
	}
	keys := make([]string, 0, len(hits))
	for key := range hits {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	// Ctrl-рамка только добавляет: снятие остаётся на повторной рамке
	// без Ctrl по уже выбранным отрядам.
	missing := ctrlHeld(c)
	for _, key := range keys {
		if !missing && !g.squadSelected(key) {
			missing = true
			break
		}
	}
	if missing {
		for _, key := range keys {
			g.addSquad(key)
		}
		g.selectedID = keys[0]
		g.command = fmt.Sprintf(loc.T("Squads selected: %d"), len(g.selectionCards()))
		g.selectVoice()
		return
	}
	for _, key := range keys {
		g.removeSquad(key)
	}
	if len(g.selectionCards()) == 0 {
		g.command = loc.T("Select your unit")
	}
}

// selectCardAtClick переключает главный отряд кликом по карточке панели
// мультивыделения, а с Ctrl — убирает отряд из выделения.
func (g *GameplayHUD) selectCardAtClick(c *scene.Ctx) {
	if len(g.multiCards) < 2 {
		return
	}
	for _, click := range c.Input.Clicks {
		if click.Button != 1 {
			continue
		}
		for _, card := range g.multiCards {
			if !inRect(float32(click.X), float32(click.Y), card.rect) {
				continue
			}
			if ctrlHeld(c) {
				g.removeSquad(card.focus)
			} else {
				g.focusSquad(card.focus)
			}
			return
		}
	}
}

// drawDragBox рисует рамку выделения, пока её тянут.
func (g *GameplayHUD) drawDragBox(c *scene.Ctx) {
	if !c.Input.Dragging || c.Input.DragCurrentButton != 1 || c.Input.DragW <= 0 || c.Input.DragH <= 0 {
		return
	}
	x, y := float32(c.Input.DragX), float32(c.Input.DragY)
	w, h := float32(c.Input.DragW), float32(c.Input.DragH)
	c.Rect(x, y, w, h, [4]float32{0.35, 0.65, 1, 0.12})
	border := [4]float32{0.55, 0.85, 1, 0.9}
	c.Rect(x, y, w, 2, border)
	c.Rect(x, y+h-2, w, 2, border)
	c.Rect(x, y, 2, h, border)
	c.Rect(x+w-2, y, 2, h, border)
}

// cardRects раскладывает карточки отрядов в центральной панели
// мультивыделения по разметке gamescreen.screen: grpMultiSquadSelection и её
// шаблон grpMultiSelected01. Слева внизу — миникарта, поэтому карточки живут
// в центре, а не в старой левой панели.
func (g *GameplayHUD) cardRects(c *scene.Ctx, count int) []screenui.Rect {
	if count <= 0 {
		return nil
	}
	viewport := screenui.Rect{W: float32(c.W), H: float32(c.H)}
	centerW := min(float32(c.W)*0.46, 660)
	fallback := screenui.Rect{
		X: (float32(c.W) - centerW) / 2,
		Y: max(float32(c.H)-172, 92),
		W: centerW, H: 156,
	}
	group := g.screenRect("grpMultiSquadSelection", viewport, fallback)
	cardW, cardH := float32(105), float32(98)
	if g.screen != nil {
		if template := g.screen.Find("grpMultiSelected01"); template != nil {
			if rect := template.Layout(viewport); rect.W > 0 && rect.H > 0 {
				cardW, cardH = rect.W, rect.H
			}
		}
	}
	// Зазор и поля внутри панели заданы в пикселях — ужимаются вместе с её
	// рамкой, иначе на узком окне карточки уезжают за край панели.
	gap := g.uiPx(5)
	inset := g.uiPx(3)
	padding := g.uiPx(6)
	if total := cardW*float32(count) + gap*float32(count-1); total > group.W-padding {
		cardW = (group.W - padding - gap*float32(count-1)) / float32(count)
	}
	if minW := g.uiPx(36); cardW < minW {
		cardW = minW
	}
	if cardH > group.H-padding {
		cardH = group.H - padding
	}
	rects := make([]screenui.Rect, count)
	for i := range rects {
		rects[i] = screenui.Rect{X: group.X + inset + float32(i)*(cardW+gap), Y: group.Y + inset, W: cardW, H: cardH}
	}
	return rects
}

// drawMultiSelection рисует карточки отрядов: настоящая иконка из арта,
// имя, полоски здоровья и морали со значениями под ними. Главный отряд
// обведён ярче.
func (g *GameplayHUD) drawMultiSelection(c *scene.Ctx, cards []selectionCard, rects []screenui.Rect) {
	for i, card := range cards {
		if i >= len(rects) {
			break
		}
		r := rects[i]
		primary := card.focus == g.selectedID
		border := [4]float32{0.22, 0.26, 0.33, 1}
		if primary {
			border = ColAccent
		}
		c.Rect(r.X, r.Y, r.W, r.H, [4]float32{0.07, 0.08, 0.11, 0.94})
		c.Rect(r.X, r.Y, r.W, 2, border)
		iconH := max(r.H-g.uiPx(40), g.uiPx(24))
		iconRect := screenui.Rect{X: r.X + g.uiPx(4), Y: r.Y + g.uiPx(5), W: max(r.W-g.uiPx(8), 1), H: iconH}
		if entity, ok := g.factory.Get(card.focus); ok {
			g.drawIcon(c, g.iconFor(entity), iconRect)
		} else {
			c.Rect(iconRect.X, iconRect.Y, iconRect.W, iconRect.H, [4]float32{0.18, 0.22, 0.28, 1})
		}
		labelPx := g.uiFont(8)
		inset := g.uiPx(4)
		c.Text(fitCardName(g.cardName(card), int(r.W/5)), r.X+inset, r.Y+iconH+g.uiPx(12), labelPx, ColText)
		health, healthMax, morale, moraleMax := g.cardStats(card)
		barW := max(r.W-g.uiPx(8), 1)
		barY := r.Y + r.H - g.uiPx(24)
		drawStatBar(c, r.X+inset, barY, barW, healthRatio(health, healthMax), ColHealth)
		c.Text(fmt.Sprintf("%.0f/%.0f", health, healthMax), r.X+inset, barY+g.uiPx(10), labelPx, ColTextDim)
		if moraleMax > 0 {
			drawStatBar(c, r.X+inset, barY+g.uiPx(13), barW, healthRatio(morale, moraleMax), ColMorale)
			c.Text(fmt.Sprintf("%.0f/%.0f", morale, moraleMax), r.X+inset, barY+g.uiPx(23), labelPx, ColTextDim)
		}
	}
}

// fitCardName укорачивает имя под ширину карточки: при шести отрядах
// карточка узкая, и длинное имя залезло бы на соседнюю.
func fitCardName(name string, maxRunes int) string {
	if maxRunes < 4 {
		maxRunes = 4
	}
	runes := []rune(name)
	if len(runes) <= maxRunes {
		return name
	}
	return string(runes[:maxRunes-1]) + "…"
}

// cardName — имя отряда для карточки: имя главного бойца.
func (g *GameplayHUD) cardName(card selectionCard) string {
	if entity, ok := g.factory.Get(card.focus); ok {
		return entity.Name
	}
	return loc.T("squad")
}

// cardStats — суммарное здоровье отряда и его мораль.
func (g *GameplayHUD) cardStats(card selectionCard) (health, healthMax, morale, moraleMax float64) {
	for _, id := range card.ids {
		entity, ok := g.factory.Get(id)
		if !ok {
			continue
		}
		health += entity.Health.Current
		healthMax += entity.Health.Max
		if entity.MoraleMax > moraleMax {
			moraleMax = entity.MoraleMax
			morale = entity.Morale
		}
	}
	return health, healthMax, morale, moraleMax
}

// healthRatio — доля полоски 0..1.
func healthRatio(value, max float64) float64 {
	if max <= 0 {
		return 0
	}
	ratio := value / max
	if ratio < 0 {
		return 0
	}
	if ratio > 1 {
		return 1
	}
	return ratio
}

// drawStatBar рисует полоску состояния: тёмная подложка и заполнение.
func drawStatBar(c *scene.Ctx, x, y, w float32, ratio float64, col [4]float32) {
	const height = float32(4)
	c.Rect(x, y, w, height, [4]float32{0.13, 0.15, 0.20, 1})
	if ratio <= 0 {
		return
	}
	c.Rect(x, y, max(w*float32(ratio), 1), height, col)
}

// drawFallbackSelection — панель одиночного выделения без .screen (или без
// шаблонов в нём): иконка из арта слева, имя и полоски справа.
func (g *GameplayHUD) drawFallbackSelection(c *scene.Ctx, selected entities.Entity, r screenui.Rect) {
	if r.W <= 0 || r.H <= 0 {
		return
	}
	c.Rect(r.X, r.Y, r.W, r.H, [4]float32{0.06, 0.07, 0.10, 0.9})
	iconSize := min(max(r.H-20, 24), 96)
	iconRect := screenui.Rect{X: r.X + 10, Y: r.Y + 10, W: min(iconSize, max(r.W*0.22, 32)), H: iconSize}
	g.drawIcon(c, g.iconFor(selected), iconRect)
	textX := iconRect.X + iconRect.W + 12
	c.Text(loc.T("SELECTED OBJECT"), textX, r.Y+20, 12, ColTextDim)
	c.Text(selected.Name, textX, r.Y+42, 18, ColText)
	c.Text(g.ownerLabel(selected.Owner), textX, r.Y+60, 11, ColTextDim)
	g.drawSelectedStats(c, selected, textX, r.Y+r.H-40, max(r.X+r.W-textX-10, 30))
}

// drawBarValue рисует полоску с числом: полоски панели берутся из .screen,
// а значение HUD пишет сам — над полоской или под ней, чтобы два числа не
// наезжали друг на друга.
func drawBarValue(c *scene.Ctx, r screenui.Rect, value, max float64, col [4]float32, above bool) {
	drawStatBar(c, r.X, r.Y, r.W, healthRatio(value, max), col)
	y := r.Y + r.H + 9
	if above {
		y = r.Y - 4
	}
	c.Text(fmt.Sprintf("%.0f/%.0f", value, max), r.X, y, 8, ColTextDim)
}

// drawSelectedStats рисует полоски здоровья и морали одного объекта — для
// панели одиночного выделения. Морали нет (постройка, техника) — вторая
// полоска не рисуется.
func (g *GameplayHUD) drawSelectedStats(c *scene.Ctx, selected entities.Entity, x, y, w float32) {
	px := g.uiFont(8)
	drawStatBar(c, x, y, w, healthRatio(selected.Health.Current, selected.Health.Max), ColHealth)
	c.Text(fmt.Sprintf("%.0f/%.0f", selected.Health.Current, selected.Health.Max), x, y+g.uiPx(13), px, ColTextDim)
	if selected.MoraleMax > 0 {
		drawStatBar(c, x, y+g.uiPx(18), w, healthRatio(selected.Morale, selected.MoraleMax), ColMorale)
		c.Text(fmt.Sprintf("%.0f/%.0f", selected.Morale, selected.MoraleMax), x, y+g.uiPx(31), px, ColTextDim)
	}
}
