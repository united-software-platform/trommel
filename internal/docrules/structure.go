package docrules

import (
	"fmt"
	"strings"

	"github.com/united-software-platform/trommel/internal/rules"
	"github.com/united-software-platform/trommel/internal/verdict"
)

// CodeLanguage реализует DOC-016: у каждого огороженного блока кода указан язык.
type CodeLanguage struct{}

// Code возвращает код правила.
func (CodeLanguage) Code() string { return "DOC-016" }

// Subject возвращает предмет проверки.
func (CodeLanguage) Subject() rules.Subject { return rules.SubjectTree }

// RequiredParams сообщает, что правило параметров не требует.
func (CodeLanguage) RequiredParams() []string { return nil }

// Check ищет блоки кода без указания языка.
func (c CodeLanguage) Check(ctx rules.Context) verdict.Result {
	return walk(ctx, func(doc Document) []verdict.Finding {
		var findings []verdict.Finding

		for _, line := range doc.Lines {
			if !line.Opens || line.Info != "" {
				continue
			}
			findings = append(findings, verdict.Finding{
				File:    doc.Path,
				Line:    line.Number,
				Message: "блок кода без указания языка",
			})
		}
		return findings
	})
}

// Samples предъявляет образцы самопроверки.
func (CodeLanguage) Samples() rules.Samples {
	return rules.Samples{
		Violating: []rules.Sample{
			{Name: "безъязыка.md", Content: "Пример:\n\n```\nmake init\n```\n"},
		},
		Clean: []rules.Sample{
			{Name: "сязыком.md", Content: "Пример:\n\n```bash\nmake init\n```\n"},
			{Name: "вложенный.md", Content: "````markdown\n```bash\nmake init\n```\n````\n"},
		},
	}
}

// HeadingLevels реализует DOC-012: уровни заголовков не пропускаются.
type HeadingLevels struct{}

// Code возвращает код правила.
func (HeadingLevels) Code() string { return "DOC-012" }

// Subject возвращает предмет проверки.
func (HeadingLevels) Subject() rules.Subject { return rules.SubjectTree }

// RequiredParams сообщает, что правило параметров не требует.
func (HeadingLevels) RequiredParams() []string { return nil }

// Check ищет пропуски уровней в иерархии заголовков.
func (h HeadingLevels) Check(ctx rules.Context) verdict.Result {
	return walk(ctx, func(doc Document) []verdict.Finding {
		var findings []verdict.Finding

		previous := 0
		for _, line := range doc.Lines {
			if line.InCode {
				continue
			}

			level := headingLevel(line.Text)
			if level == 0 {
				continue
			}

			if previous > 0 && level > previous+1 {
				findings = append(findings, verdict.Finding{
					File:    doc.Path,
					Line:    line.Number,
					Message: fmt.Sprintf("пропуск уровня заголовка: %d после %d", level, previous),
				})
			}
			previous = level
		}
		return findings
	})
}

// Samples предъявляет образцы самопроверки.
func (HeadingLevels) Samples() rules.Samples {
	return rules.Samples{
		Violating: []rules.Sample{
			{Name: "пропуск.md", Content: "# Документ\n\n### Раздел\n"},
			{Name: "глубже.md", Content: "# Документ\n\n## Раздел\n\n#### Подраздел\n"},
		},
		Clean: []rules.Sample{
			{Name: "порядок.md", Content: "# Документ\n\n## Раздел\n\n### Подраздел\n"},
			{Name: "возврат.md", Content: "# Документ\n\n## Первый\n\n### Вложенный\n\n## Второй\n"},
			{Name: "вкоде.md", Content: "# Документ\n\n```markdown\n#### Пример\n```\n"},
		},
	}
}

// headingLevel возвращает уровень заголовка строки или ноль.
func headingLevel(text string) int {
	trimmed := strings.TrimLeft(text, " ")

	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || level >= len(trimmed) || trimmed[level] != ' ' {
		return 0
	}
	return level
}

// FileOpening реализует DOC-014: документ открывается заголовком, назначением
// и секцией навигации.
//
// Проверяется порядок трёх обязательных элементов, а не их содержание: объём
// назначения и полнота навигации остаются предметом суждения.
type FileOpening struct{}

// Code возвращает код правила.
func (FileOpening) Code() string { return "DOC-014" }

// Subject возвращает предмет проверки.
func (FileOpening) Subject() rules.Subject { return rules.SubjectTree }

// RequiredParams сообщает, что правило параметров не требует.
func (FileOpening) RequiredParams() []string { return nil }

// Check проверяет открывающие элементы документа.
func (f FileOpening) Check(ctx rules.Context) verdict.Result {
	return walk(ctx, func(doc Document) []verdict.Finding {
		var (
			title      bool
			purpose    bool
			navigation bool
			titleLine  int
		)

		for _, line := range doc.Lines {
			if line.InCode {
				continue
			}
			trimmed := strings.TrimSpace(line.Text)
			if trimmed == "" {
				continue
			}

			switch {
			case !title:
				if headingLevel(line.Text) != 1 {
					return []verdict.Finding{{
						File:    doc.Path,
						Line:    line.Number,
						Message: "документ открывается не заголовком первого уровня",
					}}
				}
				title, titleLine = true, line.Number
			case !purpose:
				if headingLevel(line.Text) > 0 {
					return []verdict.Finding{{
						File:    doc.Path,
						Line:    line.Number,
						Message: "между заголовком и первым разделом нет назначения документа",
					}}
				}
				purpose = true
			case strings.EqualFold(trimmed, "## Навигация"):
				navigation = true
			}

			if navigation {
				break
			}
		}

		if !title {
			return []verdict.Finding{{File: doc.Path, Message: "документ пуст или без заголовка"}}
		}
		if !navigation {
			return []verdict.Finding{{
				File:    doc.Path,
				Line:    titleLine,
				Message: "в документе нет секции навигации",
			}}
		}
		return nil
	})
}

// Samples предъявляет образцы самопроверки.
func (FileOpening) Samples() rules.Samples {
	const правильный = "# Документ\n\nНазначение в одну строку.\n\n---\n\n" +
		"## Навигация\n\n- [Раздел](#раздел)\n\n---\n\n## Раздел\n\nТекст.\n"

	return rules.Samples{
		Violating: []rules.Sample{
			{Name: "безназначения.md", Content: "# Документ\n\n## Навигация\n\n- [Раздел](#раздел)\n"},
			{Name: "безнавигации.md", Content: "# Документ\n\nНазначение.\n\n## Раздел\n\nТекст.\n"},
			{Name: "беззаголовка.md", Content: "## Раздел\n\nТекст без заголовка документа.\n"},
		},
		Clean: []rules.Sample{
			{Name: "правильный.md", Content: правильный},
		},
	}
}
