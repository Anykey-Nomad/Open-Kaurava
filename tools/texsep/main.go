// Command texsep декодирует текстуры нижних панелей интерфейса
// (map_separated / taskbar_separated) и печатает границы их непрозрачного
// содержимого: по ним видно, где у блоков реальный зазор, а где — пустые
// поля внутри текстуры, которые нельзя принимать за содержимое панели.
//
//	go run ./tools/texsep [-out папка]
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/Anykey-Nomad/Open-Kaurava/ui"

	"dow-mod-studio/imgdec"
	"dow-mod-studio/modapi"
)

const dowRoot = `C:\Program Files\Steam\steamapps\common\Dawn of War Definitive Edition`

// paths — текстуры нижних панелей из gamescreen.screen; путь перебирается
// так же, как в игре: расовый скин → taskbar_share, .tga → .dds → .rtx.
var paths = []string{
	`Data/art/ui/textures/taskbar/taskbar_share/map_separated.tga`,
	`Data/art/ui/textures/taskbar/taskbar_share/taskbar_separated.tga`,
	`Data/art/ui/textures/taskbar/taskbar_share/taskbar.tga`,
}

// skinDirs — папки скинов панелей, которые движок подставляет вместо share.
var skinDirs = []string{"necron", "taskbar_share"}

var exts = []string{".tga", ".dds", ".rtx"}

func main() {
	out := flag.String("out", ".tmp_ui_sep", "куда писать PNG")
	flag.Parse()

	modules, err := modapi.DiscoverModules(dowRoot)
	if err != nil {
		fatal("discover: %v", err)
	}
	mod := modapi.FindModule(modules, "DoWDE")
	if mod == nil {
		fatal("модуль DoWDE не найден")
	}
	tree, err := modapi.BuildModTree(mod.Path, "English")
	if err != nil {
		fatal("дерево модуля: %v", err)
	}
	defer tree.Close()

	if err := os.MkdirAll(*out, 0o755); err != nil {
		fatal("%v", err)
	}
	for _, p := range paths {
		var (
			data []byte
			used string
			err  error
		)
		for _, cand := range candidates(p) {
			data, err = tree.ReadFile(cand)
			if err == nil {
				used = cand
				break
			}
		}
		if data == nil {
			fmt.Printf("%-28s НЕ НАЙДЕН: %v\n", filepath.Base(p), err)
			continue
		}
		img, err := decode(data)
		if err != nil {
			fmt.Printf("%-28s (%s) декодер отказал: %v\n", filepath.Base(p), used, err)
			continue
		}
		fmt.Printf("  источник: %s\n", used)
		b := opaqueBounds(img)
		fmt.Printf("%-28s size=%s непрозрачный=%s\n",
			filepath.Base(p), rectStr(img.Bounds()), rectStr(b))
		dst := filepath.Join(*out, filepath.Base(p)+".png")
		f, err := os.Create(dst)
		if err != nil {
			fatal("%v", err)
		}
		if err := png.Encode(f, img); err != nil {
			f.Close()
			fatal("%v", err)
		}
		f.Close()
		fmt.Printf("  -> %s\n", dst)
	}
}

// candidates — пути текстуры в том порядке, в каком их пробует игра.
func candidates(p string) []string {
	p = strings.ReplaceAll(p, "/", `\`)
	i := strings.Index(strings.ToLower(p), `textures\taskbar\`)
	if i < 0 {
		return []string{p}
	}
	seg := i + len(`textures\taskbar\`)
	j := seg
	for j < len(p) && p[j] != '\\' {
		j++
	}
	rest := p[j:]
	stem := rest
	if dot := strings.LastIndex(rest, "."); dot >= 0 {
		stem = rest[:dot]
	}
	var out []string
	for _, d := range skinDirs {
		for _, ext := range exts {
			out = append(out, p[:seg]+d+stem+ext)
		}
	}
	return out
}

// decode декодирует текстуру любого поддерживаемого формата панелей.
func decode(data []byte) (image.Image, error) {
	switch {
	case len(data) >= 12 && string(data[:12]) == "Relic Chunky":
		return imgdec.Decode("rtx", data)
	case len(data) >= 4 && string(data[:4]) == "DDS ":
		return imgdec.Decode("dds", data)
	}
	return ui.DecodeTGA(data)
}

// opaqueBounds — прямоугольник пикселей с ненулевой альфой.
func opaqueBounds(img image.Image) image.Rectangle {
	b := img.Bounds()
	out := image.Rectangle{}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			r := image.Rect(x, y, x+1, y+1)
			if out.Empty() {
				out = r
			} else {
				out = out.Union(r)
			}
		}
	}
	return out
}

func rectStr(r image.Rectangle) string {
	if r.Empty() {
		return "пусто"
	}
	return fmt.Sprintf("(%d,%d %dx%d)", r.Min.X, r.Min.Y, r.Dx(), r.Dy())
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
