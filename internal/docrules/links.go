package docrules

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/united-software-platform/trommel/internal/rules"
	"github.com/united-software-platform/trommel/internal/verdict"
)

// linkPattern — ссылка Markdown вместе с необязательным восклицательным знаком
// изображения: `[текст](цель)` и `![alt](цель)`.
var linkPattern = regexp.MustCompile(`(!?)\[[^\]]*\]\(([^)\s]+)[^)]*\)`)

// absolutePattern — адрес, не являющийся относительным: со схемой, протоколо-независимый
// либо отсчитываемый от корня.
var absolutePattern = regexp.MustCompile(`^([a-zA-Z][a-zA-Z0-9+.-]*:|//|/)`)

// RelativeLinks реализует DOC-015: ссылки только относительные.
//
// Исключение действует для README.md: он адресован читателю извне, и цель ссылки
// там бывает действительно внешней — ресурс вне репозитория, к которому
// относительного пути не существует.
type RelativeLinks struct{}

// Code возвращает код правила.
func (RelativeLinks) Code() string { return "DOC-015" }

// Subject возвращает предмет проверки.
func (RelativeLinks) Subject() rules.Subject { return rules.SubjectTree }

// RequiredParams сообщает, что правило параметров не требует.
func (RelativeLinks) RequiredParams() []string { return nil }

// Check ищет абсолютные ссылки вне README.md.
func (r RelativeLinks) Check(ctx rules.Context) verdict.Result {
	return walk(ctx, func(doc Document) []verdict.Finding {
		if strings.EqualFold(path.Base(doc.Path), "README.md") {
			return nil
		}

		var findings []verdict.Finding
		for _, line := range doc.Lines {
			if line.InCode {
				continue
			}

			text := withoutInlineCode(line.Text)
			for _, match := range linkPattern.FindAllStringSubmatch(text, -1) {
				target := match[2]
				if !absolutePattern.MatchString(target) {
					continue
				}
				findings = append(findings, verdict.Finding{
					File:    doc.Path,
					Line:    line.Number,
					Message: fmt.Sprintf("абсолютная ссылка %s вне README.md", target),
				})
			}
		}
		return findings
	})
}

// Samples предъявляет образцы самопроверки.
func (RelativeLinks) Samples() rules.Samples {
	return rules.Samples{
		Violating: []rules.Sample{
			{Name: "правила.md", Content: "Смотри [проект](https://example.org/repo).\n"},
			{Name: "откорня.md", Content: "Смотри [документ](/docs/other.md).\n"},
		},
		Clean: []rules.Sample{
			{Name: "README.md", Content: "Смотри [проект](https://example.org/repo).\n"},
			{Name: "относительная.md", Content: "Смотри [документ](./other.md#якорь).\n"},
			{Name: "вкоде.md", Content: "Запись `[текст](https://example.org)` в примере.\n"},
		},
	}
}
