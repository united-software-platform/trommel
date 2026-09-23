package codefacts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/united-software-platform/trommel/internal/scope"
)

// Образцы ответов анализатора взяты с прогона выпуска 0.4.0 по дереву Trommel
// и урезаны до значимых полей: тест обязан ломаться от смены формы вывода,
// а не от объёма образца.

const structureAnswer = `{
  "root": "/project",
  "language": "go",
  "files": [
    {
      "path": "internal/rules/registry.go",
      "classes": ["Registry"],
      "method_infos": [
        {"name": "Register", "signature": "func (r *Registry) Register(impl Implementation) error {", "line": 83, "line_end": 90}
      ],
      "imports": [{"module": "fmt", "is_from": false}],
      "definitions": [{"name": "Subject", "kind": "class", "line_start": 23, "line_end": 23, "signature": "Subject string"}]
    }
  ]
}`

const callsAnswer = `{
  "root": "/project",
  "language": "go",
  "nodes": ["cmd/trommel/main.go:main"],
  "edges": [
    {"src_file": "cmd/trommel/main.go", "src_func": "main", "dst_file": "cmd/trommel/main.go", "dst_func": "run", "call_type": "intra"}
  ],
  "truncated": false,
  "total_edges": 1,
  "shown_edges": 1
}`

const deadAnswer = `{
  "dead_functions": [{"file": "internal/verdict/report.go", "name": "unused", "line": 12}],
  "possibly_dead": [{"file": "internal/catalog/catalog.go", "name": "LoadDir", "line": 74}],
  "by_file": {},
  "total_dead": 1,
  "total_possibly_dead": 1,
  "functions_analyzed": 204,
  "total_functions": 204
}`

const depsAnswer = `{
  "root": "/project",
  "language": "go",
  "internal_dependencies": {"cmd/trommel/main.go": ["internal/harness/harness.go"]},
  "circular_dependencies": [{"path": ["internal/catalog/rule.go", "internal/catalog/catalog.go"], "length": 2}],
  "stats": {"total_files": 27},
  "files_skipped": 0
}`

func TestРазборСтруктурыДаётМодули(t *testing.T) {
	var slice Slice
	if err := parseStructure([]byte(structureAnswer), &slice); err != nil {
		t.Fatalf("разбор не удался: %v", err)
	}

	if len(slice.Modules) != 1 {
		t.Fatalf("модулей: %d, ожидался 1", len(slice.Modules))
	}
	module := slice.Modules[0]
	switch {
	case module.Path != "internal/rules/registry.go":
		t.Errorf("путь модуля: %q", module.Path)
	case len(module.Functions) != 1 || module.Functions[0].Name != "Register":
		t.Errorf("функции модуля: %+v", module.Functions)
	case len(module.Imports) != 1 || module.Imports[0].Module != "fmt":
		t.Errorf("импорты модуля: %+v", module.Imports)
	case len(module.Types) != 1 || module.Types[0] != "Registry":
		t.Errorf("типы модуля: %+v", module.Types)
	case len(module.Definitions) != 1 || module.Definitions[0].Kind != "class":
		t.Errorf("объявления модуля: %+v", module.Definitions)
	case slice.Language != "go":
		t.Errorf("язык среза: %q", slice.Language)
	}
}

func TestРазборОтказываетНаЧужойФорме(t *testing.T) {
	cases := map[string]struct {
		parse  func([]byte, *Slice) error
		answer string
	}{
		"structure без files":    {parseStructure, `{"root": "/project", "language": "go"}`},
		"calls без truncated":    {parseCalls, `{"edges": [], "total_edges": 0, "shown_edges": 0}`},
		"dead без total":         {parseDead, `{"dead_functions": [], "possibly_dead": []}`},
		"deps без files_skipped": {parseDeps, `{"internal_dependencies": {}}`},
		"не JSON":                {parseStructure, `<html>`},
		"усечённый JSON":         {parseCalls, `{"edges": [`},
	}

	for name, sample := range cases {
		t.Run(name, func(t *testing.T) {
			var slice Slice
			err := sample.parse([]byte(sample.answer), &slice)
			if err == nil {
				t.Fatalf("разбор принял чужую форму вывода и отказа не дал")
			}
			if len(slice.Modules)+len(slice.Calls)+len(slice.Unreachable) > 0 {
				t.Errorf("при отказе в срез попали факты: %+v", slice)
			}
		})
	}
}

