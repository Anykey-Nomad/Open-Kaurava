package scenes

import (
	"fmt"
	"github.com/Anykey-Nomad/Open-Kaurava/loc"
	"math"
	"strings"

	"github.com/Anykey-Nomad/Open-Kaurava/attrib"
	"github.com/Anykey-Nomad/Open-Kaurava/entities"
)

// Очередь найма постройки: один заказ нанимается (крупная клетка панели
// производства с полосой прогресса), следующие ждут. Время и цену дают
// чертежи: ebps бойца отряда хранит их сразу для всего отряда, см. trainInfo.
const (
	// trainQueueLimit — сколько заказов ждёт за текущим: панель показывает
	// шесть клеток ожидания, седьмой заказ туда не влезает.
	trainQueueLimit = 6
	// trainFallbackSeconds — время найма, если чертёж его не дал.
	trainFallbackSeconds = 10.0
)

// trainItem — один заказ найма.
type trainItem struct {
	// squadPath — чертёж отряда (sbps) из squad_table постройки.
	squadPath string
	label     string
	// icon — имя иконки из ui_info/icon_name (space_marine_icons/…).
	icon string
	// cost — цена найма по чертежу, seconds — его время производства.
	cost    attrib.Cost
	seconds float64
	// elapsed — сколько секунд уже отработано. Растёт только пока игра не
	// на паузе; приостановка держит прогресс на месте.
	elapsed float64
}

// ratio — доля готовности заказа от 0 до 1.
func (i trainItem) ratio() float64 {
	if i.seconds <= 0 {
		return 1
	}
	ratio := i.elapsed / i.seconds
	if ratio < 0 {
		return 0
	}
	if ratio > 1 {
		return 1
	}
	return ratio
}

// remaining — сколько секунд найма осталось.
func (i trainItem) remaining() float64 {
	if i.seconds <= 0 {
		return 0
	}
	left := i.seconds - i.elapsed
	if left < 0 {
		return 0
	}
	return left
}

// trainQueue — очередь одной постройки. items[0] нанимается, остальные
// (до trainQueueLimit) ждут.
type trainQueue struct {
	buildingID string
	items      []trainItem
}

// trainSquad ставит отряд в очередь найма постройки. Возвращает строку для
// панели приказов: что встало в очередь, за сколько секунд и по какой цене.
func (p *Gameplay) trainSquad(buildingID, squadPath string) string {
	if p.hud == nil || p.hud.attribData == nil || p.hud.factory == nil {
		return loc.T("Training unavailable")
	}
	building, ok := p.hud.factory.Get(buildingID)
	if !ok {
		return loc.T("Building not found")
	}
	if building.Owner != p.hud.ownPlayer {
		return loc.T("Only your own building can train")
	}
	if building.Dormant {
		return loc.T("The building is awake and cannot train")
	}
	if building.UnderConstruction() {
		return loc.T("The building is still under construction")
	}
	squad, ok := p.hud.attribData.squadByPath(squadPath)
	if !ok || len(squad.Troopers) == 0 {
		return loc.T("Squad not read: ") + fileNameOf(squadPath)
	}
	if p.queues == nil {
		p.queues = map[string]*trainQueue{}
	}
	queue := p.queues[buildingID]
	if queue == nil {
		queue = &trainQueue{buildingID: buildingID}
		p.queues[buildingID] = queue
	}
	if len(queue.items) > trainQueueLimit {
		return fmt.Sprintf(loc.T("Queue is full: %d orders"), trainQueueLimit)
	}
	cost, seconds, icon := p.hud.attribData.trainInfo(squad)
	label := p.hud.attribData.squadLabel(squadPath)
	queue.items = append(queue.items, trainItem{
		squadPath: squadPath, label: label, icon: icon,
		cost: cost, seconds: seconds,
	})
	return fmt.Sprintf(loc.T("Queued: %s — %s, %g s (orders %d)"), label, cost, seconds, len(queue.items))
}

// queueOf — очередь найма постройки для панели производства. Возвращается
// копия, чтобы HUD не мог изменить очередь при отрисовке.
func (p *Gameplay) queueOf(buildingID string) []trainItem {
	queue := p.queues[buildingID]
	if queue == nil {
		return nil
	}
	return append([]trainItem(nil), queue.items...)
}

