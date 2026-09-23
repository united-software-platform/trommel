// Пакет scope задаёт область прогона: какая часть дерева проверяемого проекта
// в неё входит.
//
// Определение вынесено в отдельный пакет, потому что область обязана значить одно
// и то же для всех предметов проверки — и для документов, и для фактов о коде.
// Разойдись они, одна и та же карта убирала бы из вердикта разное в зависимости
// от того, какое правило её читает.
package scope

import (
	"path"
	"sort"
	"strings"
)

// Map — карта проекта: что сканируется и что не сканируется.
//
// Карта — ответ на стоимость полного съёма фактов. Снять все уровни разбора можно
// только с той части проекта, о которой спрашивают: соседний каталог с чужой библиотекой
// стоит столько же, сколько собственный код, и молчаливое включение его в область
// превращает прогон в многочасовой.
type Map struct {
	// Scan — сканируемые пути от корня проекта. Пустой перечень означает проект
	// целиком: карта сужает область, но не обязана её задавать.
	Scan []string
	// Exclude — пути, выведенные из области. Действуют и внутри сканируемых путей.
	Exclude []string
}

// Covers отвечает, входит ли путь в область карты.
//
// Исключение сильнее включения: путь, названный и сканируемым, и исключённым,
// в область не входит. Иначе исключение внутри сканируемого каталога не выражалось бы
// вовсе, а оно и есть обычный случай — «этот модуль, кроме сгенерированного подкаталога».
func (m Map) Covers(candidate string) bool {
	candidate = clean(candidate)
	if candidate == "" {
		return false
	}
	if under(candidate, m.Exclude) {
		return false
	}
	if len(m.Scan) == 0 {
		return true
	}
	return under(candidate, m.Scan) || covers(candidate, m.Scan)
}

// Roots возвращает пути, с которых начинается обход, в устойчивом порядке.
// Пустая карта даёт корень проекта: обход всего дерева.
func (m Map) Roots() []string {
	if len(m.Scan) == 0 {
		return []string{"."}
	}

	out := make([]string, 0, len(m.Scan))
	for _, candidate := range m.Scan {
		if candidate = clean(candidate); candidate != "" {
			out = append(out, candidate)
		}
	}
	if len(out) == 0 {
		return []string{"."}
	}
	sort.Strings(out)
	return out
}

// Narrowed отвечает, сужает ли карта область прогона.
func (m Map) Narrowed() bool { return len(m.Scan) > 0 || len(m.Exclude) > 0 }

// Excluded отвечает, выведен ли путь из области исключением карты.
//
// Отдельный ответ нужен обходу дерева: исключённый каталог пропускается целиком,
// тогда как каталог вне сканируемых путей может содержать сканируемый подкаталог.
func Excluded(candidate string, excludeDirs []string) bool {
	return under(clean(candidate), excludeDirs)
}

// under отвечает, лежит ли путь в одном из перечисленных путей или совпадает с ним.
func under(candidate string, paths []string) bool {
	for _, prefix := range paths {
		if prefix = clean(prefix); prefix != "" &&
			(candidate == prefix || strings.HasPrefix(candidate, prefix+"/")) {
			return true
		}
	}
	return false
}

// covers отвечает, содержит ли путь хотя бы один из перечисленных: каталог верхнего
// уровня входит в обход, если сканируется лежащий в нём подкаталог.
func covers(candidate string, paths []string) bool {
	for _, nested := range paths {
		if nested = clean(nested); nested != "" && strings.HasPrefix(nested, candidate+"/") {
			return true
		}
	}
	return false
}

// clean приводит путь к виду, в котором пути сравнимы между собой.
func clean(candidate string) string {
	candidate = strings.TrimSpace(candidate)
	candidate = strings.Trim(candidate, "/")
	if candidate == "" || candidate == "." {
		return ""
	}
	return path.Clean(candidate)
}
