package scenes

import (
	"github.com/Anykey-Nomad/Open-Kaurava/loc"
	"math"

	"dow-mod-studio/sgbview"

	"github.com/Anykey-Nomad/Open-Kaurava/entities"
	"github.com/Anykey-Nomad/Open-Kaurava/scene"
	"github.com/Anykey-Nomad/Open-Kaurava/window"
)

// Приказы движения отдаются правой кнопкой мыши по карте или по объекту,
// когда юнит выделен. Атака пока не реализована: по вражеским юнитам и
// постройкам приказ не отдаётся.

// dynamicObjectNames — имена мешей объектов, которые могут двигаться
// (юниты, трупы, точки захвата): рендерер копирует их вершины при загрузке
// сцены. Постройки и маркеры стартовых позиций остаются статикой.
func dynamicObjectNames(sc *sgbview.Scene) map[string]bool {
	names := map[string]bool{}
	if sc == nil {
		return names
	}
	for _, object := range sc.Objects {
		if isStartingPositionName(object.Name) || isBuildingName(object.Name) {
			continue
		}
		for _, part := range object.Parts {
			if part.Mesh != nil && part.Mesh.Name != "" {
				names[part.Mesh.Name] = true
			}
		}
	}
	return names
}

// updateOrders обрабатывает клики правой кнопкой: приказ движения выбранному
// юниту в точку на карте или к объекту под курсором. Без выделения, на паузе
// и по кликам на панелях HUD приказов нет.
func (p *Gameplay) updateOrders(c *scene.Ctx) {
	if p.hud == nil || p.hud.factory == nil || p.hud.paused {
		return
	}
	// Правая кнопка в режиме размещения отменяет постройку, а не отдаёт
	// приказ движения.
	if p.hud.placeBlueprint != "" {
		for _, click := range c.Input.Clicks {
			if click.Button == 3 && !p.hud.pointInHUD(click.X, click.Y, c) {
				p.hud.clearPlacement()
				p.hud.command = loc.T("Placement cancelled")
			}
		}
		return
	}
	if _, ok := p.hud.selectedEntity(); !ok || !p.hud.canIssueOrders() {
		return
	}
	// Delete уничтожает своё выделение — как в игре, где этим добивают
	// застрявший юнит. У пробуждённого монолита смерть вернёт постройку
	// (см. syncMovedObjects).
	if c.Key(window.KeyDelete) {
		p.killSelection()
	}
	for _, click := range c.Input.Clicks {
		if click.Button != 3 || p.hud.pointInHUD(click.X, click.Y, c) {
			continue
		}
		// Правая кнопка — обычный приказ: приказ, выбранный кнопкой панели и
		// ждавший цели, снимается, чтобы не выстрелить следующим кликом.
		p.hud.clearPendingOrder()
		p.issueOrderToSelection(click.X, click.Y, c)
	}
}

// setObjectAlpha задаёт прозрачность меша объекта: 1 — обычная, 0 —
// полностью прозрачный. По ней трупы гаснут плавно.
func (p *Gameplay) setObjectAlpha(o *gameplayObject, alpha float32) {
	if o == nil || o.meshName == "" || p.world == nil {
		return
	}
	p.world.SetObjectAlpha(o.meshName, alpha)
}

// setObjectVisible прячет или показывает меш объекта. Скрытый объект
// остаётся в списке сцены — так пробуждённый монолит убирает постройку со
// своего места и возвращает её обратно, не перезагружая сцену.
func (p *Gameplay) setObjectVisible(o *gameplayObject, visible bool) {
	if o == nil || o.hidden == !visible {
		return
	}
	o.hidden = !visible
	if p.world != nil && o.meshName != "" {
		p.world.SetVisible(o.meshName, visible)
	}
}

