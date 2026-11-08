package scenes

import (
	"fmt"
	"github.com/Anykey-Nomad/Open-Kaurava/loc"

	"github.com/Anykey-Nomad/Open-Kaurava/scene"
	"github.com/Anykey-Nomad/Open-Kaurava/settings"
	"github.com/Anykey-Nomad/Open-Kaurava/window"
)

// Меню паузы: затемнение боя, список пунктов и экран настроек с ползунками
// громкости. Открывается кнопкой МЕНЮ на верхней панели или Escape; пока оно
// открыто, HUD не получает ввод, фабрика не тикает и анимации стоят
// (hud.paused), а музыка продолжает играть.
//
// Пункты «Перезапустить сценарий», «Сохранить игру» и «Загрузить игру» пока
// заглушки: первые два ничего не меняют, третий пишет пояснение внизу меню.
const (
	pauseItemHeight = 42
	pauseItemGap    = 8
	pauseSliderGap  = 74
	pauseSliderH    = 26
	pauseSliders    = 3
)

// pauseMenu — состояние меню паузы и его элементов.
type pauseMenu struct {
	open     bool
	settings bool
	items    []item
	sliders  [pauseSliders]slider
	apply    item
	cancel   item
	note     string
}

// slider — ползунок громкости: подпись, дорожка и текущее значение 0..1.
type slider struct {
	label    string
	x, y, w  float32
	h        float32
	value    float64
	dragging bool
}

// ensure заводит подписи элементов один раз: геометрию им считает layout.
func (m *pauseMenu) ensure() {
	if len(m.items) != 0 {
		return
	}
	m.items = []item{
		{label: loc.T("RESUME")},
		{label: loc.T("RESTART SCENARIO")},
		{label: loc.T("SAVE GAME")},
		{label: loc.T("LOAD GAME")},
		{label: loc.T("SETTINGS")},
		{label: loc.T("EXIT TO MAIN MENU")},
		{label: loc.T("EXIT TO WINDOWS")},
	}
	m.sliders = [pauseSliders]slider{
		{label: loc.T("SPEECH")},
		{label: loc.T("ACTIONS AND EFFECTS")},
		{label: loc.T("MUSIC")},
	}
	m.apply = item{label: loc.T("APPLY")}
	m.cancel = item{label: loc.T("STOP")}
}

// pauseFrame — прямоугольник панели меню по центру кадра.
func pauseFrame(c *scene.Ctx) (x, y, w, h float32) {
	w = min(float32(c.W)*0.62, 560)
	h = min(float32(c.H)*0.94, 480)
	x = (float32(c.W) - w) / 2
	y = (float32(c.H) - h) / 2
	return x, y, w, h
}

// layout раскладывает пункты, ползунки и кнопки настроек по панели.
func (m *pauseMenu) layout(c *scene.Ctx) {
	m.ensure()
	x, y, w, h := pauseFrame(c)
	btnW := w - 64
	start := y + 86
	for i := range m.items {
		m.items[i].x = x + 32
		m.items[i].y = start + float32(i)*(pauseItemHeight+pauseItemGap)
		m.items[i].w = btnW
		m.items[i].h = pauseItemHeight
	}
	sliderY := y + 108
	for i := range m.sliders {
		m.sliders[i].x = x + 32
		m.sliders[i].y = sliderY + float32(i)*pauseSliderGap
		m.sliders[i].w = btnW
		m.sliders[i].h = pauseSliderH
	}
	// Кнопки настроек — внизу панели, по половине ширины каждая.
	by := y + h - 72
	bw := (btnW - 16) / 2
	m.apply.x, m.apply.y, m.apply.w, m.apply.h = x+32, by, bw, pauseItemHeight
	m.cancel.x, m.cancel.y, m.cancel.w, m.cancel.h = x+32+bw+16, by, bw, pauseItemHeight
}

// syncSliders ставит ползунки на действующие настройки.
func (m *pauseMenu) syncSliders() {
	m.sliders[0].value = settings.Speech()
	m.sliders[1].value = settings.Effects()
	m.sliders[2].value = settings.Music()
}

// closeState забывает открытое меню (выход из игры, перезапуск сценария).
func (m *pauseMenu) closeState() {
	m.open, m.settings, m.note = false, false, ""
	for i := range m.sliders {
		m.sliders[i].dragging = false
	}
}

// openPauseMenu ставит бой на паузу и показывает меню.
func (p *Gameplay) openPauseMenu() {
	// Ставить на паузу нечего, пока карта не загружена: меню поверх экрана
	// загрузки только мешало бы.
	if p == nil || p.world == nil || !p.world.HasMeshes() {
		return
	}
	p.pause.ensure()
	p.pause.open, p.pause.settings, p.pause.note = true, false, ""
	p.pause.syncSliders()
	if p.hud != nil {
		p.hud.paused = true
	}
}

// closePauseMenu возвращает в бой.
func (p *Gameplay) closePauseMenu() {
	if p == nil {
		return
	}
	p.pause.closeState()
	if p.hud != nil {
		p.hud.paused = false
	}
}

