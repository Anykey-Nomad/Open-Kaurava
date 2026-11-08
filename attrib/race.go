package attrib

import (
	"fmt"
	"strings"

	"dow-mod-studio/modapi"

	"github.com/Anykey-Nomad/Open-Kaurava/rgdmeta"
)

// RaceCaps — базовые лимиты расы: race_squad_cap_table из чертежа расы
// (data/attrib/racebps/<раса>_race.rgd).
//
// Отряд занимает несколько единиц общего лимита отрядов (squad_cap_usage),
// техника — единицы лимита техники (support_cap_usage). base_* — с чего
// начинает игрок, max_* — потолок; некоторые постройки поднимают текущее
// значение, но не потолок.
type RaceCaps struct {
	Path string

	MaxSquadCap    int
	MaxSupportCap  int
	BaseSquadCap   int
	BaseSupportCap int

	// UsesRequisition — race_details/uses_requesition: у некронов false,
	// у них вместо очков влияния — «тайм бонус» (Time Bonus).
	UsesRequisition bool
	// Starting — starting_res_normal: ресурсы игрока на старте.
	Starting Cost
	// IncomeCap — income_cap: потолок прироста ресурсов в секунду.
	IncomeCap Cost

	// StartingSquads — starting_squads (squad_01…squad_10): отряды, которые
	// появляются на старте вокруг постройки игрока. Обычно это строители
	// (сервитор, костоправ, грунт), у некронов — скараб, три томб-спайдера
	// и лорд некронов; последние появляются мёртвыми (см. StartingUnit.Dead).
	// Каждому отряду соответствует смещение starting_squad_offset_NN —
	// точка появления относительно стартовой позиции игрока.
	StartingSquads []StartingSquad

	// Folder — папка расы из race_path/unit_path ("Races/Space_Marines/Troops"
	// даёт "space_marines"): по ней чертёж расы сопоставляется с папками
	// ebps/sbps, имена файлов между ними не совпадают.
	Folder string
}

// StartingSquad — стартовый отряд расы: чертёж sbps и место его появления.
type StartingSquad struct {
	// Path — путь чертежа отряда в дереве модуля (Data/attrib/sbps/…).
	Path string
	// Offset — x, y, z из starting_squad_offset_NN: x и z — плоскость
	// земли, y — высота. Номер смещения совпадает с номером отряда.
	Offset [3]float32
	// HasOffset — в чертеже было starting_squad_offset_NN. Без него
	// место появления считается по кругу вокруг постройки игрока.
	HasOffset bool
}

// ReadRaceCaps читает лимиты из чертежа расы.
func ReadRaceCaps(tree *modapi.ModTree, path string) (caps RaceCaps, err error) {
	node, err := readBlueprint(tree, path)
	if err != nil {
		return RaceCaps{}, err
	}
	caps.Path = path
	table := node.Get("race_squad_cap_table")
	if table == nil {
		return caps, fmt.Errorf("attrib: в %s нет race_squad_cap_table", path)
	}
	read := func(name string) int {
		if value, found := table.IntAt(name); found {
			return int(value)
		}
		return 0
	}
	caps.MaxSquadCap = read("max_squad_cap")
	caps.MaxSupportCap = read("max_support_cap")
	caps.BaseSquadCap = read("base_squad_cap")
	caps.BaseSupportCap = read("base_support_cap")
	caps.UsesRequisition, _ = node.BoolAt("race_details", "uses_requesition")
	caps.Starting, _ = cost(node.Get("starting_res_normal"))
	caps.IncomeCap, _ = cost(node.Get("income_cap"))
	caps.Folder = folderFromRacePath(node)
	caps.StartingSquads = startingSquads(node, caps.Folder)
	return caps, nil
}

// LoadRaceCaps читает чертежи всех рас и возвращает их по папкам ebps/sbps.
func LoadRaceCaps(tree *modapi.ModTree) map[string]RaceCaps {
	out := map[string]RaceCaps{}
	if tree == nil {
		return out
	}
	for _, file := range modapi.FilesInFolder(tree, `Data\attrib\racebps`) {
		caps, err := ReadRaceCaps(tree, file.FullPath)
		if err != nil || caps.Folder == "" {
			continue
		}
		out[strings.ToLower(caps.Folder)] = caps
	}
	return out
}

// startingSquads читает starting_squads/squad_NN, приводит значения к
// путям sbps в дереве модуля и парно подставляет смещение
// starting_squad_offset_NN. Игра пишет туда и полные ссылки
// ("sbps\\races\\necrons\\necron_lord_squad.lua"), и просто имя файла
// ("space_marine_squad_servitor").
func startingSquads(root *rgdmeta.Node, folder string) []StartingSquad {
	if root == nil {
		return nil
	}
	table := root.Get("starting_squads")
	if table == nil {
		return nil
	}
	var out []StartingSquad
	for slot := 1; slot <= 10; slot++ {
		value, found := table.StrAt(fmt.Sprintf("squad_%02d", slot))
		if !found {
			continue
		}
		path := squadPath(folder, value)
		if path == "" {
			continue
		}
		entry := StartingSquad{Path: path}
		if offset := root.Get(fmt.Sprintf("starting_squad_offset_%02d", slot)); offset != nil {
			entry.Offset = [3]float32{coord(offset, "x"), coord(offset, "y"), coord(offset, "z")}
			entry.HasOffset = true
		}
		out = append(out, entry)
	}
	return out
}

// coord читает координату из таблицы position (float или int).
func coord(node *rgdmeta.Node, name string) float32 {
	if value, ok := node.FloatAt(name); ok {
		return value
	}
	if value, ok := node.IntAt(name); ok {
		return float32(value)
	}
	return 0
}

// squadPath приводит значение starting_squads к пути .rgd в дереве модуля.
func squadPath(folder, value string) string {
	p := strings.ReplaceAll(strings.TrimSpace(value), `\`, "/")
	if p == "" {
		return ""
	}
	lower := strings.ToLower(p)
	switch {
	case strings.HasPrefix(lower, "data/"):
		// путь уже от корня дерева
	case strings.HasPrefix(lower, "attrib/"):
		p = "Data/" + p
	case strings.HasPrefix(lower, "sbps/"):
		p = "Data/attrib/" + p
	default:
		p = "Data/attrib/sbps/races/" + folder + "/" + p
	}
	lastSlash := strings.LastIndexByte(p, '/')
	if dot := strings.LastIndexByte(strings.ToLower(p), '.'); dot > lastSlash {
		p = p[:dot]
	}
	if !strings.HasSuffix(strings.ToLower(p), ".rgd") {
		p += ".rgd"
	}
	return p
}

// folderFromRacePath берёт имя папки расы из race_path/unit_path либо из
// building_path, когда юнитов у расы не указано.
func folderFromRacePath(node *rgdmeta.Node) string {
	for _, key := range []string{"unit_path", "building_path", "squad_path"} {
		value, found := node.StrAt("race_path", key)
		if !found || strings.TrimSpace(value) == "" {
			continue
		}
		parts := strings.Split(strings.ReplaceAll(value, `\`, "/"), "/")
		for _, part := range parts {
			if strings.EqualFold(part, "Races") || part == "" {
				continue
			}
			return part
		}
	}
	return ""
}
