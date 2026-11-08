package attrib

import (
	"fmt"
	"sort"
	"strings"

	"dow-mod-studio/modapi"

	"github.com/Anykey-Nomad/Open-Kaurava/rgdmeta"
)

// tableRefs читает непустые ссылки из таблицы слотов чертежа — так устроены
// spawner_ext/squad_table, research_ext/research_table и addon_ext/addons:
// поле слота (squad_01 … squad_10) с путём или пустой строкой.
//
// Слот ищется по имени через Node.Get, а не перебором Children: имена полей в
// RGD — хэши, и без словаря RGD_DIC у полей вообще нет имён (Node.Name —
// хэш), зато Get сверяет хэш имени сам.
func tableRefs(node *rgdmeta.Node, slotPrefix string, max int) []string {
	if node == nil {
		return nil
	}
	out := make([]string, 0, max)
	for slot := 1; slot <= max; slot++ {
		child := node.Get(fmt.Sprintf("%s%02d", slotPrefix, slot))
		if child == nil {
			child = node.Get(fmt.Sprintf("%s%d", slotPrefix, slot))
		}
		if child == nil {
			continue
		}
		ref, ok := child.StrAt()
		if !ok || strings.TrimSpace(ref) == "" {
			continue
		}
		if path := refPath(ref); path != "" {
			out = append(out, path)
		}
	}
	return out
}

// ReadBuildables читает постройки расы, которые попадают в меню
// строительства: у чертежа есть structure_buildable_ext. Порядок — по
// build_menu_priority (так сетку упорядочивает и сама игра), «продвинутые»
// постройки (advanced_build_option) идут после обычных.
//
// Является ли постройка доступной конкретному строителю, решают
// requirements чертежа; здесь возвращается весь номенклатурный список расы.
func ReadBuildables(tree *modapi.ModTree, race string) []Unit {
	race = strings.ToLower(strings.TrimSpace(race))
	if tree == nil || race == "" {
		return nil
	}
	files := modapi.FilesInFolder(tree, fmt.Sprintf(`Data\attrib\ebps\races\%s\structures`, race))
	units := make([]Unit, 0, len(files))
	for _, file := range files {
		unit, err := ReadUnit(tree, file.FullPath)
		if err != nil || !unit.Buildable {
			continue
		}
		units = append(units, unit)
	}
	sort.SliceStable(units, func(i, j int) bool {
		if units[i].AdvancedBuild != units[j].AdvancedBuild {
			return !units[i].AdvancedBuild
		}
		if units[i].BuildMenuPriority != units[j].BuildMenuPriority {
			return units[i].BuildMenuPriority < units[j].BuildMenuPriority
		}
		return units[i].Name < units[j].Name
	})
	return units
}
