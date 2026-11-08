package fx

import "math"

// Порт internal/fx/beam.go из dow-mod-studio: узлы STYLE_BEAM (ленты) и
// STYLE_RING (кольца). Без них эффект читается наполовину: лента — не
// частица, её нельзя нарисовать спрайтом, а кольцо, прочитанное как спрайт,
// висит в начале координат, потому что его место задаёт Offset в системе
// маркера, а не распылитель.
//
// Лента тянется между двумя точками, и в файле эффекта этих точек нет:
// второй конец называет событие .whe (fx_other_marker, fx_length). Если
// события нет, Length читается в единицах модели — так игра понимает файлы,
// которые тянут луч от ствола на заданную длину.
//
// Кольцо — плоский четырёхугольник радиуса Radius: он лежит в плоскости
// маркера (Fixed) и поворачивается кривыми Spin; SpinEpsilon разводит
// одинаковые панели по разным углам, а SpinRandom разбрасывает их, а не
// раскладывает по порядку. Именно так собирается кристалл некронов: его
// панели — отдельные кольца с фиксированным SpinZ и разбросом EpsilonZ.

// fxBeamCurveMax ограничивает кривую ленты: движку больше не нужно, а файл,
// который пишет больше, уже нечитаем.
const fxBeamCurveMax = 64

// MaxBeamSegments — предел Segments ленты. Файлы просят пять или восемь;
// буфер вершин рассчитан на этот предел.
const MaxBeamSegments = 32

// fxRingDefaultRadius — радиус кольца без кривой Radius.
const fxRingDefaultRadius = 0.1

// RingSpreadBuckets — сколько разных лиц раздаёт упорядоченный разброс,
// прежде чем повториться.
const RingSpreadBuckets = 8

// Beam — узел STYLE_BEAM: форма ленты, её анимация и карта. Концы ленты
// приходят из события, а не из файла.
type Beam struct {
	Texture  string
	Blending int
	Lifetime float64

	Length   []Key // длина: доля зазора между маркерами или единицы модели
	Width    []Key // половина толщины в единицах модели
	Segments []Key
	Noise    []Key // боковое дрожание ленты
	UOffset  []Key // прокрутка карты вдоль ленты
	URepeat  []Key
	Colour   []ColourStop
	Arc      []Vec3Key // изгиб ленты
	Offset   []Vec3Key
	// SelfLit — SelfIlluminated узла: без него лента гаснет вместе со сценой.
	SelfLit bool
	// DepthTest — UseDepthTest; в файлах игры у лент он выключен: свечение
	// должно читаться сквозь стрелка.
	DepthTest bool
}

// beamFromNode читает узел STYLE_BEAM; false — узел другого стиля.
func beamFromNode(n *Node) (Beam, bool) {
	if n == nil || n.Style != "STYLE_BEAM" {
		return Beam{}, false
	}
	b := Beam{
		Texture:   n.String("Texture"),
		Blending:  int(n.Float("Blending", 2)),
		Lifetime:  n.Float("Duration", 1),
		DepthTest: n.Bool("UseDepthTest", false),
		SelfLit:   n.Bool("SelfIlluminated", false),
	}
	b.Length = beamCurve(n, "Length")
	b.Width = beamCurve(n, "Width")
	b.Segments = beamCurve(n, "Segments")
	b.Noise = beamCurve(n, "Noise")
	b.UOffset = beamCurve(n, "Texture_U_Offset")
	b.URepeat = beamCurve(n, "Texture_U_Repeat")
	if p, ok := n.Prop("Colour"); ok {
		b.Colour = p.Colour
	}
	if p, ok := n.Prop("Arc"); ok {
		b.Arc = p.Vec3
	}
	if p, ok := n.Prop("Offset"); ok {
		b.Offset = p.Vec3
	}
	if b.Lifetime <= 0 {
		b.Lifetime = 1
	}
	return b, true
}

// beamCurve читает анимационное свойство ленты, обрезая слишком длинные
// кривые; пустая кривая означает «свойства нет».
func beamCurve(n *Node, name string) []Key {
	p, ok := n.Prop(name)
	if !ok || len(p.Curve) == 0 {
		return nil
	}
	if len(p.Curve) > fxBeamCurveMax {
		return p.Curve[:fxBeamCurveMax]
	}
	return p.Curve
}

