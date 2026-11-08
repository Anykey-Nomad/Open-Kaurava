package scenes

import (
	"fmt"
	"github.com/Anykey-Nomad/Open-Kaurava/loc"
	"math"

	"github.com/Anykey-Nomad/Open-Kaurava/attrib"
	"github.com/Anykey-Nomad/Open-Kaurava/entities"
)

// Пополнение отряда — кнопка Reinforce из gamescreen.screen.
//
// В DoW отряд после потерь добирает бойцов: squad_reinforce_ext говорит,
// можно ли это (can_upgrade), сколько стоит один боец (cost) и сколько он
// идёт (cost/time). Здесь это перенесено так же: нажатие ставит одного бойца
// в пополнение, время берётся из чертежа, готовый боец встаёт рядом с
// отрядом и получает его SquadID — то есть сразу считается частью отряда
// для выделения, приказов и расталкивания.
//
// Пока идёт пополнение, второе нажатие отклоняется; отряд, добравшийся до
// squad_loadout_ext/unit_max, дальше не пополняется.

// reinforceTask — идущее пополнение отряда. Ключ — SquadID отряда, поэтому
// в отряде одновременно пополняется только один боец.
type reinforceTask struct {
	// squadKey — отряд, memberID — боец, с которого нажали кнопку (от него
	// берутся владелец, место и чертёж).
	squadKey string
	memberID string
	// path и label — чертёж нового бойца и подпись отряда для сообщений.
	path  string
	label string
	// elapsed, seconds — сколько идёт пополнение и сколько ему отведено.
	elapsed, seconds float64
}

// reinforceAt — начать пополнение отряда, в котором состоит боец. Возвращает
// строку для панели: что происходит и почему нельзя.
func (p *Gameplay) reinforceAt(memberID string) string {
	if p.hud == nil || p.hud.attribData == nil || p.hud.factory == nil {
		return loc.T("Reinforce unavailable")
	}
	member, ok := p.hud.factory.Get(memberID)
	if !ok || !member.Alive {
		return loc.T("Unit not found")
	}
	if member.Owner != p.hud.ownPlayer {
		return loc.T("You can only reinforce your own squad")
	}
	if member.Kind != entities.KindUnit || member.SquadID == "" {
		return loc.T("This unit is not in a squad")
	}
	squad, ok := p.hud.attribData.squadFor(p.hud.blueprint[memberID])
	if !ok || len(squad.Troopers) == 0 {
		return loc.T("Squad blueprint not read")
	}
	if !squad.CanReinforce {
		return loc.T("This squad cannot be reinforced")
	}
	if p.reinforce[member.SquadID] != nil {
		return loc.T("Reinforcement already in progress")
	}
	alive := p.aliveSquadMembers(member.SquadID)
	if squad.UnitMax > 0 && alive >= squad.UnitMax {
		return fmt.Sprintf(loc.T("Squad is full: %d/%d"), alive, squad.UnitMax)
	}
	// Новый боец — первый чертёж отряда: остальные (если их несколько)
	// попадают в отряд очереди найма, а не в пополнение.
	path := squad.Troopers[0]
	if _, ok := p.hud.attribData.unitByPath(path); !ok {
		return loc.T("Unit blueprint not found: ") + fileNameOf(path)
	}
	label := p.hud.attribData.squadLabel(squad.Path)
	if label == "" {
		label = member.Name
	}
	if p.reinforce == nil {
		p.reinforce = map[string]*reinforceTask{}
	}
	seconds := float64(squad.ReinforceTime)
	if seconds <= 0 {
		seconds = 1
	}
	p.reinforce[member.SquadID] = &reinforceTask{
		squadKey: member.SquadID,
		memberID: memberID,
		path:     path,
		label:    label,
		seconds:  seconds,
	}
	return fmt.Sprintf(loc.T("Reinforce: %s — %s, %g s"), label, squad.ReinforceCost, seconds)
}

// reinforceState — прогресс пополнения отряда для панели: сколько прошло,
// сколько всего и идёт ли оно.
func (p *Gameplay) reinforceState(squadKey string) (elapsed, seconds float64, active bool) {
	task := p.reinforce[squadKey]
	if task == nil {
		return 0, 0, false
	}
	return task.elapsed, task.seconds, true
}

