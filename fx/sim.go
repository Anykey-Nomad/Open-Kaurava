package fx

import "math"

// Симуляция одного эффекта: разобранный файл превращается в распылители
// (STYLE_SPRAY) и «висящие» узлы, а шаг по времени рождает частицы и
// выдаёт спрайты — то, что рисует gfx3d.
//
// Полной копии движка игры здесь и не нужно: задачей было, чтобы эффект
// читался как тот же самый. Распылитель выпускает частицы вдоль своего
// направления с частотой из Emitter_Rate, каждая живёт столько, сколько
// задано листу (Duration), растёт по кривой Radius и гаснет по градиенту
// Colour, а всё это зациклено — эффект играет, пока его не выключат.
//
// Порт из dow-mod-studio (internal/fx/sim.go): логика и порядок чтения
// свойств те же, но без fauxgl (свой Vec3) и без лент/колец — те придут
// отдельным шагом вместе с beam.go.

// simMaxParticles — предел частиц одного эффекта. Файлы игры просят и
// тысячи в секунду, а для кадра этого с запасом: переполнение рвёт поток
// ровно так же, как его рвёт игра, — новые частицы просто не появляются.
const simMaxParticles = 400

// Visual — то, как выглядит одна частица: лист эффекта, из которого она
// родилась.
type Visual struct {
	Texture  string
	Blending int
	Lifetime float64
	Speed    float64
	Radius   []Key
	Colour   []ColourStop
	// SelfLit — свой SelfIlluminated узла: такая частица сама себе свет и
	// не гаснет вместе со сценой, а без флага её притеняет свет модели.
	SelfLit bool
	// Scale — размер листа за время жизни, SpeedCurve — скорость запуска.
	// Обе пишутся на листе, а не на распылителе, поэтому читаются с
	// ребёнка; если лист своих не несёт, он наследует кривые распылителя.
	Scale      []Key
	SpeedCurve []Key
}

// Emitter — один узел STYLE_SPRAY со своими счётчиками.
type Emitter struct {
	Rate      float64 // частиц в секунду, первая точка Emitter_Rate
	RateCurve []Key
	Duration  float64
	Loop      bool
	Drag      float64
	Dev       float64 // половина угла конуса в радианах
	Visuals   []Visual
	// Volume — половина размеров коробки, внутри которой рождаются частицы,
	// Offset — смещение её центра в системе распылителя. Обе записаны
	// векторными кривыми, поэтому читаются как любая анимация.
	Volume []Vec3Key
	Offset []Vec3Key
	// Speed — кривая скорости запуска: распылитель гасит её за время
	// жизни, и именно это заставляет облако разлететься и зависнуть.
	Speed   []Key
	Scale   []Key
	Gravity float64

	// owned — пути узлов, которые распылитель забрал себе как частицы:
	// обход обязан их перешагнуть, иначе тот же клуб нарисуется дважды —
	// летящим и стоящим на месте.
	Owned map[string]bool

	Age        float64 // секунды внутри текущего цикла выпуска
	Acc        float64 // дробный счётчик частиц
	Cycle      int
	SpawnIndex int // стабильный номер для случайностей частицы
}

// owns — забрал ли распылитель узел как свою частицу.
func (e *Emitter) owns(n *Node) bool {
	return e.Owned != nil && e.Owned[n.Effect]
}

// claim — отметить, что узел стал частицей этого распылителя.
func (e *Emitter) claim(n *Node) {
	if e.Owned == nil {
		e.Owned = map[string]bool{}
	}
	e.Owned[n.Effect] = true
}

// SimParticle — одна частица в полёте, в системе самого эффекта (Y вверх).
type SimParticle struct {
	V        Visual
	Drag     float64
	grav     float64
	Pos      Vec3
	Vel      Vec3
	Age      float64
	SpriteID int
}

