package attrib

import (
	"strings"

	"dow-mod-studio/modapi"

	"github.com/Anykey-Nomad/Open-Kaurava/rgdmeta"
)

// Income — прирост ресурсов в секунду: что чертёж даёт своему владельцу
// (resource_ext/*_per_second). Складывается по всем постройкам, точкам и
// юнитам игрока и ограничивается income_cap расы.
type Income struct {
	Requisition float64
	Power       float64
	Faith       float64
}

// Add складывает два прироста.
func (i Income) Add(other Income) Income {
	return Income{
		Requisition: i.Requisition + other.Requisition,
		Power:       i.Power + other.Power,
		Faith:       i.Faith + other.Faith,
	}
}

// CapBonus — сколько единиц лимита отрядов и техники даёт постройка:
// в чертеже это модификаторы squad_cap_player_modifier и
// support_cap_player_modifier из modifier_apply_ext/modifiers.
type CapBonus struct {
	Squad   int
	Support int
}

// Unit — чертёж одного объекта (ebps): бойца, машины или постройки.
// Постройки отличаются только папкой (…/structures/) и наличием
// structure_ext; поле IsStructure заполняется по пути чертежа.
type Unit struct {
	Path string
	Name string
	Race string

	// HitPoints — health_ext/hitpoints: здоровье по умолчанию.
	HitPoints float32
	// Invulnerable — health_ext/invulnerable: нельзя уничтожить.
	Invulnerable bool
	// KeepPersistentBody — health_ext/keep_persistent_body: «живые» бойцы
	// (строители, рядовые) имеют true; у «трупов» — лорда некронов,
	// томб-спайдера, лорда-дестройера — false, и на старте игры они
	// появляются уже мёртвыми. PersistentBodySet=false — поля в чертеже нет.
	KeepPersistentBody bool
	PersistentBodySet  bool
	// Восстановление после уничтожения — health_ext/get_back_up_*: шанс
	// подняться, доля здоровья после подъёма и задержка до него. Нули —
	// юнит не встаёт (у большинства этих полей в чертеже нет).
	GetUpChance        float64
	GetUpHealthPercent float64
	GetUpTime          float64
	// SpawnUsableBodyOnDeath — health_ext/spawn_usable_body_on_death: чертёж
	// тела, которое остаётся на карте после гибели (у некронов — воины,
	// бессмертные, ободранные, призраки). Непустое значение означает, что
	// тело можно собрать томб-спайдером или поднять воскрешением лорда;
	// у парий, скарабеев и техники поле пустое.
	SpawnUsableBodyOnDeath string
	// Harvest — harvest_ext: томб-спайдер собирает тела павших и призывает
	// из них отряды. nil — объект тела не собирает.
	Harvest *Harvest
	// Abilities — ability_ext/abilities: чертежи способностей юнита
	// (у лорда некронов среди них массовое воскрешение).
	Abilities []string
	// DisplayHealthBar — health_ext/display_health_bar.
	DisplayHealthBar bool

	// Cost и BuildTime — cost_ext/time_cost: цена объекта и время его
	// производства (для постройки — строительства).
	Cost      Cost
	BuildTime float32

	// Speed — moving_ext/speed_max, пересчитанный в метры мира в секунду:
	// движок DoW хранит скорость в условных футах в секунду (некрон 10,
	// тактический морпех 16, грут 24), а мир сцены измеряется в метрах
	// (карта 256×256 при ячейке 2 м). Без перевода юниты разных чертежей
	// (16…24) пересекают карту за 15 секунд, и разница между ними не видна.
	// Постройки раздела не имеют — их Speed остаётся нулём, и фабрика не
	// даёт им приказов движения. Модификаторы скорости пока не учитываются.
	Speed float32
	// AccelTime и BrakeTime — moving_ext/acceleration_time и
	// deceleration_time: за сколько секунд юнит разгоняется с нуля до
	// speed_max и тормозит до остановки (пехота 2 с и 1 с, dreadnought
	// 0.5 с). 0 — поля нет (постройки), движение тогда постоянной
	// скоростью, как раньше.
	AccelTime float32
	BrakeTime float32

	// Icon и ScreenNameID — ui_ext/ui_info: иконка и идентификатор имени.
	Icon         string
	ScreenNameID string
	// Speech — ui_ext/speech_directory: папка озвучки юнита
	// ("Speech/Races/Necron/Builder_Scarab"). Физически реплики лежат в
	// Data/sound/speech/races/… или Data/sound/races/…, внутри — категории
	// действий (move, capture_strategic_point …), по которым игра
	// разрешает голос приказа (см. пакет sfx).
	Speech string

	// Animator — entity_blueprint_ext/animator: путь к модели.
	Animator string

	// Selectable — sim_entity_ext/is_collide: участвует ли объект в
	// коллизиях, то есть доступен ли он для выделения.
	Selectable bool

	// IsStructure — чертёж лежит в папке построек расы.
	IsStructure bool

	// TypeEnvironment — type_ext/type_environment: тип местности объекта
	// ("tp_strategic_point_struct", "tp_slag"). По нему постройка со
	// structure_ext/attach_to находит своё место.
	TypeEnvironment string
	// AttachTo — structure_ext/attach_to/attach_to_tp: тип объекта, на
	// который постройка ставится (пост слуха — на стратегический пункт или
	// реликт, термогенератор — на слаг-депозит). Пусто — можно ставить на
	// свободную землю.
	AttachTo string
	// StrategicObjective — strategic_point_ext/is_strategic_objective: цель
	// сценария, а не позиция под пост.
	StrategicObjective bool

	// Income — прирост ресурсов в секунду от этого объекта.
	Income Income
	// CapBonus — лимиты отрядов и техники, которые даёт постройка.
	CapBonus CapBonus

	// Spawns — sbps отрядов из spawner_ext/squad_table: что постройка
	// производит (штаб, казарма). Пути — от Data/attrib.
	Spawns []string
	// Researches — research_ext/research_table: исследования постройки.
	Researches []string
	// Addons — addon_ext/addons: добавки к постройке.
	Addons []string
	// Buildable — structure_buildable_ext: постройка есть в меню
	// строительства строителя, BuildMenuPriority — её место в сетке,
	// AdvancedBuild — «продвинутая» постройка (вторая страница меню).
	Buildable         bool
	BuildMenuPriority int
	AdvancedBuild     bool
	// BuildRate — с какой скоростью юнит возводит и ремонтирует постройки:
	// repair_ext/health_repaired_per_second (в DoW ремонт и стройка идут с
	// одной скоростью — сервitor 30, enginseer 60). 0 — юнит не строитель.
	BuildRate float64

	// ArmourClass — type_ext/type_armour: класс брони бойца
	// ("tp_infantry_heavy_med"), по нему оружие выбирает процент пробития.
	ArmourClass string
	// Armour — health_ext/armour: базовое снижение урона (у всех юнитов и
	// построек DoWDE — 100).
	Armour float64
	// Ranged — первое стрелковое оружие из combat_ext/hardpoints; nil —
	// стрелять нечем (постройка, рабочая единица).
	Ranged *Weapon
}

