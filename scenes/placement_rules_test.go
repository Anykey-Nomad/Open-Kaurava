package scenes

import (
	"strings"
	"testing"

	"github.com/Anykey-Nomad/Open-Kaurava/entities"
)

const (
	testPost   = `Data\attrib\ebps\races\space_marines\structures\space_marine_listening_post.rgd`
	testThermo = `Data\attrib\ebps\races\space_marines\structures\space_marine_thermo_generator.rgd`
	testHQ     = `Data\attrib\ebps\races\space_marines\structures\space_marine_hq.rgd`
)

// TestBuildSpotRules проверяет правила особых мест: пост слуха ставится только
// на свою стратегическую позицию или реликт, термогенератор — только на
// слаг-депозит, и на занятую постройкой того же вида точку не ставится ничего.
func TestBuildSpotRules(t *testing.T) {
	game, _ := placementFixture(t)
	spawnPoint := game.pointSpawner(t)

	point := spawnPoint(`Data\attrib\ebps\environment\gameplay\strategic_point_flag.rgd`, "blue", 10)
	spawnPoint(`Data\attrib\ebps\environment\gameplay\strategic_objective_struct.rgd`, "blue", 30)
	slag := spawnPoint(`Data\attrib\ebps\environment\gameplay\slag_deposit.rgd`, "red", 50)
	spawnPoint(`Data\attrib\ebps\environment\gameplay\relic_struct.rgd`, "red", 70)

	if id, reason, ok := game.buildSpotAt([2]float64{10, 0}, testPost); !ok || id != point.ID {
		t.Fatalf("пост на своей позиции: ok=%v точка=%q причина=%q", ok, id, reason)
	}
	if _, reason, ok := game.buildSpotAt([2]float64{30, 0}, testPost); ok || !strings.Contains(reason, "objective") {
		t.Fatalf("пост на strategic objective должен быть отклонён: ok=%v причина=%q", ok, reason)
	}
	if _, reason, ok := game.buildSpotAt([2]float64{70, 0}, testPost); ok || !strings.Contains(reason, "not captured") {
		t.Fatalf("пост на чужом реликте должен быть отклонён: ok=%v причина=%q", ok, reason)
	}
	if _, reason, ok := game.buildSpotAt([2]float64{0, 0}, testPost); ok || !strings.Contains(reason, "strategic point") {
		t.Fatalf("пост на пустой земле должен быть отклонён: ok=%v причина=%q", ok, reason)
	}
	if id, reason, ok := game.buildSpotAt([2]float64{50, 0}, testThermo); !ok || id != slag.ID {
		t.Fatalf("термогенератор на слаг-депозите: ok=%v точка=%q причина=%q", ok, id, reason)
	}
	if _, reason, ok := game.buildSpotAt([2]float64{10, 0}, testThermo); ok || !strings.Contains(reason, "slag") {
		t.Fatalf("термогенератор на позиции должен быть отклонён: ok=%v причина=%q", ok, reason)
	}
	if _, reason, ok := game.buildSpotAt([2]float64{50, 0}, testPost); ok || reason == "" {
		t.Fatalf("пост на слаг-депозите должен быть отклонён: ok=%v причина=%q", ok, reason)
	}
	if _, _, ok := game.buildSpotAt([2]float64{500, 500}, testHQ); !ok {
		t.Fatal("обычная постройка должна ставиться на свободную землю")
	}

	// Постройка того же вида на точке не даёт поставить вторую, а сама точка
	// перестаёт приниматься для приказов захвата.
	built := game.spawnEntity(t, entities.Spec{
		Name: "Термогенератор", Kind: entities.KindBuilding, ProducedBy: "red", Owner: "red",
		BuiltOn: slag.ID, HitPoints: 100,
	}, testThermo, 50)
	if _, reason, ok := game.buildSpotAt([2]float64{50, 0}, testThermo); ok || !strings.Contains(reason, "already has") {
		t.Fatalf("второй термогенератор должен быть отклонён: ok=%v причина=%q", ok, reason)
	}
	if !game.pointBusy(slag.ID) {
		t.Fatal("занятая постройкой точка должна считаться занятой")
	}
	if id, _, found := game.capturableAt(50, 0); found {
		t.Fatalf("застроенная точка не должна приниматься для захвата: %q", id)
	}
	if err := game.hud.factory.Destroy(built.ID); err != nil {
		t.Fatal(err)
	}
	if game.pointBusy(slag.ID) {
		t.Fatal("после уничтожения постройки точка должна освободиться")
	}
	if _, _, ok := game.buildSpotAt([2]float64{50, 0}, testThermo); !ok {
		t.Fatal("на освободившийся слаг-депозит термогенератор должен ставиться")
	}
	// Постройка другого вида на том же месте не мешает термогенератору.
	game.spawnEntity(t, entities.Spec{
		Name: "Штаб", Kind: entities.KindBuilding, ProducedBy: "blue", Owner: "blue",
		BuiltOn: slag.ID, HitPoints: 100,
	}, testHQ, 50)
	if _, reason, ok := game.buildSpotAt([2]float64{50, 0}, testThermo); !ok {
		t.Fatalf("постройка другого вида не должна мешать термогенератору: %q", reason)
	}
}