// Simulation играет один файл эффекта. Координаты — в системе файла
// (Y вверх), относительно начала эффекта: маркер модели переносит их в мир.
type Simulation struct {
	Emitters []Emitter
	// NoRotate — флаг No_Rotate корневого узла: с ним эффект сохраняет
	// свою систему вместо того, чтобы наследовать поворот маркера.
	NoRotate bool
	// Axis — заданное направление выпуска в системе файла.
	Axis      Vec3
	Anchored  []Visual // листья, висящие в начале координат сами по себе
	Particles []SimParticle
	// Ribbons — ленты STYLE_BEAM и кольца STYLE_RING: они не частицы, а
	// гнущиеся четырёхугольники со своим временем жизни.
	Ribbons []RibbonNode
	Time    float64

	spriteBuf []Sprite
	frameBuf  []Particle
	quadBuf   []Quad
}

// RibbonNode — один узел ленты или кольца в симуляции.
//
// Index — номер кольца среди таких же узлов: по нему считается доля разброса
// SpinEpsilon, поэтому панели кристалла стоят под разными углами, а не
// сходятся в один плоский лист.
type RibbonNode struct {
	Beam  *Beam
	Ring  *Ring
	Index int
	Age   float64
}

// lifetime — сколько живёт узел: Duration ленты или кольца.
func (r *RibbonNode) lifetime() float64 {
	switch {
	case r.Beam != nil:
		return r.Beam.Lifetime
	case r.Ring != nil:
		return r.Ring.Lifetime
	}
	return 0
}

// loops — зациклен ли узел. Ленты в эффекте играют всё время, пока он
// включён; кольцо решает по своему Loop.
func (r *RibbonNode) loops() bool {
	if r.Beam != nil {
		return true
	}
	return r.Ring != nil && r.Ring.Loop
}

// NewSimulation обходит разобранный файл и готовит распылители и висящие
// эффекты.
func NewSimulation(f *File) *Simulation {
	s := &Simulation{}
	if f == nil {
		return s
	}
	root := f.Root()
	if root == nil {
		return s
	}
	s.NoRotate = root.Bool("No_Rotate", false)
	if p, ok := root.Prop("Emitter_Direction"); ok {
		if axis := vecNorm(p.Tuple); vecLen(p.Tuple) > 1e-6 {
			s.Axis = axis
		}
	}
	// add читает один узел, заходя внутрь комбо, которые держат другие.
	var add func(n *Node)
	var walk func(path string)
	add = func(n *Node) {
		switch n.Style {
		case "STYLE_SPRAY":
			e := emitterFromNode(n, f)
			s.Emitters = append(s.Emitters, e)
			// Распылитель может держать поддерево, и не всегда только
			// листья: бывает распылитель с комбо внутри, у которого свой
			// ребёнок — луч. То, что распылитель уже забрал как частицу,
			// дальше не идёт: лист под распылителем описывает клуб, а не
			// отдельную фигуру.
			for _, c := range f.Children(n.Effect) {
				if !e.owns(c) {
					add(c)
				}
			}
		case "STYLE_COMBO":
			walk(n.Effect)
		case "STYLE_BEAM":
			// Лента тянется между двумя точками, и спрайтом её не нарисовать:
			// в кадре она отдаёт четырёхугольники (см. Quads).
			if b, ok := beamFromNode(n); ok {
				s.Ribbons = append(s.Ribbons, RibbonNode{Beam: &b, Index: len(s.Ribbons)})
			}
		case "STYLE_RING":
			// Кольцо — плоский четырёхугольник в плоскости маркера. Прочитанное
			// спрайтом, оно висело бы в начале координат: его место задаёт
			// Offset. Но под распылителем кольцо — частица, и туда эта ветка не
			// доходит: распылитель забирает лист себе.
			if ring, ok := RingFromNode(n); ok {
				s.Ribbons = append(s.Ribbons, RibbonNode{Ring: &ring, Index: len(s.Ribbons)})
			}
		default:
			if v, ok := visualFromNode(n); ok {
				s.Anchored = append(s.Anchored, v)
			}
		}
	}
	walk = func(path string) {
		for _, n := range f.Children(path) {
			add(n)
		}
	}
	// Файл из одного узла без собственного пути: этот узел и есть весь
	// эффект, искать его среди детей "root" нечего. Корень-распылитель
	// читается так же: его дети — частицы, а не отдельные фигуры, и обход
	// одних лишь детей оставил бы эффект без единого распылителя.
	// Файл из одного узла-фигуры (лента, кольцо, спрайт) без детей: этот
	// узел и есть весь эффект, искать его среди детей "root" нечего.
	leafRoot := (root.Style == "STYLE_BEAM" || root.Style == "STYLE_RING") && len(f.Children(root.Effect)) == 0
	if f.Single || root.Style == "STYLE_SPRAY" || leafRoot {
		add(root)
		return s
	}
	walk(root.Effect)
	return s
}

