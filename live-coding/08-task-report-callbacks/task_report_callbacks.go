// Задача LC06-8: обработка слайса через callback-реализации и локальную функцию.
//
// Это следующая ступень после простого callback и pipeline:
// теперь нужно написать функцию, которая принимает несколько реализаций поведения
// и применяет их к массиву/слайсу данных.
//
// Нужно реализовать:
//
//	func BuildTaskReport(
//		tasks []Task,
//		keep func(Task) bool,
//		line func(Task) string,
//		decorate func(string) string,
//	) []string
//
//	func WithPrefix(prefix string, line func(Task) string) func(Task) string
//
// Правила:
//   - BuildTaskReport проходит tasks в исходном порядке.
//   - Для каждой task сначала вызывается keep(task).
//   - Если keep вернул false, task пропускается.
//   - Если keep вернул true, нужно получить base := line(task).
//   - Потом применить decorate(base) и добавить результат в итоговый []string.
//   - BuildTaskReport ничего не печатает и не меняет исходный slice.
//   - Если ни одна task не прошла фильтр, вернуть пустой non-nil slice: []string{}.
//   - Внутри BuildTaskReport заведи локальную функцию add(task Task), которая
//     делает line -> decorate -> append. Это тренировка вложенной функции.
//   - WithPrefix возвращает новую функцию, которая захватывает prefix и line,
//     а при вызове возвращает prefix + line(task).
//   - Для простоты nil callback'и в этой задаче не проверяем.
//
// Что здесь проверяется:
//   - передача конкретных реализаций функций в общую обработку;
//   - фильтрация слайса через callback;
//   - форматирование элемента через callback;
//   - post-processing строки через callback;
//   - локальная функция внутри другой функции;
//   - closure: WithPrefix захватывает prefix и line;
//   - аккуратный пустой результат без nil slice.
//
// Ожидаемый вывод после правильной реализации:
//
//	OK: report filters, formats and decorates in order
//	OK: WithPrefix captures formatter
//	OK: empty filter result is non-nil empty slice
package main

import (
	"fmt"
	"reflect"
)

type Task struct {
	ID       int
	Title    string
	Owner    string
	Priority int
	Done     bool
}

func BuildTaskReport(
	tasks []Task,
	keep func(Task) bool,
	line func(Task) string,
	decorate func(string) string,
) []string {
	// TODO: создай пустой result и локальную функцию add(task Task).
	// add должна сделать line(task), decorate(base) и append в result.
	// Потом пройди tasks по порядку и вызывай add только для task, где keep == true.
	var results []string
	
	add := func(task Task) {
		results = append(results, decorate(line(task)))
	}
	
	for _, task := range tasks {
		if keep(task) {
			add(task)
		}
	}
	
	if len(results) == 0 {
		return []string{}
	}
	return results
}

func WithPrefix(prefix string, line func(Task) string) func(Task) string {
	// TODO: верни анонимную функцию, которая использует prefix и line.
	return func(task Task) string {
		return prefix + line(task)
	}
}

func main() {
	runReportExample()
	runPrefixExample()
	runEmptyExample()
}

func runReportExample() {
	tasks := []Task{
		{ID: 1, Title: "pay invoice", Owner: "Ann", Priority: 4, Done: false},
		{ID: 2, Title: "write docs", Owner: "Bob", Priority: 2, Done: false},
		{ID: 3, Title: "release", Owner: "Kate", Priority: 5, Done: true},
		{ID: 4, Title: "fix cache", Owner: "Ann", Priority: 5, Done: false},
	}
	
	minPriority := 4
	keepOpenImportant := func(task Task) bool {
		return !task.Done && task.Priority >= minPriority
	}
	
	format := WithPrefix("TODO: ", func(task Task) string {
		return fmt.Sprintf("%s#%d:%s", task.Owner, task.ID, task.Title)
	})
	
	decorate := func(line string) string {
		return "[" + line + "]"
	}
	
	got := BuildTaskReport(tasks, keepOpenImportant, format, decorate)
	
	expectStrings(
		"report filters, formats and decorates in order",
		got,
		[]string{
			"[TODO: Ann#1:pay invoice]",
			"[TODO: Ann#4:fix cache]",
		},
	)
}

func runPrefixExample() {
	format := WithPrefix("owner=", func(task Task) string {
		return task.Owner
	})
	
	got := format(Task{Owner: "Kate"})
	expectString("WithPrefix captures formatter", got, "owner=Kate")
}

func runEmptyExample() {
	tasks := []Task{
		{ID: 1, Title: "done", Owner: "Ann", Priority: 1, Done: true},
	}
	
	got := BuildTaskReport(
		tasks,
		func(task Task) bool {
			return !task.Done
		},
		func(task Task) string {
			return task.Title
		},
		func(line string) string {
			return line
		},
	)
	
	expectStrings("empty filter result is non-nil empty slice", got, []string{})
}

func expectString(name, got, want string) {
	if got == want {
		fmt.Println("OK:", name)
		return
	}
	
	fmt.Printf("MISMATCH: %s\n got: %q\nwant: %q\n", name, got, want)
}

func expectStrings(name string, got, want []string) {
	if reflect.DeepEqual(got, want) {
		fmt.Println("OK:", name)
		return
	}
	
	fmt.Printf("MISMATCH: %s\n got: %#v\nwant: %#v\n", name, got, want)
}
