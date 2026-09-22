package intent

import (
	"errors"
	"fmt"
	"io"
	"io/fs"

	"gopkg.in/yaml.v3"
)

// ErrNotFound возвращается, когда файла контрактов в проекте нет.
var ErrNotFound = errors.New("файл контрактов намерения не найден")

// Load читает файл контрактов проекта.
//
// Разбор строгий: неизвестное поле отвергается, а не игнорируется. Поле, которое
// сборка не понимает, означает, что контракт писался под другую версию Trommel,
// и молчаливое игнорирование дало бы вердикт по частично понятому намерению.
func Load(fsys fs.FS, name string) (*File, error) {
	file, err := fsys.Open(name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		return nil, fmt.Errorf("контракт %q недоступен: %w", name, err)
	}
	defer file.Close()

	return decode(name, file)
}

// decode разбирает содержимое файла контрактов и проверяет его версию.
func decode(name string, reader io.Reader) (*File, error) {
	decoder := yaml.NewDecoder(reader)
	decoder.KnownFields(true)

	var parsed File
	if err := decoder.Decode(&parsed); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("контракт %q пуст", name)
		}
		return nil, fmt.Errorf("контракт %q не разобран: %w", name, err)
	}

	if parsed.Version != Version {
		return nil, fmt.Errorf(
			"контракт %q объявляет версию %d, эта сборка поддерживает %d",
			name, parsed.Version, Version)
	}
	if len(parsed.Contracts) == 0 {
		return nil, fmt.Errorf("в %q не объявлено ни одного контракта", name)
	}

	return &parsed, nil
}