// emitterFromNode читает узел STYLE_SPRAY и листья, которые он распыляет.
func emitterFromNode(n *Node, f *File) Emitter {
	e := Emitter{
		Rate:     firstKey(n, "Emitter_Rate"),
		Duration: n.Float("Emitter_Duration", 1),
		Loop:     n.Bool("Emitter_Loop", true),
		Drag:     n.Float("Emitter_Drag", 1),
		Gravity:  n.Float("Particle_Gravity", 0),
	}
	e.RateCurve = curveOf(n, "Emitter_Rate")
	// Отклонение редактора — половина угла конуса в градусах: у файлов
	// бывает и 0 (струя), и 180 (всплеск). Записано кривой, поэтому
	// берётся первая точка.
	dev := firstKey(n, "Emitter_Deviation")
	if dev == 0 {
		dev = n.Float("Emitter_Deviation", 8)
	}
	e.Dev = dev * math.Pi / 180
	e.Volume = vec3CurveOf(n, "Emitter_Volume")
	e.Offset = vec3CurveOf(n, "Emitter_Offset")
	// Распылитель описывает частицы на себе, а лист — только их вид.
	// Где у листа своей кривой нет, действует кривая распылителя.
	e.Scale = curveOf(n, "Particle_Scale")
	e.Speed = curveOf(n, "Particle_Speed")
	for _, c := range f.Children(n.Effect) {
		if v, ok := particleFromNode(c); ok {
			v.Scale = curveOf(c, "Particle_Scale")
			v.SpeedCurve = curveOf(c, "Particle_Speed")
			e.adopt(&v)
			e.Visuals = append(e.Visuals, v)
			e.claim(c)
		} else if c.Style == "STYLE_COMBO" {
			for _, cc := range f.Children(c.Effect) {
				if v, ok := particleFromNode(cc); ok {
					v.Scale = curveOf(cc, "Particle_Scale")
					v.SpeedCurve = curveOf(cc, "Particle_Speed")
					e.adopt(&v)
					e.Visuals = append(e.Visuals, v)
					e.claim(cc)
				}
			}
		}
	}
	return e
}

// adopt докладывает листу скорость запуска распылителя, если своей у него
// нет: Particle_Speed пишется на распылителе, и лист с одним радиусом
// иначе полетел бы со встроенным умолчанием, а не со скоростью файла.
func (e *Emitter) adopt(v *Visual) {
	if len(v.SpeedCurve) == 0 {
		v.SpeedCurve = e.Speed
	}
	if len(v.Scale) == 0 {
		v.Scale = e.Scale
	}
	if len(v.SpeedCurve) > 0 {
		v.Speed = SampleCurve(v.SpeedCurve, 0)
	}
}

// curveOf читает анимационное свойство в виде кривой.
func curveOf(n *Node, name string) []Key {
	p, ok := n.Prop(name)
	if !ok || len(p.Curve) == 0 {
		return nil
	}
	return p.Curve
}

// vec3CurveOf читает векторное свойство: файлы пишут его кривой
// время/вектор, как Arc у ленты.
func vec3CurveOf(n *Node, name string) []Vec3Key {
	p, ok := n.Prop(name)
	if !ok {
		return nil
	}
	return p.Vec3
}