// spinCurve читает Spin или SpinEpsilon: одно и то же свойство пишется и
// кривой, и голым числом (у кристалла SpinEpsilonZ — VARTYPE_FLOAT), а без
// второго панели кристалла встают квадратно.
func spinCurve(n *Node, name string) []Key {
	if c := beamCurve(n, name); c != nil {
		return c
	}
	p, ok := n.Prop(name)
	if !ok {
		return nil
	}
	if p.Type == "VARTYPE_FLOAT" || p.Type == "VARTYPE_INT" {
		return []Key{{T: 0, V: p.Num}, {T: 1, V: p.Num}}
	}
	return nil
}

// BeamRibbon — геометрия одной ленты в этом кадре: два конца, толщина и
// цвет, к которому она дожила.
type BeamRibbon struct {
	// From и To — концы ленты; To отстоит от From на Length.
	From, To Vec3
	Width    float64
	R, G, B  float64
	Alpha    float64
	Texture  string
	// U0 и U1 — где начинается и кончается карта вдоль ленты: прокрутка и
	// повторение задают её кривые Texture_U_*.
	U0, U1   float64
	Segments int
	Noise    float64
	Arc      Vec3
	Additive bool
	SelfLit  bool
	// Across — куда лента ширится. Лента оставляет его пустым и поворачивает
	// сечение к глазу; кольцо задаёт Across и Fixed, потому что кольцо,
	// лежащее на поверхности, обязано остаться в своей плоскости.
	Across    Vec3
	Fixed     bool
	Billboard bool
	// FlipV кладёт верхний ряд карты на дальний конец ленты (для кольца —
	// на верх маркера): у кристалла этим верхние и нижние грани сходятся в
	// пояс, а не расходятся врозь.
	FlipV     bool
	DepthTest bool
}

// NewBeamRibbon сэмплирует ленту в нормализованном возрасте u для заданных
// концов. spans сообщает, что событие назвало второй маркер и Length —
// доля зазора; иначе Length — длина в единицах модели.
func NewBeamRibbon(b Beam, from, to Vec3, u float64, spans bool) BeamRibbon {
	dist := to.Sub(from).Length()
	spans = spans && len(b.Length) > 0
	fraction := SampleCurve(b.Length, u)
	if !spans {
		if fraction < 0 {
			fraction = 0
		}
	} else if fraction <= 0 || fraction > 1 {
		fraction = 1
	}
	segments := int(SampleCurve(b.Segments, u) + 0.5)
	if segments < 1 {
		segments = 1
	}
	if segments > MaxBeamSegments {
		segments = MaxBeamSegments
	}
	r := BeamRibbon{
		From:      from,
		To:        to,
		Width:     SampleCurve(b.Width, u),
		Segments:  segments,
		Texture:   b.Texture,
		Additive:  b.Blending == 2,
		DepthTest: b.DepthTest,
		Noise:     SampleCurve(b.Noise, u),
		Alpha:     1,
		SelfLit:   b.SelfLit,
	}
	// Прокрутка и повторение — две половины одной координаты: где стоит
	// карта и сколько раз она проходит вдоль ленты.
	scroll := SampleCurve(b.UOffset, u)
	repeat := SampleCurve(b.URepeat, u)
	r.U0 = scroll
	r.U1 = scroll + repeat
	// Дальний конец — Length вдоль зазора: доля зазора, когда лента тянется
	// между маркерами, и столько единиц модели, когда она идёт сама.
	if dist > 1e-9 {
		span := fraction
		if !spans {
			span = fraction / dist
		}
		r.To = from.Add(to.Sub(from).MulScalar(span))
	}
	if st := sampleColour(b.Colour, u); st != nil {
		r.R, r.G, r.B, r.Alpha = st.R, st.G, st.B, st.A
	}
	// Arc и Offset записаны в системе эффекта (Y вверх), как вся остальная
	// локальная геометрия: маркер переносит их в модель, поэтому здесь они
	// остаются как есть.
	if a, ok := vec3Sample(b.Arc, u); ok {
		r.Arc = a
	}
	if o, ok := vec3Sample(b.Offset, u); ok {
		r.From = r.From.Add(o)
	}
	return r
}

