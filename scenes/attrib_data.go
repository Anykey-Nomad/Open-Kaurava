package scenes

import (
	"fmt"
	"github.com/Anykey-Nomad/Open-Kaurava/loc"
	"math"
	"path/filepath"
	"strings"

	"dow-mod-studio/modapi"
	"dow-mod-studio/sgbview"
	"dow-mod-studio/ucslocale"

	"github.com/Anykey-Nomad/Open-Kaurava/attrib"
	"github.com/Anykey-Nomad/Open-Kaurava/entities"
	"github.com/Anykey-Nomad/Open-Kaurava/mods"
	"github.com/Anykey-Nomad/Open-Kaurava/rgdmeta"
)

// attribData — чертежи Data/attrib, прочитанные один раз при загрузке
// сценария: юниты и постройки (ebps), отряды (sbps) и лимиты рас.
//
// Читается в фоне вместе со сценарием (см. Gameplay.startLoad): несколько
// сотен файлов на каждый кадр основной поток читать не должен.
type attribData struct {
	// units — чертежи ebps объектов сценария, ключ — rgdmeta.Key.
	units map[string]attrib.Unit
	// squads — отряд по любому из его бойцов (trooper_base), ключ —
	// rgdmeta.Key чертежа ebps бойца.
	squads map[string]attrib.Squad
	// squadsByName — отряд по имени файла sbps ("necron_basic_warrior_squad"):
	// по нему томб-спайдер находит отряд призыва из harvest_ext.
	squadsByName map[string]attrib.Squad
	// abilities — кэш способностей по пути чертежа (воскрешение лорда).
	abilities map[string]attrib.Ability
	// races — лимиты рас по папкам ebps/sbps.
	races map[string]attrib.RaceCaps
	loc   *ucslocale.Table
	// tree — дерево модуля: панель приказов дочитывает по нему найм и
	// строительство, когда подменю открывается (архивы уже проиндексированы).
	tree *modapi.ModTree
	// unitsByPath, squadsByPath — кэш чертежей по пути: меню строительства
	// помнит прочитанные постройки, меню найма — отряды.
	unitsByPath  map[string]attrib.Unit
	squadsByPath map[string]attrib.Squad
	// buildables — постройки расы из structure_buildable_ext и папка, для
	// которой они прочитаны.
	buildables       []attrib.Unit
	buildablesFolder string
	// footprints — размеры основания построек по пути ebps: полуоси и высота
	// из DATASEUI модели, для «призрака» размещения.
	footprints map[string]buildingFootprint
	// buildingTuning — общие правила строительства (tuning/building_info).
	buildingTuning *attrib.BuildingTuning
}

// repairRateBaseline — базовая скорость ремонта и стройки DoWDE: столько
// здоровья в секунду добавляет обычный строитель (space_marine_servitor).
// Время постройки здания из ebps задано для неё, а более быстрый строитель
// (enginseer — 60) масштабирует его.
const repairRateBaseline = 30.0

// buildingFootprint — размеры основания постройки для «призрака» размещения.
type buildingFootprint struct {
	half   [2]float64
	height float64
}

// loadAttribData читает чертежи в фоне: юниты объектов сценария, отряды всех
// рас модуля и лимиты рас. lib может быть nil — тогда без таблиц локализации.
func loadAttribData(tree *modapi.ModTree, sc *sgbview.Scene, lib *mods.Library) *attribData {
	data := &attribData{
		tree:         tree,
		units:        map[string]attrib.Unit{},
		squads:       map[string]attrib.Squad{},
		squadsByName: map[string]attrib.Squad{},
		abilities:    map[string]attrib.Ability{},
		unitsByPath:  map[string]attrib.Unit{},
		squadsByPath: map[string]attrib.Squad{},
		footprints:   map[string]buildingFootprint{},
		races:        attrib.LoadRaceCaps(tree),
	}
	if lib != nil {
		data.loc = ucslocale.Load(lib.Root(), libraryModuleDir(lib), mods.Locale)
	}
	if tree == nil {
		return data
	}
	if sc != nil {
		for _, object := range sc.Objects {
			key := rgdmeta.Key(object.Name)
			if _, seen := data.units[key]; seen {
				continue
			}
			unit, err := attrib.ReadUnit(tree, rgdmeta.ReadPath(object.Name))
			if err != nil {
				continue
			}
			data.units[key] = unit
		}
	}
	// Отряды читаются целиком по расам: связь «бойец → отряд» есть только в
	// sbps (squad_loadout_ext/trooper_base), поэтому без обхода папок расы
	// найти отряд объекта неоткуда.
	for folder := range data.races {
		for _, file := range modapi.FilesInFolder(tree, fmt.Sprintf(`Data\attrib\sbps\races\%s`, folder)) {
			squad, ok, err := attrib.ReadSquad(tree, file.FullPath)
			if err != nil || !ok {
				continue
			}
			name := strings.ToLower(fileNameOf(file.FullPath))
			if _, seen := data.squadsByName[name]; !seen {
				data.squadsByName[name] = squad
			}
			for _, trooper := range squad.Troopers {
				key := rgdmeta.Key(trooper)
				if _, seen := data.squads[key]; !seen {
					data.squads[key] = squad
				}
			}
		}
	}
	return data
}