// updateReinforce двигает пополнение: время идёт только пока игра не на
// паузе, готовый боец встаёт рядом с отрядом.
func (p *Gameplay) updateReinforce(dt float64) {
	if p.hud == nil || p.hud.factory == nil || p.hud.paused || !(dt > 0) || len(p.reinforce) == 0 {
		return
	}
	for key, task := range p.reinforce {
		task.elapsed += dt
		if task.elapsed < task.seconds {
			continue
		}
		if !p.deployReinforcement(task) {
			// Отряд уничтожили, пока шло пополнение, — заказ пропадает.
			delete(p.reinforce, key)
			continue
		}
		delete(p.reinforce, key)
	}
}

// deployReinforcement выпускает одного бойца к отряду: место — свободная
// точка на кольце вокруг живых бойцов, отряд — тот же SquadID.
func (p *Gameplay) deployReinforcement(task *reinforceTask) bool {
	if task == nil || p.hud == nil || p.hud.factory == nil || p.hud.attribData == nil {
		return false
	}
	squad, ok := p.hud.attribData.squadFor(p.hud.blueprint[task.memberID])
	if !ok {
		squad = attrib.Squad{}
	}
	unit, ok := p.hud.attribData.unitByPath(task.path)
	if !ok {
		return false
	}
	members := p.squadMembers(task.memberID)
	anchor, count := p.squadAnchor(members)
	if count == 0 {
		// Всех бойцов отряда уже нет — пополнять некого.
		return false
	}
	// Точка на кольце: угол зависит от числа живых бойцов, чтобы новые
	// вставали с разных сторон, а не друг в друга.
	angle := float64(count) * 2.399963 // золотое сечение: точки не липнут
	radius := entities.SquadSpacing + 1
	x := anchor[0] + math.Cos(angle)*radius
	y := anchor[1] + math.Sin(angle)*radius
	if !p.passableAt(x, y) {
		x, y = anchor[0], anchor[1]
	}
	spec := entities.Spec{
		Name: task.label, Kind: entities.KindUnit,
		Owner: p.hud.ownPlayer, Selectable: true,
		SquadID:     task.squadKey,
		HitPoints:   float64(unit.HitPoints),
		Speed:       float64(unit.Speed),
		AccelTime:   float64(unit.AccelTime),
		BrakeTime:   float64(unit.BrakeTime),
		Weapon:      weaponFor(unit),
		ArmourClass: unit.ArmourClass,
		Armour:      unit.Armour,
		Radius:      1.5,
		// Восстановление после уничтожения и тело для сбора — из чертежа.
		GetUpChance:        unit.GetUpChance,
		GetUpHealthPercent: unit.GetUpHealthPercent,
		GetUpTime:          unit.GetUpTime,
		UsableBody:         unit.SpawnUsableBodyOnDeath != "",
		MoraleMax:          squad.MoraleMax,
	}
	spawned, err := p.hud.factory.Spawn(spec, entities.Position{X: x, Y: y})
	if err != nil {
		return false
	}
	p.hud.blueprint[spawned.ID] = task.path
	p.requestUnitModel(spawned.ID, task.path, x, y, p.groundHeight(x, y))
	// Состав отряда помнит нового бойца: клик по любому выделяет всех.
	key := task.squadKey
	p.squads[key] = append(p.squads[key], spawned.ID)
	if p.entitySquad == nil {
		p.entitySquad = map[string]string{}
	}
	p.entitySquad[spawned.ID] = key
	p.hud.command = fmt.Sprintf(loc.T("Reinforce: %s (%d units)"), task.label, p.aliveSquadMembers(key))
	return true
}

// aliveSquadMembers — сколько бойцов отряда ещё живо.
func (p *Gameplay) aliveSquadMembers(squadKey string) int {
	if p.hud == nil || p.hud.factory == nil || squadKey == "" {
		return 0
	}
	alive := 0
	for _, id := range p.squads[squadKey] {
		if entity, ok := p.hud.factory.Get(id); ok && entity.Alive {
			alive++
		}
	}
	return alive
}

// squadAnchor — средняя точка живых бойцов отряда и их число: к ней
// подходит пополнение.
func (p *Gameplay) squadAnchor(members []string) ([2]float64, int) {
	var x, y float64
	count := 0
	for _, id := range members {
		entity, ok := p.hud.factory.Get(id)
		if !ok || !entity.Alive {
			continue
		}
		x += entity.Position.X
		y += entity.Position.Y
		count++
	}
	if count == 0 {
		return [2]float64{}, 0
	}
	return [2]float64{x / float64(count), y / float64(count)}, count
}

// passableAt — можно ли поставить бойца в точку: по DATAIMAP, как у приказов
// движения.
func (p *Gameplay) passableAt(x, y float64) bool {
	if p.pass == nil {
		return true
	}
	return p.pass.Passable(x, y)
}
