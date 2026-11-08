package entities

import "math"

// Экономика игрока: ресурсы копятся из того, что дают его сущности, а лимиты
// отрядов и техники считаются как «база расы + бонусы построек».
//
// Здесь только арифметика по уже известным данным: чертежи (resource_ext,
// модификаторы лимитов, racebps) читает пакет attrib, а фабрика складывает
// их по владельцам.

// Tick начисляет ресурсы за dt секунд: суммарный прирост всех сущностей
// игрока, ограниченный income_cap его расы. Нейтральные сущности ресурсов
// не дают — их доход достаётся тому, кто точку захватил.
//
// Здесь же двигаются сущности с приказами: шаг движения за dt секунд
// (см. stepMovement).
func (f *Factory) Tick(dt float64) {
	if f == nil || !(dt > 0) || math.IsNaN(dt) || math.IsInf(dt, 0) {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	f.stepMovement(dt)
	f.stepCapture(dt)
	f.stepSeparation(dt)
	f.stepCombat(dt)
	f.stepRevival(dt)

	rates := map[PlayerID]Income{}
	for _, entity := range f.entities {
		owner := entity.Owner
		if owner == "" || owner == NeutralPlayer {
			continue
		}
		// Уничтоженная или спящая (пробуждённая в юнита) постройка денег не
		// приносит.
		if !entity.Alive || entity.Dormant {
			continue
		}
		// Недостроенная постройка ещё не работает и ничего не даёт.
		if entity.UnderConstruction() {
			continue
		}
		rates[owner] = addIncome(rates[owner], entity.Income)
	}

	for id, player := range f.players {
		rate := limitIncome(rates[id], player.IncomeCap)
		player.Resources.Requisition += rate.Requisition * dt
		player.Resources.Power += rate.Power * dt
		player.Resources.Faith += rate.Faith * dt
		f.players[id] = player
	}
}

// Economy возвращает срез экономики игрока: кошелёк, текущий прирост и
// занятые лимиты с учётом бонусов построек. ok=false — игрока нет.
func (f *Factory) Economy(id PlayerID) (Economy, bool) {
	if f == nil {
		return Economy{}, false
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	player, ok := f.players[id]
	if !ok {
		return Economy{}, false
	}

	income := Income{}
	used := CapUsage{}
	bonus := CapBonus{}
	for _, entity := range f.entities {
		if entity.Owner != id || entity.UnderConstruction() || !entity.Alive {
			continue
		}
		income = addIncome(income, entity.Income)
		used.Squad += entity.CapUsage.Squad
		used.Support += entity.CapUsage.Support
		bonus.Squad += entity.CapBonus.Squad
		bonus.Support += entity.CapBonus.Support
	}
	return Economy{
		Resources:   player.Resources,
		Income:      limitIncome(income, player.IncomeCap),
		SquadUsed:   used.Squad,
		SquadMax:    player.SquadCapMax + bonus.Squad,
		SupportUsed: used.Support,
		SupportMax:  player.SupportCapMax + bonus.Support,
	}, true
}

// limitIncome ограничивает прирост потолком расы; нулевой потолок означает,
// что для этой валюты ограничения нет.
func limitIncome(rate, cap Income) Income {
	limit := func(value, ceiling float64) float64 {
		if ceiling > 0 && value > ceiling {
			return ceiling
		}
		if value < 0 {
			return 0
		}
		return value
	}
	return Income{
		Requisition: limit(rate.Requisition, cap.Requisition),
		Power:       limit(rate.Power, cap.Power),
		Faith:       limit(rate.Faith, cap.Faith),
	}
}

// addIncome складывает два прироста.
func addIncome(a, b Income) Income {
	return Income{
		Requisition: a.Requisition + b.Requisition,
		Power:       a.Power + b.Power,
		Faith:       a.Faith + b.Faith,
	}
}
