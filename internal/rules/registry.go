// Пакет rules ведёт реестр реализаций правил и сопоставляет его с нормой.
//
// Норма объявляет, какие правила действуют; реестр — какие из них Trommel умеет
// проверять. Расхождение между ними значимо в обе стороны: правило без реализации
// остаётся в составе прогона и предъявляется непроверенным, а реализация без правила
// означает, что норма ушла вперёд, и работа прекращается.
package rules

import (
	"fmt"
	"io/fs"
	"sort"

	"github.com/united-software-platform/trommel/internal/catalog"
	"github.com/united-software-platform/trommel/internal/verdict"
)

// Subject — предмет проверки, который требуется правилу.
//
// Предмет вычисляется из состава правил, а не задаётся вызывающей стороной:
// нельзя запросить правило и не дать ему того, что оно проверяет. Правило,
// оставшееся без предмета, отказывает в запуске, а не выдаёт ok на пустом входе.
type Subject string

const (
	// SubjectTree — дерево файлов проверяемого проекта.
	SubjectTree Subject = "дерево файлов"
	// SubjectCommitMessage — текст сообщения коммита.
	SubjectCommitMessage Subject = "текст сообщения коммита"
	// SubjectPath — путь, к которому собираются обратиться.
	SubjectPath Subject = "путь обращения"
)

// Context — то, что реализация получает для проверки.
type Context struct {
	// Tree — содержимое проверяемого проекта, только для чтения.
	Tree fs.FS
	// CommitMessage — текст сообщения коммита, если предмет затребован.
	CommitMessage string
	// Path — путь обращения, если предмет затребован.
	Path string
	// Params — параметры правила из контракта намерения.
	Params map[string]string
	// ExcludeDirs — каталоги, не подлежащие обходу. Сужение области задаёт вызывающая
	// сторона, и оно остаётся видимым в вердикте: исключённая область не исчезает
	// из вывода, а называется в нём.
	ExcludeDirs []string
}

// Implementation — реализация одного правила нормы.
type Implementation interface {
	// Code возвращает код правила, которое проверяет эта реализация.
	Code() string
	// Subject возвращает предмет, без которого правило не может быть проверено.
	Subject() Subject
	// RequiredParams возвращает имена обязательных параметров правила.
	RequiredParams() []string
	// Check выполняет проверку и сообщает, что просмотрено, что не удалось
	// просмотреть и какие нарушения найдены.
	Check(Context) verdict.Result
	// Samples возвращает образцы самопроверки: на чём реализация обязана
	// сработать и на чём обязана смолчать.
	Samples() Samples
	// Describe описывает границу проверки: что именно реализация считает
	// нарушением. Описание попадает в сводку покрытия, поэтому читатель видит
	// не только состояние правила, но и объём утверждения о нём.
	Describe() string
}

// Registry — реестр реализаций: одна реализация на правило.
type Registry struct {
	byCode map[string]Implementation
}

// NewRegistry создаёт пустой реестр.
func NewRegistry() *Registry {
	return &Registry{byCode: make(map[string]Implementation)}
}

// Register добавляет реализацию. Повторная регистрация того же кода отвергается:
// правило — единица реализации, и две реализации одного правила означали бы, что
// вердикт зависит от порядка регистрации.
func (r *Registry) Register(impl Implementation) error {
	code := impl.Code()
	if _, exists := r.byCode[code]; exists {
		return fmt.Errorf("реализация правила %s зарегистрирована повторно", code)
	}
	r.byCode[code] = impl
	return nil
}

// Lookup возвращает реализацию правила и признак её наличия.
func (r *Registry) Lookup(code string) (Implementation, bool) {
	impl, ok := r.byCode[code]
	return impl, ok
}

// Implemented отвечает, есть ли у правила реализация.
func (r *Registry) Implemented(code string) bool {
	_, ok := r.byCode[code]
	return ok
}

// Codes возвращает коды зарегистрированных реализаций в алфавитном порядке.
func (r *Registry) Codes() []string {
	out := make([]string, 0, len(r.byCode))
	for code := range r.byCode {
		out = append(out, code)
	}
	sort.Strings(out)
	return out
}

// Dangling возвращает коды реализаций, которым в норме не соответствует ни одно
// правило. Такая реализация ссылается на правило, которое переименовали или удалили,
// и исполнять её нельзя: она проверяет то, чего норма больше не требует.
func (r *Registry) Dangling(norm *catalog.Catalog) []string {
	var out []string
	for _, code := range r.Codes() {
		if _, declared := norm.Rule(code); !declared {
			out = append(out, code)
		}
	}
	return out
}

// Unimplemented возвращает коды правил нормы, у которых нет реализации, в порядке
// объявления. Эти правила остаются в составе прогона и предъявляются непроверенными.
func (r *Registry) Unimplemented(norm *catalog.Catalog) []string {
	var out []string
	for _, rule := range norm.Rules() {
		if !r.Implemented(rule.Code) {
			out = append(out, rule.Code)
		}
	}
	return out
}

// RequiredParams возвращает обязательные параметры правила: реестр отвечает
// на вопрос контракта о требованиях, не раскрывая ему устройство реализаций.
func (r *Registry) RequiredParams(code string) []string {
	impl, ok := r.byCode[code]
	if !ok {
		return nil
	}
	return impl.RequiredParams()
}

// Subjects возвращает предметы, требуемые реализациями перечисленных правил.
func (r *Registry) Subjects(codes []string) []Subject {
	seen := make(map[Subject]struct{})
	var out []Subject

	for _, code := range codes {
		impl, ok := r.byCode[code]
		if !ok {
			continue
		}
		subject := impl.Subject()
		if _, known := seen[subject]; known {
			continue
		}
		seen[subject] = struct{}{}
		out = append(out, subject)
	}
	return out
}

// Describe возвращает описание проверки правила либо пустую строку, если
// реализации нет.
func (r *Registry) Describe(code string) string {
	impl, ok := r.byCode[code]
	if !ok {
		return ""
	}
	return impl.Describe()
}