// speedToWorld — условный фут движка DoW к метру мира сцены: столько
// составляет один «движковый» unit speed_max в метрах в секунду.
const speedToWorld = 0.3048

// HarvestSlot — одно призвание томб-спайдера: какое тело он собирает,
// какой отряд из них появляется и сколько тел для этого нужно.
// Слоты в harvest_ext — a, b, c (воины 5, ободранные 5, бессмертные 6).
type HarvestSlot struct {
	Slot   string
	Entity string
	Squad  string
	Bodies int
}

// Harvest — harvest_ext томб-спайдера: сбор тел павших некронов.
// Собранное тело пропадает с карты и больше не доступно ни другим
// спайдерам, ни воскрешению лорда; накопленные тела обмениваются на отряд
// по кнопке призыва.
type Harvest struct {
	Slots []HarvestSlot
	// BodyLimit — body_count_limit: сколько тел спайдер держит за раз.
	BodyLimit int
	// SearchRadius — search_radius_limit: насколько далеко он ищет тела,
	// PickupRadius — pickup_radius_limit: с какого расстояния подбирает,
	// PickupDuration — pick_up_duration: сколько секунд длится подбор.
	SearchRadius   float64
	PickupRadius   float64
	PickupDuration float64
}

// readHarvest читает harvest_ext спайдера. nil — объект тела не собирает.
func readHarvest(node *rgdmeta.Node) *Harvest {
	if node == nil {
		return nil
	}
	harvest := &Harvest{}
	if value, found := node.IntAt("body_count_limit"); found {
		harvest.BodyLimit = int(value)
	}
	if value, found := numAt(node, "search_radius_limit"); found {
		harvest.SearchRadius = float64(value)
	}
	if value, found := numAt(node, "pickup_radius_limit"); found {
		harvest.PickupRadius = float64(value)
	}
	if value, found := numAt(node, "pick_up_duration"); found {
		harvest.PickupDuration = float64(value)
	}
	// Слоты называются spawn_slot_a_*, spawn_slot_b_*, spawn_slot_c_*.
	for _, name := range []string{"a", "b", "c"} {
		entity, _ := node.StrAt("spawn_slot_" + name + "_entity")
		squad, _ := node.StrAt("spawn_slot_" + name + "_squad")
		bodies := 0
		if value, found := node.IntAt("spawn_slot_" + name + "_bodies_requirement"); found {
			bodies = int(value)
		}
		if strings.TrimSpace(entity) == "" || strings.TrimSpace(squad) == "" {
			continue
		}
		harvest.Slots = append(harvest.Slots, HarvestSlot{
			Slot: strings.ToUpper(name), Entity: strings.TrimSpace(entity),
			Squad: strings.TrimSpace(squad), Bodies: bodies,
		})
	}
	if len(harvest.Slots) == 0 && harvest.BodyLimit == 0 {
		return nil
	}
	return harvest
}

