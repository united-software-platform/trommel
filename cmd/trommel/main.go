// Точка входа командной строки Trommel.
//
// Trommel выдаёт вердикт о соблюдении правил нормы и не принимает решений
// о блокировании: код возврата и вердикт предназначены вызывающей стороне,
// которая сама решает, что с ними делать.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/united-software-platform/trommel/internal/codefacts"
	"github.com/united-software-platform/trommel/internal/docrules"
	"github.com/united-software-platform/trommel/internal/harness"
	"github.com/united-software-platform/trommel/internal/rules"
	"github.com/united-software-platform/trommel/internal/verdict"
)

// version — версия сборки. Значение по умолчанию действует при сборке из рабочего
// дерева; при выпуске подставляется компоновщиком через -ldflags.
var version = "0.0.0-dev"

// registry — реестр реализаций правил этой сборки. Правило, которого здесь нет,
// остаётся в составе прогона и предъявляется состоянием `--`: Trommel не умалчивает
// о том, чего не умеет.
func registry() *rules.Registry {
	registry := rules.NewRegistry()

	for _, implementation := range []rules.Implementation{
		docrules.HTML{},
		docrules.Wikilink{},
		docrules.IndentedCode{},
		docrules.SetextHeading{},
		docrules.CodeLanguage{},
		docrules.HeadingLevels{},
		docrules.FileOpening{},
		docrules.RelativeLinks{},
		docrules.VersionTable{},
		docrules.VersionOrder{},
	} {
		if err := registry.Register(implementation); err != nil {
			panic(fmt.Sprintf("реестр реализаций собран неверно: %v", err))
		}
	}

	return registry
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run исполняет команду и возвращает код возврата. Вынесен из main и принимает
// потоки вывода аргументами, чтобы поведение командной строки проверялось тестом,
// а не только запуском процесса.
func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("trommel", flag.ContinueOnError)
	flags.SetOutput(stderr)

	var (
		showVersion   = flags.Bool("version", false, "напечатать версию и завершиться")
		project       = flags.String("project", ".", "каталог проверяемого проекта")
		contractFile  = flags.String("contract-file", "trommel.yaml", "файл контрактов намерения")
		contractName  = flags.String("contract", "", "имя контракта намерения")
		format        = flags.String("format", "text", "форма вердикта: text, json или markdown")
		commitMessage = flags.String("commit-message", "", "текст сообщения коммита")
		path          = flags.String("path", "", "путь обращения")
		analyzer      = flags.String("analyzer", codefacts.Executable,
			"имя или путь внешнего анализатора кода")
		facts = flags.Bool("facts", false,
			"снять срез фактов о коде и напечатать его, правила не исполнять")
		levels      каталоги
		excludeDirs каталоги
		scanDirs    каталоги
	)

	flags.Var(&excludeDirs, "exclude-dir",
		"каталог, не подлежащий обходу; можно указать несколько раз")
	flags.Var(&levels, "level",
		"уровень съёма фактов: L1, L2, L3, L4, L5, deps; можно указать несколько раз; "+
			"без указания снимаются все уровни")
	flags.Var(&scanDirs, "scan",
		"сканируемый путь карты проекта; можно указать несколько раз; "+
			"без указания сканируется проект целиком")

	if err := flags.Parse(args); err != nil {
		return verdict.ExitRefused
	}

	if *showVersion {
		fmt.Fprintf(stdout, "trommel %s\n", version)
		return verdict.ExitOK
	}

	if *facts {
		return collect(harness.Facts{
			Project:         os.DirFS(*project),
			ProjectDir:      *project,
			Analyzer:        *analyzer,
			AnalyzerWorkDir: os.TempDir(),
			Levels:          levels,
			ScanDirs:        scanDirs,
			ExcludeDirs:     excludeDirs,
		}, *format, stdout, stderr)
	}

	if *contractName == "" {
		fmt.Fprintf(stderr, "не указан контракт намерения: --contract <имя>\n")
		return verdict.ExitRefused
	}
	switch *format {
	case "text", "json", "markdown":
	default:
		fmt.Fprintf(stderr,
			"неизвестная форма вердикта %q: ожидалось text, json или markdown\n", *format)
		return verdict.ExitRefused
	}

	report, err := harness.Run(harness.Options{
		Version:    version,
		Project:    os.DirFS(*project),
		ProjectDir: *project,
		Analyzer:   *analyzer,
		// Рабочий каталог анализатора — временный каталог машины, а не проверяемый
		// проект: проект остаётся неизменным, чего бы инструмент ни захотел записать.
		AnalyzerWorkDir: os.TempDir(),
		ContractFile:    *contractFile,
		ContractName:    *contractName,
		Registry:        registry(),
		CommitMessage:   *commitMessage,
		Path:            *path,
		ExcludeDirs:     excludeDirs,
	})
	if err != nil {
		fmt.Fprintf(stderr, "прогон не выполнялся: %v\n", err)
		return verdict.ExitRefused
	}

	if err := write(report, *format, stdout); err != nil {
		fmt.Fprintf(stderr, "вердикт не напечатан: %v\n", err)
		return verdict.ExitRefused
	}

	return report.ExitCode()
}

