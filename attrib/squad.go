package attrib

import (
	"fmt"
	"path/filepath"
	"strings"

	"dow-mod-studio/modapi"
)

// Leader — встроенный лидер отряда (сержант, апоктекарь): кого можно
// добавить в отряд и во что это обходится. Слоты leader_1…leader_4 в
// чертеже; незаполненные (type пусто) в список не попадают.
type Leader struct {
	Slot        string
	Unit        string // путь ebps бойца-лидера
	Cost        Cost
	TimeSeconds float32
}

// Squad — чертёж отряда (sbps): всё, что относится к отряду как к группе,
// а не к одному бойцу.
type Squad struct {
	// Path — путь чертежа в дереве модуля, Name — имя файла без расширения,
	// Race — папка расы (space_marines).
	Path string
	Name string
	Race string

	// UnitMin/UnitMax — squad_loadout_ext/unit_min и unit_max: сколько
	// бойцов в отряде по умолчанию и сколько максимально (пополнение).
	UnitMin int
	UnitMax int
	// Troopers — ebps бойцов отряда из squad_loadout_ext/trooper_base.
	Troopers []string
	// Birth — squad_loadout_ext/birth_resource: цена поднятия отряда.
	Birth Cost

	// CanCapture/CanUncapture — squad_capture_strategic_point_ext.
	// У командиров и строителей этой таблицы нет вовсе — и это ответ на
	// вопрос, кто может захватывать точки.
	CanCapture   bool
	CanUncapture bool
	CaptureRate  float32

	// CanReinforce и цена пополнения — squad_reinforce_ext: могут ли
	// бойцов добавлять и сколько стоит один.
	CanReinforce  bool
	ReinforceCost Cost
	ReinforceTime float32

	// Leaders и MaxLeaders — squad_leader_ext: кого встраивают в отряд.
	Leaders    []Leader
	MaxLeaders int

	// SquadCap — сколько единиц общего лимита отрядов занимает этот отряд
	// (squad_cap_ext/squad_cap_usage), SupportCap — сколько единиц лимита
	// техники (support_cap_usage): машины и техника занимают именно его.
	// Потолки обоих лимитов лежат в чертеже расы (race_squad_cap_table).
	SquadCap   int
	SupportCap int

	// MoraleMax — полная шкала морали отряда из squad_morale_ext/max
	// (у воинов 400, у скарабеев 250). 0 — таблицы нет или максимум не
	// задан: у техники и построек морали нет вовсе, и полоска не рисуется.
	// Убыль и восстановление пока не симулируются, но шкала — из чертежа.
	MoraleMax float64

	// Icon и ScreenNameID — squad_ui_ext/ui_info: иконка в панели
	// производства и идентификатор локализованного имени.
	Icon         string
	ScreenNameID string
}

// Captures — отряд умеет захватывать точки.
func (s Squad) Captures() bool { return s.CanCapture }

// Reinforceable — отряд можно пополнять после найма.
func (s Squad) Reinforceable() bool { return s.CanReinforce }

// ReadSquad читает чертёж отряда. Ошибка означает, что файла нет или он не
// разобрался; отряд без squad_loadout_ext (не бой, а окружение) возвращается
// с флагом ok=false.
func ReadSquad(tree *modapi.ModTree, path string) (squad Squad, ok bool, err error) {
	node, err := readBlueprint(tree, path)
	if err != nil {
		return Squad{}, false, err
	}
	squad = Squad{Path: path, Name: squadName(path), Race: raceOf(path)}
	loadout := node.Get("squad_loadout_ext")
	if loadout == nil {
		return squad, false, nil
	}
	if value, found := loadout.IntAt("unit_min"); found {
		squad.UnitMin = int(value)
	}
	if value, found := loadout.IntAt("unit_max"); found {
		squad.UnitMax = int(value)
	}
	for _, child := range loadout.All("trooper_base") {
		ref, found := child.StrAt("type")
		if !found || strings.TrimSpace(ref) == "" {
			continue
		}
		squad.Troopers = append(squad.Troopers, refPath(ref))
	}
	if birth := loadout.Get("birth_resource"); birth != nil {
		squad.Birth, _ = cost(birth)
	}

	if capture := node.Get("squad_capture_strategic_point_ext"); capture != nil {
		squad.CanCapture, _ = capture.BoolAt("able_to_capture")
		squad.CanUncapture, _ = capture.BoolAt("able_to_uncapture")
		if value, found := capture.FloatAt("capture_rate"); found {
			squad.CaptureRate = value
		}
	}

	if reinforce := node.Get("squad_reinforce_ext"); reinforce != nil {
		squad.CanReinforce, _ = reinforce.BoolAt("can_upgrade")
		squad.ReinforceCost, squad.ReinforceTime = cost(reinforce.Get("cost"))
	}

	if leaders := node.Get("squad_leader_ext"); leaders != nil {
		if value, found := leaders.IntAt("max_leaders"); found {
			squad.MaxLeaders = int(value)
		}
		for slot := 1; slot <= 8; slot++ {
			child := leaders.Get(fmt.Sprintf("leader_%d", slot))
			if child == nil {
				continue
			}
			ref, found := child.StrAt("leader", "type")
			if !found || strings.TrimSpace(ref) == "" {
				continue
			}
			entry := Leader{Slot: fmt.Sprintf("leader_%d", slot), Unit: refPath(ref)}
			entry.Cost, entry.TimeSeconds = cost(child.Get("cost_time"))
			squad.Leaders = append(squad.Leaders, entry)
		}
	}

	if node.Get("squad_morale_ext") != nil {
		// max — полная шкала (400 у воинов, 250 у скарабеев); default —
		// запасное значение для тех же отрядов, если max не задан.
		if value, found := numAt(node, "squad_morale_ext", "max"); found {
			squad.MoraleMax = float64(value)
		} else if value, found := numAt(node, "squad_morale_ext", "default"); found {
			squad.MoraleMax = float64(value)
		}
	}
	if value, found := node.IntAt("squad_cap_ext", "squad_cap_usage"); found {
		squad.SquadCap = int(value)
	}
	if value, found := node.IntAt("squad_cap_ext", "support_cap_usage"); found {
		squad.SupportCap = int(value)
	}
	if value, found := node.StrAt("squad_ui_ext", "ui_info", "icon_name"); found {
		squad.Icon = value
	}
	if value, found := node.StrAt("squad_ui_ext", "ui_info", "screen_name_id"); found {
		squad.ScreenNameID = value
	}
	return squad, true, nil
}

// squadName — имя файла без пути и расширения.
func squadName(path string) string {
	base := filepath.Base(strings.ReplaceAll(path, `\`, "/"))
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// raceOf берёт расу из пути вида Data/attrib/<kind>/races/<race>/...
func raceOf(path string) string {
	parts := strings.Split(strings.ToLower(strings.ReplaceAll(path, `\`, "/")), "/")
	for i, part := range parts {
		if part == "races" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}
