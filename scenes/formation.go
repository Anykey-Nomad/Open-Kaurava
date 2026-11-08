package scenes

import (
	"math"

	"github.com/Anykey-Nomad/Open-Kaurava/entities"
)

// Строй отряда: юнит юниту — препятствие, поэтому отряд идёт и стоит рядами,
// а не сходится в одну точку. Ряды по три бойца, шаг — в одного юнита;
// передний ряд ставится у цели, остальные позади него.
const (
	// formationSpacing — расстояние между соседями в строю. Оно больше
	// SquadSpacing в entities, иначе расталкивание бойцов спорило бы со
	// строем.
	formationSpacing = 2.4
)

// formationRows раскладывает бойцов отряда по рядам — числа бойцов в ряду
// спереди назад. Правило взято из игры: до трёх бойцов один ряд, 4 — один и
// три, 5 — один, три и один, а дальше ряды по три с остатком в последнем:
// 6 — 3/3, 7 — 3/3/1, 8 — 3/3/2, 9 — 3/3/3, 10 — 3/3/3/1.
func formationRows(count int) []int {
	switch {
	case count <= 0:
		return nil
	case count == 1:
		return []int{1}
	case count == 2:
		return []int{2}
	case count == 3:
		return []int{3}
	case count == 4:
		return []int{1, 3}
	case count == 5:
		return []int{1, 3, 1}
	}
	rows := make([]int, 0, (count+2)/3)
	for count > 3 {
		rows = append(rows, 3)
		count -= 3
	}
	if count > 0 {
		rows = append(rows, count)
	}
	return rows
}

// formationTargets — точки строя на count бойцов: передний ряд у цели,
// следующие позади с шагом formationSpacing, внутри ряда бойцы стоят вокруг
// середины. forward — направление движения (единичный вектор), target — куда
// идёт отряд.
func formationTargets(count int, target [2]float64, forward [2]float64, spacing float64) [][2]float64 {
	rows := formationRows(count)
	if len(rows) == 0 {
		return nil
	}
	if spacing <= 0 {
		spacing = formationSpacing
	}
	if forward[0] == 0 && forward[1] == 0 {
		forward = [2]float64{1, 0}
	}
	right := [2]float64{-forward[1], forward[0]}
	out := make([][2]float64, 0, count)
	for row, length := range rows {
		behind := -float64(row) * spacing
		for i := 0; i < length; i++ {
			lateral := (float64(i) - float64(length-1)/2) * spacing
			out = append(out, [2]float64{
				target[0] + right[0]*lateral + forward[0]*behind,
				target[1] + right[1]*lateral + forward[1]*behind,
			})
		}
	}
	return out
}

// formationForward — направление движения отряда: от середины его бойцов к
// цели приказа. Нулевое — отряд уже на месте, строй разворачивается по +X.
func formationForward(members []entities.Entity, target entities.Position) [2]float64 {
	if len(members) == 0 {
		return [2]float64{1, 0}
	}
	var cx, cy float64
	for _, member := range members {
		cx += member.Position.X
		cy += member.Position.Y
	}
	cx /= float64(len(members))
	cy /= float64(len(members))
	dx, dy := target.X-cx, target.Y-cy
	dist := math.Hypot(dx, dy)
	if dist < 1e-6 {
		return [2]float64{1, 0}
	}
	return [2]float64{dx / dist, dy / dist}
}