// updateTraining двигает прогресс найма: время идёт только пока игра не на
// паузе (как и экономика). Готовый заказ выпускает бойцов отряда и уходит из
// очереди, следующий становится текущим.
func (p *Gameplay) updateTraining(dt float64) {
	if p.hud == nil || p.hud.factory == nil || p.hud.paused || !(dt > 0) {
		return
	}
	for id, queue := range p.queues {
		if _, ok := p.hud.factory.Get(id); !ok {
			// Постройку уничтожили — вместе с ней пропадает и очередь.
			delete(p.queues, id)
			continue
		}
		if len(queue.items) == 0 {
			delete(p.queues, id)
			continue
		}
		item := &queue.items[0]
		if item.seconds <= 0 {
			item.seconds = trainFallbackSeconds
		}
		item.elapsed += dt
		if item.elapsed < item.seconds {
			continue
		}
		p.deploySquad(queue)
		queue.items = queue.items[1:]
		if len(queue.items) == 0 {
			delete(p.queues, id)
		}
	}
}

// deploySquad выпускает бойцов готового заказа вокруг постройки: число
// бойцов и их чертежи задаёт squad_loadout_ext (unit_min, trooper_base).
// Модель каждого бойца читается в фоне (requestUnitModel).
func (p *Gameplay) deploySquad(queue *trainQueue) {
	if queue == nil || len(queue.items) == 0 || p.hud.attribData == nil {
		return
	}
	item := queue.items[0]
	squad, ok := p.hud.attribData.squadByPath(item.squadPath)
	if !ok || len(squad.Troopers) == 0 {
		p.hud.command = loc.T("Training finished, but the unit blueprint was not found: ") + item.label
		return
	}
	building, ok := p.hud.factory.Get(queue.buildingID)
	if !ok || building.Dormant {
		// Спящая постройка прогресс не двигает; очередь ждёт её
		// возвращения, как и подобает.
		return
	}
	count := squad.UnitMin
	if count < 1 {
		count = 1
	}
	if count > 8 {
		count = 8
	}
	// Бойцы выходят кольцом у постройки: радиус — по её основанию из
	// DATASEUI, чтобы не появляться внутри стен.
	half := [2]float64{6, 6}
	if blueprint := p.hud.blueprintOf(building); blueprint != "" {
		half, _ = p.hud.attribData.footprintFor(blueprint)
	}
	radius := math.Max(half[0], half[1]) + 3
	// Бойцы выходят строем перед постройкой, а не в одной точке: ряды
	// начинаются за её основанием и уходят от неё.
	squadKey := p.newSquadKey()
	rows := formationRows(count)
	depth := radius + 2 + formationSpacing*float64(max(len(rows)-1, 0))
	forward := [2]float64{0, 1}
	anchor := [2]float64{
		building.Position.X + forward[0]*depth,
		building.Position.Y + forward[1]*depth,
	}
	targets := formationTargets(count, anchor, forward, formationSpacing)
	// Пробуждение монолита: если монолит-постройка выпускает
	// восстановленного монолита, тот занимает её место, а сама постройка
	// уходит в «сон» до его гибели.
	awakenedFrom := ""
	if isAwakenedMonolithUnit(squad.Troopers[0]) && isMonolithStructure(p.hud.blueprintOf(building)) {
		awakenedFrom = building.ID
	}
	spawnedIDs := make([]string, 0, count)
	for i := 0; i < count; i++ {
		path := squad.Troopers[i%len(squad.Troopers)]
		unit, ok := p.hud.attribData.unitByPath(path)
		if !ok {
			continue
		}
		x, y := anchor[0], anchor[1]
		if i < len(targets) {
			x, y = targets[i][0], targets[i][1]
		}
		// Пробуждённый монолит встаёт на место своей постройки, а не в
		// строй перед ней: постройка и юнит — один и тот же объект.
		if awakenedFrom != "" {
			x, y = building.Position.X, building.Position.Y
		}
		// У лорда некронов и томб-спайдера на карте с начала лежит их тело:
		// найм всегда поднимает это тело на его месте с полным здоровьем, а
		// не ставит второго юнита рядом.
		if body := p.startingBody(building.Owner, path); body != nil {
			if entity, found := p.hud.factory.Get(body.id); found && p.reviveCorpse(body, entity, 1) {
				p.hud.blueprint[body.id] = path
				spawnedIDs = append(spawnedIDs, body.id)
				continue
			}
		}
		spec := entities.Spec{
			Name: item.label, Kind: entities.KindUnit,
			Owner: building.Owner, Selectable: true,
			SquadID:   squadKey,
			HitPoints: float64(unit.HitPoints),
			Speed:     float64(unit.Speed),
			AccelTime: float64(unit.AccelTime),
			BrakeTime: float64(unit.BrakeTime),
			// Бой: оружие и броня из чертежа бойца. Радиус у пехоты
			// небольшой — им и меряется, докуда подходить к цели.
			Weapon:      weaponFor(unit),
			ArmourClass: unit.ArmourClass,
			Armour:      unit.Armour,
			Radius:      1.5,
			// Восстановление после уничтожения — из health_ext: у некронов
			// есть шанс подняться, у остальных нули.
			GetUpChance:        unit.GetUpChance,
			GetUpHealthPercent: unit.GetUpHealthPercent,
			GetUpTime:          unit.GetUpTime,
			UsableBody:         unit.SpawnUsableBodyOnDeath != "",
			FromBuilding:       awakenedFrom,
			MoraleMax:          squad.MoraleMax,
		}
		// Лимит отрядов занимает отряд целиком, поэтому его ставит только
		// первый боец — иначе отряд занял бы лимит столько раз, сколько бойцов.
		if i == 0 {
			spec.CapUsage = entities.CapUsage{Squad: squad.SquadCap, Support: squad.SupportCap}
		}
		spawned, err := p.hud.factory.Spawn(spec, entities.Position{X: x, Y: y})
		if err != nil {
			continue
		}
		p.hud.blueprint[spawned.ID] = path
		p.requestUnitModel(spawned.ID, path, x, y, p.groundHeight(x, y))
		spawnedIDs = append(spawnedIDs, spawned.ID)
	}
	if len(spawnedIDs) == 0 {
		p.hud.command = loc.T("Training finished, but no units appeared: ") + item.label
		return
	}
	if awakenedFrom != "" {
		_ = p.hud.factory.SetDormant(awakenedFrom, true)
	}
	// Отряд запоминается целиком: клик по любому бойцу выделяет всех.
	p.registerSquad(squadKey, spawnedIDs)
	p.hud.command = fmt.Sprintf(loc.T("Training finished: %s (%d units)"), item.label, len(spawnedIDs))
}

