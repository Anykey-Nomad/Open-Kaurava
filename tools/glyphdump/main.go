// Диагностика растеризации глифов: печатает маску символа ASCII-артом
// (строки памяти сверху вниз) вместе с dr, maskp и advance.
//
//	go run ./tools/glyphdump [символ] [размер]
package main

import (
	"fmt"
	"image"
	"os"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

func main() {
	r := 'A'
	size := 32
	if len(os.Args) > 1 {
		for _, c := range os.Args[1] {
			r = c
			break
		}
	}
	if len(os.Args) > 2 {
		fmt.Sscanf(os.Args[2], "%d", &size)
	}

	data, err := os.ReadFile(`C:\Windows\Fonts\segoeui.ttf`)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	src, err := opentype.Parse(data)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	face, err := opentype.NewFace(src, &opentype.FaceOptions{
		Size:    float64(size),
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer face.Close()

	m := face.Metrics()
	fmt.Printf("метрики: ascent=%d descent=%d height=%d\n", m.Ascent, m.Descent, m.Height)

	dr, mask, maskp, adv, ok := face.Glyph(fixed.Point26_6{}, r)
	if !ok {
		fmt.Fprintln(os.Stderr, "глиф не растеризован")
		os.Exit(1)
	}
	fmt.Printf("символ %q: dr=%v (Min.Y=%d Max.Y=%d) maskp=%v advance=%d\n",
		r, dr, dr.Min.Y, dr.Max.Y, maskp, adv)

	alpha, ok := mask.(*image.Alpha)
	if !ok {
		fmt.Printf("маска не *image.Alpha: %T\n", mask)
		return
	}
	fmt.Printf("маска: rect=%v stride=%d\n", alpha.Rect, alpha.Stride)

	// Строки маски в порядке памяти: сверху (row 0) вниз.
	for j := 0; j < alpha.Rect.Dy(); j++ {
		line := make([]byte, 0, alpha.Rect.Dx())
		for i := 0; i < alpha.Rect.Dx(); i++ {
			a := alpha.AlphaAt(alpha.Rect.Min.X+i, alpha.Rect.Min.Y+j).A
			switch {
			case a > 200:
				line = append(line, '#')
			case a > 80:
				line = append(line, '+')
			case a > 10:
				line = append(line, '.')
			default:
				line = append(line, ' ')
			}
		}
		fmt.Printf("%3d |%s|\n", j, string(line))
	}

	// Где в маске лежит dr.Min.Y (по мнению opentype) — для сверки.
	fmt.Printf("maskp.Y=%d  dr.Min.Y=%d  dr.Max.Y=%d  dr.Dy=%d\n",
		maskp.Y, dr.Min.Y, dr.Max.Y, dr.Dy())
}
