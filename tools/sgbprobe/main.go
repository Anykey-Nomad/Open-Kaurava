// Command sgbprobe is a scratch tool: it builds the file tree of a Dawn of
// War module, takes a scenario from it and loads the 3D scene the way the
// engine does (package sgbview). It exists to check the data path — tree,
// .sgb, textures, models — from a shell, without opening a window.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"dow-mod-studio/modapi"
	"dow-mod-studio/sgbview"
)

const dowRoot = `C:\Program Files\Steam\steamapps\common\Dawn of War Definitive Edition`

func main() {
	root := flag.String("root", dowRoot, "путь к установленной Dawn of War")
	module := flag.String("module", "DoWDE", "модуль по умолчанию")
	name := flag.String("scenario", "", "имя файла сценария (.sgb); пусто — первый найденный")
	flag.Parse()

	modules, err := modapi.DiscoverModules(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "discover:", err)
		os.Exit(1)
	}
	target := modapi.FindModule(modules, *module)
	if target == nil {
		fmt.Fprintln(os.Stderr, "модуль не найден:", *module)
		os.Exit(1)
	}
	fmt.Printf("модуль: %s (%s)\n", target.Name, target.Path)

	start := time.Now()
	tree, err := modapi.BuildModTree(target.Path, "English")
	if err != nil {
		fmt.Fprintln(os.Stderr, "build:", err)
		os.Exit(1)
	}
	fmt.Printf("дерево построено за %s\n", time.Since(start).Round(time.Millisecond))

	files := modapi.FilesInFolder(tree, `Data\scenarios\mp`)
	var pick *modapi.TreeFile
	for _, f := range files {
		// В папке лежат и .scar (кампании) — сценариями являются .sgb.
		if !strings.EqualFold(filepath.Ext(f.Name), ".sgb") {
			continue
		}
		if *name == "" || f.Name == *name {
			pick = f
			break
		}
	}
	if pick == nil {
		fmt.Fprintf(os.Stderr, "сценарий %q не найден (в папке %d файлов)\n", *name, len(files))
		os.Exit(1)
	}
	fmt.Printf("сценарий: %s\nпуть: %s\n", pick.Name, pick.FullPath)

	start = time.Now()
	sc, err := sgbview.Load(tree, pick.FullPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "загрузка сцены:", err)
		os.Exit(1)
	}
	fmt.Printf("сцена собрана за %s\n", time.Since(start).Round(time.Millisecond))
	fmt.Printf("  имя: %q, ячейка: %g м\n", sc.Name, sc.CellSize)
	fmt.Printf("  границы: X %g..%g, Y %g..%g, высота в центре %g\n",
		sc.GroundMin[0], sc.GroundMax[0], sc.GroundMin[1], sc.GroundMax[1], sc.CenterHeight)
	if sc.Terrain != nil {
		fmt.Printf("  рельеф: %d вершин, %d индексов\n", len(sc.Terrain.Vertices), len(sc.Terrain.Indices))
	}
	if sc.TerrainTexture != nil {
		fmt.Printf("  текстура земли: %dx%d\n", sc.TerrainTexture.Width, sc.TerrainTexture.Height)
	} else {
		fmt.Println("  текстура земли: нет")
	}
	if sc.Water != nil {
		fmt.Printf("  вода: %d вершин\n", len(sc.Water.Vertices))
	}
	fmt.Printf("  объектов: %d\n", len(sc.Objects))
	if len(sc.Skipped) > 0 {
		fmt.Printf("  пропущено: %d\n", len(sc.Skipped))
		shown := map[string]int{}
		for _, s := range sc.Skipped {
			shown[s.Name+" — "+s.Reason]++
		}
		keys := make([]string, 0, len(shown))
		for k := range shown {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("    x%-4d %s\n", shown[k], k)
		}
	}
	parts, withTex := 0, 0
	for _, o := range sc.Objects {
		for _, p := range o.Parts {
			parts++
			if p.Texture != nil {
				withTex++
			}
		}
	}
	fmt.Printf("  групп геометрии: %d (с текстурой: %d)\n", parts, withTex)
	for i, o := range sc.Objects {
		if i >= 10 {
			fmt.Println("  ...")
			break
		}
		verts := 0
		for _, p := range o.Parts {
			if p.Mesh != nil {
				verts += len(p.Mesh.Vertices)
			}
		}
		fmt.Printf("    %-48s вершин: %d\n", o.Name, verts)
	}
}
