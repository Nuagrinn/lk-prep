---
lk:
  source_role: official_reference
  source_refs:
    - "Go Spec: Go statements"
    - "Go Spec: Channel types, send statements, receive operator, select statements"
    - "Go Memory Model"
    - "Go package docs: sync"
    - "Go package docs: sync/atomic"
    - "Go Blog: Go Concurrency Patterns: Pipelines and cancellation"
    - "Go Blog: Go Concurrency Patterns: Context"
  prompt_helper: |
    Это Go-тема по официальным источникам. Генерируй больше практических задач:
    что выведет код, где deadlock, где data race, почему goroutine leak,
    чем channel отличается от mutex, что делает WaitGroup, когда нужен close,
    почему atomic не заменяет бизнесовую атомарность. Проверяй понимание
    happens-before, ownership, cancellation и жизненного цикла goroutine.
---

# Базовые примитивы конкурентности в Go

Эта тема не про то, чтобы запомнить набор рецептов: "для счетчика mutex", "для
очереди channel", "для ожидания WaitGroup". На собеседовании важнее показать,
что ты понимаешь базовую модель: goroutine выполняются независимо, память сама
по себе не становится согласованной, а примитивы синхронизации нужны, чтобы
явно связать действия разных goroutine.

В Go есть несколько основных инструментов:

- `go` запускает функцию в отдельной goroutine;
- `channel` передает значения и может синхронизировать goroutine;
- `select` ждет одну из нескольких channel-операций;
- `sync.Mutex` и `sync.RWMutex` защищают shared mutable state;
- `sync.WaitGroup` ждет завершения набора goroutine;
- `sync.Once` гарантирует однократное выполнение;
- `sync/atomic` дает низкоуровневые атомарные операции;
- `context.Context` обычно передает cancellation/deadline через границы вызовов.

## Primary Sources

