package scenes

import (
	"fmt"
	"github.com/Anykey-Nomad/Open-Kaurava/loc"
	"math"
	"strings"

	"github.com/Anykey-Nomad/Open-Kaurava/attrib"
	"github.com/Anykey-Nomad/Open-Kaurava/entities"
)

// Трупы некронов — не просто украшение карты. Тело, у которого в
// health_ext есть spawn_usable_body_on_death (воины, бессмертные,
// ободранные, призраки), остаётся лежать, пока его не заберут:
//
//   - томб-спайдер собирает тела и по кнопке призыва обменивает их на отряд
//     (harvest_ext: воины 5 тел, ободранные 5, бессмертные 6);
//   - лорд некронов способностью массового воскрешения поднимает все тела в
//     радиусе (area_effect, 26 м) с долей здоровья из её модификатора
//     health_resurrect_modifier.
//
// Собранное тело пропадает с карты и больше не достаётся ни другому
// спайдеру, ни воскрешению: entities.CorpseTaken помнит, что оно уже
// использовано.

// harvestState — что собрал томб-спайдер: сколько тел каждого чертежа, к
// какому телу он идёт и сколько уже подбирает текущее.
type harvestState struct {
	bodies map[string]int
	target string
	timer  float64
	// retry — пауза после неудачного поиска пути: тело за стеной не должно
	// заставлять искать маршрут каждый кадр.
	retry float64
}

// total — сколько тел спайдер держит всего (body_count_limit).
func (s *harvestState) total() int {
	if s == nil {
		return 0
	}
	sum := 0
	for _, count := range s.bodies {
		sum += count
	}
	return sum
}

// harvestOf — состояние сбора спайдера: заводится при первом обращении.
func (p *Gameplay) harvestOf(id string) *harvestState {
	if p.harvest == nil {
		p.harvest = map[string]*harvestState{}
	}
	state := p.harvest[id]
	if state == nil {
		state = &harvestState{bodies: map[string]int{}}
		p.harvest[id] = state
	}
	if state.bodies == nil {
		state.bodies = map[string]int{}
	}
	return state
}

// harvestCounts — сколько тел каждого чертежа собрал спайдер; HUD берёт
// копию, чтобы подписи кнопок призыва показывали счёт.
func (p *Gameplay) harvestCounts(id string) map[string]int {
	state := p.harvest[id]
	if state == nil {
		return map[string]int{}
	}
	out := make(map[string]int, len(state.bodies))
	for name, count := range state.bodies {
		out[name] = count
	}
	return out
}

// corpseName — имя чертежа тела для счётчиков сбора: нижний регистр, без
// пути и расширения ("necron_basic_warrior").
func corpseName(blueprint string) string {
	return strings.ToLower(fileNameOf(blueprint))
}

// corpseAvailable — тело на карте, которое ещё можно забрать: мёртвый юнит
// с пригодным телом, которое не собрали и не подняли.
func (p *Gameplay) corpseAvailable(entity entities.Entity) bool {
	return entity.Kind == entities.KindUnit && !entity.Alive &&
		entity.UsableBody && !entity.CorpseTaken
}

// startingBody — мёртвое тело того же чертежа у того же игрока. Его
// занимает найм лорда некронов и томб-спайдера: тело на карте — это они и
// есть, новый юнит рядом не появляется.
func (p *Gameplay) startingBody(owner entities.PlayerID, blueprint string) *gameplayObject {
	if p.hud == nil || p.hud.factory == nil {
		return nil
	}
	want := corpseName(blueprint)
	for i := range p.objects {
		o := &p.objects[i]
		if o.id == "" || o.kind != entities.KindUnit || o.owner != owner {
			continue
		}
		entity, ok := p.hud.factory.Get(o.id)
		if !ok || entity.Alive || entity.CorpseTaken {
			continue
		}
		if corpseName(p.hud.blueprintOf(entity)) != want {
			continue
		}
		return o
	}
	return nil
}

