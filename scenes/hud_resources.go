package scenes

import (
	"fmt"
	"github.com/Anykey-Nomad/Open-Kaurava/loc"

	"github.com/Anykey-Nomad/Open-Kaurava/scene"
	"github.com/Anykey-Nomad/Open-Kaurava/screenui"
)

// Панель ресурсов в левом верхнем углу (grpResourceInfo из gamescreen.screen).
//
// Состав панели зависит от расы — это описано в Data/game/taskbar.lua
// (секция resources): у всех есть энергия и лимиты отрядов/техники, очки
// влияния есть у рас с race_details/uses_requesition, «тайм бонус» — у тех,
// у кого очков влияния нет (некроны), вера — у сестёр, души — у тёмных
// эльдаров.

// drawResources рисует значения ресурсов и лимитов по геометрии .screen.
func (g *GameplayHUD) drawResources(c *scene.Ctx) {
	viewport := screenui.Rect{W: float32(c.W), H: float32(c.H)}
	econ, hasEconomy := g.factory.Economy(g.ownPlayer)
	if g.screen == nil {
		if !hasEconomy {
			return
		}
		c.Text(fmt.Sprintf(loc.T("Influence %d (+%.1f/s) · Power %d (+%.1f/s)"),
			int(econ.Resources.Requisition), econ.Income.Requisition,
			int(econ.Resources.Power), econ.Income.Power), 28, 74, 13, ColText)
		return
	}

	kinds := g.resourceKinds()
	for group, visible := range kinds {
		g.setGroupVisible(group, visible)
	}
	if !hasEconomy {
		return
	}

	// Валюты: имя группы, которое из каких полей брать значение и прирост.
	if kinds["grpReq"] {
		g.resourceValue(c, "txtReq", econ.Resources.Requisition, econ.Income.Requisition, viewport)
	}
	g.resourceValue(c, "txtPower", econ.Resources.Power, econ.Income.Power, viewport)
	if kinds["grpFaith"] {
		g.resourceValue(c, "txtFaith", econ.Resources.Faith, econ.Income.Faith, viewport)
	}
	// Души и «тайм бонус» движок пока не считает: панель показывает их место,
	// но значения остаются нулевыми.
	if kinds["grpSouls"] {
		g.resourceValue(c, "txtSouls", 0, 0, viewport)
	}
	if kinds["grpBonus"] {
		g.resourceValue(c, "txtBonus", 0, 0, viewport)
	}

	g.resourceCap(c, "txtTroopsCap", econ.SquadUsed, econ.SquadMax, viewport)
	g.resourceCap(c, "txtVehicleCap", econ.SupportUsed, econ.SupportMax, viewport)
}

// resourceKinds — какие группы панели показывать для расы игрока.
func (g *GameplayHUD) resourceKinds() map[string]bool {
	kinds := map[string]bool{
		"grpReq":   true,
		"grpPower": true,
	}
	if g.raceCaps == nil {
		return kinds
	}
	kinds["grpReq"] = g.raceCaps.UsesRequisition
	kinds["grpFaith"] = g.raceCaps.IncomeCap.Faith > 0
	kinds["grpSouls"] = g.raceCaps.IncomeCap.Souls > 0
	// «тайм бонус» (resource_time.tga) — вместо очков влияния у некронов.
	kinds["grpBonus"] = !g.raceCaps.UsesRequisition
	return kinds
}

// setGroupVisible переключает видимость группы в .screen: значений в файле
// нет, их задаёт гонка расы при старте.
func (g *GameplayHUD) setGroupVisible(name string, visible bool) {
	if g.screen == nil {
		return
	}
	widget := g.screen.Find(name)
	if widget == nil {
		return
	}
	widget.Visible = &visible
}

// resourceValue печатает значение валюты и её прирост в секунду у виджета
// txt* из .screen; если виджета нет, строка рисуется под панелью.
func (g *GameplayHUD) resourceValue(c *scene.Ctx, widget string, value, income float64, viewport screenui.Rect) {
	label := fmt.Sprintf("%d", int(value))
	rect, ok := g.screen.Layout(widget, viewport)
	if !ok {
		return
	}
	valuePx := g.uiFont(13)
	c.Text(label, rect.X, rect.Y+rect.H-g.uiPx(3), valuePx, ColText)
	if income > 0 {
		incomeLabel := fmt.Sprintf(loc.T("+%.1f/s"), income)
		x := rect.X + c.Measure(label, valuePx) + g.uiPx(6)
		c.Text(incomeLabel, x, rect.Y+rect.H-g.uiPx(3), g.uiFont(11), ColTextDim)
	}
}

// resourceCap печатает занятые/максимальные единицы лимита.
func (g *GameplayHUD) resourceCap(c *scene.Ctx, widget string, used, max int, viewport screenui.Rect) {
	rect, ok := g.screen.Layout(widget, viewport)
	if !ok {
		return
	}
	c.Text(fmt.Sprintf("%d/%d", used, max), rect.X, rect.Y+rect.H-g.uiPx(2), g.uiFont(12), ColText)
}
