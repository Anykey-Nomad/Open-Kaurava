package attrib

import (
	"os"
	"testing"

	"dow-mod-studio/modapi"
)

// dowRoot — установка Dawn of War. Проверка ниже разбирает настоящие чертежи
// DoWDE: без игры она пропускается, чтобы пакет собирался где угодно.
const dowRoot = `C:\Program Files\Steam\steamapps\common\Dawn of War Definitive Edition`

// TestReadProductionTablesFromInstalledGame — номенклатура панели приказов
// берётся из чертежей: постройки строителя лежат в structure_buildable_ext,
// найм постройки — в spawner_ext/squad_table. Проверяет, что чтение этих
// таблиц действительно даёт список, а не пустоту.
func TestReadProductionTablesFromInstalledGame(t *testing.T) {
	if _, err := os.Stat(dowRoot); err != nil {
		t.Skip("установка Dawn of War не найдена")
	}
	modules, err := modapi.DiscoverModules(dowRoot)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	mod := modapi.FindModule(modules, "DoWDE")
	if mod == nil {
		t.Skip("модуль DoWDE не найден")
	}
	tree, err := modapi.BuildModTree(mod.Path, "English")
	if err != nil {
		t.Fatalf("дерево модуля: %v", err)
	}
	defer tree.Close()

	buildings := ReadBuildables(tree, "space_marines")
	if len(buildings) == 0 {
		t.Fatal("меню строительства космодесанта пусто: structure_buildable_ext не прочитан")
	}
	for _, building := range buildings {
		if !building.Buildable {
			t.Fatalf("в списке построек оказался %q без structure_buildable_ext", building.Name)
		}
	}

	hq, err := ReadUnit(tree, `Data\attrib\ebps\races\space_marines\structures\space_marine_hq.rgd`)
	if err != nil {
		t.Fatalf("чтение штаба: %v", err)
	}
	if len(hq.Spawns) == 0 {
		t.Fatal("у штаба не прочитан spawner_ext/squad_table")
	}
	for _, path := range hq.Spawns {
		if _, ok, err := ReadSquad(tree, path); err != nil || !ok {
			t.Fatalf("найм %q не читается как отряд: %v", path, err)
		}
	}
}
