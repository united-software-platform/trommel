package harness

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/united-software-platform/trommel/internal/rules"
	"github.com/united-software-platform/trommel/internal/verdict"
)

// реализация — управляемая реализация правила для проверки поведения прогона.
// Нарушением считает присутствие искомой строки в файле: поведение зависит
// от содержимого, поэтому образцы самопроверки для неё осмысленны.
type реализация struct {
	code        string
	subject     rules.Subject
	needle      string
	unexamined  []string
	безОбразцов bool
}

func (р реализация) Code() string { return р.code }

func (р реализация) Subject() rules.Subject { return р.subject }

func (р реализация) RequiredParams() []string { return nil }

func (р реализация) Check(ctx rules.Context) verdict.Result {
	result := verdict.Result{Unexamined: р.unexamined}
	if ctx.Tree == nil || р.needle == "" {
		return result
	}

	_ = fs.WalkDir(ctx.Tree, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		content, readErr := fs.ReadFile(ctx.Tree, path)
		if readErr != nil {
			result.Unexamined = append(result.Unexamined, path)
			return nil
		}
		result.Examined++
		if strings.Contains(string(content), р.needle) {
			result.Findings = append(result.Findings,
				verdict.Finding{File: path, Message: "найдено " + р.needle})
		}
		return nil
	})
	return result
}

func (р реализация) Describe() string { return "описание проверки" }

func (р реализация) Samples() rules.Samples {
	if р.безОбразцов {
		return rules.Samples{}
	}
	return rules.Samples{
		Violating: []rules.Sample{{Name: "образец.md", Content: "текст " + р.needle}},
		Clean:     []rules.Sample{{Name: "образец.md", Content: "чистый текст"}},
	}
}

// проект собирает проверяемый проект с нормой и контрактом.
func проект(contract string, codes ...string) fstest.MapFS {
	var norm strings.Builder
	norm.WriteString("# Норма\n\n| ID | Правило |\n|----|---------|\n")
	for _, code := range codes {
		norm.WriteString("| " + code + " | Формулировка |\n")
	}

	return fstest.MapFS{
		"rules/norm.md": &fstest.MapFile{Data: []byte(norm.String())},
		"trommel.yaml":  &fstest.MapFile{Data: []byte(contract)},
	}
}

const простойКонтракт = "version: 1\ncontracts:\n  docs: {}\n"

func прогон(t *testing.T, project fstest.MapFS, registry *rules.Registry) (*verdict.Report, error) {
	t.Helper()

	return Run(Options{
		Version:      "тест",
		Project:      project,
		ContractFile: "trommel.yaml",
		ContractName: "docs",
		Registry:     registry,
	})
}

// Правило без реализации остаётся в составе и предъявляется непроверенным.
func TestRunПравилоБезРеализацииПолучаетСостояниеNone(t *testing.T) {
	report, err := прогон(t, проект(простойКонтракт, "DOC-005", "DOC-006"), rules.NewRegistry())
	if err != nil {
		t.Fatalf("прогон не выполнен: %v", err)
	}

	if len(report.Rules) != 2 {
		t.Fatalf("правил в вердикте = %d, ожидалось 2", len(report.Rules))
	}
	for _, rule := range report.Rules {
		if rule.Status != verdict.StatusNone {
			t.Errorf("состояние %s = %q, ожидалось %q", rule.Code, rule.Status, verdict.StatusNone)
		}
	}
	if report.Checked() != 0 {
		t.Errorf("проверенных правил = %d, ожидалось 0", report.Checked())
	}
	if report.ExitCode() != verdict.ExitOK {
		t.Errorf("код возврата = %d, ожидался %d", report.ExitCode(), verdict.ExitOK)
	}
}

// Реализация, которой не соответствует правило нормы, прекращает прогон: норма ушла
// вперёд, и проверять больше нечего.
func TestRunОтвергаетВисячуюРеализацию(t *testing.T) {
	registry := rules.NewRegistry()
	if err := registry.Register(
		реализация{code: "DOC-099", subject: rules.SubjectTree, needle: "HTML"}); err != nil {
		t.Fatalf("реализация не зарегистрирована: %v", err)
	}

	_, err := прогон(t, проект(простойКонтракт, "DOC-005"), registry)
	if err == nil {
		t.Fatal("прогон выполнен, хотя реализация ссылается на отсутствующее правило")
	}
	if !strings.Contains(err.Error(), "DOC-099") {
		t.Errorf("сообщение не называет висячую реализацию: %v", err)
	}
}

