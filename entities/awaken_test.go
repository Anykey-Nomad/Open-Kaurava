package entities

import "testing"

// TestKillCommand — Delete уничтожает юнита обычной гибелью: Alive=false,
// приказ снят, а пробуждённый монолит помнит свою постройку.
func TestKillCommand(t *testing.T) {
	f := newCombatFactory(t)
	unit, err := f.Spawn(Spec{
		Name: "restored_monolith", Kind: KindUnit, Owner: "blue", Selectable: true,
		HitPoints: 100, Speed: 5, Radius: 1.5, FromBuilding: "building-1",
	}, Position{X: 0, Y: 0})
	if err != nil {
		t.Fatalf("спавн: %v", err)
	}
	if err := f.Kill(unit.ID); err != nil {
		t.Fatalf("уничтожение: %v", err)
	}
	dead, ok := f.Get(unit.ID)
	if !ok || dead.Alive {
		t.Fatalf("после Delete: alive=%v, найдена=%v", dead.Alive, ok)
	}
	if dead.FromBuilding != "building-1" {
		t.Fatalf("юнит потерял постройку: %q", dead.FromBuilding)
	}
	if err := f.Kill(unit.ID); err == nil {
		t.Fatal("повторное уничтожение мертвеца прошло")
	}
}

// TestDormantBuildingIsNotTargeted — спящая постройка (место занято
// пробуждённым монолитом) не выбирается автоатакой и не принимает огонь, а
// после возвращения снова становится целью.
func TestDormantBuildingIsNotTargeted(t *testing.T) {
	f := newCombatFactory(t)
	if err := f.RegisterPlayer(Player{ID: "red", Name: "Красный"}); err != nil {
		t.Fatalf("регистрация второго игрока: %v", err)
	}
	building, err := f.Spawn(Spec{
		Name: "monolith", Kind: KindBuilding, Owner: "red", HitPoints: 1000, Radius: 3,
	}, Position{X: 5, Y: 0})
	if err != nil {
		t.Fatalf("спавн постройки: %v", err)
	}
	shooter := spawnCombatant(t, f, Position{X: 0, Y: 0}, Spec{
		Name: "warrior", HitPoints: 100, Speed: 5, Radius: 1, Weapon: testWeapon(),
	})
	if err := f.SetDormant(building.ID, true); err != nil {
		t.Fatalf("усыпление: %v", err)
	}
	for i := 0; i < 20; i++ {
		f.Tick(0.1)
	}
	dormant, _ := f.Get(building.ID)
	if dormant.Health.Current != 1000 {
		t.Fatalf("спящую постройку расстреляли: здоровье %g", dormant.Health.Current)
	}
	if got := f.nearestVictim(shooter); got != "" {
		t.Fatalf("автоатака выбрала спящую постройку: %q", got)
	}
	if err := f.SetDormant(building.ID, false); err != nil {
		t.Fatalf("пробуждение: %v", err)
	}
	if got := f.nearestVictim(shooter); got != building.ID {
		t.Fatalf("после возвращения цель %q, ожидалась постройка", got)
	}
	for i := 0; i < 20; i++ {
		f.Tick(0.1)
	}
	awake, _ := f.Get(building.ID)
	if awake.Health.Current >= 1000 {
		t.Fatalf("постройка не получает урон после возвращения: %g", awake.Health.Current)
	}
}

// TestDormantBuildingGivesNoIncome — спящая постройка не приносит дохода,
// пока её место занято юнитом.
func TestDormantBuildingGivesNoIncome(t *testing.T) {
	f := newCombatFactory(t)
	building, err := f.Spawn(Spec{
		Name: "monolith", Kind: KindBuilding, Owner: "blue", HitPoints: 100,
		Income: Income{Requisition: 10},
	}, Position{X: 0, Y: 0})
	if err != nil {
		t.Fatalf("спавн: %v", err)
	}
	if err := f.SetDormant(building.ID, true); err != nil {
		t.Fatalf("усыпление: %v", err)
	}
	f.Tick(1)
	economy, ok := f.Economy("blue")
	if !ok {
		t.Fatal("нет экономики игрока")
	}
	if economy.Resources.Requisition != 0 {
		t.Fatalf("спящая постройка дала %g припасов", economy.Resources.Requisition)
	}
}
