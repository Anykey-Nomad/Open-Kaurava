package attrib

import (
	"dow-mod-studio/modapi"
)

// BuildingTuning — общие правила строительства из
// Data/attrib/tuning/building_info.rgd (в объединённом дереве модуля он же
// виден как tables/tuning/building_info). Параметры одинаковы для всех рас,
// если мод их не переопределяет.
type BuildingTuning struct {
	// StartHealthPercent — доля здоровья, с которой появляется стройплощадка
	// (в DoWDE — 0.2, то есть 20% от максимума чертежа).
	StartHealthPercent float64
	// BuildDecrease — базовый шаг работы строителя из tuning.
	BuildDecrease float64
	// NoBuildDistance — минимальный зазор между постройками, метров:
	// ближе этого расстояния новую площадку ставить нельзя.
	NoBuildDistance float64
	// BuildingSnapDistance — шаг привязки площадки к сетке, метров.
	BuildingSnapDistance float64
	// BuildZoneHeight — высота зоны застройки, метров.
	BuildZoneHeight float64
}

// LoadBuildingTuning читает общие правила строительства. Без дерева модуля
// или без файла возвращаются значения DoWDE — по ним и считается HUD.
func LoadBuildingTuning(tree *modapi.ModTree) BuildingTuning {
	tuning := BuildingTuning{
		StartHealthPercent:   0.2,
		BuildDecrease:        0.5,
		NoBuildDistance:      8,
		BuildingSnapDistance: 2,
		BuildZoneHeight:      5,
	}
	if tree == nil {
		return tuning
	}
	node, err := readBlueprint(tree, `Data\attrib\tuning\building_info.rgd`)
	if err != nil {
		return tuning
	}
	if value, ok := numAt(node, "start_health_percent"); ok {
		tuning.StartHealthPercent = float64(value)
	}
	if value, ok := numAt(node, "build_decrease"); ok {
		tuning.BuildDecrease = float64(value)
	}
	if value, ok := numAt(node, "no_build_distance"); ok {
		tuning.NoBuildDistance = float64(value)
	}
	if value, ok := numAt(node, "building_snap_distance"); ok {
		tuning.BuildingSnapDistance = float64(value)
	}
	if value, ok := numAt(node, "build_zone_height"); ok {
		tuning.BuildZoneHeight = float64(value)
	}
	return tuning
}
