package attrib

import (
	"dow-mod-studio/modapi"
)

// StartingUnit — боец, который появляется на карте в начале игры:
// разрешённый из starting_squads конкретный чертёж ebps.
type StartingUnit struct {
	// Ebps — путь чертежа бойца (Data/attrib/ebps/…).
	Ebps string
	// Dead — появляется мёртвым: его нельзя выделить и ими нельзя
	// управлять. Так лорд некронов, томб-спайдеры и лорд-дестройер:
	// у их чертежей health_ext/keep_persistent_body = false.
	Dead bool
	// Offset — смещение точки появления относительно стартовой позиции
	// игрока (x, y, z из starting_squad_offset_NN; x и z — плоскость
	// земли, y — высота).
	Offset [3]float32
	// HasOffset — в чертеже расы было смещение для этого отряда; без
	// него место появления считается по кругу вокруг постройки.
	HasOffset bool
}

// StartingUnits разрешает стартовые отряды расы в конкретных бойцов.
//
// Чертёж отряда (sbps) даёт ebps бойца (squad_loadout_ext/trooper_base) и
// число бойцов по умолчанию (unit_min — у стартовых отрядов это обычно 1).
// Чертёж бойца (ebps) решает, появится ли он мёртвым: поле
// health_ext/keep_persistent_body.
//
// Ошибка чтения молча пропускает отряд: на старте не должно быть ни одного
// юнита — но и падать из-за отсутствующего чертежа нельзя.
func StartingUnits(tree *modapi.ModTree, caps RaceCaps) []StartingUnit {
	if tree == nil || len(caps.StartingSquads) == 0 {
		return nil
	}
	var out []StartingUnit
	for _, start := range caps.StartingSquads {
		squad, ok, err := ReadSquad(tree, start.Path)
		if err != nil || !ok || len(squad.Troopers) == 0 {
			continue
		}
		for i, ebps := range squad.Troopers {
			if ebps == "" {
				continue
			}
			count := 1
			if i == 0 {
				count = squad.UnitMin
				if count < 1 {
					count = 1
				}
			}
			unit, err := ReadUnit(tree, ebps)
			dead := false
			if err == nil && unit.PersistentBodySet {
				dead = !unit.KeepPersistentBody
			}
			for n := 0; n < count; n++ {
				out = append(out, StartingUnit{
					Ebps:      ebps,
					Dead:      dead,
					Offset:    start.Offset,
					HasOffset: start.HasOffset,
				})
			}
		}
	}
	return out
}
