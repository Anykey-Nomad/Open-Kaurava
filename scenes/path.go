package scenes

import (
	"math"

	"dow-mod-studio/sgbview"

	"github.com/Anykey-Nomad/Open-Kaurava/entities"
)

// Путь приказа строится один раз — в момент щелчка правой кнопкой — по
// карте проходимости сценария (DATAIMAP): сетка вдвое реже рельефа, у
// каждой ячейки всего два состояния (проходимо/нет), поэтому обычный A*
// со звёздочкой по 8 соседям проходит карту 128×128 за доли миллисекунды
// и не требует предподсчёта потоков.

// pathResult — итог поиска маршрута.
type pathResult int

const (
	// pathNoData — карты проходимости нет: приказ идёт по прямой,
	// как раньше.
	pathNoData pathResult = iota
	// pathFound — маршрут построен.
	pathFound
	// pathBlocked — прохода нет: приказ выдавать нельзя.
	pathBlocked
)

// pathCell — одна ячейка маршрута (индексы в сетке проходимости).
type pathCell struct {
	x, y int
	f    float64
}

// pathHeap — мин-куча по f для открытого списка A*: маленькая своя
// реализация, чтобы не тянуть container/heap ради одного алгоритма.
type pathHeap []pathCell

func (h pathHeap) Len() int { return len(h) }
func (h pathHeap) less(i, j int) bool {
	return h[i].f < h[j].f
}
func (h pathHeap) swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *pathHeap) push(c pathCell) {
	*h = append(*h, c)
	h.up(len(*h) - 1)
}
func (h *pathHeap) pop() pathCell {
	old := *h
	n := len(old)
	out := old[0]
	old[0] = old[n-1]
	old = old[:n-1]
	*h = old
	if len(*h) > 0 {
		h.down(0)
	}
	return out
}
func (h pathHeap) up(i int) {
	for i > 0 {
		p := (i - 1) / 2
		if !h.less(i, p) {
			break
		}
		h.swap(i, p)
		i = p
	}
}
func (h *pathHeap) down(i int) {
	n := len(*h)
	for {
		l, r := 2*i+1, 2*i+2
		best := i
		if l < n && h.less(l, best) {
			best = l
		}
		if r < n && h.less(r, best) {
			best = r
		}
		if best == i {
			return
		}
		h.swap(i, best)
		i = best
	}
}

// findPath ищет маршрут от точки к точке по карте проходимости. Карта
// отсутствует — pathNoData (двигаемся по прямой); цель в непроходимой
// ячейке — берётся ближайшая проходимая в радиусе; прохода нет —
// pathBlocked. Точки возвращаются в мировых метрах (центры ячеек) уже
// без промежуточных коллинеарных поворотов.
func (p *Gameplay) findPath(fromX, fromY, toX, toY float64) ([]entities.Position, pathResult) {
	pass := p.pass
	if pass == nil || pass.W < 2 || pass.H < 2 || pass.CellX <= 0 {
		return nil, pathNoData
	}
	clamp := func(v, hi int) int {
		if v < 0 {
			return 0
		}
		if v > hi {
			return hi
		}
		return v
	}
	sx, sy := pass.Cell(fromX, fromY)
	sx, sy = clamp(sx, pass.W-1), clamp(sy, pass.H-1)
	tx, ty := pass.Cell(toX, toY)
	tx, ty = clamp(tx, pass.W-1), clamp(ty, pass.H-1)
	if !pass.PassableCell(tx, ty) {
		var ok bool
		// 32 кольца по 2 м = 64 м: цель в широких руинах или за
		// длинной стеной всё равно найдёт стоянку рядом.
		tx, ty, ok = nearestPassable(pass, tx, ty, 32)
		if !ok {
			return nil, pathBlocked
		}
	}
	if sx == tx && sy == ty {
		// Цель в той же ячейке — идти некуда, приказ по прямой.
		return nil, pathNoData
	}

	w, h := pass.W, pass.H
	n := w * h
	const inf = math.MaxFloat64
	gScore := make([]float64, n)
	for i := range gScore {
		gScore[i] = inf
	}
	cameFrom := make([]int32, n)
	for i := range cameFrom {
		cameFrom[i] = -1
	}
	closed := make([]bool, n)
	start := sy*w + sx
	goal := ty*w + tx
	gScore[start] = 0

	// Октолинейная эвристика: честная оценка для движения по 8 соседям.
	heuristic := func(cx, cy int) float64 {
		dx := float64(abs(cx - tx))
		dy := float64(abs(cy - ty))
		return math.Max(dx, dy) + (math.Sqrt2-1)*math.Min(dx, dy)
	}
	var open pathHeap
	open.push(pathCell{x: sx, y: sy, f: heuristic(sx, sy)})

	// 8 соседей: диагональ только при двух открытых ортогональных ячейках
	// (юнит не «срезает углы» через стену).
	type step struct {
		dx, dy int
		cost   float64
	}
	steps := []step{
		{1, 0, 1}, {-1, 0, 1}, {0, 1, 1}, {0, -1, 1},
		{1, 1, math.Sqrt2}, {1, -1, math.Sqrt2},
		{-1, 1, math.Sqrt2}, {-1, -1, math.Sqrt2},
	}
	found := false
	for open.Len() > 0 {
		cur := open.pop()
		idx := cur.y*w + cur.x
		if closed[idx] {
			continue
		}
		closed[idx] = true
		if idx == goal {
			found = true
			break
		}
		for _, s := range steps {
			nx, ny := cur.x+s.dx, cur.y+s.dy
			if nx < 0 || ny < 0 || nx >= w || ny >= h {
				continue
			}
			if !pass.PassableCell(nx, ny) {
				continue
			}
			if s.dx != 0 && s.dy != 0 &&
				(!pass.PassableCell(cur.x+s.dx, cur.y) ||
					!pass.PassableCell(cur.x, cur.y+s.dy)) {
				continue
			}
			ni := ny*w + nx
			if closed[ni] {
				continue
			}
			tentative := gScore[idx] + s.cost
			if tentative >= gScore[ni] {
				continue
			}
			gScore[ni] = tentative
			cameFrom[ni] = int32(idx)
			open.push(pathCell{x: nx, y: ny, f: tentative + heuristic(nx, ny)})
		}
	}
	if !found {
		return nil, pathBlocked
	}

	// Сбор ячеек от цели к началу, затем прямой в обратную сторону.
	// Стартовая ячейка в список не попадает (цикл останавливается на ней),
	// поэтому первый waypoint — соседняя ячейка юнита.
	var cells [][2]int
	for idx := goal; idx != start && idx >= 0; idx = int(cameFrom[idx]) {
		cells = append(cells, [2]int{idx % w, idx / w})
	}
	for i, j := 0, len(cells)-1; i < j; i, j = i+1, j-1 {
		cells[i], cells[j] = cells[j], cells[i]
	}
	return simplifyPath(pass, cells), pathFound
}

