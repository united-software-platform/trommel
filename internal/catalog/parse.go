package catalog

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Заголовки таблицы правил. Документ нормы содержит и другие таблицы — версий,
// принципов, соответствий, — поэтому таблица правил опознаётся по точному составу
// заголовков, а не по виду содержимого.
var ruleTableHeader = []string{"ID", "Правило"}

// parseDocument извлекает правила из одного документа нормы.
//
// Содержимое огороженных блоков кода пропускается: документы нормы приводят примеры
// разметки, и таблица внутри примера не объявляет правило.
func parseDocument(source string, reader io.Reader) ([]Rule, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var (
		rules   []Rule
		fence   string // открытый забор блока кода, пусто вне блока
		inTable bool
		lineNo  int
	)

	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if marker := fenceMarker(trimmed); marker != "" {
			switch {
			case fence == "":
				fence = marker
			case strings.HasPrefix(marker, fence):
				fence = ""
			}
			inTable = false
			continue
		}
		if fence != "" {
			continue
		}

		if !strings.HasPrefix(trimmed, "|") {
			inTable = false
			continue
		}

		cells := splitRow(trimmed)

		if isRuleTableHeader(cells) {
			inTable = true
			continue
		}
		if !inTable || isDelimiterRow(cells) {
			continue
		}

		rule, err := ruleFromRow(source, lineNo, cells)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%s: чтение прервано: %w", source, err)
	}
	return rules, nil
}

// ruleFromRow превращает строку таблицы правил в правило.
func ruleFromRow(source string, lineNo int, cells []string) (Rule, error) {
	if len(cells) < 2 {
		return Rule{}, fmt.Errorf(
			"%s:%d: в строке таблицы правил меньше двух ячеек", source, lineNo)
	}

	group, err := parseCode(cells[0])
	if err != nil {
		return Rule{}, fmt.Errorf("%s:%d: %w", source, lineNo, err)
	}

	text := strings.TrimSpace(cells[1])
	if text == "" {
		return Rule{}, fmt.Errorf(
			"%s:%d: у правила %s пустая формулировка", source, lineNo, cells[0])
	}

	return Rule{Code: cells[0], Group: group, Text: text, Source: source, Line: lineNo}, nil
}

// isRuleTableHeader отвечает, открывает ли строка таблицу правил.
func isRuleTableHeader(cells []string) bool {
	if len(cells) != len(ruleTableHeader) {
		return false
	}
	for i, expected := range ruleTableHeader {
		if !strings.EqualFold(cells[i], expected) {
			return false
		}
	}
	return true
}

// isDelimiterRow отвечает, является ли строка разделителем заголовка таблицы.
func isDelimiterRow(cells []string) bool {
	for _, cell := range cells {
		if strings.Trim(cell, "-: ") != "" {
			return false
		}
	}
	return len(cells) > 0
}

// fenceMarker возвращает забор блока кода, если строка его открывает или закрывает.
func fenceMarker(trimmed string) string {
	for _, marker := range []string{"````", "```", "~~~"} {
		if strings.HasPrefix(trimmed, marker) {
			return marker
		}
	}
	return ""
}
