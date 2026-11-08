package scenes

import (
	"image"
	"strings"

	"github.com/Anykey-Nomad/Open-Kaurava/scene"
	"github.com/Anykey-Nomad/Open-Kaurava/screenui"
)

// Имена групп .screen, в которые HUD пишет своё содержимое. Всё остальное —
// арт: он рисуется прямо из файла, и новая панель в интерфейсе появляется
// без правок кода.
const (
	screenMinimap    = "grpMinimap"
	screenProduction = "grpBuildQueue"
	screenUpgrades   = "grpUpgrades"
	screenCommand    = "grpCommandIcons"
)

// drawScreenArt рисует арт всего интерфейса из .screen в порядке документа:
// панели, меню, рамки и иконки уже расставлены в самом файле, поэтому
// отдельные вызовы на каждую панель (artMenubar*, artTaskbar*, шаблоны
// выделения) больше не нужны.
func (g *GameplayHUD) drawScreenArt(c *scene.Ctx) {
	if g.screen == nil || c.UI == nil {
		return
	}
	viewport := screenui.Rect{W: float32(c.W), H: float32(c.H)}
	var images []screenui.Image
	for _, widget := range g.screen.All() {
		if !widget.IsVisible() {
			continue
		}
		images = append(images, widget.TextureRects(viewport)...)
	}
	g.drawImages(c, preferSeparated(images), viewport)
}

// drawImages рисует готовые прямоугольники текстур.
func (g *GameplayHUD) drawImages(c *scene.Ctx, images []screenui.Image, viewport screenui.Rect) {
	if c.UI == nil {
		return
	}
	for _, image := range images {
		region, ok := g.loadScreenTexture(c.UI, image.Path)
		if !ok {
			continue
		}
		rect := image.Rect
		if band, ok := g.textureBands[image.Path]; ok {
			rect = artPlacement(rect, band)
		}
		rect = clipScreenRect(rect, viewport)
		if rect.W <= 0 || rect.H <= 0 {
			continue
		}
		c.UI.Image(region, rect.X, rect.Y, rect.W, rect.H, [4]float32{1, 1, 1, 1})
	}
}

// artBand — где в кадре лежит сам арт: [верх, низ] в долях высоты. У
// ремастера панели лежат в кадре выше .screen-прямоугольника: снизу панель,
// сверху прозрачная надставка (1580×512 в файле против 1580×395 в .screen).
// Режем надставку только если арт доходит до низа кадра и она заметная:
// у меню рамка лежит в верхней половине кадра, её трогать нельзя.
func artBand(img image.Image) ([2]float32, bool) {
	if img == nil {
		return [2]float32{0, 1}, false
	}
	bounds := img.Bounds()
	height := bounds.Dy()
	if bounds.Dx() <= 0 || height <= 0 {
		return [2]float32{0, 1}, false
	}
	top, bottom := -1, -1
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		opaque := false
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a >= 0x0800 {
				opaque = true
				break
			}
		}
		if !opaque {
			continue
		}
		if top < 0 {
			top = y - bounds.Min.Y
		}
		bottom = y - bounds.Min.Y
	}
	if top <= 0 || bottom != height-1 || float64(top) < 0.1*float64(height) {
		return [2]float32{0, 1}, false
	}
	return [2]float32{float32(top) / float32(height), float32(bottom+1) / float32(height)}, true
}

// artPlacement — прямоугольник отрисовки арта с надставкой. По ширине кадр и
// .screen-прямоугольник совпадают, поэтому ширина берётся как есть (пиксели не
// растягиваются по горизонтали). По высоте в прямоугольник точно ложится
// видимая часть панели: её верх — на верхней границе прямоугольника, низ — на
// нижней, а прозрачная надставка уходит выше кадра.
func artPlacement(rect screenui.Rect, band [2]float32) screenui.Rect {
	span := band[1] - band[0]
	if rect.W <= 0 || rect.H <= 0 || span <= 0 {
		return rect
	}
	height := rect.H / span
	rect.Y -= band[0] * height
	rect.H = height
	return rect
}

// preferSeparated отбрасывает объединённую текстуру панели, если рядом видна
// её «разделённая» версия: ремастер рисует панели половинками
// (taskbar_menu_separated_left/right), а старую общую в .screen оставил
// видимой — иначе получается второе, сбитое по геометрии изображение.
func preferSeparated(images []screenui.Image) []screenui.Image {
	separated := make(map[string]bool, len(images))
	for _, image := range images {
		if stem, ok := separatedStem(image.Path); ok {
			separated[stem] = true
		}
	}
	if len(separated) == 0 {
		return images
	}
	out := images[:0]
	for _, image := range images {
		stem, isSeparated := separatedStem(image.Path)
		if !isSeparated && separated[stem] {
			continue
		}
		out = append(out, image)
	}
	return out
}

// separatedStem — общее имя семейства текстур панели: из пути убираются
// расширение и пометки «_separated»/«_left»/«_right»/«_padded». Второй
// результат — была ли у текстуры пометка разделённой версии.
func separatedStem(path string) (string, bool) {
	lower := strings.ToLower(path)
	if dot := strings.LastIndexByte(lower, '.'); dot >= 0 {
		lower = lower[:dot]
	}
	isSeparated := strings.Contains(lower, "_separated")
	for _, mark := range []string{"_separated", "_left", "_right", "_padded", "_pad"} {
		lower = strings.ReplaceAll(lower, mark, "")
	}
	return lower, isSeparated
}
