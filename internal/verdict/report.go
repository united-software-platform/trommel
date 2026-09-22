package verdict

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Коды возврата. Три исхода различаются намеренно: «нарушения есть» и «прогон
// не состоялся» — разные события для вызывающей стороны, и сводить их к одному
// ненулевому коду значило бы лишить её возможности их различить.
const (
	ExitOK      = 0 // нарушений нет
	ExitFail    = 1 // есть нарушения
	ExitRefused = 2 // прогон не состоялся
)

// RuleVerdict — состояние одного правила состава.
type RuleVerdict struct {
	Code     string    `json:"code"`
	Status   Status    `json:"status"`
	Detail   string    `json:"detail,omitempty"`
	Examined int       `json:"examined,omitempty"`
	Findings []Finding `json:"findings,omitempty"`
}

// Excluded — правило, выведенное из состава контрактом.
type Excluded struct {
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

// Report — вердикт прогона целиком.
type Report struct {
	Trommel     string        `json:"trommel"`     // версия сборки
	Contract    string        `json:"contract"`    // имя применённого контракта
	RulesSource string        `json:"rulesSource"` // откуда взята норма
	Fingerprint string        `json:"fingerprint"` // отпечаток нормы
	Rules       []RuleVerdict `json:"rules"`
	Excluded    []Excluded    `json:"excluded,omitempty"`
}

// Counts — сводка по состояниям правил.
type Counts map[Status]int

// Counts подсчитывает состояния правил состава.
func (r *Report) Counts() Counts {
	counts := make(Counts)
	for _, rule := range r.Rules {
		counts[rule.Status]++
	}
	return counts
}

// Checked возвращает число проверенных правил — знаменатель утверждения вердикта.
func (r *Report) Checked() int {
	checked := 0
	for _, rule := range r.Rules {
		if rule.Status.Проверено() {
			checked++
		}
	}
	return checked
}

// ExitCode возвращает код возврата прогона. Правило в состоянии err не считается
// нарушением: прогон состоялся, но об этом правиле Trommel не утверждает ничего —
// решение принимает вызывающая сторона по самому вердикту.
func (r *Report) ExitCode() int {
	for _, rule := range r.Rules {
		if rule.Status == StatusFail {
			return ExitFail
		}
	}
	return ExitOK
}

// WriteText печатает вердикт для человека: строка на правило и сводка со знаменателем.
//
// Печатаются все правила состава, включая непроверенные: вердикт, показывающий
// только находки, читался бы как «всё в порядке» там, где проверено подмножество.
func (r *Report) WriteText(out io.Writer) error {
	fmt.Fprintf(out, "trommel %s\n", r.Trommel)
	fmt.Fprintf(out, "контракт: %s\n", r.Contract)
	fmt.Fprintf(out, "норма:    %s, правил %d, отпечаток %s\n\n",
		r.RulesSource, len(r.Rules)+len(r.Excluded), short(r.Fingerprint))

	width := 0
	for _, rule := range r.Rules {
		if len(rule.Code) > width {
			width = len(rule.Code)
		}
	}

	for _, rule := range r.Rules {
		fmt.Fprintf(out, "%-*s  %-5s %s\n", width, rule.Code, rule.Status, rule.Detail)
		for _, finding := range rule.Findings {
			fmt.Fprintf(out, "%-*s        %s\n", width, "", finding)
		}
	}

	if len(r.Excluded) > 0 {
		fmt.Fprintf(out, "\nисключено контрактом:\n")
		for _, excluded := range groupExclusions(r.Excluded) {
			fmt.Fprintf(out, "  %-30s %s\n", excluded.codes, excluded.reason)
		}
	}

	counts := r.Counts()
	fmt.Fprintf(out, "\nсостав %d | ok %d | fail %d | err %d | skip %d | -- %d    проверено %d/%d\n",
		len(r.Rules), counts[StatusOK], counts[StatusFail], counts[StatusErr],
		counts[StatusSkip], counts[StatusNone], r.Checked(), len(r.Rules))

	return nil
}

// WriteJSON печатает вердикт для машинной обработки. Набор состояний тот же,
// что и в читаемой форме: две формы одного вердикта, а не два разных вердикта.
func (r *Report) WriteJSON(out io.Writer) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(r)
}

// exclusionGroup — исключения, сведённые по общему обоснованию.
type exclusionGroup struct {
	codes  string
	reason string
}

// groupExclusions сводит исключения с одним обоснованием в одну строку вывода:
// перечень из пятнадцати одинаково обоснованных правил читать незачем.
func groupExclusions(excluded []Excluded) []exclusionGroup {
	byReason := make(map[string][]string)
	for _, item := range excluded {
		byReason[item.Reason] = append(byReason[item.Reason], item.Code)
	}

	reasons := make([]string, 0, len(byReason))
	for reason := range byReason {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)

	out := make([]exclusionGroup, 0, len(reasons))
	for _, reason := range reasons {
		codes := byReason[reason]
		sort.Strings(codes)

		label := strings.Join(codes, ", ")
		if len(codes) > 3 {
			label = summarizeGroups(codes)
		}
		out = append(out, exclusionGroup{codes: label, reason: reason})
	}
	return out
}

// short укорачивает отпечаток до различимой части.
func short(fingerprint string) string {
	if len(fingerprint) <= 12 {
		return fingerprint
	}
	return fingerprint[:12]
}

// summarizeGroups сводит перечень кодов к группам с их числом: диапазон от первого
// кода до последнего охватывал бы несколько групп и читался бы как одна.
func summarizeGroups(codes []string) string {
	counts := make(map[string]int)
	var order []string

	for _, code := range codes {
		group, _, found := strings.Cut(code, "-")
		if !found {
			group = code
		}
		if _, seen := counts[group]; !seen {
			order = append(order, group)
		}
		counts[group]++
	}
	sort.Strings(order)

	parts := make([]string, 0, len(order))
	for _, group := range order {
		parts = append(parts, fmt.Sprintf("%s (%d)", group, counts[group]))
	}
	return strings.Join(parts, ", ")
}
