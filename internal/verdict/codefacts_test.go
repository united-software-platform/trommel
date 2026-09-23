package verdict

import (
	"encoding/json"
	"strings"
	"testing"
)

// отчёт собирает вердикт со срезом фактов о коде.
func отчёт(facts *CodeFacts) *Report {
	return &Report{
		Trommel:     "тест",
		Contract:    "full",
		RulesSource: "rules",
		Fingerprint: "0123456789abcdef",
		Rules:       []RuleVerdict{{Code: "CLAR-001", Status: StatusOK}},
		CodeFacts:   facts,
	}
}

// формы печатает вердикт во всех трёх формах.
func формы(t *testing.T, report *Report) map[string]string {
	t.Helper()

	out := map[string]string{}
	for name, write := range map[string]func(*strings.Builder) error{
		"text":     func(b *strings.Builder) error { return report.WriteText(b) },
		"json":     func(b *strings.Builder) error { return report.WriteJSON(b) },
		"markdown": func(b *strings.Builder) error { return report.WriteMarkdown(b) },
	} {
		var buffer strings.Builder
		if err := write(&buffer); err != nil {
			t.Fatalf("форма %s не напечатана: %v", name, err)
		}
		out[name] = buffer.String()
	}
	return out
}

// Вердикт, опиравшийся на факты о коде, называет инструмент и его версию во всех
// формах: без этого повторный прогон не с чем сравнивать.
func TestВердиктНазываетАнализаторВоВсехФормах(t *testing.T) {
	report := отчёт(&CodeFacts{
		Analyzer:  "tldr",
		Version:   "0.4.0",
		Declared:  []string{"L1", "L2"},
		Collected: []string{"L1", "L2"},
	})

	for name, text := range формы(t, report) {
		if !strings.Contains(text, "tldr") || !strings.Contains(text, "0.4.0") {
			t.Errorf("форма %s не называет анализатор и версию:\n%s", name, text)
		}
	}
}

// Объявленный и собранный состав печатаются раздельно: расхождение между ними —
// это то, чего в срезе нет.
func TestВердиктРазличаетОбъявленныйИСобранныйСостав(t *testing.T) {
	report := отчёт(&CodeFacts{
		Analyzer:  "tldr",
		Version:   "0.4.0",
		Declared:  []string{"L1", "L2", "deps"},
		Collected: []string{"L1"},
		Unexamined: []Unexamined{
			{Where: "tools/deploy.sh", Reason: "язык Shell анализатором не поддержан"},
		},
	})

	text := формы(t, report)["text"]
	if !strings.Contains(text, "объявлено L1, L2, deps") {
		t.Errorf("объявленный состав не напечатан:\n%s", text)
	}
	if !strings.Contains(text, "собрано L1") {
		t.Errorf("собранный состав не напечатан:\n%s", text)
	}
	if !strings.Contains(text, "tools/deploy.sh") {
		t.Errorf("граница среза не напечатана:\n%s", text)
	}

	var parsed Report
	if err := json.Unmarshal([]byte(формы(t, report)["json"]), &parsed); err != nil {
		t.Fatalf("машинная форма не разобрана: %v", err)
	}
	if len(parsed.CodeFacts.Declared) != 3 || len(parsed.CodeFacts.Collected) != 1 {
		t.Errorf("составы в машинной форме: %+v", parsed.CodeFacts)
	}
	if len(parsed.CodeFacts.Unexamined) != 1 {
		t.Errorf("граница среза в машинной форме: %+v", parsed.CodeFacts.Unexamined)
	}
}

// Уровень, собранный и не давший фактов, отличим от несобранного: первый входит
// в собранный состав, второго там нет.
func TestСобранныйПустойУровеньОтличимОтНесобранного(t *testing.T) {
	report := отчёт(&CodeFacts{
		Analyzer: "tldr", Version: "0.4.0",
		Declared:  []string{"L1", "L2"},
		Collected: []string{"L1", "L2"},
	})

	text := формы(t, report)["text"]
	if !strings.Contains(text, "собрано L1, L2") {
		t.Errorf("пустой собранный уровень пропал из состава:\n%s", text)
	}
}

// Объявленный и не собиравшийся срез предъявляется отдельно: читатель не должен
// принимать необходимость подготовки за её выполнение.
func TestНеиспользованныйСрезПредъявлен(t *testing.T) {
	report := отчёт(&CodeFacts{Declared: []string{"L1"}, Unused: true})

	for name, text := range формы(t, report) {
		if name == "json" {
			continue
		}
		if !strings.Contains(text, "не собирался") {
			t.Errorf("форма %s не предъявляет неиспользованный срез:\n%s", name, text)
		}
	}
}

// Карта прогона называется в вердикте: срез части проекта не должен читаться
// как срез проекта целиком.
func TestВердиктНазываетКартуПрогона(t *testing.T) {
	report := отчёт(&CodeFacts{
		Analyzer: "tldr", Version: "0.4.0",
		Declared:  []string{"L1"},
		Collected: []string{"L1"},
	})
	report.ScanDirs = []string{"lib", "www"}
	report.ExcludeDirs = []string{"lib/vendor"}

	for name, text := range формы(t, report) {
		if !strings.Contains(text, "lib") || !strings.Contains(text, "www") {
			t.Errorf("форма %s не называет сканируемые пути:\n%s", name, text)
		}
		if !strings.Contains(text, "lib/vendor") {
			t.Errorf("форма %s не называет исключённые пути:\n%s", name, text)
		}
	}
}