// collect снимает срез фактов о коде и печатает его.
//
// Код возврата различает то же, что и прогон: съём состоялся или не состоялся.
// Нарушений съём не ищет, поэтому исхода «нарушения есть» у него нет.
func collect(facts harness.Facts, format string, stdout, stderr io.Writer) int {
	slice, err := harness.Collect(facts)
	if err != nil {
		fmt.Fprintf(stderr, "срез не снят: %v\n", err)
		return verdict.ExitRefused
	}

	if format == "json" {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(slice); err != nil {
			fmt.Fprintf(stderr, "срез не напечатан: %v\n", err)
			return verdict.ExitRefused
		}
		return verdict.ExitOK
	}

	summary(slice, stdout)
	return verdict.ExitOK
}

// summary печатает срез для человека: состав, объём и граница.
func summary(slice *codefacts.Slice, out io.Writer) {
	fmt.Fprintf(out, "анализатор: %s %s\n", slice.Analyzer.Name, slice.Analyzer.Version)
	fmt.Fprintf(out, "язык:       %s\n", slice.Language)
	if len(slice.Scanned) > 0 {
		fmt.Fprintf(out, "карта:      сканируется %s\n", strings.Join(slice.Scanned, ", "))
	}
	if len(slice.Excluded) > 0 {
		fmt.Fprintf(out, "вне обхода: %s\n", strings.Join(slice.Excluded, ", "))
	}

	levels := make([]string, 0, len(slice.Collected))
	for _, level := range slice.Collected {
		levels = append(levels, string(level))
	}
	fmt.Fprintf(out, "уровни:     %s\n\n", strings.Join(levels, ", "))

	fmt.Fprintf(out, "модулей:      %d\n", len(slice.Modules))
	fmt.Fprintf(out, "вызовов:      %d\n", len(slice.Calls))
	fmt.Fprintf(out, "недостижимо:  %d\n", len(slice.Unreachable))
	fmt.Fprintf(out, "зависимостей: %d, циклов %d\n", len(slice.Dependencies), len(slice.Cycles))
	fmt.Fprintf(out, "функций:      %d\n", len(slice.Functions))

	fmt.Fprintf(out, "\nбез фактов:   %d\n", len(slice.Unexamined))
	for i, record := range slice.Unexamined {
		if i >= 10 {
			fmt.Fprintf(out, "  … ещё %d\n", len(slice.Unexamined)-i)
			break
		}
		fmt.Fprintf(out, "  %s — %s\n", record.Where, record.Reason)
	}
}

// write печатает вердикт в затребованной форме.
func write(report *verdict.Report, format string, stdout io.Writer) error {
	switch format {
	case "json":
		return report.WriteJSON(stdout)
	case "markdown":
		return report.WriteMarkdown(stdout)
	default:
		return report.WriteText(stdout)
	}
}

// каталоги — повторяемый флаг командной строки: каждое указание добавляет каталог
// к исключаемым, а не заменяет прежние.
type каталоги []string

// String даёт представление значения флага.
func (к *каталоги) String() string { return strings.Join(*к, ", ") }

// Set добавляет каталог к исключаемым.
func (к *каталоги) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("пустое имя каталога")
	}
	*к = append(*к, value)
	return nil
}
