package entities

import (
	"math"
	"testing"
)

// spawnCombatant — боец для тестов боя: владелец уже зарегистрирован.
func spawnCombatant(t *testing.T, f *Factory, position Position, spec Spec) Entity {
	t.Helper()
	spec.Kind = KindUnit
	spec.Owner = "blue"
	spec.Selectable = true
	entity, err := f.Spawn(spec, position)
	if err != nil {
		t.Fatalf("спавн %q: %v", spec.Name, err)
	}
	return entity
}

// newCombatFactory — фабрика с одним игроком, которому можно всё.
func newCombatFactory(t *testing.T) *Factory {
	t.Helper()
	f := NewFactory()
	if err := f.RegisterPlayer(Player{
		ID: "blue", Name: "Синий",
		AllowedProduction: []Kind{KindUnit, KindBuilding},
	}); err != nil {
		t.Fatalf("регистрация игрока: %v", err)
	}
	return f
}

// testWeapon — оружие с известными числами: 10 урона, попадание всегда,
// выстрел раз в полсекунды, 50 % по средней пехоте.
func testWeapon() *Weapon {
	return &Weapon{
		Range: 5, Accuracy: 1, ReloadTime: 0.5,
		MinDamage: 10, MaxDamage: 10,
		Piercing: map[string]float64{"tp_infantry_med": 50},
	}
}

// TestAttackClosesRangeAndKills — боец идёт к цели, открывает огонь только
// в дистанции оружия и доводит цель до нуля здоровья.
func TestAttackClosesRangeAndKills(t *testing.T) {
	f := newCombatFactory(t)
	attacker := spawnCombatant(t, f, Position{X: 0, Y: 0}, Spec{
		Name: "necron_warrior", Speed: 10, HitPoints: 100, Radius: 1,
		Weapon: testWeapon(), Armour: 100, ArmourClass: "tp_infantry_heavy_med",
	})
	target := spawnCombatant(t, f, Position{X: 20, Y: 0}, Spec{
		Name: "marine", Speed: 10, HitPoints: 100, Radius: 1,
		Armour: 100, ArmourClass: "tp_infantry_med",
	})
	if err := f.Move(attacker.ID, Order{
		Target: Position{X: 20, Y: 0}, TargetID: target.ID, Attack: true,
	}); err != nil {
		t.Fatalf("приказ атаки: %v", err)
	}
	// До дистанции боец идёт 20 − (1 + 5) = 14 м при скорости 10 м/с, то
	// есть дольше секунды: за это время здоровье цели не должно упасть.
	for i := 0; i < 4; i++ {
		f.Tick(0.25)
	}
	if current, _ := f.Get(target.ID); current.Health.Current != 100 {
		t.Fatalf("урон нанесён вне дистанции: здоровье %g", current.Health.Current)
	}
	// Дальше цель в дистанции: 5 урона за выстрел, 100 здоровья — 20
	// выстрелов по полсекунды, с запасом.
	for i := 0; i < 60; i++ {
		f.Tick(0.25)
	}
	dead, _ := f.Get(target.ID)
	if dead.Alive {
		t.Fatalf("цель выжила: здоровье %g", dead.Health.Current)
	}
	if dead.Health.Current != 0 {
		t.Fatalf("у убитой цели здоровье %g, ожидалось 0", dead.Health.Current)
	}
	owner, _ := f.Get(attacker.ID)
	if owner.Order == nil || owner.OrderStatus != OrderDone {
		t.Fatalf("приказ атакующего не закрыт: статус %v", owner.OrderStatus)
	}
}

