package catalog

import (
	"strings"
	"testing"
	"testing/fstest"
)

// документНормы собирает минимальный документ нормы с таблицей правил.
func документНормы(rows ...string) string {
	var builder strings.Builder
	builder.WriteString("# Норма\n\n## Таблица правил\n\n| ID | Правило |\n|----|---------|\n")
	for _, row := range rows {
		builder.WriteString(row + "\n")
	}
	return builder.String()
}

// Строка таблицы правил с кодом неверного формата — ошибка разбора. Пропуск такой
// строки означал бы потерю правила, о которой никто не узнает, поэтому каталог
// не строится вовсе.
func TestLoadОтвергаетНеразобранныйКод(t *testing.T) {
	fsys := fstest.MapFS{
		"norm/broken.md": &fstest.MapFile{
			Data: []byte(документНормы("| DOC-005 | Запрет HTML |", "| DOC-5 | Опечатка в коде |")),
		},
	}

	_, err := Load(fsys, "norm")
	if err == nil {
		t.Fatal("каталог построен, хотя код правила не соответствует формату")
	}
	if !strings.Contains(err.Error(), "DOC-5") {
		t.Errorf("в сообщении нет неразобранного кода: %v", err)
	}
}

// Правило с пустой формулировкой объявлением не является.
func TestLoadОтвергаетПустуюФормулировку(t *testing.T) {
	fsys := fstest.MapFS{
		"norm/empty.md": &fstest.MapFile{Data: []byte(документНормы("| DOC-005 |  |"))},
	}

	if _, err := Load(fsys, "norm"); err == nil {
		t.Fatal("каталог построен, хотя формулировка правила пуста")
	}
}

// Один код обозначает ровно одно правило: повторное объявление — расхождение в норме.
func TestLoadОтвергаетПовторныйКод(t *testing.T) {
	fsys := fstest.MapFS{
		"norm/a.md": &fstest.MapFile{Data: []byte(документНормы("| DOC-005 | Запрет HTML |"))},
		"norm/b.md": &fstest.MapFile{Data: []byte(документНормы("| DOC-005 | То же другими словами |"))},
	}

	_, err := Load(fsys, "norm")
	if err == nil {
		t.Fatal("каталог построен, хотя код объявлен дважды")
	}
	if !strings.Contains(err.Error(), "повторно") {
		t.Errorf("сообщение не называет причину: %v", err)
	}
}

// Таблица внутри огороженного блока кода — пример разметки, а не объявление правила.
func TestLoadПропускаетПримерыВБлокахКода(t *testing.T) {
	document := документНормы("| DOC-005 | Запрет HTML |") +
		"\n```markdown\n| ID | Правило |\n|----|---------|\n| XXX-001 | Пример из документации |\n```\n"

	fsys := fstest.MapFS{"norm/doc.md": &fstest.MapFile{Data: []byte(document)}}

	catalog, err := Load(fsys, "norm")
	if err != nil {
		t.Fatalf("каталог не построен: %v", err)
	}
	if catalog.Len() != 1 {
		t.Errorf("правил в каталоге = %d, ожидалось 1", catalog.Len())
	}
	if _, ok := catalog.Rule("XXX-001"); ok {
		t.Error("правило из примера в блоке кода попало в каталог")
	}
}

// Отпечаток различает состояния нормы: правка формулировки его меняет.
func TestFingerprintРазличаетСостоянияНормы(t *testing.T) {
	before := fstest.MapFS{
		"norm/doc.md": &fstest.MapFile{Data: []byte(документНормы("| DOC-005 | Запрет HTML |"))},
	}
	after := fstest.MapFS{
		"norm/doc.md": &fstest.MapFile{Data: []byte(документНормы("| DOC-005 | Запрет любого HTML |"))},
	}

	first, err := Load(before, "norm")
	if err != nil {
		t.Fatalf("каталог не построен: %v", err)
	}
	second, err := Load(after, "norm")
	if err != nil {
		t.Fatalf("каталог не построен: %v", err)
	}

	if first.Fingerprint() == second.Fingerprint() {
		t.Error("отпечаток не изменился при правке формулировки правила")
	}
	if first.Fingerprint() != first.Fingerprint() {
		t.Error("отпечаток неустойчив при повторном вычислении")
	}
}
