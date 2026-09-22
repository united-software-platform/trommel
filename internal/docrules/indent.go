package docrules

import (
	"strings"

	"github.com/united-software-platform/trommel/internal/rules"
	"github.com/united-software-platform/trommel/internal/verdict"
)

// IndentedCode реализует DOC-007: запрет отступа в четыре пробела вместо огороженного
// блока кода.
//
// Отступ сам по себе нарушением не является: в списках он задаёт вложенность. Нарушение
// образует блок с отступом, начинающийся после пустой строки вне списка — именно такой
// блок CommonMark трактует как код.
type IndentedCode struct{}

// Code возвращает код правила.
func (IndentedCode) Code() string { return "DOC-007" }

// Subject возвращает предмет проверки.
func (IndentedCode) Subject() rules.Subject { return rules.SubjectTree }

// RequiredParams сообщает, что правило параметров не требует.
func (IndentedCode) RequiredParams() []string { return nil }

// Check ищет блоки кода, заданные отступом.
func (i IndentedCode) Check(ctx rules.Context) verdict.Result {
	return walk(ctx, func(doc Document) []verdict.Finding {
		var findings []verdict.Finding

		inList := false
		blankBefore := true

		for _, line := range doc.Lines {
			if line.InCode {
				continue
			}

			trimmed := strings.TrimSpace(line.Text)
			if trimmed == "" {
				blankBefore = true
				continue
			}

			indent := len(line.Text) - len(strings.TrimLeft(line.Text, " "))

			switch {
			case indent == 0:
				inList = isListItem(trimmed)
			case blankBefore && !inList && indent >= 4:
				findings = append(findings, verdict.Finding{
					File:    doc.Path,
					Line:    line.Number,
					Message: "блок кода задан отступом вместо огороженного блока",
				})
			}

			blankBefore = false
		}
		return findings
	})
}

// Describe описывает границу проверки.
func (IndentedCode) Describe() string {
	return "Блок с отступом в четыре пробела и более после пустой строки вне списка"
}

// Samples предъявляет образцы самопроверки.
func (IndentedCode) Samples() rules.Samples {
	return rules.Samples{
		Violating: []rules.Sample{
			{Name: "отступ.md", Content: "Пример вызова:\n\n    make init\n"},
		},
		Clean: []rules.Sample{
			{Name: "забор.md", Content: "Пример вызова:\n\n```bash\nmake init\n```\n"},
			{Name: "список.md", Content: "- пункт списка\n\n    продолжение пункта\n"},
			{Name: "текст.md", Content: "Обычный абзац без отступов.\n"},
		},
	}
}

// isListItem отвечает, открывает ли строка пункт списка.
func isListItem(trimmed string) bool {
	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") ||
		strings.HasPrefix(trimmed, "+ ") {
		return true
	}

	digits := 0
	for digits < len(trimmed) && trimmed[digits] >= '0' && trimmed[digits] <= '9' {
		digits++
	}
	return digits > 0 && digits+1 < len(trimmed) &&
		(trimmed[digits] == '.' || trimmed[digits] == ')') && trimmed[digits+1] == ' '
}
