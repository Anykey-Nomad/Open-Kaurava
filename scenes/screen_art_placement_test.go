package scenes

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/Anykey-Nomad/Open-Kaurava/screenui"
)

// TestArtBandFindsOverhang проверяет, что полоса арта ищется только когда
// содержимое начинается ниже верха кадра и доходит до его низа.
func TestArtBandFindsOverhang(t *testing.T) {
	opaque := color.RGBA{R: 255, A: 255}
	// 8×10, содержимое в нижних 6 строках: надставка 4 строки сверху.
	panel := image.NewRGBA(image.Rect(0, 0, 8, 10))
	for y := 4; y < 10; y++ {
		for x := 0; x < 8; x++ {
			panel.Set(x, y, opaque)
		}
	}
	band, shifted := artBand(panel)
	if !shifted {
		t.Fatal("надставка панели должна распознаваться")
	}
	if math.Abs(float64(band[0]-0.4)) > 0.001 || math.Abs(float64(band[1]-1)) > 0.001 {
		t.Fatalf("полоса панели = %v, ждали [0.4 1]", band)
	}

	// Содержимое в верхней половине (рамка меню) — не режем.
	menu := image.NewRGBA(image.Rect(0, 0, 8, 10))
	for y := 0; y < 5; y++ {
		for x := 0; x < 8; x++ {
			menu.Set(x, y, opaque)
		}
	}
	if _, shifted := artBand(menu); shifted {
		t.Fatal("арт, не доходящий до низа кадра, трогать нельзя")
	}
	// Мелкая надставка — тоже не надставка.
	thin := image.NewRGBA(image.Rect(0, 0, 8, 20))
	for y := 1; y < 20; y++ {
		for x := 0; x < 8; x++ {
			thin.Set(x, y, opaque)
		}
	}
	if _, shifted := artBand(thin); shifted {
		t.Fatal("надставка меньше десятой части кадра не режется")
	}
}

// TestArtPlacementFitsPanel: видимая часть кадра ложится в прямоугольник
// консоли 895×211 точно — верх панели на верхней границе, низ на нижней, а
// ширина остаётся 1:1 (кадр 1580 пикселей в 895 точек).
func TestArtPlacementFitsPanel(t *testing.T) {
	rect := screenui.Rect{X: 728, Y: 658, W: 895, H: 211}
	band := [2]float32{134.0 / 512.0, 1}
	placed := artPlacement(rect, band)
	if placed.W != rect.W || placed.X != rect.X {
		t.Fatalf("ширина должна остаться 1:1 по прямоугольнику: %+v", placed)
	}
	panelTop := placed.Y + band[0]*placed.H
	if math.Abs(float64(panelTop-rect.Y)) > 0.01 {
		t.Fatalf("верх панели %g должен быть на верхней границе %g", panelTop, rect.Y)
	}
	if math.Abs(float64(placed.Y+placed.H-(rect.Y+rect.H))) > 0.01 {
		t.Fatalf("низ панели должен совпасть с низом прямоугольника: %+v", placed)
	}
	// Кадр при этом выше прямоугольника на долю надставки — она уходит вверх.
	if math.Abs(float64(placed.H-rect.H/(band[1]-band[0]))) > 0.01 {
		t.Fatalf("высота кадра = %g, ждали %g", placed.H, rect.H/(band[1]-band[0]))
	}
	// Пустой прямоугольник и вырожденная полоса — без изменений.
	if got := artPlacement(screenui.Rect{}, band); got != (screenui.Rect{}) {
		t.Fatalf("пустой прямоугольник менять нечего: %+v", got)
	}
	if got := artPlacement(rect, [2]float32{0.5, 0.5}); got != rect {
		t.Fatalf("вырожденная полоса должна оставить прямоугольник: %+v", got)
	}
}