// nearestPassable ищет проходимую ячейку кольцами вокруг (cx, cy) в
// радиусе maxRing: цель часто попадает в непроходимую клетку (под
// объектом, у стены), а юниту достаточно подойти рядом.
func nearestPassable(pass *sgbview.PassMap, cx, cy, maxRing int) (int, int, bool) {
	for ring := 1; ring <= maxRing; ring++ {
		bestX, bestY := -1, -1
		bestDist := math.MaxFloat64
		for dy := -ring; dy <= ring; dy++ {
			for dx := -ring; dx <= ring; dx++ {
				if maxAbs(dx, dy) != ring {
					continue
				}
				x, y := cx+dx, cy+dy
				if x < 0 || y < 0 || x >= pass.W || y >= pass.H {
					continue
				}
				if !pass.PassableCell(x, y) {
					continue
				}
				dist := float64(dx*dx + dy*dy)
				if dist < bestDist {
					bestX, bestY, bestDist = x, y, dist
				}
			}
		}
		if bestX >= 0 {
			return bestX, bestY, true
		}
	}
	return 0, 0, false
}

// simplifyPath убирает промежуточные точки, лежащие на одной прямой:
// маршрут из коллинеарных ячеек сживается до двух концов, повороты
// остаются. Точки — центры ячеек в мировых метрах.
func simplifyPath(pass *sgbview.PassMap, cells [][2]int) []entities.Position {
	if len(cells) == 0 {
		return nil
	}
	out := make([]entities.Position, 0, len(cells))
	stepDir := func(a, b [2]int) [2]int { return [2]int{b[0] - a[0], b[1] - a[1]} }
	for i, cell := range cells {
		if i > 0 && i < len(cells)-1 {
			prev := stepDir(cells[i-1], cell)
			next := stepDir(cell, cells[i+1])
			if prev == next {
				continue // та же прямая — точка не нужна
			}
		}
		x, y := pass.CellCenter(cell[0], cell[1])
		out = append(out, entities.Position{X: x, Y: y})
	}
	return out
}

// attachPath подбирает маршрут и кладёт его в приказ: false — прохода
// нет, приказ выдавать нельзя. Если цель оказалась в непроходимой
// ячейке, она подменяется центром ближайшей проходимой — иначе последний
// участок шёл бы прямо по стене.
func (p *Gameplay) attachPath(order *entities.Order, from entities.Entity) bool {
	path, res := p.findPath(from.Position.X, from.Position.Y, order.Target.X, order.Target.Y)
	switch res {
	case pathBlocked:
		return false
	case pathFound:
		order.Path = path
		if p.pass != nil && !p.pass.Passable(order.Target.X, order.Target.Y) && len(path) > 0 {
			order.Target = path[len(path)-1]
		}
	}
	return true
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func maxAbs(a, b int) int {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	if a > b {
		return a
	}
	return b
}
