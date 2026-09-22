package docrules

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/united-software-platform/trommel/internal/rules"
	"github.com/united-software-platform/trommel/internal/verdict"
)

// versionTableHeader — заголовки таблицы версий документа.
var versionTableHeader = []string{"Версия", "Дата", "Задача", "Агент", "Модель"}

// VersionTable реализует DOC-022: документ завершается таблицей версий.
// Исключение — README.md: он адресован читателю извне, и историю правок
// хранит система контроля версий.
type VersionTable struct{}

// Code возвращает код правила.
func (VersionTable) Code() string { return "DOC-022" }

// Subject возвращает предмет проверки.
func (VersionTable) Subject() rules.Subject { return rules.SubjectTree }

// RequiredParams сообщает, что правило параметров не требует.
func (VersionTable) RequiredParams() []string { return nil }

// Check проверяет наличие таблицы версий.
func (v VersionTable) Check(ctx rules.Context) verdict.Result {
	return walk(ctx, func(doc Document) []verdict.Finding {
		if strings.EqualFold(path.Base(doc.Path), "README.md") {
			return nil
		}
		if len(versionRows(doc)) > 0 {
			return nil
		}
		return []verdict.Finding{{File: doc.Path, Message: "в документе нет таблицы версий"}}
	})
}

// Samples предъявляет образцы самопроверки.
func (VersionTable) Samples() rules.Samples {
	return rules.Samples{
		Violating: []rules.Sample{
			{Name: "безверсий.md", Content: "# Документ\n\nТекст без истории изменений.\n"},
		},
		Clean: []rules.Sample{
			{Name: "сверсиями.md", Content: версии("1.0.0")},
			{Name: "README.md", Content: "# Проект\n\nОписание без таблицы версий.\n"},
		},
	}
}

// VersionOrder реализует DOC-024: строки таблицы версий идут по возрастанию версии.
type VersionOrder struct{}

// Code возвращает код правила.
func (VersionOrder) Code() string { return "DOC-024" }

// Subject возвращает предмет проверки.
func (VersionOrder) Subject() rules.Subject { return rules.SubjectTree }

// RequiredParams сообщает, что правило параметров не требует.
func (VersionOrder) RequiredParams() []string { return nil }

// Check проверяет порядок строк таблицы версий.
func (v VersionOrder) Check(ctx rules.Context) verdict.Result {
	return walk(ctx, func(doc Document) []verdict.Finding {
		var findings []verdict.Finding

		rows := versionRows(doc)
		for index := 1; index < len(rows); index++ {
			if compareVersions(rows[index-1].version, rows[index].version) <= 0 {
				continue
			}
			findings = append(findings, verdict.Finding{
				File: doc.Path,
				Line: rows[index].line,
				Message: fmt.Sprintf("версия %s идёт после %s: порядок убывающий",
					strings.Join(itoa(rows[index].version), "."),
					strings.Join(itoa(rows[index-1].version), ".")),
			})
		}
		return findings
	})
}

// Samples предъявляет образцы самопроверки.
func (VersionOrder) Samples() rules.Samples {
	return rules.Samples{
		Violating: []rules.Sample{
			{Name: "убывание.md", Content: версии("1.0.1", "1.0.0")},
			{Name: "перестановка.md", Content: версии("1.0.0", "1.2.0", "1.1.0")},
		},
		Clean: []rules.Sample{
			{Name: "возрастание.md", Content: версии("1.0.0", "1.0.1", "1.1.0")},
			{Name: "одна.md", Content: версии("1.0.0")},
		},
	}
}

// versionRow — строка таблицы версий: разобранный номер и место в документе.
type versionRow struct {
	version []int
	line    int
}

// versionRows извлекает строки таблицы версий документа.
func versionRows(doc Document) []versionRow {
	var (
		rows    []versionRow
		inTable bool
	)

	for _, line := range doc.Lines {
		if line.InCode {
			continue
		}

		trimmed := strings.TrimSpace(line.Text)
		if !strings.HasPrefix(trimmed, "|") {
			inTable = false
			continue
		}

		cells := splitCells(trimmed)
		if isVersionHeader(cells) {
			inTable = true
			continue
		}
		if !inTable || len(cells) == 0 {
			continue
		}

		version, ok := parseVersion(cells[0])
		if !ok {
			continue
		}
		rows = append(rows, versionRow{version: version, line: line.Number})
	}

	return rows
}

// isVersionHeader отвечает, открывает ли строка таблицу версий.
func isVersionHeader(cells []string) bool {
	if len(cells) < len(versionTableHeader) {
		return false
	}
	for index, expected := range versionTableHeader {
		if !strings.EqualFold(cells[index], expected) {
			return false
		}
	}
	return true
}

// parseVersion разбирает номер версии вида X.Y.Z.
func parseVersion(cell string) ([]int, bool) {
	parts := strings.Split(strings.TrimSpace(cell), ".")
	if len(parts) != 3 {
		return nil, false
	}

	version := make([]int, 0, 3)
	for _, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil {
			return nil, false
		}
		version = append(version, number)
	}
	return version, true
}

// compareVersions сравнивает номера версий поразрядно.
func compareVersions(left, right []int) int {
	for index := range left {
		switch {
		case left[index] < right[index]:
			return -1
		case left[index] > right[index]:
			return 1
		}
	}
	return 0
}

// itoa приводит разряды версии к строкам для сообщения.
func itoa(version []int) []string {
	out := make([]string, 0, len(version))
	for _, number := range version {
		out = append(out, strconv.Itoa(number))
	}
	return out
}

// splitCells разбирает строку таблицы на ячейки.
func splitCells(trimmed string) []string {
	body := strings.TrimSuffix(strings.TrimPrefix(trimmed, "|"), "|")

	cells := strings.Split(body, "|")
	for index, cell := range cells {
		cells[index] = strings.TrimSpace(cell)
	}
	return cells
}

// версии собирает документ с таблицей версий из перечисленных номеров.
func версии(numbers ...string) string {
	var builder strings.Builder
	builder.WriteString("# Документ\n\nНазначение.\n\n## Версионирование\n\n")
	builder.WriteString("| Версия | Дата | Задача | Агент | Модель | Описание изменений |\n")
	builder.WriteString("|--------|------|--------|-------|--------|--------------------|\n")
	for _, number := range numbers {
		builder.WriteString("| " + number + " | 2026-09-22 | Задача | Агент | Модель | Изменение |\n")
	}
	return builder.String()
}
