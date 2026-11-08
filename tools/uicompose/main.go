// Command uicompose собирает нижнюю панель интерфейса из текстур панелей на
// прямоугольниках из gamescreen.screen: показывает, как встают блок миникарты
// и консоль, и есть ли между ними зазор, как в настоящей игре.
//
//	go run ./tools/uicompose -w 1280 -h 720
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"

	"golang.org/x/image/draw"
)

// panel — текстура панели и её прямоугольник в экране (нормированные
// значения .screen, умноженные на размер окна; см. tools/screendump).
type panel struct {
	tex   string
	x, y  float64
	w, h  float64
	title string
}

var panels = []panel{
	{`.tmp_ui_sep/map_separated.tga.png`, -19, 478, 378, 240, "artTaskbarLeft (map_separated)"},
	{`.tmp_ui_sep/taskbar_separated.tga.png`, 318, 478, 988, 240, "artTaskbarRight (taskbar_separated)"},
}

func main() {
	width := flag.Int("w", 1280, "ширина экрана")
	height := flag.Int("h", 720, "высота экрана")
	out := flag.String("out", ".tmp_ui_sep/compose.png", "куда писать результат")
	flag.Parse()

	dst := image.NewNRGBA(image.Rect(0, 0, *width, *height))
	// Фон — серый, чтобы прозрачные поля текстур были видны.
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.NRGBA{90, 96, 104, 255}), image.Point{}, draw.Src)

	for _, p := range panels {
		src, err := decode(p.tex)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", p.tex, err)
			continue
		}
		r := image.Rect(int(p.x), int(p.y), int(p.x+p.w), int(p.y+p.h))
		draw.CatmullRom.Scale(dst, r, src, src.Bounds(), draw.Over, nil)
		fmt.Printf("%-34s rect=(%d,%d %gx%g)\n", p.title, int(p.x), int(p.y), p.w, p.h)
	}

	f, err := os.Create(*out)
	if err != nil {
		fatal("%v", err)
	}
	defer f.Close()
	if err := png.Encode(f, dst); err != nil {
		fatal("%v", err)
	}
	fmt.Println("->", *out)
}

func decode(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