// ageCorpse ведёт труп к исчезновению. Тело, которое ещё можно собрать
// спайдером или поднять воскрешением, лежит на карте, пока его не заберут;
// тело с восстановлением — до подъёма; остальные corpseLinger секунд лежат
// и corpseFade секунд гаснут. У постройки играет клип смерти, и после него
// обломки сразу убираются — здания не гаснут.
func (p *Gameplay) ageCorpse(o *gameplayObject, entity entities.Entity, dt float64) {
	if o == nil {
		return
	}
	if p.hud != nil && p.hud.paused {
		dt = 0
	}
	if dt > 0 {
		o.deathAge += dt
	}
	// Таймер подъёма уже тикает в entities.stepRevival — тело должно быть
	// на виду, пока он не вышел. Собираемое тело ждёт спайдера или
	// воскрешения лорда сколько угодно долго.
	if entity.ReviveIn > 0 || (entity.UsableBody && !entity.CorpseTaken) || o.keepCorpse {
		p.setObjectVisible(o, true)
		p.setObjectAlpha(o, 1)
		return
	}
	// Тело забрал спайдер — оно уже ушло с карты, и таймер трупа его не
	// должен вернуть.
	if entity.CorpseTaken {
		p.setObjectVisible(o, false)
		return
	}
	if entity.Kind == entities.KindBuilding {
		if life := p.world.DeathDuration(o.meshName); life > 0 && o.deathAge < life {
			p.setObjectVisible(o, true)
			return
		}
		p.setObjectVisible(o, false)
		return
	}
	if o.deathAge < corpseLinger {
		p.setObjectVisible(o, true)
		return
	}
	// После corpseLinger труп гаснет corpseFade секунд, а не пропадает на
	// глазах: так видно, что тело убирается, а не исчезает рывком.
	alpha := 1 - (o.deathAge-corpseLinger)/corpseFade
	if alpha <= 0 {
		p.setObjectVisible(o, false)
		return
	}
	p.setObjectVisible(o, true)
	p.setObjectAlpha(o, float32(alpha))
}

// killSelection уничтожает свои выделенные отряды и постройки. Гибель идёт
// обычным путём (entities.Kill): у некронов остаётся шанс подняться, а у
// пробуждённого монолита — вернуться в постройку.
func (p *Gameplay) killSelection() {
	if p.hud == nil || p.hud.factory == nil {
		return
	}
	killed := 0
	for _, member := range p.hud.selectedEntities() {
		if member.Owner != p.hud.ownPlayer || !member.Alive {
			continue
		}
		if err := p.hud.factory.Kill(member.ID); err == nil {
			killed++
		}
	}
	if killed > 0 {
		p.hud.command = loc.T("Squad destroyed")
	}
}

// issueCommandOrder выдаёт приказ, выбранный кнопкой панели приказов:
// следующий левый клик по карте указывает цель. Движение и захват — один и
// тот же приказ с целью: точка захвата под курсором становится целью захвата.
// Атака появится вместе с боевой моделью, пока приказ не отдаётся.
func (p *Gameplay) issueCommandOrder(order string, c *scene.Ctx, x, y int) {
	if _, ok := p.hud.selectedEntity(); !ok || !p.hud.canIssueOrders() {
		return
	}
	if order == orderAttack {
		p.issueAttackAt(x, y, c)
		return
	}
	p.issueOrderToSelection(x, y, c)
}

// issueOrderToSelection раздаёт приказ всем выделенным: отряд выбирается
// целиком и идёт строем — каждый боец получает свою точку в ряду, а не одну
// общую цель, и не сходится с товарищами в одну точку.
func (p *Gameplay) issueOrderToSelection(x, y int, c *scene.Ctx) {
	members := p.hud.selectedEntities()
	if len(members) == 0 {
		return
	}
	// Лидер идёт обычным путём: он выбирает цель, строит маршрут и
	// проверяет, что проход есть.
	leader := members[0]
	p.issueOrder(leader, x, y, c)
	if len(members) == 1 {
		return
	}
	current, ok := p.hud.factory.Get(leader.ID)
	if !ok || current.Order == nil || current.OrderStatus != entities.OrderInProgress {
		return
	}
	base := *current.Order
	// Атака — одна цель на отряд: строй не нужен, каждый боец сам решает,
	// докуда ему идти, по дистанции своего оружия.
	if base.Attack {
		for _, member := range members {
			if member.Owner != p.hud.ownPlayer || member.ID == leader.ID {
				continue
			}
			order := base
			if !p.attachPath(&order, member) {
				continue
			}
			_ = p.hud.factory.Move(member.ID, order)
		}
		return
	}
	targets := formationTargets(len(members),
		[2]float64{base.Target.X, base.Target.Y},
		formationForward(members, base.Target), formationSpacing)
	for i, member := range members {
		if member.Owner != p.hud.ownPlayer || i >= len(targets) {
			continue
		}
		order := base
		order.Target = entities.Position{X: targets[i][0], Y: targets[i][1]}
		// Точку захватывает один боец, остальные встают рядом в строю.
		if order.Capture {
			order.Capture = false
			order.TargetID = ""
		}
		if !p.attachPath(&order, member) {
			continue
		}
		_ = p.hud.factory.Move(member.ID, order)
	}
}

