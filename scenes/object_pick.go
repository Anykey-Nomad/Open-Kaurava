package scenes

import (
	"github.com/Anykey-Nomad/Open-Kaurava/loc"
	"math"
	"strings"

	"dow-mod-studio/sgbview"
	"github.com/Anykey-Nomad/Open-Kaurava/entities"
	"github.com/Anykey-Nomad/Open-Kaurava/gfx3d"
	"github.com/Anykey-Nomad/Open-Kaurava/rgdmeta"
	"github.com/Anykey-Nomad/Open-Kaurava/scene"
)

type gameplayObject struct {
	id    string
	name  string
	owner entities.PlayerID
	kind  entities.Kind
	// selectable — sim_entity_ext/is_collide из чертежа объекта.
	selectable bool
	// hidden — меш объекта скрыт: спящая постройка, чьё место занято
	// пробудённым юнитом, и труп самого пробудённого монолита.
	hidden bool
	// selected — DATASEUI из .whe (индикатор и объём выделения) в мировых
	// координатах; nil, если у модели нет своих данных выделения.
	selected *sgbview.SelectedUI
	center   [3]float64
	radius   float64
	// zOffset — насколько центр объекта выше земли под ним. Центр берётся
	// по габаритам модели (середина по высоте), поэтому при движении по
	// рельефу высоту нельзя брать прямо из земли: без запаса модель уходила
	// бы вниз на половину своей высоты.
	zOffset float64
	parts   []sgbview.Part
	// meshName — имя меша объекта в рендерере (все части одного объекта
	// носят одно имя): по нему меш сдвигается при движении.
	meshName string
	// moveOffset — смещение от первоначального места. Геометрия на CPU
	// остаётся в точке появления, поэтому луч выбора и индикатор
	// переносятся в систему координат объекта этим смещением.
	moveOffset [3]float64
	// deathAge — сколько секунд тело лежит после гибели: по нему труп без
	// шанса подняться убирается, а постройка — после клипа смерти.
	deathAge float64
	// keepCorpse — тело было мёртвым с самого начала (лорд некронов,
	// томб-спайдер): оно лежит на карте, пока найм не поднимет его, и не
	// убирается по таймеру.
	keepCorpse bool
}

