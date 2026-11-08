// Command mmpng конвертирует TGA-миникарту сценария в PNG для
// визуального сравнения с картой проходимости (passprobe).
package main

import (
	"fmt"
	"image/png"
	"os"

	"github.com/Anykey-Nomad/Open-Kaurava/ui"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: mmpng in.tga out.png")
		os.Exit(2)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	img, err := ui.DecodeTGA(data)
	if err != nil {
		fmt.Fprintln(os.Stderr, "decode:", err)
		os.Exit(1)
	}
	out, err := os.Create(os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer out.Close()
	if err := png.Encode(out, img); err != nil {
		fmt.Fprintln(os.Stderr, "encode:", err)
		os.Exit(1)
	}
	fmt.Println("ok", img.Bounds())
}