// updateHarvest ведёт сбор тел всеми томб-спайдерами. Пауза сцены
// останавливает и его: тело не должно исчезать, пока игра стоит.
func (p *Gameplay) updateHarvest(dt float64) {
	if p.hud == nil || p.hud.factory == nil || p.hud.attribData == nil {
		return
	}
	if p.hud.paused {
		dt = 0
	}
	for i := range p.objects {
		o := &p.objects[i]
		if o.id == "" || o.kind != entities.KindUnit {
			continue
		}
		spider, ok := p.hud.factory.Get(o.id)
		if !ok || !spider.Alive {
			continue
		}
		unit, ok := p.hud.attribData.unitFor(p.hud.blueprintOf(spider))
		if !ok || unit.Harvest == nil {
			continue
		}
		p.stepHarvest(spider, unit.Harvest, dt)
	}
}

// stepHarvest — один шаг сбора для одного спайдера: выбрать тело, подойти,
// подобрать за pick_up_duration секунд.
func (p *Gameplay) stepHarvest(spider entities.Entity, harvest *attrib.Harvest, dt float64) {
	state := p.harvestOf(spider.ID)
	// Предел собранных тел: лишние не берём, пока не призовут отряд.
	if harvest.BodyLimit > 0 && state.total() >= harvest.BodyLimit {
		state.target, state.timer = "", 0
		return
	}
	corpse, body, ok := p.harvestTarget(spider, harvest)
	if !ok {
		state.target, state.timer = "", 0
		return
	}
	if state.target != body.ID {
		state.target, state.timer = body.ID, 0
	}
	distance := math.Hypot(body.Position.X-spider.Position.X, body.Position.Y-spider.Position.Y)
	if distance > harvest.PickupRadius {
		// К телу спайдер идёт сам, но только когда стоит без приказа:
		// приказ игрока всегда важнее, а после него сбор продолжится.
		if state.retry > 0 {
			if dt > 0 {
				state.retry -= dt
			}
			return
		}
		if spider.Order == nil || spider.OrderStatus != entities.OrderInProgress {
			order := entities.Order{
				Target: entities.Position{X: body.Position.X, Y: body.Position.Y},
				Stop:   0.8,
			}
			if p.attachPath(&order, spider) {
				_ = p.hud.factory.Move(spider.ID, order)
			} else {
				state.retry = 1
			}
		}
		return
	}
	if dt <= 0 {
		return
	}
	state.timer += dt
	if state.timer < harvest.PickupDuration {
		return
	}
	if err := p.hud.factory.TakeCorpse(body.ID); err != nil {
		state.target, state.timer = "", 0
		return
	}
	// Тело уходит с карты целиком: труп больше не увидеть ни на земле, ни в
	// списке целей — его собрали.
	p.setObjectAlpha(corpse, 1)
	p.setObjectVisible(corpse, false)
	corpse.selectable = false
	state.bodies[corpseName(p.hud.blueprintOf(body))]++
	state.target, state.timer = "", 0
}

// harvestTarget — ближайшее свободное тело в search_radius_limit. Тело,
// которое уже подбирает другой спайдер, не трогаем: иначе двое гнались бы
// за одной и той же добычей.
func (p *Gameplay) harvestTarget(spider entities.Entity, harvest *attrib.Harvest) (*gameplayObject, entities.Entity, bool) {
	var bestObj *gameplayObject
	var best entities.Entity
	bestDistance := math.Inf(1)
	for i := range p.objects {
		o := &p.objects[i]
		if o.id == "" || o.kind != entities.KindUnit {
			continue
		}
		entity, ok := p.hud.factory.Get(o.id)
		if !ok || !p.corpseAvailable(entity) {
			continue
		}
		if p.corpseClaimed(entity.ID, spider.ID) {
			continue
		}
		distance := math.Hypot(o.center[0]-spider.Position.X, o.center[1]-spider.Position.Y)
		if distance > harvest.SearchRadius || distance >= bestDistance {
			continue
		}
		bestObj, best, bestDistance = o, entity, distance
	}
	if bestObj == nil || harvest.BodyLimit > 0 && p.harvestOf(spider.ID).total() >= harvest.BodyLimit {
		return nil, entities.Entity{}, false
	}
	return bestObj, best, true
}