// vec3Sample берёт вектор из кривой в нормализованном возрасте.
func vec3Sample(keys []Vec3Key, u float64) (Vec3, bool) {
	if len(keys) == 0 {
		return Vec3{}, false
	}
	if len(keys) == 1 {
		return Vec3{X: keys[0].X, Y: keys[0].Y, Z: keys[0].Z}, true
	}
	u = clamp(u, 0, 1)
	span := keys[len(keys)-1].T - keys[0].T
	if span <= 0 {
		return Vec3{X: keys[0].X, Y: keys[0].Y, Z: keys[0].Z}, true
	}
	t := keys[0].T + u*span
	for i := 1; i < len(keys); i++ {
		if t <= keys[i].T {
			a, b := keys[i-1], keys[i]
			if b.T <= a.T {
				return Vec3{X: b.X, Y: b.Y, Z: b.Z}, true
			}
			f := (t - a.T) / (b.T - a.T)
			return Vec3{
				X: a.X + f*(b.X-a.X),
				Y: a.Y + f*(b.Y-a.Y),
				Z: a.Z + f*(b.Z-a.Z),
			}, true
		}
	}
	last := keys[len(keys)-1]
	return Vec3{X: last.X, Y: last.Y, Z: last.Z}, true
}

// emitterPhase переводит возраст распылителя в 0..1 — туда, где живут его
// кривые частоты, объёма и смещения.
func emitterPhase(e *Emitter) float64 {
	if e == nil || e.Duration <= 0 {
		return 0
	}
	return clamp(e.Age/e.Duration, 0, 1)
}

// visualFromNode читает свойства, которые нужны спрайту.
//
// Стиль узла говорит, какая это фигура сама по себе, но под STYLE_SPRAY он
// не значит ничего: любой ребёнок распылителя описывает частицу, и самый
// частый из них — STYLE_RING, то кольцо, в которое разворачивается всплеск.
// Отказать кольцам здесь — оставить такие распылители без частиц вовсе.
//
// Только то, что требует от распылителя больше, чем он может дать, не
// бывает частицей: распылитель и комбо — контейнеры, а не вид. Луч — особый
// случай, и поэтому particleFromNode ниже его принимает, а эта функция нет.
func visualFromNode(n *Node) (Visual, bool) {
	// Под распылителем STYLE_RING — обычная частица, и отказывать ему здесь
	// нельзя: таких листьев у эффектов игры больше всего. Отдельно стоящее
	// кольцо до этой функции не доходит — его забирает ветка STYLE_RING в
	// NewSimulation, где оно становится плоскостью со спинами.
	if n.Style == "STYLE_SPRAY" || n.Style == "STYLE_COMBO" || n.Style == "STYLE_BEAM" {
		return Visual{}, false
	}
	v := Visual{
		Texture:  n.String("Texture"),
		Blending: int(n.Float("Blending", 2)),
		Lifetime: n.Float("Duration", 1),
		Speed:    n.Float("Particle_Speed", 2),
		SelfLit:  n.Bool("SelfIlluminated", false),
	}
	if p, ok := n.Prop("Radius"); ok {
		v.Radius = p.Curve
	}
	if p, ok := n.Prop("Colour"); ok {
		v.Colour = p.Colour
	}
	if v.Lifetime <= 0 {
		v.Lifetime = 1
	}
	return v, true
}

// particleFromNode читает, как выглядит одна частица распылителя, — любой
// его лист, включая STYLE_BEAM.
//
// Под распылителем луч — не лента: второго маркера там никто не называет,
// и тянуться ему некуда. Движок рождает вместо этого короткий светящийся
// отрезок на частицу, и от собственных свойств у него остаются только
// толщина, яркость и срок жизни — те же четыре числа, что нужны спрайту.
// Поэтому луч читается спрайтом ширины Width, а Length, Segments, Arc и
// пара маркеров остаются в стороне: они описывают ленту, которой этот узел
// был бы на корне, а не то, что рисуется.
func particleFromNode(n *Node) (Visual, bool) {
	if n.Style != "STYLE_BEAM" {
		return visualFromNode(n)
	}
	v := Visual{
		Texture:  n.String("Texture"),
		Blending: int(n.Float("Blending", 2)),
		Lifetime: n.Float("Duration", 1),
		SelfLit:  n.Bool("SelfIlluminated", false),
	}
	// Width — половина толщины ленты, то же самое, что радиус спрайта:
	// насколько свечение отходит от оси частицы.
	if p, ok := n.Prop("Width"); ok {
		v.Radius = p.Curve
	}
	if p, ok := n.Prop("Colour"); ok {
		v.Colour = p.Colour
	}
	if v.Lifetime <= 0 {
		v.Lifetime = 1
	}
	return v, true
}

