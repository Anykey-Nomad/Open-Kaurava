package entities

import (
	"errors"
	"testing"
)

func TestSpawnAssignsProducerOrExplicitOwner(t *testing.T) {
	factory := newTestFactory(t)
	produced, err := factory.Spawn(Spec{Name: "soldier", Kind: KindUnit, ProducedBy: "blue", Selectable: true}, Position{X: 3, Y: 5})
	if err != nil {
		t.Fatal(err)
	}
	if produced.Owner != "blue" || produced.ProducedBy != "blue" || produced.Position != (Position{X: 3, Y: 5}) {
		t.Fatalf("producer ownership/placement not initialized: %+v", produced)
	}

	override, err := factory.Spawn(Spec{Name: "summoned", Kind: KindSummoned, ProducedBy: "blue", Owner: "red"}, Position{})
	if err != nil {
		t.Fatal(err)
	}
	if override.Owner != "red" || override.ProducedBy != "blue" {
		t.Fatalf("explicit owner must override producer: %+v", override)
	}
}

func TestNeutralAndDeferredOwnershipAndCapture(t *testing.T) {
	factory := newTestFactory(t)
	neutral, err := factory.Spawn(Spec{Name: "relic", Kind: KindGameplay, Owner: NeutralPlayer, Capturable: true}, Position{})
	if err != nil {
		t.Fatal(err)
	}
	if neutral.Owner != NeutralPlayer {
		t.Fatalf("neutral owner lost: %+v", neutral)
	}
	if err := factory.Capture(neutral.ID, "red"); err != nil {
		t.Fatal(err)
	}
	captured, _ := factory.Get(neutral.ID)
	if captured.Owner != "red" {
		t.Fatalf("capture did not update current owner: %+v", captured)
	}

	later, err := factory.Spawn(Spec{Name: "objective", Kind: KindGameplay, Capturable: true}, Position{})
	if err != nil {
		t.Fatal(err)
	}
	if later.Owner != "" {
		t.Fatalf("expected unassigned initial owner, got %q", later.Owner)
	}
	if err := factory.AssignOwner(later.ID, "blue"); err != nil {
		t.Fatal(err)
	}
	if err := factory.Capture(later.ID, "green"); err != nil {
		t.Fatal(err)
	}
	current, _ := factory.Get(later.ID)
	if current.Owner != "green" {
		t.Fatalf("ownership change not tracked: %+v", current)
	}
}

func TestSelectableAndPlace(t *testing.T) {
	factory := newTestFactory(t)
	entity, err := factory.Spawn(Spec{Name: "hidden", Kind: KindUnit}, Position{})
	if err != nil {
		t.Fatal(err)
	}
	if factory.CanSelect(entity.ID) {
		t.Fatal("new non-selectable entity was selectable")
	}
	if _, err := factory.Select(entity.ID); !errors.Is(err, ErrNotSelectable) {
		t.Fatalf("Select error = %v, want ErrNotSelectable", err)
	}
	if err := factory.SetSelectable(entity.ID, true); err != nil {
		t.Fatal(err)
	}
	if !factory.CanSelect(entity.ID) {
		t.Fatal("entity did not become selectable")
	}
	selected, err := factory.Select(entity.ID)
	if err != nil || selected.ID != entity.ID {
		t.Fatalf("Select = %+v, %v", selected, err)
	}
	if err := factory.Place(entity.ID, Position{X: 11, Y: -4}); err != nil {
		t.Fatal(err)
	}
	placed, _ := factory.Get(entity.ID)
	if placed.Position != (Position{X: 11, Y: -4}) {
		t.Fatalf("Place = %+v", placed.Position)
	}
}

func TestSelectAtIncludesEnemyAndChoosesNearestSelectable(t *testing.T) {
	factory := newTestFactory(t)
	friendly, err := factory.Spawn(Spec{Name: "friendly", Kind: KindUnit, ProducedBy: "blue", Selectable: true}, Position{X: 5, Y: 0})
	if err != nil {
		t.Fatal(err)
	}
	enemy, err := factory.Spawn(Spec{Name: "enemy", Kind: KindBuilding, ProducedBy: "red", Selectable: true}, Position{X: 1, Y: 0})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := factory.SelectAt(Position{}, 2); err != nil {
		t.Fatalf("enemy should be selectable: %v", err)
	} else {
		selected, _ := factory.SelectAt(Position{}, 2)
		if selected.ID != enemy.ID {
			t.Fatalf("selected %s, expected nearest %s", selected.ID, enemy.ID)
		}
	}
	if selected, err := factory.SelectAt(Position{X: 5, Y: 0}, 1); err != nil || selected.ID != friendly.ID {
		t.Fatalf("friendly selection = %+v, %v", selected, err)
	}
	if _, err := factory.SelectAt(Position{}, 0.5); !errors.Is(err, ErrNotFound) {
		t.Fatalf("out-of-range selection error = %v", err)
	}
}

