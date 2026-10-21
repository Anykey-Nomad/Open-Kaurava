package scenes

import (
	"fmt"

	"github.com/Anykey-Nomad/Open-Kaurava/mods"
	"github.com/Anykey-Nomad/Open-Kaurava/scene"
	"github.com/Anykey-Nomad/Open-Kaurava/window"
)

// Selected — сценарий, выбранный пользователем (читает геймплей).
var Selected = ""

// SelectedPath — виртуальный путь выбранного сценария в дереве модуля.
var SelectedPath = ""

// Select запоминает выбранный сценарий. Вызывается из главного потока:
// значения читают сцены каждый кадр.
func Select(name, path string) {
	Selected = name
	SelectedPath = path
}

// Scenarios — меню выбора сценария.
//
// Список сценариев берётся из дерева файлов модуля: папка Data\scenarios\mp
// загруженного модуля (по умолчанию — DoWDE). Список длинный, поэтому он
// прокручивается колесом мыши и стрелками.
type Scenarios struct {
	lib *mods.Library

	items     []item
	back      item
	scenarios []mods.Scenario

	offset int // индекс первого видимого сценария
	hover  int // сценарий под курсором (-1 — курсор не над списком)
	rows   int // сколько сценариев помещается в список
}

// NewScenarios создаёт меню выбора сценария.
func NewScenarios(lib *mods.Library) *Scenarios {
	return &Scenarios{lib: lib, hover: -1}
}

// Name — имя сцены.
func (s *Scenarios) Name() string { return "scenarios" }

// Размеры списка сценариев.
const (
	listWidth = float32(420)
	rowHeight = float32(30)
	rowGap    = float32(4)
	listTop   = float32(112)
	listLeft  = float32(scenarioMargin)
)

// Update обрабатывает прокрутку, наведение и выбор сценария.
func (s *Scenarios) Update(c *scene.Ctx, dt float64) {
	s.syncScenarios()

	// Сколько строк помещается в список на этом экране.
	avail := float32(c.H) - listTop - 96
	rows := int(avail / (rowHeight + rowGap))
	if rows < 1 {
		rows = 1
	}
	s.rows = rows

	// Колесо мыши: Scroll > 0 — вверх, значит список показывается выше.
	if c.Input.Scroll != 0 {
		s.offset -= int(c.Input.Scroll * 3)
	}
	// Стрелки двигают выделение.
	if c.Key(window.KeyDown) {
		s.hover++
	}
	if c.Key(window.KeyUp) {
		s.hover--
	}
	s.clamp()

	// Курсор над списком: находим строку под ним.
	s.hover = -1
	for i := 0; i < rows; i++ {
		idx := s.offset + i
		if idx >= len(s.items) {
			break
		}
		it := &s.items[idx]
		it.x = listLeft
		it.y = listTop + float32(i)*(rowHeight+rowGap)
		it.w = listWidth
		it.h = rowHeight
		if c.Hover(it.x, it.y, it.w, it.h) {
			s.hover = idx
			it.hover = true
		}
	}

	// Кнопка «Назад» под списком.
	s.back.label = "Назад"
	s.back.x = listLeft
	s.back.y = listTop + float32(rows)*(rowHeight+rowGap) + 16
	s.back.w = listWidth
	s.back.h = 44
	s.back.update(c)

	// Клик по строке — выбор сценария.
	for i := 0; i < rows; i++ {
		idx := s.offset + i
		if idx >= len(s.items) {
			break
		}
		it := &s.items[idx]
		if c.ClickedAt(it.x, it.y, it.w, it.h) {
			s.choose(idx)
			c.Switch("players")
			return
		}
	}
	if c.Key(window.KeyEnter) && s.hover >= 0 {
		s.choose(s.hover)
		c.Switch("players")
		return
	}
	if s.back.clicked {
		c.Switch("menu")
	}
}

// syncScenarios забирает сценарии из модуля: дерево строится в фоне, и
// список может прийти уже после того, как сцену открыли.
func (s *Scenarios) syncScenarios() {
	if s.lib == nil {
		return
	}
	scen := s.lib.Scenarios()
	if len(scen) == len(s.scenarios) {
		return
	}
	s.scenarios = scen
	s.items = s.items[:0]
	for _, sc := range scen {
		s.items = append(s.items, item{label: sc.Name})
	}
	if s.hover >= len(s.items) {
		s.hover = len(s.items) - 1
	}
	s.clamp()
}

// clamp держит прокрутку и выделение в пределах списка.
func (s *Scenarios) clamp() {
	maxOffset := len(s.items) - s.rows
	if maxOffset < 0 {
		maxOffset = 0
	}
	if s.offset > maxOffset {
		s.offset = maxOffset
	}
	if s.offset < 0 {
		s.offset = 0
	}
	if s.hover >= len(s.items) {
		s.hover = len(s.items) - 1
	}
	if s.hover >= 0 {
		if s.hover < s.offset {
			s.offset = s.hover
		}
		if s.hover >= s.offset+s.rows {
			s.offset = s.hover - s.rows + 1
		}
	}
}