// issueOrder выдаёт один приказ: объект под курсором становится целью
// движения (точка захвата — ещё и целью захвата), иначе цель — точка
// рельефа под курсором.
func (p *Gameplay) issueOrder(selected entities.Entity, x, y int, c *scene.Ctx) {
	ownRadius := 1.0
	if own, ok := p.objectByID(selected.ID); ok {
		ownRadius = own.radius
	}

	// 1) Объект под курсором.
	if id, ok := p.pickWorldObject(x, y, c.W, c.H); ok {
		target, found := p.hud.factory.Get(id)
		if !found {
			return
		}
		// Вражеский юнит или постройка — приказ атаки: цель запоминается,
		// бойцы подходят на дистанцию своего оружия и открывают огонь
		// (см. entities.stepCombat).
		if target.Owner != selected.Owner && !target.Capturable {
			if !target.Alive {
				p.hud.command = loc.T("Target already destroyed")
				return
			}
			order := entities.Order{
				Target:   entities.Position{X: target.Position.X, Y: target.Position.Y},
				TargetID: id,
				Attack:   true,
			}
			if !p.attachPath(&order, selected) {
				p.hud.command = loc.T("Path not found")
				return
			}
			if err := p.hud.factory.Move(selected.ID, order); err != nil {
				p.hud.command = loc.T("Order rejected: ") + err.Error()
				return
			}
			p.hud.command = loc.T("Order: attack")
			p.playOrderSound(selected, order)
			return
		}
		order := entities.Order{
			Target:   entities.Position{X: target.Position.X, Y: target.Position.Y},
			TargetID: id,
			Stop:     0.5,
		}
		switch {
		case target.Capturable && !p.pointBusy(id):
			// Незастроенная стратегическая позиция: встать на неё и
			// захватить, если отряд умеет это делать.
			order.Capture = p.selectedCanCapture(selected)
			if !order.Capture {
				p.hud.command = loc.T("This squad cannot capture")
			} else {
				p.hud.command = loc.T("Order: capture point")
			}
		default:
			// Подойти к объекту, не накладываясь на него.
			if targetObj, ok := p.objectByID(id); ok {
				order.Stop = targetObj.radius + ownRadius*0.5
			}
			p.hud.command = loc.T("Order: move to target")
		}
		if !p.attachPath(&order, selected) {
			p.hud.command = loc.T("Path not found")
			return
		}
		if err := p.hud.factory.Move(selected.ID, order); err != nil {
			p.hud.command = loc.T("Order rejected: ") + err.Error()
		} else {
			p.playOrderSound(selected, order)
		}
		return
	}

	// 2) Рельеф под курсором. Если попали в незастроенную стратегическую
	// позицию — приказ идти на её место и захватывать: цель в таком случае
	// сама точка, юнит добирается до неё и только потом захват начинается.
	hit, ok := p.groundHit(x, y, c.W, c.H)
	if !ok {
		return
	}
	order := entities.Order{
		Target: entities.Position{X: hit[0], Y: hit[1]},
		Stop:   0.3,
	}
	if id, point, found := p.capturableAt(hit[0], hit[1]); found {
		order.Target = entities.Position{X: point.center[0], Y: point.center[1]}
		order.TargetID = id
		order.Stop = 0
		target, _ := p.hud.factory.Get(id)
		if target.Owner == selected.Owner {
			// Своя точка — захватывать нечего, просто подойти.
			p.hud.command = loc.T("Point is already yours — order: move")
		} else if order.Capture = p.selectedCanCapture(selected); !order.Capture {
			p.hud.command = loc.T("This squad cannot capture")
		} else {
			p.hud.command = loc.T("Order: capture point")
		}
	} else {
		p.hud.command = loc.T("Order: move")
	}
	if !p.attachPath(&order, selected) {
		p.hud.command = loc.T("Path not found")
		return
	}
	if err := p.hud.factory.Move(selected.ID, order); err != nil {
		p.hud.command = loc.T("Order rejected: ") + err.Error()
		return
	}
	p.playOrderSound(selected, order)
}