// Правило, которому не передан требуемый предмет, отказывает в запуске, а не выдаёт
// ok на пустом входе.
func TestRunТребуетПредметПроверки(t *testing.T) {
	registry := rules.NewRegistry()
	if err := registry.Register(
		реализация{code: "GIT-009", subject: rules.SubjectCommitMessage, needle: "подпись"}); err != nil {
		t.Fatalf("реализация не зарегистрирована: %v", err)
	}

	_, err := прогон(t, проект(простойКонтракт, "GIT-009"), registry)
	if err == nil {
		t.Fatal("прогон выполнен без требуемого предмета проверки")
	}
	if !strings.Contains(err.Error(), "GIT-009") {
		t.Errorf("сообщение не называет правило: %v", err)
	}
}

// Найденное нарушение даёт состояние fail и ненулевой код возврата.
func TestRunНарушениеДаётFail(t *testing.T) {
	registry := rules.NewRegistry()
	if err := registry.Register(
		реализация{code: "DOC-005", subject: rules.SubjectTree, needle: "<div>"}); err != nil {
		t.Fatalf("реализация не зарегистрирована: %v", err)
	}

	project := проект(простойКонтракт, "DOC-005")
	project["README.md"] = &fstest.MapFile{Data: []byte("текст <div> ещё текст")}

	report, err := прогон(t, project, registry)
	if err != nil {
		t.Fatalf("прогон не выполнен: %v", err)
	}

	if report.Rules[0].Status != verdict.StatusFail {
		t.Errorf("состояние = %q, ожидалось %q", report.Rules[0].Status, verdict.StatusFail)
	}
	if report.ExitCode() != verdict.ExitFail {
		t.Errorf("код возврата = %d, ожидался %d", report.ExitCode(), verdict.ExitFail)
	}
}

// Непросмотренное содержимое не даёт состояния ok, каким бы ни был перечень находок:
// частичный просмотр права на утверждение не даёт.
func TestRunНеполныйПросмотрДаётErr(t *testing.T) {
	registry := rules.NewRegistry()
	err := registry.Register(реализация{
		code:       "DOC-005",
		subject:    rules.SubjectTree,
		needle:     "<div>",
		unexamined: []string{"rules/broken.md"},
	})
	if err != nil {
		t.Fatalf("реализация не зарегистрирована: %v", err)
	}

	report, err := прогон(t, проект(простойКонтракт, "DOC-005"), registry)
	if err != nil {
		t.Fatalf("прогон не выполнен: %v", err)
	}

	if report.Rules[0].Status != verdict.StatusErr {
		t.Errorf("состояние = %q, ожидалось %q", report.Rules[0].Status, verdict.StatusErr)
	}
	if report.ExitCode() != verdict.ExitOK {
		t.Errorf("err не является нарушением: код = %d", report.ExitCode())
	}
}

// Реализация, не предъявившая оба набора образцов, не может утверждать соблюдение
// правила: о её работоспособности ничего не известно.
func TestRunРеализацияБезОбразцовНеДаётOK(t *testing.T) {
	registry := rules.NewRegistry()
	err := registry.Register(реализация{
		code:        "DOC-005",
		subject:     rules.SubjectTree,
		needle:      "<div>",
		безОбразцов: true,
	})
	if err != nil {
		t.Fatalf("реализация не зарегистрирована: %v", err)
	}

	report, err := прогон(t, проект(простойКонтракт, "DOC-005"), registry)
	if err != nil {
		t.Fatalf("прогон не выполнен: %v", err)
	}

	if report.Rules[0].Status != verdict.StatusErr {
		t.Errorf("состояние = %q, ожидалось %q", report.Rules[0].Status, verdict.StatusErr)
	}
	if !strings.Contains(report.Rules[0].Detail, "образц") {
		t.Errorf("пояснение не называет причину: %q", report.Rules[0].Detail)
	}
}

// Вердикт воспроизводим: те же входы дают те же состояния правил.
func TestRunВердиктВоспроизводим(t *testing.T) {
	build := func() *verdict.Report {
		registry := rules.NewRegistry()
		if err := registry.Register(
			реализация{code: "DOC-005", subject: rules.SubjectTree, needle: "<div>"}); err != nil {
			t.Fatalf("реализация не зарегистрирована: %v", err)
		}

		report, err := прогон(t, проект(простойКонтракт, "DOC-005"), registry)
		if err != nil {
			t.Fatalf("прогон не выполнен: %v", err)
		}
		return report
	}

	first, second := build(), build()

	if first.Fingerprint != second.Fingerprint {
		t.Error("отпечаток нормы изменился между прогонами на неизменных входах")
	}
	if len(first.Rules) != len(second.Rules) {
		t.Fatalf("состав различается: %d и %d", len(first.Rules), len(second.Rules))
	}
	for i := range first.Rules {
		if first.Rules[i].Status != second.Rules[i].Status {
			t.Errorf("состояние %s различается: %q и %q",
				first.Rules[i].Code, first.Rules[i].Status, second.Rules[i].Status)
		}
	}
}
