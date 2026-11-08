package scenes

import (
	"slices"
	"strings"
	"testing"

	"dow-mod-studio/racebps"

	"github.com/Anykey-Nomad/Open-Kaurava/entities"
	"github.com/Anykey-Nomad/Open-Kaurava/scene"
	"github.com/Anykey-Nomad/Open-Kaurava/screenui"
)

func TestGameplaySceneUsesGameplayHUD(t *testing.T) {
	game := NewGameplay(nil, nil)
	if game.Name() != "gameplay" {
		t.Fatalf("world scene name = %q, want gameplay", game.Name())
	}
	if game.hud == nil {
		t.Fatal("gameplay scene should own a gameplay HUD")
	}
	ctx := scene.Ctx{W: 1280, H: 720}
	game.hud.layout(&ctx)
	if len(game.hud.topButtons) != 3 || len(game.hud.orderButtons) == 0 {
		t.Fatal("gameplay scene should have top and command HUD buttons")
	}
}

func TestGameplaySelectsEnemyButDoesNotAcceptOrders(t *testing.T) {
	game := newDemoGameplayHUD()
	ctx := scene.Ctx{W: 1280, H: 720}

	game.updateHUD(&ctx)
	if !game.canIssueOrders() {
		t.Fatal("initial friendly entity should allow orders")
	}

	ctx.Input.Clicks = []scene.Click{{Button: 1, X: 740, Y: 250}}
	game.updateHUD(&ctx)
	selected, ok := game.selectedEntity()
	if !ok || selected.Name != "Scout" || selected.Owner != "red" {
		t.Fatalf("enemy selection = %+v, found=%v", selected, ok)
	}
	if game.canIssueOrders() {
		t.Fatal("enemy entity must not allow orders")
	}
	if game.command != "" {
		t.Fatalf("enemy selection should not expose command state, got %q", game.command)
	}

	game.layout(&ctx)
	order := game.orderButtons[0]
	ctx.Input.Clicks = []scene.Click{{Button: 1, X: int(order.x + 2), Y: int(order.y + 2)}}
	game.updateHUD(&ctx)
	if game.command != "" {
		t.Fatalf("enemy order click changed command state to %q", game.command)
	}
}

func TestGameplayHUDButtonsUseScreenWidgets(t *testing.T) {
	game := newDemoGameplayHUD()
	screen, err := screenui.Parse([]byte(`Screen = {
	Widgets = { type = "Group", name = "root", position = {0, 0}, size = {1, 1}, Children = {
		{ type = "Group", name = "grpMenuButtons", position = {0.7, 0}, size = {0.3, 0.05}, Children = {
			{ type = "Button", name = "btnOBJ", text = "$9049", position = {0, 0}, size = {0.08, 0.03} },
			{ type = "Button", name = "btnDIP", text = "$9002", position = {0.09, 0}, size = {0.08, 0.03} },
			{ type = "Button", name = "btnMENU", text = "$9047", position = {0.18, 0}, size = {0.1, 0.03} },
		} },
	} },
}`))
	if err != nil {
		t.Fatal(err)
	}
	game.screen = screen
	ctx := scene.Ctx{W: 1280, H: 720}
	game.layout(&ctx)

	menu := game.topButtons[2]
	if menu.x < 1100 || menu.x > 1150 || menu.y != 0 || menu.w != 128 || menu.h < 21 || menu.h > 22 {
		t.Fatalf("menu button should use screen geometry: %+v", menu)
	}
	switched := ""
	ctx.Switch = func(name string) { switched = name }
	ctx.Input.Clicks = []scene.Click{{Button: 1, X: int(menu.x + 4), Y: 10}}
	game.updateHUD(&ctx)
	if switched != "menu" {
		t.Fatalf("screen menu button should use its click behavior, switched to %q", switched)
	}
	if game.command != "Select your unit" {
		t.Fatalf("menu click should not be treated as a map selection: %q", game.command)
	}
}