func (p *Gameplay) buildGameplayObjects(sc *sgbview.Scene, collide map[string]bool) []gameplayObject {
	p.hud.ownPlayer = entities.PlayerID(scenarioPlayerID(Setup.Own))
	p.hud.factory = entities.NewFactory()
	if sc == nil {
		return nil
	}
	for _, slot := range Setup.SortedSlots() {
		id := scenarioPlayerID(slot.Owner)
		player := entities.Player{
			ID: entities.PlayerID(id), Name: Setup.RaceLabel(slot.Owner),
			AllowedProduction: []entities.Kind{entities.KindUnit, entities.KindBuilding, entities.KindSummoned, entities.KindGameplay},
		}
		// Экономика берётся из чертежа расы: стартовые ресурсы, потолок
		// прироста и с чего начинаются лимиты отрядов/техники.
		if caps, ok := p.hud.attribData.raceFolder(raceFolderOf(slot.Owner)); ok {
			player.Resources = entities.Resources{
				Requisition: caps.Starting.Requisition,
				Power:       caps.Starting.Power,
				Faith:       caps.Starting.Faith,
			}
			player.IncomeCap = entities.Income{
				Requisition: caps.IncomeCap.Requisition,
				Power:       caps.IncomeCap.Power,
				Faith:       caps.IncomeCap.Faith,
			}
			player.SquadCapMax, player.SupportCapMax = caps.BaseSquadCap, caps.BaseSupportCap
		}
		if err := p.hud.factory.RegisterPlayer(player); err != nil {
			continue
		}
	}
	if _, ok := p.hud.factory.Player(entities.PlayerID(scenarioPlayerID(Setup.Own))); !ok {
		_ = p.hud.factory.RegisterPlayer(entities.Player{
			ID: entities.PlayerID(scenarioPlayerID(Setup.Own)), Name: loc.T("Player"),
			AllowedProduction: []entities.Kind{entities.KindUnit, entities.KindBuilding, entities.KindSummoned, entities.KindGameplay},
		})
	}
	// Панель ресурсов показывает валюты расы игрока: какие группы видны
	// (у некронов нет очков влияния, зато есть «тайм бонус»).
	p.hud.raceCaps = nil
	if caps, ok := p.hud.attribData.raceFolder(raceFolderOf(Setup.Own)); ok {
		p.hud.raceCaps = &caps
	}

	objects := make([]gameplayObject, 0, len(sc.Objects))
	for _, object := range sc.Objects {
		if isStartingPositionName(object.Name) {
			continue
		}
		center, radius := objectBounds(object)
		if radius <= 0 {
			continue
		}
		owner := entities.NeutralPlayer
		switch {
		case object.StartingSlot > 0:
			// Постройка или стартовый боец: слот известен уже по
			// расстановке, гадать по близости не нужно.
			owner = entities.PlayerID(scenarioPlayerID(object.StartingSlot))
		default:
			if slot := startingBuildOwnerAt(object.Name, center); slot > 0 {
				owner = entities.PlayerID(scenarioPlayerID(slot))
			}
		}
		kind := entities.KindUnit
		if isBuildingName(object.Name) {
			kind = entities.KindBuilding
		}
		// Выделение объекта определяется его чертежом: sim_entity_ext/
		// is_collide. Если поле прочитать не удалось, объект остаётся
		// выбираемым, как и раньше. «Трупы» на старте (лорд некронов,
		// томб-спайдер) не выделяются никогда — чертёж говорит, что тело
		// у них непостоянное (keep_persistent_body = false).
		selectable := true
		if object.Dead {
			selectable = false
		} else if value, ok := collide[rgdmeta.Key(object.Name)]; ok {
			selectable = value
		}
		// Точки захвата нельзя уничтожить — только захватывать; сами они
		// выбираются любым игроком (см. правила захвата в entities.Factory).
		capturePoint := isCapturePointName(object.Name)
		name := displayObjectName(object.Name)
		spec := entities.Spec{Name: name, Kind: kind, Owner: owner, Selectable: selectable, Capturable: capturePoint, Indestructible: capturePoint, Dead: object.Dead}
		if owner != entities.NeutralPlayer {
			spec.ProducedBy = owner
		}
		// Ресурсы, которые даёт объект, и лимиты, которые он занимает или
		// добавляет, читаются из его чертежа (resource_ext, squad_cap_ext,
		// модификаторы постройки).
		if unit, ok := p.hud.attribData.unitFor(object.Name); ok {
			spec.Income = entities.Income{
				Requisition: unit.Income.Requisition,
				Power:       unit.Income.Power,
				Faith:       unit.Income.Faith,
			}
			spec.CapBonus = entities.CapBonus{Squad: unit.CapBonus.Squad, Support: unit.CapBonus.Support}
			// Скорость — moving_ext/speed_max: только с ней фабрика
			// принимает приказы движения. Здоровье уезжает в State.Health,
			// чтобы статус бойца был полным с самого спавна.
			spec.Speed = float64(unit.Speed)
			spec.AccelTime = float64(unit.AccelTime)
			spec.BrakeTime = float64(unit.BrakeTime)
			spec.HitPoints = float64(unit.HitPoints)
			// Бой: оружие из хардпоинтов, класс брони и радиус объекта —
			// по радиусу атакующие встают к краю цели, а не в её центр.
			spec.Weapon = weaponFor(unit)
			spec.ArmourClass = unit.ArmourClass
			spec.Armour = unit.Armour
			spec.Radius = radius
			// Восстановление после уничтожения: у некронов в health_ext
			// есть шанс подняться с долей здоровья.
			spec.GetUpChance = unit.GetUpChance
			spec.GetUpHealthPercent = unit.GetUpHealthPercent
			spec.GetUpTime = unit.GetUpTime
			// Тело, которое остаётся на карте после гибели, собирается
			// томб-спайдером или поднимается воскрешением лорда.
			spec.UsableBody = unit.SpawnUsableBodyOnDeath != ""
		}
		// Бокс основания — по геометрии модели, даже если чертёж не дал
		// оружия: по нему стрелки меряют, докуда подходить к постройке.
		spec.Half = objectHalfExtents(object)
		if squad, ok := p.hud.attribData.squadFor(object.Name); ok {
			spec.CapUsage = entities.CapUsage{Squad: squad.SquadCap, Support: squad.SupportCap}
			spec.MoraleMax = squad.MoraleMax
		}
		spawned, err := p.hud.factory.Spawn(spec, entities.Position{X: center[0], Y: center[1]})
		if err != nil {
			continue
		}
		meshName := ""
		for _, part := range object.Parts {
			if part.Mesh != nil && part.Mesh.Name != "" {
				meshName = part.Mesh.Name
				break
			}
		}
		objects = append(objects, gameplayObject{
			id: spawned.ID, name: name, owner: owner, kind: kind, selectable: selectable,
			selected: object.Selected, center: center, radius: radius,
			zOffset: center[2] - p.groundHeight(center[0], center[1]),
			parts:   object.Parts, meshName: meshName,
			keepCorpse: object.Dead,
		})
		if p.hud.blueprint == nil {
			p.hud.blueprint = map[string]string{}
		}
		p.hud.blueprint[spawned.ID] = object.Name
	}
	return objects
}