// updatePauseMenu обрабатывает меню паузы. true — кадр занят меню: камера,
// HUD и приказы не должны получать этот ввод.
func (p *Gameplay) updatePauseMenu(c *scene.Ctx) bool {
	if p == nil {
		return false
	}
	m := &p.pause
	if !m.open {
		if c.Key(window.KeyEscape) && p.world != nil && p.world.HasMeshes() {
			p.openPauseMenu()
			return p.pause.open
		}
		return false
	}
	m.layout(c)
	if c.Key(window.KeyEscape) {
		if m.settings {
			m.settings = false
			m.syncSliders()
			m.note = ""
		} else {
			p.closePauseMenu()
		}
		return true
	}
	if m.settings {
		for i := range m.sliders {
			m.sliders[i].update(c)
		}
		m.apply.update(c)
		m.cancel.update(c)
		switch {
		case m.apply.clicked:
			settings.SetSpeech(m.sliders[0].value)
			settings.SetEffects(m.sliders[1].value)
			settings.SetMusic(m.sliders[2].value)
			p.applyMusicVolume()
			m.settings = false
			m.note = loc.T("audio settings applied")
		case m.cancel.clicked:
			m.syncSliders()
			m.settings = false
			m.note = ""
		}
		return true
	}
	for i := range m.items {
		m.items[i].update(c)
	}
	switch {
	case m.items[0].clicked: // Продолжить
		p.closePauseMenu()
	case m.items[1].clicked: // Перезапустить сценарий
		p.restartScenario(c)
	case m.items[2].clicked: // Сохранить игру
		m.note = loc.T("saving is not implemented yet")
	case m.items[3].clicked: // Загрузить игру
		m.note = loc.T("loading is not implemented yet")
	case m.items[4].clicked: // Настройки
		m.settings = true
		m.note = ""
		m.syncSliders()
	case m.items[5].clicked: // Выйти в главное меню
		p.leaveToMainMenu(c)
	case m.items[6].clicked: // Выход в Windows
		if c.Quit != nil {
			c.Quit()
		}
	}
	return true
}

// drawPauseMenu рисует затемнение, панель, пункты меню или экран настроек.
func (p *Gameplay) drawPauseMenu(c *scene.Ctx) {
	if p == nil || !p.pause.open {
		return
	}
	m := &p.pause
	m.layout(c)
	c.Rect(0, 0, float32(c.W), float32(c.H), [4]float32{0, 0, 0, 0.62})
	x, y, w, h := pauseFrame(c)
	c.Rect(x, y, w, h, ColPanel)
	c.Rect(x, y, w, 3, ColAccent)
	if m.settings {
		c.Text(loc.T("AUDIO SETTINGS"), x+32, y+56, 24, ColText)
		for i := range m.sliders {
			m.sliders[i].draw(c)
		}
		m.apply.draw(c, 16)
		m.cancel.draw(c, 16)
	} else {
		c.Text(loc.T("PAUSED"), x+32, y+56, 24, ColText)
		for i := range m.items {
			m.items[i].draw(c, 16)
		}
	}
	if m.note != "" {
		c.Text(m.note, x+32, y+h-22, 14, ColTextWarn)
	}
}

// setFromCursor ставит значение по позиции курсора на дорожке.
func (s *slider) setFromCursor(mx float32) {
	if s.w <= 0 {
		return
	}
	v := (mx - s.x) / s.w
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	s.value = float64(v)
}

// hitRow — область строки ползунка: подпись, дорожка и запас вокруг. Она
// заметно выше самой дорожки, чтобы попадать мышью без прицеливания.
func (s *slider) hitRow() (float32, float32, float32, float32) {
	return s.x - 12, s.y - 26, s.w + 24, s.h + 38
}

// update пересчитывает ползунок: клик по дорожке, перетаскивание мышью и
// колесо (шаг 5 %). Перетаскивание не зависит от события клика: оно идёт,
// пока зажата левая кнопка и курсор в строке ползунка, — так ползунок
// двигается, даже если мышь вышла из тонкой дорожки.
func (s *slider) update(c *scene.Ctx) {
	mx := float32(c.Input.MouseX)
	// Левая кнопка — бит 1 (1<<1): нумерация кнопок в Input с единицы.
	held := c.Input.Buttons&(1<<1) != 0
	hx, hy, hw, hh := s.hitRow()
	inside := c.Hover(hx, hy, hw, hh)

	if inside && c.Input.Scroll != 0 {
		s.value += c.Input.Scroll * 0.05
		if s.value < 0 {
			s.value = 0
		}
		if s.value > 1 {
			s.value = 1
		}
	}
	// Нажатие ловим по зажатой кнопке, а не по событию клика: событие
	// приходит только при отпускании и может быть съедено трекером, а
	// ползунок должен реагировать сразу.
	if inside && held {
		s.dragging = true
		s.setFromCursor(mx)
	}
	if c.ClickedAt(hx, hy, hw, hh) && !s.dragging {
		s.dragging = true
		s.setFromCursor(mx)
	}
	if !held {
		s.dragging = false
	}
	if s.dragging {
		// При зажатой кнопке значение следует за курсором, даже если он
		// вышел за пределы строки ползунка.
		s.setFromCursor(mx)
	}
}

// draw рисует подпись, процент и дорожку с заполнением.
func (s *slider) draw(c *scene.Ctx) {
	c.Text(s.label, s.x, s.y-10, 15, ColTextDim)
	percent := fmt.Sprintf("%d%%", int(s.value*100+0.5))
	c.Text(percent, s.x+s.w-c.Measure(percent, 15), s.y-10, 15, ColText)
	c.Rect(s.x, s.y, s.w, s.h, [4]float32{0.04, 0.05, 0.08, 1})
	fill := float32(s.value) * s.w
	c.Rect(s.x, s.y, fill, s.h, ColAccent)
	c.Rect(s.x+fill-3, s.y-4, 6, s.h+8, ColText)
}
