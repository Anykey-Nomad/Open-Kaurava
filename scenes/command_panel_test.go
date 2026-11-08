package scenes

import (
	"testing"

	"github.com/Anykey-Nomad/Open-Kaurava/scene"
)

// TestCommandPanelUsesTwelveCellGrid — панель приказов повторяет сетку
// gamescreen.screen DoWDE: двенадцать клеток CommandIcon01..12 в четыре
// столбца и три ряда, «Строить» подменяет сетку подменю строительства с
// кнопкой возврата.
func TestCommandPanelUsesTwelveCellGrid(t *testing.T) {
	game := newDemoGameplayHUD()
	ctx := scene.Ctx{W: 1280, H: 720}
	game.updateHUD(&ctx)
	if len(game.orderButtons) != commandColumns*commandRows {
		t.Fatalf("клеток панели = %d, want %d", len(game.orderButtons), commandColumns*commandRows)
	}
	build := cellWithAction(game.orderActions, commandBuild)
	if build < 0 {
		t.Fatal("в сетке приказов нет кнопки «Строить»")
	}
	clickCell(&ctx, game.orderButtons[build])
	game.updateHUD(&ctx)
	if game.orderMode != orderModeBuild {
		t.Fatalf("режим панели = %q, want подменю строительства", game.orderMode)
	}

	// Следующий кадр раскладывает уже подменю.
	ctx.Input.Clicks = nil
	game.updateHUD(&ctx)
	back := cellWithAction(game.orderActions, commandBack)
	if back < 0 {
		t.Fatal("в подменю строительства нет кнопки возврата")
	}
	clickCell(&ctx, game.orderButtons[back])
	game.updateHUD(&ctx)
	if game.orderMode != orderModeRoot {
		t.Fatalf("после возврата режим панели = %q, want сетку приказов", game.orderMode)
	}
}

// TestCommandPanelArmsOrderForNextClick — нажатие «Движение» выбирает приказ,
// который отдаётся следующим левым кликом по карте, а не меняет выделение.
func TestCommandPanelArmsOrderForNextClick(t *testing.T) {
	game := newDemoGameplayHUD()
	ctx := scene.Ctx{W: 1280, H: 720}
	game.updateHUD(&ctx)
	move := cellWithAction(game.orderActions, orderMove)
	if move < 0 {
		t.Fatal("в сетке приказов нет кнопки движения")
	}
	clickCell(&ctx, game.orderButtons[move])
	game.updateHUD(&ctx)
	if game.pendingOrder != orderMove {
		t.Fatalf("выбранный приказ = %q, want %q", game.pendingOrder, orderMove)
	}

	issued := ""
	game.orderAt = func(order string, c *scene.Ctx, x, y int) {
		issued = order
	}
	ctx.Input.Clicks = []scene.Click{{Button: 1, X: 900, Y: 400}}
	game.updateHUD(&ctx)
	if issued != orderMove {
		t.Fatalf("приказ по клику = %q, want %q", issued, orderMove)
	}
	if game.pendingOrder != orderNone {
		t.Fatalf("приказ остался выбранным: %q", game.pendingOrder)
	}
	if _, ok := game.selectedEntity(); !ok {
		t.Fatal("указание цели не должно сбрасывать выделение")
	}
}

func cellWithAction(actions []string, want string) int {
	for i, action := range actions {
		if action == want {
			return i
		}
	}
	return -1
}

func clickCell(ctx *scene.Ctx, cell item) {
	ctx.Input.Clicks = []scene.Click{{Button: 1, X: int(cell.x + 2), Y: int(cell.y + 2)}}
}
