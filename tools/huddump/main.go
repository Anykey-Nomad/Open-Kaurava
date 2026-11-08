// Command huddump печатает список текстур HUD так, как его собирает игровая
// сцена: прямоугольники .screen при заданном размере окна, с отбрасыванием
// объединённых текстур ради разделённых версий. Нужно, чтобы найти, какая
// плитка ложится поверх карты, и сравнить рамку с собственным размером
// текстуры.
package main

import (
	"flag"
	"fmt"
	"image"
	"sort"
	"strings"

	"dow-mod-studio/imgdec"
	"dow-mod-studio/modapi"

	"github.com/Anykey-Nomad/Open-Kaurava/screenui"
)

func main() {
	root := flag.String("root", `C:\Program Files\Steam\steamapps\common\Dawn of War Definitive Edition`, "путь к Dawn of War")
	module := flag.String("module", "DoWDE", "модуль")
	w := flag.Float64("w", 1600, "ширина окна")
	h := flag.Float64("h", 870, "высота окна")
	flag.Parse()

	modules, err := modapi.DiscoverModules(*root)
	if err != nil {
		fmt.Println("discover:", err)
		return
	}
	mod := modapi.FindModule(modules, *module)
	tree, err := modapi.BuildModTree(mod.Path, "English")
	if err != nil {
		fmt.Println("tree:", err)
		return
	}
	defer tree.Close()
	data, err := tree.ReadFile(`Data\art\ui\screens\gamescreen.screen`)
	if err != nil {
		fmt.Println("read:", err)
		return
	}
	screen, err := screenui.Parse(data)
	if err != nil {
		fmt.Println("parse:", err)
		return
	}
	viewport := screenui.Rect{W: float32(*w), H: float32(*h)}
	var images []screenui.Image
	for _, widget := range screen.All() {
		if !widget.IsVisible() {
			continue
		}
		images = append(images, widget.TextureRects(viewport)...)
	}
	images = preferSeparated(images)
	sort.SliceStable(images, func(i, j int) bool {
		return images[i].Rect.W*images[i].Rect.H > images[j].Rect.W*images[j].Rect.H
	})
	sizes := map[string][2]int{}
	opaqueBounds := map[string][4]int{}
	fmt.Printf("всего текстур: %d, окно %.0fx%.0f\n", len(images), *w, *h)
	for i, image := range images {
		if i >= 24 {
			break
		}
		r := image.Rect
		size, ok := sizes[image.Path]
		if !ok {
			size = textureSize(tree, image.Path)
			sizes[image.Path] = size
			opaqueBounds[image.Path] = textureOpaque(tree, image.Path)
		}
		aspect := 0.0
		if size[0] > 0 && size[1] > 0 {
			aspect = float64(size[0]) / float64(size[1])
		}
		opaque := opaqueBounds[image.Path]
		fmt.Printf("%4d x %4d  (%6.1f,%6.1f)  %.1f%%  текстура %4dx%-4d(%5.2f) непрозр %4d,%4d %4dx%-4d(%.2f) рамка(%5.2f)  %s\n",
			int(r.W), int(r.H), r.X, r.Y, float64(r.W*r.H)*100/float64(*w**h),
			size[0], size[1], aspect,
			opaque[0], opaque[1], opaque[2], opaque[3], ratioOf(opaque), float64(r.W)/float64(r.H), shortPath(image.Path))
	}
}

// ratioOf — отношение ширины непрозрачного прямоугольника к высоте.
func ratioOf(box [4]int) float64 {
	if box[3] == 0 {
		return 0
	}
	return float64(box[2]) / float64(box[3])
}

// textureOpaque возвращает границы непрозрачной части текстуры (x, y, w, h).
// В арте панелей вокруг рамки большие прозрачные поля, поэтому видимая
// панель меньше своего прямоугольника в .screen.
func textureOpaque(tree *modapi.ModTree, screenPath string) [4]int {
	img, ok := textureImage(tree, screenPath)
	if !ok {
		return [4]int{}
	}
	bounds := img.Bounds()
	minX, minY, maxX, maxY := bounds.Max.X, bounds.Max.Y, bounds.Min.X, bounds.Min.Y
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a < 0x0800 {
				continue
			}
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x > maxX {
				maxX = x
			}
			if y > maxY {
				maxY = y
			}
		}
	}
	if maxX < minX || maxY < minY {
		return [4]int{0, 0, bounds.Dx(), bounds.Dy()}
	}
	return [4]int{minX - bounds.Min.X, minY - bounds.Min.Y, maxX - minX + 1, maxY - minY + 1}
}

// textureSize читает собственный размер текстуры по пути из .screen.
// Расширения перебираются: движок ремастера подменяет .tga на .rtx, поэтому
// рядом с путём из .screen может лежать другая версия файла.
func textureSize(tree *modapi.ModTree, screenPath string) [2]int {
	img, ok := textureImage(tree, screenPath)
	if !ok {
		return [2]int{}
	}
	bounds := img.Bounds()
	return [2]int{bounds.Dx(), bounds.Dy()}
}

func textureImage(tree *modapi.ModTree, screenPath string) (image.Image, bool) {
	base := `Data\` + strings.ReplaceAll(screenPath, "/", `\`)
	stem := base[:strings.LastIndexByte(base, '.')]
	for _, ext := range []string{".tga", ".rtx", ".rsh", ".dds"} {
		data, err := tree.ReadFile(stem + ext)
		if err != nil {
			continue
		}
		img, err := imgdec.Decode(strings.TrimPrefix(ext, "."), data)
		if err != nil {
			continue
		}
		return img, true
	}
	return nil, false
}

func shortPath(path string) string {
	if idx := strings.Index(path, "textures/"); idx >= 0 {
		return path[idx+len("textures/"):]
	}
	return path
}

func preferSeparated(images []screenui.Image) []screenui.Image {
	separated := map[string]bool{}
	for _, image := range images {
		if stem, ok := separatedStem(image.Path); ok {
			separated[stem] = true
		}
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