// blueprintKey приводит путь чертежа к нижнему регистру с '/' — так проще
// сравнивать имена из разных источников (SGB, squad_table, меню).
func blueprintKey(name string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(name)), `\`, "/")
}

// isMonolithStructure — чертёж монолита-постройки. Имя восстановленного
// монолита содержит то же слово, поэтому он отдельным случаем.
func isMonolithStructure(name string) bool {
	key := blueprintKey(name)
	return strings.Contains(key, "monolith") && !strings.Contains(key, "restored_monolith")
}

// isAwakenedMonolithUnit — чертёж восстановленного монолита: только он,
// выйдя из постройки, возвращается в неё после гибели.
func isAwakenedMonolithUnit(name string) bool {
	return strings.Contains(blueprintKey(name), "restored_monolith")
}

// registerSquad запоминает состав нанятого отряда: из него выделение берёт
// всех бойцов по клику на одного.
func (p *Gameplay) registerSquad(key string, ids []string) {
	if len(ids) == 0 || key == "" {
		return
	}
	if p.squads == nil {
		p.squads = map[string][]string{}
	}
	if p.entitySquad == nil {
		p.entitySquad = map[string]string{}
	}
	p.squads[key] = append([]string(nil), ids...)
	for _, id := range ids {
		p.entitySquad[id] = key
	}
}

// newSquadKey — новый ключ отряда: его получают и сущности (Spec.SquadID),
// и карта состава сцены.
func (p *Gameplay) newSquadKey() string {
	p.nextSquad++
	return fmt.Sprintf("squad-%d", p.nextSquad)
}

// squadMembers — бойцы отряда, в котором состоит юнит. Пусто — юнит один
// (постройка, боец из сценария без отряда).
func (p *Gameplay) squadMembers(id string) []string {
	if p.entitySquad == nil {
		return nil
	}
	return p.squads[p.entitySquad[id]]
}
