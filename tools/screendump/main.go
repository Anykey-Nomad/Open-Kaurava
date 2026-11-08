// Command screendump печатает дерево виджетов .screen-файла: имена, типы и
// прямоугольники. Нужно, чтобы найти, куда игрока показывает ресурсы
// (grpResourceInfo) и какие там подписи.
//
// Файл берётся из объединённого дерева модуля — как его берёт движок.
package main

import (
	"flag"
	"fmt"
	"os"

	"dow-mod-studio/modapi"

	"github.com/Anykey-Nomad/Open-Kaurava/screenui"
)

const dowRoot = `C:\Program Files\Steam\steamapps\common\Dawn of War Definitive Edition`

func main() {
	root := flag.String("root", dowRoot, "путь к установленной Dawn of War")
	module := flag.String("module", "DoWDE", "модуль")
	file := flag.String("file", `Data\art\ui\screens\gamescreen.screen`, "файл .screen")
	group := flag.String("group", "", "печатать только поддерево группы с таким именем")
	depth := flag.Int("depth", 4, "на какую глубину печатать")
	chain := flag.String("chain", "", "печатать предков виджета от корня с флагами видимости")
	width := flag.Float64("w", 1280, "ширина окна для рамок")
	height := flag.Float64("h", 720, "высота окна для рамок")
	flag.Parse()

	modules, err := modapi.DiscoverModules(*root)
	if err != nil {
		fatal("discover: %v", err)
	}
	mod := modapi.FindModule(modules, *module)
	if mod == nil {
		fatal("модуль не найден: %s", *module)
	}
	tree, err := modapi.BuildModTree(mod.Path, "English")
	if err != nil {
		fatal("дерево модуля: %v", err)
	}
	defer tree.Close()

	data, err := tree.ReadFile(*file)
	if err != nil {
		fatal("чтение %s: %v", *file, err)
	}
	screen, err := screenui.Parse(data)
	if err != nil {
		fatal("разбор %s: %v", *file, err)
	}
	var target *screenui.Widget = screen.Widgets
	if *group != "" {
		target = screen.Find(*group)
		if target == nil {
			fatal("группа не найдена: %s", *group)
		}
	}
	if target == nil {
		fatal("в файле нет корневого дерева виджетов")
	}
	if *chain != "" {
		w := screen.Find(*chain)
		if w == nil {
			fatal("виджет не найден: %s", *chain)
		}
		printChain(w)
		return
	}
	printWidget(target, 0, *depth, screenui.Rect{W: float32(*width), H: float32(*height)})
}

// printChain печатает путь от корня до виджета: имя, тип и флаг видимости
// каждого узла — по нему видно, почему движок виджет не рисует.
func printChain(w *screenui.Widget) {
	var path []*screenui.Widget
	for cur := w; cur != nil; cur = cur.Parent {
		path = append(path, cur)
	}
	for i := len(path) - 1; i >= 0; i-- {
		cur := path[i]
		fmt.Printf("%s%-28s %-12s visible=%v\n",
			repeat("  ", len(path)-1-i), cur.Name, cur.Type, cur.Visible == nil || *cur.Visible)
	}
}

func printWidget(w *screenui.Widget, level, maxLevel int, viewport screenui.Rect) {
	if w == nil || level > maxLevel {
		return
	}
	rect := w.Layout(viewport)
	ax, ay := w.Anchor()
	fmt.Printf("%s%-28s %-12s visible=%v rect=(%d,%d %dx%d) anchor=(%d,%d) raw=%s\n",
		repeat("  ", level), w.Name, w.Type, w.Visible == nil || *w.Visible,
		int(rect.X), int(rect.Y), int(rect.W), int(rect.H), ax, ay, rawGeometry(w))
	if tex := w.TexturePath(); tex != "" {
		fmt.Printf("%s%stex=%s\n", repeat("  ", level+1), repeat("  ", 30), tex)
	}
	for _, child := range w.Children {
		printWidget(child, level+1, maxLevel, viewport)
	}
}

// rawGeometry печатает авторские position/size виджета как есть — по ним
// видно, как движок считает геометрию: значения нормализованы к вьюпорту
// или к родителю, а наивная семантика "размер = вьюпорт" им не соответствует.
func rawGeometry(w *screenui.Widget) string {
	var out string
	if w.HasPosition {
		out += fmt.Sprintf("pos=(%.4f,%.4f)", w.Position[0], w.Position[1])
	} else {
		out += "pos=-"
	}
	if w.HasSize {
		out += fmt.Sprintf(" size=(%.4f,%.4f)", w.Size[0], w.Size[1])
	} else {
		out += " size=-"
	}
	if w.Owner != nil {
		out += fmt.Sprintf(" owner=%s", w.Owner.Name)
	}
	return out
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
