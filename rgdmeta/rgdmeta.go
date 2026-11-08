// Package rgdmeta читает из чертежей объектов Dawn of War (.rgd) отдельные
// поля, нужные игровой логике.
//
// Полный парсер RGD живёт в dow-mod-studio (internal/rgd), но internal-пакеты
// недоступны другим модулям, поэтому здесь свой минимальный читатель. Имена
// в RGD хранятся не строками, а хэшами (Jenkins lookup3 из hash.c игры), так
// что искать нужные поля можно сразу по хэшу — словарь RGD_DIC не нужен.
//
// Сейчас читается ровно одно поле: sim_entity_ext/is_collide, от которого в
// игре зависит, участвует ли объект в коллизиях, то есть доступен ли он для
// выделения.
package rgdmeta

import (
	"encoding/binary"
	"strings"

	"dow-mod-studio/modapi"
)

// Типы значений RGD.
const (
	rgdTypeBool     uint32 = 2
	rgdTypeTable    uint32 = 100
	rgdTypeTableInt uint32 = 101
)

// Глубина вложенности таблиц, которую мы готовы обходить.
const maxDepth = 8

// Хэши искомых полей: hash("sim_entity_ext") и hash("is_collide").
var (
	simEntityExtHash = hash("sim_entity_ext")
	isCollideHash    = hash("is_collide")
)

// Collide читает sim_entity_ext/is_collide у чертежа объекта. found=false
// означает, что файла нет, он не разобрался или поля в нём нет.
func Collide(tree *modapi.ModTree, blueprint string) (value, found bool) {
	if tree == nil {
		return false, false
	}
	data, err := tree.ReadFile(readPath(blueprint))
	if err != nil || len(data) == 0 {
		return false, false
	}
	return IsCollide(data)
}

// CollideMap читает is_collide для списка чертежей. Ключи карты —
// нормализованные пути (см. Key); в карту попадает только то, что удалось
// прочитать, поэтому отсутствие ключа означает «неизвестно».
func CollideMap(tree *modapi.ModTree, blueprints []string) map[string]bool {
	out := make(map[string]bool, len(blueprints))
	if tree == nil {
		return out
	}
	for _, blueprint := range blueprints {
		if blueprint == "" {
			continue
		}
		key := Key(blueprint)
		if _, seen := out[key]; seen {
			continue
		}
		if value, ok := Collide(tree, blueprint); ok {
			out[key] = value
		}
	}
	return out
}