- [Go Spec: Go statements](https://go.dev/ref/spec#Go_statements)
- [Go Spec: Channel types](https://go.dev/ref/spec#Channel_types)
- [Go Spec: Send statements](https://go.dev/ref/spec#Send_statements)
- [Go Spec: Receive operator](https://go.dev/ref/spec#Receive_operator)
- [Go Spec: Select statements](https://go.dev/ref/spec#Select_statements)
- [Go Memory Model](https://go.dev/ref/mem)
- [Package sync](https://pkg.go.dev/sync)
- [Package sync/atomic](https://pkg.go.dev/sync/atomic)
- [Go Blog: Pipelines and cancellation](https://go.dev/blog/pipelines)
- [Go Blog: Context](https://go.dev/blog/context)

## 1. Главная модель: shared memory требует синхронизации

Goroutine выполняются конкурентно. Это значит, что несколько потоков исполнения
могут продвигаться независимо: одна goroutine читает из сети, другая считает,
третья пишет в map, четвертая ждет channel.

Но переменные в памяти не становятся безопасными автоматически. Если одна
goroutine пишет в переменную, а другая одновременно читает или пишет ту же
переменную без синхронизации, возникает data race.

Go Memory Model полезно держать в голове так:

```text
ordinary read/write сами по себе не синхронизируют goroutine

channel send/receive, close/receive,
mutex unlock/lock,
atomic operations
могут создавать synchronization edges
```

Иными словами, примитив синхронизации отвечает не только за "подождать", но и
за видимость памяти: когда одна goroutine что-то записала до синхронизации,
другая goroutine после соответствующей синхронизации может это увидеть.

Практическая формула:

> Если данные разделяются между goroutine и хотя бы одна goroutine их меняет,
> должен быть явный механизм синхронизации или ownership-модель.

Ownership-модель означает, что mutable state принадлежит одной goroutine, а
остальные общаются с ней через channel. Тогда данные не шарятся напрямую.

## 2. Goroutine

Goroutine запускается оператором `go`:

```go
go doWork()
```

Важно: `go` запускает выполнение функции независимо от текущей goroutine, но
не дает автоматического ожидания результата. Если `main` завершится, программа
завершится вместе с оставшимися goroutine.

Аргументы функции вычисляются в текущей goroutine до запуска новой goroutine.

```go
for _, id := range ids {
	go process(id)
}
```

Здесь значение `id` передается как аргумент. Это хороший стиль: новая goroutine
получает свое значение, а не зависит от внешней переменной цикла.

Goroutine не равна OS thread. Runtime Go сам планирует множество goroutine на
меньшее или равное количество worker threads. Поэтому goroutine дешевая по
сравнению с OS thread, но не бесплатная: у нее есть stack, scheduler cost,
работа с памятью, и она может утечь.

Типичная ошибка:

```go
func handler() {
	go func() {
		for event := range events {
			process(event)
		}
	}()
}
```

Если `events` никогда не закрывается и нет cancellation, эта goroutine может
жить дольше запроса. На ревью надо спрашивать: кто владелец goroutine, когда
она должна завершиться, кто закрывает channel или отменяет context?

## 3. Channels

Channel - типизированный канал передачи значений:

```go
ch := make(chan int)
ch <- 10
v := <-ch
```

Channel соединяет две идеи:

- передать значение;
- синхронизировать send и receive.

Unbuffered channel:

```go
ch := make(chan int)
```

Отправка в unbuffered channel блокируется, пока кто-то не примет значение.
Получение блокируется, пока кто-то не отправит значение. Поэтому unbuffered
channel - это handoff: передача "из рук в руки".

Buffered channel:

```go
ch := make(chan int, 10)
```

Отправка блокируется, когда buffer заполнен. Получение блокируется, когда buffer
пуст. Buffer может сгладить разницу скоростей producer/consumer, но не делает
систему магически безопасной. Если producer производит быстрее, чем consumer
читает, buffer просто заполнится позже.

Nil channel:

```go
var ch chan int
```

Отправка и получение из nil channel блокируются навсегда. В `select` nil channel
часто используют, чтобы временно выключить case.

Closed channel:

```go
close(ch)
v, ok := <-ch
```

После закрытия channel:

- получать оставшиеся buffered values можно;
- когда значения закончились, receive возвращает zero value и `ok == false`;
- отправка в закрытый channel вызывает panic;
- повторный `close` вызывает panic.

Главное правило ownership:

> Channel обычно закрывает sender, то есть сторона, которая знает, что значений
> больше не будет. Receiver обычно не закрывает чужой channel.

Закрытие channel - это не "освободить ресурс". Это сигнал receiver-ам:
"больше значений не будет".

## 4. Range over channel

Частый паттерн:

```go
for v := range ch {
	handle(v)
}
```

Такой цикл завершится только когда channel закрыт и все buffered values
прочитаны. Если sender не закрывает channel, `range` будет ждать бесконечно.

Pipeline pattern из Go Blog держится на простом договоре:

- stage закрывает outbound channel, когда закончила отправлять;
- stage читает inbound channel, пока он не закрыт;
- cancellation нужен, если downstream перестал читать раньше времени.

## 5. Select

`select` ждет одну из нескольких channel-операций:

```go
select {
case v := <-in:
	handle(v)
case out <- value:
	// sent
case <-ctx.Done():
	return ctx.Err()
}
```

Если готово несколько операций, Go выбирает одну pseudo-random. Поэтому нельзя
писать код, который зависит от "первый case всегда приоритетнее".

Если нет готовых операций:

- при наличии `default` выполняется `default`;
- без `default` `select` блокируется.

Nil channel в `select` никогда не готов. Это полезно для включения/выключения
case:

```go
var out chan<- int
if ready {
	out = realOut
}

select {
case out <- value:
	// case enabled only when out != nil
case <-ctx.Done():
	return
}
```

## 6. Mutex

`sync.Mutex` защищает критическую секцию:

```go
mu.Lock()
counter++
mu.Unlock()
```

Идиоматично почти всегда писать:

```go
mu.Lock()
defer mu.Unlock()

counter++
```

Но `defer` внутри очень горячего короткого цикла может быть лишней ценой, а
длинный `defer` может случайно расширить lock scope. На ревью важнее проверить,
что под lock находится только доступ к shared state, а не долгий сетевой вызов,
запись в БД или тяжелая обработка.

Mutex должен быть частью контракта:

```go
type Cache struct {
	mu    sync.Mutex // protects items
	items map[string]User
}
```

Плохая модель: "в структуре есть mutex, значит структура безопасна".

Правильная модель: "все чтения и записи protected fields идут под тем же mutex,
структура с mutex не копируется после первого использования, наружу не отдаются
mutable внутренности без копии или отдельного контракта".

Нельзя копировать `sync.Mutex`, `sync.RWMutex`, `sync.WaitGroup`, `sync.Once`
после первого использования. Из-за этого методы структур с mutex обычно должны
быть pointer receiver.

```go
type Counter struct {
	mu sync.Mutex
	n  int
}

func (c *Counter) Inc() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
}
```

Если сделать value receiver, будет копироваться весь `Counter`, включая mutex.
Это классическая ошибка на ревью.

## 7. RWMutex

`sync.RWMutex` позволяет нескольким readers держать `RLock` одновременно, но
writer с `Lock` должен быть один.

```go
mu.RLock()
v := cache[key]
mu.RUnlock()

mu.Lock()
cache[key] = value
mu.Unlock()
```

RWMutex не всегда быстрее Mutex. Он полезен, когда:

- чтений действительно много;
- записи редкие;
- критическая секция достаточно заметная;
- нет сложного upgrade/downgrade сценария.

Частые ошибки:

- взять `RLock`, а потом писать в protected state;
- пытаться "апгрейдить" `RLock` в `Lock`;
- использовать RWMutex автоматически "потому что он продвинутее";
- держать read lock слишком долго и задерживать writer.

Для простого счетчика или маленькой map обычный `Mutex` часто понятнее.

## 8. WaitGroup

`sync.WaitGroup` ждет завершения набора goroutine. Это счетчик:

```go
var wg sync.WaitGroup

for _, job := range jobs {
	wg.Add(1)
	go func(job Job) {
		defer wg.Done()
		process(job)
	}(job)
}

wg.Wait()
```

Главное правило: `Add(1)` обычно должен выполниться до запуска goroutine.

Плохо:

```go
go func() {
	wg.Add(1)
	defer wg.Done()
	process()
}()
wg.Wait()
```

`Wait` может увидеть счетчик равным нулю раньше, чем goroutine выполнит `Add`.

Важно: WaitGroup не защищает данные. Он только ждет завершения. Если goroutine
параллельно пишут в один slice, map или счетчик, нужен mutex, channel ownership
или atomic.

## 9. Once

`sync.Once` гарантирует, что функция будет выполнена один раз:

```go
var once sync.Once

func initConfig() {
	once.Do(func() {
		loadConfig()
	})
}
```

Это удобно для lazy initialization. Но `Once` не стоит использовать как способ
спрятать сложный жизненный цикл. Если инициализация может вернуть ошибку,
нужно явно решить, что делать с ошибкой и повторными попытками.

`Once` тоже нельзя копировать после первого использования.

## 10. Atomic

`sync/atomic` дает низкоуровневые атомарные операции:

```go
var counter atomic.Int64

counter.Add(1)
value := counter.Load()
```

Atomic подходит для простых независимых значений:

- счетчик;
- флаг;
- pointer/snapshot, который целиком заменяется;
- быстрый read-mostly configuration через immutable value.

Но atomic не заменяет mutex для связанных инвариантов.

Плохая модель:

```go
atomic.StoreInt64(&balance, 100)
atomic.StoreInt64(&version, 2)
```

Если `balance` и `version` должны читаться как единый согласованный snapshot,
две отдельные atomic-переменные не дают такой бизнесовой атомарности. Читатель
может увидеть новое `balance` и старую `version`.

Официальная документация прямо предупреждает: atomic primitives требуют большой
осторожности; кроме специальных низкоуровневых случаев лучше использовать
channels или `sync`.

## 11. Context и cancellation

`context.Context` - не примитив синхронизации уровня `sync`, но в современном
Go он почти всегда рядом с goroutine.

Если запрос отменен или deadline истек, goroutine, работающие на этот запрос,
должны быстро завершиться. Обычно worker слушает:

```go
select {
case job := <-jobs:
	process(job)
case <-ctx.Done():
	return ctx.Err()
}
```

Context особенно важен в pipeline/fan-out коде: если downstream перестал ждать
результат, upstream не должен навсегда зависнуть на send.

## Как выбирать примитив

Короткая эвристика:

- нужен доступ к shared mutable state - `Mutex`;
- много чтений, мало записей, заметная критическая секция - возможно `RWMutex`;
- нужно дождаться завершения goroutine - `WaitGroup`;
- нужно передать значения между goroutine - `channel`;
- нужно владение состоянием одной goroutine - channel ownership;
- нужно отменить работу - `context`;
- нужен простой независимый счетчик/флаг - `atomic`;
- нужно выполнить инициализацию один раз - `Once`;
- нужно ограничить параллелизм - buffered channel как semaphore или отдельный limiter.

Очень собеседовательная фраза:

> Channel лучше, когда я передаю ownership или события между goroutine. Mutex
> лучше, когда у меня есть shared state, который естественно живет в одной
> структуре и должен быть защищен единым lock contract.

## Типичные ошибки на ревью

### Goroutine без жизненного цикла

```go
go worker()
```

Вопросы:

- кто остановит worker?
- что будет при shutdown?
- есть ли `ctx.Done()` или закрытие input channel?
- кто ждет завершения?

### Send без receiver

```go
ch := make(chan int)
ch <- 1
```

Unbuffered send без receiver блокируется. Если это единственная goroutine,
будет deadlock.

### Range по channel без close

```go
for v := range ch {
	handle(v)
}
```

Если sender не закрывает channel, цикл не завершится.

### Close не владельцем

Receiver не должен закрывать channel, если не он владеет отправкой. Иначе
другой sender может отправить в закрытый channel и получить panic.

### WaitGroup Add внутри goroutine

```go
go func() {
	wg.Add(1)
	defer wg.Done()
}()
wg.Wait()
```

`Wait` может пройти раньше `Add`.

### WaitGroup как "защита данных"

```go
results = append(results, value)
```

Если это делают несколько goroutine одновременно, `WaitGroup` не помогает.
Он только ждет завершения, но не защищает `results`.

### Копирование mutex

```go
func (c Counter) Inc() {}
```

Если `Counter` содержит `sync.Mutex`, value receiver копирует lock.

### Atomic для нескольких связанных полей

Atomic операция атомарна для одной переменной/операции. Она не делает набор
полей единым бизнесовым snapshot.

## Мини-шпаргалка "что выведет"

### Receive from closed channel

```go
ch := make(chan int, 1)
ch <- 7
close(ch)

a, ok1 := <-ch
b, ok2 := <-ch
fmt.Println(a, ok1, b, ok2)
```

Ответ: сначала читается buffered value `7, true`, потом channel пуст и закрыт:
`0, false`.

### Select with default

```go
var ch chan int

select {
case <-ch:
	fmt.Println("receive")
default:
	fmt.Println("default")
}
```

Receive из nil channel никогда не готов, поэтому выполнится `default`.

### WaitGroup и общий счетчик

```go
var wg sync.WaitGroup
count := 0

for i := 0; i < 1000; i++ {
	wg.Add(1)
	go func() {
		defer wg.Done()
		count++
	}()
}

wg.Wait()
fmt.Println(count)
```

Это data race. Даже если иногда напечатает `1000`, программа некорректна.
Нужен mutex, channel aggregation или atomic counter.

## Ответ на собеседовании

В Go goroutine - легковесная единица конкурентного выполнения, которую runtime
планирует поверх OS threads. Но запуск goroutine сам по себе не дает ни
ожидания результата, ни безопасности памяти.

Для синхронизации есть несколько базовых примитивов. Channel передает значения
и синхронизирует отправителя с получателем; unbuffered channel делает handoff,
buffered channel дает очередь ограниченного размера. Mutex защищает shared
mutable state, и важно, чтобы все обращения к protected fields шли под одним
контрактом. WaitGroup только ждет завершения goroutine и не защищает данные.
Atomic подходит для простых независимых счетчиков или флагов, но не заменяет
mutex для связанных инвариантов.

На ревью я сначала ищу жизненный цикл goroutine, затем ownership channel-ов,
правильное закрытие, data race на shared state, misuse WaitGroup, копирование
mutex и отсутствие cancellation.

