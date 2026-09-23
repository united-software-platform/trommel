package scope

import "testing"

func TestПустаяКартаОхватываетПроектЦеликом(t *testing.T) {
	var карта Map

	for _, путь := range []string{"internal/rules/registry.go", "docs/guide.md", "Makefile"} {
		if !карта.Covers(путь) {
			t.Errorf("путь %q не вошёл в область пустой карты", путь)
		}
	}
	if roots := карта.Roots(); len(roots) != 1 || roots[0] != "." {
		t.Errorf("корни обхода пустой карты: %+v", roots)
	}
	if карта.Narrowed() {
		t.Errorf("пустая карта объявлена сужающей область")
	}
}

func TestКартаОграничиваетОбластьСканируемымиПутями(t *testing.T) {
	карта := Map{Scan: []string{"lib/Application"}}

	if !карта.Covers("lib/Application/FindByFilterUseCase.php") {
		t.Errorf("файл сканируемого каталога не вошёл в область")
	}
	if карта.Covers("www/index.php") {
		t.Errorf("файл вне сканируемых путей вошёл в область")
	}

	// Каталог верхнего уровня входит в обход: в нём лежит сканируемый подкаталог,
	// и пропустить его целиком значило бы не дойти до области карты.
	if !карта.Covers("lib") {
		t.Errorf("каталог, содержащий сканируемый путь, выведен из обхода")
	}
}

func TestИсключениеСильнееВключения(t *testing.T) {
	карта := Map{Scan: []string{"lib"}, Exclude: []string{"lib/Generated"}}

	if !карта.Covers("lib/Application/UseCase.php") {
		t.Errorf("файл сканируемого каталога не вошёл в область")
	}
	if карта.Covers("lib/Generated/Stub.php") {
		t.Errorf("исключённый подкаталог сканируемого пути вошёл в область")
	}
	if !карта.Narrowed() {
		t.Errorf("карта с путями объявлена не сужающей область")
	}
}

func TestПутиСравниваютсяНезависимоОтНаписания(t *testing.T) {
	карта := Map{Scan: []string{"/lib/", " www "}, Exclude: []string{"lib/vendor/"}}

	for _, путь := range []string{"lib/Application/UseCase.php", "www/index.php"} {
		if !карта.Covers(путь) {
			t.Errorf("путь %q не вошёл в область", путь)
		}
	}
	if карта.Covers("lib/vendor/lib.php") {
		t.Errorf("исключённый путь вошёл в область")
	}
	if roots := карта.Roots(); len(roots) != 2 || roots[0] != "lib" || roots[1] != "www" {
		t.Errorf("корни обхода: %+v", roots)
	}
}
