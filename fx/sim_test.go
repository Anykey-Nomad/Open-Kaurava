package fx

import (
	"math"
	"testing"
)

// sprayFile — распылитель с листом-кольцом: так записано большинство
// эффектов игры.
const sprayFile = `
fxtypes = {
  [1] = {
    effect = "root",
    style = "STYLE_SPRAY",
    properties = {
      property_01 = { name = "Emitter_Rate", type = "VARTYPE_ARRAY_TIMEFLOAT", value = { 0, 20, 1, 20 } },
      property_02 = { name = "Emitter_Duration", type = "VARTYPE_FLOAT", value = 1 },
      property_03 = { name = "Emitter_Loop", type = "VARTYPE_BOOL", value = 1 },
      property_04 = { name = "Emitter_Drag", type = "VARTYPE_FLOAT", value = 0.5 },
      property_05 = { name = "Particle_Speed", type = "VARTYPE_ARRAY_TIMEFLOAT", value = { 0, 4, 1, 2 } },
      property_06 = { name = "Emitter_Deviation", type = "VARTYPE_FLOAT", value = 10 },
    },
  },
  [2] = {
    effect = "root>puff",
    style = "STYLE_RING",
    properties = {
      property_01 = { name = "Texture", type = "VARTYPE_STRING", value = "art/fx/puff.tga" },
      property_02 = { name = "Duration", type = "VARTYPE_FLOAT", value = 2 },
      property_03 = { name = "Blending", type = "VARTYPE_FLOAT", value = 2 },
      property_04 = { name = "Radius", type = "VARTYPE_ARRAY_TIMEFLOAT", value = { 0, 1, 1, 3 } },
      property_05 = { name = "Colour", type = "VARTYPE_ARRAY_TIMECOLOUR", value = { entry_00 = { 0, 255, 255, 255, 255 }, entry_01 = { 1, 255, 255, 255, 0 } } },
    },
  },
}
`

// TestSimulationEmits — после шага у распылителя есть живые частицы, а
// спрайты несут радиус и цвет листа.
func TestSimulationEmits(t *testing.T) {
	f, err := ParseFile([]byte(sprayFile))
	if err != nil {
		t.Fatalf("разбор: %v", err)
	}
	sim := NewSimulation(f)
	if len(sim.Emitters) != 1 {
		t.Fatalf("распылителей %d, ожидался 1", len(sim.Emitters))
	}
	if len(sim.Emitters[0].Visuals) != 1 {
		t.Fatalf("листьев %d, ожидался 1", len(sim.Emitters[0].Visuals))
	}
	if got := len(sim.Sprites()); got != 0 {
		t.Fatalf("до шага спрайтов %d", got)
	}
	sim.Step(0.1)
	if len(sim.Particles) == 0 {
		t.Fatal("за 0.1 с не родилось ни одной частицы")
	}
	sprites := sim.Sprites()
	if len(sprites) == 0 {
		t.Fatal("спрайтов нет")
	}
	sp := sprites[0]
	if sp.Texture != "art/fx/puff.tga" || !sp.Additive {
		t.Fatalf("спрайт %+v", sp)
	}
	// В начале жизни радиус равен первой точке кривой.
	if math.Abs(sp.Radius-1) > 0.2 {
		t.Fatalf("радиус %g, ожидался около 1", sp.Radius)
	}
	if sp.A <= 0 {
		t.Fatalf("альфа %g", sp.A)
	}
}

// TestSimulationExpires — частицы старше Duration уходят, и кадр пустеет.
func TestSimulationExpires(t *testing.T) {
	f, err := ParseFile([]byte(sprayFile))
	if err != nil {
		t.Fatalf("разбор: %v", err)
	}
	sim := NewSimulation(f)
	sim.Step(0.1)
	sim.Emitters[0].Rate = 0
	sim.Emitters[0].RateCurve = nil
	for i := 0; i < 40; i++ {
		sim.Step(0.1)
	}
	if len(sim.Particles) != 0 {
		t.Fatalf("осталось частиц: %d", len(sim.Particles))
	}
}

// TestSimulationAnchored — одиночное кольцо файла (без распылителя) — не
// спрайт, а плоскость со спинами: в кадре оно даёт четырёхугольники.
func TestSimulationAnchored(t *testing.T) {
	data := `
fx =
{
  style = "STYLE_RING",
  properties =
  {
    property_01 = { name = "Texture", type = "VARTYPE_STRING", value = "art/fx/ring.tga" },
    property_02 = { name = "Duration", type = "VARTYPE_FLOAT", value = 1 },
    property_03 = { name = "Radius", type = "VARTYPE_ARRAY_TIMEFLOAT", value = { 0, 0.5, 1, 2 } },
    property_04 = { name = "Colour", type = "VARTYPE_ARRAY_TIMECOLOUR", value = { entry_00 = { 0, 0, 0, 0, 255 } } },
  },
}
`
	f, err := ParseFile([]byte(data))
	if err != nil {
		t.Fatalf("разбор: %v", err)
	}
	sim := NewSimulation(f)
	if len(sim.Anchored) != 0 || len(sim.Emitters) != 0 {
		t.Fatalf("висящих %d, распылителей %d", len(sim.Anchored), len(sim.Emitters))
	}
	if len(sim.Ribbons) != 1 || sim.Ribbons[0].Ring == nil {
		t.Fatalf("колец %d", len(sim.Ribbons))
	}
	quads := sim.Quads(Vec3{Z: 10})
	if len(quads) != 1 {
		t.Fatalf("четырёхугольников %d, ожидался 1", len(quads))
	}
	if quads[0].Texture != "art/fx/ring.tga" || !quads[0].Additive {
		t.Fatalf("четырёхугольник %+v", quads[0])
	}
	// В начале жизни радиус — первая точка кривой: половина ширины 0.5.
	width := quads[0].P[0].Sub(quads[0].P[1]).Length()
	if math.Abs(width-1) > 0.05 {
		t.Fatalf("ширина %g, ожидалась около 1 (радиус 0.5)", width)
	}
}

// TestSampleCurveStretchesTime — кривая с осью в секундах читается как
// 0..1, а концы не выходят за свои значения.
func TestSampleCurveStretchesTime(t *testing.T) {
	keys := []Key{{T: 0, V: 10}, {T: 2, V: 30}}
	cases := []struct{ u, want float64 }{
		{-1, 10}, {0, 10}, {0.5, 20}, {1, 30}, {2, 30},
	}
	for _, c := range cases {
		if got := SampleCurve(keys, c.u); math.Abs(got-c.want) > 1e-9 {
			t.Fatalf("u=%g: %g, ожидалось %g", c.u, got, c.want)
		}
	}
}

// TestFrameConvertsToWorld — кадр отдаёт частицы в мире с Z вверх: верх
// файла становится верхом мира.
func TestFrameConvertsToWorld(t *testing.T) {
	sim := &Simulation{Anchored: []Visual{{
		Texture:  "t.tga",
		Lifetime: 1,
		Radius:   []Key{{T: 0, V: 1}},
	}}}
	sim.Anchored[0].Speed = 0
	frame := sim.Frame()
	if len(frame) != 1 {
		t.Fatalf("частиц %d, ожидалась 1", len(frame))
	}
	if frame[0].SizeAt() <= 0 {
		t.Fatalf("размер %g", frame[0].SizeAt())
	}
	if got, want := simToWorld(Vec3{X: 1, Y: 2, Z: 3}), [3]float64{1, -3, 2}; got != want {
		t.Fatalf("перевод осей: %v, ожидалось %v", got, want)
	}
}
