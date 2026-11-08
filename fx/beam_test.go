package fx

import (
	"math"
	"testing"
)

// beamFile — лента без события: Length записан в единицах модели, поэтому
// она тянется от начала координат вдоль оси эффекта.
const beamFile = `
fxtypes = {
  [1] = {
    effect = "root",
    style = "STYLE_BEAM",
    properties = {
      property_01 = { name = "Texture", type = "VARTYPE_STRING", value = "art/fx/beam.tga" },
      property_02 = { name = "Duration", type = "VARTYPE_FLOAT", value = 1 },
      property_03 = { name = "Blending", type = "VARTYPE_FLOAT", value = 2 },
      property_04 = { name = "Length", type = "VARTYPE_ARRAY_TIMEFLOAT", value = { 0, 6, 1, 6 } },
      property_05 = { name = "Width", type = "VARTYPE_ARRAY_TIMEFLOAT", value = { 0, 0.5, 1, 0.5 } },
      property_06 = { name = "Segments", type = "VARTYPE_ARRAY_TIMEFLOAT", value = { 0, 8, 1, 8 } },
      property_07 = { name = "Texture_U_Repeat", type = "VARTYPE_ARRAY_TIMEFLOAT", value = { 0, 1, 1, 1 } },
      property_08 = { name = "Colour", type = "VARTYPE_ARRAY_TIMECOLOUR", value = { entry_00 = { 0, 10, 20, 255, 255 } } },
    },
  },
}
`

// ringFile — кольцо в плоскости маркера: Billboard выключен, поэтому оно не
// разворачивается к камере, а остаётся в своей плоскости.
const ringFile = `
fxtypes = {
  [1] = {
    effect = "root",
    style = "STYLE_RING",
    properties = {
      property_01 = { name = "Texture", type = "VARTYPE_STRING", value = "art/fx/panel.tga" },
      property_02 = { name = "Duration", type = "VARTYPE_FLOAT", value = 1 },
      property_03 = { name = "Blending", type = "VARTYPE_FLOAT", value = 2 },
      property_04 = { name = "Radius", type = "VARTYPE_ARRAY_TIMEFLOAT", value = { 0, 1, 1, 1 } },
      property_05 = { name = "SpinZ", type = "VARTYPE_ARRAY_TIMEFLOAT", value = { 0, 0.25, 1, 0.25 } },
      property_06 = { name = "SpinEpsilonZ", type = "VARTYPE_FLOAT", value = 1 },
      property_07 = { name = "SpinRandom", type = "VARTYPE_BOOL", value = 1 },
      property_08 = { name = "Offset", type = "VARTYPE_ARRAY_TIMEVECTOR3", value = { entry_00 = { 0, 0, 0, 2 } } },
      property_09 = { name = "Colour", type = "VARTYPE_ARRAY_TIMECOLOUR", value = { entry_00 = { 0, 255, 255, 255, 255 } } },
    },
  },
}
`

// TestBeamProducesQuads — лента читается как лента, а кадр даёт по
// четырёхугольнику на каждый Segments.
func TestBeamProducesQuads(t *testing.T) {
	f, err := ParseFile([]byte(beamFile))
	if err != nil {
		t.Fatalf("разбор: %v", err)
	}
	sim := NewSimulation(f)
	if len(sim.Ribbons) != 1 || sim.Ribbons[0].Beam == nil {
		t.Fatalf("лент %d", len(sim.Ribbons))
	}
	if len(sim.Anchored) != 0 {
		t.Fatalf("лента попала в висящие спрайты: %d", len(sim.Anchored))
	}
	quads := sim.Quads(Vec3{Z: 10})
	if len(quads) != 8 {
		t.Fatalf("четырёхугольников %d, ожидалось 8", len(quads))
	}
	if quads[0].Texture != "art/fx/beam.tga" || !quads[0].Additive {
		t.Fatalf("четырёхугольник %+v", quads[0])
	}
	// Length в единицах модели: дальний конец на 6 единиц вдоль оси.
	far := 0.0
	for _, q := range quads {
		for _, p := range q.P {
			far = math.Max(far, p.X)
		}
	}
	if math.Abs(far-6) > 0.1 {
		t.Fatalf("лента дотянулась до %g, ожидалось 6", far)
	}
	// Ширина — половина толщины: между парой вершин сечения 1 единица.
	if width := quads[0].P[0].Sub(quads[0].P[1]).Length(); math.Abs(width-1) > 0.05 {
		t.Fatalf("толщина %g, ожидалась около 1", width)
	}
}

