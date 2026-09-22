package verdict

import "fmt"

// Finding — одно обнаруженное нарушение правила.
type Finding struct {
	// File — где найдено нарушение.
	File string `json:"file"`
	// Line — строка, если применимо.
	Line int `json:"line,omitempty"`
	// Message — в чём нарушение.
	Message string `json:"message"`
}

// String даёт представление нарушения для вывода человеку.
func (f Finding) String() string {
	if f.Line > 0 {
		return fmt.Sprintf("%s:%d  %s", f.File, f.Line, f.Message)
	}
	return fmt.Sprintf("%s  %s", f.File, f.Message)
}

// Result — итог работы одной реализации правила.
//
// Инвариант статуса ok обеспечивается здесь, а не дисциплиной реализаций:
// пока перечень непросмотренного не пуст, состояние ok недостижимо, какими бы
// ни были находки. Реализация сообщает, что она просмотрела и чего не смогла,
// а состояние выводится из этого.
type Result struct {
	// Examined — число просмотренных единиц содержимого.
	Examined int
	// Unexamined — единицы, которые следовало просмотреть и не удалось.
	Unexamined []string
	// Findings — обнаруженные нарушения.
	Findings []Finding
	// Inapplicable — правило к проекту неприменимо; причина в Detail.
	Inapplicable bool
	// Detail — краткое пояснение для строки вывода.
	Detail string
}

// Status выводит состояние правила из итога работы реализации.
func (r Result) Status() Status {
	switch {
	case r.Inapplicable:
		return StatusSkip
	case len(r.Unexamined) > 0:
		return StatusErr
	case len(r.Findings) > 0:
		return StatusFail
	default:
		return StatusOK
	}
}
