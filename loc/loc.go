// Package loc — локализация интерфейса.
//
// Английская строка в коде — это и есть ключ: loc.T("Resume") возвращает
// перевод для действующего языка, а если перевода нет — саму английскую
// строку. Поэтому по умолчанию интерфейс английский, а новый язык — это
// ещё одна таблица рядом (см. loc/ru.go): её заводят одной картой
// «английская строка → перевод».
package loc

import "sync/atomic"

// Lang — язык интерфейса.
type Lang string

// Поддерживаемые языки. Английский — язык по умолчанию.
const (
	EN Lang = "en"
	RU Lang = "ru"
)

// catalog — переводы по языкам. Английского здесь нет: английские строки
// лежат прямо в коде как ключи.
var catalog = map[Lang]map[string]string{
	RU: ru,
}

// currentLang — действующий язык (atomic: читается и из кадра, и из звука).
var currentLang atomic.Value

func init() {
	currentLang.Store(EN)
}

// SetLang задаёт язык интерфейса; неизвестный язык возвращает к английскому.
func SetLang(l Lang) {
	if _, ok := catalog[l]; !ok {
		l = EN
	}
	currentLang.Store(l)
}

// Current — действующий язык.
func Current() Lang {
	l, _ := currentLang.Load().(Lang)
	if l == "" {
		return EN
	}
	return l
}

// T возвращает строку интерфейса на действующем языке. Ключ — английская
// строка; нет перевода — возвращается она же.
func T(key string) string {
	if l := Current(); l != EN {
		if s, ok := catalog[l][key]; ok {
			return s
		}
	}
	return key
}
