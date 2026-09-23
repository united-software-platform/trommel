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
	"sort"
	"strings"

	"github.com/united-software-platform/trommel/internal/catalog"
	"github.com/united-software-platform/trommel/internal/codefacts"
	"github.com/united-software-platform/trommel/internal/intent"
	"github.com/united-software-platform/trommel/internal/rules"
	"github.com/united-software-platform/trommel/internal/scope"
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
	// ScanDirs — сканируемые пути карты проекта. Пустой перечень означает проект
	// целиком: карта сужает область, но не обязана её задавать.
	ScanDirs []string
	// ProjectDir — каталог проверяемого проекта на файловой системе. Нужен фазе
	// подготовки: внешний анализатор принимает путь, а не дерево только для чтения.
	ProjectDir string
	// Analyzer — имя или путь исполняемого файла анализатора кода.
	Analyzer string
	// AnalyzerWorkDir — рабочий каталог процесса анализатора. Лежит вне проверяемого
	// проекта: проект остаётся доступным только на чтение.
	AnalyzerWorkDir string

	// facts — срез, собранный фазой подготовки. Поле внутреннее: вызывающая сторона
	// срез не передаёт и подменить его не может.
	facts *codefacts.Slice
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

	// Фаза подготовки: предмет, который Trommel готовит сам. Она идёт после проверки
	// исполнимости намерения и до исполнения правил — состав уже известен, поэтому
	// анализатор запускается только тогда, когда факты о коде кому-то нужны.
	facts, declared, err := prepare(composition, contract, registry, options)
	if err != nil {
		return nil, err
	}
	options.facts = facts

	if err := checkSubjects(composition, registry, options); err != nil {
		return nil, err
	}

	out := report(composition, registry, options)
	out.ExcludeDirs = options.ExcludeDirs
	out.ScanDirs = options.ScanDirs
	out.RulesSource = file.RulesDir()
	out.Fingerprint = norm.Fingerprint()
	out.CodeFacts = describe(facts, declared)
	return out, nil
}

// prepare выполняет фазу подготовки и возвращает срез вместе с объявленным составом.
//
// Состав среза проверяется всегда, даже когда фактов никто не требует: контракт
// обязан быть исполним целиком, и неизвестный уровень — ошибка контракта, а не
// неудача прогона. Сама подготовка выполняется только при наличии в составе правила,
// которому факты нужны: запуск анализатора ради никому не нужного среза — плата
// без покупки.
func prepare(
	composition *intent.Composition, contract intent.Contract,
	registry *rules.Registry, options Options,
) (*codefacts.Slice, []codefacts.Level, error) {
	declared := make([]codefacts.Level, 0, len(contract.Facts))
	for _, name := range contract.Facts {
		declared = append(declared, codefacts.Level(name))
	}
	if len(declared) > 0 {
		if err := codefacts.CheckLevels(declared); err != nil {
			return nil, nil, fmt.Errorf("контракт %s: %w", contract.Name, err)
		}
	}

	codes := make([]string, 0, len(composition.Rules))
	needed := false
	for _, rule := range composition.Rules {
		codes = append(codes, rule.Code)
		if impl, ok := registry.Lookup(rule.Code); ok &&
			impl.Subject() == rules.SubjectCodeFacts {
			needed = true
		}
	}
	if !needed {
		return nil, declared, nil
	}

	required := registry.CodeLevels(codes)
	if len(declared) == 0 {
		return nil, nil, fmt.Errorf(
			"состав прогона включает правила, требующие фактов о коде (%s), "+
				"а контракт %s состава среза не объявляет",
			demands(required), contract.Name)
	}
	for level, demanding := range required {
		if !declaredLevel(declared, level) {
			return nil, nil, fmt.Errorf(
				"правилам %s требуется уровень среза %s, не объявленный контрактом %s",
				strings.Join(demanding, ", "), level, contract.Name)
		}
	}

	slice, err := codefacts.Prepare(codefacts.Options{
		Executable: options.Analyzer,
		ProjectDir: options.ProjectDir,
		Tree:       options.Project,
		WorkDir:    options.AnalyzerWorkDir,
		Levels:     declared,
		Map:        scope.Map{Scan: options.ScanDirs, Exclude: options.ExcludeDirs},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("подготовка среза фактов о коде не состоялась: %w", err)
	}
	return slice, declared, nil
}

// declaredLevel отвечает, объявлен ли уровень контрактом.
func declaredLevel(declared []codefacts.Level, level codefacts.Level) bool {
	for _, candidate := range declared {
		if candidate == level {
			return true
		}
	}
	return false
}

// demands перечисляет правила, потребовавшие фактов о коде, с их уровнями.
func demands(required map[codefacts.Level][]string) string {
	levels := make([]codefacts.Level, 0, len(required))
	for level := range required {
		levels = append(levels, level)
	}
	sort.Slice(levels, func(i, j int) bool { return levels[i] < levels[j] })

	parts := make([]string, 0, len(levels))
	for _, level := range levels {
		parts = append(parts, fmt.Sprintf("%s — %s",
			strings.Join(required[level], ", "), level))
	}
	return strings.Join(parts, "; ")
}

// describe переносит сведения о срезе в вердикт.
func describe(slice *codefacts.Slice, declared []codefacts.Level) *verdict.CodeFacts {
	if len(declared) == 0 && slice == nil {
		return nil
	}

	names := func(levels []codefacts.Level) []string {
		out := make([]string, 0, len(levels))
		for _, level := range levels {
			out = append(out, string(level))
		}
		return out
	}

	if slice == nil {
		return &verdict.CodeFacts{Declared: names(declared), Unused: true}
	}

	facts := &verdict.CodeFacts{
		Analyzer:  slice.Analyzer.Name,
		Version:   slice.Analyzer.Version,
		Declared:  names(slice.Declared),
		Collected: names(slice.Collected),
	}
	for _, record := range slice.Unexamined {
		facts.Unexamined = append(facts.Unexamined,
			verdict.Unexamined{Where: record.Where, Reason: record.Reason})
	}
	return facts
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
		case rules.SubjectCodeFacts:
			// Предмет готовит сам Trommel, поэтому отсутствие среза здесь означает
			// несостоявшуюся подготовку, а не забывчивость вызывающей стороны.
			if options.facts == nil {
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
			Code:          options.facts,
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
