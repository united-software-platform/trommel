package rules

import (
	"fmt"
	"testing/fstest"
)

// Sample — образец содержимого для самопроверки реализации правила.
type Sample struct {
	// Name — имя файла, под которым образец попадает в подставное дерево.
	Name string
	// Content — содержимое образца.
	Content string
}

// Samples — два набора образцов реализации правила.
//
// Наличие обоих наборов обязательно: реализация, о которой неизвестно, срабатывает ли
// она на нарушении и молчит ли на чистом содержимом, не может утверждать соблюдение
// правила. Наборы исполняются при каждом прогоне, а не только в тестах: вердикт
// опирается на доказанную работоспособность реализации, а не на её наличие.
type Samples struct {
	// Violating — образцы, на которых реализация обязана сработать.
	Violating []Sample
	// Clean — образцы, на которых реализация обязана смолчать.
	Clean []Sample
}

// Verify исполняет образцы реализации и возвращает ошибку, если реализация ведёт себя
// не так, как заявляет, либо не предъявила оба набора.
func Verify(impl Implementation) error {
	samples := impl.Samples()

	if len(samples.Violating) == 0 || len(samples.Clean) == 0 {
		return fmt.Errorf(
			"реализация правила %s не предъявила оба набора образцов", impl.Code())
	}

	for _, sample := range samples.Violating {
		result := impl.Check(sampleContext(sample))
		if len(result.Findings) == 0 {
			return fmt.Errorf(
				"реализация правила %s смолчала на образце нарушения %q",
				impl.Code(), sample.Name)
		}
	}

	for _, sample := range samples.Clean {
		result := impl.Check(sampleContext(sample))
		if len(result.Findings) > 0 {
			return fmt.Errorf(
				"реализация правила %s сработала на чистом образце %q: %s",
				impl.Code(), sample.Name, result.Findings[0].Message)
		}
	}

	return nil
}

// sampleContext собирает подставное дерево из одного образца.
func sampleContext(sample Sample) Context {
	return Context{
		Tree: fstest.MapFS{sample.Name: &fstest.MapFile{Data: []byte(sample.Content)}},
	}
}
