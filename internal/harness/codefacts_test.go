package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/united-software-platform/trommel/internal/codefacts"
	"github.com/united-software-platform/trommel/internal/rules"
	"github.com/united-software-platform/trommel/internal/verdict"
)

// правилоКода — реализация, предметом которой служат факты о коде. Нарушением
// считает модуль с искомым импортом: поведение зависит от среза, поэтому по вердикту
// видно, дошёл ли срез до реализации.
type правилоКода struct {
	code   string
	levels []codefacts.Level
	needle string
}

func (п правилоКода) Code() string { return п.code }

func (п правилоКода) Subject() rules.Subject { return rules.SubjectCodeFacts }

func (п правилоКода) RequiredParams() []string { return nil }

func (п правилоКода) Levels() []codefacts.Level { return п.levels }

func (п правилоКода) Describe() string { return "модуль ввозит " + п.needle }

func (п правилоКода) Check(ctx rules.Context) verdict.Result {
	var result verdict.Result
	if ctx.Code == nil {
		result.Unexamined = append(result.Unexamined, "срез фактов о коде не передан")
		return result
	}

	for _, module := range ctx.Code.Modules {
		result.Examined++
		for _, imported := range module.Imports {
			if imported.Module == п.needle {
				result.Findings = append(result.Findings, verdict.Finding{
					File: module.Path, Message: "ввозит " + п.needle,
				})
			}
		}
	}
	for _, record := range ctx.Code.Unexamined {
		result.Unexamined = append(result.Unexamined, record.Where)
	}
	return result
}

// Образцы самопроверки предъявляют тот же предмет, что и прогон, — подставной срез:
// на образце нарушения реализация обязана сработать, на чистом — смолчать.
func (п правилоКода) Samples() rules.Samples {
	срез := func(imported string) *codefacts.Slice {
		return &codefacts.Slice{Modules: []codefacts.Module{{
			Path:    "образец.go",
			Imports: []codefacts.Import{{Module: imported}},
		}}}
	}

	return rules.Samples{
		Violating: []rules.Sample{{Name: "образец.go", Code: срез(п.needle)}},
		Clean:     []rules.Sample{{Name: "образец.go", Code: срез("другое")}},
	}
}