// TestAttackStopsOutOfRange — ушла цель из дистанции, огонь прекращается.
func TestAttackStopsOutOfRange(t *testing.T) {
	f := newCombatFactory(t)
	attacker := spawnCombatant(t, f, Position{X: 0, Y: 0}, Spec{
		Name: "necron_warrior", Speed: 10, HitPoints: 100, Radius: 1,
		Weapon: testWeapon(), Armour: 100, ArmourClass: "tp_infantry_heavy_med",
	})
	target := spawnCombatant(t, f, Position{X: 6, Y: 0}, Spec{
		Name: "marine", Speed: 0, HitPoints: 100, Radius: 1,
		Armour: 100, ArmourClass: "tp_infantry_med",
	})
	if err := f.Move(attacker.ID, Order{
		Target: Position{X: 6, Y: 0}, TargetID: target.ID, Attack: true,
	}); err != nil {
		t.Fatalf("приказ атаки: %v", err)
	}
	for i := 0; i < 8; i++ {
		f.Tick(0.25)
	}
	damaged, _ := f.Get(target.ID)
	if damaged.Health.Current >= 100 {
		t.Fatalf("цель в дистанции не получила урона: %g", damaged.Health.Current)
	}
	// Цель уходит далеко: приказ атаки остаётся, но стрелять нечем.
	far := 400.0
	if err := f.Place(target.ID, Position{X: far, Y: 0}); err != nil {
		t.Fatalf("перенос цели: %v", err)
	}
	before := damaged.Health.Current
	for i := 0; i < 8; i++ {
		f.Tick(0.25)
	}
	after, _ := f.Get(target.ID)
	if after.Health.Current != before {
		t.Fatalf("урон вне дистанции: было %g, стало %g", before, after.Health.Current)
	}
}

// TestAttackNeedsWeapon — без оружия приказ атаки не принимается.
func TestAttackNeedsWeapon(t *testing.T) {
	f := newCombatFactory(t)
	worker := spawnCombatant(t, f, Position{X: 0, Y: 0}, Spec{
		Name: "builder", Speed: 10, HitPoints: 50, Radius: 1,
	})
	target := spawnCombatant(t, f, Position{X: 10, Y: 0}, Spec{
		Name: "marine", Speed: 0, HitPoints: 100, Radius: 1,
	})
	err := f.Move(worker.ID, Order{Target: Position{X: 10, Y: 0}, TargetID: target.ID, Attack: true})
	if err != ErrNoWeapon {
		t.Fatalf("ожидалась ошибка «нет оружия», получено %v", err)
	}
}

// TestWeaponDamageByArmour — урон по таблице пробития: 50 % по средней
// пехоте при броне 100, ноль по классу, которого в таблице нет.
func TestWeaponDamageByArmour(t *testing.T) {
	weapon := testWeapon()
	if got := weapon.Damage(100, "tp_infantry_med"); math.Abs(got-5) > 1e-9 {
		t.Fatalf("урон по средней пехоте %g, ожидалось 5", got)
	}
	if got := weapon.Damage(100, "tp_vehicle_high"); got != 0 {
		t.Fatalf("урон по классу вне таблицы %g, ожидалось 0", got)
	}
	// Броня ниже 100 пропускает больше: 1 − 0.5·(1 − 0.5) = 0.75.
	if got := weapon.Damage(50, "tp_infantry_med"); math.Abs(got-7.5) > 1e-9 {
		t.Fatalf("урон при броне 50 %g, ожидалось 7.5", got)
	}
	// Цель без класса брони (чертёж не дал данных) получает полный урон,
	// а не ноль: иначе бой молча не работал бы.
	if got := weapon.Damage(100, ""); math.Abs(got-10) > 1e-9 {
		t.Fatalf("урон по неизвестной броне %g, ожидалось 10", got)
	}
}

// TestWeaponDamageAgainstHQ — по тяжёлой постройке (HQ: 6000 hp, 10 %
// пробития у флеера) урон идёт, но медленно: это данные чертежа, а не
// отсутствие боя.
func TestWeaponDamageAgainstHQ(t *testing.T) {
	weapon := testWeapon()
	weapon.Piercing = map[string]float64{"tp_building_high": 10}
	if got := weapon.Damage(100, "tp_building_high"); math.Abs(got-1) > 1e-9 {
		t.Fatalf("урон по HQ %g, ожидалось 1 (10 %% от 10)", got)
	}
}