// TestRingKeepsItsPlane — кольцо с выключенным Billboard не разворачивается к
// камере: его плоскость остаётся плоскостью маркера, откуда бы ни смотрели.
func TestRingKeepsItsPlane(t *testing.T) {
	f, err := ParseFile([]byte(ringFile))
	if err != nil {
		t.Fatalf("разбор: %v", err)
	}
	sim := NewSimulation(f)
	if len(sim.Ribbons) != 1 || sim.Ribbons[0].Ring == nil {
		t.Fatalf("колец %d", len(sim.Ribbons))
	}
	for _, eye := range []Vec3{{Z: 10}, {X: 10}, {X: 3, Y: 4, Z: 5}} {
		quads := sim.Quads(eye)
		if len(quads) != 1 {
			t.Fatalf("глаз %v: четырёхугольников %d", eye, len(quads))
		}
		q := quads[0]
		// Кольцо лежит в плоскости маркера (XZ, вертикаль Y): поворот Spin
		// крутит его внутри этой плоскости, но из неё не выводит.
		for _, p := range q.P {
			if math.Abs(p.Y) > 1e-6 {
				t.Fatalf("глаз %v: угол %v ушёл из плоскости маркера", eye, p)
			}
		}
		// Offset (0, 0, 2) — в системе маркера, то есть центр по Z.
		center := q.P[0].Add(q.P[1]).Add(q.P[2]).Add(q.P[3]).MulScalar(0.25)
		if math.Abs(center.Z-2) > 1e-6 || math.Abs(center.X) > 1e-6 {
			t.Fatalf("глаз %v: центр кольца %v, ожидался (0, 0, 2)", eye, center)
		}
		// Нормаль плоскости — вертикаль маркера (Y): кольцо не смотрит на
		// камеру, как делал бы билборд.
		u := q.P[1].Sub(q.P[0])
		v := q.P[3].Sub(q.P[0])
		normal := u.Cross(v).Norm()
		if math.Abs(math.Abs(normal.Y)-1) > 1e-6 {
			t.Fatalf("глаз %v: нормаль %v, ожидалась вдоль Y", eye, normal)
		}
	}
}

// TestRingSpinEpsilonSpread — SpinEpsilon разводит панели по разным углам:
// без разброса все кольца узла встали бы одинаково.
func TestRingSpinEpsilonSpread(t *testing.T) {
	f, err := ParseFile([]byte(ringFile))
	if err != nil {
		t.Fatalf("разбор: %v", err)
	}
	ring := NewSimulation(f).Ribbons[0].Ring
	seen := map[float64]bool{}
	for i := 0; i < 8; i++ {
		spread := ring.Spread(i)
		if spread < 0 || spread > 1 {
			t.Fatalf("панель %d: доля %g", i, spread)
		}
		seen[math.Round(spread*1e6)] = true
	}
	if len(seen) < 8 {
		t.Fatalf("разных долей %d, ожидалось 8", len(seen))
	}
	// Упорядоченный разброс детерминирован: одна и та же панель получает
	// одно и то же лицо на каждом кадре.
	if ring.Spread(3) != ring.Spread(3) {
		t.Fatal("разброс панели меняется между кадрами")
	}
}

// TestBeamArcBowsAcrossSpan — Arc выгибает ленту поперёк пролёта, а не вдоль
// него: составляющая вдоль пролёта только удлиняет ленту.
func TestBeamArcBowsAcrossSpan(t *testing.T) {
	eye := Vec3{X: 2, Y: 1, Z: 3}
	spans := []Vec3{{X: 6}, {Y: -6}, {Z: 6}}
	arcs := []Vec3{{Y: 0.5}, {X: 0.5}, {X: 0.3, Y: -0.2, Z: 0.4}}
	for _, span := range spans {
		for _, arc := range arcs {
			parent := BeamRibbon{From: Vec3{}, To: span, Width: 0.5, Segments: 8, Arc: arc}
			withArc := parent
			withArc.Noise = 0
			flat := withArc
			flat.Arc = Vec3{}
			withVerts := BeamQuads(withArc, eye)
			flatVerts := BeamQuads(flat, eye)
			if len(withVerts) == 0 || len(withVerts) != len(flatVerts) {
				t.Fatalf("пролёт %v: геометрии нет", span)
			}
			dir := span.Norm()
			peak, lateral := 0.0, 0.0
			for i := range withVerts {
				delta := withVerts[i].Pos.Sub(flatVerts[i].Pos)
				if d := delta.Length(); d > peak {
					peak = d
				}
				if l := delta.Sub(dir.MulScalar(delta.Dot(dir))).Length(); l > lateral {
					lateral = l
				}
			}
			if peak < 1e-9 {
				t.Fatalf("пролёт %v, дуга %v: изгиба нет", span, arc)
			}
			if lateral < peak*0.99 {
				t.Fatalf("пролёт %v, дуга %v: поперёк пролёта только %.0f%% изгиба",
					span, arc, lateral/peak*100)
			}
		}
	}
}
