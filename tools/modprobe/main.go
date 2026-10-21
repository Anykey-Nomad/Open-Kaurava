// Command modprobe is a scratch tool: it lists the .module files of a Dawn
// of War installation, builds the file tree of one module and prints the
// scenarios it holds. It exists to check the module plumbing from a shell.
package main

import (
	"fmt"
	"os"
	"time"

	"dow-mod-studio/modapi"
)

const dowRoot = `C:\Program Files\Steam\steamapps\common\Dawn of War Definitive Edition`

func main() {
	root := dowRoot
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	modules, err := modapi.DiscoverModules(root)
	if err != nil {
		fmt.Println("discover:", err)
		return
	}
	fmt.Printf("модулей: %d\n", len(modules))
	for _, m := range modules {
		fmt.Printf("  %-18s ui=%q ver=%q folder=%q\n", m.Name, m.UIName, m.Version, m.ModFolder)
	}

	target := modapi.FindModule(modules, "DoWDE")
	if target == nil {
		fmt.Println("модуль DoWDE не найден")
		return
	}
	start := time.Now()
	tree, err := modapi.BuildModTree(target.Path, "English")
	if err != nil {
		fmt.Println("build:", err)
		return
	}
	fmt.Printf("дерево построено за %s, папок=%d файлов=%d\n",
		time.Since(start).Round(time.Millisecond), countFolders(tree.Root), countFiles(tree.Root))
	for _, p := range []string{`Data\scenarios\mp`, `Data\scenarios\MP`, `Data\scenarios`} {
		f := modapi.FilesInFolder(tree, p)
		fmt.Printf("папка %q: %d файлов\n", p, len(f))
		for i, file := range f {
			if i >= 20 {
				fmt.Println("  ...")
				break
			}
			fmt.Printf("  %-40s src=%s prio=%d sga=%v\n", file.Name, file.Source.ModName, file.Source.Priority, file.Source.IsSGA)
		}
	}
}

func countFolders(f *modapi.TreeFolder) int {
	n := len(f.Children)
	for _, c := range f.Children {
		n += countFolders(c)
	}
	return n
}

func countFiles(f *modapi.TreeFolder) int {
	n := len(f.Files)
	for _, c := range f.Children {
		n += countFiles(c)
	}
	return n
}
