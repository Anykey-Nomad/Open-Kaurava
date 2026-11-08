package screenui

import "testing"

func TestParseAndResolveNestedWidget(t *testing.T) {
	screen, err := Parse([]byte(`Screen = {
	AspectRatio = 1.33333,
	version = 4,
	Widgets = { type = "Group", name = "root", position = {0, 0}, size = {1, 1}, Children = {
		{ type = "Group", name = "grpBackground", position = {-0.375, 0}, size = {1, 1}, Children = {
			{ type = "Group", name = "grpTaskbar", position = {0, 0.65}, size = {1.75, 0.35}, Children = {
				{ type = "Group", name = "grpTaskbarRight", position = {0.375, -0.65}, size = {1, 1} },
				{ type = "Group", name = "grpSingleSquadSelection", visible = false, position = {0.2816, 0.815}, size = {0.34389, 0.18} },
			} },
		} },
	} },
}`))
	if err != nil {
		t.Fatal(err)
	}
	if screen.Version != 4 || screen.AspectRatio < 1.3 {
		t.Fatalf("screen metadata = version %d aspect %f", screen.Version, screen.AspectRatio)
	}
	r, ok := screen.Layout("grpTaskbarRight", Rect{W: 1280, H: 720})
	if !ok {
		t.Fatal("nested taskbar group not found")
	}
	// Смещения предков взаимно гасятся, а раскладка авторская на 4:3: на
	// окне 16:9 блок ужимается на 0.75 и прижимается к правому низу.
	if !rectNear(r, Rect{X: 320, Y: 180, W: 960, H: 540}, 1) {
		t.Fatalf("resolved rect = %+v", r)
	}
	hidden := screen.Find("grpSingleSquadSelection")
	if hidden == nil || hidden.IsVisible() {
		t.Fatalf("hidden selection state not retained: %+v", hidden)
	}
}

func TestLayoutUsesFullScreenNormalizedOffsets(t *testing.T) {
	screen, err := Parse([]byte(`Screen = {
	Widgets = { type = "Group", name = "screen", position = {0, 0}, size = {1, 1}, Children = {
		{ type = "Group", name = "background", position = {-0.375, 0}, size = {1, 1}, Children = {
			{ type = "Group", name = "taskbar", position = {0, 0.65}, size = {1.75, 0.35}, Children = {
				{ type = "Group", name = "left", position = {0.375, -0.65}, size = {1, 1} },
			} },
		} },
	} },
}`))
	if err != nil {
		t.Fatal(err)
	}
	left := screen.Find("left")
	if left == nil || left.Parent == nil || left.Parent.Name != "taskbar" {
		t.Fatalf("parent chain lost: %+v", left)
	}
	r, ok := screen.Layout("left", Rect{W: 1280, H: 720})
	if !ok || r.X != 0 || r.Y != 0 || r.W != 1280 || r.H != 720 {
		t.Fatalf("layout should accumulate screen-normalized ancestor offsets: %+v", r)
	}
}

func TestVisibilityInheritedFromParent(t *testing.T) {
	screen, err := Parse([]byte(`Screen = {
	Widgets = { type = "Group", name = "screen", Children = {
		{ type = "Group", name = "hidden-parent", hidden = true, Children = {
			{ type = "Button", name = "child" },
		} },
	} },
}`))
	if err != nil {
		t.Fatal(err)
	}
	if screen.Find("child").IsVisible() {
		t.Fatal("child should inherit parent's hidden state")
	}
}

func TestParseRetainsButtonTextAndUnknownProperties(t *testing.T) {
	screen, err := Parse([]byte(`Screen = {
	Widgets = { type = "Group", name = "screen", Children = {
		{ type = "Button", name = "btnMENU", text = "$9047", buttonType = "ClickOnPress", position = {0.8, 0}, size = {0.1, 0.03} },
	} },
}`))
	if err != nil {
		t.Fatal(err)
	}
	button := screen.Find("btnMENU")
	if button == nil || button.Text != "$9047" {
		t.Fatalf("button text not retained: %+v", button)
	}
	if button.Properties["buttonType"] == nil {
		t.Fatal("unknown buttonType property should remain available")
	}
	r, ok := screen.Layout("btnMENU", Rect{W: 1280, H: 720})
	if !ok || r.X != 1024 || r.Y != 0 || r.W != 128 || r.H != 21.6 {
		t.Fatalf("button rect = %+v", r)
	}
}

func TestParseRejectsMalformedScreen(t *testing.T) {
	if _, err := Parse([]byte(`Screen = { Widgets = {`)); err == nil {
		t.Fatal("expected malformed screen error")
	}
	if _, err := Parse([]byte(`return 1`)); err == nil {
		t.Fatal("expected missing Screen error")
	}
}

// TestToolInfoHiddenOverridesDefinition — экземплярные переопределения из
// ToolInfo.WidgetInfo перекрывают видимость определений: в разнесённой
// раскладке ремастера сплошная полоса taskbar скрыта, а блоки миникарты
// и консоли остаются видимы.
func TestToolInfoHiddenOverridesDefinition(t *testing.T) {
	screen, err := Parse([]byte(`Screen = {
	Widgets = { type = "Group", name = "root", position = {0, 0}, size = {1, 1}, Children = {
		{ type = "ArtLabel", name = "artTaskbar", position = {0, 0.65}, size = {1, 1} },
		{ type = "ArtLabel", name = "artTaskbarRight", position = {0.25, 0.66}, size = {0.77, 0.33} },
	} },
}
ToolInfo = {
	version = 3,
	WidgetInfo = {
		{ name = "artTaskbar", hidden = true },
		{ name = "artTaskbarRight" },
	},
}`))
	if err != nil {
		t.Fatal(err)
	}
	strip := screen.Find("artTaskbar")
	if strip == nil || strip.IsVisible() {
		t.Fatalf("полоса taskbar должна быть скрыта WidgetInfo: %+v", strip)
	}
	console := screen.Find("artTaskbarRight")
	if console == nil || !console.IsVisible() {
		t.Fatalf("блок консоли должен остаться видимым: %+v", console)
	}
}
