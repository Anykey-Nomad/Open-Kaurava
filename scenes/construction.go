package scenes

import (
	"fmt"
	"github.com/Anykey-Nomad/Open-Kaurava/loc"
	"math"

	"dow-mod-studio/sgbview"

	"github.com/Anykey-Nomad/Open-Kaurava/entities"
	"github.com/Anykey-Nomad/Open-Kaurava/scene"
)

// Строительство игроком идёт тремя шагами, как в оригинале:
//
//  1. Клетка «Строить» панели приказов (номенклатура — structure_buildable_ext)
//     включает режим размещения: под курсором на земле виден «призрак»
//     основания постройки, его полуоси берутся из DATASEUI модели (.whe,
//     страница Selected UI).
//  2. Левый клик ставит стройплощадку: здание появляется почти без здоровья
//     (entities.ConstructionStartHealth) и ещё не работает.
//  3. Строитель идёт к площадке и работает, пока ему не отдадут другой
//     приказ: прогресс и здоровье растут вместе, а по завершении постройка
//     становится обычной.

// constructionSite — поставленная, но ещё не достроенная площадка.
type constructionSite struct {
	buildingID string
	blueprint  string
	label      string
	center     [2]float64
	ground     float64
	half       [2]float64
	height     float64
	// angle — откуда строитель подходит к площадке в следующий раз: он
	// обходит постройку, а не стоит в одной точке.
	angle     float64
	stepTimer float64
	// builders — кто был назначен на площадку: по завершении им снимается
	// приказ обхода (фабрика к этому моменту список уже очистила).
	builders []string
	// done — работа закончена.
	done bool
	// meshName — имя меша постройки в рендерере. Пусто, пока модель не
	// загрузилась: до этого площадка рисуется коробкой.
	meshName string
}

// placementState — «призрак» постройки в текущем кадре.
type placementState struct {
	path   string
	label  string
	half   [2]float64
	height float64
	at     [2]float64
	ground float64
	// ok — курсор попал на землю, valid — место свободно, blocker — имя
	// объекта или причина, по которой место не годится.
	ok      bool
	valid   bool
	blocker string
	// point — точка захвата под основанием: пост слуха и термогенератор
	// ставятся только на неё (см. buildSpotAt).
	point string
}

// updatePlacement ведёт режим размещения: считает «призрак» под курсором и
// ставит постройку по левому клику. Клик по самой карте, а не по HUD.
func (p *Gameplay) updatePlacement(c *scene.Ctx) {
	if p.hud == nil || p.hud.factory == nil || p.hud.placeBlueprint == "" {
		p.place = placementState{}
		return
	}
	if p.place.path != p.hud.placeBlueprint {
		half, height := p.hud.attribData.footprintFor(p.hud.placeBlueprint)
		p.place = placementState{
			path:   p.hud.placeBlueprint,
			label:  p.hud.placeLabel,
			half:   half,
			height: height,
		}
	}
	hit, ok := p.groundHit(c.Input.MouseX, c.Input.MouseY, c.W, c.H)
	if !ok {
		p.place.ok = false
		p.place.point = ""
		return
	}
	p.place.at = [2]float64{hit[0], hit[1]}
	p.place.ground = p.groundHeight(hit[0], hit[1])
	p.place.ok = true
	// Особая постройка сама притягивается к подходящей точке: наводить
	// мышью точно в неё не нужно, «призрак» стоит уже на точке.
	if snapped, pointID := p.snapToPoint(p.place.at, p.place.path); pointID != "" {
		p.place.at = snapped
		p.place.ground = p.groundHeight(snapped[0], snapped[1])
	}
	p.place.point, p.place.blocker, p.place.valid = p.placementCheck(p.place.at, p.place.path, p.place.half)
	for _, click := range c.Input.Clicks {
		if click.Button != 1 || p.hud.pointInHUD(click.X, click.Y, c) {
			continue
		}
		if !p.place.valid {
			p.hud.command = p.placementRefusal()
			return
		}
		p.placeConstruction()
		return
	}
}

// placementRefusal — строка отказа: что мешает и каким вышло основание.
// Размер в метрах нужен не для красоты: если DATASEUI прочитан неверно, по
// нему сразу видно, что «призрак» не того масштаба.
func (p *Gameplay) placementRefusal() string {
	reason := loc.T("site is occupied")
	if p.place.blocker != "" {
		reason = loc.T("blocked by ") + p.place.blocker
	}
	return fmt.Sprintf(loc.T("Cannot build here: %s (footprint %.0f×%.0f m)"),
		reason, 2*p.place.half[0], 2*p.place.half[1])
}

// footprintHalf — радиус основания постройки: её большая полуось из DATASEUI.
func footprintHalf(half [2]float64) float64 {
	if half[0] > half[1] {
		return half[0]
	}
	return half[1]
}