// issueAttackAt отдаёт приказ атаки по вражескому объекту под курсором.
// Клик по земле приказ атаки не выдаёт: цель нужна конкретная, иначе боец
// пошёл бы воевать с пустотой.
func (p *Gameplay) issueAttackAt(x, y int, c *scene.Ctx) {
	id, ok := p.pickWorldObject(x, y, c.W, c.H)
	if !ok {
		p.hud.command = loc.T("Attack: select an enemy target")
		return
	}
	target, found := p.hud.factory.Get(id)
	if !found || target.Capturable {
		p.hud.command = loc.T("Attack: select an enemy target")
		return
	}
	if target.Owner == p.hud.ownPlayer {
		p.hud.command = loc.T("Attack: that is a friendly object")
		return
	}
	if !target.Alive {
		p.hud.command = loc.T("Attack: target already destroyed")
		return
	}
	p.issueOrderToSelection(x, y, c)
}

// capturableAt — незастроенная стратегическая позиция под точкой земли:
// её ID и объект. Точки захвата маленькие, поэтому берётся ближайшая
// в радиусе — юниту приказ ставится на саму точку, а не в сторону.
func (p *Gameplay) capturableAt(x, y float64) (string, gameplayObject, bool) {
	if p.hud == nil || p.hud.factory == nil {
		return "", gameplayObject{}, false
	}
	best, bestDist := "", math.Inf(1)
	var bestObj gameplayObject
	for _, object := range p.objects {
		if object.id == "" {
			continue
		}
		entity, ok := p.hud.factory.Get(object.id)
		if !ok || !entity.Capturable {
			continue
		}
		// На точке стоит постройка: точка недоступна для приказов захвата и
		// снятия флага, пока постройка цела.
		if p.pointBusy(object.id) {
			continue
		}
		distance := math.Hypot(x-object.center[0], y-object.center[1])
		radius := object.radius
		if radius < 3 {
			radius = 3
		}
		if distance <= radius && distance < bestDist {
			best, bestDist, bestObj = object.id, distance, object
		}
	}
	return best, bestObj, best != ""
}

// objectByID — объект сцены по ID сущности.
func (p *Gameplay) objectByID(id string) (gameplayObject, bool) {
	for _, object := range p.objects {
		if object.id == id {
			return object, true
		}
	}
	return gameplayObject{}, false
}

// aimAngles — куда бойцу целиться: пеленг на цель относительно его взгляда
// и подъём ствола. Оба угла считаются заново каждый кадр — по ним клипы
// прицела выбирают тот свой кадр, где боец развёрнут к цели, так что и
// идущая цель, и идущий стрелок остаются на линии огня.
//
// Без известной цели — прямо перед собой: в автобое юнит сам находит
// противника, и его направление уже лежит в Facing.
func (p *Gameplay) aimAngles(entity entities.Entity) (bearing, pitch float64) {
	if entity.Order == nil || entity.Order.TargetID == "" || p.hud == nil || p.hud.factory == nil {
		return 0, 0
	}
	target, ok := p.hud.factory.Get(entity.Order.TargetID)
	if !ok || !target.Alive {
		return 0, 0
	}
	dx := target.Position.X - entity.Position.X
	dy := target.Position.Y - entity.Position.Y
	horizontal := math.Hypot(dx, dy)
	// Пеленг в системе бойца: −π/2 модели (её лицо) уже учтён выбором
	// кадра прицела, поэтому здесь только разница направлений.
	bearing = math.Atan2(dy, dx) - entity.Facing
	if horizontal <= 1e-6 {
		return bearing, 0
	}
	// Целится в корпус, а не в точку на земле: подъём — разница высот
	// цели и ствола на горизонтальной дистанции.
	targetZ := p.groundHeight(target.Position.X, target.Position.Y) + 1
	if object, ok := p.objectByID(target.ID); ok {
		targetZ = object.center[2] + math.Max(object.radius*0.4, 0.5)
	}
	shooterZ := p.groundHeight(entity.Position.X, entity.Position.Y) + 1.2
	if object, ok := p.objectByID(entity.ID); ok {
		shooterZ = object.center[2] + 1.2
	}
	pitch = math.Atan2(targetZ-shooterZ, horizontal)
	return bearing, pitch
}