// BeamVert — один угол одного четырёхугольника ленты в системе модели с
// координатой карты.
type BeamVert struct {
	Pos Vec3
	U   float64
	V   float64
}

// BeamQuads разбивает ленту на четырёхугольники. Движок строит её полосой:
// идёт по оси Segments шагами и ставит в каждом сечение шириной Width,
// повёрнутое к глазу, — так луч читается круглым валом, а не плоской
// карточкой с ребра. Noise сдвигает шаг вбок, Arc выгибает всю полосу.
func BeamQuads(r BeamRibbon, eye Vec3) []BeamVert {
	axis := r.To.Sub(r.From)
	length := axis.Length()
	if length < 1e-9 || r.Width <= 0 {
		return nil
	}
	dir := axis.MulScalar(1 / length)
	var across Vec3
	if r.Fixed {
		across = r.Across
	} else {
		across = eye.Sub(r.From).Cross(dir)
	}
	if across.Length() < 1e-6 {
		// Смотрим вдоль ленты: годится любая перпендикулярная ось, ширины
		// с этого ракурса всё равно нет.
		across = dir.Cross(Vec3{Z: 1})
		if across.Length() < 1e-6 {
			across = dir.Cross(Vec3{X: 1})
		}
		if across.Length() < 1e-6 {
			return nil
		}
	}
	across = across.Norm()
	// Дуга — смещение поперёк пролёта; составляющая вдоль пролёта не
	// выгибает ленту, а только удлиняет её.
	bow := r.Arc.Sub(dir.MulScalar(r.Arc.Dot(dir)))
	if bow.Length() < 1e-6 && r.Arc.Length() > 1e-6 {
		bow = across
	}
	steps := r.Segments
	if steps < 1 {
		steps = 1
	}
	verts := make([]BeamVert, 0, (steps+1)*2)
	for i := 0; i <= steps; i++ {
		f := float64(i) / float64(steps)
		p := r.From.Add(axis.MulScalar(f))
		bowAt := bow.MulScalar(math.Sin(f * math.Pi))
		var wobble Vec3
		if r.Noise != 0 {
			// Дрожание — функция места вдоль ленты, а не часов: иначе
			// лента ползла бы между кадрами.
			wobble = across.MulScalar(r.Noise * math.Sin(f*math.Pi*3+r.U0))
		}
		centre := p.Add(bowAt).Add(wobble)
		half := r.Width
		lo, hi := 0.0, 1.0
		if r.FlipV {
			lo, hi = 1, 0
		}
		verts = append(verts,
			BeamVert{Pos: centre.Sub(across.MulScalar(half)), U: r.U0 + (r.U1-r.U0)*f, V: lo},
			BeamVert{Pos: centre.Add(across.MulScalar(half)), U: r.U0 + (r.U1-r.U0)*f, V: hi},
		)
	}
	return verts
}

// BeamTriangles превращает пары вершин ленты в индексы: по четырёхугольнику
// за шаг.
func BeamTriangles(verts []BeamVert) []uint16 {
	n := len(verts) / 2
	if n < 2 {
		return nil
	}
	out := make([]uint16, 0, (n-1)*6)
	for i := 0; i+1 < n; i++ {
		a := uint16(i * 2)
		out = append(out, a, a+1, a+3, a, a+3, a+2)
	}
	return out
}

// Quad — готовый четырёхугольник кадра: четыре угла и координаты карты.
// Ленты и кольца отдают их рендереру, спрайты частиц — нет.
type Quad struct {
	P        [4]Vec3
	UV       [4][2]float64
	R, G, B  float64
	A        float64
	Texture  string
	Additive bool
}