// TestAttackStopsAtRangeDespitePath — маршрут ведёт в центр цели, но боец
// останавливается на дистанции оружия, а не прижимается к объекту.
func TestAttackStopsAtRangeDespitePath(t *testing.T) {
	f := newCombatFactory(t)
	attacker := spawnCombatant(t, f, Position{X: 0, Y: 0}, Spec{
		Name: "necron_warrior", Speed: 10, HitPoints: 100, Radius: 1,
		Weapon: testWeapon(), Armour: 100, ArmourClass: "tp_infantry_heavy_med",
	})
	target := spawnCombatant(t, f, Position{X: 40, Y: 0}, Spec{
		Name: "barracks", Speed: 0, HitPoints: 500, Radius: 8,
		Armour: 100, ArmourClass: "tp_building_med",
	})
	if err := f.Move(attacker.ID, Order{
		Target: Position{X: 40, Y: 0}, TargetID: target.ID, Attack: true,
		// Маршрут «вплотную», как его строит путь до цели.
		Path: []Position{{X: 20, Y: 0}, {X: 39, Y: 0}},
	}); err != nil {
		t.Fatalf("приказ атаки: %v", err)
	}
	for i := 0; i < 80; i++ {
		f.Tick(0.1)
	}
	unit, _ := f.Get(attacker.ID)
	distance := math.Hypot(unit.Position.X-40, unit.Position.Y) - 8
	if distance < 5-0.6 {
		t.Fatalf("боец прижался к цели: дистанция до поверхности %g, ожидалось около 5", distance)
	}
	if distance > 5.6 {
		t.Fatalf("боец не подошёл на дистанцию огня: %g", distance)
	}
	if !unit.Attacking {
		t.Fatal("на дистанции боец не ведёт огонь")
	}
}

// TestAutoAttackEngagesNearestEnemy — без приказа боец сам стреляет по
// врагу в дистанции, но не по своим и не по нейтральным.
func TestAutoAttackEngagesNearestEnemy(t *testing.T) {
	f := newCombatFactory(t)
	if err := f.RegisterPlayer(Player{
		ID: "red", Name: "Красный",
		AllowedProduction: []Kind{KindUnit, KindBuilding},
	}); err != nil {
		t.Fatalf("регистрация игрока: %v", err)
	}
	guard := spawnCombatant(t, f, Position{X: 0, Y: 0}, Spec{
		Name: "necron_warrior", Speed: 10, HitPoints: 100, Radius: 1,
		Weapon: testWeapon(), Armour: 100, ArmourClass: "tp_infantry_heavy_med",
	})
	ally, err := f.Spawn(Spec{
		Name: "ally", Kind: KindUnit, Owner: "blue", Speed: 10, HitPoints: 100, Radius: 1,
	}, Position{X: 2, Y: 0})
	if err != nil {
		t.Fatalf("свой боец: %v", err)
	}
	enemy, err := f.Spawn(Spec{
		Name: "marine", Kind: KindUnit, Owner: "red", Speed: 10, HitPoints: 100, Radius: 1,
		Armour: 100, ArmourClass: "tp_infantry_med",
	}, Position{X: 0, Y: 5})
	if err != nil {
		t.Fatalf("враг: %v", err)
	}
	for i := 0; i < 20; i++ {
		f.Tick(0.25)
	}
	if current, _ := f.Get(enemy.ID); current.Health.Current >= 100 {
		t.Fatalf("автоатака не сработала: здоровье врага %g", current.Health.Current)
	}
	if current, _ := f.Get(ally.ID); current.Health.Current != 100 {
		t.Fatalf("автоатака попала по своему: %g", current.Health.Current)
	}
	if current, _ := f.Get(guard.ID); !current.Attacking {
		t.Fatal("боец не в состоянии стрельбы")
	}
}

// TestAutoAttackCanBeDisabled — выключенная автоатака не стреляет.
func TestAutoAttackCanBeDisabled(t *testing.T) {
	f := newCombatFactory(t)
	if err := f.RegisterPlayer(Player{
		ID: "red", Name: "Красный",
		AllowedProduction: []Kind{KindUnit, KindBuilding},
	}); err != nil {
		t.Fatalf("регистрация игрока: %v", err)
	}
	spawnCombatant(t, f, Position{X: 0, Y: 0}, Spec{
		Name: "necron_warrior", Speed: 10, HitPoints: 100, Radius: 1,
		Weapon: testWeapon(), Armour: 100, ArmourClass: "tp_infantry_heavy_med",
		DisableAutoAttack: true,
	})
	enemy, err := f.Spawn(Spec{
		Name: "marine", Kind: KindUnit, Owner: "red", Speed: 10, HitPoints: 100, Radius: 1,
		Armour: 100, ArmourClass: "tp_infantry_med",
	}, Position{X: 0, Y: 3})
	if err != nil {
		t.Fatalf("враг: %v", err)
	}
	for i := 0; i < 20; i++ {
		f.Tick(0.25)
	}
	if current, _ := f.Get(enemy.ID); current.Health.Current != 100 {
		t.Fatalf("выключенная автоатака стреляла: %g", current.Health.Current)
	}
}