// placeConstruction ставит стройплощадку и отправляет к ней выделенного
// юнита: без строителя работа стоять не будет.
func (p *Gameplay) placeConstruction() {
	blueprint := p.hud.placeBlueprint
	label := p.hud.placeLabel
	unit, ok := p.hud.attribData.unitByPath(blueprint)
	if !ok {
		p.hud.command = loc.T("Building blueprint not read")
		return
	}
	owner := p.hud.ownPlayer
	// Время постройки — time_cost чертежа (секунды для обычного строителя),
	// а коэффициент скорости даёт repair_ext его строителя: сервитор 30 hp/с
	// строит ровно за это время, enginseer 60 — вдвое быстрее.
	work := float64(unit.BuildTime)
	if work <= 0 {
		work = 12
	}
	// Стартовая доля здоровья площадки — из tuning/building_info, как в игре.
	startHealth := p.hud.attribData.tuning().StartHealthPercent
	spec := entities.Spec{
		Name: label, Kind: entities.KindBuilding, ProducedBy: owner, Owner: owner,
		Selectable:       true,
		BuiltOn:          p.place.point,
		HitPoints:        float64(unit.HitPoints),
		ConstructionWork: work,
		StartHealthRatio: startHealth,
		Income: entities.Income{
			Requisition: unit.Income.Requisition,
			Power:       unit.Income.Power,
			Faith:       unit.Income.Faith,
		},
		CapBonus: entities.CapBonus{Squad: unit.CapBonus.Squad, Support: unit.CapBonus.Support},
		// Постройка с оружием (турели) стреляет сама — автоатака у неё
		// включена наравне с юнитами.
		// Радиус основания — для дистанции атаки: атакующие встают к
		// краю постройки, а не в её центр. Броня — из чертежа: без неё
		// оружие не найдёт класс брони и урон станет нулевым.
		Radius: footprintHalf(p.place.half),
		// Полуоси бокса — прямо из основания площадки: по ним цель
		// меряется до стены, и разница орудий не тонет в габаритах.
		Half:        p.place.half,
		ArmourClass: unit.ArmourClass,
		Armour:      unit.Armour,
	}
	spawned, err := p.hud.factory.Spawn(spec, entities.Position{X: p.place.at[0], Y: p.place.at[1]})
	if err != nil {
		p.hud.command = loc.T("Building not created: ") + err.Error()
		return
	}
	if p.hud.blueprint == nil {
		p.hud.blueprint = map[string]string{}
	}
	p.hud.blueprint[spawned.ID] = blueprint
	// Объект на карте: по нему работает выбор лучом и рисуется индикатор.
	p.objects = append(p.objects, gameplayObject{
		id: spawned.ID, name: label, owner: owner, kind: entities.KindBuilding,
		selectable: true,
		center:     [3]float64{p.place.at[0], p.place.at[1], p.place.ground},
		radius:     math.Hypot(p.place.half[0], p.place.half[1]),
	})
	site := &constructionSite{
		buildingID: spawned.ID, blueprint: blueprint, label: label,
		center: p.place.at, ground: p.place.ground,
		half: p.place.half, height: p.place.height,
	}
	if p.sites == nil {
		p.sites = map[string]*constructionSite{}
	}
	p.sites[spawned.ID] = site
	// Модель постройки читается в фоне: до её готовности площадка видна
	// коробкой (см. site_models.go).
	p.requestSiteModel(spawned.ID, blueprint)
	p.hud.clearPlacement()
	p.hud.command = loc.T("Placed: ") + label + loc.T(" — requires a builder")

	// Строителем становится выделенный отряд: он идёт к площадке и работает,
	// пока не получит другой приказ.
	builder, hasBuilder := p.hud.selectedEntity()
	if !hasBuilder || builder.Kind != entities.KindUnit || builder.Owner != owner {
		return
	}
	if err := p.hud.factory.AddBuilder(spawned.ID, builder.ID); err != nil {
		return
	}
	site.builders = append(site.builders, builder.ID)
	p.orderBuilderToSite(builder, site)
	p.hud.command = loc.T("Construction: ") + label
}

// orderBuilderToSite отправляет строителя к площадке: он останавливается у её
// края, чтобы не влезать в основание.
func (p *Gameplay) orderBuilderToSite(builder entities.Entity, site *constructionSite) {
	order := entities.Order{
		Target: entities.Position{X: site.center[0], Y: site.center[1]},
		Stop:   math.Max(site.half[0], site.half[1]) + 2,
	}
	if !p.attachPath(&order, builder) {
		p.hud.command = loc.T("Builder cannot reach the site: ") + site.label
		return
	}
	_ = p.hud.factory.Move(builder.ID, order)
}

