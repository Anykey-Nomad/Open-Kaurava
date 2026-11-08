package scenes

import (
	"testing"

	"dow-mod-studio/sgbview"
	"github.com/Anykey-Nomad/Open-Kaurava/scene"
)

func TestPlayersStartsGameplayAfterConfirm(t *testing.T) {
	previousSetup := Setup
	previousSelected, previousPath := Selected, SelectedPath
	defer func() {
		Setup = previousSetup
		Select(previousSelected, previousPath)
	}()
	Select("test-map", "test-map.sgb")

	players := &Players{
		slots:  []sgbview.PlayerSlot{{Owner: 2}},
		own:    2,
		pick:   map[int]int{2: -1},
		path:   SelectedPath,
		loaded: SelectedPath,
	}
	ctx := scene.Ctx{
		W: 1280,
		H: 720,
		Input: scene.Input{
			MouseX: int(playersLeft + 8),
			MouseY: 630,
			Clicks: []scene.Click{{Button: 1, X: int(playersLeft + 8), Y: 630}},
		},
	}
	var switched string
	ctx.Switch = func(name string) { switched = name }

	players.Update(&ctx, 0)

	if switched != "gameplay" {
		t.Fatalf("start should enter gameplay, switched to %q", switched)
	}
	if Setup.Own != 2 || len(Setup.Slots) != 1 || Setup.Slots[0].Owner != 2 {
		t.Fatalf("player setup was not confirmed before gameplay: %+v", Setup)
	}
}
