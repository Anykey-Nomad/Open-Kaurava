package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func russian(s string) bool {
	for _, r := range s {
		if (r >= 'А' && r <= 'я') || r == 'Ё' || r == 'ё' {
			return true
		}
	}
	return false
}

func main() {
	root := os.Args[1]
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil
		}
		for _, lit := range f.Comments {
			_ = lit
		}
		// обходим литералы через токены исходника
		src, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		f2, err := parser.ParseFile(fset, path, src, parser.ParseComments)
		if err != nil {
			return nil
		}
		var visit func(n interface{})
		_ = visit
		ast.Inspect(f2, func(n ast.Node) bool {
			bl, ok := n.(*ast.BasicLit)
			if !ok || bl.Kind != token.STRING {
				return true
			}
			val, err := strconv.Unquote(bl.Value)
			if err != nil || !russian(val) {
				return true
			}
			pos := fset.Position(bl.Pos())
			fmt.Printf("%s:%d: %s\n", filepath.ToSlash(pos.Filename), pos.Line, bl.Value)
			return true
		})
		return nil
	})
}
