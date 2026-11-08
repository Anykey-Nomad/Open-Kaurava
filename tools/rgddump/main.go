// Command rgddump печатает чертежи Dawn of War (.rgd) из дерева модуля:
// по одному файлу — целиком, по папке — список файлов с путями.
//
// Инструмент разведки: им нужно посмотреть, какие таблицы и поля лежат в
// ebps/sbps расы, прежде чем читать их движком. Разбор берётся из rgdmeta
// (тот же код, что будет работать в игре), имена полей — из словаря RGD_DIC
// соседнего проекта dow-mod-studio-go, он читается как обычный текстовый файл.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"dow-mod-studio/modapi"
	"dow-mod-studio/ucslocale"

	"github.com/Anykey-Nomad/Open-Kaurava/attrib"
	"github.com/Anykey-Nomad/Open-Kaurava/rgdmeta"
)

// modsLocale — язык таблиц локализации (в движке это mods.Locale).
const modsLocale = "English"

const dowRoot = `C:\Program Files\Steam\steamapps\common\Dawn of War Definitive Edition`

func main() {
	root := flag.String("root", dowRoot, "путь к установленной Dawn of War")
	module := flag.String("module", "DoWDE", "модуль")
	dict := flag.String("dic", `..\dow-mod-studio-go\internal\rgd\RGD_DIC.TXT`, "словарь хэшей RGD_DIC.TXT")
	maxDepth := flag.Int("depth", 4, "на какую глубину печатать таблицы")
	race := flag.String("race", "", "сводка по расе: отряды, бойцы, постройки")
	locID := flag.String("loc", "", "показать строки локализации через запятую (без $)")
	grep := flag.String("grep", "", "найти поля с таким именем (подстрока) во всех .rgd папки")
	flag.Parse()
	if *race == "" && *grep == "" && *locID == "" && flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "укажите файл .rgd, папку, -race <раса> или -grep <поле> <папка>")
		os.Exit(2)
	}
	target := flag.Arg(0)

	modules, err := modapi.DiscoverModules(*root)
	if err != nil {
		fatal("discover: %v", err)
	}
	mod := modapi.FindModule(modules, *module)
	if mod == nil {
		fatal("модуль не найден: %s", *module)
	}
	tree, err := modapi.BuildModTree(mod.Path, "English")
	if err != nil {
		fatal("дерево модуля: %v", err)
	}
	defer tree.Close()

	if *locID != "" {
		table := ucslocale.Load(*root, "", modsLocale)
		for _, id := range strings.Split(*locID, ",") {
			id = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(id), "$"))
			fmt.Printf("$%s = %s\n", id, table.Text(id))
		}
		if flag.NArg() == 0 {
			return
		}
	}

	if *race != "" {
		raceReport(tree, *root, *race)
		return
	}

	if *grep != "" {
		if flag.NArg() == 0 {
			fatal("-grep требует папку")
		}
		grepTree(tree, loadDict(*dict), flag.Arg(0), *grep)
		return
	}

	if !strings.HasSuffix(strings.ToLower(target), ".rgd") {
		folder := tree.FindFolder(target)
		if folder == nil {
			fatal("папка не найдена: %s", target)
		}
		for _, child := range folder.Children {
			fmt.Printf("[dir]  %s\n", child.FullPath)
		}
		files := modapi.FilesInFolder(tree, target)
		for _, f := range files {
			fmt.Printf("       %s\n", f.FullPath)
		}
		fmt.Printf("— папок: %d, файлов: %d\n", len(folder.Children), len(files))
		return
	}

	data, err := tree.ReadFile(target)
	if err != nil {
		fatal("чтение %s: %v", target, err)
	}
	dictionary := loadDict(*dict)
	node, err := rgdmeta.Parse(data, dictionary)
	if err != nil {
		fatal("разбор %s: %v", target, err)
	}
	fmt.Printf("# %s\n", target)
	printNode(node, 0, *maxDepth)
}

// grepTree ищет во всех .rgd падки поля, чьё имя содержит needle, и печатает
// файл и путь до поля. Имена берутся из словаря: без него искать по имени
// не по чему.
func grepTree(tree *modapi.ModTree, dict map[uint32]string, folder, needle string) {
	root := tree.FindFolder(folder)
	if root == nil {
		fatal("папка не найдена: %s", folder)
	}
	needle = strings.ToLower(needle)
	var files []*modapi.TreeFile
	collectFiles(root, &files)
	found := 0
	for _, file := range files {
		if !strings.EqualFold(filepath.Ext(file.Name), ".rgd") {
			continue
		}
		data, err := tree.ReadTreeFile(file)
		if err != nil {
			continue
		}
		node, err := rgdmeta.Parse(data, dict)
		if err != nil {
			continue
		}
		for _, hit := range searchNames(node, "", needle, 0) {
			fmt.Printf("%s\n  %s\n", file.FullPath, hit)
			found++
		}
	}
	fmt.Printf("— совпадений: %d, файлов просмотрено: %d\n", found, len(files))
}