// updateConstruction двигает работу на площадках: строитель даёт прогресс,
// только пока стоит у постройки. Ушёл по другому приказу или погиб — работа
// стоит (прогресс не сбрасывается).
func (p *Gameplay) updateConstruction(dt float64) {
	if p.hud == nil || p.hud.factory == nil || p.hud.paused || dt <= 0 {
		return
	}
	for id, site := range p.sites {
		building, ok := p.hud.factory.Get(id)
		if !ok {
			delete(p.sites, id)
			continue
		}
		if !building.UnderConstruction() {
			if !site.done {
				// Работа закончена: строитель больше не обходит постройку, а
				// постройка переходит со стадии стройки на свой idle.
				site.done = true
				if site.meshName != "" && p.world != nil {
					p.world.SetAnim(site.meshName, [3]float64{}, sgbview.AnimIdle)
				}
				for _, builderID := range site.builders {
					_ = p.hud.factory.ClearOrder(builderID)
				}
				p.hud.command = loc.T("Built: ") + site.label
			}
			continue
		}
		// Анимация стройки ведётся прогрессом работы: сколько сделано, на
		// том кадре клип и стоит — пауза в стройке держит кадр.
		if site.meshName != "" && p.world != nil {
			p.world.SetConstruction(site.meshName, building.ConstructionRatio())
		}
		if site.stepTimer > 0 {
			site.stepTimer -= dt
		}
		for _, builderID := range building.Build.Builders {
			builder, ok := p.hud.factory.Get(builderID)
			if !ok || !builder.Alive || !p.atSite(builder, site) {
				continue
			}
			// Скорость работы — коэффициент строителя из его чертежа.
			work := dt * p.hud.attribData.builderSpeed(p.hud.blueprintOf(builder))
			if _, err := p.hud.factory.Contribute(id, builderID, work); err != nil {
				continue
			}
			// Строитель «бегает» вокруг постройки: новая точка на окружности
			// основания, а не одна и та же поза.
			if site.stepTimer <= 0 {
				p.walkAround(builder, site)
				site.stepTimer = 1.5
			}
		}
	}
}

// atSite — строитель стоит у площадки и может работать.
func (p *Gameplay) atSite(builder entities.Entity, site *constructionSite) bool {
	radius := math.Max(site.half[0], site.half[1]) + 5
	dx, dy := builder.Position.X-site.center[0], builder.Position.Y-site.center[1]
	return dx*dx+dy*dy <= radius*radius
}

// walkAround ставит строителю точку на окружности вокруг площадки: так он
// обходит постройку, «строя» её то с одной, то с другой стороны.
func (p *Gameplay) walkAround(builder entities.Entity, site *constructionSite) {
	site.angle += 1.1
	radius := math.Max(site.half[0], site.half[1]) + 2.5
	order := entities.Order{
		Target: entities.Position{
			X: site.center[0] + math.Cos(site.angle)*radius,
			Y: site.center[1] + math.Sin(site.angle)*radius,
		},
		Stop: 0.4,
	}
	if !p.attachPath(&order, builder) {
		return
	}
	_ = p.hud.factory.Move(builder.ID, order)
}

// placementFree — место свободно: основание не пересекается ни с одним живым
// объектом карты (постройки, юниты). Метка — имя объекта, который помешал.
//
// Зазор «no_build_distance» из tuning здесь намеренно не используется как
// жёсткий запрет: в DoWDE он обслуживает подсветку зоны застройки, а не
// минимальное расстояние между постройками — в качестве запрета он делает
// непоставимой почти любую площадку рядом с юнитами.
func (p *Gameplay) placementFree(at [2]float64, half [2]float64) (string, bool) {
	if p.hud == nil || p.hud.factory == nil {
		return loc.T("gameplay objects not loaded"), false
	}
	radius := math.Hypot(half[0], half[1])
	if badSize(radius) {
		radius = 8
	}
	for _, entity := range p.hud.factory.All() {
		if !entity.Alive {
			continue
		}
		// Постройка и точки захвата занимают своё основание, а юнит — только
		// своё тело: отряд рядом с местом застройки не должен запрещать
		// постановку, иначе у своего штаба строить негде.
		other := 2.0
		if entity.Kind != entities.KindUnit {
			other = objectRadius(entity.ID, p.objectRadius)
		}
		limit := radius + other + 0.5
		dx, dy := entity.Position.X-at[0], entity.Position.Y-at[1]
		if notFinite(dx) || notFinite(dy) {
			continue
		}
		if dx*dx+dy*dy < limit*limit {
			return entity.Name, false
		}
	}
	return "", true
}

// badSize — размер не годится для геометрии: ноль, отрицательное, NaN или
// невероятная для карты величина. Такие числа приходят из данных мода
// (например, если модель не имеет DATASEUI) и иначе блокируют всей карте
// постановку разом. Для координат и разниц не используется — там
// отрицательные значения нормальны.
func badSize(value float64) bool {
	return notFinite(value) || value <= 0 || value > 60
}

