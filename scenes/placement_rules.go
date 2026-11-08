package scenes

import (
	"github.com/Anykey-Nomad/Open-Kaurava/loc"
	"math"
	"strings"

	"github.com/Anykey-Nomad/Open-Kaurava/entities"
)

// Постановка построек на особые места. Чертёж постройки говорит об этом сам:
// structure_ext/attach_to/attach_to_tp — тип местности, к которому она
// крепится (у listening post — tp_strategic_point_struct, у термогенератора —
// tp_slag), а active в той же записи означает, что место обязательно. Тип
// местности самой точки лежит в её type_ext/type_environment.
//
// Поставленная постройка занимает точку: второй такой на ней не будет, а сама
// точка, пока постройка цела, не выбирается и не захватывается.

// Типы местности, к которым крепятся особые постройки.
const (
	attachStrategicPoint = "tp_strategic_point_struct"
	attachSlag           = "tp_slag"
)

// pointKind — вид точки захвата по имени чертежа. Нужен, когда данных чертежа
// нет (нет дерева модуля): по нему восстанавливается тип местности.
type pointKind int

const (
	pointNone pointKind = iota
	pointStrategic
	pointObjective
	pointRelic
	pointSlag
)

// pointKindOf — вид точки по имени файла чертежа.
func pointKindOf(blueprint string) pointKind {
	name := strings.ToLower(blueprint)
	switch {
	case strings.Contains(name, "strategic_objective"):
		return pointObjective
	case strings.Contains(name, "strategic_point"):
		return pointStrategic
	case strings.Contains(name, "relic"):
		return pointRelic
	case strings.Contains(name, "slag_deposit"):
		return pointSlag
	}
	return pointNone
}

// attachTypeFor — к какому типу местности крепится постройка. Пусто — место
// любое. Если чертёж не прочитан, работает запасное правило по имени файла.
func (p *Gameplay) attachTypeFor(blueprint string) string {
	if unit, ok := p.hud.unitFor(blueprint); ok {
		return unit.AttachTo
	}
	name := strings.ToLower(blueprint)
	switch {
	case strings.Contains(name, "listening_post"):
		return attachStrategicPoint
	case strings.Contains(name, "thermo"):
		return attachSlag
	}
	return ""
}

// pointAttachType — тип местности точки захвата и признак «цель сценария».
// Без данных чертежа тип восстанавливается по имени файла.
func (p *Gameplay) pointAttachType(entity entities.Entity) (string, bool) {
	blueprint := p.hud.blueprintOf(entity)
	if unit, ok := p.hud.unitFor(blueprint); ok {
		return unit.TypeEnvironment, unit.StrategicObjective
	}
	switch pointKindOf(blueprint) {
	case pointSlag:
		return attachSlag, false
	case pointRelic, pointStrategic:
		return attachStrategicPoint, false
	case pointObjective:
		// Цель сценария — та же стратегическая местность, но постом не
		// занимается.
		return attachStrategicPoint, true
	}
	return "", false
}

// capturePointAt — точка захвата под основанием постройки: ближайшая в
// пределах своего радиуса (но не меньше четырёх метров, иначе попасть в неё
// мышью почти невозможно).
func (p *Gameplay) capturePointAt(at [2]float64) string {
	if p.hud == nil || p.hud.factory == nil {
		return ""
	}
	best, bestDist := "", math.Inf(1)
	for _, object := range p.objects {
		if object.id == "" {
			continue
		}
		entity, ok := p.hud.factory.Get(object.id)
		if !ok || !entity.Capturable {
			continue
		}
		distance := math.Hypot(at[0]-object.center[0], at[1]-object.center[1])
		tolerance := object.radius
		if tolerance < 4 {
			tolerance = 4
		}
		if distance <= tolerance && distance < bestDist {
			best, bestDist = object.id, distance
		}
	}
	return best
}

// placedOn — ID точек, на которых уже стоит живая постройка.
func (p *Gameplay) placedOn() map[string]bool {
	busy := map[string]bool{}
	if p.hud == nil || p.hud.factory == nil {
		return busy
	}
	for _, entity := range p.hud.factory.All() {
		if entity.BuiltOn != "" && entity.Alive {
			busy[entity.BuiltOn] = true
		}
	}
	return busy
}

// snapRadius — насколько далеко от курсора ищется точка под особую постройку.
const snapRadius = 60.0

