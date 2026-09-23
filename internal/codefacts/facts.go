// Пакет codefacts собирает факты о коде проверяемого проекта внешним анализатором.
//
// Модель фактов принадлежит Trommel, а не анализатору: правило получает модуль,
// вызов и зависимость в терминах нормы, а не поля JSON конкретного инструмента.
// Смена формы вывода анализатора ломает разбор в одном месте, а не все правила сразу.
//
// Вторая обязанность пакета — граница среза. Факт, которого анализатор не дал,
// не исчезает: он становится записью о непросмотренном, и правило, опиравшееся
// на это содержимое, не может выдать ok.
package codefacts

// Analyzer — инструмент, собравший срез, и его версия. Попадает в вердикт:
// без этих двух значений повторный прогон не с чем сравнивать.
type Analyzer struct {
	// Name — имя исполняемого файла анализатора.
	Name string `json:"name"`
	// Version — версия, объявленная самим анализатором.
	Version string `json:"version"`
}

// Slice — срез фактов о коде: то, что собрано, и то, что собрать не удалось.
type Slice struct {
	// Analyzer — чем собран срез.
	Analyzer Analyzer `json:"analyzer"`
	// Declared — уровни, объявленные контрактом намерения.
	Declared []Level `json:"declared"`
	// Scanned, Excluded — карта проекта, по которой снят срез. Пустой перечень
	// сканируемых путей означает проект целиком.
	Scanned  []string `json:"scanned,omitempty"`
	Excluded []string `json:"excluded,omitempty"`
	// Collected — уровни, собранные в этом прогоне. Уровень, собранный и не давший
	// фактов, остаётся здесь: пустой уровень отличается от несобранного.
	Collected []Level `json:"collected"`
	// Language — язык, которым анализатор счёл проект. Пустое значение означает,
	// что язык не определён — фактов в срезе нет.
	Language string `json:"language,omitempty"`
	// Modules — факты уровня L1: файл и то, что в нём объявлено.
	Modules []Module `json:"modules,omitempty"`
	// Calls — факты уровня L2: вызовы между функциями.
	Calls []Call `json:"calls,omitempty"`
	// Unreachable — факты уровня L2: функции, до которых не доходит ни один вызов.
	Unreachable []Unreachable `json:"unreachable,omitempty"`
	// Functions — факты уровней L3..L5: по одной записи на функцию области карты.
	Functions []FunctionFacts `json:"functions,omitempty"`
	// Dependencies — зависимости между файлами проекта.
	Dependencies []Dependency `json:"dependencies,omitempty"`
	// Cycles — циклы в графе зависимостей.
	Cycles []Cycle `json:"cycles,omitempty"`
	// Unexamined — граница среза: содержимое, о котором фактов не получено.
	Unexamined []Unexamined `json:"unexamined,omitempty"`
}

// Collect отмечает уровень собранным.
func (s *Slice) Collect(level Level) {
	for _, known := range s.Collected {
		if known == level {
			return
		}
	}
	s.Collected = append(s.Collected, level)
}

// Note добавляет запись о непросмотренном содержимом.
func (s *Slice) Note(where, reason string) {
	s.Unexamined = append(s.Unexamined, Unexamined{Where: where, Reason: reason})
}

// Module — файл проверяемого проекта и объявленное в нём. Уровень L1.
type Module struct {
	// Path — путь файла от корня проверяемого проекта.
	Path string `json:"path"`
	// Imports — то, что файл ввозит.
	Imports []Import `json:"imports,omitempty"`
	// Functions — функции и методы файла.
	Functions []Function `json:"functions,omitempty"`
	// Definitions — прочие объявления: типы, константы, переменные.
	Definitions []Definition `json:"definitions,omitempty"`
	// Types — имена объявленных в файле типов.
	Types []string `json:"types,omitempty"`
}

// Import — ввоз модуля.
type Import struct {
	// Module — что ввозится: путь пакета или имя модуля.
	Module string `json:"module"`
	// Selective — ввоз отдельных имён, а не модуля целиком.
	Selective bool `json:"selective,omitempty"`
}