// corpseClaimed — тело уже выбрано другим спайдером.
func (p *Gameplay) corpseClaimed(corpseID, except string) bool {
	for id, state := range p.harvest {
		if id == except || state == nil {
			continue
		}
		if state.target == corpseID {
			return true
		}
	}
	return false
}

// summonAt — кнопка призыва томб-спайдера: отряд из собранных тел появляется
// вокруг спайдера, а нужное число тел списывается.
func (p *Gameplay) summonAt(unitID, slot string) string {
	if p.hud == nil || p.hud.factory == nil || p.hud.attribData == nil {
		return loc.T("Summon unavailable")
	}
	spider, ok := p.hud.factory.Get(unitID)
	if !ok || !spider.Alive {
		return loc.T("Summon: spider unavailable")
	}
	unit, ok := p.hud.attribData.unitFor(p.hud.blueprintOf(spider))
	if !ok || unit.Harvest == nil {
		return loc.T("Summon: unit has no corpse harvest")
	}
	var chosen *attrib.HarvestSlot
	for i := range unit.Harvest.Slots {
		if strings.EqualFold(unit.Harvest.Slots[i].Slot, slot) {
			chosen = &unit.Harvest.Slots[i]
			break
		}
	}
	if chosen == nil {
		return loc.T("Summon: no such squad")
	}
	state := p.harvestOf(unitID)
	key := strings.ToLower(chosen.Entity)
	label := key
	squad, ok := p.hud.attribData.squadByName(chosen.Squad)
	if !ok {
		return loc.T("Summon: squad not found: ") + chosen.Squad
	}
	label = p.hud.attribData.squadTitle(squad)
	have := state.bodies[key]
	if have < chosen.Bodies {
		return fmt.Sprintf(loc.T("Summon: corpses for %s — %d/%d"), label, have, chosen.Bodies)
	}
	spawned := p.spawnSquadAt(spider.Owner, squad, spider.Position, label)
	if len(spawned) == 0 {
		return loc.T("Summon: squad did not appear: ") + label
	}
	state.bodies[key] -= chosen.Bodies
	if state.bodies[key] <= 0 {
		delete(state.bodies, key)
	}
	return fmt.Sprintf(loc.T("Squad summoned: %s (%d)"), label, len(spawned))
}

// spawnSquadAt выпускает отряд вокруг точки: так томб-спайдер призывает
// собранных некронов рядом с собой, а не у постройки.
func (p *Gameplay) spawnSquadAt(owner entities.PlayerID, squad attrib.Squad, center entities.Position, label string) []string {
	if p.hud == nil || p.hud.factory == nil || p.hud.attribData == nil || len(squad.Troopers) == 0 {
		return nil
	}
	count := squad.UnitMin
	if count < 1 {
		count = 1
	}
	if count > 8 {
		count = 8
	}
	key := p.newSquadKey()
	ids := make([]string, 0, count)
	for i := 0; i < count; i++ {
		path := squad.Troopers[i%len(squad.Troopers)]
		unit, ok := p.hud.attribData.unitByPath(path)
		if !ok {
			continue
		}
		angle := 2 * math.Pi * float64(i) / float64(count)
		x := center.X + math.Cos(angle)*4.5
		y := center.Y + math.Sin(angle)*4.5
		spec := entities.Spec{
			Name: label, Kind: entities.KindUnit,
			Owner: owner, Selectable: true,
			SquadID:            key,
			HitPoints:          float64(unit.HitPoints),
			Speed:              float64(unit.Speed),
			AccelTime:          float64(unit.AccelTime),
			BrakeTime:          float64(unit.BrakeTime),
			Weapon:             weaponFor(unit),
			ArmourClass:        unit.ArmourClass,
			Armour:             unit.Armour,
			Radius:             1.5,
			GetUpChance:        unit.GetUpChance,
			GetUpHealthPercent: unit.GetUpHealthPercent,
			GetUpTime:          unit.GetUpTime,
			UsableBody:         unit.SpawnUsableBodyOnDeath != "",
			MoraleMax:          squad.MoraleMax,
		}
		spawned, err := p.hud.factory.Spawn(spec, entities.Position{X: x, Y: y})
		if err != nil {
			continue
		}
		p.hud.blueprint[spawned.ID] = path
		p.requestUnitModel(spawned.ID, path, x, y, p.groundHeight(x, y))
		ids = append(ids, spawned.ID)
	}
	if len(ids) == 0 {
		return nil
	}
	p.registerSquad(key, ids)
	return ids
}