// collectFiles собирает все файлы папки рекурсивно.
func collectFiles(folder *modapi.TreeFolder, out *[]*modapi.TreeFile) {
	*out = append(*out, folder.Files...)
	for _, child := range folder.Children {
		collectFiles(child, out)
	}
}

// searchNames обходит дерево и возвращает пути узлов с нужным именем.
func searchNames(node *rgdmeta.Node, prefix, needle string, depth int) []string {
	if depth > 8 {
		return nil
	}
	var out []string
	for _, child := range node.Children {
		path := prefix + child.Name
		value := ""
		switch child.Type {
		case rgdmeta.TypeBool:
			value = fmt.Sprintf("%v", child.Bool)
		case rgdmeta.TypeInt:
			value = fmt.Sprintf("%d", child.Int)
		case rgdmeta.TypeFloat:
			value = fmt.Sprintf("%g", child.Float)
		case rgdmeta.TypeString, rgdmeta.TypeWString:
			value = fmt.Sprintf("%q", child.Str)
		}
		// Ищем и в имени поля, и в строковом значении, и в $REF: модификаторы
		// лежат в ссылках вида "modifiers\\squad_cap_player_modifier".
		if strings.Contains(strings.ToLower(child.Name), needle) ||
			strings.Contains(strings.ToLower(child.Str), needle) ||
			strings.Contains(strings.ToLower(child.Ref), needle) {
			if value == "" && child.Ref != "" {
				value = "$REF=" + child.Ref
			}
			out = append(out, fmt.Sprintf("%s = %s", path, value))
		}
		if child.IsTable() {
			out = append(out, searchNames(child, path+".", needle, depth+1)...)
		}
	}
	return out
}

// raceReport печатает сводку по расе: отряды (sbps) с их бойцами (ebps) и
// постройки. Именно так удобно проверять, кто умеет захватывать точки и кого
// можно добавлять в отряд.
func raceReport(tree *modapi.ModTree, root, race string) {
	loc := ucslocale.Load(root, "", modsLocale)
	name := func(id string) string {
		id = strings.TrimPrefix(strings.TrimSpace(id), "$")
		if id == "" || id == "0" {
			return ""
		}
		return loc.Text(id)
	}

	if caps := attrib.LoadRaceCaps(tree)[strings.ToLower(race)]; caps.Path != "" {
		fmt.Printf("=== %s: лимиты и ресурсы расы ===\n", race)
		fmt.Printf("  отряды: старт %d, потолок %d; техника: старт %d, потолок %d\n",
			caps.BaseSquadCap, caps.MaxSquadCap, caps.BaseSupportCap, caps.MaxSupportCap)
		fmt.Printf("  очки влияния: %v · старт: %s · потолок прироста: %s/с\n\n",
			caps.UsesRequisition, caps.Starting, caps.IncomeCap)
	}

	squads := modapi.FilesInFolder(tree, fmt.Sprintf(`Data\attrib\sbps\races\%s`, race))
	fmt.Printf("=== %s: отряды (%d) ===\n", race, len(squads))
	for _, file := range squads {
		squad, ok, err := attrib.ReadSquad(tree, file.FullPath)
		if err != nil {
			fmt.Printf("%s: %v\n", file.Name, err)
			continue
		}
		if !ok {
			continue
		}
		fmt.Printf("\n%s  %s\n", squad.Name, name(squad.ScreenNameID))
		fmt.Printf("  размер: %d–%d, занимает лимита: отряды %d, техника %d, цена найма: %s\n",
			squad.UnitMin, squad.UnitMax, squad.SquadCap, squad.SupportCap, squad.Birth)
		switch {
		case squad.CanCapture && squad.CanUncapture:
			fmt.Printf("  захват: да, снятие флага: да (скорость %g)\n", squad.CaptureRate)
		case squad.CanCapture:
			fmt.Printf("  захват: да, снятие флага: нет (скорость %g)\n", squad.CaptureRate)
		case squad.CanUncapture:
			fmt.Printf("  захват: нет, снятие флага: да (скорость %g)\n", squad.CaptureRate)
		default:
			fmt.Printf("  захват: нет\n")
		}
		if squad.CanReinforce {
			fmt.Printf("  пополнение: да, %s, %g с\n", squad.ReinforceCost, squad.ReinforceTime)
		} else {
			fmt.Printf("  пополнение: нет\n")
		}
		for _, leader := range squad.Leaders {
			fmt.Printf("  лидер %s: %s, %s, %g с\n", leader.Slot, baseName(leader.Unit), leader.Cost, leader.TimeSeconds)
		}
		for _, trooper := range squad.Troopers {
			unit, err := attrib.ReadUnit(tree, trooper)
			if err != nil {
				fmt.Printf("  бойцы: %s (не прочитан: %v)\n", trooper, err)
				continue
			}
			fmt.Printf("  бойцы: %s %s, hp %g, %s, %g с, выделение=%v\n",
				unit.Name, name(unit.ScreenNameID), unit.HitPoints, unit.Cost, unit.BuildTime, unit.Selectable)
		}
	}

	structures := modapi.FilesInFolder(tree, fmt.Sprintf(`Data\attrib\ebps\races\%s\structures`, race))
	fmt.Printf("\n=== %s: постройки (%d) ===\n", race, len(structures))
	for _, file := range structures {
		unit, err := attrib.ReadUnit(tree, file.FullPath)
		if err != nil {
			fmt.Printf("%s: %v\n", file.Name, err)
			continue
		}
		fmt.Printf("%-40s hp %-7g %s, %g с", unit.Name, unit.HitPoints, unit.Cost, unit.BuildTime)
		if unit.Income.Requisition+unit.Income.Power+unit.Income.Faith > 0 {
			fmt.Printf(" · доход: влияние %.1f/с, энергия %.1f/с, вера %.1f/с",
				unit.Income.Requisition, unit.Income.Power, unit.Income.Faith)
		}
		if unit.CapBonus != (attrib.CapBonus{}) {
			fmt.Printf(" · лимиты +%d/+%d", unit.CapBonus.Squad, unit.CapBonus.Support)
		}
		fmt.Println()
	}
}

