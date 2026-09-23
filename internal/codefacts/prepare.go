package codefacts

import (
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strconv"

	"github.com/united-software-platform/trommel/internal/scope"
)

// Prepare собирает срез фактов о коде — фаза подготовки прогона.
//
// Порядок работы повторяет порядок прогона: сначала проверяется исполнимость
// намерения — объявленные уровни и версия инструмента, — и только потом Trommel
// обращается к содержимому проверяемого проекта. Отказ на любом из первых шагов
// означает несостоявшуюся подготовку: срез не выдаётся, а вызывающая сторона
// не вправе считать отсутствие фактов отсутствием нарушений.
type Options struct {
	// Executable — имя или путь исполняемого файла анализатора.
	Executable string
	// ProjectDir — каталог проверяемого проекта на файловой системе.
	ProjectDir string
	// Tree — то же дерево, только для чтения: по нему считается граница среза.
	Tree fs.FS
	// WorkDir — рабочий каталог процесса анализатора, вне проверяемого проекта.
	WorkDir string
	// Levels — состав среза, объявленный контрактом намерения.
	Levels []Level
	// Map — карта проекта: что сканируется и что не сканируется.
	Map scope.Map
}

// Prepare выполняет фазу подготовки и возвращает срез фактов о коде.
func Prepare(options Options) (*Slice, error) {
	if len(options.Levels) == 0 {
		return nil, fmt.Errorf("состав среза фактов о коде не объявлен")
	}
	if options.ProjectDir == "" {
		return nil, fmt.Errorf("каталог проверяемого проекта не передан")
	}
	if err := CheckLevels(options.Levels); err != nil {
		return nil, err
	}

	runner := Runner{
		Executable: options.Executable,
		ProjectDir: options.ProjectDir,
		WorkDir:    options.WorkDir,
	}

	version, err := runner.Version()
	if err != nil {
		return nil, fmt.Errorf("анализатор кода недоступен: %w", err)
	}
	if err := CheckVersion(version); err != nil {
		return nil, err
	}

	name := options.Executable
	if name == "" {
		name = Executable
	}

	slice := &Slice{
		Analyzer: Analyzer{Name: name, Version: version},
		Declared: options.Levels,
		Scanned:  options.Map.Scan,
		Excluded: options.Map.Exclude,
	}

	if err := collectProject(slice, runner, options); err != nil {
		return nil, err
	}
	restrict(slice, options.Map)
	if err := collectFunctions(slice, runner, options); err != nil {
		return nil, err
	}
	note(slice, options.Tree, options.Map)

	return slice, nil
}

// CheckLevels проверяет состав уровней до обращения к анализатору.
//
// Уровень функций без уровня структуры неисполним: перечень функций берётся именно
// из структуры, и без неё обходить нечего. Это ошибка контракта, а не неудача прогона,
// поэтому она обнаруживается здесь, до запуска инструмента.
func CheckLevels(levels []Level) error {
	declared := make(map[Level]struct{}, len(levels))
	for _, level := range levels {
		if err := Validate(level); err != nil {
			return err
		}
		declared[level] = struct{}{}
	}

	if _, structure := declared[LevelAST]; structure {
		return nil
	}
	for level := range declared {
		if ByFunction(level) {
			return fmt.Errorf(
				"уровень %s снимается обходом функций, а перечень функций даёт уровень %s, "+
					"не объявленный контрактом", level, LevelAST)
		}
	}
	return nil
}

// collectProject снимает уровни, обходящие дерево, по каждому пути карты.
//
// Обращение к анализатору идёт по сканируемым путям, а не по корню проекта: обход
// стоит ровно столько, сколько в карте названо. Пути ответа приводятся к корню проекта,
// поэтому срез не зависит от того, за сколько обращений он собран.
func collectProject(slice *Slice, runner Runner, options Options) error {
	for _, level := range options.Levels {
		commands := ProjectCommands(level)
		if len(commands) == 0 {
			continue
		}

		for _, root := range options.Map.Roots() {
			target := options.ProjectDir
			prefix := ""
			if root != "." {
				target = filepath.Join(options.ProjectDir, root)
				prefix = root
			}

			for _, command := range commands {
				output, err := runner.Collect(command.name, target)
				if err != nil {
					return fmt.Errorf("уровень %s не собран: %w", level, err)
				}

				part := &Slice{}
				if err := command.parse(output, part); err != nil {
					return fmt.Errorf("уровень %s не собран: %w", level, err)
				}
				merge(slice, rebase(part, prefix))
			}
		}
		slice.Collect(level)
	}
	return nil
}

// collectFunctions снимает уровни, обходящие функции области карты.
func collectFunctions(slice *Slice, runner Runner, options Options) error {
	var commands []functionCommand
	var levels []Level
	for _, level := range options.Levels {
		if found := FunctionCommands(level); len(found) > 0 {
			commands = append(commands, found...)
			levels = append(levels, level)
		}
	}
	if len(commands) == 0 {
		return nil
	}

	for _, module := range slice.Modules {
		file := filepath.Join(options.ProjectDir, module.Path)

		for _, function := range module.Functions {
			facts := FunctionFacts{File: module.Path, Name: function.Name, Line: function.Line}

			for _, command := range commands {
				args := []string{file, function.Name}
				if command.line {
					args = append(args, strconv.Itoa(function.Line))
				}

				output, err := runner.Ask(command.name, args...)
				if err != nil {
					return fmt.Errorf("факты функции %s (%s) не собраны: %w",
						function.Name, module.Path, err)
				}
				if err := command.parse(output, &facts); err != nil {
					return fmt.Errorf("факты функции %s (%s) не собраны: %w",
						function.Name, module.Path, err)
				}
			}
			slice.Functions = append(slice.Functions, facts)
		}
	}

	for _, level := range levels {
		slice.Collect(level)
	}
	return nil
}

// rebase приводит пути части среза к корню проекта.
func rebase(part *Slice, prefix string) *Slice {
	if prefix == "" {
		return part
	}
	at := func(p string) string {
		if p == "" {
			return p
		}
		return path.Join(prefix, p)
	}

	for i := range part.Modules {
		part.Modules[i].Path = at(part.Modules[i].Path)
	}
	for i := range part.Calls {
		part.Calls[i].FromFile = at(part.Calls[i].FromFile)
		part.Calls[i].ToFile = at(part.Calls[i].ToFile)
	}
	for i := range part.Unreachable {
		part.Unreachable[i].File = at(part.Unreachable[i].File)
	}
	for i := range part.Dependencies {
		part.Dependencies[i].From = at(part.Dependencies[i].From)
		part.Dependencies[i].To = at(part.Dependencies[i].To)
	}
	for i := range part.Cycles {
		for j := range part.Cycles[i].Path {
			part.Cycles[i].Path[j] = at(part.Cycles[i].Path[j])
		}
	}
	return part
}

// merge присоединяет часть среза к целому.
func merge(slice, part *Slice) {
	if slice.Language == "" {
		slice.Language = part.Language
	}
	slice.Modules = append(slice.Modules, part.Modules...)
	slice.Calls = append(slice.Calls, part.Calls...)
	slice.Unreachable = append(slice.Unreachable, part.Unreachable...)
	slice.Dependencies = append(slice.Dependencies, part.Dependencies...)
	slice.Cycles = append(slice.Cycles, part.Cycles...)
	slice.Unexamined = append(slice.Unexamined, part.Unexamined...)
}
