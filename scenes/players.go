package scenes

import (
	"fmt"
	"github.com/Anykey-Nomad/Open-Kaurava/loc"
	"path/filepath"

	"dow-mod-studio/racebps"
	"dow-mod-studio/sgbview"
	"dow-mod-studio/ucslocale"
	"github.com/Anykey-Nomad/Open-Kaurava/mods"
	"github.com/Anykey-Nomad/Open-Kaurava/scene"
	"github.com/Anykey-Nomad/Open-Kaurava/window"
)

// Players — экран выбора игроков и рас.
//
// Сценарий задаёт, где стоят стартовые позиции игроков, а модуль — за
// кого можно играть: список рас лежит в чертежах data/attrib/racebps,
// а названия рас — в таблицах локализации. Здесь игрок выбирает, за какой
// слот играет, и какая раса достаётся каждому слоту; после этого
// стартовые постройки расы встают на свои позиции в 3D-предпросмотре.
type Players struct {
	lib *mods.Library

	slots []sgbview.PlayerSlot
	races []racebps.Race

	// sel — выбранный слот (индекс в slots).
	sel int
	// own — номер слота игрока (1..N).
	own int
	// pick — выбор расы по номеру слота: индекс в races, -1 — случайно.
	pick map[int]int

	// path — сценарий, для которого сейчас показаны данные.
	path string
	// loaded — сценарий, данные которого уже прочитаны (или читаются).
	// Без этого сцена перечитывала бы сценарий каждый кадр, и список
	// игроков мерцал бы между «загрузка» и содержимым.
	loaded string
	// pending — канал с результатом фоновой загрузки.
	pending chan *playersData
	loading bool
	err     error

	// offset — прокрутка списка рас.
	offset int
	// rows — сколько рас помещается в список.
	rows int

	// Кнопки.
	raceItems []item
	ownItems  []item
	start     item
	back      item
}

// playersData — то, что читается в фоне: слоты сценария и расы модуля.
type playersData struct {
	slots []sgbview.PlayerSlot
	races []racebps.Race
	err   error
}

// NewPlayers создаёт экран выбора игроков.
func NewPlayers(lib *mods.Library) *Players {
	return &Players{lib: lib, sel: -1, own: 1, pick: map[int]int{}}
}

// Name — имя сцены.
func (p *Players) Name() string { return "players" }

// Размеры списков на экране.
const (
	playersTop    = float32(144)
	playersWidth  = float32(440)
	playersRowH   = float32(30)
	playersRowGap = float32(4)
	playersGapX   = float32(48)
	playersLeft   = float32(scenarioMargin)
)

// playersRight — левый край правой колонки (список рас).
func playersRight() float32 { return playersLeft + playersWidth + playersGapX }

