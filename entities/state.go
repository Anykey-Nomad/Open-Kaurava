package entities

import (
	"errors"
	"github.com/Anykey-Nomad/Open-Kaurava/loc"
	"math"
)

// Здесь собрано «живое» состояние сущности: где она стоит, может ли
// двигаться, здорова ли, что с её моралью и модификаторами и какой приказ
// она исполняет. Фабрика (factory.go) держит реестр — владение, экономику
// и строительство, — а всё, что происходит с бойцем в мире, живёт в State
// и меняется только методами фабрики (Move, Place): они берут мьютекс.

// State — живое состояние сущности на карте. Встроено в Entity, поэтому
// поля читаются как entity.Position, entity.OrderStatus и т. д.
type State struct {
	// Position — положение на земле (мировые X/Y).
	Position Position
	// Speed — базовая скорость из чертежа (moving_ext/speed_max) в
	// единицах мира в секунду; 0 — сущность не двигается. С учётом
	// модификаторов — см. CurrentSpeed.
	Speed float64
	// AccelTime, BrakeTime — секунды разгона с нуля до скорости и
	// торможения до остановки (moving_ext/acceleration_time и
	// deceleration_time; пехота 2 с и 1 с). 0 — данных нет, движение
	// идёт с постоянной скоростью, как раньше.
	AccelTime, BrakeTime float64
	// Velocity — текущая фактическая скорость: растёт по AccelTime,
	// падает по BrakeTime при торможении у цели, обнуляется по остановке.
	Velocity float64
	// Alive — жива ли сущность. Ложь у трупов: стартовые «мертвецы»
	// некронов (лорд, томб-спайдеры) появляются уже мёртвыми, и когда
	// появится урон, здесь же будет фиксироваться гибель.
	Alive bool
	// Health — здоровье: Max по чертежу, Current — сколько осталось.
	// Урона пока никто не наносит, поэтому Current либо равен Max, либо
	// 0 — у трупа или если чертёж здоровья не дал.
	Health Health
	// Morale — мораль бойца. Данные лежат в sbps (squad_morale_ext), но
	// бой не симулируется, поэтому значение пока не читается и не меняется.
	// MoraleMax — полная шкала отряда (0 — морали нет, например у техники
	// и построек): по ней панель выделения рисует вторую полоску.
	Morale    float64
	MoraleMax float64
	// Modifiers — активные модификаторы бойца (способности, исследования).
	// Пусто, пока способностей нет; бонус к скорости уже учитывается,
	// чтобы движение не переделывалось, когда модификаторы появятся.
	Modifiers []Modifier
	// Capturing — боец прибыл на захватываемую точку и стоит на ней.
	// Прогресс захвата живёт на самой точке (Entity.CaptureProgress), а
	// здесь лежит его копия для панелей. Сбрасывается новым приказом:
	// ушёл с точки — захват прерван.
	Capturing       bool
	CaptureProgress float64
	// Attacking — цель в дистанции оружия и боец ведёт огонь (см.
	// stepCombat); Cooldown — сколько секунд осталось до следующего выстрела.
	Attacking bool
	Cooldown  float64
	// Shots — сколько выстрелов сделал боец. Счётчик растёт только,
	// поэтому сцена видит каждый выстрел и рисует его вспышку, не
	// подслушивая внутренности боя.
	Shots uint64
	// LastShotTarget и LastShotHit — цель последнего выстрела и попал ли
	// он: промах по accuracy урона не наносит, и эффект должен пройти
	// мимо цели, а не закончиться на ней. Поля живут рядом со счётчиком,
	// чтобы сцена читала выстрел одним движением.
	LastShotTarget string
	LastShotHit    bool
	// AutoAttack — боец сам открывает огонь по ближайшему врагу в дистанции
	// оружия. Включена по умолчанию; выключать её будет отдельный приказ.
	AutoAttack bool
	// Order — текущий или последний приказ; nil — приказов не было.
	Order *Order
	// OrderStatus — статус Order: выполняется, выполнен, не выполнен.
	OrderStatus OrderStatus
}

// Health — здоровье сущности: максимум по чертежу и текущее значение.
type Health struct {
	Current float64
	Max     float64
}

