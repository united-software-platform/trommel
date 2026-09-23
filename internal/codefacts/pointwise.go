package codefacts

import "sort"

// Разбор ответов о теле одной функции: уровни L3, L4 и L5.
//
// Из вывода берётся то, что выражается в терминах нормы: переменная и строка,
// выражение и строка, строка среза и род зависимости. Блоки, имена SSA, номера
// базовых блоков и прочее устройство анализатора в модель не переносятся — правило
// рассуждает о коде, а не о внутреннем представлении инструмента.

// parseReaching переносит ответ команды reaching-defs: достигающие определения.
func parseReaching(output []byte, into *FunctionFacts) error {
	var answer struct {
		Blocks *[]struct {
			Gen []struct {
				Var  string `json:"var"`
				Line int    `json:"line"`
			} `json:"gen"`
		} `json:"blocks"`
	}

	if err := decode("reaching-defs", output, &answer); err != nil {
		return err
	}
	if answer.Blocks == nil {
		return missing("reaching-defs", "blocks")
	}

	seen := make(map[VariableDef]struct{})
	for _, block := range *answer.Blocks {
		for _, def := range block.Gen {
			seen[VariableDef{Var: def.Var, Line: def.Line}] = struct{}{}
		}
	}
	into.Reaching = sortedDefs(seen)
	return nil
}

// parseAvailable переносит ответ команды available: доступные выражения.
func parseAvailable(output []byte, into *FunctionFacts) error {
	var answer struct {
		AvailIn *map[string][]struct {
			Text string `json:"text"`
			Line int    `json:"line"`
		} `json:"avail_in"`
	}

	if err := decode("available", output, &answer); err != nil {
		return err
	}
	if answer.AvailIn == nil {
		return missing("available", "avail_in")
	}

	seen := make(map[Expression]struct{})
	for _, expressions := range *answer.AvailIn {
		for _, expression := range expressions {
			seen[Expression{Text: expression.Text, Line: expression.Line}] = struct{}{}
		}
	}

	out := make([]Expression, 0, len(seen))
	for expression := range seen {
		out = append(out, expression)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Text < out[j].Text
	})
	if len(out) == 0 {
		return nil
	}
	into.Available = out
	return nil
}

// parseDeadStores переносит ответ команды dead-stores: мёртвые присваивания.
func parseDeadStores(output []byte, into *FunctionFacts) error {
	var answer struct {
		Stores *[]struct {
			Variable string `json:"variable"`
			Line     int    `json:"line"`
		} `json:"dead_stores_ssa"`
	}

	if err := decode("dead-stores", output, &answer); err != nil {
		return err
	}
	if answer.Stores == nil {
		return missing("dead-stores", "dead_stores_ssa")
	}

	seen := make(map[VariableDef]struct{})
	for _, store := range *answer.Stores {
		seen[VariableDef{Var: store.Variable, Line: store.Line}] = struct{}{}
	}
	into.DeadStores = sortedDefs(seen)
	return nil
}

// parseSlice переносит ответ команды slice: строки среза программы.
func parseSlice(output []byte, into *FunctionFacts) error {
	var answer struct {
		Lines *[]struct {
			Line    int    `json:"line"`
			DepType string `json:"dep_type"`
		} `json:"slice_lines"`
	}

	if err := decode("slice", output, &answer); err != nil {
		return err
	}
	if answer.Lines == nil {
		return missing("slice", "slice_lines")
	}

	for _, line := range *answer.Lines {
		into.Slice = append(into.Slice, SliceLine{Line: line.Line, Dep: line.DepType})
	}
	return nil
}

// sortedDefs выкладывает присваивания в устойчивом порядке: срез обязан совпадать
// от прогона к прогону, а обход отображения порядка не даёт.
func sortedDefs(seen map[VariableDef]struct{}) []VariableDef {
	if len(seen) == 0 {
		return nil
	}

	out := make([]VariableDef, 0, len(seen))
	for def := range seen {
		out = append(out, def)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Var < out[j].Var
	})
	return out
}