// firstKey — начальное значение кривой: частота, с которой распылитель
// начинает.
func firstKey(n *Node, name string) float64 {
	p, ok := n.Prop(name)
	if !ok || len(p.Curve) == 0 {
		return 0
	}
	return p.Curve[0].V
}

// Reset обнуляет симуляцию: эффект начинается заново. Так игра поступает
// с эффектами клипа при его смене — событие срабатывает в кадре начала, и
// свет, горевший минуту, не появился бы по новому требованию.
func (s *Simulation) Reset() {
	s.Particles = nil
	s.Time = 0
	for i := range s.Ribbons {
		s.Ribbons[i].Age = 0
	}
	for i := range s.Emitters {
		s.Emitters[i].Age = 0
		s.Emitters[i].Acc = 0
		s.Emitters[i].Cycle = 0
		s.Emitters[i].SpawnIndex = 0
	}
}

// Step продвигает симуляцию на dt секунд.
func (s *Simulation) Step(dt float64) {
	if s == nil || dt <= 0 {
		return
	}
	if dt > 0.1 {
		dt = 0.1 // зависание кадра не должно телепортировать частицы
	}
	s.Time += dt
	for ei := range s.Emitters {
		e := &s.Emitters[ei]
		if len(e.Visuals) == 0 || (e.Rate <= 0 && len(e.RateCurve) == 0) {
			continue
		}
		e.Age += dt
		if e.Duration > 0 && e.Age >= e.Duration {
			if !e.Loop {
				continue
			}
			e.Age = math.Mod(e.Age, e.Duration)
			e.Cycle++
			e.Acc = 0
		}
		phase := emitterPhase(e)
		rate := e.Rate
		if len(e.RateCurve) > 0 {
			rate = SampleCurve(e.RateCurve, phase)
		}
		if rate <= 0 {
			continue
		}
		e.Acc += rate * dt
		for e.Acc >= 1 {
			e.Acc--
			if len(s.Particles) >= simMaxParticles {
				break
			}
			// Выпуск идёт за анимированным положением распылителя:
			// выборка на каждом шаге сохраняет заданную вертикальную
			// струю, а не прибивает все частицы к смещению при t=0.
			s.Spawn(e)
		}
	}
	kept := s.Particles[:0]
	for i := range s.Particles {
		p := &s.Particles[i]
		p.Age += dt
		if p.Age >= p.V.Lifetime {
			continue
		}
		damp := math.Pow(clamp(p.Drag, 0.01, 1), dt)
		p.Vel.X *= damp
		p.Vel.Y *= damp
		p.Vel.Z *= damp
		// Частицы живут в системе файла (Y вверх), как его Volume и Offset,
		// поэтому гравитация тянет по Y, а поворот маркера наклоняет её
		// вместе с собой. Particle_Gravity — ускорение в единицах модели в
		// секунду: у файлов оно либо 0, либо ровно 1, и единица в секунду
		// мягка против скоростей запуска в несколько единиц в секунду.
		p.Vel.Y -= p.grav * dt
		p.Pos.X += p.Vel.X * dt
		p.Pos.Y += p.Vel.Y * dt
		p.Pos.Z += p.Vel.Z * dt
		kept = append(kept, *p)
	}
	s.Particles = kept
	for i := range s.Ribbons {
		r := &s.Ribbons[i]
		life := r.lifetime()
		if life <= 0 {
			continue
		}
		r.Age += dt
		if r.Age < life {
			continue
		}
		if r.loops() {
			r.Age = math.Mod(r.Age, life)
		} else {
			r.Age = life
		}
	}
}

