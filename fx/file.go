package fx

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Файлы эффектов (art/fx/*.lua) — вывод редактора FX Relic: плоский список
// fxtypes, где каждый узел несёт путь эффекта ("root", "root>spray",
// "root>spray>ring"), стиль (STYLE_SPRAY, STYLE_RING, …) и таблицу свойств.
// Путь и есть дерево: родитель узла — путь без последнего сегмента.
//
// Здесь только чтение описания эффекта. Что из него получается на экране
// (частицы, ленты, кольца) — дело симуляции, а когда эффект играет —
// привязки к событиям модели; это следующие шаги портирования.
//
// Порт из dow-mod-studio (internal/fx/lua.go): разбор тот же, но без
// зависимости на fauxgl — вектор здесь свой, а комментарии и имена — по
// соглашениям этого проекта.

// Vec3 — вектор в мире эффекта (Z вверх).
type Vec3 struct {
	X, Y, Z float64
}

// Prop — одно свойство узла: скаляр, кривая времени или цветовой градиент,
// в зависимости от Type.
type Prop struct {
	Type   string // VARTYPE_… как записано в файле
	Str    string // VARTYPE_STRING
	Num    float64
	Curve  []Key        // VARTYPE_ARRAY_TIMEFLOAT: пары (время, значение)
	Colour []ColourStop // VARTYPE_ARRAY_TIMECOLOUR
	Vec3   []Vec3Key    // VARTYPE_ARRAY_TIMEVECTOR3: (время, x, y, z)
	Tuple  Vec3         // VARTYPE_VECTOR3
}

// Key — точка кривой время/значение.
type Key struct {
	T, V float64
}

// Vec3Key — точка кривой время/вектор: так записаны Arc и Offset ленты.
type Vec3Key struct {
	T       float64
	X, Y, Z float64
}

// ColourStop — точка цветового градиента, каналы 0..1.
type ColourStop struct {
	T       float64
	R, G, B float64
	A       float64
}

// Node — один узел эффекта.
type Node struct {
	Effect string // "root", "root>spray", …
	Style  string // STYLE_… без кавычек, может быть пустым
	Props  map[string]Prop
}

// File — разобранный файл эффекта.
type File struct {
	Nodes []Node
	// Single — файл из одного узла без собственного пути: около четверти
	// эффектов игры записаны как `fx = { properties = … }` без
	// effect = "root>…". Дерева в таком файле нет — единственный узел и
	// есть весь эффект, поэтому симуляции надо знать об этом отдельно.
	Single bool
}

// Prop ищет свойство узла и сообщает, есть ли оно вообще.
func (n Node) Prop(name string) (Prop, bool) {
	p, ok := n.Props[name]
	return p, ok
}

// Float — скалярное свойство или запасное значение.
func (n Node) Float(name string, fallback float64) float64 {
	if p, ok := n.Props[name]; ok {
		return p.Num
	}
	return fallback
}

// Bool — логическое свойство или запасное значение.
func (n Node) Bool(name string, fallback bool) bool {
	if p, ok := n.Props[name]; ok {
		return p.Num != 0
	}
	return fallback
}

// String — строковое свойство или пустая строка.
func (n Node) String(name string) string {
	if p, ok := n.Props[name]; ok {
		return p.Str
	}
	return ""
}

// Root — узел, у пути которого нет родителя.
func (f *File) Root() *Node {
	if f == nil {
		return nil
	}
	for i := range f.Nodes {
		if !strings.Contains(f.Nodes[i].Effect, ">") {
			return &f.Nodes[i]
		}
	}
	if len(f.Nodes) > 0 {
		return &f.Nodes[0]
	}
	return nil
}

// Children — прямые потомки узла с таким путём.
func (f *File) Children(effect string) []*Node {
	if f == nil {
		return nil
	}
	var out []*Node
	for i := range f.Nodes {
		p := f.Nodes[i].Effect
		if !strings.HasPrefix(p, effect+">") {
			continue
		}
		if !strings.Contains(strings.TrimPrefix(p, effect+">"), ">") {
			out = append(out, &f.Nodes[i])
		}
	}
	return out
}

var (
	fxEffectRe = regexp.MustCompile(`effect\s*=\s*"([^"]*)"`)
	fxStyleRe  = regexp.MustCompile(`style\s*=\s*"([^"]*)"`)
	fxNameRe   = regexp.MustCompile(`name\s*=\s*"([^"]+)"`)
	fxTypeRe   = regexp.MustCompile(`type\s*=\s*"(VARTYPE_[A-Z0-9_]+)"`)
	fxValueRe  = regexp.MustCompile(`value\s*=`)
	fxEntryRe  = regexp.MustCompile(`entry_\d+\s*=\s*\{`)
	// fxPropertyRe открывает блок одного свойства: именно в нём лежат его
	// тип и значение, в каком бы порядке файл их ни записал.
	fxPropertyRe = regexp.MustCompile(`property_\d+\s*=\s*\{`)
	fxNumRe      = regexp.MustCompile(`[-+]?\d+(?:\.\d+)?(?:[eE][-+]?\d+)?`)
	// fxBareNodeRe узнаёт форму из одного узла: `fx =` с таблицей свойств и
	// без effect =, которым узел назван. fxBareRootName — путь, под которым
	// такой узел хранится, чтобы читаться как корень любого другого файла.
	fxBareNodeRe   = regexp.MustCompile(`(?m)^\s*fx\s*=\s*$`)
	fxBareRootName = "root"
)

// ParseFile читает файл эффекта. Это сканер по тексту, а не интерпретатор
// lua: файлы пишет машина, у всех узлов одна форма, а свойство, значение
// которого не прочиталось, пропускается — показать почти весь эффект лучше,
// чем не показать ничего.
func ParseFile(data []byte) (*File, error) {
	s := string(data)
	locs := fxEffectRe.FindAllStringSubmatchIndex(s, -1)
	if len(locs) == 0 {
		// Другая форма записи: один узел без собственного effect =. В нём
		// есть всё, что нужно ленте или кольцу — дерево, длина, текстура.
		// Узел представляет весь файл и читается как корень.
		if fxBareNodeRe.MatchString(s) {
			node := Node{Effect: fxBareRootName, Props: map[string]Prop{}}
			if m := fxStyleRe.FindStringSubmatchIndex(s); m != nil {
				node.Style = s[m[2]:m[3]]
			}
			parseProps(s, &node)
			return &File{Nodes: []Node{node}, Single: true}, nil
		}
		return nil, fmt.Errorf("fx: в файле эффекта нет узлов fxtypes")
	}
	f := &File{}
	for i, loc := range locs {
		end := len(s)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		span := s[loc[2]:end]
		node := Node{
			Effect: s[loc[2]:loc[3]],
			Props:  map[string]Prop{},
		}
		if m := fxStyleRe.FindStringSubmatchIndex(span); m != nil {
			node.Style = span[m[2]:m[3]]
		}
		parseProps(span, &node)
		f.Nodes = append(f.Nodes, node)
	}
	return f, nil
}

// parseProps заполняет таблицу свойств одного узла по его куску текста.
//
// Две формы записи кладут три поля свойства в разном порядке: в одной
// сначала name, затем type, затем value, в другой — value, type, name.
// Поиск вперёд от имени работает только для первой: во второй он
// перешагивает конец свойства и берёт тип и значение соседнего. Чтение
// блока каждого свойства обрабатывает обе формы, потому что сам блок
// записан одинаково.
func parseProps(span string, node *Node) {
	names := fxNameRe.FindAllStringSubmatchIndex(span, -1)
	for i, nm := range names {
		name := span[nm[2]:nm[3]]
		searchFrom := nm[1]
		spanEnd := len(span)
		if i+1 < len(names) {
			spanEnd = names[i+1][0]
		}
		// Блок, которому принадлежит имя: в нём лежат его тип и значение
		// в любом порядке.
		blocks := fxPropertyRe.FindAllStringIndex(span[:searchFrom], -1)
		if n := len(blocks); n > 0 {
			searchFrom = blocks[n-1][1]
		}
		region := span[searchFrom:spanEnd]
		tm := fxTypeRe.FindStringSubmatchIndex(region)
		if tm == nil {
			continue
		}
		typ := region[tm[2]:tm[3]]
		// Значение ищется само по себе, а не после типа: в форме
		// «значение первым» оно стоит до него. В блоке один тип и одно
		// значение, поэтому первое из каждого — его собственные.
		vm := fxValueRe.FindStringIndex(region)
		if vm == nil {
			continue
		}
		value := region[vm[1]:]
		node.Props[name] = parseValue(typ, value)
	}
}

// parseValue читает одно значение свойства из текста сразу после "value =".
func parseValue(typ, value string) Prop {
	// Сканер останавливается сразу после "value =", поэтому пробел перед
	// литералом ещё в строке: файлы пишут `value = "map.tga"`, и ведущий
	// пробел заставил бы пропустить открывающую кавычку.
	value = strings.TrimLeft(value, " \t\r\n")
	p := Prop{Type: typ}
	switch typ {
	case "VARTYPE_STRING":
		if len(value) > 0 && value[0] == '"' {
			if j := strings.IndexByte(value[1:], '"'); j >= 0 {
				p.Str = value[1 : 1+j]
			}
		}
	case "VARTYPE_ARRAY_TIMEFLOAT":
		p.Curve = parseCurve(value)
	case "VARTYPE_ARRAY_TIMECOLOUR":
		p.Colour = parseColourRamp(value)
	case "VARTYPE_ARRAY_TIMEVECTOR3":
		p.Vec3 = parseVec3Curve(value)
	case "VARTYPE_VECTOR3":
		if body, ok := braceBody(value); ok {
			parts := fxNumRe.FindAllString(body, -1)
			if len(parts) >= 3 {
				p.Tuple.X, _ = strconv.ParseFloat(parts[0], 64)
				p.Tuple.Y, _ = strconv.ParseFloat(parts[1], 64)
				p.Tuple.Z, _ = strconv.ParseFloat(parts[2], 64)
			}
		}
	default:
		// Числа, булевы значения и небольшие перечисления начинаются
		// одинаково.
		if m := fxNumRe.FindString(value); m != "" {
			p.Num, _ = strconv.ParseFloat(m, 64)
		}
	}
	return p
}

// parseCurve читает плоскую таблицу {t, v, t, v, …} в пары.
func parseCurve(value string) []Key {
	body, ok := braceBody(value)
	if !ok {
		return nil
	}
	var nums []float64
	for _, m := range fxNumRe.FindAllString(body, -1) {
		if v, err := strconv.ParseFloat(m, 64); err == nil {
			nums = append(nums, v)
		}
	}
	if len(nums)%2 == 1 {
		nums = nums[:len(nums)-1]
	}
	keys := make([]Key, 0, len(nums)/2)
	for i := 0; i+1 < len(nums); i += 2 {
		keys = append(keys, Key{T: nums[i], V: nums[i+1]})
	}
	return keys
}

// parseVec3Curve читает { entry_NN = {t, x, y, z}, … }. Так записаны Arc и
// Offset ленты.
func parseVec3Curve(value string) []Vec3Key {
	body, ok := braceBody(value)
	if !ok {
		return nil
	}
	locs := fxEntryRe.FindAllStringIndex(body, -1)
	var keys []Vec3Key
	for i, loc := range locs {
		end := len(body)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		chunk := body[loc[1]:end]
		var nums []float64
		for _, m := range fxNumRe.FindAllString(chunk, -1) {
			if v, err := strconv.ParseFloat(m, 64); err == nil {
				nums = append(nums, v)
			}
		}
		if len(nums) < 4 {
			continue
		}
		keys = append(keys, Vec3Key{T: nums[0], X: nums[1], Y: nums[2], Z: nums[3]})
	}
	return keys
}

// parseColourRamp читает { entry_NN = {t, r, g, b, a}, … } в точки с
// каналами 0..1. Градиент, числа которого уже лежат в 0..1, считается
// записанным так изначально.
func parseColourRamp(value string) []ColourStop {
	body, ok := braceBody(value)
	if !ok {
		return nil
	}
	locs := fxEntryRe.FindAllStringIndex(body, -1)
	var stops []ColourStop
	normalized := !strings.Contains(body, "255") && !hasLargeColour(body, locs)
	for i, loc := range locs {
		end := len(body)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		chunk := body[loc[1]:end]
		var nums []float64
		for _, m := range fxNumRe.FindAllString(chunk, -1) {
			if v, err := strconv.ParseFloat(m, 64); err == nil {
				nums = append(nums, v)
			}
		}
		if len(nums) < 4 {
			continue
		}
		stop := ColourStop{T: nums[0]}
		scale := 1.0 / 255
		if normalized {
			scale = 1
		}
		stop.R, stop.G, stop.B = nums[1]*scale, nums[2]*scale, nums[3]*scale
		if len(nums) >= 5 {
			stop.A = nums[4] * scale
		} else {
			stop.A = 1
		}
		stops = append(stops, stop)
	}
	return stops
}

// hasLargeColour — есть ли среди первых цветовых чисел значение больше
// того, что дал бы градиент, записанный в 0..1.
func hasLargeColour(body string, locs [][]int) bool {
	for i, loc := range locs {
		end := len(body)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		nums := fxNumRe.FindAllString(body[loc[1]:end], -1)
		if len(nums) >= 2 {
			if v, err := strconv.ParseFloat(nums[1], 64); err == nil && v > 1.5 {
				return true
			}
		}
	}
	return false
}

// braceBody возвращает содержимое первых {…} значения.
func braceBody(value string) (string, bool) {
	open := strings.IndexByte(value, '{')
	if open < 0 {
		return "", false
	}
	depth := 0
	for i := open; i < len(value); i++ {
		switch value[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return value[open+1 : i], true
			}
		}
	}
	return "", false
}
