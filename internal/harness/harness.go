// Пакет harness выполняет прогон: сверяет намерение с нормой, исполняет реализации
// правил состава и собирает вердикт.
//
// Порядок работы отвечает трём вопросам подряд: намерение выражено — намерение
// исполнимо — намерение исполнено. Первые два решаются до обращения к содержимому
// проверяемого проекта, и отрицательный ответ на любой из них прекращает работу.
package harness

import (
	"fmt"
	"io/fs"

	"github.com/united-software-platform/trommel/internal/catalog"
	"github.com/united-software-platform/trommel/internal/intent"
	"github.com/united-software-platform/trommel/internal/rules"
	"github.com/united-software-platform/trommel/internal/verdict"
)

// Options — всё, что нужно прогону.
type Options struct {
	// Version — версия сборки Trommel, попадает в вердикт.
	Version string
	// Project — содержимое проверяемого проекта, только для чтения.
	Project fs.FS
	// ContractFile — путь к файлу контрактов внутри проекта.
	ContractFile string
	// ContractName — имя контракта, выбранное вызывающей стороной.
	ContractName string
	// Registry — реестр реализаций правил.
	Registry *rules.Registry
	// CommitMessage, Path — предметы проверки, передаваемые вызывающей стороной
	// для правил, которым они нужны.
	CommitMessage string
	Path          string
	// ExcludeDirs — каталоги, выведенные из обхода вызывающей стороной.
	ExcludeDirs []string
}

// Run выполняет прогон и возвращает вердикт.
//
// Ошибка означает несостоявшийся прогон: вердикта нет, и вызывающая сторона не должна
// принимать отсутствие нарушений за их отсутствие.
func Run(options Options) (*verdict.Report, error) {
	file, err := intent.Load(options.Project, options.ContractFile)
	if err != nil {
		return nil, err
	}

	contract, err := file.Select(options.ContractName)
	if err != nil {
		return nil, err
	}

	norm, err := catalog.Load(options.Project, file.RulesDir())
	if err != nil {
		return nil, err
	}

	registry := options.Registry
	if registry == nil {
		registry = rules.NewRegistry()
	}

	if dangling := registry.Dangling(norm); len(dangling) > 0 {
		return nil, fmt.Errorf(
			"реализации ссылаются на правила, которых нет в норме: %v; норма ушла вперёд",
			dangling)
	}

	composition, err := contract.Resolve(norm, registry)
	if err != nil {
		return nil, err
	}
	composition.Notes = file.Notes

	if err := checkSubjects(composition, registry, options); err != nil {
		return nil, err
	}

	out := report(composition, registry, options)
	out.ExcludeDirs = options.ExcludeDirs
	out.RulesSource = file.RulesDir()
	out.Fingerprint = norm.Fingerprint()
	return out, nil
}

// checkSubjects проверяет, что каждому правилу состава передан требуемый им предмет.
// Отсутствие предмета — отказ, а не тихое ok на пустом входе.
func checkSubjects(
	composition *intent.Composition, registry *rules.Registry, options Options,
) error {
	for _, rule := range composition.Rules {
		impl, ok := registry.Lookup(rule.Code)
		if !ok {
			continue
		}

		switch subject := impl.Subject(); subject {
		case rules.SubjectCommitMessage:
			if options.CommitMessage == "" {
				return subjectError(rule.Code, subject)
			}
		case rules.SubjectPath:
			if options.Path == "" {
				return subjectError(rule.Code, subject)
			}
		case rules.SubjectTree:
			if options.Project == nil {
				return subjectError(rule.Code, subject)
			}
		}
	}
	return nil
}

func subjectError(code string, subject rules.Subject) error {
	return fmt.Errorf("правилу %s не передан требуемый предмет проверки: %s", code, subject)
}

// report исполняет реализации правил состава и собирает вердикт.
func report(
	composition *intent.Composition, registry *rules.Registry, options Options,
) *verdict.Report {
	out := &verdict.Report{
		Trommel:     options.Version,
		Contract:    composition.Contract,
		RulesSource: "",
		Rules:       make([]verdict.RuleVerdict, 0, len(composition.Rules)),
	}

	for _, rule := range composition.Rules {
		impl, ok := registry.Lookup(rule.Code)
		if !ok {
			out.Rules = append(out.Rules, verdict.RuleVerdict{
				Code:      rule.Code,
				Status:    verdict.StatusNone,
				Text:      rule.Text,
				Check:     composition.Notes[rule.Code],
				Group:     rule.Group,
				SourceDoc: rule.Source,
			})
			continue
		}

		if err := rules.Verify(impl); err != nil {
			out.Rules = append(out.Rules, verdict.RuleVerdict{
				Code:      rule.Code,
				Status:    verdict.StatusErr,
				Detail:    err.Error(),
				Text:      rule.Text,
				Check:     impl.Describe(),
				Group:     rule.Group,
				SourceDoc: rule.Source,
			})
			continue
		}

		result := impl.Check(rules.Context{
			Tree:          options.Project,
			CommitMessage: options.CommitMessage,
			Path:          options.Path,
			Params:        contractParams(composition, rule.Code),
			ExcludeDirs:   options.ExcludeDirs,
		})

		out.Rules = append(out.Rules, verdict.RuleVerdict{
			Code:      rule.Code,
			Status:    result.Status(),
			Detail:    detail(result),
			Text:      rule.Text,
			Check:     impl.Describe(),
			Examined:  result.Examined,
			Findings:  result.Findings,
			Group:     rule.Group,
			SourceDoc: rule.Source,
		})
	}

	for _, excluded := range composition.Excluded {
		out.Excluded = append(out.Excluded, verdict.Excluded{
			Code:   excluded.Rule.Code,
			Reason: excluded.Reason,
			Text:   excluded.Rule.Text,
			Group:  excluded.Rule.Group,
		})
	}

	return out
}

// contractParams возвращает параметры правила из состава.
func contractParams(composition *intent.Composition, code string) map[string]string {
	if composition.Params == nil {
		return nil
	}
	return composition.Params[code]
}

// detail даёт краткое пояснение к строке вердикта.
func detail(result verdict.Result) string {
	if result.Detail != "" {
		return result.Detail
	}
	if len(result.Unexamined) > 0 {
		return fmt.Sprintf("не разобрано: %s", result.Unexamined[0])
	}
	if result.Examined > 0 {
		return fmt.Sprintf("%d просмотрено", result.Examined)
	}
	return ""
}
