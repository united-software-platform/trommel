package docrules

import (
	"testing"

	"github.com/united-software-platform/trommel/internal/rules"
)

// Каждая реализация группы доказывает работоспособность своими образцами:
// срабатывает на нарушении и молчит на чистом содержимом.
func TestРеализацииПроходятСобственныеОбразцы(t *testing.T) {
	implementations := []rules.Implementation{
		HTML{}, Wikilink{}, IndentedCode{}, SetextHeading{},
		CodeLanguage{}, HeadingLevels{}, FileOpening{},
		RelativeLinks{}, VersionTable{}, VersionOrder{},
	}

	for _, implementation := range implementations {
		t.Run(implementation.Code(), func(t *testing.T) {
			if err := rules.Verify(implementation); err != nil {
				t.Errorf("образцы не пройдены: %v", err)
			}
		})
	}
}
