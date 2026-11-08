package scenes

import (
	"math"
	"testing"

	"github.com/Anykey-Nomad/Open-Kaurava/entities"
)

// TestPlacementFreeRejectsOccupiedGround — место под постройку отклоняется,
// если основание пересекается с уже стоящим на карте объектом.
func TestPlacementFreeRejectsOccupiedGround(t *testing.T) {
	game := NewGameplay(nil, nil)
	game.hud.factory = entities.NewFactory()
	if err := game.hud.factory.RegisterPlayer(entities.Player{
		ID: "blue", Name: "Союзник",
		AllowedProduction: []entities.Kind{entities.KindBuilding},
	}); err != nil {
		t.Fatal(err)
	}
	base, err := game.hud.factory.Spawn(entities.Spec{
		Name: "Штаб", Kind: entities.KindBuilding, ProducedBy: "blue", Selectable: true,
		HitPoints: 500,
	}, entities.Position{X: 100, Y: 100})
	if err != nil {
		t.Fatal(err)
	}
	game.objects = append(game.objects, gameplayObject{id: base.ID, radius: 10})

	if blocker, free := game.placementFree([2]float64{105, 100}, [2]float64{8, 8}); free {
		t.Fatal("место поверх штаба должно быть занято")
	} else if blocker != "Штаб" {
		t.Fatalf("помешал %q, ждали штаб", blocker)
	}
	if blocker, free := game.placementFree([2]float64{200, 200}, [2]float64{8, 8}); !free {
		t.Fatalf("свободное место должно приниматься, помешал %q", blocker)
	}
	// Основание с мусорным размером (модель без DATASEUI) не должно
	// превращать всю карту в «занятую».
	if _, free := game.placementFree([2]float64{400, 400}, [2]float64{math.NaN(), math.NaN()}); !free {
		t.Fatal("нечисловой размер основания не должен блокировать постановку")
	}
}

// TestConstructionKeepsHealthUnderMaximumUntilFinished — стройплощадка
// появляется почти без здоровья, и вместе с прогрессом оно доходит до
// максимума чертежа: на 100% работы запас уже полный.
func TestConstructionKeepsHealthUnderMaximumUntilFinished(t *testing.T) {
	factory := entities.NewFactory()
	if err := factory.RegisterPlayer(entities.Player{
		ID: "blue", Name: "Союзник",
		AllowedProduction: []entities.Kind{entities.KindUnit, entities.KindBuilding},
	}); err != nil {
		t.Fatal(err)
	}
	builder, err := factory.Spawn(entities.Spec{Name: "Строитель", Kind: entities.KindUnit, ProducedBy: "blue"}, entities.Position{})
	if err != nil {
		t.Fatal(err)
	}
	site, err := factory.Spawn(entities.Spec{
		Name: "Площадка", Kind: entities.KindBuilding, ProducedBy: "blue",
		HitPoints: 100, ConstructionWork: 10,
	}, entities.Position{X: 4, Y: 4})
	if err != nil {
		t.Fatal(err)
	}
	if site.Health.Current > 100*entities.ConstructionStartHealth+0.001 {
		t.Fatalf("стройплощадка появилась с %g hp — это не «почти без здоровья»", site.Health.Current)
	}
	if err := factory.AddBuilder(site.ID, builder.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := factory.Contribute(site.ID, builder.ID, 5); err != nil {
		t.Fatal(err)
	}
	half, _ := factory.Get(site.ID)
	if half.Health.Current <= site.Health.Current || half.Health.Current >= 100 {
		t.Fatalf("на середине работы здоровье = %g, want между стартом и максимумом", half.Health.Current)
	}
	if _, err := factory.Contribute(site.ID, builder.ID, 5); err != nil {
		t.Fatal(err)
	}
	done, _ := factory.Get(site.ID)
	if done.UnderConstruction() || done.Health.Current != 100 {
		t.Fatalf("по завершении ждали полное здоровье и конец работы: %+v", done)
	}
}