func startingBuildOwnerAt(name string, center [3]float64) int {
	name = normalizeObjectName(name)
	bestSlot := 0
	bestDistance := math.Inf(1)
	for _, build := range Setup.Builds {
		if normalizeObjectName(build.Blueprint) != name {
			continue
		}
		for _, slot := range Setup.Slots {
			if slot.Owner != build.Slot {
				continue
			}
			for _, position := range slot.Positions {
				dx := center[0] - float64(position.X)

				dy := center[1] - float64(position.Z)
				distance := dx*dx + dy*dy
				if distance < bestDistance {
					bestSlot, bestDistance = slot.Owner, distance
				}
			}
		}
	}
	return bestSlot
}

func isStartingPositionName(name string) bool {
	return strings.Contains(normalizeObjectName(name), "starting_position")
}

func sceneWithoutStartingMarkers(sc *sgbview.Scene) *sgbview.Scene {
	if sc == nil {
		return nil
	}
	visible := *sc
	visible.Objects = make([]sgbview.Object, 0, len(sc.Objects))
	for _, object := range sc.Objects {
		if !isStartingPositionName(object.Name) {
			visible.Objects = append(visible.Objects, object)
		}
	}
	return &visible
}

func scenarioPlayerID(slot int) string { return "player-" + intString(slot) }

func normalizeObjectName(name string) string {
	name = strings.ToLower(strings.ReplaceAll(name, "\\", "/"))
	name = strings.TrimPrefix(name, "data:")
	name = strings.TrimPrefix(name, "data/")
	return strings.TrimLeft(name, "/")
}

func displayObjectName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	if slash := strings.LastIndex(name, "/"); slash >= 0 {
		name = name[slash+1:]
	}
	name = strings.TrimSuffix(name, ".rgd")
	if name == "" {
		return loc.T("Object")
	}
	return name
}

func isBuildingName(name string) bool {
	name = strings.ToLower(name)
	return strings.Contains(name, "/structures/") || strings.Contains(name, "structure") || strings.Contains(name, "building") || strings.Contains(name, "starting_build")
}

// isCapturePointName — точка захвата по имени чертежа: стратегические
// позиции и объекты, реликтовые точки, слаг-депозиты. Их нельзя уничтожить,
// они только переходят от игрока к игроку.
func isCapturePointName(name string) bool {
	name = normalizeObjectName(name)
	return strings.Contains(name, "strategic_point") ||
		strings.Contains(name, "strategic_objective") ||
		strings.Contains(name, "relic_struct") ||
		strings.Contains(name, "slag_deposit")
}

func intString(value int) string {
	if value < 0 {
		return "0"
	}
	if value == 0 {
		return "0"
	}
	const digits = "0123456789"
	var buf [20]byte
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = digits[value%10]
		value /= 10
	}
	return string(buf[i:])
}

// objectHalfExtents — полуширина и полуглубина модели в плоскости земли.
// По ним стрелок встаёт на дальность оружия от стены постройки, а не от её
// геометрического центра.
func objectHalfExtents(object sgbview.Object) [2]float64 {
	var min, max [2]float64
	first := true
	for _, part := range object.Parts {
		if part.Mesh == nil {
			continue
		}
		for _, vertex := range part.Mesh.Vertices {
			point := [2]float64{float64(vertex.Pos[0]), float64(vertex.Pos[1])}
			if first {
				min, max = point, point
				first = false
				continue
			}
			min[0] = math.Min(min[0], point[0])
			min[1] = math.Min(min[1], point[1])
			max[0] = math.Max(max[0], point[0])
			max[1] = math.Max(max[1], point[1])
		}
	}
	if first {
		return [2]float64{}
	}
	return [2]float64{(max[0] - min[0]) * 0.5, (max[1] - min[1]) * 0.5}
}