// slotRace — слот игрока и папка его расы ("space_marines"): связка
// считается в основном потоке по глобальному Setup, а чертежи расы
// читаются в фоне загрузки.
type slotRace struct {
	slot   int
	folder string
}

// startingUnitsFor — стартовые бойцы слотов: starting_squads чертежей их
// рас, разрешённые в чертежи ebps. Бойцы с флагом Dead (лорд некронов,
// томб-спайдер — у их чертежей keep_persistent_body = false) появляются
// мёртвыми: их нельзя выделить и ими нельзя управлять.
func startingUnitsFor(tree *modapi.ModTree, raceSlots []slotRace) []sgbview.SlotUnit {
	if tree == nil || len(raceSlots) == 0 {
		return nil
	}
	races := attrib.LoadRaceCaps(tree)
	var units []sgbview.SlotUnit
	for _, rs := range raceSlots {
		caps, ok := races[rs.folder]
		if !ok {
			continue
		}
		for _, start := range attrib.StartingUnits(tree, caps) {
			units = append(units, sgbview.SlotUnit{
				Slot:      rs.slot,
				Blueprint: start.Ebps,
				Offset:    start.Offset,
				HasOffset: start.HasOffset,
				Dead:      start.Dead,
			})
		}
	}
	return units
}

// unitFor возвращает чертёж объекта сценария, а если такого нет — дочитывает
// по пути. Бойцы, нанятые в игре, не лежат в объектах сценария, поэтому без
// этого озвучка у них не находилась (играл сигнал-заглушка).
func (d *attribData) unitFor(blueprint string) (attrib.Unit, bool) {
	if d == nil {
		return attrib.Unit{}, false
	}
	if unit, ok := d.units[rgdmeta.Key(blueprint)]; ok {
		return unit, true
	}
	unit, ok := d.unitByPath(blueprint)
	if !ok {
		return attrib.Unit{}, false
	}
	if d.units == nil {
		d.units = map[string]attrib.Unit{}
	}
	d.units[rgdmeta.Key(blueprint)] = unit
	return unit, true
}

// squadFor возвращает отряд, в который входит объект как бойец.
func (d *attribData) squadFor(blueprint string) (attrib.Squad, bool) {
	if d == nil {
		return attrib.Squad{}, false
	}
	squad, ok := d.squads[rgdmeta.Key(blueprint)]
	return squad, ok
}

