package entities

import (
	"math"
	"math/rand"
)

// Бой: пока только стрельба. У каждого бойца своё оружие со своей
// дистанцией, поэтому отряд, получивший приказ атаки, подходит к цели
// каждый на своё расстояние и открывает огонь сам. Разошлись — огонь
// прекращается, сошлись снова — продолжается: приказ атаки живёт, пока
// цель жива или пока его не отменят.
//
// Без приказа атаки работает автоатака: боец сам стреляет по ближайшему
// врагу, до которого дотягивается его оружие. По умолчанию она включена у
// всех (units и построек с оружием).

// Weapon — дальнобойное оружие бойца в метрах мира. Числа берутся из
// чертежей Data/attrib/weapon, читает их пакет attrib.
type Weapon struct {
	// Range — максимальная дистанция стрельбы в метрах мира.
	Range float64
	// Accuracy — доля попаданий (0..1): промах урона не наносит.
	Accuracy float64
	// ReloadTime — секунды между выстрелами.
	ReloadTime float64
	// MinDamage, MaxDamage — разброс урона за попадание.
	MinDamage, MaxDamage float64
	// Piercing — урон по классам брони цели в процентах
	// ("tp_infantry_heavy_med"). Пустая карта — оружие без данных о броне
	// и бьёт по всем одинаково.
	Piercing map[string]float64
}

// Damage считает урон одного выстрела: бросок в разбросе, проверка
// попадания по accuracy и множитель пробития по классу брони цели.
//
// Броня цели (health_ext/armour) снижает урон, а пробитие оружия эту защиту
// уменьшает:
//
//	множитель = 1 − (броня/100) · (1 − пробитие/100)
//
// У всех юнитов и построек DoWDE броня равна 100, поэтому множитель равен
// проценту из таблицы пробития: 45 % по пехоте в тяжёлой броне, 20 % по
// средней постройке, 5 % по тяжёлому танку. Формулу движка DoW сверить по
// чертежам нельзя — она выведена из этих чисел и проверена только на
// броне 100, где сводится к проценту.
func (w Weapon) Damage(armour float64, armourClass string) float64 {
	damage, _ := w.DamageHit(armour, armourClass)
	return damage
}

// DamageHit — то же, что Damage, но ещё и сообщает, попал ли выстрел: сцена
// рисует промах проходящим мимо цели эффектом, а не попаданием.
func (w Weapon) DamageHit(armour float64, armourClass string) (damage float64, hit bool) {
	if w.MaxDamage <= 0 {
		return 0, false
	}
	if w.Accuracy > 0 && w.Accuracy < 1 && rand.Float64() > w.Accuracy {
		// Промах: оружие без зоны поражения урона не наносит вовсе.
		return 0, false
	}
	roll := w.MinDamage
	if w.MaxDamage > w.MinDamage {
		roll += rand.Float64() * (w.MaxDamage - w.MinDamage)
	}
	multiplier := 1 - (armour/100)*(1-w.piercing(armourClass)/100)
	if multiplier <= 0 {
		return 0, true
	}
	if multiplier > 1 {
		multiplier = 1
	}
	return roll * multiplier, true
}

// piercing — процент урона по классу брони цели. Оружие без таблицы
// пробития бьёт по всем классам одинаково; цель с неизвестным классом
// (чертёж броню не дал) считается незащищённой — иначе урон молча обнулился
// бы, а класс, которого в таблице нет, не пробивается вовсе.
func (w Weapon) piercing(class string) float64 {
	if len(w.Piercing) == 0 || class == "" {
		return 100
	}
	if value, ok := w.Piercing[class]; ok {
		return value
	}
	return 0
}

// stepCombat ведёт стрельбу за dt секунд: по приказу атаки боец стреляет,
// когда цель в дистанции его оружия и жива; без приказа — автоатака по
// ближайшему врагу в дистанции, но только с места (идущий не
// останавливается пострелять).
//
// Вызывается из Tick после шага движения, с уже взятым мьютексом.
func (f *Factory) stepCombat(dt float64) {
	if f == nil || !(dt > 0) {
		return
	}
	var kills []string
	for id, entity := range f.entities {
		if !entity.Alive || entity.Weapon == nil || entity.Weapon.Range <= 0 {
			continue
		}
		order := entity.Order
		attackOrder := order != nil && entity.OrderStatus == OrderInProgress &&
			order.Attack && order.TargetID != ""
		if !attackOrder {
			if !entity.AutoAttack || entity.Velocity > 1e-6 {
				if entity.Attacking {
					entity.Attacking = false
					f.entities[id] = entity
				}
				continue
			}
			victimID := f.nearestVictim(entity)
			if victimID == "" {
				if entity.Attacking {
					entity.Attacking = false
					f.entities[id] = entity
				}
				continue
			}
			updated, killed := f.shootAt(entity, victimID, dt)
			entity = updated
			if killed {
				kills = append(kills, victimID)
			}
			f.entities[id] = entity
			continue
		}
		victim, ok := f.entities[order.TargetID]
		if !ok || !victim.Alive || victim.Dormant {
			// Цель уничтожена или спит: делать больше нечего.
			entity.OrderStatus = OrderDone
			entity.Attacking = false
			entity.Cooldown = 0
			f.entities[id] = entity
			continue
		}
		updated, killed := f.shootAt(entity, order.TargetID, dt)
		if killed {
			kills = append(kills, order.TargetID)
		}
		f.entities[id] = updated
	}
	for _, victimID := range kills {
		f.killLocked(victimID)
	}
}

