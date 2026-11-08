// Разбор RGD в дерево узелов.
//
// Раньше rgdmeta умел ровно одно — найти один bool по хэшу (см. IsCollide).
// Чтобы читать чертежи юнитов (ebps) и отрядов (sbps), нужен весь файл:
// вложенные таблицы, числа, строки. Здесь описан общий разбор того же
// формата, что и в IsCollide, но с типами значений и именами узлов.
//
// Имена полей в RGD — хэши Jenkins lookup3 (см. hash). Имена из словаря
// (RGD_DIC) не обязательны: обход идёт по известным именам, которые мы
// хэшируем сами. Словарь нужен только для отладочной печати незнакомых полей.
package rgdmeta

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode/utf16"
)

// Типы значений RGD — как их объявляет игра.
const (
	TypeFloat    uint32 = 0
	TypeInt      uint32 = 1
	TypeBool     uint32 = 2
	TypeString   uint32 = 3
	TypeWString  uint32 = 4
	TypeTable    uint32 = 100
	TypeTableInt uint32 = 101
	TypeNoData   uint32 = 254
)

// refHash — хэш "$REF": запись со ссылкой-путём на другой чертёж.
const refHash = 0x49D60FAE

// Node — узел разобранного чертежа. Скалярные поля заполнены по типу узла,
// у таблиц заполнены Children. Ref — строка $REF, если таблица ссылается
// на другой файл (например, trooper_base/type указывает на ebps).
type Node struct {
	Hash     uint32
	Name     string
	Type     uint32
	Bool     bool
	Int      uint32
	Float    float32
	Str      string
	Children []*Node
	Ref      string
}

// IsTable — узел является вложенной таблицей.
func (n *Node) IsTable() bool {
	return n != nil && (n.Type == TypeTable || n.Type == TypeTableInt)
}

// Get ищет ребёнка по имени поля (по хэшу — словарь не нужен, а при
// наличии словаря срабатывает и совпадение имён). Путь проваливается по
// вложенным таблицам: Get("squad_loadout_ext", "unit_max").
func (n *Node) Get(path ...string) *Node {
	cur := n
	for _, name := range path {
		if cur == nil {
			return nil
		}
		var next *Node
		for _, child := range cur.Children {
			if child.Named(name) {
				next = child
				break
			}
		}
		cur = next
	}
	return cur
}

// Named — узел называется name: либо это его хэш, либо имя из словаря.
func (n *Node) Named(name string) bool {
	if n == nil {
		return false
	}
	return n.Hash == hash(name) || strings.EqualFold(n.Name, name)
}

// All возвращает всех детей с таким именем: некоторые таблицы повторяют
// ключ (trooper_base, leader_1…leader_NN — каждый своё имя, а вот
// trooper_base может быть несколько).
func (n *Node) All(name string) []*Node {
	if n == nil {
		return nil
	}
	var out []*Node
	for _, child := range n.Children {
		if child.Named(name) {
			out = append(out, child)
		}
	}
	return out
}

// BoolAt — значение bool по пути; found=false, если пути или значения нет.
func (n *Node) BoolAt(path ...string) (value, found bool) {
	node := n.Get(path...)
	if node == nil || node.Type != TypeBool {
		return false, false
	}
	return node.Bool, true
}