// Update загружает данные сценария и обрабатывает выбор.
func (p *Players) Update(c *scene.Ctx, dt float64) {
	p.updateData()
	if p.err != nil {
		return
	}

	// Сколько строк помещается в списке рас.
	avail := float32(c.H) - playersTop - 190
	p.rows = int(avail / (playersRowH + playersRowGap))
	if p.rows < 1 {
		p.rows = 1
	}
	if c.Input.Scroll != 0 {
		p.offset -= int(c.Input.Scroll * 3)
	}
	if p.offset < 0 {
		p.offset = 0
	}
	if max := len(p.races) + 1 - p.rows; max > 0 && p.offset > max {
		p.offset = max
	}

	// Список слотов: слева, по одному на игрока. Клик выбирает слот,
	// которому назначается раса.
	slots := p.slots
	for i := range slots {
		x := playersLeft
		y := playersTop + float32(i)*(playersRowH+playersRowGap)
		if c.Hover(x, y, playersWidth, playersRowH) {
			p.sel = i
		}
		if c.ClickedAt(x, y, playersWidth, playersRowH) {
			p.sel = i
		}
	}

	// Стрелки двигают выделение по слотам.
	if c.Key(window.KeyDown) {
		p.sel++
	}
	if c.Key(window.KeyUp) {
		p.sel--
	}
	if p.sel < 0 {
		p.sel = 0
	}
	if p.sel >= len(slots) {
		p.sel = len(slots) - 1
	}
	if p.sel < 0 {
		p.sel = 0
	}

	// Список рас: справа. Клик назначает расу выбранному слоту.
	p.raceItems = p.raceItems[:0]
	for i := 0; i < p.rows; i++ {
		idx := p.offset + i
		if idx >= len(p.races)+1 {
			break
		}
		label := loc.T("Random race")
		if idx > 0 {
			label = p.races[idx-1].Label()
		}
		it := item{
			label: label,
			x:     playersRight(),
			y:     playersTop + float32(i)*(playersRowH+playersRowGap),
			w:     playersWidth,
			h:     playersRowH,
		}
		it.update(c)
		p.raceItems = append(p.raceItems, it)
		if it.clicked {
			p.assign(idx - 1)
		}
	}

	// Свой номер игрока: кнопки под списком слотов.
	ownTop := playersTop + float32(len(slots))*(playersRowH+playersRowGap) + 24
	p.ownItems = p.ownItems[:0]
	for i := range slots {
		it := item{
			label: fmt.Sprintf("%d", slots[i].Owner),
			x:     playersLeft + float32(i)*56,
			y:     ownTop,
			w:     48,
			h:     40,
		}
		it.update(c)
		p.ownItems = append(p.ownItems, it)
		if it.clicked {
			p.own = slots[i].Owner
			p.sel = i
		}
	}

	// Кнопки внизу.
	btnY := float32(c.H) - 96
	p.start.label = loc.T("Start")
	p.start.x = playersLeft
	p.start.y = btnY
	p.start.w = 260
	p.start.h = 48
	p.start.update(c)
	p.back.label = loc.T("Back")
	p.back.x = playersLeft + 280
	p.back.y = btnY
	p.back.w = 220
	p.back.h = 48
	p.back.update(c)

	if p.start.clicked || c.Key(window.KeyEnter) {
		p.confirm()
		c.Switch("gameplay")
		return
	}
	if p.back.clicked || c.Key(window.KeyEscape) {
		c.Switch("scenarios")
	}
}

// updateData запускает фоновую загрузку слотов и рас.
//
// Данные читаются один раз на сценарий: иначе разбор .sgb и чтение
// таблиц локализации повторялись бы каждый кадр.
func (p *Players) updateData() {
	// Сценарий сменился — прежние данные больше не подходят.
	if SelectedPath != p.path {
		p.path = SelectedPath
		p.loaded = ""
		p.slots = nil
		p.races = nil
		p.pick = map[int]int{}
		p.err = nil
		p.sel = -1
		p.loading = false
		p.pending = nil
	}
	if p.err != nil {
		return
	}
	// Забираем готовый результат фоновой загрузки.
	if p.pending != nil {
		select {
		case res := <-p.pending:
			p.pending = nil
			p.loading = false
			if res.err != nil {
				p.err = res.err
				return
			}
			p.slots = res.slots
			p.races = res.races
			p.syncPick()
		default:
		}
		return
	}
	if p.loading || p.lib == nil {
		return
	}
	path := SelectedPath
	if path == "" || path == p.loaded {
		return
	}
	tree := p.lib.Tree()
	if tree == nil {
		return // дерево модуля ещё строится — вернёмся к нему позже
	}
	ch := make(chan *playersData, 1)
	p.pending = ch
	p.loading = true
	p.loaded = path
	go func() {
		slots, err := sgbview.LoadPlayers(tree, path)
		if err != nil {
			ch <- &playersData{err: err}
			return
		}
		loc := ucslocale.Load(p.lib.Root(), p.moduleDir(), mods.Locale)
		traces, err := racebps.Load(tree, loc)
		if err != nil {
			ch <- &playersData{err: err}
			return
		}
		ch <- &playersData{slots: slots, races: playableRaces(traces)}
	}()
}