// RibbonQuads превращает вершины ленты в четырёхугольники.
func RibbonQuads(r BeamRibbon, verts []BeamVert) []Quad {
	n := len(verts) / 2
	if n < 2 {
		return nil
	}
	out := make([]Quad, 0, n-1)
	for i := 0; i+1 < n; i++ {
		lo0, hi0 := verts[i*2], verts[i*2+1]
		lo1, hi1 := verts[(i+1)*2], verts[(i+1)*2+1]
		out = append(out, Quad{
			P:        [4]Vec3{lo0.Pos, hi0.Pos, hi1.Pos, lo1.Pos},
			UV:       [4][2]float64{{lo0.U, lo0.V}, {hi0.U, hi0.V}, {hi1.U, hi1.V}, {lo1.U, lo1.V}},
			R:        r.R,
			G:        r.G,
			B:        r.B,
			A:        r.Alpha,
			Texture:  r.Texture,
			Additive: r.Additive,
		})
	}
	return out
}

// Ring — узел STYLE_RING: плоский четырёхугольник радиуса Radius в
// плоскости маркера, повёрнутый кривыми Spin.
type Ring struct {
	Texture   string
	Blending  int
	Lifetime  float64
	billboard bool
	// SelfLit — SelfIlluminated узла: 86 из 218 колец игры его ставят, и под
	// одним светом они ярче остальных.
	SelfLit bool
	Radius  []Key
	Colour  []ColourStop
	Offset  []Vec3Key
	spinX   []Key
	SpinY   []Key
	SpinZ   []Key
	OffsetX []Key
	OffsetY []Key
	OffsetZ []Key
	Loop    bool
	// EpsX/EpsY/EpsZ — SpinEpsilon: добавка к Spin своей оси, чтобы
	// несколько панелей одного узла смотрели в разные стороны; SpinRandom
	// просит разбросать её случайно, а не по порядку.
	EpsX       []Key
	EpsY       []Key
	EpsZ       []Key
	SpinRandom bool
}

// RingFromNode читает узел STYLE_RING; false — узел другого стиля.
func RingFromNode(n *Node) (Ring, bool) {
	if n == nil || n.Style != "STYLE_RING" {
		return Ring{}, false
	}
	r := Ring{
		Texture:   n.String("Texture"),
		Blending:  int(n.Float("Blending", 2)),
		Lifetime:  n.Float("Duration", 1),
		billboard: n.Bool("Billboard", false),
		Loop:      n.Bool("Loop", true),
		SelfLit:   n.Bool("SelfIlluminated", false),
	}
	r.Radius = beamCurve(n, "Radius")
	r.spinX = beamCurve(n, "SpinX")
	r.SpinY = beamCurve(n, "SpinY")
	r.SpinZ = beamCurve(n, "SpinZ")
	r.OffsetX = beamCurve(n, "OffsetX")
	r.OffsetY = beamCurve(n, "OffsetY")
	r.OffsetZ = beamCurve(n, "OffsetZ")
	r.EpsX = spinCurve(n, "SpinEpsilonX")
	r.EpsY = spinCurve(n, "SpinEpsilonY")
	r.EpsZ = spinCurve(n, "SpinEpsilonZ")
	r.SpinRandom = n.Bool("SpinRandom", false)
	if p, ok := n.Prop("Colour"); ok {
		r.Colour = p.Colour
	}
	if p, ok := n.Prop("Offset"); ok {
		r.Offset = p.Vec3
	}
	if r.Lifetime <= 0 {
		r.Lifetime = 1
	}
	if r.Radius == nil {
		r.Radius = []Key{{T: 0, V: fxRingDefaultRadius}, {T: 1, V: fxRingDefaultRadius}}
	}
	return r, true
}

// Spread — доля разброса панели i в 0..1. Она выводится из номера, а не
// бросается заново: панель обязана держать своё лицо от кадра к кадру, иначе
// кристалл мерцал бы.
func (r Ring) Spread(i int) float64 {
	if len(r.EpsX) == 0 && len(r.EpsY) == 0 && len(r.EpsZ) == 0 {
		return 0
	}
	if r.SpinRandom {
		return fxRingRandomShare(i)
	}
	return float64(i%RingSpreadBuckets) / float64(RingSpreadBuckets-1)
}