func TestУсечениеВыводаСтановитсяГраницейСреза(t *testing.T) {
	answer := strings.NewReplacer(
		`"truncated": false`, `"truncated": true`,
		`"total_edges": 1`, `"total_edges": 191`,
		`"shown_edges": 1`, `"shown_edges": 5`,
	).Replace(callsAnswer)

	var slice Slice
	if err := parseCalls([]byte(answer), &slice); err != nil {
		t.Fatalf("разбор не удался: %v", err)
	}

	if len(slice.Unexamined) != 1 {
		t.Fatalf("граница среза: %+v, ожидалась одна запись", slice.Unexamined)
	}
	if !strings.Contains(slice.Unexamined[0].Reason, "5") ||
		!strings.Contains(slice.Unexamined[0].Reason, "191") {
		t.Errorf("причина не называет объём усечения: %q", slice.Unexamined[0].Reason)
	}
}

func TestНедостижимыйКодРазличаетУверенность(t *testing.T) {
	var slice Slice
	if err := parseDead([]byte(deadAnswer), &slice); err != nil {
		t.Fatalf("разбор не удался: %v", err)
	}

	if len(slice.Unreachable) != 2 {
		t.Fatalf("недостижимых функций: %d, ожидалось 2", len(slice.Unreachable))
	}
	if !slice.Unreachable[0].Certain || slice.Unreachable[1].Certain {
		t.Errorf("уверенность не различена: %+v", slice.Unreachable)
	}
}

func TestЗависимостиИЦиклыПереносятсяВСрез(t *testing.T) {
	var slice Slice
	if err := parseDeps([]byte(depsAnswer), &slice); err != nil {
		t.Fatalf("разбор не удался: %v", err)
	}

	if len(slice.Dependencies) != 1 || slice.Dependencies[0].To != "internal/harness/harness.go" {
		t.Errorf("зависимости: %+v", slice.Dependencies)
	}
	if len(slice.Cycles) != 1 || len(slice.Cycles[0].Path) != 2 {
		t.Errorf("циклы: %+v", slice.Cycles)
	}
}

func TestПропущенныеФайлыПопадаютВГраницу(t *testing.T) {
	var slice Slice
	answer := strings.Replace(depsAnswer, `"files_skipped": 0`, `"files_skipped": 3`, 1)
	if err := parseDeps([]byte(answer), &slice); err != nil {
		t.Fatalf("разбор не удался: %v", err)
	}

	if len(slice.Unexamined) != 1 || !strings.Contains(slice.Unexamined[0].Reason, "3") {
		t.Errorf("граница среза: %+v", slice.Unexamined)
	}
}

func TestИсключённыйКаталогИзСрезаУбран(t *testing.T) {
	slice := Slice{
		Modules: []Module{{Path: "internal/rules/registry.go"}, {Path: "vendor/foreign/lib.go"}},
		Calls: []Call{
			{FromFile: "internal/rules/registry.go", ToFile: "internal/rules/samples.go"},
			{FromFile: "internal/rules/registry.go", ToFile: "vendor/foreign/lib.go"},
		},
		Dependencies: []Dependency{{From: "vendor/foreign/lib.go", To: "internal/rules/registry.go"}},
		Cycles:       []Cycle{{Path: []string{"vendor/foreign/lib.go", "internal/rules/registry.go"}}},
	}

	restrict(&slice, scope.Map{Exclude: []string{"vendor"}})

	switch {
	case len(slice.Modules) != 1 || slice.Modules[0].Path != "internal/rules/registry.go":
		t.Errorf("модули: %+v", slice.Modules)
	case len(slice.Calls) != 1:
		t.Errorf("вызовы: %+v", slice.Calls)
	case slice.Dependencies != nil:
		t.Errorf("зависимости: %+v", slice.Dependencies)
	case slice.Cycles != nil:
		t.Errorf("циклы: %+v", slice.Cycles)
	}
}

