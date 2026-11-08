// Package fx — простые частицы для эффектов на карте: дым повреждённых
// построек, пыль стройки, вспышки и искры выстрелов. Симуляция живёт на
// CPU (без GPU-вычислений), а рисует её gfx3d квадами, развёрнутыми к
// камере: частица — это точка в мире, размер, цвет и время жизни.
package fx

import (
	"math"
	"math/rand"
	"time"
)

// MaxParticles — сколько частиц система держит одновременно. Старые
// выбрасываются: эффект важнее истории, а память и скорость отрисовки
// должны оставаться предсказуемыми.
const MaxParticles = 2000

// Particle — одна частица: точка в мире (Z вверх), скорость, возраст и
// цвет. Размер растёт от Size0 к Size1 за время жизни, альфа гаснет к нулю.
type Particle struct {
	Pos          [3]float64
	Vel          [3]float64
	Age, Life    float64
	Size0, Size1 float64
	Color        [4]float32
	// Additive — частица складывается с кадром (вспышка, искра), иначе
	// смешивается по альфе (дым, пыль).
	Additive bool
	// Texture — карта эффекта из его свойства Texture (DATA:ART/FX/...).
	// Пусто — у частицы нет своей карты (старые искры и дым), и рендерер
	// рисует её встроенным кругом.
	Texture string
}

// Ratio — доля прожитой жизни 0..1.
func (p Particle) Ratio() float64 {
	if p.Life <= 0 {
		return 1
	}
	ratio := p.Age / p.Life
	if ratio < 0 {
		return 0
	}
	if ratio > 1 {
		return 1
	}
	return ratio
}

// ColorAt — цвет частицы сейчас: альфа гаснет к концу жизни, яркость
// вспышек тоже (иначе искра потухает «пятном»).
func (p Particle) ColorAt() [4]float32 {
	fade := float32(1 - p.Ratio())
	color := p.Color
	color[3] *= fade
	if p.Additive {
		color[0] *= fade
		color[1] *= fade
		color[2] *= fade
	}
	return color
}

// SizeAt — размер частицы сейчас: растёт от начального к конечному.
func (p Particle) SizeAt() float64 {
	ratio := p.Ratio()
	return p.Size0 + (p.Size1-p.Size0)*ratio
}

// System — набор частиц и генератор случайности для разброса.
type System struct {
	particles []Particle
	rnd       *rand.Rand
}

// New создаёт пустую систему частиц.
func New() *System {
	return NewSeeded(time.Now().UnixNano())
}

// NewSeeded создаёт систему с заданным зерном — для тестов, где нужен
// повторяемый разброс.
func NewSeeded(seed int64) *System {
	return &System{rnd: rand.New(rand.NewSource(seed))}
}

// Len — сколько частиц живёт сейчас.
func (s *System) Len() int {
	if s == nil {
		return 0
	}
	return len(s.particles)
}

// Particles — срез живых частиц для отрисовки. Менять его нельзя.
func (s *System) Particles() []Particle {
	if s == nil {
		return nil
	}
	return s.particles
}

// Clear убирает все частицы (смена сценария, пауза на загрузке).
func (s *System) Clear() {
	if s == nil {
		return
	}
	s.particles = s.particles[:0]
}

// Add добавляет частицу, выбрасывая самую старую при переполнении.
func (s *System) Add(p Particle) {
	if s == nil || p.Life <= 0 {
		return
	}
	if len(s.particles) >= MaxParticles {
		copy(s.particles, s.particles[1:])
		s.particles = s.particles[:len(s.particles)-1]
	}
	s.particles = append(s.particles, p)
}

// Update продвигает симуляцию на dt секунд: частицы летят, дым поднимается,
// искры падают, прожившие свой срок убираются.
func (s *System) Update(dt float64) {
	if s == nil || !(dt > 0) {
		return
	}
	kept := s.particles[:0]
	for _, p := range s.particles {
		p.Age += dt
		if p.Age >= p.Life {
			continue
		}
		for axis := range p.Pos {
			p.Pos[axis] += p.Vel[axis] * dt
		}
		if p.Additive {
			// Искры и вспышки падают.
			p.Vel[2] -= 9.0 * dt
		} else {
			// Дым и пыль поднимаются и тормозят.
			p.Vel[2] += 1.2 * dt
			for axis := range p.Vel {
				p.Vel[axis] *= 1 - 0.6*dt
			}
		}
		kept = append(kept, p)
	}
	s.particles = kept
}