// fxRingRandomShare разбрасывает номер панели по 0..1 хэшем, а не
// генератором: ответ зависит только от номера, поэтому панель получает одно
// и то же лицо при каждом проигрывании эффекта.
func fxRingRandomShare(i int) float64 {
	h := uint64(i+1) * 0x9E3779B97F4A7C15
	h ^= h >> 30
	h *= 0xBF58476D1CE4E5B9
	h ^= h >> 27
	h *= 0x94D049BB133111EB
	h ^= h >> 31
	return float64(h>>40) / float64(1<<24)
}

// RingRibbon сэмплирует кольцо в возрасте u в четырёхугольник, который оно
// рисует. centre — где висит кольцо, axX/axY/axZ — первые три оси системы
// маркера (в ней записан Offset), camUp — верх камеры для билборда.
//
// Кольцо, остающееся в своей плоскости, кладёт карту так, как читается
// маркер: вертикаль карты вдоль его X, ширина — вдоль Z. Панели кристалла —
// тот случай, который это и задаёт: их маркеры стоят вокруг камня, и любая
// другая раскладка превращает грань в плавник.
func RingRibbon(r Ring, centre, axX, axY, axZ, camUp Vec3, u, spread float64) BeamRibbon {
	half := SampleCurve(r.Radius, u)
	out := BeamRibbon{
		Texture:   r.Texture,
		Width:     half,
		Segments:  1,
		Additive:  r.Blending == 2,
		Alpha:     1,
		U0:        0,
		U1:        1,
		SelfLit:   r.SelfLit,
		Billboard: r.billboard,
	}
	if st := sampleColour(r.Colour, u); st != nil {
		out.R, out.G, out.B, out.Alpha = st.R, st.G, st.B, st.A
	}
	// Offset записан в системе маркера — вот почему кольцо знает, что висит
	// на конце ствола, а не у его основания.
	if o, ok := vec3Sample(r.Offset, u); ok {
		centre = centre.
			Add(axX.MulScalar(o.X)).
			Add(axY.MulScalar(o.Y)).
			Add(axZ.MulScalar(o.Z))
	}
	// Кольца распылителя несут OffsetX/Y/Z вместо одного общего Offset:
	// они двигают центр плоскости, а ориентацию берут у маркера.
	if len(r.OffsetX) > 0 || len(r.OffsetY) > 0 || len(r.OffsetZ) > 0 {
		local := Vec3{
			X: SampleCurve(r.OffsetX, u),
			Y: SampleCurve(r.OffsetY, u),
			Z: SampleCurve(r.OffsetZ, u),
		}
		centre = centre.Add(axX.MulScalar(local.X)).Add(axY.MulScalar(local.Y)).Add(axZ.MulScalar(local.Z))
		return ringSpriteRibbon(out, centre, axX, axY, axZ, r, u, spread)
	}
	axR, axU := axX, axZ
	if r.billboard {
		axR, axU = CamRight(camUp, centre), camUp
	}
	// Spin — поворот за время жизни в той же системе; SpinEpsilon
	// добавляется сверху своей долей разброса.
	axR, axU = rotateInPlane(axR, axU, (SampleCurve(r.spinX, u)+SampleCurve(r.EpsX, u)*spread)*math.Pi*2)
	axR, axU = rotateInPlane(axR, axU, (SampleCurve(r.SpinY, u)+SampleCurve(r.EpsY, u)*spread)*math.Pi*2)
	axR, axU = rotateInPlane(axR, axU, (SampleCurve(r.SpinZ, u)+SampleCurve(r.EpsZ, u)*spread)*math.Pi*2)
	out.From = centre.Sub(axU.MulScalar(half))
	out.To = centre.Add(axU.MulScalar(half))
	// Across задан, а не оставлен глазу: правило ленты развернуло бы кольцо
	// к камере, чего кольцо на поверхности делать не должно.
	out.Across = axR
	out.Fixed = true
	// Верхний ряд карты — к верху маркера: у кристалла нижние грани
	// нарисованы узкими сверху и расширяются книзу, и перевёрнутые они
	// сходятся не в пояс, а в бочку.
	out.FlipV = !r.billboard
	return out
}

