package verdict

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// Версия формата сводки покрытия и дата её последнего изменения.
//
// Таблица версий порождаемого документа описывает историю его формата, а не данных:
// данные меняются каждым прогоном, и запись о каждом прогоне превратила бы историю
// в шум. Дата фиксирована здесь и меняется вместе с форматом.
const (
	coverageFormatVersion = "1.0.0"
	coverageFormatDate    = "2026-09-22"
)

// statusTitle — название состояния для читателя сводки.
var statusTitle = map[Status]string{
	StatusOK:   "гарантировано",
	StatusFail: "нарушено",
	StatusErr:  "проверка не состоялась",
	StatusSkip: "неприменимо",
	StatusNone: "не проверяется",
}

// WriteMarkdown печатает сводку покрытия — документ, порождаемый прогоном.
//
// Сводка отвечает на вопрос, чего стоит зелёный прогон: рядом с каждым правилом нормы
// стоит его состояние и граница утверждения о нём. Правило без реализации остаётся
// в таблице с причиной, объявленной контрактом намерения, а не исчезает из неё.
func (r *Report) WriteMarkdown(out io.Writer) error {
	counts := r.Counts()
	total := len(r.Rules) + len(r.Excluded)

	fmt.Fprintf(out, "# Покрытие нормы\n\n")
	fmt.Fprintf(out,
		"Состояние каждого правила действующей нормы: что Trommel гарантирует, "+
			"что остаётся непроверенным и почему. Документ порождается прогоном "+
			"и вручную не правится.\n\n")
	fmt.Fprintf(out, "---\n\n## Навигация\n\n")
	fmt.Fprintf(out, "- [Сводка](#сводка)\n- [Правила состава](#правила-состава)\n")
	if len(r.Excluded) > 0 {
		fmt.Fprintf(out, "- [Вне состава](#вне-состава)\n")
	}
	fmt.Fprintf(out, "- [Версионирование](#версионирование)\n\n---\n\n")

	fmt.Fprintf(out, "## Сводка\n\n")
	fmt.Fprintf(out, "| Показатель | Значение |\n|------------|----------|\n")
	fmt.Fprintf(out, "| Правил в норме | %d |\n", total)
	fmt.Fprintf(out, "| В составе прогона | %d |\n", len(r.Rules))
	fmt.Fprintf(out, "| Проверено | %d |\n", r.Checked())
	for _, status := range []Status{StatusOK, StatusFail, StatusErr, StatusSkip, StatusNone} {
		fmt.Fprintf(out, "| %s | %d |\n", capitalize(statusTitle[status]), counts[status])
	}
	fmt.Fprintf(out, "| Каталог нормы | `%s` |\n", r.RulesSource)
	fmt.Fprintf(out, "| Отпечаток нормы | `%s` |\n", short(r.Fingerprint))
	fmt.Fprintf(out, "| Контракт | `%s` |\n", r.Contract)
	if len(r.ExcludeDirs) > 0 {
		fmt.Fprintf(out, "| Вне обхода | `%s` |\n", strings.Join(r.ExcludeDirs, "`, `"))
	}
	fmt.Fprintf(out, "\n---\n\n")

	fmt.Fprintf(out, "## Правила состава\n\n")
	fmt.Fprintf(out, "| Код | Правило | Состояние | Проверка |\n")
	fmt.Fprintf(out, "|-----|---------|-----------|----------|\n")
	for _, rule := range r.Rules {
		fmt.Fprintf(out, "| `%s` | %s | %s | %s |\n",
			rule.Code, cell(rule.Text), statusTitle[rule.Status], cell(checkText(rule)))
	}

	if len(r.Excluded) > 0 {
		fmt.Fprintf(out, "\n---\n\n## Вне состава\n\n")
		fmt.Fprintf(out, "Правила, выведенные из состава контрактом намерения. "+
			"Исключение не отменяет правила: оно означает, что этот прогон о нём "+
			"ничего не утверждает.\n\n")
		fmt.Fprintf(out, "| Код | Правило | Основание исключения |\n")
		fmt.Fprintf(out, "|-----|---------|----------------------|\n")
		for _, excluded := range sortedExclusions(r.Excluded) {
			fmt.Fprintf(out, "| `%s` | %s | %s |\n",
				excluded.Code, cell(excluded.Text), cell(excluded.Reason))
		}
	}

	fmt.Fprintf(out, "\n---\n\n## Версионирование\n\n")
	fmt.Fprintf(out, "| Версия | Дата | Задача | Агент | Модель | Описание изменений |\n")
	fmt.Fprintf(out, "|--------|------|--------|-------|--------|--------------------|\n")
	fmt.Fprintf(out,
		"| %s | %s | Порождение сводки покрытия прогоном | Trommel | — | "+
			"Формат документа: сводка, правила состава с состоянием и границей проверки, "+
			"правила вне состава с основанием исключения. Данные документа обновляются "+
			"каждым прогоном и в этой таблице не отражаются |\n",
		coverageFormatVersion, coverageFormatDate)

	return nil
}

// checkText возвращает границу проверки правила либо причину её отсутствия.
func checkText(rule RuleVerdict) string {
	if rule.Check != "" {
		return rule.Check
	}
	if rule.Status == StatusNone {
		return "реализации нет; причина контрактом не объявлена"
	}
	return rule.Detail
}

// sortedExclusions упорядочивает исключённые правила по коду.
func sortedExclusions(excluded []Excluded) []Excluded {
	out := make([]Excluded, len(excluded))
	copy(out, excluded)
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// cell готовит текст к помещению в ячейку таблицы Markdown.
func cell(text string) string {
	text = strings.ReplaceAll(text, "|", "\\|")
	return strings.Join(strings.Fields(text), " ")
}

// capitalize поднимает первую букву строки.
func capitalize(text string) string {
	runes := []rune(text)
	if len(runes) == 0 {
		return text
	}
	return strings.ToUpper(string(runes[0])) + string(runes[1:])
}
