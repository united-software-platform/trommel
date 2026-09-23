package codefacts

import (
	"encoding/json"
	"fmt"
)

// Разбор ответов анализатора в модель фактов.
//
// Обязательные поля объявлены указателями: отсутствующее поле даёт nil, и разбор
// отказывает вместо того, чтобы выдать пустой срез. Отсутствие фактов и отсутствие
// разбора — разные исходы, и подменять второй первым нельзя: пустой срез выглядел бы
// как «в проекте ничего нет».

// parseStructure переносит ответ команды structure в модель: уровень L1.
func parseStructure(output []byte, into *Slice) error {
	var answer struct {
		Language *string `json:"language"`
		Files    *[]struct {
			Path        string   `json:"path"`
			Classes     []string `json:"classes"`
			MethodInfos []struct {
				Name      string `json:"name"`
				Signature string `json:"signature"`
				Line      int    `json:"line"`
				LineEnd   int    `json:"line_end"`
			} `json:"method_infos"`
			Imports []struct {
				Module string `json:"module"`
				IsFrom bool   `json:"is_from"`
			} `json:"imports"`
			Definitions []struct {
				Name      string `json:"name"`
				Kind      string `json:"kind"`
				Signature string `json:"signature"`
				LineStart int    `json:"line_start"`
			} `json:"definitions"`
		} `json:"files"`
		Warnings []string `json:"warnings"`
	}

	if err := decode("structure", output, &answer); err != nil {
		return err
	}
	if answer.Files == nil {
		return missing("structure", "files")
	}

	if answer.Language != nil {
		into.Language = *answer.Language
	}

	for _, file := range *answer.Files {
		module := Module{Path: file.Path, Types: file.Classes}
		for _, imported := range file.Imports {
			module.Imports = append(module.Imports,
				Import{Module: imported.Module, Selective: imported.IsFrom})
		}
		for _, method := range file.MethodInfos {
			module.Functions = append(module.Functions, Function{
				Name:      method.Name,
				Signature: method.Signature,
				Line:      method.Line,
				LineEnd:   method.LineEnd,
			})
		}
		for _, definition := range file.Definitions {
			module.Definitions = append(module.Definitions, Definition{
				Name:      definition.Name,
				Kind:      definition.Kind,
				Signature: definition.Signature,
				Line:      definition.LineStart,
			})
		}
		into.Modules = append(into.Modules, module)
	}

	// Предупреждение анализатора — не фон, а граница: «нет исходных файлов»
	// означает, что фактов нет вовсе, а не что их нет в природе.
	for _, warning := range answer.Warnings {
		into.Note("уровень L1", "анализатор предупреждает: "+warning)
	}
	return nil
}

// parseCalls переносит ответ команды calls в модель: уровень L2.
func parseCalls(output []byte, into *Slice) error {
	var answer struct {
		Edges *[]struct {
			SrcFile  string `json:"src_file"`
			SrcFunc  string `json:"src_func"`
			DstFile  string `json:"dst_file"`
			DstFunc  string `json:"dst_func"`
			CallType string `json:"call_type"`
		} `json:"edges"`
		Truncated  *bool `json:"truncated"`
		TotalEdges *int  `json:"total_edges"`
		ShownEdges *int  `json:"shown_edges"`
	}

	if err := decode("calls", output, &answer); err != nil {
		return err
	}
	switch {
	case answer.Edges == nil:
		return missing("calls", "edges")
	case answer.Truncated == nil:
		return missing("calls", "truncated")
	case answer.TotalEdges == nil || answer.ShownEdges == nil:
		return missing("calls", "total_edges, shown_edges")
	}

	for _, edge := range *answer.Edges {
		into.Calls = append(into.Calls, Call{
			FromFile: edge.SrcFile,
			FromFunc: edge.SrcFunc,
			ToFile:   edge.DstFile,
			ToFunc:   edge.DstFunc,
			Kind:     edge.CallType,
		})
	}

	// Усечение анализатор объявляет сам — и это единственная причина, по которой
	// неполный граф вызовов может пройти незамеченным. Признак переносится
	// в границу среза, а не в факт: части графа нет, и правило об этом узнает.
	if *answer.Truncated {
		into.Note("граф вызовов", fmt.Sprintf(
			"вывод анализатора усечён: показано %d рёбер из %d",
			*answer.ShownEdges, *answer.TotalEdges))
	}
	return nil
}

// parseDead переносит ответ команды dead в модель: уровень L2.
func parseDead(output []byte, into *Slice) error {
	type function struct {
		File string `json:"file"`
		Name string `json:"name"`
		Line int    `json:"line"`
	}
	var answer struct {
		DeadFunctions     *[]function `json:"dead_functions"`
		PossiblyDead      *[]function `json:"possibly_dead"`
		FunctionsAnalyzed *int        `json:"functions_analyzed"`
		TotalFunctions    *int        `json:"total_functions"`
	}

	if err := decode("dead", output, &answer); err != nil {
		return err
	}
	switch {
	case answer.DeadFunctions == nil || answer.PossiblyDead == nil:
		return missing("dead", "dead_functions, possibly_dead")
	case answer.FunctionsAnalyzed == nil || answer.TotalFunctions == nil:
		return missing("dead", "functions_analyzed, total_functions")
	}

	for _, found := range *answer.DeadFunctions {
		into.Unreachable = append(into.Unreachable, Unreachable{
			File: found.File, Name: found.Name, Line: found.Line, Certain: true,
		})
	}
	for _, found := range *answer.PossiblyDead {
		into.Unreachable = append(into.Unreachable, Unreachable{
			File: found.File, Name: found.Name, Line: found.Line, Certain: false,
		})
	}

	if *answer.FunctionsAnalyzed < *answer.TotalFunctions {
		into.Note("недостижимый код", fmt.Sprintf(
			"анализатор разобрал %d функций из %d",
			*answer.FunctionsAnalyzed, *answer.TotalFunctions))
	}
	return nil
}

// parseDeps переносит ответ команды deps в модель: граф зависимостей.
func parseDeps(output []byte, into *Slice) error {
	var answer struct {
		Internal *map[string][]string `json:"internal_dependencies"`
		Circular []struct {
			Path []string `json:"path"`
		} `json:"circular_dependencies"`
		Skipped *int `json:"files_skipped"`
	}

	if err := decode("deps", output, &answer); err != nil {
		return err
	}
	switch {
	case answer.Internal == nil:
		return missing("deps", "internal_dependencies")
	case answer.Skipped == nil:
		return missing("deps", "files_skipped")
	}

	for from, targets := range *answer.Internal {
		for _, to := range targets {
			into.Dependencies = append(into.Dependencies, Dependency{From: from, To: to})
		}
	}
	for _, cycle := range answer.Circular {
		into.Cycles = append(into.Cycles, Cycle{Path: cycle.Path})
	}

	if *answer.Skipped > 0 {
		into.Note("граф зависимостей", fmt.Sprintf(
			"анализатор пропустил файлов: %d", *answer.Skipped))
	}
	return nil
}

// decode разбирает ответ команды, сообщая её имя: иначе читатель ошибки не знает,
// на каком из обращений к анализатору прогон остановился.
func decode(command string, output []byte, into any) error {
	if err := json.Unmarshal(output, into); err != nil {
		return fmt.Errorf("ответ команды %s не разобран: %w", command, err)
	}
	return nil
}

// missing сообщает об отсутствии обязательного поля ответа.
func missing(command, field string) error {
	return fmt.Errorf(
		"ответ команды %s не содержит обязательного поля (%s): форма вывода анализатора "+
			"не та, на которую рассчитана сборка", command, field)
}
