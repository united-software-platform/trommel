package docrules

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/united-software-platform/trommel/internal/rules"
	"github.com/united-software-platform/trommel/internal/verdict"
)

// htmlPattern — конструкции, которые CommonMark относит к HTML: открывающий тег,
// закрывающий тег, комментарий, объявление и инструкция обработки.
var htmlPattern = regexp.MustCompile(
	`</?[a-zA-Z][a-zA-Z0-9-]*(\s[^<>]*)?/?>|<!--|<!\[CDATA\[|<\?`)

// autolinkPattern — ссылка в угловых скобках: это разметка Markdown, а не HTML.
var autolinkPattern = regexp.MustCompile(`^<[a-zA-Z][a-zA-Z0-9+.-]*:[^<>\s]*>$|^<[^<>\s@]+@[^<>\s]+>$`)

// wikilinkPattern — ссылка формата Wikilink.
var wikilinkPattern = regexp.MustCompile(`\[\[[^\]]+\]\]`)

// HTML реализует DOC-005: запрет HTML целиком вне вставок и блоков кода.
type HTML struct{}

// Code возвращает код правила.
func (HTML) Code() string { return "DOC-005" }

// Subject возвращает предмет проверки.
func (HTML) Subject() rules.Subject { return rules.SubjectTree }

// RequiredParams сообщает, что правило параметров не требует.
func (HTML) RequiredParams() []string { return nil }

// Check ищет конструкции HTML в документах.
func (h HTML) Check(ctx rules.Context) verdict.Result {
	return walk(ctx, func(doc Document) []verdict.Finding {
		var findings []verdict.Finding

		for _, line := range doc.Lines {
			if line.InCode {
				continue
			}

			text := withoutInlineCode(line.Text)
			for _, match := range htmlPattern.FindAllString(text, -1) {
				if autolinkPattern.MatchString(match) {
					continue
				}
				findings = append(findings, verdict.Finding{
					File:    doc.Path,
					Line:    line.Number,
					Message: fmt.Sprintf("конструкция HTML %s вне блока кода", match),
				})
			}
		}
		return findings
	})
}

// Samples предъявляет образцы самопроверки.
func (HTML) Samples() rules.Samples {
	return rules.Samples{
		Violating: []rules.Sample{
			{Name: "тег.md", Content: "Текст с <div> внутри.\n"},
			{Name: "комментарий.md", Content: "<!-- скрытый комментарий -->\n"},
			{Name: "закрывающий.md", Content: "Текст</span>\n"},
		},
		Clean: []rules.Sample{
			{Name: "вставка.md", Content: "Запрещён тег `<div>` в тексте.\n"},
			{Name: "блок.md", Content: "```html\n<div>пример</div>\n```\n"},
			{Name: "ссылка.md", Content: "Адрес <https://example.org> в угловых скобках.\n"},
		},
	}
}

// Wikilink реализует DOC-006: запрет ссылок формата Wikilink.
type Wikilink struct{}

// Code возвращает код правила.
func (Wikilink) Code() string { return "DOC-006" }

// Subject возвращает предмет проверки.
func (Wikilink) Subject() rules.Subject { return rules.SubjectTree }

// RequiredParams сообщает, что правило параметров не требует.
func (Wikilink) RequiredParams() []string { return nil }

// Check ищет ссылки формата Wikilink.
func (w Wikilink) Check(ctx rules.Context) verdict.Result {
	return walk(ctx, func(doc Document) []verdict.Finding {
		var findings []verdict.Finding

		for _, line := range doc.Lines {
			if line.InCode {
				continue
			}

			text := withoutInlineCode(line.Text)
			for _, match := range wikilinkPattern.FindAllString(text, -1) {
				findings = append(findings, verdict.Finding{
					File:    doc.Path,
					Line:    line.Number,
					Message: fmt.Sprintf("ссылка формата Wikilink %s", match),
				})
			}
		}
		return findings
	})
}

// Samples предъявляет образцы самопроверки.
func (Wikilink) Samples() rules.Samples {
	return rules.Samples{
		Violating: []rules.Sample{
			{Name: "вики.md", Content: "Смотри [[Другая заметка]].\n"},
		},
		Clean: []rules.Sample{
			{Name: "ссылка.md", Content: "Смотри [другой документ](./other.md).\n"},
			{Name: "вставка.md", Content: "Запрещены `[[Note]]` в тексте.\n"},
			{Name: "массив.md", Content: "Значение [[1, 2], [3]] в примере.\n"},
		},
	}
}

// SetextHeading реализует DOC-008: запрет подчёркивания заголовков.
//
// Подчёркивание отличается от тематического разделителя предыдущей строкой: у заголовка
// она непустая и содержит его текст, у разделителя — пуста.
type SetextHeading struct{}

// Code возвращает код правила.
func (SetextHeading) Code() string { return "DOC-008" }

// Subject возвращает предмет проверки.
func (SetextHeading) Subject() rules.Subject { return rules.SubjectTree }

// RequiredParams сообщает, что правило параметров не требует.
func (SetextHeading) RequiredParams() []string { return nil }

// Check ищет заголовки, подчёркнутые вместо решёток.
func (s SetextHeading) Check(ctx rules.Context) verdict.Result {
	return walk(ctx, func(doc Document) []verdict.Finding {
		var findings []verdict.Finding

		for index, line := range doc.Lines {
			if line.InCode || index == 0 {
				continue
			}
			if !isUnderline(line.Text) {
				continue
			}

			previous := doc.Lines[index-1]
			if previous.InCode || strings.TrimSpace(previous.Text) == "" {
				continue
			}
			if strings.HasPrefix(strings.TrimSpace(previous.Text), "|") {
				continue
			}

			findings = append(findings, verdict.Finding{
				File:    doc.Path,
				Line:    line.Number,
				Message: "заголовок подчёркнут вместо уровня решётками",
			})
		}
		return findings
	})
}

// Samples предъявляет образцы самопроверки.
func (SetextHeading) Samples() rules.Samples {
	return rules.Samples{
		Violating: []rules.Sample{
			{Name: "равно.md", Content: "Заголовок документа\n===================\n"},
			{Name: "дефис.md", Content: "Название раздела\n----------------\n"},
		},
		Clean: []rules.Sample{
			{Name: "решётки.md", Content: "# Заголовок документа\n\n## Название раздела\n"},
			{Name: "разделитель.md", Content: "Абзац текста.\n\n---\n\nСледующий абзац.\n"},
			{Name: "таблица.md", Content: "| ID | Правило |\n|----|---------|\n| X-001 | Текст |\n"},
		},
	}
}

// isUnderline отвечает, состоит ли строка только из знаков подчёркивания заголовка.
func isUnderline(text string) bool {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) < 2 {
		return false
	}
	return strings.Trim(trimmed, "=") == "" || strings.Trim(trimmed, "-") == ""
}