// ringSpriteRibbon — кольцо распылителя как спрайт в фиксированной
// плоскости: OffsetX/Y/Z задают центр, плоскость остаётся локальной XY с
// нормалью axZ, а вертикаль четырёхугольника идёт по axY.
func ringSpriteRibbon(out BeamRibbon, centre, axX, axY, axZ Vec3, r Ring, u, spread float64) BeamRibbon {
	half := SampleCurve(r.Radius, u)
	right := axX
	up := axY
	if r.billboard {
		right = CamRight(Vec3{Z: 1}, centre)
		up = Vec3{Z: 1}
	}
	angle := (SampleCurve(r.spinX, u) + SampleCurve(r.SpinY, u) + SampleCurve(r.SpinZ, u) +
		SampleCurve(r.EpsX, u)*spread + SampleCurve(r.EpsY, u)*spread + SampleCurve(r.EpsZ, u)*spread) * math.Pi * 2
	right, up = rotateInPlane(right, up, angle)
	out.From = centre.Sub(up.MulScalar(half))
	out.To = centre.Add(up.MulScalar(half))
	out.Across = right
	out.Fixed = true
	out.FlipV = !r.billboard
	return out
}

// CamRight — право камеры из её верха и направления на наблюдателя. Порядок
// произведения важен: камера смотрит вдоль towardsEye, поэтому право —
// camUp × towardsEye; обратный порядок дал бы лево и зеркалил кольцо.
func CamRight(camUp, towardsEye Vec3) Vec3 {
	r := camUp.Cross(towardsEye)
	if n := r.Length(); n > 1e-6 {
		return r.Norm()
	}
	if n := camUp.Length(); n > 1e-6 {
		u := camUp.Norm()
		helper := Vec3{X: 1}
		if s := math.Abs(u.X); s > math.Abs(u.Y) {
			helper = Vec3{Y: 1}
			if s > math.Abs(u.Z) {
				helper = Vec3{Z: 1}
			}
		}
		if r := u.Cross(helper); r.Length() > 1e-6 {
			return r.Norm()
		}
	}
	return Vec3{X: 1}
}

// CrossOf — векторное произведение двух векторов.
func CrossOf(a, b Vec3) Vec3 { return a.Cross(b) }

// rotateInPlane поворачивает пару осей плоскости вокруг их нормали.
func rotateInPlane(right, up Vec3, ang float64) (Vec3, Vec3) {
	if ang == 0 {
		return right, up
	}
	c, s := math.Cos(ang), math.Sin(ang)
	return right.MulScalar(c).Add(up.MulScalar(s)), up.MulScalar(c).Sub(right.MulScalar(s))
}

// Add — сумма векторов.
func (v Vec3) Add(o Vec3) Vec3 { return Vec3{X: v.X + o.X, Y: v.Y + o.Y, Z: v.Z + o.Z} }

// Sub — разность векторов.
func (v Vec3) Sub(o Vec3) Vec3 { return Vec3{X: v.X - o.X, Y: v.Y - o.Y, Z: v.Z - o.Z} }

// MulScalar — умножение на скаляр.
func (v Vec3) MulScalar(k float64) Vec3 { return Vec3{X: v.X * k, Y: v.Y * k, Z: v.Z * k} }

// Dot — скалярное произведение.
func (v Vec3) Dot(o Vec3) float64 { return v.X*o.X + v.Y*o.Y + v.Z*o.Z }

// Cross — векторное произведение.
func (v Vec3) Cross(o Vec3) Vec3 {
	return Vec3{
		X: v.Y*o.Z - v.Z*o.Y,
		Y: v.Z*o.X - v.X*o.Z,
		Z: v.X*o.Y - v.Y*o.X,
	}
}

// Length — длина вектора.
func (v Vec3) Length() float64 { return math.Sqrt(v.Dot(v)) }

// Norm — единичный вектор; нулевой остаётся нулевым.
func (v Vec3) Norm() Vec3 {
	l := v.Length()
	if l < 1e-9 {
		return Vec3{}
	}
	return Vec3{X: v.X / l, Y: v.Y / l, Z: v.Z / l}
}