// selectedCanCapture — умеет ли выделенный отряд захватывать точки
// (squad_capture_strategic_point_ext из чертежа sbps).
func (p *Gameplay) selectedCanCapture(selected entities.Entity) bool {
	if p.hud == nil || p.hud.attribData == nil {
		return false
	}
	blueprint := p.hud.blueprintOf(selected)
	if blueprint == "" {
		return false
	}
	squad, ok := p.hud.attribData.squadFor(blueprint)
	return ok && squad.CanCapture
}

// groundHit — точка на рельефе под экранным пикселем: луч камеры идёт по
// шагам, пока не пересечёт землю (сетка высот p.groundHeight), затем
// точка уточняется пропорцией пересечения.
func (p *Gameplay) groundHit(x, y, w, h int) ([3]float64, bool) {
	var miss [3]float64
	origin, dir, ok := p.cam.RayFromScreen(x, y, w, h)
	if !ok {
		return miss, false
	}
	const step = 2.0
	const maxDist = 4000.0
	prevT := 0.0
	prevD := origin[2] - p.groundHeight(origin[0], origin[1])
	if prevD <= 0 {
		return miss, false
	}
	for t := step; t <= maxDist; t += step {
		px := origin[0] + dir[0]*t
		py := origin[1] + dir[1]*t
		pz := origin[2] + dir[2]*t
		d := pz - p.groundHeight(px, py)
		if d <= 0 {
			frac := prevD / (prevD - d)
			tHit := prevT + frac*(t-prevT)
			return [3]float64{
				origin[0] + dir[0]*tHit,
				origin[1] + dir[1]*tHit,
				origin[2] + dir[2]*tHit,
			}, true
		}
		prevT, prevD = t, d
	}
	return miss, false
}

// corpseLinger — сколько секунд труп юнита без шанса подняться остаётся на
// карте. Тело с восстановлением лежит до своего таймера (stepRevival), а
// тело, пригодное для сборки или воскрешения, — пока его не заберут.
const corpseLinger = 8.0

// corpseFade — сколько секунд длится плавное исчезновение трупа после
// corpseLinger. Постройки гаснут не плавно, а сразу после клипа смерти.
const corpseFade = 1.5

