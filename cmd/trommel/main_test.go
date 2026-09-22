package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/united-software-platform/trommel/internal/verdict"
)

// Версия печатается в поток вывода, а не ошибок, и завершается успехом:
// вызывающая сторона читает версию из stdout наравне с вердиктом.
func TestRunПечатаетВерсию(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"--version"}, &stdout, &stderr)

	if code != verdict.ExitOK {
		t.Errorf("код возврата = %d, ожидался %d", code, verdict.ExitOK)
	}
	if !strings.Contains(stdout.String(), version) {
		t.Errorf("вывод %q не содержит версию %q", stdout.String(), version)
	}
	if stderr.Len() != 0 {
		t.Errorf("поток ошибок не пуст: %q", stderr.String())
	}
}

// Вызов без выбранного контракта — несостоявшийся прогон, а не отсутствие нарушений:
// коды этих исходов обязаны различаться.
func TestRunБезАргументовОтказывает(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run(nil, &stdout, &stderr)

	if code != verdict.ExitRefused {
		t.Errorf("код возврата = %d, ожидался %d", code, verdict.ExitRefused)
	}
	if stderr.Len() == 0 {
		t.Error("причина отказа не названа в потоке ошибок")
	}
}

// Нераспознанный флаг — ошибка вызова, а не повод начать прогон.
func TestRunНеизвестныйФлагОтказывает(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"--нет-такого-флага"}, &stdout, &stderr)

	if code != verdict.ExitRefused {
		t.Errorf("код возврата = %d, ожидался %d", code, verdict.ExitRefused)
	}
}