func TestНеразобранныеФайлыНазваныВГранице(t *testing.T) {
	tree := fstest.MapFS{
		"internal/rules/registry.go": {Data: []byte("package rules")},
		"internal/rules/samples.go":  {Data: []byte("package rules")},
		"tools/deploy.sh":            {Data: []byte("#!/bin/sh")},
		"docs/guide.md":              {Data: []byte("# Документ")},
		"vendor/foreign/lib.go":      {Data: []byte("package foreign")},
	}
	slice := Slice{Modules: []Module{{Path: "internal/rules/registry.go"}}}

	note(&slice, tree, scope.Map{Exclude: []string{"vendor"}})

	reasons := map[string]string{}
	for _, record := range slice.Unexamined {
		reasons[record.Where] = record.Reason
	}

	if _, named := reasons["internal/rules/samples.go"]; !named {
		t.Errorf("неразобранный файл Go не назван: %+v", slice.Unexamined)
	}
	if reason, named := reasons["tools/deploy.sh"]; !named ||
		!strings.Contains(reason, "не поддержан") {
		t.Errorf("файл неподдержанного языка не назван: %+v", slice.Unexamined)
	}
	if _, named := reasons["docs/guide.md"]; named {
		t.Errorf("документ назван непросмотренным исходным кодом: %+v", slice.Unexamined)
	}
	if _, named := reasons["vendor/foreign/lib.go"]; named {
		t.Errorf("исключённый каталог попал в границу среза: %+v", slice.Unexamined)
	}
}

func TestНеизвестныйУровеньОтвергнут(t *testing.T) {
	for _, level := range Levels() {
		if err := Validate(level); err != nil {
			t.Errorf("известный уровень %s отвергнут: %v", level, err)
		}
	}

	err := Validate(Level("L9"))
	if err == nil || !strings.Contains(err.Error(), "неизвестный") {
		t.Errorf("неизвестный уровень принят либо отказ невнятен: %v", err)
	}
}

// Перечень функций даёт уровень структуры: уровень функций без него неисполним,
// и это ошибка контракта, обнаруживаемая до запуска анализатора.
func TestУровеньФункцийБезУровняСтруктурыОтвергнут(t *testing.T) {
	for _, level := range []Level{LevelCFG, LevelDFG, LevelPDG} {
		err := CheckLevels([]Level{level})
		if err == nil {
			t.Fatalf("уровень %s принят без уровня %s", level, LevelAST)
		}
		if !strings.Contains(err.Error(), string(LevelAST)) {
			t.Errorf("отказ не называет недостающий уровень: %v", err)
		}
	}

	if err := CheckLevels([]Level{LevelAST, LevelCFG, LevelDFG, LevelPDG}); err != nil {
		t.Errorf("полный состав уровней отвергнут: %v", err)
	}
}

func TestВерсияВнеДиапазонаОтвергнута(t *testing.T) {
	if err := CheckVersion("0.4.0"); err != nil {
		t.Fatalf("поддержанная версия отвергнута: %v", err)
	}
	for _, version := range []string{"0.5.0", "1.0.0", "0.2.4", "неизвестно"} {
		if err := CheckVersion(version); err == nil {
			t.Errorf("версия %s принята", version)
		}
	}
}

// analyzerScript собирает подставной анализатор: тесты прогоняются там, где
// настоящего инструмента нет, а проверять надо запуск и перенос отказа.
func analyzerScript(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "tldr")
	script := "#!/bin/sh\n" + body
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("подставной анализатор не создан: %v", err)
	}
	return path
}

