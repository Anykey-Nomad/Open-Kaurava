package scenes

import (
	"github.com/Anykey-Nomad/Open-Kaurava/loc"
	"strings"

	"github.com/Anykey-Nomad/Open-Kaurava/entities"
	"github.com/Anykey-Nomad/Open-Kaurava/scene"
	"github.com/Anykey-Nomad/Open-Kaurava/screenui"
	"github.com/Anykey-Nomad/Open-Kaurava/ui"
)

// Иконки отрядов и построек из арта.
//
// Чертёж (sbps/ebps) даёт ui_info/icon_name — путь от Data/art/ui/ingame,
// например "necron_icons/necron_warrior_icon". Ремастер держит карту в
// .tga и/или .rtx, поэтому расширение перебирается, а готовое изображение
// кладётся в общий атлас интерфейса и живёт там до смены сцены.

// iconFor — иконка выбранного объекта: из отряда, а без него — из чертежа
// бойца/постройки. Пусто — иконки в арте нет.
func (g *GameplayHUD) iconFor(selected entities.Entity) string {
	if g == nil || g.attribData == nil {
		return ""
	}
	blueprint := g.blueprintOf(selected)
	if squad, ok := g.attribData.squadFor(blueprint); ok && squad.Icon != "" {
		return squad.Icon
	}
	if unit, ok := g.attribData.unitByPath(blueprint); ok && unit.Icon != "" {
		return unit.Icon
	}
	return ""
}

// drawIcon рисует иконку в прямоугольник, сохраняя пропорции и центрируя её.
// Нет файла — остаётся тёмная плашка и подпись, чтобы панель не «прыгала».
func (g *GameplayHUD) drawIcon(c *scene.Ctx, icon string, rect screenui.Rect) {
	if rect.W <= 0 || rect.H <= 0 {
		return
	}
	c.Rect(rect.X, rect.Y, rect.W, rect.H, [4]float32{0.10, 0.12, 0.16, 1})
	if c.UI == nil {
		return
	}
	region, ok := g.loadIconTexture(c.UI, icon)
	if !ok {
		c.Text(loc.T("icon"), rect.X+5, rect.Y+16, 8, ColTextDim)
		return
	}
	w, h := float32(region.W), float32(region.H)
	if w <= 0 || h <= 0 {
		return
	}
	scale := min(rect.W/w, rect.H/h)
	width, height := w*scale, h*scale
	c.UI.Image(region, rect.X+(rect.W-width)/2, rect.Y+(rect.H-height)/2, width, height, [4]float32{1, 1, 1, 1})
}

// loadIconTexture читает иконку из арта: путь из чертежа относителен
// Data/art/ui/ingame, а полный data-путь и art/... тоже принимаются.
func (g *GameplayHUD) loadIconTexture(d *ui.Draw, icon string) (ui.Glyph, bool) {
	icon = strings.TrimSpace(icon)
	if icon == "" || d == nil || g.library == nil {
		return ui.Glyph{}, false
	}
	key := "icon:" + strings.ToLower(icon)
	if region, ok := g.textureRegions[key]; ok {
		return region, true
	}
	if g.textureLoaded[key] {
		return ui.Glyph{}, false
	}
	g.textureLoaded[key] = true
	tree := g.library.Tree()
	if tree == nil {
		delete(g.textureLoaded, key)
		return ui.Glyph{}, false
	}
	for _, candidate := range iconTextureCandidates(icon) {
		data, err := tree.ReadFile(candidate)
		if err != nil || len(data) == 0 {
			continue
		}
		region, err := d.LoadImage(key, data)
		if err != nil {
			continue
		}
		g.textureRegions[key] = region
		return region, true
	}
	return ui.Glyph{}, false
}

// iconTextureCandidates перечисляет файлы, под которыми может лежать иконка:
// относительный путь от art/ui/ingame, полный data-путь и art/...; расширение
// из чертежа часто .tga, а в архиве ремастера карта может быть .rtx/.dds.
func iconTextureCandidates(icon string) []string {
	name := strings.ReplaceAll(strings.TrimSpace(icon), `\`, "/")
	name = strings.TrimLeft(name, "/")
	for _, prefix := range []string{"data:", "data/"} {
		if len(name) >= len(prefix) && strings.EqualFold(name[:len(prefix)], prefix) {
			name = name[len(prefix):]
			break
		}
	}
	if name == "" {
		return nil
	}
	rels := make([]string, 0, 2)
	switch {
	case strings.HasPrefix(strings.ToLower(name), "art/"):
		rels = append(rels, name)
	default:
		rels = append(rels, "art/ui/ingame/"+name)
	}
	exts := []string{".tga", ".rtx", ".dds", ".png"}
	out := make([]string, 0, 16)
	seen := map[string]bool{}
	for _, rel := range rels {
		stem := rel
		if dot := strings.LastIndex(rel, "."); dot >= 0 {
			stem = rel[:dot]
		}
		for _, ext := range exts {
			for _, candidate := range []string{
				"Data/" + stem + ext,
				strings.ToLower("Data/" + stem + ext),
				stem + ext,
				strings.ToLower(stem + ext),
			} {
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
