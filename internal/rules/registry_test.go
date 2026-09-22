package rules

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/united-software-platform/trommel/internal/catalog"
	"github.com/united-software-platform/trommel/internal/verdict"
)

// заглушка — реализация правила, не выполняющая проверки: в этих тестах значима
// только её принадлежность коду.
type заглушка struct{ code string }

func (з заглушка) Code() string { return з.code }

func (з заглушка) Subject() Subject { return SubjectTree }

func (з заглушка) RequiredParams() []string { return nil }

func (з заглушка) Check(Context) verdict.Result { return verdict.Result{} }

func (з заглушка) Describe() string { return "описание проверки" }

func (з заглушка) Samples() Samples { return Samples{} }

func нормаИзКодов(t *testing.T, codes ...string) *catalog.Catalog {
	t.Helper()

	var builder strings.Builder
	builder.WriteString("# Норма\n\n| ID | Правило |\n|----|---------|\n")
	for _, code := range codes {
		builder.WriteString("| " + code + " | Формулировка |\n")
	}

	fsys := fstest.MapFS{"norm/doc.md": &fstest.MapFile{Data: []byte(builder.String())}}

	norm, err := catalog.Load(fsys, "norm")
	if err != nil {
		t.Fatalf("норма не построена: %v", err)
	}
	return norm
}

// Правило без реализации остаётся в норме и помечается непроверяемым — оно не
// исчезает из состава и не выдаёт себя за соблюдённое.
func TestПравилоБезРеализацииОстаётсяВСоставе(t *testing.T) {
	norm := нормаИзКодов(t, "DOC-005", "DOC-006")

	registry := NewRegistry()
	if err := registry.Register(заглушка{code: "DOC-005"}); err != nil {
		t.Fatalf("реализация не зарегистрирована: %v", err)
	}

	if !registry.Implemented("DOC-005") {
		t.Error("реализованное правило помечено как непроверяемое")
	}
	if registry.Implemented("DOC-006") {
		t.Error("правило без реализации помечено как проверяемое")
	}

	unimplemented := registry.Unimplemented(norm)
	if len(unimplemented) != 1 || unimplemented[0] != "DOC-006" {
		t.Errorf("перечень правил без реализации = %v, ожидался [DOC-006]", unimplemented)
	}
}

// Две реализации одного правила сделали бы вердикт зависимым от порядка регистрации.
func TestПовторнаяРегистрацияОтвергается(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(заглушка{code: "DOC-005"}); err != nil {
		t.Fatalf("первая регистрация не удалась: %v", err)
	}

	if err := registry.Register(заглушка{code: "DOC-005"}); err == nil {
		t.Error("повторная регистрация правила DOC-005 принята")
	}
}

// Реализация, которой не соответствует ни одно правило нормы, обнаруживается:
// норма ушла вперёд, и проверять больше нечего.
func TestВисячаяРеализацияОбнаруживается(t *testing.T) {
	norm := нормаИзКодов(t, "DOC-005")

	registry := NewRegistry()
	for _, code := range []string{"DOC-005", "DOC-099"} {
		if err := registry.Register(заглушка{code: code}); err != nil {
			t.Fatalf("реализация %s не зарегистрирована: %v", code, err)
		}
	}

	dangling := registry.Dangling(norm)
	if len(dangling) != 1 || dangling[0] != "DOC-099" {
		t.Errorf("перечень висячих реализаций = %v, ожидался [DOC-099]", dangling)
	}
}