func TestGameplayInterpretsDoWScreenGeometryAndSelectionStates(t *testing.T) {
	game := newDemoGameplayHUD()
	screen, err := screenui.Parse([]byte(`Screen = {
	Widgets = { type = "Group", name = "screen", position = {0, 0}, size = {1, 1}, Children = {
		{ type = "Group", name = "grpBackground", position = {-0.375, 0}, size = {1.75, 1}, Children = {
			{ type = "Group", name = "grpTaskbar", position = {0, 0.65}, size = {1.75, 0.35}, Children = {
				{ type = "Group", name = "grpTaskbarRight", position = {0.375, -0.65}, size = {1, 1}, Children = {
					{ type = "Group", name = "grpSingleSquadSelection", visible = false, position = {0.2816, 0.815}, size = {0.34389, 0.18} },
					{ type = "Group", name = "grpStructureSelection", position = {0.2814, 0.815}, size = {0.344, 0.18} },
					{ type = "Group", name = "grpCommandIcons", position = {0.80766, 0.815}, size = {0.17983, 0.18} },
					{ type = "Group", name = "grpBuildQueue", position = {0.63441, 0.815}, size = {0.07865, 0.179} },
					{ type = "Group", name = "grpUpgrades", position = {0.72825, 0.814}, size = {0.06656, 0.18} },
				} },
			} },
		} },
	} },
}`))
	if err != nil {
		t.Fatal(err)
	}
	game.screen = screen
	ctx := scene.Ctx{W: 1280, H: 720}
	game.layout(&ctx)

	if _, ok := game.selectedEntity(); !ok || !game.screenWidgetVisible("grpSingleSquadSelection") {
		t.Fatal("friendly unit should activate the single-squad selection template")
	}
	if game.screenWidgetVisible("grpStructureSelection") {
		t.Fatal("unit selection should not activate the structure template")
	}
	if game.screenWidgetVisible("grpBuildQueue") || !game.screenWidgetVisible("grpUpgrades") {
		t.Fatal("unit selection should show upgrades, not building production")
	}
	selection := game.screen.Find("grpSingleSquadSelection").Layout(screenui.Rect{W: 1280, H: 720})
	if selection.X < 350 || selection.Y < 580 || selection.W < 400 || selection.H < 120 {
		t.Fatalf("selection widget geometry was not resolved from .screen: %+v", selection)
	}
	if !game.screenWidgetVisible("grpCommandIcons") {
		t.Fatal("friendly unit should expose its command group")
	}

	var buildingID, enemyID string
	for _, entity := range game.factory.All() {
		switch {
		case entity.Kind == entities.KindBuilding:
			buildingID = entity.ID
		case entity.Owner == "red":
			enemyID = entity.ID
		}
	}
	game.selectedID = buildingID
	if !game.screenWidgetVisible("grpStructureSelection") {
		t.Fatal("building selection should activate the structure template")
	}
	if !game.screenWidgetVisible("grpBuildQueue") || game.screenWidgetVisible("grpUpgrades") {
		t.Fatal("building selection should show production, not squad upgrades")
	}
	game.selectedID = enemyID
	if game.screenWidgetVisible("grpCommandIcons") {
		t.Fatal("enemy selection must not expose command controls")
	}
}

func TestGameplayHUDClipsScreenPanelsAndDoesNotFillArtBounds(t *testing.T) {
	game := newDemoGameplayHUD()
	screen, err := screenui.Parse([]byte(`Screen = {
	Widgets = { type = "Group", name = "screen", position = {0, 0}, size = {1, 1}, Children = {
		{ type = "Group", name = "background", position = {-0.375, 0}, size = {1.75, 1}, Children = {
			{ type = "Group", name = "taskbar", position = {0, 0.65}, size = {1.75, 0.35}, Children = {
				{ type = "Group", name = "taskbarRight", position = {0.375, -0.65}, size = {1, 1}, Children = {
					{ type = "ArtLabel", name = "artTaskbar", position = {0, 0.65}, size = {1.75, 0.35} },
					{ type = "ArtLabel", name = "artMenubar", position = {0, 0}, size = {1, 0.17} },
					{ type = "Group", name = "grpCommandIcons", position = {0.8, 0.815}, size = {0.18, 0.18} },
				} },
			} },
		} },
	} },
}`))
	if err != nil {
		t.Fatal(err)
	}
	game.screen = screen
	viewport := screenui.Rect{W: 1920, H: 1040}
	art := game.screen.Find("artTaskbar").Layout(viewport)
	clipped := clipScreenRect(art, viewport)
	if clipped.Y < 0 || clipped.X < 0 || clipped.X+clipped.W > viewport.W || clipped.Y+clipped.H > viewport.H {
		t.Fatalf("clipped taskbar bounds exceed viewport: %+v", clipped)
	}
	if clipped.H >= viewport.H {
		t.Fatalf("taskbar art bounds should not become a full-screen-height rectangle: raw=%+v clipped=%+v", art, clipped)
	}
	if rect := clipScreenRect(screenui.Rect{X: -20, Y: -10, W: 40, H: 20}, viewport); rect != (screenui.Rect{X: 0, Y: 0, W: 20, H: 10}) {
		t.Fatalf("unexpected viewport clipping: %+v", rect)
	}
	if rect := clipScreenRect(screenui.Rect{X: -300, Y: -300, W: 20, H: 20}, viewport); rect.W != 0 || rect.H != 0 {
		t.Fatalf("offscreen rectangle should be empty after clipping: %+v", rect)
	}
}

func TestGameplayClearsSelectionOutsideEntities(t *testing.T) {
	game := newDemoGameplayHUD()
	ctx := scene.Ctx{W: 1280, H: 720, Input: scene.Input{Clicks: []scene.Click{{Button: 1, X: 900, Y: 400}}}}
	game.updateHUD(&ctx)
	if _, ok := game.selectedEntity(); ok {
		t.Fatal("clicking empty map should clear selection")
	}
}

