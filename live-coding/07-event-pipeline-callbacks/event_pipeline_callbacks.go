// Задача LC06-7: event pipeline через callbacks, closure и nil validation.
//
// Это задача посложнее предыдущей: теперь функцию нужно не просто принять
// аргументом и сразу вызвать, а сохранить несколько callback'ов в структуре,
// вызвать их позже из метода и вернуть функцию-обертку из другой функции.
//
// Нужно реализовать:
//
//	type EventPipeline struct { ... }
//
//	func NewEventPipeline(
//		allow func(Event) bool,
//		format func(Event) string,
//		emit func(string),
//	) *EventPipeline
//
//	func (p *EventPipeline) Process(events []Event)
//	func WithPrefix(prefix string, format func(Event) string) func(Event) string
//
// Правила:
//   - NewEventPipeline сохраняет все три callback'а в структуре.
//   - Если allow, format или emit == nil, NewEventPipeline должен panic-нуть.
//   - Process проходит events в исходном порядке.
//   - Для каждого event сначала вызывается allow(event).
//   - Если allow вернул false, event пропускается.
//   - Если allow вернул true, нужно вызвать format(event), а результат передать
//     в emit(line).
//   - Process ничего не печатает и ничего не возвращает.
//   - WithPrefix возвращает новую функцию, которая захватывает prefix и format,
//     а при вызове возвращает prefix + format(event).
//
// Что здесь проверяется:
//   - func как поле структуры;
//   - callback, который вызывается позже;
//   - анонимная функция, которую возвращают из функции;
//   - closure: WithPrefix захватывает prefix и format;
//   - защита от nil function value в конструкторе.
//
// Ожидаемый вывод после правильной реализации:
//
//	OK: pipeline filters and emits in order
//	OK: WithPrefix captures prefix
//	OK: nil allow panics
package main

import (
	"errors"
	"fmt"
	"reflect"
)

type Event struct {
	User   string
	Action string
	Score  int
}

type EventPipeline struct {
	// TODO: добавь поля для callback'ов.
	allow  func(Event) bool
	format func(Event) string
	emit   func(string)
}

func NewEventPipeline(
	allow func(Event) bool,
	format func(Event) string,
	emit func(string),
) *EventPipeline {
	// TODO: проверь nil callback'и и сохрани функции в структуре.
	if allow == nil || format == nil || emit == nil {
		panic(errors.New("invalid arguments"))
	}
	return &EventPipeline{
		allow:  allow,
		format: format,
		emit:   emit,
	}
}

func (p *EventPipeline) Process(events []Event) {
	// TODO: отфильтруй events через allow, отформатируй через format и отдай в emit.
	for _, event := range events {
		if p.allow(event) {
			p.emit(p.format(event))
		}
		
	}
}

func WithPrefix(prefix string, format func(Event) string) func(Event) string {
	// TODO: верни анонимную функцию, которая использует prefix и format.
	return func(event Event) string {
		return prefix + format(event)
	}
}

func main() {
	runPipelineExample()
	runPrefixExample()
	runNilExample()
}

func runPipelineExample() {
	events := []Event{
		{User: "Ann", Action: "login", Score: 5},
		{User: "Bob", Action: "view", Score: 1},
		{User: "Kate", Action: "pay", Score: 10},
		{User: "Ann", Action: "logout", Score: 2},
	}
	
	minScore := 5
	var got []string
	
	pipeline := NewEventPipeline(
		func(event Event) bool {
			return event.Score >= minScore
		},
		WithPrefix("AUDIT: ", func(event Event) string {
			return fmt.Sprintf("%s/%s/%d", event.User, event.Action, event.Score)
		}),
		func(line string) {
			got = append(got, line)
		},
	)
	
	minScore = 6
	pipeline.Process(events)
	
	expectStrings(
		"pipeline filters and emits in order",
		got,
		[]string{"AUDIT: Kate/pay/10"},
	)
}

func runPrefixExample() {
	format := WithPrefix("event=", func(event Event) string {
		return event.Action
	})
	
	got := format(Event{Action: "signup"})
	expectString("WithPrefix captures prefix", got, "event=signup")
}

func runNilExample() {
	expectPanic("nil allow panics", func() {
		_ = NewEventPipeline(
			nil,
			func(Event) string { return "" },
			func(string) {},
		)
	})
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

func expectPanic(name string, fn func()) {
	didPanic := false
	
	func() {
		defer func() {
			if recover() != nil {
				didPanic = true
			}
		}()
		
		fn()
	}()
	
	if didPanic {
		fmt.Println("OK:", name)
		return
	}
	
	fmt.Printf("MISMATCH: %s\n got: no panic\nwant: panic\n", name)
}