// Key нормализует путь чертежа для поиска в CollideMap: в нижний регистр,
// только '/', без схемы и префикса Data, с расширением .rgd.
func Key(name string) string {
	p := stripScheme(strings.ReplaceAll(strings.TrimSpace(name), `\`, "/"))
	p = strings.TrimLeft(p, "/")
	if !strings.EqualFold(p, "") && len(p) >= 5 && !strings.EqualFold(p[:5], "data/") {
		p = "data/" + p
	}
	if !strings.EqualFold(p, "") && !strings.HasSuffix(strings.ToLower(p), ".rgd") {
		p += ".rgd"
	}
	return strings.ToLower(p)
}

// readPath собирает путь для чтения из дерева модуля в каноническом виде
// (как его строит sgbview: "Data/..." с сохранением регистра).
func readPath(name string) string {
	p := stripScheme(strings.ReplaceAll(strings.TrimSpace(name), `\`, "/"))
	p = strings.TrimLeft(p, "/")
	if len(p) < 5 || !strings.EqualFold(p[:5], "data/") {
		p = "Data/" + p
	}
	if !strings.HasSuffix(strings.ToLower(p), ".rgd") {
		p += ".rgd"
	}
	return p
}

// ReadPath — канонический путь чтения чертежа из дерева модуля (см.
// readPath): публикуется, потому что остальным пакетам приходится читать
// чертежи по именам из сценария.
func ReadPath(name string) string { return readPath(name) }

// stripScheme убирает префикс-схему из путей .screen/sgb вида "generic:art/...".
func stripScheme(value string) string {
	colon := strings.IndexByte(value, ':')
	if colon <= 0 {
		return value
	}
	for _, r := range value[:colon] {
		if r != '_' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return value
		}
	}
	return value[colon+1:]
}

// IsCollide разбирает байты .rgd и возвращает sim_entity_ext/is_collide.
// found=false — поля нет или файл не разобрался.
func IsCollide(data []byte) (value, found bool) {
	chunk, ok := gameDataChunk(data)
	if !ok {
		return false, false
	}
	// Поле лежит в таблице sim_entity_ext; на всякий случай смотрим и в корне.
	if pos, ok := findBool(chunk, 0, isCollideHash, 0); ok {
		return chunk[pos] != 0, true
	}
	ext, ok := findTable(chunk, 0, simEntityExtHash, 0)
	if !ok {
		return false, false
	}
	pos, ok := findBool(chunk, ext, isCollideHash, 1)
	if !ok {
		return false, false
	}
	return chunk[pos] != 0, true
}

// gameDataChunk находит в контейнере Relic Chunky данные таблицы GameData.
func gameDataChunk(data []byte) ([]byte, bool) {
	if len(data) < 24 || string(data[:12]) != "Relic Chunky" {
		return nil, false
	}
	le := binary.LittleEndian
	pos := 16
	version := le.Uint32(data[pos:])
	pos += 4
	_ = le.Uint32(data[pos:]) // unknown3
	pos += 4
	if version != 1 && version != 3 {
		return nil, false
	}
	if version == 3 {
		if pos+12 > len(data) {
			return nil, false
		}
		pos += 12
	}
	for pos+8 <= len(data) {
		chunkType := string(data[pos : pos+8])
		pos += 8
		if pos+4 > len(data) {
			break
		}
		pos += 4 // версия чанка
		if pos+4 > len(data) {
			break
		}
		pos += 4 // длина чанка
		if pos+4 > len(data) {
			break
		}
		strLen := int(le.Uint32(data[pos:]))
		pos += 4
		if strLen < 0 || pos+strLen > len(data) {
			break
		}
		pos += strLen // дескриптор
		if version == 3 {
			if pos+8 > len(data) {
				break
			}
			pos += 8
		}
		if pos+8 > len(data) {
			break
		}
		pos += 4 // crc
		dataLen := int(le.Uint32(data[pos:]))
		pos += 4
		if dataLen < 0 || pos+dataLen > len(data) {
			break
		}
		payload := data[pos : pos+dataLen]
		pos += dataLen
		if chunkType == "DATAAEGD" {
			return payload, true
		}
	}
	return nil, false
}

// tableHeaders разбирает заголовок таблицы по смещению base: ключей, начало
// блока значений и список (hash, type, offset).
func tableHeaders(data []byte, base int) (keyCount int, dataStart int, ok bool) {
	if base < 0 || base+4 > len(data) {
		return 0, 0, false
	}
	le := binary.LittleEndian
	keyCount = int(le.Uint32(data[base:]))
	if keyCount <= 0 || keyCount > 4096 {
		return 0, 0, false
	}
	headers := base + 4
	dataStart = headers + keyCount*12
	if headers < 0 || dataStart > len(data) {
		return 0, 0, false
	}
	return keyCount, dataStart, true
}

// findBool ищет запись want с типом bool в таблице base и возвращает позицию
// байта значения.
func findBool(data []byte, base int, want uint32, depth int) (int, bool) {
	if depth > maxDepth {
		return 0, false
	}
	keyCount, dataStart, ok := tableHeaders(data, base)
	if !ok {
		return 0, false
	}
	le := binary.LittleEndian
	headers := base + 4
	for i := 0; i < keyCount; i++ {
		off := headers + i*12
		if off+12 > len(data) {
			return 0, false
		}
		if le.Uint32(data[off:]) != want || le.Uint32(data[off+4:]) != rgdTypeBool {
			continue
		}
		valPos := dataStart + int(le.Uint32(data[off+8:]))
		if valPos < 0 || valPos >= len(data) {
			return 0, false
		}
		return valPos, true
	}
	return 0, false
}

// findTable ищет запись want типа table и возвращает смещение её данных,
// при необходимости обходя вложенные таблицы.
func findTable(data []byte, base int, want uint32, depth int) (int, bool) {
	if depth > maxDepth {
		return 0, false
	}
	keyCount, dataStart, ok := tableHeaders(data, base)
	if !ok {
		return 0, false
	}
	le := binary.LittleEndian
	headers := base + 4
	for i := 0; i < keyCount; i++ {
		off := headers + i*12
		if off+12 > len(data) {
			return 0, false
		}
		hash := le.Uint32(data[off:])
		typ := le.Uint32(data[off+4:])
		valPos := dataStart + int(le.Uint32(data[off+8:]))
		if valPos < 0 || valPos > len(data) {
			continue
		}
		if hash == want && (typ == rgdTypeTable || typ == rgdTypeTableInt) {
			return valPos, true
		}
		if typ == rgdTypeTable || typ == rgdTypeTableInt {
			if nested, ok := findTable(data, valPos, want, depth+1); ok {
				return nested, true
			}
		}
	}
	return 0, false
}

// hash — Jenkins lookup3 (Bob Jenkins, как в hash.c игры): по нему игра
// находит имена полей в RGD.
func hash(s string) uint32 {
	k := []byte(s)
	var a, b, c uint32
	a = 0x9e3779b9
	b = 0x9e3779b9
	c = 0
	origLen := uint32(len(k))
	length := origLen
	i := 0
	for int(length) >= 12 {
		a += uint32(k[i]) | uint32(k[i+1])<<8 | uint32(k[i+2])<<16 | uint32(k[i+3])<<24
		b += uint32(k[i+4]) | uint32(k[i+5])<<8 | uint32(k[i+6])<<16 | uint32(k[i+7])<<24
		c += uint32(k[i+8]) | uint32(k[i+9])<<8 | uint32(k[i+10])<<16 | uint32(k[i+11])<<24
		mix(&a, &b, &c)
		i += 12
		length -= 12
	}
	c += origLen
	switch length {
	case 11:
		c += uint32(k[i+10]) << 24
		fallthrough
	case 10:
		c += uint32(k[i+9]) << 16
		fallthrough
	case 9:
		c += uint32(k[i+8]) << 8
		fallthrough
	case 8:
		b += uint32(k[i+7]) << 24
		fallthrough
	case 7:
		b += uint32(k[i+6]) << 16
		fallthrough
	case 6:
		b += uint32(k[i+5]) << 8
		fallthrough
	case 5:
		b += uint32(k[i+4])
		fallthrough
	case 4:
		a += uint32(k[i+3]) << 24
		fallthrough
	case 3:
		a += uint32(k[i+2]) << 16
		fallthrough
	case 2:
		a += uint32(k[i+1]) << 8
		fallthrough
	case 1:
		a += uint32(k[i])
	}
	mix(&a, &b, &c)
	return c
}

func mix(a, b, c *uint32) {
	*a -= *b
	*a -= *c
	*a ^= *c >> 13
	*b -= *c
	*b -= *a
	*b ^= *a << 8
	*c -= *a
	*c -= *b
	*c ^= *b >> 13
	*a -= *b
	*a -= *c
	*a ^= *c >> 12
	*b -= *c
	*b -= *a
	*b ^= *a << 16
	*c -= *a
	*c -= *b
	*c ^= *b >> 5
	*a -= *b
	*a -= *c
	*a ^= *c >> 3
	*b -= *c
	*b -= *a
	*b ^= *a << 10
	*c -= *a
	*c -= *b
	*c ^= *b >> 15
}