// ReadUnit читает чертёж объекта ebps.
func ReadUnit(tree *modapi.ModTree, path string) (unit Unit, err error) {
	node, err := readBlueprint(tree, path)
	if err != nil {
		return Unit{}, err
	}
	unit = Unit{
		Path:        path,
		Name:        squadName(path),
		Race:        raceOf(path),
		IsStructure: isStructurePath(path),
		Animator:    strAt(node, "entity_blueprint_ext", "animator"),
	}
	unit.Icon, _ = node.StrAt("ui_ext", "ui_info", "icon_name")
	unit.ScreenNameID, _ = node.StrAt("ui_ext", "ui_info", "screen_name_id")
	if value, found := node.FloatAt("health_ext", "hitpoints"); found {
		unit.HitPoints = value
	} else if value, found := node.IntAt("health_ext", "hitpoints"); found {
		unit.HitPoints = float32(value)
	}
	unit.Invulnerable, _ = node.BoolAt("health_ext", "invulnerable")
	// Речь юнита: обычно лежит в ui_ext, но раздел может называться иначе,
	// поэтому вторым шагом — поиск по всему чертежу.
	unit.Speech, _ = node.StrAt("ui_ext", "speech_directory")
	if unit.Speech == "" {
		unit.Speech = findStrField(node, "speech_directory")
	}
	unit.KeepPersistentBody, unit.PersistentBodySet = node.BoolAt("health_ext", "keep_persistent_body")
	unit.SpawnUsableBodyOnDeath, _ = node.StrAt("health_ext", "spawn_usable_body_on_death")
	unit.Harvest = readHarvest(node.Get("harvest_ext"))
	unit.Abilities = tableRefs(node.Get("ability_ext", "abilities"), "ability_", 10)
	if value, found := numAt(node, "health_ext", "get_back_up_chance"); found {
		unit.GetUpChance = fraction(value)
	}
	if value, found := numAt(node, "health_ext", "get_back_up_health_percent"); found {
		unit.GetUpHealthPercent = fraction(value)
	}
	if value, found := numAt(node, "health_ext", "get_back_up_get_up_time"); found {
		unit.GetUpTime = float64(value)
	}
	unit.DisplayHealthBar, _ = node.BoolAt("health_ext", "display_health_bar")
	if armour, found := numAt(node, "health_ext", "armour"); found {
		unit.Armour = float64(armour)
	}
	if armourType := node.Get("type_ext", "type_armour"); armourType != nil {
		unit.ArmourClass = refName(armourType.Ref)
	}
	unit.Cost, unit.BuildTime = cost(node.Get("cost_ext", "time_cost"))
	if value, found := node.FloatAt("moving_ext", "speed_max"); found {
		unit.Speed = value * speedToWorld
	} else if value, found := node.IntAt("moving_ext", "speed_max"); found {
		unit.Speed = float32(value) * speedToWorld
	}
	if value, found := numAt(node, "moving_ext", "acceleration_time"); found {
		unit.AccelTime = value
	}
	if value, found := numAt(node, "moving_ext", "deceleration_time"); found {
		unit.BrakeTime = value
	}
	unit.Selectable, _ = node.BoolAt("sim_entity_ext", "is_collide")
	if env := node.Get("type_ext", "type_environment"); env != nil {
		unit.TypeEnvironment = refName(env.Ref)
	}
	unit.AttachTo = attachType(node)
	unit.StrategicObjective, _ = node.BoolAt("strategic_point_ext", "is_strategic_objective")
	unit.Income = readIncome(node.Get("resource_ext"))
	unit.CapBonus = readCapBonus(node.Get("modifier_apply_ext", "modifiers"))
	// Слотов в таблицах фиксированное число: 10 отрядов в найме, 20
	// исследований и 10 добавок — так устроены tables/*_table.lua игры.
	if value, found := numAt(node, "repair_ext", "health_repaired_per_second"); found {
		unit.BuildRate = float64(value)
	} else if value, found := numAt(node, "building_engineer_ext", "build_rate"); found {
		unit.BuildRate = float64(value)
	}
	unit.Spawns = tableRefs(node.Get("spawner_ext", "squad_table"), "squad_", 10)
	unit.Researches = tableRefs(node.Get("research_ext", "research_table"), "research_", 20)
	unit.Addons = tableRefs(node.Get("addon_ext", "addons"), "addon_", 10)
	if buildable := node.Get("structure_buildable_ext"); buildable != nil {
		unit.Buildable = true
		if value, found := buildable.IntAt("build_menu_priority"); found {
			unit.BuildMenuPriority = int(value)
		}
		unit.AdvancedBuild, _ = buildable.BoolAt("advanced_build_option")
	}
	unit.Ranged = readRangedWeapon(tree, node)
	return unit, nil
}

