package fx

import (
	"math"
	"testing"
)

// TestParseTreeForm — обычная форма: несколько узлов с путями и свойствами.
func TestParseTreeForm(t *testing.T) {
	data := `
fxtypes = {
  [1] = {
    effect = "root",
    style = "STYLE_COMBO",
    properties = {
      [1] = {
        name = "Texture",
        type = "VARTYPE_STRING",
        value = "art/fx/necron/flayer.tga"
      },
      [2] = {
        name = "Duration",
        type = "VARTYPE_FLOAT",
        value = 1.5
      },
    },
  },
  [2] = {
    effect = "root>spray",
    style = "STYLE_SPRAY",
    properties = {
      [1] = {
        name = "Emitter_Rate",
        type = "VARTYPE_ARRAY_TIMEFLOAT",
        value = { 0, 10, 1, 4 },
      },
    },
  },
}
`
	f, err := ParseFile([]byte(data))
	if err != nil {
		t.Fatalf("разбор: %v", err)
	}
	if len(f.Nodes) != 2 {
		t.Fatalf("узлов %d, ожидалось 2", len(f.Nodes))
	}
	root := f.Root()
	if root == nil || root.Effect != "root" || root.Style != "STYLE_COMBO" {
		t.Fatalf("корень %+v", root)
	}
	if got := root.String("Texture"); got != "art/fx/necron/flayer.tga" {
		t.Fatalf("Texture = %q", got)
	}
	if got := root.Float("Duration", 0); math.Abs(got-1.5) > 1e-9 {
		t.Fatalf("Duration = %g", got)
	}
	children := f.Children("root")
	if len(children) != 1 || children[0].Effect != "root>spray" {
		t.Fatalf("дети корня: %+v", children)
	}
	curve := children[0].Props["Emitter_Rate"].Curve
	if len(curve) != 2 || curve[1].V != 4 {
		t.Fatalf("кривая Emitter_Rate: %+v", curve)
	}
}

// TestParseBareForm — форма из одного узла без effect:.
func TestParseBareForm(t *testing.T) {
	data := `
fx =
{
  style = "STYLE_BEAM",
  properties =
  {
    [1] = {
      name = "Texture",
      type = "VARTYPE_STRING",
      value = "art/fx/beam.tga"
    },
  },
}
`
	f, err := ParseFile([]byte(data))
	if err != nil {
		t.Fatalf("разбор: %v", err)
	}
	if !f.Single || len(f.Nodes) != 1 {
		t.Fatalf("ожидался одиночный узел: single=%v nodes=%d", f.Single, len(f.Nodes))
	}
	if f.Root().Style != "STYLE_BEAM" {
		t.Fatalf("стиль %q", f.Root().Style)
	}
	if got := f.Root().String("Texture"); got != "art/fx/beam.tga" {
		t.Fatalf("Texture = %q", got)
	}
}

// TestParseValueFirstOrder — вторая форма записи: значение стоит до типа.
func TestParseValueFirstOrder(t *testing.T) {
	data := `
fxtypes = {
  [1] = {
    effect = "root",
    properties = {
      property_01 = {
        value = 3,
        type = "VARTYPE_FLOAT",
        name = "Radius"
      },
    },
  },
}
`
	f, err := ParseFile([]byte(data))
	if err != nil {
		t.Fatalf("разбор: %v", err)
	}
	if got := f.Root().Float("Radius", -1); math.Abs(got-3) > 1e-9 {
		t.Fatalf("Radius = %g, ожидалось 3", got)
	}
}

// TestParseColourRamp — цвета 0..255 приводятся к 0..1, альфа читается.
func TestParseColourRamp(t *testing.T) {
	data := `
fxtypes = {
  [1] = {
    effect = "root",
    properties = {
      [1] = {
        name = "Colour",
        type = "VARTYPE_ARRAY_TIMECOLOUR",
        value = { entry_00 = { 0, 0, 0, 0, 0 }, entry_01 = { 1, 255, 128, 0, 255 } },
      },
    },
  },
}
`
	f, err := ParseFile([]byte(data))
	if err != nil {
		t.Fatalf("разбор: %v", err)
	}
	stops := f.Root().Props["Colour"].Colour
	if len(stops) != 2 {
		t.Fatalf("точек %d, ожидалось 2", len(stops))
	}
	last := stops[1]
	if math.Abs(last.R-1) > 1e-9 || math.Abs(last.G-128.0/255) > 1e-9 || math.Abs(last.A-1) > 1e-9 {
		t.Fatalf("точка цвета %+v", last)
	}
}

// TestParseRejectsGarbage — файл без узлов эффекта не считается разобранным.
func TestParseRejectsGarbage(t *testing.T) {
	if _, err := ParseFile([]byte("-- пусто\n")); err == nil {
		t.Fatal("ожидалась ошибка разбора")
	}
}