func TestПодготовкаСобираетСрез(t *testing.T) {
	analyzer := analyzerScript(t, `
case "$1" in
  --version) echo "tldr 0.4.0" ;;
  structure) cat <<'JSON'
`+structureAnswer+`
JSON
  ;;
  calls) cat <<'JSON'
`+callsAnswer+`
JSON
  ;;
  dead) cat <<'JSON'
`+deadAnswer+`
JSON
  ;;
  *) echo "неизвестная команда: $1" >&2; exit 2 ;;
esac
`)

	slice, err := Prepare(Options{
		Executable: analyzer,
		ProjectDir: t.TempDir(),
		WorkDir:    t.TempDir(),
		Tree:       fstest.MapFS{"internal/rules/registry.go": {Data: []byte("package rules")}},
		Levels:     []Level{LevelAST, LevelCalls},
	})
	if err != nil {
		t.Fatalf("подготовка не состоялась: %v", err)
	}

	switch {
	case slice.Analyzer.Version != "0.4.0":
		t.Errorf("версия анализатора в срезе: %q", slice.Analyzer.Version)
	case len(slice.Collected) != 2:
		t.Errorf("собранные уровни: %+v", slice.Collected)
	case len(slice.Modules) != 1:
		t.Errorf("модули: %+v", slice.Modules)
	case len(slice.Calls) != 1:
		t.Errorf("вызовы: %+v", slice.Calls)
	case len(slice.Unexamined) != 0:
		t.Errorf("граница среза непуста: %+v", slice.Unexamined)
	}
}

func TestПодготовкаОтказываетПриНеуспехеАнализатора(t *testing.T) {
	cases := map[string]struct {
		body string
		part string
	}{
		"ненулевой код возврата": {
			body: `case "$1" in
  --version) echo "tldr 0.4.0" ;;
  *) echo "Error: Path not found" >&2; exit 1 ;;
esac
`,
			part: "код возврата 1",
		},
		"версия вне диапазона": {
			body: "echo \"tldr 0.9.1\"\n",
			part: "вне поддерживаемого сборкой диапазона",
		},
		"вывод не разобран": {
			body: `case "$1" in
  --version) echo "tldr 0.4.0" ;;
  *) echo "не json" ;;
esac
`,
			part: "не разобран",
		},
	}

	for name, sample := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Prepare(Options{
				Executable: analyzerScript(t, sample.body),
				ProjectDir: t.TempDir(),
				WorkDir:    t.TempDir(),
				Levels:     []Level{LevelAST},
			})
			if err == nil {
				t.Fatalf("подготовка не отказала")
			}
			if !strings.Contains(err.Error(), sample.part) {
				t.Errorf("отказ не называет причину %q: %v", sample.part, err)
			}
		})
	}
}

func TestПодготовкаОтказываетБезАнализатора(t *testing.T) {
	_, err := Prepare(Options{
		Executable: filepath.Join(t.TempDir(), "нет-такого"),
		ProjectDir: t.TempDir(),
		WorkDir:    t.TempDir(),
		Levels:     []Level{LevelAST},
	})
	if err == nil {
		t.Fatalf("подготовка не отказала при отсутствии анализатора")
	}
	if !strings.Contains(err.Error(), "анализатор кода недоступен") {
		t.Errorf("отказ не называет причину: %v", err)
	}
}

// Образцы ответов о теле функции сняты с выпуска 0.4.0: L3 и L5 — с кода Trommel,
// L4 — с функции PHP-проекта, где мёртвые присваивания нашлись.

const reachingAnswer = `{
  "function": "Run", "file": "/project/internal/harness/harness.go",
  "blocks": [
    {"id": 0, "lines": [58, 59],
     "gen": [{"var": "err", "line": 59, "column": 7, "block": 0},
             {"var": "file", "line": 59, "column": 1, "block": 0}],
     "kill": [{"var": "err", "line": 64, "column": 11, "block": 2}]}
  ]
}`

const availableAnswer = `{
  "avail_in": {
    "12": [{"text": "err != nil", "operands": ["err", "nil"], "line": 70}],
    "22": [{"text": "err != nil", "operands": ["err", "nil"], "line": 70}]
  }
}`