// attachType — тип местности, к которому крепится постройка. Запись
// structure_ext/attach_to берётся из attachto_table и включается полем active:
// без него постройка ставится на свободную землю.
func attachType(node *rgdmeta.Node) string {
	if attach := node.Get("structure_ext", "attach_to"); attach != nil {
		if active, found := attach.BoolAt("active"); !found || !active {
			return ""
		}
		if tp := attach.Get("attach_to_tp"); tp != nil {
			return refName(tp.Ref)
		}
		return ""
	}
	if tp := node.Get("structure_ext", "attach_to_tp"); tp != nil {
		return refName(tp.Ref)
	}
	return ""
}

// readIncome читает *_per_second из resource_ext.
func readIncome(node *rgdmeta.Node) Income {
	if node == nil {
		return Income{}
	}
	number := func(name string) float64 {
		if value, ok := node.FloatAt(name); ok {
			return float64(value)
		}
		if value, ok := node.IntAt(name); ok {
			return float64(value)
		}
		return 0
	}
	return Income{
		Requisition: number("requisition_per_second"),
		Power:       number("power_per_second"),
		Faith:       number("faith_per_second"),
	}
}

// readCapBonus суммирует лимиты из модификаторов постройки: каждый
// модификатор со ссылкой на squad_/support_cap_player_modifier добавляет
// своё value (usage_type — сложение).
func readCapBonus(node *rgdmeta.Node) CapBonus {
	var bonus CapBonus
	if node == nil {
		return bonus
	}
	for _, child := range node.Children {
		if !child.IsTable() {
			continue
		}
		ref := strings.ToLower(child.Ref)
		value := 0
		if v, ok := child.IntAt("value"); ok {
			value = int(v)
		} else if v, ok := child.FloatAt("value"); ok {
			value = int(v)
		}
		switch {
		case strings.Contains(ref, "squad_cap_player_modifier"):
			bonus.Squad += value
		case strings.Contains(ref, "support_cap_player_modifier"):
			bonus.Support += value
		}
	}
	return bonus
}

// findStrField ищет строковое поле с именем name в любом разделе чертежа.
// Нужен для полей, чей раздел в разных расах назван по-разному.
func findStrField(node *rgdmeta.Node, name string) string {
	if node == nil {
		return ""
	}
	if node.Named(name) {
		if value, ok := node.StrAt(); ok && value != "" {
			return value
		}
	}
	for _, child := range node.Children {
		if value := findStrField(child, name); value != "" {
			return value
		}
	}
	return ""
}

// isStructurePath — чертёж лежит в папке построек расы.
func isStructurePath(path string) bool {
	p := strings.ToLower(strings.ReplaceAll(path, `\`, "/"))
	return strings.Contains(p, "/ebps/races/") && strings.Contains(p, "/structures/")
}

// strAt читает строку по пути или возвращает пустую.
func strAt(node *rgdmeta.Node, path ...string) string {
	value, found := node.StrAt(path...)
	if !found {
		return ""
	}
	return value
}

// numAt читает число независимо от типа поля (float или int).
func numAt(node *rgdmeta.Node, path ...string) (float32, bool) {
	if node == nil {
		return 0, false
	}
	if value, found := node.FloatAt(path...); found {
		return value, true
	}
	if value, found := node.IntAt(path...); found {
		return float32(value), true
	}
	return 0, false
}

// fraction приводит долю из чертежа к 0..1: в одних ebps она записана долей
// (0.25 = 25%), в других процентами (50), а health_percent встречается и в
// сотых долях процента (5000 = 50%). Делим на 100, пока число больше
// единицы — так одно правило покрывает все варианты.
func fraction(value float32) float64 {
	out := float64(value)
	for i := 0; i < 4 && out > 1; i++ {
		out /= 100
	}
	return out
}
