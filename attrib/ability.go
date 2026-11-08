package attrib

import (
	"fmt"
	"strings"

	"dow-mod-studio/modapi"

	"github.com/Anykey-Nomad/Open-Kaurava/rgdmeta"
)

// Ability — способность юнита из ability_ext (отдельный чертёж в
// Data/attrib/abilities). Читаются только поля, которые нужны сцене:
// подпись кнопки, радиус действия, перезарядка и доля здоровья, с которой
// способность поднимает тела.
type Ability struct {
	// Path — путь чертежа в дереве модуля, Name — имя файла без расширения.
	Path string
	Name string

	// Icon и ScreenNameID — ui_info: иконка и идентификатор локализованного
	// имени (как у юнитов, только ui_info лежит в корне чертежа).
	Icon         string
	ScreenNameID string

	// RechargeTime — recharge_time: через сколько секунд способность снова
	// доступна (у массового воскрешения 180).
	RechargeTime float64

	// Radius — area_effect/area_effect_information/radius: радиус действия
	// по земле (у массового воскрешения 26 м).
	Radius float64

	// ResurrectHealth — доля здоровья, с которой способность поднимает
	// тела: value модификатора health_resurrect_modifier из
	// area_effect/weapon_damage/modifiers. 0 — способность тела не поднимает
	// (или такого модификатора в ней нет).
	ResurrectHealth float64
}

// Resurrects — способность поднимает тела на карте.
func (a Ability) Resurrects() bool { return a.ResurrectHealth > 0 }

// ReadAbility читает чертёж способности.
func ReadAbility(tree *modapi.ModTree, path string) (Ability, error) {
	node, err := readBlueprint(tree, path)
	if err != nil {
		return Ability{}, err
	}
	ability := Ability{Path: path, Name: squadName(path)}
	ability.Icon, _ = node.StrAt("ui_info", "icon_name")
	ability.ScreenNameID, _ = node.StrAt("ui_info", "screen_name_id")
	if value, found := numAt(node, "recharge_time"); found {
		ability.RechargeTime = float64(value)
	}
	// Радиус и модификаторы лежат в area_effect — не в backfire_area_effect,
	// поэтому таблица берётся по имени.
	if area := node.Get("area_effect"); area != nil {
		if info := area.Get("area_effect_information"); info != nil {
			if value, found := numAt(info, "radius"); found {
				ability.Radius = float64(value)
			}
		}
		ability.ResurrectHealth = resurrectFraction(area.Get("weapon_damage", "modifiers"))
	}
	return ability, nil
}

// resurrectFraction ищет в таблице модификаторов health_resurrect_modifier и
// берёт его value — долю здоровья, с которой тело возвращается в строй.
func resurrectFraction(node *rgdmeta.Node) float64 {
	if node == nil {
		return 0
	}
	for _, child := range node.Children {
		if strings.Contains(strings.ToLower(child.Ref), "health_resurrect") {
			if value, found := numAt(child, "value"); found {
				return fraction(value)
			}
			return 0
		}
		if value := resurrectFraction(child); value > 0 {
			return value
		}
	}
	return 0
}

// abilityLabel — имя способности для журнала и сообщений.
func (a Ability) String() string {
	if a.Name == "" {
		return a.Path
	}
	return fmt.Sprintf("%s (%s)", a.Name, a.Path)
}