func TestGameplayRoutesMapClickToWorldPicker(t *testing.T) {
	game := newDemoGameplayHUD()
	enemy := game.factory.All()[2]
	game.pickWorld = func(x, y, width, height int) (string, bool) {
		if x != 900 || y != 400 || width != 1280 || height != 720 {
			t.Fatalf("picker received click (%d,%d) in %dx%d", x, y, width, height)
		}
		return enemy.ID, true
	}
	ctx := scene.Ctx{W: 1280, H: 720, Input: scene.Input{Clicks: []scene.Click{{Button: 1, X: 900, Y: 400}}}}
	game.updateHUD(&ctx)
	selected, ok := game.selectedEntity()
	if !ok || selected.ID != enemy.ID {
		t.Fatalf("3D picker did not select returned object: %+v, found=%v", selected, ok)
	}
	if game.canIssueOrders() {
		t.Fatal("selecting an enemy through 3D picker must not grant orders")
	}
}

// TestRaceSkinTaskbarCandidates — панели HUD берут расовый скин: настоящая
// игра через Lua-биндинг race_image подменяет папку taskbar_share на папку
// скина своей расы, а ремастер хранит панели в .dds/.rtx, а не в .tga, как
// записано в .screen. Путь перебирается «скин → share» × «tga → dds → rtx».
func TestRaceSkinTaskbarCandidates(t *testing.T) {
	for folder, want := range map[string]string{
		"necrons":       "necron",
		"guard":         "imperial_guard",
		"space_marines": "space_marines",
		"chaos":         "chaos",
		"tau":           "tau",
		"":              "",
		"unknown_race":  "",
	} {
		if got := raceSkin(folder); got != want {
			t.Errorf("raceSkin(%q) = %q, want %q", folder, got, want)
		}
	}

	const (
		path        = `art/ui/textures/taskbar/taskbar_share/taskbar.tga`
		shareFirst  = `Data\art\ui\textures\taskbar\taskbar_share\taskbar.rtx`
		necronFirst = `Data\art\ui\textures\taskbar\necron\taskbar.rtx`
		shareRTX    = `Data\art\ui\textures\taskbar\taskbar_share\taskbar.rtx`
	)

	// Раса приходит из выбора игроков, а если его не было — из чертежей
	// своих отрядов.
	old := Setup
	defer func() { Setup = old }()
	Setup.Own = 1
	Setup.Races = []racebps.Race{{UnitPath: `Races/Necrons/Troops`}}
	Setup.Resolved = map[int]int{1: 0}
	if skin := raceSkin(raceFolderOf(Setup.Own)); skin != "necron" {
		t.Fatalf("скин из выбора игроков = %q, want necron", skin)
	}
	if folder := raceFolderFromPath(`races/necrons/structures/foo`); folder != "necrons" {
		t.Fatalf("раса из чертежа = %q, want necrons", folder)
	}

	// .rtx идёт первым: ремастер хранит панели в нём, а .screen требует .tga.
	got := screenTextureCandidates(path, "necron")
	if len(got) == 0 || got[0] != necronFirst {
		t.Fatalf("с расой некронов первый кандидат = %q, want %q", firstOf(got), necronFirst)
	}
	if !slices.Contains(got, shareRTX) {
		t.Fatalf("нет fallback на share .rtx: %q", got)
	}

	// Панели, разбитые ремастером на куски, тоже берутся у расы: иначе у
	// некронов остаётся общая красная плита.
	for _, piece := range []string{
		`art/ui/textures/taskbar/taskbar_share/map_separated.tga`,
		`art/ui/textures/taskbar/taskbar_share/taskbar_separated.tga`,
		`art/ui/textures/taskbar/taskbar_share/taskbar_menu_separated_left.tga`,
	} {
		want := strings.Replace(piece, "taskbar_share", "necron", 1)
		want = strings.TrimSuffix(want, ".tga") + ".rtx"
		want = `Data\` + strings.ReplaceAll(want, "/", `\`)
		if split := screenTextureCandidates(piece, "necron"); len(split) == 0 || split[0] != want {
			t.Fatalf("%s: первый кандидат = %q, want %q", piece, firstOf(split), want)
		}
	}

	got = screenTextureCandidates(path, "")
	if len(got) == 0 || got[0] != shareFirst {
		t.Fatalf("без расы первый кандидат = %q, want %q", firstOf(got), shareFirst)
	}

	// Вне панелей taskbar путь не меняется.
	const other = `art/ui/textures/art_black_coverup.tga`
	if a, b := screenTextureCandidates(other, "necron"), screenTexturePaths(other); !slices.Equal(a, b) {
		t.Fatalf("чужой путь перебирается: %q != %q", a, b)
	}
}

func firstOf(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	return ss[0]
}
