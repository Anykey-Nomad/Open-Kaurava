package scenes

import (
	"fmt"
	"github.com/Anykey-Nomad/Open-Kaurava/loc"
	"strings"

	"github.com/Anykey-Nomad/Open-Kaurava/entities"
	"github.com/Anykey-Nomad/Open-Kaurava/scene"
	"github.com/Anykey-Nomad/Open-Kaurava/screenui"
)

// Панель производства в gamescreen.screen: группа grpBuildQueue, в ней
// currentBuildQueue (крупная клетка того, что нанимается сейчас),
// txtBuildPercent (процент справа от неё), buildProgress (полоса прогресса во
// всю ширину панели) и buildQueue01..06 — шесть клеток очереди в три столбца
// и два ряда. Геометрия берётся из файла, поэтому при другой раскладке
// интерфейса панель остаётся на месте.
const (
	trainCurrentSlot = "currentBuildQueue"
	trainPercentText = "txtBuildPercent"
	trainProgressBar = "buildProgress"
	trainSlotPrefix  = "buildQueue"
)

// trainQueueSlots — сколько клеток ожидания нарисовать (buildQueue01..06).
const trainQueueSlots = 6

// drawTrainQueue рисует очередь найма выбранной постройки: текущий заказ с
// иконкой и процентом, полосу прогресса, клетки ожидания с остатком времени,
// а в свободной нижней части панели — название, цену и время текущего заказа.
func (g *GameplayHUD) drawTrainQueue(c *scene.Ctx, selected entities.Entity, viewport screenui.Rect) {
	if g.screen == nil || g.queueOf == nil || selected.Kind != entities.KindBuilding {
		return
	}
	items := g.queueOf(selected.ID)
	var current *trainItem
	var waiting []trainItem
	if len(items) > 0 {
		current = &items[0]
		waiting = items[1:]
	}

	// Текущий заказ: крупная клетка и процент рядом с ней.
	currentRect, ok := g.screen.Layout(trainCurrentSlot, viewport)
	if !ok {
		return
	}
	if current != nil {
		g.drawTrainIcon(c, current.icon, current.label, currentRect)
		if percentRect, ok := g.screen.Layout(trainPercentText, viewport); ok {
			percent := fmt.Sprintf("%d%%", int(current.ratio()*100))
			c.Text(percent, percentRect.X, percentRect.Y+g.uiPx(14), g.uiFont(12), ColText)
		}
		// Полоса прогресса: заливка от левого края к правому.
		if barRect, ok := g.screen.Layout(trainProgressBar, viewport); ok {
			c.Rect(barRect.X, barRect.Y, barRect.W, barRect.H, [4]float32{0.09, 0.11, 0.14, 0.95})
			fill := barRect.W * float32(current.ratio())
			if fill > 0 {
				c.Rect(barRect.X, barRect.Y, fill, barRect.H, ColAccent)
			}
		}
	} else {
		c.Rect(currentRect.X, currentRect.Y, currentRect.W, currentRect.H, [4]float32{0.09, 0.11, 0.14, 0.55})
	}

	// Клетки ожидания: иконка и остаток времени в секундах.
	for i := 0; i < trainQueueSlots; i++ {
		slotRect, ok := g.screen.Layout(fmt.Sprintf("%s%02d", trainSlotPrefix, i+1), viewport)
		if !ok {
			continue
		}
		c.Rect(slotRect.X, slotRect.Y, slotRect.W, slotRect.H, [4]float32{0.09, 0.11, 0.14, 0.55})
		if i >= len(waiting) {
			continue
		}
		item := waiting[i]
		g.drawTrainIcon(c, item.icon, item.label, slotRect)
		left := fmt.Sprintf("%.0f", item.remaining())
		px := g.uiFont(8)
		width := c.Measure(left, px)
		c.Text(left, slotRect.X+slotRect.W-width-g.uiPx(1), slotRect.Y+slotRect.H-g.uiPx(7), px, ColText)
	}

	// Нижняя часть панели в .screen пустая — там умещаются название, цена и
	// время текущего заказа.
	if current == nil {
		return
	}
	group := g.screen.Find(screenProduction)
	if group == nil {
		return
	}
	groupRect := group.Layout(viewport)
	lineY := groupRect.Y + groupRect.H - g.uiPx(30)
	inset := groupRect.X + g.uiPx(3)
	c.Text(current.label, inset, lineY, g.uiFont(9), ColText)
	c.Text(current.cost.String(), inset, lineY+g.uiPx(10), g.uiFont(8), ColTextDim)
	c.Text(fmt.Sprintf(loc.T("%g s"), current.seconds), inset, lineY+g.uiPx(20), g.uiFont(8), ColTextDim)
}

// drawTrainIcon рисует иконку отряда в клетке. Имя из ui_info/icon_name
// ("space_marine_icons/tacticalmarine_icon") лежит в art/ui/textures базы
// арта; если картинки нет — остаётся подсвеченная клетка с началом названия,
// чтобы очередь читалась и без арта.
func (g *GameplayHUD) drawTrainIcon(c *scene.Ctx, icon, label string, rect screenui.Rect) {
	if c.UI != nil && icon != "" {
		for _, path := range iconTexturePaths(icon) {
			if region, ok := g.loadScreenTexture(c.UI, path); ok {
				c.UI.Image(region, rect.X, rect.Y, rect.W, rect.H, [4]float32{1, 1, 1, 1})
				return
			}
		}
	}
	c.Rect(rect.X, rect.Y, rect.W, rect.H, [4]float32{0.24, 0.30, 0.38, 0.95})
	name := strings.TrimSpace(label)
	if name == "" {
		return
	}
	initials := []rune(name)
	if len(initials) > 2 {
		initials = initials[:2]
	}
	text := string(initials)
	width := c.Measure(text, 9)
	c.Text(text, rect.X+(rect.W-width)/2, rect.Y+rect.H/2+3, 9, ColText)
}

// iconTexturePaths — где искать иконку из чертежа: пути в .screen идут от
// Data, а icon_name — от art/ui/textures базы арта модуля.
func iconTexturePaths(icon string) []string {
	icon = strings.TrimSpace(strings.ReplaceAll(icon, `\`, "/"))
	if icon == "" {
		return nil
	}
	lower := strings.ToLower(icon)
	if strings.HasSuffix(lower, ".tga") || strings.HasSuffix(lower, ".dds") || strings.HasSuffix(lower, ".rtx") {
		return []string{"art/ui/textures/" + icon}
	}
	return []string{"art/ui/textures/" + icon, "art/ui/textures/" + icon + ".tga"}
}