// notFinite — значение не число (NaN или бесконечность).
func notFinite(value float64) bool {
	return math.IsNaN(value) || math.IsInf(value, 0)
}

// objectRadius — радиус объекта, безопасный для проверки места.
func objectRadius(id string, lookup func(string) float64) float64 {
	radius := lookup(id)
	if badSize(radius) {
		return 4
	}
	return radius
}

// objectRadius — радиус объекта на карте: из загруженных объектов сцены, а
// для созданных в игре — из данных выделения сущности.
func (p *Gameplay) objectRadius(id string) float64 {
	for _, object := range p.objects {
		if object.id == id {
			return object.radius
		}
	}
	if entity, ok := p.hud.factory.Get(id); ok {
		if radius := math.Hypot(entity.SelectionUI.Scale[0], entity.SelectionUI.Scale[1]); radius > 0 {
			return radius
		}
	}
	return 4
}

// drawPlacement рисует «призрак» постройки под курсором и коробки уже
// заложенных площадок: меш постройки рендерер получает только из сцены
// сценария, поэтому до загрузки моделей по ходу игры коробка и есть её тело.
func (p *Gameplay) drawPlacement(c *scene.Ctx) {
	if p.hud != nil && p.hud.placeBlueprint != "" && p.place.ok {
		bottom := [4]float32{0.35, 0.85, 1, 0.95}
		if !p.place.valid {
			bottom = [4]float32{1, 0.40, 0.32, 0.95}
		}
		top := [4]float32{bottom[0], bottom[1], bottom[2], 0.45}
		p.drawWorldBox(c, p.place.at, p.place.ground,
			p.place.half[0], p.place.half[1], p.place.height, bottom, top)
	}
	for _, site := range p.sites {
		building, ok := p.hud.factory.Get(site.buildingID)
		if !ok {
			continue
		}
		// Модель загрузилась — постройку рисует сцена, коробка больше не нужна.
		if site.meshName != "" {
			continue
		}
		if site.done {
			// Готовая постройка: та же коробка, но уже полной высоты.
			edge := [4]float32{0.62, 0.72, 0.86, 0.85}
			topEdge := [4]float32{0.62, 0.72, 0.86, 0.55}
			p.drawWorldBox(c, site.center, site.ground, site.half[0], site.half[1], site.height, edge, topEdge)
			continue
		}
		ratio := building.ConstructionRatio()
		bottom := [4]float32{0.55, 0.75, 1, 0.9}
		top := [4]float32{0.55, 0.75, 1, 0.3 + float32(ratio)*0.55}
		// Коробка «растёт» вместе с работой — видно, сколько осталось.
		p.drawWorldBox(c, site.center, site.ground, site.half[0], site.half[1],
			site.height*(0.35+0.65*ratio), bottom, top)
	}
}

// drawWorldBox рисует коробку постройки: два прямоугольника основания и четыре
// вертикальных ребра, спроецированные камерой.
func (p *Gameplay) drawWorldBox(c *scene.Ctx, center [2]float64, ground, halfX, halfY, height float64, bottom, top [4]float32) {
	corners := [4][2]float64{{-halfX, -halfY}, {halfX, -halfY}, {halfX, halfY}, {-halfX, halfY}}
	var lo, hi [4][2]float32
	projected := 0
	for i, corner := range corners {
		x, y := center[0]+corner[0], center[1]+corner[1]
		if sx, sy, ok := projectWorld(&p.cam, [3]float64{x, y, ground}, c.W, c.H); ok {
			lo[i] = [2]float32{sx, sy}
			projected++
		}
		if sx, sy, ok := projectWorld(&p.cam, [3]float64{x, y, ground + height}, c.W, c.H); ok {
			hi[i] = [2]float32{sx, sy}
		}
	}
	if projected < 4 {
		return
	}
	for i := 0; i < 4; i++ {
		next := (i + 1) % 4
		drawScreenSegment(c, lo[i][0], lo[i][1], lo[next][0], lo[next][1], bottom)
		drawScreenSegment(c, hi[i][0], hi[i][1], hi[next][0], hi[next][1], top)
		drawScreenSegment(c, lo[i][0], lo[i][1], hi[i][0], hi[i][1], bottom)
	}
	// Полупрозрачная заливка основания: экранный прямоугольник по проекции.
	minX, minY := lo[0][0], lo[0][1]
	maxX, maxY := lo[0][0], lo[0][1]
	for _, point := range lo {
		minX, minY = min(minX, point[0]), min(minY, point[1])
		maxX, maxY = max(maxX, point[0]), max(maxY, point[1])
	}
	fill := [4]float32{bottom[0], bottom[1], bottom[2], bottom[3] * 0.18}
	c.Rect(minX, minY, maxX-minX, maxY-minY, fill)
}