// choose запоминает выбранный сценарий для геймплея.
func (s *Scenarios) choose(idx int) {
	if idx < 0 || idx >= len(s.scenarios) {
		return
	}
	Select(s.scenarios[idx].Name, s.scenarios[idx].Path)
}

// Draw рисует список сценариев, панель модуля и кнопку «Назад».
func (s *Scenarios) Draw(c *scene.Ctx) {
	c.Rect(0, 0, float32(c.W), float32(c.H), ColBG)
	c.CenterText("Выбор сценария", 120, 40, ColText)
	c.CenterText("загрузить уровень и начать игру", 156, 20, ColTextDim)

	s.drawList(c)
	s.drawPanel(c)
	s.back.draw(c, 22)

	c.CenterText("сцена: выбор сценария", float32(c.H)-36, 18, ColTextDim)
}

// drawList рисует список сценариев и полосу прокрутки.
func (s *Scenarios) drawList(c *scene.Ctx) {
	// Подложка списка.
	h := float32(s.rows) * (rowHeight + rowGap)
	c.Rect(listLeft-12, listTop-12, listWidth+24, h+24, ColPanel)

	if len(s.items) == 0 {
		c.Text("сценарии не найдены", listLeft, listTop+20, 20, ColTextDim)
		return
	}
	for i := 0; i < s.rows; i++ {
		idx := s.offset + i
		if idx >= len(s.items) {
			break
		}
		it := &s.items[idx]
		bg := [4]float32{0.09, 0.10, 0.14, 1}
		col := ColText
		if it.hover {
			bg = ColPanelHot
			col = ColAccent
		}
		c.Rect(it.x, it.y, it.w, it.h, bg)
		if it.hover {
			c.Rect(it.x, it.y, 3, it.h, ColAccent)
		}
		c.Text(it.label, it.x+12, it.y+it.h/2+6, 18, col)
	}

	// Полоса прокрутки: доля пройденного списка.
	if len(s.items) > s.rows && s.rows > 0 {
		trackH := h
		barH := trackH * float32(s.rows) / float32(len(s.items))
		if barH < 24 {
			barH = 24
		}
		barY := listTop + (trackH-barH)*float32(s.offset)/float32(maxInt(len(s.items)-s.rows, 1))
		c.Rect(listLeft+listWidth+6, listTop, 6, trackH, [4]float32{0.10, 0.11, 0.15, 1})
		c.Rect(listLeft+listWidth+6, barY, 6, barH, ColAccent)
	}
}

// drawPanel рисует панель модуля: какой модуль загружен и что в нём есть.
func (s *Scenarios) drawPanel(c *scene.Ctx) {
	const (
		panelW = float32(400)
		margin = float32(56)
	)
	x := float32(c.W) - panelW - margin
	y := listTop - 12
	h := float32(c.H) - y - 96

	c.Rect(x, y, panelW, h, ColPanel)
	c.Rect(x, y, panelW, 3, ColAccent)

	px := 18
	ty := y + 40
	line := func(s string, col [4]float32) {
		c.Text(s, x+24, ty, px, col)
		ty += 26
	}

	if s.lib == nil {
		line("модуль не загружен", ColTextDim)
		return
	}
	state, err, elapsed := s.lib.Status()
	mod := s.lib.Module()
	switch state {
	case mods.StateLoading:
		line("модуль загружается…", ColTextWarn)
	case mods.StateFailed:
		line("ошибка загрузки модуля:", ColTextWarn)
		if err != nil {
			line(err.Error(), ColTextDim)
		}
		return
	}
	if mod == nil {
		return
	}
	line("модуль: "+mod.Name, ColText)
	if mod.UIName != "" {
		line(mod.UIName, ColTextDim)
	}
	line("файл: "+mod.Path, ColTextDim)
	line(fmt.Sprintf("модулей в игре: %d", len(s.lib.Modules())), ColTextDim)
	line(fmt.Sprintf("файлов в дереве: %d", s.lib.FileCount()), ColTextDim)
	line(fmt.Sprintf("сценариев: %d", len(s.scenarios)), ColText)
	line(fmt.Sprintf("дерево собрано за %s", elapsed.Round(1e6)), ColTextDim)

	ty += 8
	line("выбрано:", ColTextDim)
	if SelectedPath != "" {
		c.Text(Selected, x+24, ty, px, ColAccent)
		ty += 26
		c.Text(SelectedPath, x+24, ty, px, ColTextDim)
	} else {
		c.Text("—", x+24, ty, px, ColTextDim)
	}

	ty += 30
	line("колесо — прокрутка, ↑↓ — выбор, Enter — выбрать", ColTextDim)
}

// maxInt — большее из двух чисел.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
