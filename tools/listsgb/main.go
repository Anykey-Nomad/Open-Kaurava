package main

import (
	"fmt"
	"strings"

	"dow-mod-studio/modapi"
)

func main() {
	modules, _ := modapi.DiscoverModules(`C:\Program Files\Steam\steamapps\common\Dawn of War Definitive Edition`)
	target := modapi.FindModule(modules, "DoWDE")
	tree, err := modapi.BuildModTree(target.Path, "English")
	if err != nil {
		fmt.Println("err", err)
		return
	}
	defer tree.Close()
	files := modapi.FilesInFolder(tree, `Data\scenarios\mp`)
	n := 0
	for _, f := range files {
		if !strings.EqualFold(f.Name[strings.LastIndex(f.Name, ".")+1:], "sgb") {
			continue
		}
		n++
		if true {
			fmt.Printf("%-44s %s\n", f.Name, f.FullPath)
		}
	}
	fmt.Println("всего .sgb:", n)
}
