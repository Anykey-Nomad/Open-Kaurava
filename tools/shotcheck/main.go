// Проверка снимка окна: находит рисованный курсор (белые пиксели) и
// заголовок (голубой цвет) и печатает их рамки.
//
//	go run ./tools/shotcheck screenshots/menu-baseline.png -x 409 -y 238
//
// Курсор должен лежать ровно в той точке, где стоит системная мышь,
// поэтому рамка белых пикселей — проверка выравнивания ввода и отрисовки.
// С -x/-y задаётся ОЖИДАЕМАЯ позиция курсора в координатах снимка: скан
// идёт по окну ±50 px, и печатается смещение рамки относительно ожидания.
package main

import (
	"flag"
	"fmt"
	"image"
	_ "image/png"
	"os"
	"strings"
)

func main() {
	ex := flag.Int("x", -1, "ожидаемая позиция курсора по X (в координатах снимка)")
	ey := flag.Int("y", -1, "ожидаемая позиция курсора по Y")
	panel := flag.Bool("panel", false, "найти рамку кнопок (панели) и напечатать её границы")
	probes := flag.String("p", "", "точки для проб: \"x1,y1;x2,y2\" — печатает цвет в каждой")
	flag.Parse()
	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: shotcheck <png> [-x px] [-y px]")
		os.Exit(1)
	}
	f, err := os.Open(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	b := img.Bounds()
	fmt.Printf("размер снимка: %dx%d\n", b.Dx(), b.Dy())

	// Область поиска белых пикселей: весь кадр или окно вокруг ожидания.
	scanMinX, scanMinY := b.Min.X, b.Min.Y
	scanMaxX, scanMaxY := b.Max.X, b.Max.Y
	if *ex >= 0 && *ey >= 0 {
		scanMinX, scanMinY = *ex-50, *ey-50
		scanMaxX, scanMaxY = *ex+50, *ey+50
		fmt.Printf("область поиска: (%d,%d)-(%d,%d), ожидание курсора (%d,%d)\n",
			scanMinX, scanMinY, scanMaxX, scanMaxY, *ex, *ey)
	}

	whiteMinX, whiteMinY := b.Max.X, b.Max.Y
	whiteMaxX, whiteMaxY := b.Min.X, b.Min.Y
	whiteN := 0

	cyanMinX, cyanMinY := b.Max.X, b.Max.Y
	cyanMaxX, cyanMaxY := b.Min.X, b.Min.Y
	cyanN := 0

	for y := scanMinY; y < scanMaxY; y++ {
		for x := scanMinX; x < scanMaxX; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			r8, g8, b8 := r>>8, g>>8, bl>>8
			switch {
			case r8 > 230 && g8 > 230 && b8 > 230: // белый (курсор, текст кнопок)
				whiteN++
				if x < whiteMinX {
					whiteMinX = x
				}
				if x > whiteMaxX {
					whiteMaxX = x
				}
				if y < whiteMinY {
					whiteMinY = y
				}
				if y > whiteMaxY {
					whiteMaxY = y
				}
			case b8 > 150 && g8 > 120 && r8 < 120 && *ex < 0: // голубой (заголовок)
				cyanN++
				if x < cyanMinX {
					cyanMinX = x
				}
				if x > cyanMaxX {
					cyanMaxX = x
				}
				if y < cyanMinY {
					cyanMinY = y
				}
				if y > cyanMaxY {
					cyanMaxY = y
				}
			}
		}
	}

	if *probes != "" {
		for _, p := range strings.Split(*probes, ";") {
			var x, y int
			if _, err := fmt.Sscanf(p, "%d,%d", &x, &y); err != nil {
				continue
			}
			r, g, bl, _ := img.At(x, y).RGBA()
			fmt.Printf("точка (%d,%d): rgb(%d,%d,%d)\n", x, y, r>>8, g>>8, bl>>8)
		}
	}

	if *panel {
		// Панель кнопок — заметно светлее фона; ищем её границы.
		minX, minY, maxX, maxY := b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
		n := 0
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				r, g, bl, _ := img.At(x, y).RGBA()
				r8, g8, b8 := int(r>>8), int(g>>8), int(bl>>8)
				if r8 >= 78 && r8 <= 92 && g8 >= 85 && g8 <= 99 && b8 >= 100 && b8 <= 115 {
					n++
					if x < minX {
						minX = x
					}
					if x > maxX {
						maxX = x
					}
					if y < minY {
						minY = y
					}
					if y > maxY {
						maxY = y
					}
				}
			}
		}
		if n > 0 {
			fmt.Printf("панель кнопок: %d px, рамка (%d,%d)-(%d,%d)\n", n, minX, minY, maxX, maxY)
		} else {
			fmt.Println("панель кнопок не найдена")
		}
	}

	if whiteN > 0 {
		fmt.Printf("белые пиксели: %d, рамка (%d,%d)-(%d,%d), центр (%.1f, %.1f)\n",
			whiteN, whiteMinX, whiteMinY, whiteMaxX, whiteMaxY,
			float64(whiteMinX+whiteMaxX)/2, float64(whiteMinY+whiteMaxY)/2)
		if *ex >= 0 && *ey >= 0 {
			cx := float64(whiteMinX+whiteMaxX) / 2
			cy := float64(whiteMinY+whiteMaxY) / 2
			fmt.Printf("смещение от ожидания: dx=%.1f dy=%.1f\n", cx-float64(*ex), cy-float64(*ey))
		}
	} else {
		fmt.Println("белых пикселей нет")
	}
	if cyanN > 0 {
		fmt.Printf("голубые пиксели: %d, рамка (%d,%d)-(%d,%d)\n",
			cyanN, cyanMinX, cyanMinY, cyanMaxX, cyanMaxY)
	}
}
