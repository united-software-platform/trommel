// Пакет docrules реализует правила группы DOC — нормы оформления документов.
//
// Предмет проверки у всех правил группы один: дерево файлов проверяемого проекта,
// из которого берутся документы Markdown. Общий обход вынесен сюда, чтобы каждое
// правило описывало только собственное условие нарушения.
package docrules

import (
	"bufio"
	"io/fs"
	"strings"

	"github.com/united-software-platform/trommel/internal/rules"
	"github.com/united-software-platform/trommel/internal/scope"
	"github.com/united-software-platform/trommel/internal/verdict"
)

// Line — строка документа с признаком принадлежности блоку кода.
type Line struct {
	// Number — номер строки, с единицы.
	Number int
	// Text — содержимое строки как есть.
	Text string
	// InCode — строка принадлежит огороженному блоку кода.
	InCode bool
	// Fence — строка открывает или закрывает блок кода.
	Fence bool
	// Opens — строка открывает блок кода; у закрывающей ложно.
	Opens bool
	// Info — слово после забора открывающей строки: язык блока кода.
	Info string
}

// Document — разобранный документ: путь и строки с разметкой блоков кода.
type Document struct {
	Path  string
	Lines []Line
}

// checker — условие нарушения, применяемое к одному документу.
type checker func(doc Document) []verdict.Finding

// walk обходит документы Markdown дерева и применяет к каждому условие нарушения.
//
// Документ, который не удалось прочитать, попадает в перечень непросмотренного:
// правило, не увидевшее часть содержимого, не вправе утверждать его чистоту.
func walk(ctx rules.Context, check checker) verdict.Result {
	var result verdict.Result

	if ctx.Tree == nil {
		result.Unexamined = append(result.Unexamined, "дерево файлов не передано")
		return result
	}

	err := fs.WalkDir(ctx.Tree, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			result.Unexamined = append(result.Unexamined, path)
			return nil
		}

		// Скрытые каталоги в обход не входят: там лежит служебное содержимое —
		// настройки среды, инструменты агента, история репозитория, — которое
		// проекту не принадлежит и его нормой не регулируется. Прочие исключения
		// задаёт вызывающая сторона и они названы в вердикте.
		if entry.IsDir() {
			if path == "." {
				return nil
			}
			if strings.HasPrefix(entry.Name(), ".") || excluded(path, ctx.ExcludeDirs) {
				return fs.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(path, ".md") {
			return nil
		}

		content, readErr := fs.ReadFile(ctx.Tree, path)
		if readErr != nil {
			result.Unexamined = append(result.Unexamined, path)
			return nil
		}

		result.Examined++
		result.Findings = append(result.Findings, check(parse(path, string(content)))...)
		return nil
	})
	if err != nil {
		result.Unexamined = append(result.Unexamined, "обход дерева прерван: "+err.Error())
	}

	return result
}

// parse размечает строки документа принадлежностью блокам кода.
func parse(path, content string) Document {
	doc := Document{Path: path}

	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	fence := ""
	for number := 1; scanner.Scan(); number++ {
		text := scanner.Text()
		trimmed := strings.TrimSpace(text)

		if marker := fenceOf(trimmed); marker != "" {
			isFence, opens := false, false
			switch {
			case fence == "":
				fence, isFence, opens = marker, true, true
			case strings.HasPrefix(marker, fence):
				fence, isFence = "", true
			}

			info := ""
			if opens {
				info = strings.TrimSpace(strings.TrimLeft(trimmed, "`~"))
			}

			doc.Lines = append(doc.Lines, Line{
				Number: number, Text: text, InCode: true,
				Fence: isFence, Opens: opens, Info: info,
			})
			continue
		}

		doc.Lines = append(doc.Lines, Line{Number: number, Text: text, InCode: fence != ""})
	}

	return doc
}

// fenceOf возвращает забор блока кода, если строка его открывает или закрывает.
func fenceOf(trimmed string) string {
	for _, marker := range []string{"````", "```", "~~~"} {
		if strings.HasPrefix(trimmed, marker) {
			return marker
		}
	}
	return ""
}

// withoutInlineCode убирает из строки содержимое вставок кода: внутри них
// конструкции являются предметом изложения, а не разметкой документа.
func withoutInlineCode(text string) string {
	var out strings.Builder

	rest := text
	for {
		start := strings.IndexByte(rest, '`')
		if start < 0 {
			out.WriteString(rest)
			return out.String()
		}

		out.WriteString(rest[:start])

		ticks := 0
		for ticks < len(rest[start:]) && rest[start+ticks] == '`' {
			ticks++
		}
		closing := strings.Index(rest[start+ticks:], strings.Repeat("`", ticks))
		if closing < 0 {
			return out.String()
		}
		rest = rest[start+ticks+closing+ticks:]
	}
}

// excluded отвечает, выведен ли каталог из обхода вызывающей стороной.
// Определение общее для всех предметов проверки — см. scope.Excluded.
func excluded(dir string, excludeDirs []string) bool {
	return scope.Excluded(dir, excludeDirs)
}
