package codefacts

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Запуск внешнего анализатора.
//
// Анализатор исполняется отдельным процессом: его код в Trommel не компонуется,
// а отказ инструмента становится отказом прогона там же, где Trommel отказывает
// при неисполнимом контракте.

// Executable — имя исполняемого файла анализатора по умолчанию.
const Executable = "tldr"

// supportedVersion — выпуск анализатора, на который рассчитана эта сборка.
//
// Номер версии анализатора начинается с нуля: состав команд и форма вывода меняются
// между выпусками без обещания совместимости. Поэтому сверяется не нижняя граница,
// а точное значение старшей и средней части: срез, собранный другим выпуском, имел бы
// неизвестный состав, и вердикт на нём ничего не значил бы.
const supportedVersion = "0.4"

// Runner — способ обратиться к анализатору.
type Runner struct {
	// Executable — имя или путь исполняемого файла анализатора.
	Executable string
	// ProjectDir — каталог проверяемого проекта на файловой системе.
	ProjectDir string
	// WorkDir — рабочий каталог процесса анализатора. Намеренно не совпадает
	// с каталогом проверяемого проекта: проект остаётся доступным только на чтение,
	// и промежуточным данным инструмента в нём места нет.
	WorkDir string
}

// Version возвращает версию анализатора, объявленную им самим.
func (r Runner) Version() (string, error) {
	output, err := r.run("--version")
	if err != nil {
		return "", err
	}

	// Ответ вида "tldr 0.4.0": имя и версия через пробел.
	fields := strings.Fields(strings.TrimSpace(string(output)))
	if len(fields) < 2 {
		return "", fmt.Errorf(
			"анализатор %s не назвал версию: %q", r.Executable, strings.TrimSpace(string(output)))
	}
	return fields[len(fields)-1], nil
}

// CheckVersion сверяет версию анализатора с той, на которую рассчитана сборка.
func CheckVersion(version string) error {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 || parts[0]+"."+parts[1] != supportedVersion {
		return fmt.Errorf(
			"версия анализатора %s вне поддерживаемого сборкой диапазона %s.x: "+
				"состав команд и форма вывода между выпусками не совместимы",
			version, supportedVersion)
	}
	return nil
}

// Collect выполняет команду анализатора по дереву: цель — сканируемый путь карты.
func (r Runner) Collect(command, target string) ([]byte, error) {
	return r.run(command, target, "--format", "json", "--quiet")
}

// Ask выполняет команду анализатора о теле функции: цель задают аргументы команды —
// файл, имя функции и, если команда того требует, строка.
func (r Runner) Ask(command string, args ...string) ([]byte, error) {
	return r.run(append(append([]string{command}, args...), "--format", "json", "--quiet")...)
}

// run исполняет анализатор и возвращает его вывод.
//
// Неуспех — это отказ прогона, а не пустой результат: код возврата и текст ошибки
// переносятся вызывающей стороне целиком, чтобы причина была видна в вердикте.
func (r Runner) run(args ...string) ([]byte, error) {
	executable := r.Executable
	if executable == "" {
		executable = Executable
	}

	command := exec.Command(executable, args...)
	command.Dir = r.WorkDir

	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("%s %s: код возврата %d: %s",
				executable, strings.Join(args, " "), exitErr.ExitCode(),
				strings.TrimSpace(stderr.String()))
		}
		return nil, fmt.Errorf("%s %s: %w", executable, strings.Join(args, " "), err)
	}

	return stdout.Bytes(), nil
}