// Function — функция или метод с местом объявления.
type Function struct {
	// Name — имя функции; у метода включает тип-получатель.
	Name string `json:"name"`
	// Signature — объявление как оно записано в файле.
	Signature string `json:"signature,omitempty"`
	// Line, LineEnd — границы объявления в файле.
	Line    int `json:"line,omitempty"`
	LineEnd int `json:"line_end,omitempty"`
}

// Definition — объявление, не являющееся функцией.
type Definition struct {
	// Name — имя объявления.
	Name string `json:"name"`
	// Kind — род объявления в терминах анализатора: тип, константа, переменная.
	Kind string `json:"kind"`
	// Signature — объявление как оно записано в файле.
	Signature string `json:"signature,omitempty"`
	// Line — строка объявления.
	Line int `json:"line,omitempty"`
}

// Call — вызов одной функции из другой. Уровень L2.
type Call struct {
	// FromFile, FromFunc — откуда вызывают.
	FromFile string `json:"from_file"`
	FromFunc string `json:"from_func"`
	// ToFile, ToFunc — кого вызывают.
	ToFile string `json:"to_file"`
	ToFunc string `json:"to_func"`
	// Kind — род вызова в терминах анализатора: внутри файла, между файлами.
	Kind string `json:"kind,omitempty"`
}

// Unreachable — функция, до которой не доходит ни один вызов. Уровень L2.
type Unreachable struct {
	// File, Name — где объявлена функция.
	File string `json:"file"`
	Name string `json:"name"`
	// Line — строка объявления.
	Line int `json:"line,omitempty"`
	// Certain — анализатор уверен в недостижимости. Значение false означает
	// предположение: ссылки есть, но вызовом их анализатор не счёл.
	Certain bool `json:"certain"`
}

// Dependency — зависимость одного файла проекта от другого.
type Dependency struct {
	// From — файл, который зависит.
	From string `json:"from"`
	// To — файл, от которого зависят.
	To string `json:"to"`
}

// Cycle — цикл в графе зависимостей: перечень файлов по порядку обхода.
type Cycle struct {
	// Path — файлы цикла в порядке зависимости.
	Path []string `json:"path"`
}

// FunctionFacts — факты о теле одной функции: уровни L3, L4 и L5.
//
// Эти уровни анализатор отдаёт только по запросу о конкретной функции, поэтому запись
// заводится на каждую функцию области карты. Область и есть мера стоимости: вне карты
// функции не обходятся.
type FunctionFacts struct {
	// File, Name, Line — где объявлена функция.
	File string `json:"file"`
	Name string `json:"name"`
	Line int    `json:"line,omitempty"`
	// Reaching — L3: определения переменных, достигающие тела функции.
	Reaching []VariableDef `json:"reaching,omitempty"`
	// Available — L3: выражения, доступные к моменту исполнения.
	Available []Expression `json:"available,omitempty"`
	// DeadStores — L4: присваивания, результат которых не используется.
	DeadStores []VariableDef `json:"dead_stores,omitempty"`
	// Slice — L5: строки, от которых зависит начало функции.
	Slice []SliceLine `json:"slice,omitempty"`
}

// VariableDef — присваивание переменной с местом в файле.
type VariableDef struct {
	Var  string `json:"var"`
	Line int    `json:"line,omitempty"`
}

// Expression — выражение, доступное в точке исполнения.
type Expression struct {
	Text string `json:"text"`
	Line int    `json:"line,omitempty"`
}

// SliceLine — строка среза программы и род зависимости, которой она включена.
type SliceLine struct {
	Line int    `json:"line"`
	Dep  string `json:"dep,omitempty"`
}

// Unexamined — содержимое, о котором фактов не получено.
//
// Запись не является нарушением и не является фактом: это граница утверждения.
// Правило, которому относится непросмотренное, обязано отдать err, а не ok.
type Unexamined struct {
	// Where — файл, уровень или иная единица, оставшаяся без фактов.
	Where string `json:"where"`
	// Reason — почему фактов нет.
	Reason string `json:"reason"`
}
