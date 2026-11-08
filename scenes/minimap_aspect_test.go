package scenes

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/Anykey-Nomad/Open-Kaurava/screenui"
)

// TestMinimapPointFillsDiamond проверяет, что ромб миникарты одинаков для
// любой карты: углы прямоугольной карты (3:1, как absolute_zero) попадают
// в середины сторон области, а не в её углы — иначе по диагонали остаются
// чёрные поля.
func TestMinimapPointFillsDiamond(t *testing.T) {
	hud := &GameplayHUD{}
	hud.minimapMin = [2]float32{0, 0}
	hud.minimapMax = [2]float32{300, 100}
	area := screenui.Rect{W: 200, H: 200}
	corners := [][2]float64{{0, 0}, {300, 0}, {300, 100}, {0, 100}}
	for _, corner := range corners {
		x, y, ok := hud.minimapPoint(area, corner[0], corner[1])
		if !ok {
			t.Fatalf("угол %v не перевёлся", corner)
		}
		if !isEdgeMid(float64(x), float64(y), float64(area.W), float64(area.H)) {
			t.Fatalf("угол %v попал в (%0.1f,%0.1f), а должен в середину стороны", corner, x, y)
		}
	}
}

// isEdgeMid — точка лежит на середине стороны прямоугольника.
func isEdgeMid(x, y, w, h float64) bool {
	const eps = 0.6
	atX := math.Abs(x) < eps || math.Abs(x-w) < eps
	atY := math.Abs(y) < eps || math.Abs(y-h) < eps
	return (atX && math.Abs(y-h/2) < eps) || (atY && math.Abs(x-w/2) < eps)
}

// TestMinimapCanvasFillsDiamond проверяет, что снимок прямоугольной карты
// заполняет ромб панели: середина каждой стороны канвы непрозрачна, а её
// углы — прозрачны.
func TestMinimapCanvasFillsDiamond(t *testing.T) {
	hud := &GameplayHUD{}
	hud.minimapMin = [2]float32{0, 0}
	hud.minimapMax = [2]float32{300, 100}
	hud.minimapGroundMin = [2]float32{0, 0}
	hud.minimapGroundMax = [2]float32{300, 100}
	src := image.NewRGBA(image.Rect(0, 0, 300, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 300; x++ {
			src.SetRGBA(x, y, color.RGBA{R: 40, G: 60, B: 30, A: 255})
		}
	}
	canvas := hud.minimapCanvas(src)
	if canvas == nil {
		t.Fatal("канва не собралась")
	}
	bounds := canvas.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	mid := func(x, y int) bool {
		_, _, _, a := canvas.At(x, y).RGBA()
		return a > 0x8000
	}
	// Небольшой отступ от края: на самой границе сэмпл попадает на ребро.
	inset := 3
	edges := [][2]int{{w / 2, inset}, {w - 1 - inset, h / 2}, {w / 2, h - 1 - inset}, {inset, h / 2}}
	for _, point := range edges {
		if !mid(point[0], point[1]) {
			t.Fatalf("середина стороны %v пустая: ромб не заполнен", point)
		}
	}
	corners := [][2]int{{inset, inset}, {w - 1 - inset, inset}, {inset, h - 1 - inset}, {w - 1 - inset, h - 1 - inset}}
	for _, point := range corners {
		if mid(point[0], point[1]) {
			t.Fatalf("угол %v закрашен: ромб вышел за свои границы", point)
		}
	}
}
