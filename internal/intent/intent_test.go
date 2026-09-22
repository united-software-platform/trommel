package intent

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/united-software-platform/trommel/internal/catalog"
)

// требования — реализация Requirements для проверки обязательных параметров.
type требования map[string][]string

func (т требования) RequiredParams(code string) []string { return т[code] }

func норма(t *testing.T, codes ...string) *catalog.Catalog {
	t.Helper()

	var builder strings.Builder
	builder.WriteString("# Норма\n\n| ID | Правило |\n|----|---------|\n")
	for _, code := range codes {
		builder.WriteString("| " + code + " | Формулировка |\n")
	}

	norm, err := catalog.Load(
		fstest.MapFS{"norm/doc.md": &fstest.MapFile{Data: []byte(builder.String())}}, "norm")
	if err != nil {
		t.Fatalf("норма не построена: %v", err)
	}
	return norm
}

func контракт(t *testing.T, body string) *File {
	t.Helper()

	file, err := Load(fstest.MapFS{"trommel.yaml": &fstest.MapFile{Data: []byte(body)}}, "trommel.yaml")
	if err != nil {
		t.Fatalf("контракт не разобран: %v", err)
	}
	return file
}

// Контракт разбирается, контракт выбирается по имени, каталог нормы берётся из него.
func TestLoadРазбираетКонтракт(t *testing.T) {
	file := контракт(t, `
version: 1
rules: rules
contracts:
  docs:
    exclude:
      - rule: DOCK
        reason: в проекте нет контейнеров
`)

	if file.RulesDir() != "rules" {
		t.Errorf("каталог нормы = %q, ожидался %q", file.RulesDir(), "rules")
	}

	selected, err := file.Select("docs")
	if err != nil {
		t.Fatalf("контракт не выбран: %v", err)
	}
	if selected.Name != "docs" {
		t.Errorf("имя контракта = %q, ожидалось %q", selected.Name, "docs")
	}
}

// Неизвестное поле означает контракт, писавшийся под другую версию: молчаливое
// игнорирование дало бы вердикт по частично понятому намерению.
func TestLoadОтвергаетНеизвестноеПоле(t *testing.T) {
	_, err := Load(fstest.MapFS{"trommel.yaml": &fstest.MapFile{Data: []byte(`
version: 1
contracts:
  docs:
    strictness: высокая
`)}}, "trommel.yaml")

	if err == nil {
		t.Fatal("контракт принят, хотя содержит неизвестное поле")
	}
	if !strings.Contains(err.Error(), "strictness") {
		t.Errorf("сообщение не называет неизвестное поле: %v", err)
	}
}

// Версия контракта грубая: совпала — работаем, не совпала — отказ.
func TestLoadОтвергаетЧужуюВерсию(t *testing.T) {
	_, err := Load(fstest.MapFS{"trommel.yaml": &fstest.MapFile{Data: []byte(`
version: 99
contracts:
  docs: {}
`)}}, "trommel.yaml")

	if err == nil {
		t.Fatal("контракт принят, хотя объявляет неподдерживаемую версию")
	}
}

// Необъявленное имя контракта — ошибка вызова; вывод называет объявленные.
func TestSelectОтвергаетНеобъявленноеИмя(t *testing.T) {
	file := контракт(t, "version: 1\ncontracts:\n  docs: {}\n  arch: {}\n")

	_, err := file.Select("нет-такого")
	if err == nil {
		t.Fatal("выбран контракт, который не объявлен")
	}
	for _, name := range []string{"arch", "docs"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("сообщение не называет объявленный контракт %s: %v", name, err)
		}
	}
}

// Правило, не упомянутое контрактом, входит в состав: норма, ушедшая вперёд,
// проявляется в прогоне сама.
func TestResolveВключаетНеупомянутоеПравило(t *testing.T) {
	norm := норма(t, "DOC-005", "DOC-006", "DOCK-011")
	file := контракт(t, `
version: 1
contracts:
  docs:
    exclude:
      - rule: DOCK
        reason: в проекте нет контейнеров
`)

	selected, err := file.Select("docs")
	if err != nil {
		t.Fatalf("контракт не выбран: %v", err)
	}

	composition, err := selected.Resolve(norm, nil)
	if err != nil {
		t.Fatalf("состав не вычислен: %v", err)
	}

	if len(composition.Rules) != 2 {
		t.Errorf("правил в составе = %d, ожидалось 2", len(composition.Rules))
	}
	if len(composition.Excluded) != 1 || composition.Excluded[0].Rule.Code != "DOCK-011" {
		t.Errorf("исключено %v, ожидалось одно правило DOCK-011", composition.Excluded)
	}
	if composition.Excluded[0].Reason == "" {
		t.Error("исключение попало в состав без обоснования")
	}
}

// Исключение без обоснования не принимается: обоснование — то, что делает
// исключение видимым в выводе, а не молчаливым.
func TestResolveТребуетОбоснованиеИсключения(t *testing.T) {
	norm := норма(t, "DOC-005")
	file := контракт(t, "version: 1\ncontracts:\n  docs:\n    exclude:\n      - rule: DOC-005\n")

	selected, _ := file.Select("docs")
	if _, err := selected.Resolve(norm, nil); err == nil {
		t.Fatal("исключение без обоснования принято")
	}
}

// Опечатка в коде правила означала бы непроверенное правило при внешне нормальном
// выводе, поэтому обнаруживается до прогона.
func TestResolveОтвергаетНесуществующийКод(t *testing.T) {
	norm := норма(t, "DOC-005")
	file := контракт(t, `
version: 1
contracts:
  docs:
    exclude:
      - rule: DOC-O05
        reason: опечатка в коде
`)

	selected, _ := file.Select("docs")
	_, err := selected.Resolve(norm, nil)
	if err == nil {
		t.Fatal("контракт со ссылкой на несуществующее правило принят")
	}
	if !strings.Contains(err.Error(), "DOC-O05") {
		t.Errorf("сообщение не называет несуществующий код: %v", err)
	}
}

// Незаданный обязательный параметр не даёт правилу исполняться.
func TestResolveТребуетОбязательныеПараметры(t *testing.T) {
	norm := норма(t, "CLAR-001")
	file := контракт(t, "version: 1\ncontracts:\n  arch: {}\n")

	selected, _ := file.Select("arch")
	_, err := selected.Resolve(norm, требования{"CLAR-001": {"layers"}})
	if err == nil {
		t.Fatal("правило с незаданным обязательным параметром допущено к прогону")
	}
	if !strings.Contains(err.Error(), "layers") {
		t.Errorf("сообщение не называет параметр: %v", err)
	}
}

// В исчерпывающем режиме правило, не названное контрактом, останавливает прогон:
// проект обязан принять решение о каждом правиле нормы.
func TestResolveСтрогийРежимТребуетУчётаВсехПравил(t *testing.T) {
	norm := норма(t, "DOC-005", "DOC-006")
	file := контракт(t, `
version: 1
contracts:
  docs:
    strict: true
    acknowledge:
      - DOC-005
`)

	selected, _ := file.Select("docs")
	_, err := selected.Resolve(norm, nil)
	if err == nil {
		t.Fatal("исчерпывающий контракт принят, хотя правило DOC-006 не учтено")
	}
	if !strings.Contains(err.Error(), "DOC-006") {
		t.Errorf("сообщение не называет неучтённое правило: %v", err)
	}
}
