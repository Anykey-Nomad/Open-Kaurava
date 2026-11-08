package entities

import "math"

// SquadSpacing — минимальная дистанция между бойцами одного отряда: юнит
// юниту — препятствие, поэтому пара, сошедшаяся ближе этого, расходится.
// Меньше, чем шаг строя в сцене, — иначе расталкивание спорило бы со строем.
const SquadSpacing = 2.0

// squadPushSpeed — с какой скоростью бойцы расходятся (м/с): толчок за кадр
// не должен выглядеть рывком.
const squadPushSpeed = 4.0

// stepSeparation расталкивает бойцов одного отряда, оказавшихся ближе
// SquadSpacing: перекрытие делится пополам, и каждый сдвигается в свою
// сторону. Юниты разных отрядов и постройки не трогаются.
//
// Вызывается из Tick после шага движения, с уже взятым мьютексом.
func (f *Factory) stepSeparation(dt float64) {
	if f == nil || !(dt > 0) {
		return
	}
	ids := make([]string, 0, len(f.entities))
	for id, entity := range f.entities {
		if entity.SquadID != "" && entity.Alive {
			ids = append(ids, id)
		}
	}
	if len(ids) < 2 {
		return
	}
	limit := squadPushSpeed * dt
	for i := 0; i < len(ids); i++ {
		a := f.entities[ids[i]]
		for j := i + 1; j < len(ids); j++ {
			b := f.entities[ids[j]]
			if a.SquadID != b.SquadID {
				continue
			}
			dx := b.Position.X - a.Position.X
			dy := b.Position.Y - a.Position.Y
			dist := math.Hypot(dx, dy)
			if dist >= SquadSpacing {
				continue
			}
			if dist < 1e-6 {
				// Точно в одной точке: расходимся по оси +X, чтобы не
				// делить на ноль.
				dx, dy, dist = 1, 0, 1
			}
			push := (SquadSpacing - dist) / 2
			if push > limit {
				push = limit
			}
			ux, uy := dx/dist, dy/dist
			a.Position.X -= ux * push
			a.Position.Y -= uy * push
			b.Position.X += ux * push
			b.Position.Y += uy * push
			f.entities[ids[i]] = a
			f.entities[ids[j]] = b
		}
	}
}
