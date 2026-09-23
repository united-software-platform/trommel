package codefacts

import (
	"fmt"
	"sort"
	"strings"
)

// Level — уровень разбора анализатора.
//
// Имена уровней взяты у анализатора, а не придуманы заново: контракт намерения
// объявляет то же, что названо в его справке, и читатель контракта не обязан
// держать в голове ещё один словарь.
type Level string

const (
	// LevelAST — L1: структура файла — функции, типы, импорты.
	LevelAST Level = "L1"
	// LevelCalls — L2: вызовы между функциями и недостижимый код.
	LevelCalls Level = "L2"
	// LevelCFG — L3: достигающие определения и доступные выражения.
	LevelCFG Level = "L3"
	// LevelDFG — L4: присваивания, результат которых не используется.
	LevelDFG Level = "L4"
	// LevelPDG — L5: срезы программы.
	LevelPDG Level = "L5"
	// LevelDeps — граф зависимостей между файлами проекта.
	LevelDeps Level = "deps"
)

// projectCommand — обращение к анализатору о дереве: одно на сканируемый путь.
type projectCommand struct {
	name  string
	parse func(output []byte, into *Slice) error
}

// functionCommand — обращение к анализатору о теле функции: одно на функцию.
//
// Такие уровни и делают карту проекта обязательной: число обращений растёт с числом
// функций области, и область — единственное, чем это число ограничивается.
type functionCommand struct {
	name string
	// line — команде нужна строка, с которой начинается запрос.
	line  bool
	parse func(output []byte, into *FunctionFacts) error
}

// byProject — уровни, снимаемые обходом дерева.
var byProject = map[Level][]projectCommand{
	LevelAST:   {{name: "structure", parse: parseStructure}},
	LevelCalls: {{name: "calls", parse: parseCalls}, {name: "dead", parse: parseDead}},
	LevelDeps:  {{name: "deps", parse: parseDeps}},
}

// byFunction — уровни, снимаемые обходом функций.
var byFunction = map[Level][]functionCommand{
	LevelCFG: {
		{name: "reaching-defs", parse: parseReaching},
		{name: "available", parse: parseAvailable},
	},
	LevelDFG: {{name: "dead-stores", parse: parseDeadStores}},
	LevelPDG: {{name: "slice", line: true, parse: parseSlice}},
}

// ProjectCommands возвращает обращения уровня о дереве.
func ProjectCommands(level Level) []projectCommand { return byProject[level] }

// FunctionCommands возвращает обращения уровня о теле функции.
func FunctionCommands(level Level) []functionCommand { return byFunction[level] }

// ByFunction отвечает, снимается ли уровень обходом функций.
func ByFunction(level Level) bool {
	_, ok := byFunction[level]
	return ok
}

// Known отвечает, известен ли уровень.
func Known(level Level) bool {
	_, project := byProject[level]
	_, function := byFunction[level]
	return project || function
}

// Levels возвращает все известные уровни в устойчивом порядке.
//
// Порядок значим: уровень структуры снимается раньше уровней функций, потому что
// перечень функций берётся именно из него.
func Levels() []Level {
	out := make([]Level, 0, len(byProject)+len(byFunction))
	for level := range byProject {
		out = append(out, level)
	}
	for level := range byFunction {
		out = append(out, level)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Validate проверяет, что уровень известен анализатору этой сборки.
func Validate(level Level) error {
	if Known(level) {
		return nil
	}
	return fmt.Errorf("неизвестный уровень среза %s: известны %s", level, join(Levels()))
}

// join перечисляет уровни для сообщения об ошибке.
func join(levels []Level) string {
	parts := make([]string, 0, len(levels))
	for _, level := range levels {
		parts = append(parts, string(level))
	}
	return strings.Join(parts, ", ")
}
