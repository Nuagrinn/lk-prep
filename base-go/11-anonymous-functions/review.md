---
lk:
  source_role: official_reference
  source_refs:
    - "Go Spec: Function literals"
    - "Go Spec: Function types"
    - "Go Spec: Calls"
    - "Go Blog: Defer, Panic, and Recover"
    - "Go Wiki: CommonMistakes"
  prompt_helper: |
    Это базовая Go-тема про function values, anonymous functions, closures и
    callbacks. Генерируй вопросы на чтение сигнатур вида func([]T), передачу
    функции в конструктор, сохранение callback в struct, захват переменных,
    range-loop closure bugs, defer с анонимными функциями и тестируемость через
    dependency injection.
  challenge_helper: |
    Давай маленькие задачи: передать функцию в конструктор и вызвать позже,
    реализовать map/filter/reduce, собрать middleware-like wrapper, проверить
    вывод кода с замыканием, найти ошибку захвата переменной в цикле.
---

# Анонимные функции, callbacks и closures в Go

Эта тема выглядит синтаксической, но на практике она часто ломает понимание
лайвкодинг-задач. Когда в условии написано:

```go
func NewBatchFlusher(maxDelay time.Duration, flush func([]string)) *BatchFlusher
```

это не просто "какая-то странная сигнатура". Это указание на архитектуру задачи:
структура должна получить функцию снаружи, сохранить ее и вызвать позже. Такая
функция часто называется callback.

Чтобы уверенно читать такой код, нужно принять одну базовую идею:

> В Go функция - это значение. Ее можно положить в переменную, передать в другую
> функцию, сохранить в поле структуры и вызвать позже.

Функция в Go не обязана существовать только как именованная декларация:

```go
func PrintBatch(batch []string) {
	fmt.Println(batch)
}
```

Ее можно создать прямо на месте:

```go
func(batch []string) {
	fmt.Println(batch)
}
```

Вот это и есть анонимная функция: функция без имени.

## Primary Sources