// raceFor возвращает лимиты расы, к которой относится чертёж.
func (d *attribData) raceFor(blueprint string) (attrib.RaceCaps, bool) {
	if d == nil {
		return attrib.RaceCaps{}, false
	}
	parts := strings.Split(strings.ToLower(strings.ReplaceAll(blueprint, `\`, "/")), "/")
	for i, part := range parts {
		if part == "races" && i+1 < len(parts) {
			caps, ok := d.races[parts[i+1]]
			return caps, ok
		}
	}
	return attrib.RaceCaps{}, false
}

// raceFolder возвращает запись расы по её папке ebps/sbps ("space_marines").
func (d *attribData) raceFolder(folder string) (attrib.RaceCaps, bool) {
	if d == nil || folder == "" {
		return attrib.RaceCaps{}, false
	}
	caps, ok := d.races[strings.ToLower(folder)]
	return caps, ok
}

// raceFolderOf — папка ebps/sbps расы слота ("space_marines"): берётся из
// race_path чертежа расы ("Races/Space_Marines/Troops").
func raceFolderOf(slot int) string {
	i, ok := Setup.Resolved[slot]
	if !ok || i < 0 || i >= len(Setup.Races) {
		return ""
	}
	race := Setup.Races[i]
	path := strings.TrimSpace(race.UnitPath)
	if path == "" {
		path = race.BuildingPath
	}
	return raceFolderFromPath(path)
}

// localName — текст из таблиц локализации по идентификатору вида "$97140".
func (d *attribData) localName(id string) string {
	if d == nil || d.loc == nil {
		return ""
	}
	return d.loc.Text(strings.TrimPrefix(strings.TrimSpace(id), "$"))
}

// libraryModuleDir — папка загруженного модуля в установке: в ней лежат его
// таблицы локализации, и они перекрывают строки движка.
func libraryModuleDir(lib *mods.Library) string {
	if lib == nil {
		return ""
	}
	mod := lib.Module()
	if mod == nil {
		return ""
	}
	if mod.ModFolder != "" {
		return filepath.Join(lib.Root(), mod.ModFolder)
	}
	return filepath.Dir(mod.Path)
}

// blueprintOf — чертёж выбранной сущности из карты, собранной при загрузке
// сценария. Пусто, если сущность создана не из объекта сценария.
func (g *GameplayHUD) blueprintOf(selected entities.Entity) string {
	if g == nil || g.blueprint == nil {
		return ""
	}
	return g.blueprint[selected.ID]
}

// selectionInfo готовит строки для панели свойств выбранного объекта:
// имя из локализации, здоровье и цена из ebps, а у бойца — размер отряда,
// право захвата и условия пополнения из sbps. Пустой список означает, что
// чертёж прочитать не удалось.
func (g *GameplayHUD) selectionInfo(selected entities.Entity) []string {
	if g == nil || g.attribData == nil {
		return nil
	}
	blueprint := g.blueprintOf(selected)
	if blueprint == "" {
		return nil
	}
	unit, hasUnit := g.attribData.unitFor(blueprint)
	squad, hasSquad := g.attribData.squadFor(blueprint)
	if !hasUnit && !hasSquad {
		return nil
	}
	var lines []string
	if hasUnit {
		if name := g.attribData.localName(unit.ScreenNameID); name != "" {
			lines = append(lines, name)
		}
		// Здоровье — текущее из мира, а не из чертежа: иначе бой не виден
		// в панели — цифра оставалась бы максимальной у избитого отряда.
		stat := ""
		switch {
		case selected.Health.Max > 0:
			stat = fmt.Sprintf(loc.T("Health %g/%g"), math.Ceil(selected.Health.Current), selected.Health.Max)
		case unit.HitPoints > 0:
			stat = fmt.Sprintf(loc.T("Health %g"), unit.HitPoints)
		}
		if !unit.Cost.Empty() {
			if stat != "" {
				stat += " · "
			}
			stat += unit.Cost.String()
		}
		if unit.BuildTime > 0 {
			stat += fmt.Sprintf(loc.T(" · %g s"), unit.BuildTime)
		}
		if stat != "" {
			lines = append(lines, stat)
		}
	}
	if hasSquad {
		lines = append(lines, fmt.Sprintf(loc.T("Squad: %d–%d units · uses caps: squads %d, vehicles %d"),
			squad.UnitMin, squad.UnitMax, squad.SquadCap, squad.SupportCap))
		capture := loc.T("capture: no")
		switch {
		case squad.CanCapture && squad.CanUncapture:
			capture = loc.T("capture: yes · flag removal: yes")
		case squad.CanCapture:
			capture = loc.T("capture: yes · flag removal: no")
		case squad.CanUncapture:
			capture = loc.T("capture: no · flag removal: yes")
		}
		lines = append(lines, capture)
		if squad.CanReinforce {
			line := fmt.Sprintf(loc.T("Reinforce: %s, %g s"), squad.ReinforceCost, squad.ReinforceTime)
			if len(squad.Leaders) > 0 {
				leader := squad.Leaders[0]
				line += fmt.Sprintf(loc.T(" · leader: %s"), leaderBaseName(leader.Unit))
			}
			lines = append(lines, line)
		}
	}
	return lines
}

// leaderBaseName — имя чертежа лидера без пути и расширения.
func leaderBaseName(path string) string {
	return fileNameOf(path)
}

// fileNameOf — последний элемент пути чертежа без расширения.
func fileNameOf(path string) string {
	name := path
	if slash := strings.LastIndexAny(name, `/\\`); slash >= 0 {
		name = name[slash+1:]
	}
	return strings.TrimSuffix(name, ".rgd")
}

// unitFor — чертёж объекта по пути с проверкой, что данные расы уже
// загружены: по нему читаются правила места постройки (structure_ext) и тип
// местности точки (type_ext).
func (g *GameplayHUD) unitFor(blueprint string) (attrib.Unit, bool) {
	if g == nil || g.attribData == nil || strings.TrimSpace(blueprint) == "" {
		return attrib.Unit{}, false
	}
	return g.attribData.unitByPath(blueprint)
}

// buildablesFor — постройки расы из меню строительства. Читаются один раз
// при первом открытии подменю: архивы уже проиндексированы, поэтому десяток
// чертежей ebps не требует диска.
func (d *attribData) buildablesFor(folder string) []attrib.Unit {
	if d == nil || d.tree == nil || folder == "" {
		return nil
	}
	if d.buildablesFolder == folder {
		return d.buildables
	}
	d.buildables = attrib.ReadBuildables(d.tree, folder)
	d.buildablesFolder = folder
	return d.buildables
}

// unitByPath — чертёж постройки по пути ebps из меню строительства.
func (d *attribData) unitByPath(path string) (attrib.Unit, bool) {
	if d == nil || d.tree == nil || path == "" {
		return attrib.Unit{}, false
	}
	if unit, ok := d.unitsByPath[path]; ok {
		return unit, true
	}
	unit, err := attrib.ReadUnit(d.tree, path)
	if err != nil {
		return attrib.Unit{}, false
	}
	d.unitsByPath[path] = unit
	return unit, true
}

// squadByPath — отряд по пути sbps из squad_table постройки (меню найма).
func (d *attribData) squadByPath(path string) (attrib.Squad, bool) {
	if d == nil || d.tree == nil || path == "" {
		return attrib.Squad{}, false
	}
	if squad, ok := d.squadsByPath[path]; ok {
		return squad, true
	}
	squad, ok, err := attrib.ReadSquad(d.tree, path)
	if err != nil || !ok {
		return attrib.Squad{}, false
	}
	d.squadsByPath[path] = squad
	return squad, true
}

// squadByName — отряд по имени файла sbps(“necron_basic_warrior_squad”):
// так томб-спайдер находит отряд призыва: harvest_ext хранит именно имя,
// а не путь.
func (d *attribData) squadByName(name string) (attrib.Squad, bool) {
	if d == nil {
		return attrib.Squad{}, false
	}
	squad, ok := d.squadsByName[strings.ToLower(strings.TrimSpace(name))]
	return squad, ok
}

// squadTitle — локализованное имя отряда для кнопки призыва.
func (d *attribData) squadTitle(squad attrib.Squad) string {
	if d == nil {
		return squad.Name
	}
	if name := d.localName(squad.ScreenNameID); name != "" {
		return name
	}
	return squad.Name
}

// abilityFor — способность по пути чертежа из ability_ext, с кэшем: панель
// приказов спрашивает её каждый кадр.
func (d *attribData) abilityFor(path string) (attrib.Ability, bool) {
	if d == nil || d.tree == nil || path == "" {
		return attrib.Ability{}, false
	}
	if ability, ok := d.abilities[path]; ok {
		return ability, true
	}
	ability, err := attrib.ReadAbility(d.tree, path)
	if err != nil {
		return attrib.Ability{}, false
	}
	if d.abilities == nil {
		d.abilities = map[string]attrib.Ability{}
	}
	d.abilities[path] = ability
	return ability, true
}

// tuning — общие правила строительства (tuning/building_info): стартовая
// доля здоровья площадки, зазор между постройками, шаг привязки.
func (d *attribData) tuning() attrib.BuildingTuning {
	if d == nil {
		return attrib.LoadBuildingTuning(nil)
	}
	if d.buildingTuning == nil {
		tuning := attrib.LoadBuildingTuning(d.tree)
		d.buildingTuning = &tuning
	}
	return *d.buildingTuning
}

// builderSpeed — коэффициент скорости строительства: repair_ext/repair_rate
// чертежа строителя к базовой скорости ремонта DoWDE (30 hp/с — сервитор).
// Юнит без данных ремонта строит с обычной скоростью.
func (d *attribData) builderSpeed(blueprint string) float64 {
	if d == nil || blueprint == "" {
		return 1
	}
	unit, ok := d.unitFor(blueprint)
	if !ok || unit.BuildRate <= 0 {
		return 1
	}
	return unit.BuildRate / repairRateBaseline
}

// footprintFor — полуоси основания постройки и её высота: DATASEUI модели
// (.whe, страница Selected UI) по пути entity_blueprint_ext/animator. Без
// модели или без её данных выделения — запасной квадрат.
func (d *attribData) footprintFor(path string) ([2]float64, float64) {
	const defaultHalf, defaultHeight = 8.0, 6.0
	if d == nil {
		return [2]float64{defaultHalf, defaultHalf}, defaultHeight
	}
	if size, ok := d.footprints[path]; ok {
		return size.half, size.height
	}
	if d.footprints == nil {
		d.footprints = map[string]buildingFootprint{}
	}
	size := buildingFootprint{half: [2]float64{defaultHalf, defaultHalf}, height: defaultHeight}
	if unit, ok := d.unitByPath(path); ok && unit.Animator != "" {
		if half, height, found := sgbview.BlueprintFootprint(d.tree, sgbview.ModelPath(unit.Animator)); found {
			size = buildingFootprint{half: half, height: height}
		}
	}
	// DATASEUI может дать пустые или безумные числа (модель без страницы
	// Selected UI) — тогда основание берётся запасное, а не гигантское.
	size.half, size.height = saneFootprint(size.half, size.height)
	d.footprints[path] = size
	return size.half, size.height
}

// saneFootprint — размеры основания, пригодные для размещения: полуоси в
// разумных пределах карты, высота — тоже.
func saneFootprint(half [2]float64, height float64) ([2]float64, float64) {
	for i, value := range half {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0.5 || value > 24 {
			half[i] = 8
		}
	}
	if math.IsNaN(height) || math.IsInf(height, 0) || height < 0.5 || height > 40 {
		height = 6
	}
	return half, height
}

// displayName — имя постройки для панели приказов: строка локализации из
// ui_ext/screen_name_id, а без неё — имя файла чертежа.
func (d *attribData) displayName(path string) string {
	if unit, ok := d.unitByPath(path); ok {
		if name := d.localName(unit.ScreenNameID); name != "" {
			return name
		}
		return unit.Name
	}
	return fileNameOf(path)
}

// trainInfo — цена, время найма и иконка для очереди производства: цена и
// время бойца из squad_loadout_ext/trooper_base (ebps cost_ext/time_cost)
// заданы сразу для всего отряда, а не для одного бойца, поэтому умножения
// на unit_min нет. birth_resource sbps — это не цена, а ресурсы, которые
// получает игрок при спавне отряда (у сестёр битвы так дают веру), поэтому в
// цену найма они не входят. Иконка — из sbps, а без неё — из чертежа бойца.
func (d *attribData) trainInfo(squad attrib.Squad) (attrib.Cost, float64, string) {
	cost := attrib.Cost{}
	seconds := 0.0
	icon := squad.Icon
	if len(squad.Troopers) > 0 {
		if unit, ok := d.unitByPath(squad.Troopers[0]); ok {
			cost = unit.Cost
			seconds = float64(unit.BuildTime)
			if icon == "" {
				icon = unit.Icon
			}
		}
	}
	if seconds <= 0 {
		seconds = 10
	}
	return cost, seconds, icon
}

// weaponFor переводит оружие из чертежа в модель боя: дистанция уже в
// метрах мира, таблица пробития копируется — фабрика не должна держать
// ссылку на чертёж и его карту.
func weaponFor(unit attrib.Unit) *entities.Weapon {
	if unit.Ranged == nil || unit.Ranged.MaxRange <= 0 {
		return nil
	}
	piercing := make(map[string]float64, len(unit.Ranged.Piercing))
	for class, value := range unit.Ranged.Piercing {
		piercing[class] = value
	}
	return &entities.Weapon{
		Range:      unit.Ranged.MaxRange,
		Accuracy:   unit.Ranged.Accuracy,
		ReloadTime: unit.Ranged.ReloadTime,
		MinDamage:  unit.Ranged.MinDamage,
		MaxDamage:  unit.Ranged.MaxDamage,
		Piercing:   piercing,
	}
}

// squadLabel — имя отряда для меню найма.
func (d *attribData) squadLabel(path string) string {
	if squad, ok := d.squadByPath(path); ok {
		if name := d.localName(squad.ScreenNameID); name != "" {
			return name
		}
		return squad.Name
	}
	return fileNameOf(path)
}
