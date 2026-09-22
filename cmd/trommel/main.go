// Точка входа командной строки Trommel.
//
// Trommel выдаёт вердикт о соблюдении правил нормы и не принимает решений
// о блокировании: код возврата и вердикт предназначены вызывающей стороне,
// которая сама решает, что с ними делать.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

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
		format        = flags.String("format", "text", "форма вердикта: text или json")
		commitMessage = flags.String("commit-message", "", "текст сообщения коммита")
		path          = flags.String("path", "", "путь обращения")
		excludeDirs   каталоги
	)

	flags.Var(&excludeDirs, "exclude-dir",
		"каталог, не подлежащий обходу; можно указать несколько раз")

	if err := flags.Parse(args); err != nil {
		return verdict.ExitRefused
	}

	if *showVersion {
		fmt.Fprintf(stdout, "trommel %s\n", version)
		return verdict.ExitOK
	}

	if *contractName == "" {
		fmt.Fprintf(stderr, "не указан контракт намерения: --contract <имя>\n")
		return verdict.ExitRefused
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintf(stderr, "неизвестная форма вердикта %q: ожидалось text или json\n", *format)
		return verdict.ExitRefused
	}

	report, err := harness.Run(harness.Options{
		Version:       version,
		Project:       os.DirFS(*project),
		ContractFile:  *contractFile,
		ContractName:  *contractName,
		Registry:      registry(),
		CommitMessage: *commitMessage,
		Path:          *path,
		ExcludeDirs:   excludeDirs,
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

// write печатает вердикт в затребованной форме.
func write(report *verdict.Report, format string, stdout io.Writer) error {
	if format == "json" {
		return report.WriteJSON(stdout)
	}
	return report.WriteText(stdout)
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