// SpawnSmoke — клуб дыма: поднимается, расширяется и гаснет. size — примерный
// размер клуба в метрах мира.
func (s *System) SpawnSmoke(pos [3]float64, size float64) {
	if s == nil {
		return
	}
	if size <= 0 {
		size = 2
	}
	life := 2.5 + s.rnd.Float64()*2
	s.Add(Particle{
		Pos:   jitter(pos, size*0.25, s.rnd),
		Vel:   [3]float64{s.rnd.Float64()*0.6 - 0.3, s.rnd.Float64()*0.6 - 0.3, 0.8 + s.rnd.Float64()*0.8},
		Life:  life,
		Size0: size * 0.4,
		Size1: size * 1.5,
		Color: [4]float32{0.28, 0.27, 0.26, 0.55},
	})
}

// SpawnDust — пыль стройки: светлее и живёт короче дыма.
func (s *System) SpawnDust(pos [3]float64, size float64) {
	if s == nil {
		return
	}
	if size <= 0 {
		size = 1.5
	}
	s.Add(Particle{
		Pos:   jitter(pos, size*0.4, s.rnd),
		Vel:   [3]float64{s.rnd.Float64()*1.6 - 0.8, s.rnd.Float64()*1.6 - 0.8, 0.5 + s.rnd.Float64()*0.7},
		Life:  1.2 + s.rnd.Float64()*1.2,
		Size0: size * 0.5,
		Size1: size * 1.4,
		Color: [4]float32{0.55, 0.48, 0.38, 0.45},
	})
}

// SpawnFlash — вспышка выстрела у ствола: короткая и яркая, складывается с
// кадром.
func (s *System) SpawnFlash(pos [3]float64) {
	if s == nil {
		return
	}
	s.Add(Particle{
		Pos:      pos,
		Life:     0.12,
		Size0:    0.9,
		Size1:    1.6,
		Color:    [4]float32{1.0, 0.85, 0.45, 0.9},
		Additive: true,
	})
}

// SpawnSpark — искры попадания: летят в стороны и вниз, гаснут быстро.
func (s *System) SpawnSparks(pos [3]float64, count int) {
	if s == nil || count <= 0 {
		return
	}
	for i := 0; i < count; i++ {
		s.Add(Particle{
			Pos:      pos,
			Vel:      [3]float64{s.rnd.Float64()*4 - 2, s.rnd.Float64()*4 - 2, 1.5 + s.rnd.Float64()*3},
			Life:     0.25 + s.rnd.Float64()*0.35,
			Size0:    0.35,
			Size1:    0.1,
			Color:    [4]float32{1.0, 0.7, 0.3, 0.95},
			Additive: true,
		})
	}
}

// jitter разбрасывает точку появления вокруг центра: клуб дыма не должен
// рождаться строго в одной точке.
func jitter(pos [3]float64, spread float64, rnd *rand.Rand) [3]float64 {
	if spread <= 0 {
		return pos
	}
	return [3]float64{
		pos[0] + (rnd.Float64()*2-1)*spread,
		pos[1] + (rnd.Float64()*2-1)*spread,
		pos[2] + rnd.Float64()*spread*0.5,
	}
}

// DirTo — единичный вектор от a к b; нулевой, если точки совпали.
func DirTo(a, b [3]float64) [3]float64 {
	d := [3]float64{b[0] - a[0], b[1] - a[1], b[2] - a[2]}
	length := math.Sqrt(d[0]*d[0] + d[1]*d[1] + d[2]*d[2])
	if length < 1e-6 {
		return [3]float64{}
	}
	return [3]float64{d[0] / length, d[1] / length, d[2] / length}
}
