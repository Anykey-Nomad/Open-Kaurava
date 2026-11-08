package scenes

import (
	"fmt"

	"github.com/Anykey-Nomad/Open-Kaurava/entities"
	"github.com/Anykey-Nomad/Open-Kaurava/scene"
	"github.com/Anykey-Nomad/Open-Kaurava/screenui"
)

// Панель пополнения и улучшений — группа grpUpgrades в gamescreen.screen.
//
// В файле у неё восемь клеток: Reinforce, AddLeader, Upgrade01..04, Attach и
// ProgressUpgrade01, размером 60×45 каждая. Перенесена пока кнопка Reinforce:
// лидеры и улучшения в сцене не реализованы, поэтому пустые клетки под них
// не рисуются — панель не должна показывать неработающие кнопки.
//
// Кнопка берёт место из того же .screen: у самого Reinforce размер не задан
// (в файле у неё 0×0, а Layout отдаёт размер предка), поэтому клетка берётся
// по соседней AddLeader — она в этой сетке такая же.
const (
	upgradeReinforceButton = "Reinforce"
	upgradeCellReference   = "AddLeader"
)

// drawUpgradePanel рисует кнопку пополнения отряда у выделенного бойца.
func (g *GameplayHUD) drawUpgradePanel(c *scene.Ctx, selected entities.Entity, viewport screenui.Rect) {
	if g.screen == nil || c.UI == nil || g.attribData == nil {
		return
	}
	cell, ok := g.screen.Layout(upgradeCellReference, viewport)
	if !ok || cell.W <= 0 || cell.H <= 0 {
		return
	}
	button, ok := g.screen.Layout(upgradeReinforceButton, viewport)
	if !ok {
		return
	}
	rect := screenui.Rect{X: button.X, Y: button.Y, W: cell.W, H: cell.H}
	rect = clipScreenRect(rect, viewport)
	if rect.W <= 0 || rect.H <= 0 {
		return
	}

	// Состояние кнопки: свой боец в отряде, отряд пополняемый, есть место.
	squad, hasSquad := g.attribData.squadFor(g.blueprintOf(selected))
	if !hasSquad || selected.Kind != entities.KindUnit {
		// Не отряд — пополнять нечего; пустую клетку не рисуем.
		return
	}
	enabled := squad.CanReinforce && selected.Owner == g.ownPlayer && selected.SquadID != ""
	alive := 0
	if g.squadOf != nil {
		for _, id := range g.squadOf(selected.ID) {
			if entity, ok := g.factory.Get(id); ok && entity.Alive {
				alive++
			}
		}
	}
	full := squad.UnitMax > 0 && alive >= squad.UnitMax
	if enabled && full {
		enabled = false
	}
	_, seconds, active := 0.0, 0.0, false
	if g.reinforceState != nil {
		_, seconds, active = g.reinforceState(selected.SquadID)
	}
	if active {
		enabled = false
	}

	// Клетка: тёмная плашка, иконка отряда, цена и прогресс пополнения.
	back := [4]float32{0.09, 0.11, 0.14, 0.92}
	if enabled && c.Hover(rect.X, rect.Y, rect.W, rect.H) {
		back = [4]float32{0.16, 0.20, 0.26, 0.96}
	}
	c.Rect(rect.X, rect.Y, rect.W, rect.H, back)
	inner := screenui.Rect{X: rect.X + g.uiPx(3), Y: rect.Y + g.uiPx(3), W: rect.W - g.uiPx(6), H: rect.H - g.uiPx(12)}
	g.drawIcon(c, g.iconFor(selected), inner)
	if !enabled {
		// Недоступная кнопка гасится, но иконка остаётся видна.
		c.Rect(rect.X, rect.Y, rect.W, rect.H, [4]float32{0.02, 0.02, 0.03, 0.45})
	}
	// Плюс в углу — что кнопка добавляет бойца, а не отдаёт приказ.
	plus := [4]float32{0.45, 0.85, 0.45, 1}
	if !enabled {
		plus = [4]float32{0.45, 0.45, 0.45, 0.8}
	}
	c.Rect(rect.X+g.uiPx(3), rect.Y+g.uiPx(3), max(1, g.uiPx(7)), max(1, g.uiPx(2)), plus)
	c.Rect(rect.X+g.uiPx(5.5), rect.Y+g.uiPx(0.5), max(1, g.uiPx(2)), max(1, g.uiPx(7)), plus)

	// Цена одного бойца — числом в правом нижнем углу клетки.
	if !squad.ReinforceCost.Empty() {
		label := fmt.Sprintf("%d", int(squad.ReinforceCost.Requisition+0.5))
		switch {
		case squad.ReinforceCost.Requisition > 0 && squad.ReinforceCost.Power > 0:
			label = fmt.Sprintf("%d/%d", int(squad.ReinforceCost.Requisition+0.5), int(squad.ReinforceCost.Power+0.5))
		case squad.ReinforceCost.Requisition <= 0:
			label = fmt.Sprintf("%d", int(squad.ReinforceCost.Power+0.5))
		}
		px := g.uiFont(8)
		width := c.Measure(label, px)
		c.Text(label, rect.X+rect.W-width-g.uiPx(2), rect.Y+rect.H-g.uiPx(3), px, ColText)
	}
	// Пока боец идёт — полоса прогресса внизу клетки.
	if active && seconds > 0 {
		elapsed, total, _ := g.reinforceState(selected.SquadID)
		barH := max(1, g.uiPx(3))
		barY := rect.Y + rect.H - barH - g.uiPx(1)
		c.Rect(rect.X, barY, rect.W, barH, [4]float32{0.05, 0.06, 0.08, 0.95})
		fill := float32(elapsed / total)
		if fill > 1 {
			fill = 1
		}
		if fill > 0 {
			c.Rect(rect.X, barY, rect.W*fill, barH, ColAccent)
		}
	}

	if c.ClickedAt(rect.X, rect.Y, rect.W, rect.H) && enabled && g.reinforceAt != nil {
		g.command = g.reinforceAt(selected.ID)
	}
}