// playableRaces оставляет только те расы, за которые можно играть: расы
// NPC в списке выбора быть не должно.
func playableRaces(races []racebps.Race) []racebps.Race {
	out := make([]racebps.Race, 0, len(races))
	for _, r := range races {
		if r.Playable {
			out = append(out, r)
		}
	}
	return out
}

// moduleDir — папка модуля в установке: в ней лежат его таблицы
// локализации, и они перекрывают строки движка.
func (p *Players) moduleDir() string {
	if p.lib == nil {
		return ""
	}
	mod := p.lib.Module()
	if mod == nil {
		return ""
	}
	if mod.ModFolder != "" {
		return filepath.Join(p.lib.Root(), mod.ModFolder)
	}
	return filepath.Dir(mod.Path)
}

// syncPick готовит выбор расы по слотам: слотам без стартовой позиции
// раса не полагается, а остальным по умолчанию достаётся случайная.
func (p *Players) syncPick() {
	if p.pick == nil {
		p.pick = map[int]int{}
	}
	for _, s := range p.slots {
		if _, ok := p.pick[s.Owner]; !ok {
			p.pick[s.Owner] = -1
		}
	}
	if p.sel < 0 || p.sel >= len(p.slots) {
		p.sel = 0
	}
	if p.own < 1 {
		p.own = 1
	}
	// Свой номер игрока должен быть одним из слотов сценария.
	found := false
	for _, s := range p.slots {
		if s.Owner == p.own {
			found = true
			break
		}
	}
	if !found && len(p.slots) > 0 {
		p.own = p.slots[0].Owner
	}
}

// assign назначает расу выбранному слоту.
func (p *Players) assign(race int) {
	if p.sel < 0 || p.sel >= len(p.slots) {
		return
	}
	p.pick[p.slots[p.sel].Owner] = race
}

// confirm записывает выбор в общее состояние и разбирает случайные расы.
func (p *Players) confirm() {
	Setup.Slots = p.slots
	Setup.Races = p.races
	Setup.Pick = p.pick
	Setup.Own = p.own
	Setup.Resolve()
}

// Draw рисует списки слотов и рас, кнопки «свой игрок», «начать» и «назад».
func (p *Players) Draw(c *scene.Ctx) {
	c.Rect(0, 0, float32(c.W), float32(c.H), ColBG)
	c.CenterText(loc.T("Player selection"), 64, 40, ColText)
	c.CenterText(p.subtitle(), 94, 20, ColTextDim)

	if p.err != nil {
		c.CenterText(loc.T("failed to read the scenario"), float32(c.H)/2-20, 26, ColTextWarn)
		c.CenterText(p.err.Error(), float32(c.H)/2+16, 18, ColTextDim)
		return
	}
	if p.loading || p.lib == nil {
		c.CenterText(loc.T("loading scenario…"), float32(c.H)/2, 26, ColText)
		return
	}
	if len(p.slots) == 0 {
		if SelectedPath == "" {
			c.CenterText(loc.T("no scenario selected"), float32(c.H)/2, 26, ColTextWarn)
			c.CenterText(loc.T("go back to the scenario list and choose a map"), float32(c.H)/2+28, 18, ColTextDim)
			return
		}
		c.CenterText(loc.T("the scenario has no players"), float32(c.H)/2, 26, ColTextWarn)
		c.CenterText(loc.T("the map has no starting_position objects"), float32(c.H)/2+28, 18, ColTextDim)
		return
	}

	p.drawSlots(c)
	p.drawRaces(c)
	p.drawOwn(c)

	p.start.draw(c, 22)
	p.back.draw(c, 22)

	c.Text(p.hint(), playersLeft, float32(c.H)-136, 16, ColTextDim)
	c.CenterText(loc.T("scene: player selection"), float32(c.H)-36, 18, ColTextDim)
}