// snapToPoint — ближайшая подходящая точка для особой постройки в пределах
// snapRadius: тип местности тот же, цель сценария не занимается, для поста
// точка своя, такой же постройки на ней нет. Нет подходящих — курсор остаётся
// на месте и обычная проверка скажет, чего не хватает.
func (p *Gameplay) snapToPoint(at [2]float64, blueprint string) ([2]float64, string) {
	if p.attachTypeFor(blueprint) == "" || p.hud == nil || p.hud.factory == nil {
		return at, ""
	}
	best, bestDist, bestID := [2]float64{}, math.Inf(1), ""
	for _, object := range p.objects {
		entity, ok := p.hud.factory.Get(object.id)
		if !ok || !entity.Capturable || !entity.Alive || entity.Dormant {
			continue
		}
		center := [2]float64{object.center[0], object.center[1]}
		distance := math.Hypot(at[0]-center[0], at[1]-center[1])
		if distance > snapRadius || distance >= bestDist {
			continue
		}
		if _, _, suitable := p.buildSpotAt(center, blueprint); !suitable {
			continue
		}
		best, bestDist, bestID = center, distance, object.id
	}
	if bestID == "" {
		return at, ""
	}
	return best, bestID
}

// placementCheck — можно ли ставить чертёж с основанием в at. У постройки с
// attach_to своё место, и занятость окружения её не касается: пост слуха и
// термогенератор встают на точку, даже если на ней стоят войска или рядом
// другие постройки. Всем остальным нужна свободная площадка, и только потом
// проверяется своё место (если оно есть).
func (p *Gameplay) placementCheck(at [2]float64, blueprint string, half [2]float64) (point, blocker string, ok bool) {
	if p.attachTypeFor(blueprint) != "" {
		return p.buildSpotAt(at, blueprint)
	}
	blocker, ok = p.placementFree(at, half)
	if !ok {
		return "", blocker, false
	}
	return p.buildSpotAt(at, blueprint)
}

// pointBuiltWith — на точке уже стоит постройка того же вида (свой или чужой
// термогенератор, чужой пост): второй такой на неё не встаёт. Постройки
// другого вида особым постройкам не мешают.
func (p *Gameplay) pointBuiltWith(pointID, attach string) bool {
	if pointID == "" || p.hud == nil || p.hud.factory == nil {
		return false
	}
	for _, entity := range p.hud.factory.All() {
		if !entity.Alive || entity.BuiltOn != pointID {
			continue
		}
		blueprint := p.hud.blueprintOf(entity)
		// Неизвестный чертёж считается своим: лучше отказать, чем поставить
		// второй генератор вслепую.
		if blueprint == "" || p.attachTypeFor(blueprint) == attach {
			return true
		}
	}
	return false
}

// pointBusy — на точке уже стоит постройка: её нельзя ни застроить второй
// раз, ни выбрать, ни захватить, пока первая цела.
func (p *Gameplay) pointBusy(pointID string) bool {
	if pointID == "" || p.hud == nil || p.hud.factory == nil {
		return false
	}
	for _, entity := range p.hud.factory.All() {
		if entity.BuiltOn == pointID && entity.Alive {
			return true
		}
	}
	return false
}

// buildSpotAt — можно ли ставить этот чертёж с основанием в точке at.
// Возвращает точку, на которую встанет постройка (пусто — обычная земля), и
// причину отказа.
func (p *Gameplay) buildSpotAt(at [2]float64, blueprint string) (pointID, reason string, ok bool) {
	attach := p.attachTypeFor(blueprint)
	if attach == "" {
		return "", "", true
	}
	pointID = p.capturePointAt(at)
	if pointID == "" {
		return "", loc.T("requires a build object: ") + attachName(attach), false
	}
	entity, found := p.hud.factory.Get(pointID)
	if !found {
		return pointID, loc.T("point not found"), false
	}
	pointType, objective := p.pointAttachType(entity)
	if pointType != attach {
		return pointID, loc.T("can only be built on ") + attachName(attach), false
	}
	if objective {
		return pointID, loc.T("scenario objective cannot be built on"), false
	}
	if p.pointBuiltWith(pointID, attach) {
		return pointID, loc.T("the point already has a building"), false
	}
	if attachNeedsOwnership(attach) && entity.Owner != p.hud.ownPlayer {
		return pointID, loc.T("point is not captured"), false
	}
	return pointID, "", true
}

// attachName — как назвать место в подсказке.
func attachName(attach string) string {
	switch attach {
	case attachSlag:
		return loc.T("slag deposit")
	case attachStrategicPoint:
		return loc.T("your strategic point or relic")
	}
	return loc.T("special point")
}

// attachNeedsOwnership — пост слуха ставится только на свою точку, а
// слаг-депозит захватывать не требуется: своё место важнее владельца.
func attachNeedsOwnership(attach string) bool {
	return attach == attachStrategicPoint
}