// Modifier — активный модификатор бойца. Пока применяет только бонус к
// скорости; урон, броня и прочие эффекты добавятся вместе с боем.
type Modifier struct {
	Name       string
	SpeedBonus float64
}

// CurrentSpeed — скорость с учётом активных модификаторов, не ниже нуля.
func (s State) CurrentSpeed() float64 {
	speed := s.Speed
	for _, modifier := range s.Modifiers {
		speed += modifier.SpeedBonus
	}
	if speed < 0 {
		return 0
	}
	return speed
}

// Order — приказ движущейся сущности: идти к точке (и, возможно, к цели
// на карте) и по прибытию захватить цель.
//
// Stop — дистанция остановки: 0 для прихода ровно в точку, радиус цели,
// чтобы не накладываться на объект, к которому идут.
type Order struct {
	// Target — координаты цели на земле (мировые X/Y).
	Target Position
	// TargetID — сущность, к которой идут; пусто — просто точка на карте.
	TargetID string
	// Attack — атаковать TargetID: боец идёт к цели, пока она не окажется в
	// дистанции его оружия, и стреляет, пока цель жива. Дистанция у каждого
	// бойца своя, поэтому отряд подходит к цели не строем, а по оружию.
	Attack bool
	// Path — промежуточные точки маршрута по карте проходимости:
	// юнит идёт по ним по очереди и только потом к Target. Пусто —
	// прямой путь (нет данных о проходимости или цель рядом).
	// Заполняется поиском пути в момент выдачи приказа.
	Path []Position
	// Stop — на каком расстоянии от Target остановиться.
	Stop float64
	// Capture — захватить TargetID по прибытии (щелчок по незастроенной
	// стратегической позиции). Ошибка захвата делает приказ невыполненным.
	Capture bool
}

// OrderStatus — статус приказа: выполняется, выполнен, не выполнен.
type OrderStatus int

// CaptureSeconds — сколько секунд боец стоит на стратегической точке,
// чтобы захватить её: приказ приходит, юнит идёт, по прибытии начинается
// захват и столько времени занимает.
const CaptureSeconds = 10

const (
	// OrderNone — приказов не было.
	OrderNone OrderStatus = iota
	// OrderInProgress — приказ выполняется.
	OrderInProgress
	// OrderDone — приказ выполнен.
	OrderDone
	// OrderFailed — приказ не выполнен: захват заблокирован постройкой
	// либо приказ отменён до завершения.
	OrderFailed
)

// String — статус для статусной строки и логов (переводится через loc).
func (s OrderStatus) String() string {
	switch s {
	case OrderInProgress:
		return loc.T("in progress")
	case OrderDone:
		return loc.T("done")
	case OrderFailed:
		return loc.T("failed")
	default:
		return loc.T("no order")
	}
}