func objectBounds(object sgbview.Object) (center [3]float64, radius float64) {
	var min, max [3]float64
	first := true
	for _, part := range object.Parts {
		if part.Mesh == nil {
			continue
		}
		for _, vertex := range part.Mesh.Vertices {
			point := [3]float64{float64(vertex.Pos[0]), float64(vertex.Pos[1]), float64(vertex.Pos[2])}
			if first {
				min, max = point, point
				first = false
				continue
			}
			for axis := range point {
				min[axis] = math.Min(min[axis], point[axis])
				max[axis] = math.Max(max[axis], point[axis])
			}
		}
	}
	if first {
		return center, 0
	}
	for axis := range center {
		center[axis] = (min[axis] + max[axis]) * 0.5
	}
	dx, dy, dz := max[0]-min[0], max[1]-min[1], max[2]-min[2]
	radius = math.Sqrt(dx*dx+dy*dy+dz*dz) * 0.5
	return center, math.Max(radius, 2)
}

func (p *Gameplay) pickWorldObject(x, y, width, height int) (string, bool) {
	origin, direction, ok := p.cam.RayFromScreen(x, y, width, height)
	if !ok {
		return "", false
	}
	best, bestDistance := "", math.Inf(1)
	for _, object := range p.objects {
		if !object.selectable {
			continue
		}
		// Движущийся объект физически сдвинут только на GPU: геометрия на
		// CPU остаётся в точке появления, поэтому луч переносится в систему
		// координат объекта его смещением.
		testOrigin := origin
		testObject := object
		if object.moveOffset != [3]float64{} {
			testOrigin = [3]float64{
				origin[0] - object.moveOffset[0],
				origin[1] - object.moveOffset[1],
				origin[2] - object.moveOffset[2],
			}
			testObject.center = [3]float64{
				object.center[0] - object.moveOffset[0],
				object.center[1] - object.moveOffset[1],
				object.center[2] - object.moveOffset[2],
			}
		}
		var (
			distance float64
			hit      bool
		)
		if object.selected != nil && object.selected.Volume != nil {
			// Выбор идёт по объёму выделения из DATASEUI — так, как это
			// описывает страница Selected UI (Classic OE) окна Object.
			distance, hit = raySelectionBox(testOrigin, direction, *object.selected.Volume)
		} else {
			distance, hit = rayObjectFallback(testOrigin, direction, testObject)
		}
		if hit && distance < bestDistance {
			best, bestDistance = object.id, distance
		}
	}
	return best, best != ""
}

// rayObjectFallback — прежний путь выбора для объектов без DATASEUI:
// сфера вокруг границ модели, затем точный луч по треугольникам меша.
func rayObjectFallback(origin, direction [3]float64, object gameplayObject) (float64, bool) {
	toCenter := [3]float64{object.center[0] - origin[0], object.center[1] - origin[1], object.center[2] - origin[2]}
	along := toCenter[0]*direction[0] + toCenter[1]*direction[1] + toCenter[2]*direction[2]
	if along < 0 {
		return 0, false
	}
	closest := [3]float64{origin[0] + direction[0]*along, origin[1] + direction[1]*along, origin[2] + direction[2]*along}
	dx, dy, dz := closest[0]-object.center[0], closest[1]-object.center[1], closest[2]-object.center[2]
	if dx*dx+dy*dy+dz*dz > object.radius*object.radius {
		return 0, false
	}
	distance, hit := rayObjectDistance(origin, direction, object)
	if hit {
		return distance, true
	}
	if objectHasTriangles(object) {
		return 0, false
	}
	// Some units have no resolved mesh in the preview. Keep them
	// selectable through the same real 3D ray using their bounds.
	distance = along - object.radius
	if distance < 0 {
		distance = 0
	}
	return distance, true
}