// updateAbilityCooldowns отсчитывает перезарядку способностей; на паузе она
// стоит вместе с игрой.
func (p *Gameplay) updateAbilityCooldowns(dt float64) {
	if len(p.cooldowns) == 0 || (p.hud != nil && p.hud.paused) {
		return
	}
	for key, left := range p.cooldowns {
		left -= dt
		if left <= 0 {
			delete(p.cooldowns, key)
			continue
		}
		p.cooldowns[key] = left
	}
}

// resurrectAt — кнопка воскрешения лорда: поднимает все пригодные тела
// своих юнитов в радиусе способности. Тела те же самые — новых юнитов не
// появляется, а здоровье берётся из модификатора способности.
func (p *Gameplay) resurrectAt(unitID, abilityPath string) string {
	if p.hud == nil || p.hud.factory == nil || p.hud.attribData == nil {
		return loc.T("Resurrection unavailable")
	}
	caster, ok := p.hud.factory.Get(unitID)
	if !ok || !caster.Alive {
		return loc.T("Resurrection: unit unavailable")
	}
	ability, ok := p.hud.attribData.abilityFor(abilityPath)
	if !ok || !ability.Resurrects() {
		return loc.T("Resurrection: ability not found")
	}
	key := unitID + "|" + abilityPath
	if left := p.cooldowns[key]; left > 0 {
		return fmt.Sprintf(loc.T("Resurrection: cooldown %.0f s"), math.Ceil(left))
	}
	radius := ability.Radius
	if radius <= 0 {
		radius = 26
	}
	raised := 0
	for i := range p.objects {
		o := &p.objects[i]
		if o.id == "" || o.kind != entities.KindUnit {
			continue
		}
		entity, ok := p.hud.factory.Get(o.id)
		if !ok || !p.corpseAvailable(entity) || entity.Owner != caster.Owner {
			continue
		}
		distance := math.Hypot(o.center[0]-caster.Position.X, o.center[1]-caster.Position.Y)
		if distance > radius {
			continue
		}
		if p.reviveCorpse(o, entity, ability.ResurrectHealth) {
			raised++
		}
	}
	if raised == 0 {
		return loc.T("Resurrection: no corpses in range")
	}
	if ability.RechargeTime > 0 {
		if p.cooldowns == nil {
			p.cooldowns = map[string]float64{}
		}
		p.cooldowns[key] = ability.RechargeTime
	}
	return fmt.Sprintf(loc.T("Resurrection: corpses raised — %d"), raised)
}

// reviveCorpse поднимает тело на том же месте: сущность оживает, а модель
// читается заново — запечённая поза смерти для живого бойца не годится,
// поэтому старый меш скрывается, а новый приходит через updateUnitModels.
func (p *Gameplay) reviveCorpse(o *gameplayObject, entity entities.Entity, health float64) bool {
	if p.hud == nil || p.hud.factory == nil || o == nil {
		return false
	}
	revived, err := p.hud.factory.Revive(entity.ID, health)
	if err != nil {
		return false
	}
	blueprint := p.hud.blueprintOf(revived)
	if p.world != nil && o.meshName != "" {
		p.world.SetObjectAlpha(o.meshName, 1)
		p.world.SetVisible(o.meshName, false)
	}
	// Имя меша снимается до прихода новой модели: иначе syncMovedObjects
	// каждый кадр показывал бы старый труп как живого юнита.
	o.meshName = ""
	o.parts = nil
	o.hidden = true
	o.deathAge = 0
	o.moveOffset = [3]float64{}
	o.selectable = revived.Selectable
	if blueprint != "" {
		p.requestUnitModel(revived.ID, blueprint, o.center[0], o.center[1], p.groundHeight(o.center[0], o.center[1]))
	}
	return true
}