// syncMovedObjects переносит сдвинувшиеся сущности в сцену: обновляет центр
// объекта (для выбора и индикатора) и сдвигает его меш в рендерере. Здесь же
// тикает возраст трупов: отыгравшие тело и обломки уходят со сцены.
func (p *Gameplay) syncMovedObjects(dt float64) {
	if p.hud == nil || p.hud.factory == nil {
		return
	}
	for i := range p.objects {
		o := &p.objects[i]
		if o.id == "" {
			continue
		}
		entity, ok := p.hud.factory.Get(o.id)
		if !ok {
			continue
		}
		// Спящая постройка занята пробуждённым юнитом: она не рисуется и не
		// выбирается, но место и всё, что с ним связано, остаются за ней.
		if entity.Dormant {
			p.setObjectVisible(o, false)
			o.selectable = false
			continue
		}
		// Живой объект всегда виден, и отсчёт трупа начинается заново: вторая
		// смерть не должна наследовать возраст первой.
		if entity.Alive {
			p.setObjectVisible(o, true)
			p.setObjectAlpha(o, 1)
			o.deathAge = 0
		}
		cur := [3]float64{entity.Position.X, entity.Position.Y, o.center[2]}
		// Двигается только приказ в работе: завершённый или отменённый
		// остаётся у сущности как последний — со статусом. Юнит идёт
		// по прямой в плоскости карты, но вертикально следует рельефу:
		// высота каждый кадр берётся из сетки земли под его текущей
		// точкой, поэтому на холмах и впадинах он движется по поверхности,
		// а не летит по прямой между высотами точек А и Б.
		if entity.Order != nil && entity.OrderStatus == entities.OrderInProgress {
			// Высота земли идёт к центру объекта с его собственным запасом:
			// центр модели сидит выше земли на половину высоты, и без
			// запаса боец при первом шаге уходил в землю по пояс.
			cur[2] = p.groundHeight(cur[0], cur[1]) + o.zOffset
		}
		// Стройку ведёт прогресс работы (updateConstruction), поэтому её
		// состояние анимации здесь не трогается.
		if entity.UnderConstruction() {
			continue
		}
		// Уничтоженные не выбираются и лежат в позе смерти: у некронов
		// тело остаётся и может подняться само (entities.stepRevival),
		// остальные трупы снимаются со сцены отдельно.
		if !entity.Alive {
			o.selectable = false
			// Пробуждённый юнит: его гибель возвращает постройку на место, а
			// сам он уходит со сцены без тела — это не обычный труп.
			if entity.FromBuilding != "" {
				p.setObjectVisible(o, false)
				_ = p.hud.factory.SetDormant(entity.FromBuilding, false)
				continue
			}
			if p.world != nil && o.meshName != "" {
				p.world.SetAnim(o.meshName, o.moveOffset, sgbview.AnimDeath)
			}
			p.ageCorpse(o, entity, dt)
			continue
		}
		// Живой пробуждённый монолит держит свою постройку спящей.
		if entity.FromBuilding != "" {
			_ = p.hud.factory.SetDormant(entity.FromBuilding, true)
		}
		o.selectable = entity.Selectable
		// Застроенная точка не выбирается: и захват, и снятие флага на ней
		// недоступны, пока постройка цела.
		if entity.Capturable && p.pointBusy(o.id) {
			o.selectable = false
		}
		// Анимация по приказу: захват в приоритете (юнит уже на точке),
		// затем движение, иначе покой — боец, постройка и окружение
		// без приказов стоят на idle.
		state := sgbview.AnimIdle
		switch {
		case entity.Capturing:
			state = sgbview.AnimCapture
		case entity.Attacking:
			// В дистанции огня боец стоит и стреляет: клипы боя играют
			// вместе с прицелом, который смотрит на цель.
			state = sgbview.AnimAttack
		case entity.Order != nil && entity.OrderStatus == entities.OrderInProgress:
			state = sgbview.AnimMove
		}
		moved := cur != o.center
		if moved {
			o.moveOffset[0] += cur[0] - o.center[0]
			o.moveOffset[1] += cur[1] - o.center[1]
			o.moveOffset[2] += cur[2] - o.center[2]
			o.center = cur
		}
		if p.world == nil || o.meshName == "" {
			continue
		}
		// Анимируемый объект: сдвиг накладывается матрицей в палитре
		// костей (геометрия остаётся в точке появления), поэтому меш
		// через MoveObject не двигаем — иначе будет двойной сдвиг.
		if p.world.SetAnim(o.meshName, o.moveOffset, state) {
			// Поворот обновляется только на ходу: у стоящего юнита
			// направление остаётся последним (и запечённым в геометрию — у
			// тех, кто ещё не ходил), иначе любой боец разворачивался бы
			// вокруг своей точки постановки на пустом месте.
			if o.kind == entities.KindUnit && (state == sgbview.AnimMove || entity.Attacking) {
				// Поправка на то, куда смотрит сама модель: направление шага
				// плюс четверть оборота, иначе боец идёт боком.
				p.world.SetFacing(o.meshName, entity.Facing+sgbview.ModelFacingOffset)
			}
			if state == sgbview.AnimAttack && p.world.SetAttack(o.meshName, entity.Weapon == nil) {
				bearing, pitch := p.aimAngles(entity)
				p.world.SetAim(o.meshName, bearing, pitch)
			}
			continue
		}
		if moved {
			_ = p.world.MoveObject(o.meshName, [3]float32{
				float32(cur[0]), float32(cur[1]), float32(cur[2]),
			})
		}
	}
}
