package scenes

import (
	"testing"

	"github.com/Anykey-Nomad/Open-Kaurava/entities"
)

// TestSnapToPoint — «призрак» особой постройки сам встаёт на ближайшую
// подходящую точку, а если подходящих рядом нет — остаётся под курсором.
func TestSnapToPoint(t *testing.T) {
	game, _ := placementFixture(t)
	spawnPoint := game.pointSpawner(t)
	near := spawnPoint(`Data\attrib\ebps\environment\gameplay\slag_deposit.rgd`, "red", 50)
	spawnPoint(`Data\attrib\ebps\environment\gameplay\slag_deposit.rgd`, "red", 200)
	own := spawnPoint(`Data\attrib\ebps\environment\gameplay\strategic_point_flag.rgd`, "blue", 300)

	at, id := game.snapToPoint([2]float64{62, 0}, testThermo)
	if id != near.ID {
		t.Fatalf("притянуло к %q, ждали слаг-депозит %q", id, near.ID)
	}
	if at != [2]float64{50, 0} {
		t.Fatalf("«призрак» встал на %v, ждали центр депозита {50 0}", at)
	}
	if _, id := game.snapToPoint([2]float64{310, 0}, testPost); id != own.ID {
		t.Fatalf("пост должен притянуться к своей позиции, а не к %q", id)
	}
	// Курсор у депозита, но строится пост: чужая по типу точка не годится.
	if _, id := game.snapToPoint([2]float64{62, 0}, testPost); id != "" {
		t.Fatalf("посту чужая точка не подходит: %q", id)
	}
	// Подходящих точек в пределах притягивания нет — курсор не двигается.
	if at, id := game.snapToPoint([2]float64{400, 0}, testThermo); id != "" || at != [2]float64{400, 0} {
		t.Fatalf("без точки «призрак» должен остаться под курсором: %v %q", at, id)
	}
	// Занятый депозит не притягивает: второй термогенератор туда не встанет.
	game.spawnEntity(t, entities.Spec{
		Name: "Термогенератор", Kind: entities.KindBuilding, ProducedBy: "blue", Owner: "blue",
		BuiltOn: near.ID, HitPoints: 100,
	}, testThermo, 50)
	if _, id := game.snapToPoint([2]float64{62, 0}, testThermo); id != "" {
		t.Fatalf("занятый депозит притягивать не должен: %q", id)
	}
}
