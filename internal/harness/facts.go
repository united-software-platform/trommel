package harness

import (
	"fmt"
	"io/fs"
	"strings"

	"github.com/united-software-platform/trommel/internal/codefacts"
	"github.com/united-software-platform/trommel/internal/scope"
)

// Съём фактов о коде без прогона правил.
//
// Режим нужен там, где контракта намерения нет: чужой проект, который сканируют,
// чтобы увидеть, что о нём вообще известно анализатору. Уровни и карту в этом случае
// задаёт прогон, а не проверяемый проект, — брать их оттуда неоткуда.
//
// Это не вердикт и вердиктом не притворяется: правила не исполняются, состояний
// не выдаётся, а результат — срез и его граница.

// Facts — всё, что нужно съёму.
type Facts struct {
	// Project — содержимое проверяемого проекта, только для чтения.
	Project fs.FS
	// ProjectDir — каталог проверяемого проекта на файловой системе.
	ProjectDir string
	// Analyzer — имя или путь исполняемого файла анализатора кода.
	Analyzer string
	// AnalyzerWorkDir — рабочий каталог процесса анализатора, вне проверяемого проекта.
	AnalyzerWorkDir string
	// Levels — уровни съёма. Пустой перечень означает все известные уровни:
	// у съёма умолчание есть, потому что вопрос задаёт прогон, а не проект.
	Levels []string
	// ScanDirs, ExcludeDirs — карта проекта.
	ScanDirs    []string
	ExcludeDirs []string
}

// Collect снимает срез фактов о коде.
func Collect(facts Facts) (*codefacts.Slice, error) {
	levels, err := levelsOf(facts.Levels)
	if err != nil {
		return nil, err
	}

	slice, err := codefacts.Prepare(codefacts.Options{
		Executable: facts.Analyzer,
		ProjectDir: facts.ProjectDir,
		Tree:       facts.Project,
		WorkDir:    facts.AnalyzerWorkDir,
		Levels:     levels,
		Map:        scope.Map{Scan: facts.ScanDirs, Exclude: facts.ExcludeDirs},
	})
	if err != nil {
		return nil, fmt.Errorf("съём фактов о коде не состоялся: %w", err)
	}
	return slice, nil
}

// levelsOf разбирает перечень уровней съёма.
func levelsOf(names []string) ([]codefacts.Level, error) {
	if len(names) == 0 {
		return codefacts.Levels(), nil
	}

	var levels []codefacts.Level
	for _, name := range names {
		for _, part := range strings.Split(name, ",") {
			if part = strings.TrimSpace(part); part != "" {
				levels = append(levels, codefacts.Level(part))
			}
		}
	}
	if err := codefacts.CheckLevels(levels); err != nil {
		return nil, err
	}
	return levels, nil
}