// анализатор собирает подставной анализатор и счётчик его запусков.
func анализатор(t *testing.T) (path, counter string) {
	t.Helper()

	dir := t.TempDir()
	path = filepath.Join(dir, "tldr")
	counter = filepath.Join(dir, "запусков")

	script := `#!/bin/sh
case "$1" in
  --version) echo "tldr 0.4.0" ;;
  structure)
    echo x >> "` + counter + `"
    cat <<'JSON'
{"root": "/project", "language": "go", "files": [
  {"path": "internal/rules/registry.go", "classes": [], "method_infos": [],
   "imports": [{"module": "fmt", "is_from": false}], "definitions": []}
]}
JSON
    ;;
  *) echo "неизвестная команда: $1" >&2; exit 2 ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("подставной анализатор не создан: %v", err)
	}
	return path, counter
}

// прогонСоСрезом выполняет прогон с фазой подготовки.
func прогонСоСрезом(
	t *testing.T, project fstest.MapFS, registry *rules.Registry, analyzer string,
) (*verdict.Report, error) {
	t.Helper()

	return Run(Options{
		Version:         "тест",
		Project:         project,
		ContractFile:    "trommel.yaml",
		ContractName:    "docs",
		Registry:        registry,
		ProjectDir:      t.TempDir(),
		Analyzer:        analyzer,
		AnalyzerWorkDir: t.TempDir(),
	})
}

// реестрКода собирает реестр с одной реализацией правила кода.
func реестрКода(t *testing.T, impl rules.Implementation) *rules.Registry {
	t.Helper()

	registry := rules.NewRegistry()
	if err := registry.Register(impl); err != nil {
		t.Fatalf("реестр не собран: %v", err)
	}
	return registry
}

const контрактСоСрезом = "version: 1\ncontracts:\n  docs:\n    facts: [L1]\n"

// Анализатор запускается только тогда, когда факты кому-то нужны: состав без правил
// кода не платит за подготовку, а объявленный срез виден в вердикте неиспользованным.
func TestПодготовкаНеВыполняетсяБезПравилКода(t *testing.T) {
	report, err := прогонСоСрезом(t,
		проект(контрактСоСрезом, "DOC-005"), rules.NewRegistry(),
		filepath.Join(t.TempDir(), "нет-такого"))
	if err != nil {
		t.Fatalf("прогон не выполнен: %v", err)
	}

	if report.CodeFacts == nil || !report.CodeFacts.Unused {
		t.Fatalf("объявленный и неиспользованный срез не предъявлен: %+v", report.CodeFacts)
	}
	if len(report.CodeFacts.Declared) != 1 || report.CodeFacts.Declared[0] != "L1" {
		t.Errorf("объявленный состав среза: %+v", report.CodeFacts.Declared)
	}
}

// Подготовка выполняется однократно на прогон и доносит срез до реализации.
func TestПодготовкаСобираетСрезИДоноситЕгоДоПравила(t *testing.T) {
	analyzer, counter := анализатор(t)
	registry := реестрКода(t, правилоКода{
		code: "CLAR-001", levels: []codefacts.Level{codefacts.LevelAST}, needle: "fmt",
	})

	report, err := прогонСоСрезом(t, проект(контрактСоСрезом, "CLAR-001"), registry, analyzer)
	if err != nil {
		t.Fatalf("прогон не выполнен: %v", err)
	}

	if report.CodeFacts == nil {
		t.Fatalf("сведения о срезе в вердикт не попали")
	}
	switch {
	case report.CodeFacts.Version != "0.4.0":
		t.Errorf("версия анализатора в вердикте: %q", report.CodeFacts.Version)
	case len(report.CodeFacts.Collected) != 1:
		t.Errorf("собранные уровни: %+v", report.CodeFacts.Collected)
	case report.Rules[0].Status != verdict.StatusFail:
		t.Errorf("состояние правила: %q (%s), ожидалось fail; граница среза: %+v",
			report.Rules[0].Status, report.Rules[0].Detail, report.CodeFacts.Unexamined)
	}

	// Счётчик запусков: подготовка обращается к анализатору один раз, а не на каждое
	// правило и не на каждую функцию проекта.
	runs, err := os.ReadFile(counter)
	if err != nil {
		t.Fatalf("счётчик запусков не прочитан: %v", err)
	}
	if got := strings.Count(string(runs), "x"); got != 1 {
		t.Errorf("запусков анализатора: %d, ожидался 1", got)
	}
}

// Контракт, не объявивший состав среза, при наличии правил кода исполнен быть не может.
func TestОтказПриНеобъявленномСрезе(t *testing.T) {
	analyzer, _ := анализатор(t)
	registry := реестрКода(t, правилоКода{
		code: "CLAR-001", levels: []codefacts.Level{codefacts.LevelAST},
	})

	_, err := прогонСоСрезом(t, проект(простойКонтракт, "CLAR-001"), registry, analyzer)
	if err == nil {
		t.Fatalf("прогон выполнен на необъявленном срезе")
	}
	if !strings.Contains(err.Error(), "CLAR-001") ||
		!strings.Contains(err.Error(), "состава среза не объявляет") {
		t.Errorf("отказ не называет правило и причину: %v", err)
	}
}

// Уровень, нужный правилу и не объявленный контрактом, — ошибка контракта.
func TestОтказПриНедостающемУровне(t *testing.T) {
	analyzer, _ := анализатор(t)
	registry := реестрКода(t, правилоКода{
		code: "CLAR-001", levels: []codefacts.Level{codefacts.LevelCalls},
	})

	_, err := прогонСоСрезом(t, проект(контрактСоСрезом, "CLAR-001"), registry, analyzer)
	if err == nil {
		t.Fatalf("прогон выполнен без требуемого уровня")
	}
	if !strings.Contains(err.Error(), "L2") {
		t.Errorf("отказ не называет недостающий уровень: %v", err)
	}
}

// Уровень функций без уровня структуры отвергается до прогона — даже когда правил
// кода в составе нет: контракт обязан быть исполним целиком.
func TestОтказПриУровнеФункцийБезСтруктуры(t *testing.T) {
	contract := "version: 1\ncontracts:\n  docs:\n    facts: [L5]\n"

	_, err := прогонСоСрезом(t, проект(contract, "DOC-005"), rules.NewRegistry(), "tldr")
	if err == nil {
		t.Fatalf("контракт с уровнем функций без уровня структуры принят")
	}
	if !strings.Contains(err.Error(), "L5") || !strings.Contains(err.Error(), "L1") {
		t.Errorf("отказ не называет оба уровня: %v", err)
	}
}

// Несостоявшаяся подготовка — отказ прогона, а не ok на пустом срезе.
func TestОтказПриНесостоявшейсяПодготовке(t *testing.T) {
	registry := реестрКода(t, правилоКода{
		code: "CLAR-001", levels: []codefacts.Level{codefacts.LevelAST},
	})

	report, err := прогонСоСрезом(t, проект(контрактСоСрезом, "CLAR-001"), registry,
		filepath.Join(t.TempDir(), "нет-такого"))
	if err == nil {
		t.Fatalf("прогон выполнен без анализатора")
	}
	if report != nil {
		t.Errorf("при несостоявшейся подготовке выдан вердикт: %+v", report)
	}
	if !strings.Contains(err.Error(), "подготовка среза фактов о коде не состоялась") {
		t.Errorf("отказ не называет фазу: %v", err)
	}
}

// Содержимое, о котором анализатор фактов не дал, лишает правило права на ok:
// граница среза доходит до состояния правила, а не остаётся сноской в вердикте.
func TestНепросмотренноеЛишаетПравилоСостоянияOK(t *testing.T) {
	analyzer, _ := анализатор(t)
	registry := реестрКода(t, правилоКода{
		code: "CLAR-001", levels: []codefacts.Level{codefacts.LevelAST}, needle: "нет-такого",
	})

	project := проект(контрактСоСрезом, "CLAR-001")
	project["internal/lost.go"] = &fstest.MapFile{Data: []byte("package lost")}

	report, err := прогонСоСрезом(t, project, registry, analyzer)
	if err != nil {
		t.Fatalf("прогон не выполнен: %v", err)
	}

	if report.Rules[0].Status != verdict.StatusErr {
		t.Errorf("состояние правила: %q, ожидалось err — часть содержимого без фактов",
			report.Rules[0].Status)
	}
	if len(report.CodeFacts.Unexamined) == 0 {
		t.Fatalf("граница среза пуста, а неразобранный файл в дереве есть")
	}
	if !strings.Contains(report.CodeFacts.Unexamined[0].Where, "internal/lost.go") {
		t.Errorf("граница среза не называет неразобранный файл: %+v", report.CodeFacts.Unexamined)
	}
}
