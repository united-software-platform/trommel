package catalog

import (
	"os"
	"testing"
)

// Каталог строится по действующим документам нормы этого репозитория: число правил
// в нём совпадает с числом строк таблиц правил.
func TestLoadРазбираетДокументыНормы(t *testing.T) {
	catalog, err := Load(os.DirFS("../.."), "rules")
	if err != nil {
		t.Fatalf("каталог не построен: %v", err)
	}

	const expected = 124
	if catalog.Len() != expected {
		t.Errorf("правил в каталоге = %d, ожидалось %d", catalog.Len(), expected)
	}

	rule, ok := catalog.Rule("DOC-005")
	if !ok {
		t.Fatal("правило DOC-005 в каталоге отсутствует")
	}
	if rule.Group != "DOC" {
		t.Errorf("группа DOC-005 = %q, ожидалась %q", rule.Group, "DOC")
	}
	if rule.Text == "" {
		t.Error("у правила DOC-005 пустая формулировка")
	}
}

// Группа собирается по префиксу кода: отдельного объявления состава группы норма
// не содержит, и добавление правила в документ расширяет группу само собой.
func TestGroupСобираетсяПоПрефиксу(t *testing.T) {
	catalog, err := Load(os.DirFS("../.."), "rules")
	if err != nil {
		t.Fatalf("каталог не построен: %v", err)
	}

	const (
		group    = "DOC"
		expected = 23
	)

	rules := catalog.Group(group)
	if len(rules) != expected {
		t.Errorf("правил в группе %s = %d, ожидалось %d", group, len(rules), expected)
	}
	for _, rule := range rules {
		if rule.Group != group {
			t.Errorf("правило %s попало в группу %s", rule.Code, group)
		}
	}

	if got, want := len(catalog.Groups()), 9; got != want {
		t.Errorf("групп в каталоге = %d, ожидалось %d", got, want)
	}
}