// raySelectionBox режет луч по ориентированному объёму выделения (slab-метод
// в локальных осях объёма).
func raySelectionBox(origin, direction [3]float64, box sgbview.SelectionBox) (float64, bool) {
	dx := origin[0] - box.Center[0]
	dy := origin[1] - box.Center[1]
	dz := origin[2] - box.Center[2]
	ch := math.Cos(box.Yaw)
	sh := math.Sin(box.Yaw)
	local := [3]float64{ch*dx + sh*dy, -sh*dx + ch*dy, dz}
	ldir := [3]float64{ch*direction[0] + sh*direction[1], -sh*direction[0] + ch*direction[1], direction[2]}

	tmin, tmax := 0.0, math.Inf(1)
	for axis := 0; axis < 3; axis++ {
		half := box.Half[axis]
		if math.Abs(ldir[axis]) < 1e-9 {
			if math.Abs(local[axis]) > half {
				return 0, false
			}
			continue
		}
		t1 := (-half - local[axis]) / ldir[axis]
		t2 := (half - local[axis]) / ldir[axis]
		if t1 > t2 {
			t1, t2 = t2, t1
		}
		if t1 > tmin {
			tmin = t1
		}
		if t2 < tmax {
			tmax = t2
		}
		if tmin > tmax {
			return 0, false
		}
	}
	return tmin, true
}

func objectHasTriangles(object gameplayObject) bool {
	for _, part := range object.parts {
		if part.Mesh == nil {
			continue
		}
		for i := 0; i+2 < len(part.Mesh.Indices); i += 3 {
			if int(part.Mesh.Indices[i]) < len(part.Mesh.Vertices) && int(part.Mesh.Indices[i+1]) < len(part.Mesh.Vertices) && int(part.Mesh.Indices[i+2]) < len(part.Mesh.Vertices) {
				return true
			}
		}
	}
	return false
}

func rayObjectDistance(origin, direction [3]float64, object gameplayObject) (float64, bool) {
	best := math.Inf(1)
	for _, part := range object.parts {
		if part.Mesh == nil {
			continue
		}
		for i := 0; i+2 < len(part.Mesh.Indices); i += 3 {
			ia, ib, ic := part.Mesh.Indices[i], part.Mesh.Indices[i+1], part.Mesh.Indices[i+2]
			if int(ia) >= len(part.Mesh.Vertices) || int(ib) >= len(part.Mesh.Vertices) || int(ic) >= len(part.Mesh.Vertices) {
				continue
			}
			a := vec3(part.Mesh.Vertices[ia].Pos)
			b := vec3(part.Mesh.Vertices[ib].Pos)
			c := vec3(part.Mesh.Vertices[ic].Pos)
			if distance, hit := rayTriangle(origin, direction, a, b, c); hit && distance < best {
				best = distance
			}
		}
	}
	return best, !math.IsInf(best, 1)
}

func vec3(v [3]float32) [3]float64 {
	return [3]float64{float64(v[0]), float64(v[1]), float64(v[2])}
}

func rayTriangle(origin, direction, a, b, c [3]float64) (float64, bool) {
	e1 := [3]float64{b[0] - a[0], b[1] - a[1], b[2] - a[2]}
	e2 := [3]float64{c[0] - a[0], c[1] - a[1], c[2] - a[2]}
	p := cross(direction, e2)
	determinant := dot(e1, p)
	if math.Abs(determinant) < 1e-9 {
		return 0, false
	}
	inverse := 1 / determinant
	t := [3]float64{origin[0] - a[0], origin[1] - a[1], origin[2] - a[2]}
	u := dot(t, p) * inverse
	if u < 0 || u > 1 {
		return 0, false
	}
	q := cross(t, e1)
	v := dot(direction, q) * inverse
	if v < 0 || u+v > 1 {
		return 0, false
	}
	distance := dot(e2, q) * inverse
	return distance, distance >= 0
}

