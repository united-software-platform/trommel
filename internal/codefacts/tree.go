package codefacts

import (
	"io/fs"
	"path"
	"strings"

	"github.com/united-software-platform/trommel/internal/scope"
)

// Граница среза со стороны дерева файлов.
//
// Анализатор сообщает, что он разобрал; он не сообщает, чего не увидел. Разницу
// Trommel считает сам: файл с исходным кодом, о котором фактов не пришло, попадает
// в границу среза. Без этого прогон на проекте, язык которого анализатору незнаком,
// выглядел бы как прогон на проекте без нарушений.

// supported — расширения языков, которые анализатор разбирает. Перечень его,
// а не наш: файл с таким расширением обязан был попасть в срез, и его отсутствие
// означает не «другой язык», а неразобранный файл.
var supported = map[string]string{
	".py": "Python", ".pyi": "Python",
	".ts": "TypeScript", ".tsx": "TypeScript",
	".js": "JavaScript", ".jsx": "JavaScript", ".mjs": "JavaScript", ".cjs": "JavaScript",
	".go": "Go", ".rs": "Rust", ".java": "Java",
	".c": "C", ".h": "C", ".cc": "C++", ".cpp": "C++", ".cxx": "C++",
	".hpp": "C++", ".hh": "C++",
	".rb": "Ruby", ".kt": "Kotlin", ".kts": "Kotlin", ".swift": "Swift",
	".cs": "C#", ".scala": "Scala", ".sc": "Scala", ".php": "PHP",
	".lua": "Lua", ".luau": "Luau", ".ex": "Elixir", ".exs": "Elixir",
	".ml": "OCaml", ".mli": "OCaml",
}

// foreign — расширения исходного кода, которых анализатор не разбирает. Перечень
// открыт по устройству: языков больше, чем можно перечислить. Отсутствие расширения
// в обоих перечнях означает, что Trommel не считает файл исходным кодом, — и это
// тоже утверждение, за которое он отвечает.
var foreign = map[string]string{
	".sh": "Shell", ".bash": "Shell", ".zsh": "Shell", ".ps1": "PowerShell",
	".pl": "Perl", ".pm": "Perl", ".r": "R", ".jl": "Julia", ".hs": "Haskell",
	".dart": "Dart", ".groovy": "Groovy", ".m": "Objective-C", ".mm": "Objective-C++",
	".vb": "Visual Basic", ".fs": "F#", ".clj": "Clojure", ".erl": "Erlang",
	".zig": "Zig", ".nim": "Nim", ".sql": "SQL", ".pas": "Pascal", ".d": "D",
	".f90": "Fortran", ".cob": "COBOL", ".asm": "Assembler", ".s": "Assembler",
}

// restrict убирает из среза факты вне области карты.
//
// Сужение применяется после разбора: собственного ключа исключения у анализатора нет,
// и вердикт не должен зависеть от того, поддержит ли инструмент такой ключ в следующей
// версии. Сканируемые пути карты при этом уже переданы ему целью команды — карта
// действует и до обращения, и после него.
func restrict(slice *Slice, area scope.Map) {
	if !area.Narrowed() {
		return
	}
	drop := func(p string) bool { return !area.Covers(p) }

	slice.Modules = filter(slice.Modules, func(m Module) bool { return !drop(m.Path) })
	slice.Calls = filter(slice.Calls, func(c Call) bool {
		return !drop(c.FromFile) && !drop(c.ToFile)
	})
	slice.Unreachable = filter(slice.Unreachable, func(u Unreachable) bool {
		return !drop(u.File)
	})
	slice.Dependencies = filter(slice.Dependencies, func(d Dependency) bool {
		return !drop(d.From) && !drop(d.To)
	})
	slice.Cycles = filter(slice.Cycles, func(c Cycle) bool {
		for _, file := range c.Path {
			if drop(file) {
				return false
			}
		}
		return true
	})
}

// note переносит в границу среза файлы дерева, о которых фактов не пришло.
//
// Граница считается по области карты: файл, о котором не спрашивали, не является
// непросмотренным — он просто вне области, и объявлять его пробелом значило бы
// требовать фактов обо всём на свете.
func note(slice *Slice, tree fs.FS, area scope.Map) {
	if tree == nil {
		slice.Note("дерево файлов", "дерево проверяемого проекта не передано")
		return
	}

	examined := make(map[string]struct{}, len(slice.Modules))
	for _, module := range slice.Modules {
		examined[module.Path] = struct{}{}
	}

	err := fs.WalkDir(tree, ".", func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			slice.Note(p, "содержимое не прочитано: "+err.Error())
			return nil
		}

		// Скрытые каталоги в обход не входят — та же граница, что и у правил
		// документов: служебное содержимое проекту не принадлежит.
		if entry.IsDir() {
			if p == "." {
				return nil
			}
			if strings.HasPrefix(entry.Name(), ".") || !area.Covers(p) {
				return fs.SkipDir
			}
			return nil
		}
		if !area.Covers(p) {
			return nil
		}
		if _, ok := examined[p]; ok {
			return nil
		}

		extension := strings.ToLower(path.Ext(p))
		if language, ok := supported[extension]; ok {
			slice.Note(p, "файл на языке "+language+" анализатором не разобран")
			return nil
		}
		if language, ok := foreign[extension]; ok {
			slice.Note(p, "язык "+language+" анализатором не поддержан")
		}
		return nil
	})
	if err != nil {
		slice.Note("дерево файлов", "обход прерван: "+err.Error())
	}
}

// filter оставляет элементы, прошедшие условие.
func filter[T any](items []T, keep func(T) bool) []T {
	out := items[:0]
	for _, item := range items {
		if keep(item) {
			out = append(out, item)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