// Spawn рождает одну частицу внутри заданного конуса распылителя.
func (s *Simulation) Spawn(e *Emitter) {
	spriteID := e.SpawnIndex
	e.SpawnIndex++
	v := e.Visuals[(e.Cycle+spriteID)%len(e.Visuals)]
	// Детерминированные псевдослучайные числа: кадр, который отличается от
	// запуска к запуску, труднее сравнивать, а зерно ничего не стоит.
	rnd := func(salt, mask uint32) float64 {
		h := uint32(spriteID)*salt + uint32(e.Cycle)*22695477
		return float64((h>>8)&mask) / float64(mask)
	}
	ang := rnd(1103515245, 0xffff) * 2 * math.Pi
	tilt := e.Dev * rnd(104729, 0xff)
	dirX := math.Sin(tilt) * math.Cos(ang)
	dirY := math.Sin(tilt) * math.Sin(ang)
	dirZ := math.Cos(tilt)
	axis := s.Axis
	if vecLen(axis) < 1e-6 {
		// Эффекты написаны с Y вверх: распылитель, не назвавший своего
		// направления, выпускает вверх, а рамка уже поворачивает это
		// вместе с моделью.
		axis = Vec3{Y: 1}
	}
	axis = vecNorm(axis)
	help := Vec3{Z: 1}
	if math.Abs(axis.Z) > 0.95 {
		help = Vec3{Y: 1}
	}
	right := vecNorm(vecCross(help, axis))
	up := vecNorm(vecCross(axis, right))
	direction := vecAdd(vecAdd(vecScale(right, dirX), vecScale(up, dirY)), vecScale(axis, dirZ))
	// Частица рождается внутри коробки распылителя, а не в её центре:
	// облако пыли, разложенное по Emitter_Volume, — вся разница между
	// клубом и объёмом. Оба вектора остаются в системе файла (Y вверх);
	// рамка крепления укладывает их в модель, и перевод здесь сломал бы
	// вертикаль.
	phase := emitterPhase(e)
	vol, volOK := vec3Sample(e.Volume, phase)
	off, offOK := vec3Sample(e.Offset, phase)
	var pos Vec3
	if volOK {
		pos.X += (rnd(22695477, 0xffff)*2 - 1) * vol.X
		pos.Y += (rnd(374761393, 0xffff)*2 - 1) * vol.Y
		pos.Z += (rnd(668265263, 0xffff)*2 - 1) * vol.Z
	}
	if offOK {
		pos.X += off.X
		pos.Y += off.Y
		pos.Z += off.Z
	}
	// Скорость запуска идёт по кривой листа за время его жизни: всплеск,
	// который стартует быстро и замедляется, зависает там, где велел файл.
	speed := v.Speed
	if len(v.SpeedCurve) > 0 {
		speed = SampleCurve(v.SpeedCurve, 0)
	}
	s.Particles = append(s.Particles, SimParticle{
		V:        v,
		Drag:     e.Drag,
		grav:     e.Gravity,
		Pos:      pos,
		Vel:      vecScale(direction, speed),
		SpriteID: spriteID,
	})
}

// Sprite — то, что рисуется в этом кадре: билборд в системе файла (Y вверх)
// с размером в единицах модели. Маркер, к которому прикреплён эффект,
// переносит его в пространство модели.
type Sprite struct {
	Pos      Vec3
	Radius   float64
	R, G, B  float64
	A        float64
	Texture  string
	Additive bool
	// SelfLit — свой SelfIlluminated листа: рендерер по нему решает, слушается
	// ли частица света сцены.
	SelfLit bool
}

// Sprites возвращает текущий кадр эффекта: все живые частицы плюс висящие
// эффекты, которые зациклены по часам симуляции.
func (s *Simulation) Sprites() []Sprite {
	out := s.spriteBuf[:0]
	if cap(out) < len(s.Particles)+len(s.Anchored) {
		out = make([]Sprite, 0, len(s.Particles)+len(s.Anchored))
	}
	for i := range s.Particles {
		p := &s.Particles[i]
		u := p.Age / p.V.Lifetime
		out = append(out, spriteFor(p.V, u, p.Pos))
	}
	for i, v := range s.Anchored {
		u := math.Mod(s.Time+float64(i)*0.37, v.Lifetime) / v.Lifetime
		out = append(out, spriteFor(v, u, Vec3{}))
	}
	s.spriteBuf = out
	return out
}

// Quads отдаёт четырёхугольники лент и колец этого кадра. Координаты — в
// системе файла (Y вверх), как у спрайтов: маркер эффекта переносит их в
// модель. eye — глаз камеры в той же системе: по нему лента поворачивает
// сечение, а билборд-кольцо разворачивается к зрителю.
func (s *Simulation) Quads(eye Vec3) []Quad {
	return s.QuadsAt(eye, Vec3{}, Vec3{X: 1}, false)
}

