package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
)

// Catalog — перечень правил действующей нормы, построенный по её документам.
type Catalog struct {
	source string
	rules  []Rule
	byCode map[string]Rule
}

// Load строит каталог по документам нормы, лежащим в каталоге dir файловой системы fsys.
//
// Разбор либо удаётся целиком, либо не удаётся вовсе: документ, разобранный частично,
// дал бы частичный каталог, а правило, потерянное при разборе, молча выпало бы из всякого
// прогона. Поэтому первая же ошибка прекращает построение.
func Load(fsys fs.FS, dir string) (*Catalog, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("каталог правил %q недоступен: %w", dir, err)
	}

	catalog := &Catalog{source: dir, byCode: make(map[string]Rule)}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		full := path.Join(dir, name)

		file, err := fsys.Open(full)
		if err != nil {
			return nil, fmt.Errorf("документ нормы %q недоступен: %w", full, err)
		}

		rules, err := parseDocument(full, file)
		closeErr := file.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, fmt.Errorf("документ нормы %q не закрыт: %w", full, closeErr)
		}

		if err := catalog.add(rules); err != nil {
			return nil, err
		}
	}

	if len(catalog.rules) == 0 {
		return nil, fmt.Errorf("в каталоге правил %q не объявлено ни одного правила", dir)
	}
	return catalog, nil
}

// LoadDir строит каталог по каталогу документов нормы в файловой системе операционной
// системы. Корнем разбора становится сам каталог: правила адресуются именами документов
// внутри него, а не путём, по которому его открыли.
func LoadDir(dir string) (*Catalog, error) {
	catalog, err := Load(os.DirFS(dir), ".")
	if err != nil {
		return nil, err
	}
	catalog.source = dir
	return catalog, nil
}

// add добавляет правила, отвергая повторное объявление кода: один код обозначает ровно
// одно правило, и два объявления означают расхождение в самой норме.
func (c *Catalog) add(rules []Rule) error {
	for _, rule := range rules {
		if existing, seen := c.byCode[rule.Code]; seen {
			return fmt.Errorf(
				"правило %s объявлено повторно: %s:%d и %s:%d",
				rule.Code, existing.Source, existing.Line, rule.Source, rule.Line)
		}
		c.byCode[rule.Code] = rule
		c.rules = append(c.rules, rule)
	}
	return nil
}

// Len возвращает число правил в каталоге.
func (c *Catalog) Len() int { return len(c.rules) }

// Source возвращает каталог документов, из которого построена норма.
func (c *Catalog) Source() string { return c.source }

// Rules возвращает правила в порядке объявления.
func (c *Catalog) Rules() []Rule {
	out := make([]Rule, len(c.rules))
	copy(out, c.rules)
	return out
}

// Rule возвращает правило по коду.
func (c *Catalog) Rule(code string) (Rule, bool) {
	rule, ok := c.byCode[code]
	return rule, ok
}

// Group возвращает правила группы в порядке объявления. Состав группы определяется
// каталогом в момент запроса, а не перечнем в вызывающей стороне: правило, добавленное
// в норму, попадает в группу само.
func (c *Catalog) Group(group string) []Rule {
	var out []Rule
	for _, rule := range c.rules {
		if rule.Group == group {
			out = append(out, rule)
		}
	}
	return out
}

// Groups возвращает группы каталога в алфавитном порядке.
func (c *Catalog) Groups() []string {
	seen := make(map[string]struct{}, len(c.rules))
	var out []string
	for _, rule := range c.rules {
		if _, ok := seen[rule.Group]; ok {
			continue
		}
		seen[rule.Group] = struct{}{}
		out = append(out, rule.Group)
	}
	sort.Strings(out)
	return out
}

// Fingerprint — отпечаток содержимого каталога: он различает два состояния нормы.
// Вердикт, снятый на одном состоянии нормы, не следует сравнивать с вердиктом,
// снятым на другом, и отпечаток делает это различие видимым в самом выводе.
//
// В отпечаток входят коды и формулировки правил, но не места их объявления:
// перенос правила между документами нормы не меняет, а правка формулировки меняет.
func (c *Catalog) Fingerprint() string {
	codes := make([]string, 0, len(c.rules))
	for code := range c.byCode {
		codes = append(codes, code)
	}
	sort.Strings(codes)

	digest := sha256.New()
	for _, code := range codes {
		fmt.Fprintf(digest, "%s\x00%s\x00", code, c.byCode[code].Text)
	}
	return hex.EncodeToString(digest.Sum(nil))
}

// HasGroup отвечает, есть ли в каталоге хотя бы одно правило этой группы.
func (c *Catalog) HasGroup(group string) bool {
	for _, rule := range c.rules {
		if rule.Group == group {
			return true
		}
	}
	return false
}

// IsCode отвечает, является ли строка кодом правила по форме, независимо от того,
// объявлено ли такое правило нормой.
func IsCode(value string) bool {
	return codePattern.MatchString(value)
}
