// Command objprobe — диагностика дерева модуля: ищет файлы по подстроке
// имени и печатает их виртуальные пути. Помогает понять, есть ли в модуле
// blueprint'ы и модели игровых объектов (strategic_point, slag_deposit).
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"dow-mod-studio/modapi"
)

// dumpFile печатает printable-строки файла дерева: так видно значения
// полей .rgd (например animator) без полного разбора формата.
func dumpFile(tree *modapi.ModTree, path string) {
	f := tree.FindFile(path)
	if f == nil {
		fmt.Println("файл не найден:", path)
		return
	}
	data, err := tree.ReadTreeFile(f)
	if err != nil {
		fmt.Println("читать не удалось:", err)
		return
	}
	fmt.Printf("%s: %d байт\n", f.FullPath, len(data))
	cur := strings.Builder{}
	for _, b := range data {
		if b >= 0x20 && b < 0x7f {
			cur.WriteByte(b)
			continue
		}
		if cur.Len() >= 4 {
			fmt.Printf("  %q\n", cur.String())
		}
		cur.Reset()
	}
	if cur.Len() >= 4 {
		fmt.Printf("  %q\n", cur.String())
	}
}

const dowRoot = `C:\Program Files\Steam\steamapps\common\Dawn of War Definitive Edition`

func main() {
	root := flag.String("root", dowRoot, "путь к установленной Dawn of War")
	module := flag.String("module", "DoWDE", "модуль")
	needles := flag.String("needles", "strategic_point,slag_deposit", "подстроки имён файлов")
	limit := flag.Int("limit", 40, "сколько файлов печатать на каждую подстроку")
	dump := flag.String("dump", "", "файл дерева, содержимое которого надо показать строками")
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
	tree, err := modapi.BuildModTree(target.Path, "English")
	if err != nil {
		fmt.Fprintln(os.Stderr, "build:", err)
		os.Exit(1)
	}
	defer tree.Close()

	// Все файлы дерева — обходим дерево рекурсивно.
	if *dump != "" {
		dumpFile(tree, *dump)
		return
	}

	var all []*modapi.TreeFile
	var walk func(f *modapi.TreeFolder)
	walk = func(f *modapi.TreeFolder) {
		if f == nil {
			return
		}
		all = append(all, f.Files...)
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(tree.Root)
	fmt.Printf("файлов в дереве: %d\n", len(all))

	for _, needle := range strings.Split(*needles, ",") {
		needle = strings.TrimSpace(needle)
		if needle == "" {
			continue
		}
		var hits []*modapi.TreeFile
		for _, f := range all {
			if strings.Contains(strings.ToLower(f.Name), strings.ToLower(needle)) {
				hits = append(hits, f)
			}
		}
		sort.Slice(hits, func(i, j int) bool { return hits[i].FullPath < hits[j].FullPath })
		fmt.Printf("\n=== %q: %d файлов ===\n", needle, len(hits))
		for i, f := range hits {
			if i >= *limit {
				fmt.Println("  ...")
				break
			}
			fmt.Printf("  %s\n", f.FullPath)
		}
	}
}