const deadStoresAnswer = `{
  "function": "execute", "file": "/project/lib/Application/FindByFilterUseCase.php",
  "dead_stores_ssa": [
    {"variable": "$fuzzi", "ssa_name": "$fuzzi_1", "line": 29, "block_id": 0, "is_phi": false},
    {"variable": "$word", "ssa_name": "$word_1", "line": 36, "block_id": 0, "is_phi": false}
  ],
  "count": 2, "dead_stores_live_vars": null, "live_vars_count": null
}`

const sliceAnswer = `{
  "file": "/project/internal/harness/harness.go", "function": "Run",
  "criterion_line": 58, "direction": "backward", "variable": null, "lines": [58, 59],
  "slice_lines": [
    {"line": 58, "code": "func Run(options Options) (*verdict.Report, error) {",
     "definitions": ["options"], "uses": ["intent"], "dep_type": "data", "dep_label": "options"}
  ]
}`

func TestРазборФактовОТелеФункции(t *testing.T) {
	var facts FunctionFacts

	for name, разбор := range map[string]func() error{
		"reaching-defs": func() error { return parseReaching([]byte(reachingAnswer), &facts) },
		"available":     func() error { return parseAvailable([]byte(availableAnswer), &facts) },
		"dead-stores":   func() error { return parseDeadStores([]byte(deadStoresAnswer), &facts) },
		"slice":         func() error { return parseSlice([]byte(sliceAnswer), &facts) },
	} {
		if err := разбор(); err != nil {
			t.Fatalf("разбор ответа %s не удался: %v", name, err)
		}
	}

	switch {
	case len(facts.Reaching) != 2 || facts.Reaching[0].Var != "err":
		t.Errorf("достигающие определения: %+v", facts.Reaching)
	case len(facts.Available) != 1 || facts.Available[0].Text != "err != nil":
		t.Errorf("доступные выражения: %+v", facts.Available)
	case len(facts.DeadStores) != 2 || facts.DeadStores[0].Var != "$fuzzi":
		t.Errorf("мёртвые присваивания: %+v", facts.DeadStores)
	case len(facts.Slice) != 1 || facts.Slice[0].Dep != "data":
		t.Errorf("срез программы: %+v", facts.Slice)
	}
}

func TestРазборФактовФункцииОтказываетНаЧужойФорме(t *testing.T) {
	cases := map[string]struct {
		parse  func([]byte, *FunctionFacts) error
		answer string
	}{
		"reaching-defs без blocks":   {parseReaching, `{"function": "Run"}`},
		"available без avail_in":     {parseAvailable, `{"function": "Run"}`},
		"dead-stores без списка":     {parseDeadStores, `{"count": 0}`},
		"slice без slice_lines":      {parseSlice, `{"criterion_line": 58}`},
		"вместо ответа текст ошибки": {parseSlice, `Error: Path not found`},
	}

	for name, sample := range cases {
		t.Run(name, func(t *testing.T) {
			var facts FunctionFacts
			if err := sample.parse([]byte(sample.answer), &facts); err == nil {
				t.Fatalf("разбор принял чужую форму вывода")
			}
		})
	}
}