// TestAttachBuildingIgnoresOccupiedGround — постам слуха и термогенераторам
// безразличны занятость окружения и непроходимость: важен только объект, на
// который они крепятся, а не то, что стоит рядом.
func TestAttachBuildingIgnoresOccupiedGround(t *testing.T) {
	game, _ := placementFixture(t)
	game.pointSpawner(t)(`Data\attrib\ebps\environment\gameplay\slag_deposit.rgd`, "blue", 50)
	// Боец стоит прямо на депозите: обычной постройке здесь места нет.
	game.spawnEntity(t, entities.Spec{
		Name: "Строитель", Kind: entities.KindUnit, ProducedBy: "blue", Owner: "blue", HitPoints: 100,
	}, testHQ, 50)

	if _, blocker, ok := game.placementCheck([2]float64{50, 0}, testHQ, [2]float64{8, 8}); ok {
		t.Fatalf("обычной постройке место на бойце должно быть занято: %q", blocker)
	}
	if _, reason, ok := game.placementCheck([2]float64{50, 0}, testThermo, [2]float64{8, 8}); !ok {
		t.Fatalf("термогенератору занятость окружения безразлична: %q", reason)
	}
	if _, reason, ok := game.placementCheck([2]float64{0, 0}, testThermo, [2]float64{8, 8}); ok {
		t.Fatalf("без слаг-депозита термогенератор не ставится: %q", reason)
	}
}

// placementFixture — игра с двумя игроками и пустым HUD.
func placementFixture(t *testing.T) (*Gameplay, map[string]string) {
	t.Helper()
	game := NewGameplay(nil, nil)
	game.hud.factory = entities.NewFactory()
	for _, id := range []string{"blue", "red"} {
		if err := game.hud.factory.RegisterPlayer(entities.Player{
			ID: entities.PlayerID(id), Name: id,
			AllowedProduction: []entities.Kind{entities.KindUnit, entities.KindBuilding, entities.KindGameplay},
		}); err != nil {
			t.Fatal(err)
		}
	}
	game.hud.ownPlayer = "blue"
	game.hud.blueprint = map[string]string{}
	return game, game.hud.blueprint
}

// pointSpawner — точка захвата на карте: сущность, объект на миникарте и
// чертёж в карте синей печати.
func (p *Gameplay) pointSpawner(t *testing.T) func(blueprint, owner string, x float64) entities.Entity {
	t.Helper()
	return func(blueprint, owner string, x float64) entities.Entity {
		return p.spawnEntity(t, entities.Spec{
			Name: blueprint, Kind: entities.KindGameplay, Owner: entities.PlayerID(owner),
			Capturable: true, Indestructible: true, Selectable: true, HitPoints: 10,
		}, blueprint, x)
	}
}

// spawnEntity — объект сцены с записью чертежа и местом на карте.
func (p *Gameplay) spawnEntity(t *testing.T, spec entities.Spec, blueprint string, x float64) entities.Entity {
	t.Helper()
	entity, err := p.hud.factory.Spawn(spec, entities.Position{X: x})
	if err != nil {
		t.Fatal(err)
	}
	if p.hud.blueprint == nil {
		p.hud.blueprint = map[string]string{}
	}
	p.hud.blueprint[entity.ID] = blueprint
	p.objects = append(p.objects, gameplayObject{
		id: entity.ID, radius: 5, center: [3]float64{x, 0, 0}, selectable: true,
	})
	return entity
}
