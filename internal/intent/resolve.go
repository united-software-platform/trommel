package intent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/united-software-platform/trommel/internal/catalog"
)

// Requirements сообщает, каких параметров требует реализация правила. Контракт
// проверяется против этих требований до прогона: правило с незаданным обязательным
// параметром исполняться не может.
type Requirements interface {
	// RequiredParams возвращает имена обязательных параметров правила.
	RequiredParams(code string) []string
}

// ExcludedRule — правило, выведенное из состава контрактом, вместе с обоснованием.
type ExcludedRule struct {
	Rule   catalog.Rule
	Reason string
}

// Composition — состав прогона: что проверяется и что выведено из состава.
type Composition struct {
	Contract string
	Rules    []catalog.Rule
	Excluded []ExcludedRule
	Params   map[string]Params
}

// Resolve сверяет контракт с нормой и вычисляет состав прогона.
//
// Проверки выполняются до обращения к содержимому проекта и прекращают работу при
// первом же расхождении: контракт, выражающий намерение неточно, не должен дать
// вердикт, который вызывающая сторона примет за полный.
func (c Contract) Resolve(norm *catalog.Catalog, requirements Requirements) (*Composition, error) {
	excluded, err := c.resolveExclusions(norm)
	if err != nil {
		return nil, err
	}

	if err := c.checkAcknowledged(norm); err != nil {
		return nil, err
	}
	if err := c.checkParamTargets(norm); err != nil {
		return nil, err
	}

	composition := &Composition{Contract: c.Name, Params: c.Params}
	for _, rule := range norm.Rules() {
		if reason, out := excluded[rule.Code]; out {
			composition.Excluded = append(composition.Excluded,
				ExcludedRule{Rule: rule, Reason: reason})
			continue
		}
		composition.Rules = append(composition.Rules, rule)
	}

	if err := c.checkStrict(norm, excluded); err != nil {
		return nil, err
	}
	if err := c.checkRequiredParams(composition.Rules, requirements); err != nil {
		return nil, err
	}

	return composition, nil
}

// resolveExclusions разворачивает исключения в перечень кодов с обоснованиями.
func (c Contract) resolveExclusions(norm *catalog.Catalog) (map[string]string, error) {
	excluded := make(map[string]string)

	for _, exclusion := range c.Exclude {
		target := strings.TrimSpace(exclusion.Rule)
		if target == "" {
			return nil, fmt.Errorf("контракт %q: исключение без указания правила", c.Name)
		}
		if strings.TrimSpace(exclusion.Reason) == "" {
			return nil, fmt.Errorf(
				"контракт %q: исключение %s без обоснования", c.Name, target)
		}

		codes, err := c.expand(norm, target)
		if err != nil {
			return nil, err
		}
		for _, code := range codes {
			excluded[code] = exclusion.Reason
		}
	}
	return excluded, nil
}

// expand разворачивает код или группу в перечень кодов правил нормы.
func (c Contract) expand(norm *catalog.Catalog, target string) ([]string, error) {
	// Дефис отличает код правила от имени группы. Проверка формата отделена от проверки
	// наличия намеренно: опечатка в коде и ссылка на удалённое правило — разные ошибки,
	// и сообщение о «несуществующей группе» увело бы читателя не туда.
	if strings.Contains(target, "-") {
		if !catalog.IsCode(target) {
			return nil, fmt.Errorf(
				"контракт %q: %s не является кодом правила формата {ПРЕФИКС}-{NNN}",
				c.Name, target)
		}
		if _, declared := norm.Rule(target); !declared {
			return nil, fmt.Errorf(
				"контракт %q: правила %s нет в норме", c.Name, target)
		}
		return []string{target}, nil
	}

	if !norm.HasGroup(target) {
		return nil, fmt.Errorf(
			"контракт %q: группы %s нет в норме; группы нормы: %s",
			c.Name, target, strings.Join(norm.Groups(), ", "))
	}

	rules := norm.Group(target)
	codes := make([]string, 0, len(rules))
	for _, rule := range rules {
		codes = append(codes, rule.Code)
	}
	return codes, nil
}

// checkAcknowledged проверяет, что подтверждённые коды и группы существуют в норме.
func (c Contract) checkAcknowledged(norm *catalog.Catalog) error {
	for _, target := range c.Acknowledge {
		if _, err := c.expand(norm, strings.TrimSpace(target)); err != nil {
			return err
		}
	}
	return nil
}

// checkParamTargets проверяет, что параметры заданы существующим правилам.
func (c Contract) checkParamTargets(norm *catalog.Catalog) error {
	codes := make([]string, 0, len(c.Params))
	for code := range c.Params {
		codes = append(codes, code)
	}
	sort.Strings(codes)

	for _, code := range codes {
		if _, declared := norm.Rule(code); !declared {
			return fmt.Errorf(
				"контракт %q: параметры заданы правилу %s, которого нет в норме", c.Name, code)
		}
	}
	return nil
}

// checkStrict требует, чтобы в исчерпывающем режиме каждое правило нормы было названо.
func (c Contract) checkStrict(norm *catalog.Catalog, excluded map[string]string) error {
	if !c.Strict {
		return nil
	}

	acknowledged := make(map[string]struct{})
	for _, target := range c.Acknowledge {
		codes, err := c.expand(norm, strings.TrimSpace(target))
		if err != nil {
			return err
		}
		for _, code := range codes {
			acknowledged[code] = struct{}{}
		}
	}

	var missing []string
	for _, rule := range norm.Rules() {
		if _, out := excluded[rule.Code]; out {
			continue
		}
		if _, known := acknowledged[rule.Code]; !known {
			missing = append(missing, rule.Code)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf(
			"контракт %q объявлен исчерпывающим, но не учитывает правила: %s",
			c.Name, strings.Join(missing, ", "))
	}
	return nil
}

// checkRequiredParams проверяет, что каждому правилу состава заданы обязательные
// параметры его реализации.
func (c Contract) checkRequiredParams(rules []catalog.Rule, requirements Requirements) error {
	if requirements == nil {
		return nil
	}

	for _, rule := range rules {
		for _, param := range requirements.RequiredParams(rule.Code) {
			if value, ok := c.Params[rule.Code][param]; !ok || strings.TrimSpace(value) == "" {
				return fmt.Errorf(
					"контракт %q: правилу %s не задан обязательный параметр %q",
					c.Name, rule.Code, param)
			}
		}
	}
	return nil
}
