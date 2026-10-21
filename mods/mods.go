// Package mods loads the Dawn of War modules the engine runs on.
//
// The module logic itself lives in the neighbouring dow-mod-studio project
// (its package modapi): this package wires it into the engine. It finds the
// game installation, lists the .module files found there, builds the merged
// file tree of the module in use and takes the scenarios out of that tree.
//
// The tree is built in the background: it reads the game archives, and the
// window should not wait for it.
package mods

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"dow-mod-studio/modapi"
)

// DefaultModule — модуль, который загружается по умолчанию.
const DefaultModule = "DoWDE"

// ScenarioFolder — виртуальная папка дерева модуля со сценариями.
const ScenarioFolder = `Data\scenarios\mp`

// RootEnv — переменная окружения с путём к установленной игре.
const RootEnv = "DOWDE_ROOT"

// Locale — языковая папка модуля.
const Locale = "English"

// KnownRoots — стандартные пути установки Dawn of War: Definitive Edition.
var KnownRoots = []string{
	`C:\Program Files\Steam\steamapps\common\Dawn of War Definitive Edition`,
	`C:\Program Files (x86)\Steam\steamapps\common\Dawn of War Definitive Edition`,
}

// fallbackModules — модули, которые берутся, если модуля по умолчанию нет.
var fallbackModules = []string{"W40k", "DXP3"}

// Scenario — сценарий: файл карты (.sgb) из дерева модуля.
type Scenario struct {
	// Name — имя файла, как его зовёт игра.
	Name string
	// Path — виртуальный путь файла в дереве модуля.
	Path string
	// Mod — модуль, из которого взят файл.
	Mod string
}

// State — состояние загрузки модуля.
type State int

const (
	// StateLoading — модуль грузится.
	StateLoading State = iota
	// StateReady — дерево модуля построено.
	StateReady
	// StateFailed — модуль загрузить не удалось.
	StateFailed
)

// String — короткое имя состояния для интерфейса.
func (s State) String() string {
	switch s {
	case StateReady:
		return "готов"
	case StateFailed:
		return "ошибка"
	default:
		return "загрузка"
	}
}

// Library — модуль игры: список .module-файлов, дерево файлов модуля
// по умолчанию и сценарии, взятые из этого дерева.
//
// Обращение к полям потокобезопасно: дерево строится в фоне, а сцены
// читают его каждый кадр.
type Library struct {
	mu sync.RWMutex

	root      string
	modules   []modapi.Module
	mod       *modapi.Module
	tree      *modapi.ModTree
	scenarios []Scenario
	state     State
	err       error
	elapsed   time.Duration
}

// New — загрузчик модулей. root — путь к установленной игре; пустая строка
// означает «найти самостоятельно». Загрузка идёт в фоне.
func New(root string) *Library {
	l := &Library{root: root, state: StateLoading}
	go l.load()
	return l
}

// FindRoot ищет установленную игру: указанный путь, переменная окружения,
// затем стандартные пути Steam.
func FindRoot(explicit string) (string, error) {
	candidates := make([]string, 0, len(KnownRoots)+2)
	if explicit != "" {
		candidates = append(candidates, explicit)
	}
	if env := os.Getenv(RootEnv); env != "" {
		candidates = append(candidates, env)
	}
	candidates = append(candidates, KnownRoots...)
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("игра не найдена (проверено: %s)", strings.Join(candidates, "; "))
}

// load находит игру, её модули и строит дерево модуля по умолчанию.
func (l *Library) load() {
	root, err := FindRoot(l.root)
	if err != nil {
		l.fail(err)
		return
	}
	modules, err := modapi.DiscoverModules(root)
	if err != nil {
		l.fail(fmt.Errorf("поиск .module в %s: %w", root, err))
		return
	}
	if len(modules) == 0 {
		l.fail(fmt.Errorf("в %s нет .module-файлов", root))
		return
	}
	mod := pickModule(modules)
	if mod == nil {
		l.fail(fmt.Errorf("в %s нет подходящего модуля", root))
		return
	}
	start := time.Now()
	tree, err := modapi.BuildModTree(mod.Path, Locale)
	if err != nil {
		l.fail(fmt.Errorf("дерево модуля %s: %w", mod.Name, err))
		return
	}
	scenarios := CollectScenarios(tree)

	l.mu.Lock()
	defer l.mu.Unlock()
	l.root = root
	l.modules = modules
	l.mod = mod
	l.tree = tree
	l.scenarios = scenarios
	l.elapsed = time.Since(start)
	l.state = StateReady
}

// fail запоминает ошибку загрузки.
func (l *Library) fail(err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.state = StateFailed
	l.err = err
}

// pickModule выбирает модуль по умолчанию: сперва DoWDE, затем базовые
// модули игры, затем первый найденный.
func pickModule(modules []modapi.Module) *modapi.Module {
	for _, name := range append([]string{DefaultModule}, fallbackModules...) {
		if m := modapi.FindModule(modules, name); m != nil {
			return m
		}
	}
	return &modules[0]
}

// CollectScenarios берёт из дерева модуля сценарии: файлы .sgb из папки
// Data\scenarios\mp, отсортированные по имени файла.
func CollectScenarios(tree *modapi.ModTree) []Scenario {
	if tree == nil {
		return nil
	}
	files := modapi.FilesInFolder(tree, ScenarioFolder)
	out := make([]Scenario, 0, len(files))
	for _, f := range files {
		if f == nil || !strings.EqualFold(filepath.Ext(f.Name), ".sgb") {
			continue
		}
		s := Scenario{Name: f.Name, Path: f.FullPath}
		if f.Source != nil {
			s.Mod = f.Source.ModName
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		li, lj := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if li == lj {
			return out[i].Name < out[j].Name
		}
		return li < lj
	})
	return out
}

// Root — путь к установленной игре (пусто, пока не найдена).
func (l *Library) Root() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.root
}

// Modules — список .module-файлов игры.
func (l *Library) Modules() []modapi.Module {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.modules
}

// Module — модуль, который загружен (nil, пока не загружен).
func (l *Library) Module() *modapi.Module {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.mod
}

// Tree — дерево файлов загруженного модуля (nil, пока не построено).
func (l *Library) Tree() *modapi.ModTree {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.tree
}

// Scenarios — сценарии модуля, отсортированные по имени файла.
func (l *Library) Scenarios() []Scenario {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.scenarios
}

// FindScenario ищет сценарий по имени файла (без учёта регистра).
func (l *Library) FindScenario(name string) (Scenario, bool) {
	if name == "" {
		return Scenario{}, false
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	for _, s := range l.scenarios {
		if strings.EqualFold(s.Name, name) {
			return s, true
		}
	}
	return Scenario{}, false
}

// Status — состояние загрузки, ошибка и время, потраченное на неё.
func (l *Library) Status() (State, error, time.Duration) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.state, l.err, l.elapsed
}

// FileCount — число файлов в дереве модуля.
func (l *Library) FileCount() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.tree == nil || l.tree.Root == nil {
		return 0
	}
	return countFiles(l.tree.Root)
}

// countFiles считает файлы в дереве.
func countFiles(folder *modapi.TreeFolder) int {
	if folder == nil {
		return 0
	}
	n := len(folder.Files)
	for _, c := range folder.Children {
		n += countFiles(c)
	}
	return n
}