func cross(a, b [3]float64) [3]float64 {
	return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

func dot(a, b [3]float64) float64 {
	return a[0]*b[0] + a[1]*b[1] + a[2]*b[2]
}

func drawGameplayObjects(c *scene.Ctx, camera *gfx3d.Camera, objects []gameplayObject, selected map[string]bool) {
	for _, object := range objects {
		// Невыделяемые объекты (трупы лорда/томб-спайдера, объекты с
		// is_collide = false) не получают индикатор ни при каком состоянии.
		if !object.selectable {
			continue
		}
		x, y, visible := projectWorld(camera, object.center, c.W, c.H)
		if !visible || x < -64 || y < -64 || x > float32(c.W)+64 || y > float32(c.H)+64 {
			continue
		}
		if selected[object.id] {
			if object.selected != nil && object.selected.Indicator.Half[0] > 0 && object.selected.Indicator.Half[1] > 0 {
				// Индикатор по данным DATASEUI: настоящие размеры и смещение
				// из .whe, спроецированные из мировых координат.
				drawWorldSelection(c, camera, object.selected, object.moveOffset)
			} else {
				radiusX := max(float32(10), float32(object.radius*0.3))
				radiusY := radiusX * 0.65
				for i := 0; i < 48; i++ {
					angle := 2 * math.Pi * float64(i) / 48
					c.Rect(x+float32(math.Cos(angle))*radiusX-1, y+float32(math.Sin(angle))*radiusY-1, 3, 3, [4]float32{0.12, 0.95, 0.22, 0.95})
				}
			}
		}
	}
}

// drawWorldSelection рисует индикатор выделения по DATASEUI: круг или
// прямоугольник в плоскости объекта, спроецированный из мировых координат.
// offset переносит индикатор за двигающимся объектом: сам DATASEUI остаётся
// в координатах точки появления.
func drawWorldSelection(c *scene.Ctx, camera *gfx3d.Camera, sel *sgbview.SelectedUI, offset [3]float64) {
	q := sel.Indicator
	q.Center = [3]float64{q.Center[0] + offset[0], q.Center[1] + offset[1], q.Center[2] + offset[2]}
	ch := math.Cos(q.Yaw)
	sh := math.Sin(q.Yaw)
	var points [][2]float32
	add := func(lx, ly float64) {
		p := [3]float64{
			ch*lx - sh*ly + q.Center[0],
			sh*lx + ch*ly + q.Center[1],
			q.Center[2],
		}
		if sx, sy, ok := projectWorld(camera, p, c.W, c.H); ok {
			points = append(points, [2]float32{sx, sy})
		}
	}
	if sel.DisplayType == 1 {
		// Прямоугольник: рёбра сэмплируются точками, чтобы спроецированная
		// рамка оставалась ровной при любом ракурсе.
		const perEdge = 8
		hx, hy := q.Half[0], q.Half[1]
		corners := [][2]float64{{-hx, -hy}, {hx, -hy}, {hx, hy}, {-hx, hy}}
		for i := 0; i < 4; i++ {
			a, b := corners[i], corners[(i+1)%4]
			for s := 0; s < perEdge; s++ {
				t := float64(s) / perEdge
				add(a[0]+(b[0]-a[0])*t, a[1]+(b[1]-a[1])*t)
			}
		}
		add(corners[0][0], corners[0][1])
	} else {
		const segments = 48
		for i := 0; i <= segments; i++ {
			angle := 2 * math.Pi * float64(i) / segments
			add(math.Cos(angle)*q.Half[0], math.Sin(angle)*q.Half[1])
		}
	}
	color := [4]float32{0.12, 0.95, 0.22, 0.95}
	for i := 1; i < len(points); i++ {
		drawScreenSegment(c, points[i-1][0], points[i-1][1], points[i][0], points[i][1], color)
	}
}

// drawScreenSegment рисует отрезок в экранных координатах цепочкой точек.
func drawScreenSegment(c *scene.Ctx, x0, y0, x1, y1 float32, col [4]float32) {
	steps := int(math.Max(math.Abs(float64(x1-x0)), math.Abs(float64(y1-y0))) / 3)
	if steps < 1 {
		steps = 1
	}
	for i := 0; i <= steps; i++ {
		t := float32(i) / float32(steps)
		x := x0 + (x1-x0)*t
		y := y0 + (y1-y0)*t
		c.Rect(x-1, y-1, 3, 3, col)
	}
}

func projectWorld(camera *gfx3d.Camera, point [3]float64, width, height int) (float32, float32, bool) {
	if width <= 0 || height <= 0 {
		return 0, 0, false
	}
	matrix := camera.ViewProj(width, height)
	clip := [4]float64{}
	world := [4]float64{point[0], point[1], point[2], 1}
	for row := range clip {
		for col := range world {
			clip[row] += float64(matrix[col*4+row]) * world[col]
		}
	}
	if clip[3] <= 1e-8 {
		return 0, 0, false
	}
	return float32((clip[0]/clip[3] + 1) * 0.5 * float64(width)), float32((clip[1]/clip[3] + 1) * 0.5 * float64(height)), true
}