- [Go Spec: Function literals](https://go.dev/ref/spec#Function_literals)
- [Go Spec: Function types](https://go.dev/ref/spec#Function_types)
- [Go Spec: Calls](https://go.dev/ref/spec#Calls)
- [Go Blog: Defer, Panic, and Recover](https://go.dev/blog/defer-panic-and-recover)
- [Go Wiki: CommonMistakes](https://go.dev/wiki/CommonMistakes)

## 1. Функция как тип

В Go у функции есть тип. Например:

```go
func([]string)
```

Это тип функции, которая:

- принимает один аргумент `[]string`;
- ничего не возвращает.

Другие примеры:

```go
func(int) int
func(string, string) bool
func(context.Context, int64) (User, error)
func()
```

Сигнатура функции полностью задает ее форму: какие аргументы она принимает и что
возвращает. Если две функции имеют одинаковую сигнатуру, одну можно передать
туда, где ожидают другую.

Пример:

```go
func apply(x int, f func(int) int) int {
	return f(x)
}

func double(x int) int {
	return x * 2
}

func main() {
	fmt.Println(apply(10, double)) // 20
}
```

Здесь `double` - обычная именованная функция. Но она подходит как аргумент,
потому что ее тип `func(int) int`.

То же самое можно написать анонимной функцией:

```go
fmt.Println(apply(10, func(x int) int {
	return x * 2
}))
```

## 2. Анонимная функция как значение

Анонимная функция - это function literal. Она создает значение-функцию прямо в
месте объявления.

```go
printer := func(s string) {
	fmt.Println(s)
}

printer("hello")
```

Мысленно это похоже на:

```go
var printer func(string)

printer = func(s string) {
	fmt.Println(s)
}
```

То есть `printer` - переменная, в которой лежит функция. Потом мы вызываем ее
как обычную функцию:

```go
printer("hello")
```

Можно передать ее в другую функцию:

```go
func repeat(n int, action func()) {
	for i := 0; i < n; i++ {
		action()
	}
}

repeat(3, func() {
	fmt.Println("tick")
})
```

Здесь второй аргумент `repeat` - не результат функции, а сама функция.

## 3. Как аргументы мапятся в конструкторе

Возьмем сигнатуру:

```go
func NewBatchFlusher(maxDelay time.Duration, flush func([]string)) *BatchFlusher
```

Вызов:

```go
bf := NewBatchFlusher(
	time.Second,
	func(batch []string) {
		fmt.Println("FLUSH:", batch)
	},
)
```

Аргументы сопоставляются по порядку:

```text
time.Second                         -> maxDelay time.Duration
func(batch []string) { ... }         -> flush func([]string)
```

Конструктор обычно сохраняет эти значения в структуру:

```go
type BatchFlusher struct {
	maxDelay time.Duration
	flush    func([]string)
}

func NewBatchFlusher(maxDelay time.Duration, flush func([]string)) *BatchFlusher {
	return &BatchFlusher{
		maxDelay: maxDelay,
		flush:    flush,
	}
}
```

Важно различать левую и правую часть:

```go
flush: flush,
```

Слева `flush` - поле структуры. Справа `flush` - параметр конструктора. В поле
кладется функция, которую передали снаружи.

Потом внутри метода можно вызвать:

```go
b.flush(batch)
```

Если при создании передали:

```go
func(batch []string) {
	fmt.Println("FLUSH:", batch)
}
```

то фактически `b.flush(batch)` выполнит именно этот код.

## 4. Callback: функция, которую вызовут позже

Callback - это функция, которую ты передаешь в чужой код, чтобы он вызвал ее в
нужный момент.

В задаче с batch flusher:

```text
BatchFlusher владеет временем и буфером.
flush владеет тем, что делать с готовой пачкой.
```

`BatchFlusher` не должен знать, куда отправлять данные:

- в консоль;
- в БД;
- в Kafka;
- в HTTP API;
- в тестовый slice.

Он должен только накопить batch и вызвать callback:

```go
b.flush(batch)
```

Пример с консолью:

```go
bf := NewBatchFlusher(time.Second, func(batch []string) {
	fmt.Println("send:", batch)
})
```

Пример с тестом:

```go
var got [][]string

bf := NewBatchFlusher(time.Second, func(batch []string) {
	got = append(got, batch)
})
```

Пример с репозиторием:

```go
bf := NewBatchFlusher(time.Second, func(batch []string) {
	_ = repo.SaveBatch(context.Background(), batch)
})
```

Одна и та же структура работает с разным внешним поведением. Это и есть польза
callback: механизм один, действие подставляется снаружи.

## 5. Почему это удобно для тестов

Если внутри структуры жестко написать:

```go
fmt.Println(batch)
```

то тестировать неудобно: нужно перехватывать stdout.

Если написать:

```go
repo.SaveBatch(batch)
```

то структура зависит от конкретного репозитория.

А если принять функцию:

```go
func NewBatchFlusher(flush func([]string)) *BatchFlusher
```

тест может передать простую функцию:

```go
var flushed [][]string

bf := NewBatchFlusher(func(batch []string) {
	flushed = append(flushed, append([]string(nil), batch...))
})
```

Так тест проверяет поведение без БД, сети, файлов и лишней инфраструктуры.

Это маленькая форма dependency injection: зависимость передается снаружи, а не
создается внутри.

## 6. Closure: анонимная функция может захватывать переменные

Анонимная функция может использовать переменные из внешней области видимости.
Такую функцию называют closure, замыкание.

```go
prefix := "user"

format := func(id int) string {
	return fmt.Sprintf("%s-%d", prefix, id)
}

fmt.Println(format(42)) // user-42
```

Функция `format` использует `prefix`, хотя `prefix` не передан аргументом. Она
захватила переменную из внешней области.

Это удобно, но важно понимать: захватывается не "снимок значения", а переменная.

```go
x := 10

f := func() {
	fmt.Println(x)
}

x = 20
f() // 20
```

Если тебе нужен снимок, создай локальную копию:

```go
x := 10
captured := x

f := func() {
	fmt.Println(captured)
}

x = 20
f() // 10
```

## 7. Захват переменных в цикле

Классический источник ошибок - анонимные функции в циклах.

В старых версиях Go частая ошибка выглядела так:

```go
for _, name := range names {
	go func() {
		fmt.Println(name)
	}()
}
```

Проблема была в том, что goroutine захватывали одну и ту же переменную цикла.
К моменту выполнения goroutine значение могло уже измениться.

Начиная с Go 1.22, переменные range-цикла при коротком объявлении имеют новую
переменную на каждой итерации. Поэтому часть старых примеров стала безопаснее.
Но базовое правило все равно полезно:

> Если анонимная функция будет выполнена позже, явно подумай, какие переменные
> она захватывает.

Универсально понятный вариант:

```go
for _, name := range names {
	name := name
	go func() {
		fmt.Println(name)
	}()
}
```

Или передать значение аргументом:

```go
for _, name := range names {
	go func(name string) {
		fmt.Println(name)
	}(name)
}
```

На собеседовании это показывает, что ты понимаешь не только синтаксис, но и
момент выполнения функции.

## 8. Немедленный вызов анонимной функции

Анонимную функцию можно объявить и сразу вызвать:

```go
result := func() int {
	x := expensive()
	return x * 2
}()
```

Обрати внимание на `()` в конце. Без них это было бы значение-функция:

```go
f := func() int {
	return 10
}
```

С ними - немедленный вызов:

```go
v := func() int {
	return 10
}()
```

В Go это иногда используют, чтобы:

- ограничить область видимости временных переменных;
- удобно инициализировать сложное значение;
- выполнить небольшой кусок логики рядом с местом использования.

Но злоупотреблять не нужно. Если блок становится большим, лучше именованная
функция.

## 9. defer и анонимные функции

`defer` часто используют с анонимными функциями:

```go
start := time.Now()

defer func() {
	fmt.Println("elapsed:", time.Since(start))
}()
```

Здесь анонимная функция выполнится при выходе из текущей функции.

Важный нюанс: аргументы deferred call вычисляются сразу, а тело deferred
анонимной функции выполнится позже.

Сравни:

```go
start := time.Now()
defer fmt.Println(time.Since(start))
```

`time.Since(start)` вычислится сразу, поэтому значение будет почти нулевым.

А здесь:

```go
start := time.Now()
defer func() {
	fmt.Println(time.Since(start))
}()
```

`time.Since(start)` вычислится в момент выполнения defer, то есть в конце
функции.

## 10. nil function value

Функциональное поле может быть nil:

```go
type Worker struct {
	onDone func()
}
```

Если вызвать nil-функцию, будет panic:

```go
var f func()
f() // panic
```

Поэтому если callback обязателен, лучше проверять его в конструкторе:

```go
func NewWorker(onDone func()) *Worker {
	if onDone == nil {
		panic("onDone is nil")
	}

	return &Worker{onDone: onDone}
}
```

На интервью можно сказать мягче: "в production-коде я бы валидировал
обязательные зависимости в конструкторе; для задачи можно либо panic, либо
возвращать ошибку, зависит от стиля".

## 11. Когда лучше interface, а когда func

Иногда зависимость удобно передать функцией:

```go
type BatchFlusher struct {
	flush func([]string)
}
```

Это хорошо, если зависимость - одно действие.

Если действий несколько, лучше интерфейс:

```go
type BatchSink interface {
	Flush([]string) error
	Close() error
}
```

Практическое правило:

- одно маленькое поведение - `func`;
- несколько методов и состояние - interface;
- нужно мокать сложный клиент - interface;
- нужно просто "что сделать с batch" - `func(batch []T)`.

## 12. Что смотреть на code review

Если видишь поле:

```go
callback func(Event)
```

проверь:

- может ли оно быть nil;
- где оно вызывается;
- не вызывается ли оно под mutex;
- что будет, если callback долгий;
- что будет, если callback panic-нет;
- захватывает ли callback переменные, которые меняются;
- не возникает ли data race на захваченных переменных;
- копируется ли slice/map перед передачей наружу, если внутренний buffer будет
  переиспользован.

Особенно важный момент со slice:

```go
batch := b.buffer
b.buffer = b.buffer[:0]
b.flush(batch)
```

Если после этого `b.buffer` переиспользует тот же underlying array, а `flush`
сохранит `batch`, могут быть неожиданные эффекты. Надежнее передать копию:

```go
batch := append([]string(nil), b.buffer...)
b.buffer = b.buffer[:0]
```

Это чуть дороже, но безопаснее для учебной задачи и часто правильнее для
асинхронного внешнего мира.

## 13. Как отвечать на собеседовании

Если спрашивают, что такое анонимная функция:

> Это function literal: функция без имени, которую можно создать прямо в месте
> использования. В Go функции являются значениями, поэтому такую функцию можно
> присвоить переменной, передать аргументом, вернуть из функции или сохранить в
> структуру.

Если спрашивают, что такое closure:

> Closure - это функция, которая использует переменные из внешней области
> видимости. Она захватывает переменные, а не просто получает их значения
> аргументами. Поэтому надо внимательно смотреть на изменяемые переменные,
> циклы и конкурентный доступ.

Если спрашивают, зачем передавать `flush func([]string)` в конструктор:

> Это callback и маленькая форма dependency injection. `BatchFlusher` отвечает
> за накопление и момент сброса, но не знает, куда отправлять пачку. Снаружи я
> передаю функцию, которая описывает действие: напечатать, сохранить в тестовый
> slice, записать в БД, отправить в брокер. Так код становится универсальным и
> тестируемым.

## 14. Мини-шпаргалка

```go
// Function type.
var f func(int) int

// Anonymous function.
f = func(x int) int {
	return x * 2
}

// Call function value.
fmt.Println(f(10))

// Pass function as argument.
apply(10, func(x int) int {
	return x + 1
})

// Store callback in struct.
type Runner struct {
	onDone func()
}

// Closure captures variable.
prefix := "id"
format := func(n int) string {
	return fmt.Sprintf("%s-%d", prefix, n)
}

// Immediate invocation.
value := func() int {
	return 42
}()
```

Главная мысль: когда ты видишь параметр вида `flush func([]string)`, читай его
как "мне передают действие, которое я должен вызвать в нужный момент".
