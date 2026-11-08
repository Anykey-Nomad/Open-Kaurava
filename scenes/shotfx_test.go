package scenes

import (
	"testing"

	"github.com/Anykey-Nomad/Open-Kaurava/fx"
)

// delayedEffect — распылитель с одним листом: так проверяется, что эффект,
// привязанный ко времени клипа (смерть, удар), ждёт своего момента.
const delayedEffect = `
fxtypes = {
  [1] = {
    effect = "root",
    style = "STYLE_SPRAY",
    properties = {
      property_01 = { name = "Emitter_Rate", type = "VARTYPE_ARRAY_TIMEFLOAT", value = { 0, 20, 1, 20 } },
      property_02 = { name = "Emitter_Duration", type = "VARTYPE_FLOAT", value = 1 },
      property_03 = { name = "Emitter_Loop", type = "VARTYPE_BOOL", value = 1 },
      property_04 = { name = "Particle_Speed", type = "VARTYPE_ARRAY_TIMEFLOAT", value = { 0, 4, 1, 2 } },
      property_05 = { name = "Emitter_Deviation", type = "VARTYPE_FLOAT", value = 10 },
    },
  },
  [2] = {
    effect = "root>puff",
    style = "STYLE_RING",
    properties = {
      property_01 = { name = "Texture", type = "VARTYPE_STRING", value = "art/fx/puff.tga" },
      property_02 = { name = "Duration", type = "VARTYPE_FLOAT", value = 2 },
      property_03 = { name = "Radius", type = "VARTYPE_ARRAY_TIMEFLOAT", value = { 0, 1, 1, 3 } },
      property_04 = { name = "Colour", type = "VARTYPE_ARRAY_TIMECOLOUR", value = { entry_00 = { 0, 255, 255, 255, 255 } } },
    },
  },
}
`

// TestShotFXDelayHoldsSimulation — до своего времени эффект стоит, после —
// играет.
func TestShotFXDelayHoldsSimulation(t *testing.T) {
	file, err := fx.ParseFile([]byte(delayedEffect))
	if err != nil {
		t.Fatalf("разбор эффекта: %v", err)
	}
	p := &Gameplay{shotFX: []activeShotFX{{sim: fx.NewSimulation(file), life: 10, delay: 0.5}}}
	p.stepShotFX(0.2)
	if len(p.shotFX) != 1 {
		t.Fatalf("эффектов %d, ожидался 1", len(p.shotFX))
	}
	if got := len(p.shotFX[0].sim.Particles); got != 0 {
		t.Fatalf("до задержки частиц %d, ожидалось 0", got)
	}
	if got := p.shotFX[0].delay; got < 0.29 || got > 0.31 {
		t.Fatalf("остаток задержки %g, ожидалось около 0.3", got)
	}
	p.stepShotFX(0.4)
	if p.shotFX[0].delay != 0 {
		t.Fatalf("задержка %g, ожидался ноль", p.shotFX[0].delay)
	}
	if len(p.shotFX[0].sim.Particles) == 0 {
		t.Fatal("после задержки эффект не заиграл")
	}
}

// TestShotFXExpiresWithDelay — общий срок жизни считается вместе с
// задержкой: эффект не остаётся в очереди после того, как отыграл.
func TestShotFXExpiresWithDelay(t *testing.T) {
	file, err := fx.ParseFile([]byte(delayedEffect))
	if err != nil {
		t.Fatalf("разбор эффекта: %v", err)
	}
	p := &Gameplay{shotFX: []activeShotFX{{sim: fx.NewSimulation(file), life: 0.5, delay: 0.4}}}
	p.stepShotFX(0.6)
	if len(p.shotFX) != 0 {
		t.Fatalf("эффектов %d, ожидалось 0", len(p.shotFX))
	}
}