// многоуровневый собирает подставной анализатор, отвечающий на все команды съёма,
// и журнал его обращений: по журналу видно, сколько раз и с какой целью его звали.
func многоуровневый(t *testing.T) (path, log string) {
	t.Helper()

	dir := t.TempDir()
	path = filepath.Join(dir, "tldr")
	log = filepath.Join(dir, "обращения")

	script := `#!/bin/sh
echo "$1 $2" >> ` + log + `
case "$1" in
  --version) echo "tldr 0.4.0" ;;
  structure)
    case "$2" in
      */lib) echo '{"root":"/p/lib","language":"php","files":[{"path":"UseCase.php","classes":[],"method_infos":[{"name":"execute","signature":"function execute()","line":29,"line_end":40}],"imports":[],"definitions":[]}]}' ;;
      */www) echo '{"root":"/p/www","language":"php","files":[{"path":"index.php","classes":[],"method_infos":[],"imports":[],"definitions":[]}]}' ;;
      *)     echo '{"root":"/p","language":"php","files":[]}' ;;
    esac ;;
  reaching-defs) cat <<'JSON'
` + reachingAnswer + `
JSON
  ;;
  available) cat <<'JSON'
` + availableAnswer + `
JSON
  ;;
  dead-stores) cat <<'JSON'
` + deadStoresAnswer + `
JSON
  ;;
  slice) cat <<'JSON'
` + sliceAnswer + `
JSON
  ;;
  *) echo "неизвестная команда: $1" >&2; exit 2 ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("подставной анализатор не создан: %v", err)
	}
	return path, log
}

// Карта проекта задаёт и число обращений к анализатору, и содержание среза:
// анализатор зовут по каждому сканируемому пути, а пути ответа приводятся к корню
// проекта — срез не зависит от того, за сколько обращений он собран.
func TestКартаЗадаётОбходИПутиПриводятсяККорню(t *testing.T) {
	analyzer, log := многоуровневый(t)

	slice, err := Prepare(Options{
		Executable: analyzer,
		ProjectDir: t.TempDir(),
		WorkDir:    t.TempDir(),
		Levels:     []Level{LevelAST},
		Map:        scope.Map{Scan: []string{"lib", "www"}},
	})
	if err != nil {
		t.Fatalf("подготовка не состоялась: %v", err)
	}

	paths := make([]string, 0, len(slice.Modules))
	for _, module := range slice.Modules {
		paths = append(paths, module.Path)
	}
	if len(paths) != 2 || paths[0] != "lib/UseCase.php" || paths[1] != "www/index.php" {
		t.Errorf("пути фактов не приведены к корню проекта: %+v", paths)
	}

	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("журнал обращений не прочитан: %v", err)
	}
	if got := strings.Count(string(calls), "structure "); got != 2 {
		t.Errorf("обращений structure: %d, ожидалось по одному на сканируемый путь", got)
	}
	if len(slice.Scanned) != 2 {
		t.Errorf("карта в срезе: %+v", slice.Scanned)
	}
}

// Уровни функций снимаются обходом функций области карты: на каждую функцию
// заводится запись, и в ней лежат факты всех объявленных уровней.
func TestУровниФункцийСнимаютсяОбходомФункций(t *testing.T) {
	analyzer, log := многоуровневый(t)

	slice, err := Prepare(Options{
		Executable: analyzer,
		ProjectDir: t.TempDir(),
		WorkDir:    t.TempDir(),
		Levels:     []Level{LevelAST, LevelCFG, LevelDFG, LevelPDG},
		Map:        scope.Map{Scan: []string{"lib"}},
	})
	if err != nil {
		t.Fatalf("подготовка не состоялась: %v", err)
	}

	if len(slice.Functions) != 1 {
		t.Fatalf("записей о функциях: %d, ожидалась одна", len(slice.Functions))
	}
	facts := slice.Functions[0]
	switch {
	case facts.File != "lib/UseCase.php" || facts.Name != "execute":
		t.Errorf("функция названа неверно: %+v", facts)
	case len(facts.Reaching) == 0 || len(facts.Available) == 0:
		t.Errorf("факты уровня L3 не собраны: %+v", facts)
	case len(facts.DeadStores) == 0:
		t.Errorf("факты уровня L4 не собраны: %+v", facts)
	case len(facts.Slice) == 0:
		t.Errorf("факты уровня L5 не собраны: %+v", facts)
	case len(slice.Collected) != 4:
		t.Errorf("собранные уровни: %+v", slice.Collected)
	}

	// Команда среза требует строки: она берётся из объявления функции уровня L1.
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("журнал обращений не прочитан: %v", err)
	}
	for _, command := range []string{"reaching-defs ", "available ", "dead-stores ", "slice "} {
		if !strings.Contains(string(calls), command) {
			t.Errorf("команда %q анализатору не отправлялась", command)
		}
	}
}