func TestBuilderConstructionProgress(t *testing.T) {
	factory := newTestFactory(t)
	builder, err := factory.Spawn(Spec{Name: "builder", Kind: KindUnit, ProducedBy: "blue"}, Position{})
	if err != nil {
		t.Fatal(err)
	}
	building, err := factory.Spawn(Spec{Name: "base", Kind: KindBuilding, ProducedBy: "blue", ConstructionWork: 10}, Position{X: 2, Y: 4})
	if err != nil {
		t.Fatal(err)
	}
	if !building.UnderConstruction() || building.ConstructionRatio() != 0 {
		t.Fatalf("new building construction state = %+v", building.Build)
	}
	if err := factory.AddBuilder(building.ID, builder.ID); err != nil {
		t.Fatal(err)
	}
	progress, err := factory.Contribute(building.ID, builder.ID, 4)
	if err != nil || progress != 4 {
		t.Fatalf("first contribution = %v, %v", progress, err)
	}
	progress, err = factory.Contribute(building.ID, builder.ID, 9)
	if err != nil || progress != 10 {
		t.Fatalf("final contribution = %v, %v", progress, err)
	}
	finished, _ := factory.Get(building.ID)
	if finished.UnderConstruction() || finished.ConstructionRatio() != 1 || len(finished.Build.Builders) != 0 {
		t.Fatalf("construction did not finish cleanly: %+v", finished)
	}
}

func TestCaptureAndBuilderOwnershipValidation(t *testing.T) {
	factory := newTestFactory(t)
	unit, err := factory.Spawn(Spec{Name: "worker", Kind: KindUnit, ProducedBy: "red"}, Position{})
	if err != nil {
		t.Fatal(err)
	}
	building, err := factory.Spawn(Spec{Name: "outpost", Kind: KindBuilding, ProducedBy: "blue", ConstructionWork: 3}, Position{})
	if err != nil {
		t.Fatal(err)
	}
	if err := factory.AddBuilder(building.ID, unit.ID); !errors.Is(err, ErrInvalidBuilder) {
		t.Fatalf("foreign worker accepted: %v", err)
	}
	if err := factory.Capture(building.ID, "red"); !errors.Is(err, ErrNotCapturable) {
		t.Fatalf("non-capturable building capture error = %v", err)
	}
}

func TestPlayerRegistryAndProductionAuthorization(t *testing.T) {
	factory := NewFactory()
	player := Player{ID: "blue", Name: "Blue", AllowedProduction: []Kind{KindUnit, KindUnit, KindBuilding}}
	if err := factory.RegisterPlayer(player); err != nil {
		t.Fatal(err)
	}
	player.AllowedProduction[0] = KindGameplay
	registered, ok := factory.Player("blue")
	if !ok || len(registered.AllowedProduction) != 2 || registered.AllowedProduction[0] != KindUnit || registered.AllowedProduction[1] != KindBuilding {
		t.Fatalf("registry did not retain a normalized defensive copy: %+v", registered)
	}
	if err := factory.RegisterPlayer(Player{ID: "blue", Name: "duplicate"}); !errors.Is(err, ErrPlayerExists) {
		t.Fatalf("duplicate registration error = %v", err)
	}
	if !factory.CanProduce("blue", KindUnit) || factory.CanProduce("blue", KindSummoned) {
		t.Fatal("production rules not enforced by CanProduce")
	}
	if _, err := factory.Spawn(Spec{Name: "soldier", Kind: KindUnit, ProducedBy: "blue"}, Position{}); err != nil {
		t.Fatalf("allowed production rejected: %v", err)
	}
	if _, err := factory.Spawn(Spec{Name: "summon", Kind: KindSummoned, ProducedBy: "blue"}, Position{}); !errors.Is(err, ErrProductionDenied) {
		t.Fatalf("forbidden production error = %v", err)
	}
	if _, err := factory.Spawn(Spec{Name: "orphan", Kind: KindUnit, ProducedBy: "missing"}, Position{}); !errors.Is(err, ErrPlayerNotFound) {
		t.Fatalf("unregistered producer error = %v", err)
	}
	if err := factory.SetProductionRules("blue", []Kind{KindSummoned}); err != nil {
		t.Fatal(err)
	}
	if factory.CanProduce("blue", KindUnit) || !factory.CanProduce("blue", KindSummoned) {
		t.Fatal("updated production rules not applied")
	}
}

func newTestFactory(t *testing.T) *Factory {
	t.Helper()
	factory := NewFactory()
	allKinds := []Kind{KindUnit, KindBuilding, KindSummoned, KindGameplay}
	for _, id := range []PlayerID{"blue", "red", "green"} {
		if err := factory.RegisterPlayer(Player{ID: id, Name: string(id), AllowedProduction: allKinds}); err != nil {
			t.Fatal(err)
		}
	}
	return factory
}