// Place меняет положение ранее созданной сущности на карте.
func (f *Factory) Place(id string, position Position) error {
	if !finite(position.X) || !finite(position.Y) {
		return errors.New("entities: координаты должны быть конечными числами")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	entity, ok := f.entities[id]
	if !ok {
		return ErrNotFound
	}
	entity.Position = position
	f.entities[id] = entity
	return nil
}

// Move отдаёт сущности приказ движения. Сущность без скорости (постройка,
// окружение) приказ получить не может.
func (f *Factory) Move(id string, order Order) error {
	if f == nil {
		return errors.New("entities: nil factory")
	}
	if !finite(order.Target.X) || !finite(order.Target.Y) ||
		!finite(order.Stop) || order.Stop < 0 {
		return ErrInvalidOrder
	}
	for _, point := range order.Path {
		if !finite(point.X) || !finite(point.Y) {
			return ErrInvalidOrder
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	entity, ok := f.entities[id]
	if !ok {
		return ErrNotFound
	}
	if !entity.Alive {
		return ErrNotAlive
	}
	if entity.CurrentSpeed() <= 0 {
		return ErrNotMoving
	}
	if order.TargetID != "" {
		if _, ok := f.entities[order.TargetID]; !ok {
			return ErrNotFound
		}
	}
	if order.Attack && (entity.Weapon == nil || entity.Weapon.Range <= 0) {
		return ErrNoWeapon
	}
	next := entity
	next.Order = &order
	next.OrderStatus = OrderInProgress
	// Новый приказ прерывает идущий захват — боец уходит со точки.
	next.Capturing = false
	next.CaptureProgress = 0
	f.entities[id] = next
	return nil
}

// ClearOrder отменяет текущий приказ: невыполненный он помечается как
// не выполненный, чтобы статус последнего приказа не врал.
func (f *Factory) ClearOrder(id string) error {
	if f == nil {
		return errors.New("entities: nil factory")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	entity, ok := f.entities[id]
	if !ok {
		return ErrNotFound
	}
	if entity.Order != nil && entity.OrderStatus == OrderInProgress {
		entity.OrderStatus = OrderFailed
	}
	entity.Capturing = false
	entity.CaptureProgress = 0
	f.entities[id] = entity
	return nil
}

// stepCapture ведёт захват точек: прогресс живёт на самой точке, а не на
// бойце, поэтому флаг не поднимается с нуля у каждого нового бойца.
//
// Правила те же, что и в игре:
//   - боец доходит до точки и начинает захват по прибытии;
//   - пока на точке стоят бойцы разных игроков, прогресс стоит;
//   - брошенная точка потихоньку теряет прогресс — вдвое быстрее, чем
//     набирала (флаг опускается);
//   - чужой флаг снимается тоже вдвое быстрее: сначала он опускается,
//     и только потом поднимается флаг нового хозяина;
//   - если точку начал брать другой игрок, недоделанный прогресс прежнего
//     пропадает сразу.
func (f *Factory) stepCapture(dt float64) {
	if !(dt > 0) {
		return
	}
	// Кто сейчас захватывает каждую точку.
	crowds := map[string][]Entity{}
	for _, entity := range f.entities {
		if !entity.Alive || !entity.Capturing || entity.Order == nil || !entity.Order.Capture {
			continue
		}
		if id := entity.Order.TargetID; id != "" {
			crowds[id] = append(crowds[id], entity)
		}
	}
	for id, point := range f.entities {
		if !point.Capturable {
			continue
		}
		f.captureTickLocked(&point, crowds[id], dt)
		f.entities[id] = point
	}
}

// captureTickLocked продвигает захват одной точки. units — снимки бойцов,
// которые стоят на ней с приказом захвата (изменять нужно через карту
// сущностей: снимки уже не связаны с реестром).
func (f *Factory) captureTickLocked(point *Entity, units []Entity, dt float64) {
	owned := point.Owner != "" && point.Owner != NeutralPlayer
	// Захватчик один: если на точке стоят бойцы разных игроков, флаг не
	// двигается вовсе — спор.
	var capturer PlayerID
	for i, unit := range units {
		if i == 0 {
			capturer = unit.Owner
			continue
		}
		if unit.Owner != capturer {
			capturer = ""
			break
		}
	}
	if capturer == "" {
		if len(units) > 0 {
			// Спорная точка: прогресс стоит, флаг не двигается.
			return
		}
		if owned {
			// Захваченная точка держит флаг, пока его не снимут.
			point.CaptureProgress = 1
			point.CapturePlayer = point.Owner
			return
		}
		// Никто не стоит — флаг опускается вдвое быстрее, чем поднимался.
		if point.CaptureProgress > 0 {
			point.CaptureProgress -= 2 * dt / CaptureSeconds
			if point.CaptureProgress <= 0 {
				point.CaptureProgress = 0
				point.CapturePlayer = ""
			}
		}
		return
	}
	if owned && point.Owner == capturer {
		// Своя точка: флаг уже поднят — захватывать нечего, приказ
		// закрывается сразу по прибытии.
		point.CaptureProgress = 1
		point.CapturePlayer = capturer
		f.finishCaptureLocked(units, 1)
		return
	}
	if owned {
		// Чужой флаг: опускается вдвое быстрее, чем поднимается свой.
		if point.CapturePlayer != point.Owner || point.CaptureProgress <= 0 {
			point.CapturePlayer = point.Owner
			point.CaptureProgress = 1
		}
		point.CaptureProgress -= 2 * dt / CaptureSeconds
		if point.CaptureProgress > 0 {
			f.setCapturersProgressLocked(units, point.CaptureProgress)
			return
		}
		point.CaptureProgress = 0
		point.CapturePlayer = capturer
		f.assignOwnerLocked(point, NeutralPlayer)
	}
	// Флаг поднимает новый хозяин; чужая недоделанная заготовка пропадает
	// сразу — иначе два игрока копили бы прогресс на одной точке.
	if point.CapturePlayer != capturer {
		point.CaptureProgress = 0
	}
	point.CapturePlayer = capturer
	point.CaptureProgress += dt / CaptureSeconds
	if point.CaptureProgress < 1 {
		f.setCapturersProgressLocked(units, point.CaptureProgress)
		return
	}
	point.CaptureProgress = 1
	f.assignOwnerLocked(point, capturer)
	f.finishCaptureLocked(units, point.CaptureProgress)
}

// setCapturersProgressLocked повторяет прогресс точки у стоящих на ней
// бойцов: по нему панель и анимация видят один и тот же процент.
func (f *Factory) setCapturersProgressLocked(units []Entity, progress float64) {
	for _, unit := range units {
		current, ok := f.entities[unit.ID]
		if !ok {
			continue
		}
		current.CaptureProgress = progress
		f.entities[current.ID] = current
	}
}

// finishCaptureLocked завершает приказ захвата у бойцов на точке.
func (f *Factory) finishCaptureLocked(units []Entity, progress float64) {
	for _, unit := range units {
		current, ok := f.entities[unit.ID]
		if !ok {
			continue
		}
		current.Capturing = false
		current.CaptureProgress = progress
		current.Velocity = 0
		if current.OrderStatus == OrderInProgress {
			current.OrderStatus = OrderDone
		}
		f.entities[current.ID] = current
	}
}

// stepMovement двигает сущности с приказами за dt секунд: движение
// разгоняется и тормозит по данным чертежа (acceleration_time /
// deceleration_time), а промежуточные точки маршрута проходятся на
// скорости — тормоз только у конечной цели. Приказ захвата живёт в два
// этапа: юнит сначала доходит до точки, и только по прибытии начинается
// захват — боец стоит на точке CaptureSeconds секунд, и по их истечении
// точка переходит игроку. Если точка заблокирована постройкой, приказ
// становится невыполненным сразу по прибытии, а новый приказ или
// отмена прерывают идущий захват.
//
// Вызывается из Tick с уже взятым мьютексом.
func (f *Factory) stepMovement(dt float64) {
	for id, entity := range f.entities {
		if entity.Order == nil || !entity.Alive || entity.OrderStatus != OrderInProgress {
			// Приказ не выполняется (выполнен, отменён, нет) — боец
			// стоит: остаточная скорость не должна пережить остановку.
			if entity.Velocity != 0 {
				entity.Velocity = 0
				f.entities[id] = entity
			}
			continue
		}
		order := *entity.Order

		// Фаза захвата: боец стоит на точке, а прогресс ведёт сама точка
		// (stepCapture): флаг на ней один, и его высота не зависит от того,
		// сколько бойцов стоят рядом. Здесь только останавливаем движение.
		if entity.Capturing {
			entity.Velocity = 0
			f.entities[id] = entity
			continue
		}

		speed := entity.CurrentSpeed()
		if speed <= 0 {
			continue
		}
		// Пока есть промежуточные точки маршрута, цель — первая из них
		// (без радиуса остановки: waypoint берётся ровно). Приказ
		// остаётся выполняющимся, пока не будет пройден последний.
		target, stop := order.Target, order.Stop
		waypoint := len(order.Path) > 0
		if waypoint {
			target, stop = order.Path[0], 0
		}
		// Атака: цель двигается, поэтому точка приказа обновляется на каждом
		// шаге. Дальность у каждого бойца своя — его оружие плюс радиус цели,
		// и как только цель оказалась в ней, боец останавливается: маршрут
		// «вплотную» уже не нужен, огонь ведёт stepCombat.
		attacking := order.Attack && order.TargetID != ""
		if attacking {
			victim, found := f.entities[order.TargetID]
			// Спящая постройка (пробуждённый монолит) больше не цель.
			if !found || !victim.Alive || victim.Dormant {
				entity.OrderStatus = OrderDone
				entity.Attacking = false
				entity.Cooldown = 0
				entity.Velocity = 0
				f.entities[id] = entity
				continue
			}
			order.Target = victim.Position
			entity.Order = &order
			// Боец подходит к стене цели ровно на дальность своего оружия:
			// расстояние меряется до бокса основания, а не до центра, иначе
			// у большой постройки габариты съедали бы разницу между орудиями
			// и все вставали бы на одну и ту же дистанцию.
			if entity.Weapon != nil && victim.SurfaceGap(entity.Position.X, entity.Position.Y) <= entity.Weapon.Range {
				// В дистанции: стоим и смотрим на цель. Остаток маршрута
				// выбрасываем — иначе после отхода цели боец пошёл бы по
				// устаревшим точкам вплоть до её старого места.
				order.Path = nil
				entity.Order = &order
				entity.Velocity = 0
				entity.Facing = math.Atan2(victim.Position.Y-entity.Position.Y,
					victim.Position.X-entity.Position.X)
				f.entities[id] = entity
				continue
			}
		}
		dx := target.X - entity.Position.X
		dy := target.Y - entity.Position.Y
		dist := math.Hypot(dx, dy)
		// Смотреть туда, куда боец шагает на самом деле: направление берётся
		// из текущего участка маршрута (waypoint), а не из конечной цели.
		if dist > 1e-6 {
			entity.Facing = math.Atan2(dy, dx)
		}

		// Разгон и торможение. accel — прибавка скорости за секунду
		// (speed_max набирается за acceleration_time), и у конечной
		// цели скорость ограничивается тормозным путём v² = 2·a·s
		// (остановка за deceleration_time): юнит подкатывает к точке,
		// а не долетает рывком. Промежуточные waypoints проходятся
		// без торможения — маршрут не тормозит на поворотах.
		vel := entity.Velocity
		if entity.AccelTime > 0 {
			vel += speed / entity.AccelTime * dt
			if vel > speed {
				vel = speed
			}
		} else {
			// Нет данных о разгоне — движение постоянной скоростью.
			vel = speed
		}
		if !waypoint && entity.BrakeTime > 0 {
			remaining := dist - stop
			if remaining < 0 {
				remaining = 0
			}
			if cap := math.Sqrt(2 * (speed / entity.BrakeTime) * remaining); vel > cap {
				vel = cap
			}
		}
		entity.Velocity = vel
		step := vel * dt
		arrived := false
		switch {
		case dist <= stop:
			// Уже на дистанции остановки.
			arrived = true
		case step >= dist-stop:
			// Шага хватает, чтобы встать на дистанции остановки.
			if stop > 0 && dist > 0 {
				entity.Position = Position{
					X: target.X - dx/dist*stop,
					Y: target.Y - dy/dist*stop,
				}
			} else {
				entity.Position = target
			}
			arrived = true
		default:
			entity.Position = Position{
				X: entity.Position.X + dx/dist*step,
				Y: entity.Position.Y + dy/dist*step,
			}
		}
		if arrived && waypoint {
			// Промежуточная точка пройдена: срезаем её и идём к следующей.
			order.Path = order.Path[1:]
			entity.Order = &order
			f.entities[id] = entity
			continue
		}
		if arrived {
			// Конечная цель: скорость обнуляется — дальше торможение
			// начинается заново с нового приказа.
			entity.Velocity = 0
			if order.Attack && order.TargetID != "" {
				// На дистанции огня приказ остаётся в работе: стреляет
				// stepCombat, а уйдёт цель дальше — боец пойдёт за ней.
				if victim, found := f.entities[order.TargetID]; found {
					entity.Facing = math.Atan2(victim.Position.Y-entity.Position.Y,
						victim.Position.X-entity.Position.X)
				}
				f.entities[id] = entity
				continue
			}
			if order.Capture && order.TargetID != "" && entity.Owner != "" && entity.Owner != NeutralPlayer {
				// Захват начинается по прибытии: сначала проверяем, что
				// точку вообще можно брать (стройка на ней блокирует),
				// и только потом встаём на неё.
				if err := f.captureReadyLocked(order.TargetID, entity.Owner); err != nil {
					entity.OrderStatus = OrderFailed
				} else {
					entity.Capturing = true
					entity.CaptureProgress = 0
				}
			} else {
				entity.OrderStatus = OrderDone
			}
		}
		f.entities[id] = entity
	}
}