// subtitle — строка под заголовком: какой сценарий выбран и сколько в нём
// игроков.
func (p *Players) subtitle() string {
	if Selected == "" {
		return loc.T("choose a scenario from the list on the left")
	}
	return fmt.Sprintf(loc.T("%s — players: %d"), Selected, len(p.slots))
}

// drawSlots рисует список игроков сценария и выбранной для каждого расы.
func (p *Players) drawSlots(c *scene.Ctx) {
	c.Text(loc.T("Scenario players"), playersLeft, playersTop-16, 20, ColTextDim)
	if len(p.slots) == 0 {
		c.Text(loc.T("the scenario has no players"), playersLeft, playersTop+20, 18, ColTextWarn)
		return
	}
	for i, s := range p.slots {
		y := playersTop + float32(i)*(playersRowH+playersRowGap)
		bg := [4]float32{0.09, 0.10, 0.14, 1}
		col := ColText
		if i == p.sel {
			bg = ColPanelHot
			col = ColAccent
		}
		c.Rect(playersLeft, y, playersWidth, playersRowH, bg)
		if i == p.sel {
			c.Rect(playersLeft, y, 3, playersRowH, ColAccent)
		}
		c.Text(fmt.Sprintf(loc.T("Player %d"), s.Owner), playersLeft+12, y+playersRowH/2+6, 18, col)
		if s.Owner == p.own {
			c.Text(loc.T("you"), playersLeft+110, y+playersRowH/2+6, 18, ColTextWarn)
		}
		race := loc.T("random")
		if i2, ok := p.pick[s.Owner]; ok && i2 >= 0 && i2 < len(p.races) {
			race = p.races[i2].Label()
		}
		w := c.Measure(race, 18)
		c.Text(race, playersLeft+playersWidth-12-w, y+playersRowH/2+6, 18, ColText)
	}
}

// drawRaces рисует список рас модуля.
func (p *Players) drawRaces(c *scene.Ctx) {
	c.Text(loc.T("Races"), playersRight(), playersTop-16, 20, ColTextDim)
	if len(p.races) == 0 {
		c.Text(loc.T("the module has no playable races"), playersRight(), playersTop+20, 18, ColTextWarn)
		return
	}
	for i := range p.raceItems {
		it := &p.raceItems[i]
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
	// Полоса прокрутки.
	if len(p.races)+1 > p.rows {
		trackH := float32(p.rows) * (playersRowH + playersRowGap)
		barH := trackH * float32(p.rows) / float32(len(p.races)+1)
		if barH < 24 {
			barH = 24
		}
		barY := playersTop + (trackH-barH)*float32(p.offset)/float32(maxInt(len(p.races)+1-p.rows, 1))
		c.Rect(playersRight()+playersWidth+6, playersTop, 6, trackH, [4]float32{0.10, 0.11, 0.15, 1})
		c.Rect(playersRight()+playersWidth+6, barY, 6, barH, ColAccent)
	}
}

// drawOwn рисует выбор своего номера игрока.
func (p *Players) drawOwn(c *scene.Ctx) {
	if len(p.ownItems) == 0 {
		return
	}
	y := p.ownItems[0].y
	c.Text(loc.T("Your player"), playersLeft, y-24, 20, ColTextDim)
	for i := range p.ownItems {
		it := &p.ownItems[i]
		bg := ColPanel
		col := ColText
		if p.own == p.slots[i].Owner {
			bg = ColPanelHot
			col = ColAccent
		}
		if it.hover {
			bg = ColPanelHot
		}
		c.Rect(it.x, it.y, it.w, it.h, bg)
		if p.own == p.slots[i].Owner {
			c.Rect(it.x, it.y, it.w, 3, ColAccent)
		}
		tw := c.Measure(it.label, 20)
		c.Text(it.label, it.x+(it.w-tw)/2, it.y+it.h/2+7, 20, col)
	}
}

// hint — подсказка по управлению.
func (p *Players) hint() string {
	return loc.T("wheel — scroll, click a player — select a slot, click a race — assign it to the slot, Enter — start")
}