// baseName — последний элемент пути чертежа без расширения.
func baseName(path string) string {
	p := strings.ReplaceAll(path, `\`, "/")
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		p = p[i+1:]
	}
	return strings.TrimSuffix(p, ".rgd")
}

// printNode печатает узел с отступом; таблицы глубже depth печатаются одной
// строкой, чтобы вывод оставался читаемым на больших чертежах.
func printNode(node *rgdmeta.Node, depth, maxDepth int) {
	for _, child := range node.Children {
		indent := strings.Repeat("  ", depth)
		if child.IsTable() {
			if depth >= maxDepth {
				fmt.Printf("%s%s: <таблица, %d полей>\n", indent, child.Name, len(child.Children))
				continue
			}
			line := fmt.Sprintf("%s%s:", indent, child.Name)
			if child.Ref != "" {
				line += fmt.Sprintf(" $REF=%s", child.Ref)
			}
			fmt.Println(line)
			printNode(child, depth+1, maxDepth)
			continue
		}
		fmt.Printf("%s%s = %s\n", indent, child.Name, scalar(child))
	}
	if node.Ref != "" && depth == 0 {
		fmt.Printf("$REF = %s\n", node.Ref)
	}
}

// scalar — значение узла в текстовом виде.
func scalar(node *rgdmeta.Node) string {
	switch node.Type {
	case rgdmeta.TypeBool:
		if node.Bool {
			return "true"
		}
		return "false"
	case rgdmeta.TypeInt:
		return fmt.Sprintf("%d", node.Int)
	case rgdmeta.TypeFloat:
		return fmt.Sprintf("%g", node.Float)
	case rgdmeta.TypeString, rgdmeta.TypeWString:
		return fmt.Sprintf("%q", node.Str)
	case rgdmeta.TypeNoData:
		return "<нет данных>"
	}
	return "?"
}

// loadDict читает словарь вида "0x885F1978=trooper_base". Отсутствие файла —
// не ошибка: без словаря в дампе останутся хэши.
func loadDict(path string) map[uint32]string {
	out := map[uint32]string{}
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "словарь не прочитан (%s): запасной вариант — хэши\n", err)
		return out
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, ";") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 2 {
			continue
		}
		code := strings.TrimPrefix(strings.TrimSpace(line[:eq]), "0x")
		code = strings.TrimPrefix(code, "0X")
		value, err := strconv.ParseUint(code, 16, 32)
		if err != nil {
			continue
		}
		out[uint32(value)] = strings.TrimSpace(line[eq+1:])
	}
	return out
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
