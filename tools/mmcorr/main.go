// Command mmcorr подбирает ориентацию карты проходимости (DATAIMAP)
// относительно миникарты сценария: тёмные участки миникарты — руины и
// стены, они обязаны совпадать с непроходимыми ячейками сетки. Для всех
// комбинаций транспонирования и отражений считается средняя яркость
// под «закрытыми» и под «открытыми» ячейками; выигрывает комбинация,
// где закрытые заметно темнее.
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/Anykey-Nomad/Open-Kaurava/ui"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: mmcorr map.sgb-pass.txt minimap.tga")
		os.Exit(2)
	}
	// ASCII-карта из passprobe: строки из '#', '.' и '+'; '#' = 1 (непроходимо).
	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()
	var grid []string
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		line := strings.TrimRight(scan.Text(), "\r")
		if line == "" {
			continue
		}
		grid = append(grid, line)
	}
	if len(grid) < 2 {
		fmt.Fprintln(os.Stderr, "пустая карта проходимости")
		os.Exit(1)
	}
	w, h := len(grid[0]), len(grid)
	pass := make([]byte, w*h)
	for y, line := range grid {
		for x := 0; x < w && x < len(line); x++ {
			if line[x] == '#' {
				pass[y*w+x] = 1
			}
		}
	}

	data, err := os.ReadFile(os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	img, err := ui.DecodeTGA(data)
	if err != nil {
		fmt.Fprintln(os.Stderr, "decode:", err)
		os.Exit(1)
	}
	b := img.Bounds()

	type combo struct {
		transpose, flipX, flipY bool
	}
	for _, c := range []combo{
		{false, false, false}, {false, true, false}, {false, false, true}, {false, true, true},
		{true, false, false}, {true, true, false}, {true, false, true}, {true, true, true},
	} {
		var blockedSum, openSum float64
		var blockedN, openN int
		for cy := 0; cy < h; cy++ {
			for cx := 0; cx < w; cx++ {
				v := pass[cy*w+cx]
				if v != 1 && v != 0 && v != 2 {
					continue
				}
				gx, gy := cx, cy
				if c.transpose {
					gx, gy = gy, gx
				}
				if c.flipX {
					gx = w - 1 - gx
				}
				if c.flipY {
					gy = h - 1 - gy
				}
				px := b.Min.X + gx*b.Dx()/w + b.Dx()/w/2
				py := b.Min.Y + gy*b.Dy()/h + b.Dy()/h/2
				r, g, bl, _ := img.At(px, py).RGBA()
				lum := (float64(r>>8)*0.299 + float64(g>>8)*0.587 + float64(bl>>8)*0.114)
				if v == 1 {
					blockedSum += lum
					blockedN++
				} else {
					openSum += lum
					openN++
				}
			}
		}
		if blockedN == 0 || openN == 0 {
			continue
		}
		blockedAvg, openAvg := blockedSum/float64(blockedN), openSum/float64(openN)
		fmt.Printf("transpose=%-5v flipX=%-5v flipY=%-5v → закрытые=%.1f (%d), открытые=%.1f (%d), разница=%+.1f\n",
			c.transpose, c.flipX, c.flipY, blockedAvg, blockedN, openAvg, openN, blockedAvg-openAvg)
	}
}