// QuadsAt — те же квады, но с концами из события эффекта: лента тянется от
// from к to, а spans говорит, что Length — доля зазора, а не длина в
// единицах модели. Так выстрел с fx_other=target заканчивается на цели, а
// промах — точкой мимо неё. from и to — в системе файла (Y вверх).
func (s *Simulation) QuadsAt(eye, from, to Vec3, spans bool) []Quad {
	if s == nil {
		return nil
	}
	out := s.quadBuf[:0]
	for i := range s.Ribbons {
		r := &s.Ribbons[i]
		u := r.phase()
		if r.Beam != nil {
			// Без второго конца Length — длина в единицах модели, а
			// направление берётся у эффекта.
			end := to
			if end.Sub(from).Length() < 1e-6 {
				end = from.Add(s.Axis)
			}
			if end.Sub(from).Length() < 1e-6 {
				end = from.Add(Vec3{X: 1})
			}
			ribbon := NewBeamRibbon(*r.Beam, from, end, u, spans)
			out = append(out, RibbonQuads(ribbon, BeamQuads(ribbon, eye))...)
			continue
		}
		if r.Ring == nil {
			continue
		}
		// Кольцо висит в начале системы эффекта; его Offset добавляет
		// RingRibbon сам, в осях маркера. Верх камеры — вертикаль файла.
		ribbon := RingRibbon(*r.Ring, Vec3{}, Vec3{X: 1}, Vec3{Y: 1}, Vec3{Z: 1}, Vec3{Y: 1}, u, r.Ring.Spread(r.Index))
		out = append(out, RibbonQuads(ribbon, BeamQuads(ribbon, eye))...)
	}
	s.quadBuf = out
	return out
}

// Lifetime — сколько эффекту играть в кадре, пока его не выключат: самый
// долгий распылитель или лента. Зацикленный эффект иначе висел бы вечно,
// а выстрелу хватает одного прохода.
func (s *Simulation) Lifetime() float64 {
	if s == nil {
		return 0
	}
	best := 0.0
	for i := range s.Emitters {
		if d := s.Emitters[i].Duration; d > best {
			best = d
		}
	}
	for i := range s.Ribbons {
		if d := s.Ribbons[i].lifetime(); d > best {
			best = d
		}
	}
	for _, v := range s.Anchored {
		if v.Lifetime > best {
			best = v.Lifetime
		}
	}
	if best <= 0 {
		best = 1
	}
	if best > 3 {
		best = 3
	}
	return best
}

// phase — возраст узла в 0..1 для его кривых.
func (r *RibbonNode) phase() float64 {
	life := r.lifetime()
	if life <= 0 {
		return 0
	}
	return clamp(r.Age/life, 0, 1)
}

// spriteFor выбирает кривые одного вида в нормализованном возрасте u.
func spriteFor(v Visual, u float64, pos Vec3) Sprite {
	sp := Sprite{
		Pos:      pos,
		Radius:   SampleCurve(v.Radius, u),
		Texture:  v.Texture,
		Additive: v.Blending == 2,
		SelfLit:  v.SelfLit,
		A:        1,
	}
	// Particle_Scale — собственный размер листа за жизнь поверх кривой
	// Radius: клуб, который быстро растёт и потом истончается, написан так,
	// а не через радиус.
	if len(v.Scale) > 0 {
		sp.Radius *= SampleCurve(v.Scale, u)
	}
	if st := sampleColour(v.Colour, u); st != nil {
		sp.R, sp.G, sp.B, sp.A = st.R, st.G, st.B, st.A
	}
	if !sp.Additive && sp.A <= 0 {
		sp.A = 1
	}
	return sp
}

// SampleCurve берёт значение кривой в нормализованном возрасте. Ось времени
// у файлов какая угодно — секунды двухсполовинойсекундного эффекта или
// развёртка 0..1, — поэтому она растягивается на собственный диапазон
// кривой.
func SampleCurve(keys []Key, u float64) float64 {
	if len(keys) == 0 {
		return 0
	}
	if len(keys) == 1 {
		return keys[0].V
	}
	span := keys[len(keys)-1].T - keys[0].T
	if span <= 0 {
		return keys[0].V
	}
	u = clamp(u, 0, 1)
	t := keys[0].T + u*span
	for i := 1; i < len(keys); i++ {
		if t <= keys[i].T {
			a, b := keys[i-1], keys[i]
			if b.T <= a.T {
				return b.V
			}
			f := (t - a.T) / (b.T - a.T)
			return a.V + f*(b.V-a.V)
		}
	}
	return keys[len(keys)-1].V
}

