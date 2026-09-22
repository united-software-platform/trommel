// Пакет catalog строит перечень правил действующей нормы по документам, которые её
// объявляют. Источником служат таблицы правил самих документов: правило, добавленное
// в норму, попадает в каталог без изменений в коде Trommel.
package catalog

import (
	"fmt"
	"regexp"
	"strings"
)

// codePattern — формат кода правила: префикс понятия латиницей и трёхзначный номер.
// Совпадает с форматом идентификатора сущности, принятым в норме.
var codePattern = regexp.MustCompile(`^([A-Z]{2,10})-([0-9]{3})$`)

// Rule — одно правило нормы: код, группа, текст и место объявления.
type Rule struct {
	Code   string // код правила целиком, например DOC-005
	Group  string // префикс кода, он же группа: DOC
	Text   string // формулировка из таблицы правил
	Source string // документ, объявивший правило
	Line   int    // номер строки объявления, для сообщений об ошибках
}

// parseCode разбирает код правила и возвращает его группу. Несоответствие формату —
// ошибка разбора, а не повод пропустить строку: пропуск означал бы потерю правила,
// о которой никто не узнает.
func parseCode(code string) (group string, err error) {
	match := codePattern.FindStringSubmatch(code)
	if match == nil {
		return "", fmt.Errorf("код правила %q не соответствует формату {ПРЕФИКС}-{NNN}", code)
	}
	return match[1], nil
}

// String даёт краткое представление правила для сообщений.
func (r Rule) String() string {
	return fmt.Sprintf("%s (%s:%d)", r.Code, r.Source, r.Line)
}

// splitRow разбирает строку таблицы Markdown на ячейки без внешних разделителей.
func splitRow(line string) []string {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimPrefix(trimmed, "|")
	trimmed = strings.TrimSuffix(trimmed, "|")

	cells := strings.Split(trimmed, "|")
	for i, cell := range cells {
		cells[i] = strings.TrimSpace(cell)
	}
	return cells
}