// shootAt ведёт огонь бойца по цели: копит перезарядку и, когда она вышла,
// наносит урон по броне цели. Возвращает обновлённого бойца и признак того,
// что цель уничтожена — гибель применяется после обхода карты.
func (f *Factory) shootAt(entity Entity, victimID string, dt float64) (Entity, bool) {
	victim, ok := f.entities[victimID]
	if !ok || !victim.Alive {
		return entity, false
	}
	// Дистанция — до поверхности цели, а не до её центра: постройка
	// большая, и стрелять в её центр снаружи невозможно. Поверхность
	// берётся по боксу основания вдоль направления на цель.
	distance := victim.SurfaceGap(entity.Position.X, entity.Position.Y)
	if distance > entity.Weapon.Range {
		// Цель вышла из дистанции: огонь прекращается, боец идёт за ней
		// по приказу (см. stepMovement).
		entity.Attacking = false
		return entity, false
	}
	entity.Attacking = true
	entity.Facing = math.Atan2(victim.Position.Y-entity.Position.Y, victim.Position.X-entity.Position.X)
	if entity.Cooldown > 0 {
		entity.Cooldown -= dt
		if entity.Cooldown < 0 {
			entity.Cooldown = 0
		}
	}
	if entity.Cooldown > 0 {
		return entity, false
	}
	reload := entity.Weapon.ReloadTime
	if reload <= 0 {
		reload = 1
	}
	entity.Cooldown = reload
	entity.Shots++
	damage, hit := entity.Weapon.DamageHit(victim.Armour, victim.ArmourClass)
	// Сцена читает эти поля вместе со счётчиком: куда ушёл выстрел и
	// попал ли он. Промах не отменяет выстрел — эффект всё равно играет.
	entity.LastShotTarget = victimID
	entity.LastShotHit = hit
	if damage <= 0 {
		return entity, false
	}
	victim.Health.Current -= damage
	if victim.Health.Current <= 0 {
		victim.Health.Current = 0
		f.entities[victimID] = victim
		return entity, true
	}
	f.entities[victimID] = victim
	return entity, false
}

// nearestVictim — ближайший враг в дистанции оружия бойца: автоатака бьёт по
// первой цели, до которой дотягивается. Свои, нейтральные и мёртвые не
// рассматриваются.
func (f *Factory) nearestVictim(entity Entity) string {
	best, bestDistance := "", math.Inf(1)
	for id, other := range f.entities {
		if id == entity.ID || !other.Alive || other.Dormant {
			continue
		}
		if other.Owner == "" || other.Owner == NeutralPlayer || other.Owner == entity.Owner {
			continue
		}
		distance := other.SurfaceGap(entity.Position.X, entity.Position.Y)
		if distance > entity.Weapon.Range || distance >= bestDistance {
			continue
		}
		best, bestDistance = id, distance
	}
	return best
}

// stepRevival отсчитывает задержку до подъёма уничтоженных с шансом
// восстановления и возвращает их в строй с долей здоровья. Тело лежит и
// не выбирается, пока ReviveIn > 0; встать может только юнит.
func (f *Factory) stepRevival(dt float64) {
	for id, entity := range f.entities {
		if entity.Alive || entity.ReviveIn <= 0 {
			continue
		}
		entity.ReviveIn -= dt
		if entity.ReviveIn > 0 {
			f.entities[id] = entity
			continue
		}
		entity.ReviveIn = 0
		entity.Alive = true
		health := entity.Health.Max * entity.GetUpHealthPercent
		if health < 1 {
			health = 1
		}
		entity.Health.Current = health
		entity.Order = nil
		entity.OrderStatus = OrderNone
		entity.Attacking = false
		entity.Cooldown = 0
		entity.Velocity = 0
		f.entities[id] = entity
	}
}

// killLocked отмечает цель уничтоженной и закрывает приказы тех, кто её
// расстреливал. Труп остаётся в реестре (Alive=false): убрать модель со
// сцены — дело сцены, а логика мёртвого бойца не считает ни в движении, ни
// в бою, ни в экономике.
func (f *Factory) killLocked(victimID string) {
	victim, ok := f.entities[victimID]
	if !ok || !victim.Alive {
		return
	}
	victim.Health.Current = 0
	victim.Alive = false
	// Новая гибель — новое тело: прежняя отметка о сборке не должна
	// делать этот труп недоступным.
	victim.CorpseTaken = false
	victim.Attacking = false
	victim.Cooldown = 0
	victim.Velocity = 0
	victim.Capturing = false
	victim.CaptureProgress = 0
	victim.Order = nil
	// Некроны (и всякий, у кого health_ext даёт шанс) могут встать: тело
	// лежит GetUpTime секунд, затем поднимается с долей здоровья. Шанс
	// бросается один раз при смерти.
	if victim.Kind == KindUnit && victim.GetUpChance > 0 && rand.Float64() < victim.GetUpChance {
		victim.ReviveIn = victim.GetUpTime
	}
	if victim.OrderStatus == OrderInProgress {
		victim.OrderStatus = OrderFailed
	}
	f.entities[victimID] = victim
	// Все, кто вёл огонь по уничтоженной цели, остаются с выполненным
	// приказом: идти больше некуда.
	for id, other := range f.entities {
		if other.Order == nil || other.Order.TargetID != victimID {
			continue
		}
		if other.OrderStatus == OrderInProgress {
			other.OrderStatus = OrderDone
		}
		other.Attacking = false
		other.Cooldown = 0
		f.entities[id] = other
	}
}
