package scene

import (
	"fmt"

	"github.com/Anykey-Nomad/Open-Kaurava/vulkan"
)

// World3D — сцена с 3D-частью.
//
// Движок рисует 3D-сцену в render pass до интерфейса: так интерфейс
// остаётся поверх мира, а глубина считается только по 3D-объектам.
type World3D interface {
	// Record3D пишет команды отрисовки 3D-сцены в активный render pass.
	Record3D(cmd vulkan.CommandBuffer, width, height float32) error
}

// Manager — реестр сцен и переходы между ними.
//
// Переход по умолчанию идёт через экран загрузки: Switch() запоминает
// целевую сцену, Manager показывает загрузку, и по её истечении переключается.
// Мгновенный переход — SwitchNow().
type Manager struct {
	scenes map[string]Scene
	order  []string

	current string
	pending string

	// LoadTime — длительность экрана загрузки в секундах (0 — без загрузки).
	LoadTime float64
	loading  float64

	// Quit — сцена попросила закрыть приложение.
	Quit bool
}

// New создаёт менеджер сцен. loadTime — длительность экрана загрузки.
func New(loadTime float64) *Manager {
	return &Manager{
		scenes:   map[string]Scene{},
		LoadTime: loadTime,
	}
}

// Register добавляет сцену под её именем.
func (m *Manager) Register(s Scene) {
	if s == nil {
		return
	}
	if _, ok := m.scenes[s.Name()]; !ok {
		m.order = append(m.order, s.Name())
	}
	m.scenes[s.Name()] = s
}

// Names возвращает имена зарегистрированных сцен в порядке регистрации.
func (m *Manager) Names() []string { return m.order }

// Current — имя текущей сцены.
func (m *Manager) Current() string { return m.current }

// World3D возвращает 3D-часть текущей сцены (nil, если её нет).
// На экране загрузки 3D не рисуется: показывается заглушка.
func (m *Manager) World3D() World3D {
	if m.pending != "" {
		return nil
	}
	s, ok := m.scenes[m.current]
	if !ok {
		return nil
	}
	w, ok := s.(World3D)
	if !ok {
		return nil
	}
	return w
}

// SwitchNow переключает сцену немедленно.
func (m *Manager) SwitchNow(name string) error {
	if _, ok := m.scenes[name]; !ok {
		return fmt.Errorf("scene: сцена %q не зарегистрирована", name)
	}
	m.current = name
	m.pending = ""
	return nil
}

// Switch запрашивает переход на сцену. Если задан LoadTime, переход
// выполняется через экран загрузки.
func (m *Manager) Switch(name string) error {
	if _, ok := m.scenes[name]; !ok {
		return fmt.Errorf("scene: сцена %q не зарегистрирована", name)
	}
	if m.LoadTime <= 0 {
		return m.SwitchNow(name)
	}
	m.pending = name
	m.loading = m.LoadTime
	return nil
}

// Loading сообщает, что идёт экран загрузки, и имя целевой сцены.
func (m *Manager) Loading() (bool, string) {
	return m.pending != "", m.pending
}

// Update продвигает кадр: обрабатывает загрузку и обновляет текущую сцену.
func (m *Manager) Update(c *Ctx, dt float64) {
	c.Switch = func(name string) {
		if err := m.Switch(name); err != nil && c.Quit != nil {
			// Неизвестная сцена — молча игнорируем, но не роняем кадр.
			_ = err
		}
	}
	c.Quit = func() { m.Quit = true }

	if m.pending != "" {
		m.loading -= dt
		if m.loading <= 0 {
			_ = m.SwitchNow(m.pending)
		}
		return
	}
	if s, ok := m.scenes[m.current]; ok {
		s.Update(c, dt)
	}
}

// Draw рисует текущую сцену или экран загрузки.
func (m *Manager) Draw(c *Ctx) {
	if m.pending != "" {
		m.drawLoading(c)
		return
	}
	if s, ok := m.scenes[m.current]; ok {
		s.Draw(c)
	}
}

// drawLoading — экран загрузки: затемнение, подпись и полоса прогресса.
func (m *Manager) drawLoading(c *Ctx) {
	c.Rect(0, 0, float32(c.W), float32(c.H), [4]float32{0.05, 0.05, 0.08, 1})

	progress := 1.0
	if m.LoadTime > 0 {
		progress = 1 - m.loading/m.LoadTime
		if progress < 0 {
			progress = 0
		}
	}
	label := "Загрузка: " + m.pending
	c.CenterText(label, float32(c.H)/2-36, 26, [4]float32{0.92, 0.94, 0.98, 1})

	barW := float32(c.W) * 0.4
	barH := float32(6)
	x := (float32(c.W) - barW) / 2
	y := float32(c.H) / 2
	c.Rect(x, y, barW, barH, [4]float32{0.18, 0.19, 0.24, 1})
	c.Rect(x, y, barW*float32(progress), barH, [4]float32{0.35, 0.72, 1, 1})
}