// IntAt — значение int по пути (RGD пишет целые и как int, и как float).
func (n *Node) IntAt(path ...string) (value uint32, found bool) {
	node := n.Get(path...)
	if node == nil {
		return 0, false
	}
	switch node.Type {
	case TypeInt:
		return node.Int, true
	case TypeFloat:
		return uint32(node.Float), true
	case TypeBool:
		if node.Bool {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

// FloatAt — значение float по пути.
func (n *Node) FloatAt(path ...string) (value float32, found bool) {
	node := n.Get(path...)
	if node == nil {
		return 0, false
	}
	switch node.Type {
	case TypeFloat:
		return node.Float, true
	case TypeInt:
		return float32(node.Int), true
	}
	return 0, false
}

// StrAt — строка по пути: строки и wide-строки отдаются как есть, числа и
// bool — в текстовом виде, потому что чертежи пишут ссылки то тем, то этим.
func (n *Node) StrAt(path ...string) (value string, found bool) {
	node := n.Get(path...)
	if node == nil {
		return "", false
	}
	switch node.Type {
	case TypeString, TypeWString:
		return node.Str, true
	case TypeInt:
		return fmt.Sprintf("%d", node.Int), true
	case TypeFloat:
		return fmt.Sprintf("%g", node.Float), true
	case TypeBool:
		if node.Bool {
			return "true", true
		}
		return "false", true
	}
	return "", false
}

// Parse разбирает байты .rgd в дерево. dict (hash → имя) нужен, чтобы в
// дереве были читаемые имена; nil допустим — тогда остаются хэши.
func Parse(data []byte, dict map[uint32]string) (*Node, error) {
	chunk, ok := gameDataChunk(data)
	if !ok {
		return nil, fmt.Errorf("rgd: нет чанка DATAAEGD")
	}
	children, ref, err := parseTable(chunk, 0, dict)
	if err != nil {
		return nil, err
	}
	root := &Node{Name: "GameData", Type: TypeTable, Children: children, Ref: ref}
	return root, nil
}

// parseTable разбирает таблицу по смещению base и возвращает её узлы и
// запись $REF (как её читает игра: $REF — свойство таблицы, а не ребёнок).
func parseTable(data []byte, base int, dict map[uint32]string) ([]*Node, string, error) {
	keyCount, dataStart, ok := tableHeaders(data, base)
	if !ok {
		return nil, "", fmt.Errorf("rgd: не разбирается заголовок таблицы")
	}
	le := binary.LittleEndian
	headers := base + 4
	var children []*Node
	var ref string
	for i := 0; i < keyCount; i++ {
		off := headers + i*12
		if off+12 > len(data) {
			return nil, "", fmt.Errorf("rgd: заголовок %d обрезан", i)
		}
		hash := le.Uint32(data[off:])
		typ := le.Uint32(data[off+4:])
		valPos := dataStart + int(le.Uint32(data[off+8:]))
		if valPos < 0 || valPos > len(data) {
			continue
		}
		name := hashName(hash, dict)
		node := &Node{Hash: hash, Name: name, Type: typ}
		switch typ {
		case TypeFloat:
			if valPos+4 > len(data) {
				return nil, "", fmt.Errorf("rgd: float обрезан у %s", name)
			}
			node.Float = math.Float32frombits(le.Uint32(data[valPos:]))
		case TypeInt:
			if valPos+4 > len(data) {
				return nil, "", fmt.Errorf("rgd: int обрезан у %s", name)
			}
			node.Int = le.Uint32(data[valPos:])
		case TypeBool:
			if valPos >= len(data) {
				return nil, "", fmt.Errorf("rgd: bool обрезан у %s", name)
			}
			node.Bool = data[valPos] != 0
		case TypeString:
			end := valPos
			for end < len(data) && data[end] != 0 {
				end++
			}
			node.Str = string(data[valPos:end])
		case TypeWString:
			var units []uint16
			for end := valPos; end+1 < len(data); end += 2 {
				v := le.Uint16(data[end:])
				if v == 0 {
					break
				}
				units = append(units, v)
			}
			node.Str = string(utf16.Decode(units))
		case TypeTable, TypeTableInt:
			sub, subRef, err := parseTable(data, valPos, dict)
			if err != nil {
				return nil, "", fmt.Errorf("rgd: таблица %s: %w", name, err)
			}
			node.Children = sub
			node.Type = TypeTable
			node.Ref = subRef
		case TypeNoData:
			// значения нет — узел остаётся пустым
		default:
			return nil, "", fmt.Errorf("rgd: неизвестный тип %d у %s", typ, name)
		}
		if hash == refHash {
			ref = node.Str
			continue
		}
		children = append(children, node)
	}
	sort.SliceStable(children, func(i, j int) bool { return children[i].Hash < children[j].Hash })
	return children, ref, nil
}

// hashName подставляет имя из словаря; без словаря остаётся хэш.
func hashName(hash uint32, dict map[uint32]string) string {
	if name, ok := dict[hash]; ok && name != "" {
		return name
	}
	return fmt.Sprintf("0x%08X", hash)
}