// sampleColour берёт цвет из градиента тем же способом.
func sampleColour(stops []ColourStop, u float64) *ColourStop {
	if len(stops) == 0 {
		return nil
	}
	if len(stops) == 1 {
		return &stops[0]
	}
	span := stops[len(stops)-1].T - stops[0].T
	if span <= 0 {
		return &stops[0]
	}
	u = clamp(u, 0, 1)
	t := stops[0].T + u*span
	for i := 1; i < len(stops); i++ {
		if t <= stops[i].T {
			a, b := stops[i-1], stops[i]
			if b.T <= a.T {
				return &stops[i]
			}
			f := (t - a.T) / (b.T - a.T)
			return &ColourStop{
				T: t,
				R: a.R + f*(b.R-a.R),
				G: a.G + f*(b.G-a.G),
				B: a.B + f*(b.B-a.B),
				A: a.A + f*(b.A-a.A),
			}
		}
	}
	return &stops[len(stops)-1]
}

// Frame перекладывает текущий кадр эффекта в частицы, которые рисует gfx3d.
// Спрайт уже имеет свой размер и цвет, поэтому частица получает их готовыми
// (Life и Age подобраны так, чтобы система ничего не досчитывала), а
// положение переводится из системы файла в мир с Z вверх.
//
// Срез годен до следующего вызова — ровно то, что нужно отрисовке: она
// читает кадр и рисует его, прежде чем спросить что-то ещё.
func (s *Simulation) Frame() []Particle {
	if s == nil {
		return nil
	}
	sprites := s.Sprites()
	out := s.frameBuf[:0]
	if cap(out) < len(sprites) {
		out = make([]Particle, 0, len(sprites))
	}
	for _, sp := range sprites {
		size := sp.Radius * 2
		if size <= 0 {
			continue
		}
		out = append(out, Particle{
			Pos:      simToWorld(sp.Pos),
			Life:     1,
			Size0:    size,
			Size1:    size,
			Color:    [4]float32{float32(sp.R), float32(sp.G), float32(sp.B), float32(sp.A)},
			Additive: sp.Additive,
			Texture:  sp.Texture,
		})
	}
	s.frameBuf = out
	return out
}

// simToWorld переводит точку из системы файла (Y вверх) в мир с Z вверх:
// X остаётся собой, вертикаль уходит в Z, а Z файла — в -Y мира, чтобы
// тройка осей осталась правой.
func simToWorld(v Vec3) [3]float64 {
	return [3]float64{v.X, -v.Z, v.Y}
}

// clamp держит значение в отрезке.
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// vecLen — длина вектора.
func vecLen(v Vec3) float64 {
	return math.Sqrt(v.X*v.X + v.Y*v.Y + v.Z*v.Z)
}

// vecNorm — единичный вектор; нулевой остаётся нулевым.
func vecNorm(v Vec3) Vec3 {
	l := vecLen(v)
	if l < 1e-9 {
		return Vec3{}
	}
	return Vec3{X: v.X / l, Y: v.Y / l, Z: v.Z / l}
}

// vecCross — векторное произведение.
func vecCross(a, b Vec3) Vec3 {
	return Vec3{
		X: a.Y*b.Z - a.Z*b.Y,
		Y: a.Z*b.X - a.X*b.Z,
		Z: a.X*b.Y - a.Y*b.X,
	}
}

// vecScale — умножение на скаляр.
func vecScale(v Vec3, k float64) Vec3 {
	return Vec3{X: v.X * k, Y: v.Y * k, Z: v.Z * k}
}

// vecAdd — сумма векторов.
func vecAdd(a, b Vec3) Vec3 {
	return Vec3{X: a.X + b.X, Y: a.Y + b.Y, Z: a.Z + b.Z}
}
