---
lk:
  source_role: official_reference
  source_refs:
    - "Go package docs: time"
    - "Go package docs: context"
    - "Go Blog: Go Concurrency Patterns: Context"
    - "Go Docs: Canceling in-progress database operations"
    - "Go Memory Model"
    - "Go package docs: net/http"
    - "Go package docs: database/sql"
  prompt_helper: |
    Это базовая Go-тема для лайвкодинга и service layer. Генерируй вопросы
    про time.Duration, time.Time, monotonic clock, Timer, Ticker, time.After,
    context.WithTimeout, TTL cache, background refresh, shutdown, highload
    handlers, stale value policy и ошибки жизненного цикла timer/ticker.
  challenge_helper: |
    Давай небольшие практические задачи: TTL cache, периодическое обновление
    значения через внешний вызов, request timeout, delayed flush, debounce,
    graceful shutdown ticker-goroutine. Проверяй, что кандидат умеет выбрать
    time.Timer/time.Ticker/context и не делает data race.
---

# Время в Go: time, context, timers, tickers, TTL и background refresh

Эта тема кажется маленькой, пока на лайвкодинге не просят написать TTL-cache,
периодическое обновление прогноза, timeout для внешнего вызова или ручку,
которая должна выдержать 10k RPS и не дергать тяжелую функцию на каждый запрос.
В этот момент оказывается, что надо не просто помнить названия пакетов, а
понимать модель: что такое момент времени, что такое длительность, как runtime
будит goroutine, чем `Timer` отличается от `Ticker`, где нужен `context`, почему
нельзя делать внешний вызов прямо в highload handler и почему cache update без
синхронизации превращается в data race.

В Go работа со временем почти всегда крутится вокруг двух пакетов:

- `time` - моменты времени, длительности, таймеры, тикеры, измерение времени;
- `context` - cancellation, deadline, timeout и request scope.

`time` отвечает на вопрос "когда" и "через сколько". `context` отвечает на
вопрос "эта работа еще нужна или ее уже надо остановить". В реальном сервисном
коде они часто используются вместе: `context.WithTimeout` внутри себя опирается
на timer, HTTP/SQL/gRPC-вызовы принимают `ctx`, а фоновые goroutine обычно
останавливаются через `ctx.Done()`.

## Primary Sources

- [Package time](https://pkg.go.dev/time)
- [Package context](https://pkg.go.dev/context)
- [Go Blog: Go Concurrency Patterns: Context](https://go.dev/blog/context)
- [Go Docs: Canceling in-progress database operations](https://go.dev/doc/database/cancel-operations)
- [Go Memory Model](https://go.dev/ref/mem)
- [Package net/http](https://pkg.go.dev/net/http)
- [Package database/sql](https://pkg.go.dev/database/sql)

## 1. Базовая модель: Time, Duration и clock

Самые важные типы:

```go
time.Time      // конкретный момент времени
time.Duration  // длительность, internally int64 наносекунд
```

`time.Time` отвечает на вопрос: "какой момент?". Например, `time.Now()` вернет
текущее время. `time.Duration` отвечает на вопрос: "сколько длится?". Например,
`500*time.Millisecond`, `2*time.Second`, `3*time.Minute`.

Важно: `time.Duration` - это не "секунды по умолчанию". Это количество
наносекунд. Поэтому:

```go
time.Sleep(10)              // sleep на 10 наносекунд
time.Sleep(10 * time.Second) // sleep на 10 секунд
```

На собеседовании это простая, но очень частая ошибка. Если в коде видишь
`time.Sleep(5)`, `time.After(100)` или `context.WithTimeout(ctx, 10)`, почти
всегда это баг: автор забыл единицу измерения.

Основные операции:

```go
now := time.Now()
deadline := now.Add(2 * time.Second)

fmt.Println(time.Since(now))      // сколько прошло с now
fmt.Println(time.Until(deadline)) // сколько осталось до deadline
fmt.Println(now.Before(deadline))
fmt.Println(deadline.After(now))
```

Практическое правило:

- для elapsed time используй `time.Since(start)`;
- для deadline calculations используй `time.Until(deadline)`;
- для TTL храни `expiresAt time.Time`;
- для конфигов храни `time.Duration`, а не `int`.

Плохой вариант:

```go
type Config struct {
	TimeoutSeconds int
}
```

Лучше:

```go
type Config struct {
	Timeout time.Duration
}
```

Так сигнатура сама говорит, что это длительность, и вызывающий код не спорит,
секунды это, миллисекунды или наносекунды.

## 2. Wall clock и monotonic clock

У времени есть неприятная особенность: системные часы могут меняться. Их может
поправить NTP, администратор, виртуальная машина, контейнерная среда. Если ты
измеряешь длительность через "текущее wall-clock время минус старое wall-clock
время", можно получить странные результаты.

Go частично решает это через monotonic clock. Значение, полученное через
`time.Now()`, может содержать две части:

- wall-clock часть: календарное время, которое можно показать человеку;
- monotonic часть: монотонное время для измерения интервалов.

Когда ты делаешь:

```go
start := time.Now()
doWork()
elapsed := time.Since(start)
```

Go использует monotonic reading, если он есть. Поэтому `time.Since(start)` лучше
подходит для измерения длительности, чем ручные вычисления через Unix timestamp.

Практическая формула:

> `time.Time` - для момента, `time.Duration` - для интервала, `time.Since` - для
> latency/elapsed time.

Нюанс: monotonic часть не сериализуется в JSON, не хранится в БД и обычно
теряется при парсинге/форматировании. Это нормально: monotonic clock нужен
внутри процесса для измерения интервалов, а не для передачи времени наружу.

## 3. time.Sleep: когда можно, а когда подозрительно

`time.Sleep(d)` останавливает текущую goroutine минимум на `d`. Это не busy
wait: goroutine паркуется, runtime не крутит CPU в цикле, а потом будит ее,
когда наступит время.

Нормальные места для `Sleep`:

- маленькие примеры и playground-код;
- тестовые ожидания, если нет лучшего синхронизационного механизма;
- простейший retry/backoff, хотя лучше оформлять явно;
- демо-код.

Подозрительные места:

- HTTP handler;
- критический путь highload-ручки;
- ожидание "пока другая goroutine что-то сделает";
- попытка чинить race condition задержкой.

Если код выглядит так:

```go
go worker()
time.Sleep(100 * time.Millisecond)
fmt.Println(result)
```

это почти всегда неправильная синхронизация. Нужно использовать channel,
`sync.WaitGroup`, `context`, mutex или другую явную модель завершения.

## 4. Timer: одноразовое событие в будущем

`time.Timer` нужен, когда тебе надо один раз дождаться момента в будущем:

```go
timer := time.NewTimer(500 * time.Millisecond)
defer timer.Stop()

select {
case <-timer.C:
	fmt.Println("timeout")
}
```

Timer - одноразовый. Он "выстреливает" один раз, отправляя время в канал `C`.
После этого его можно остановить или переиспользовать через `Reset`, но
переиспользование требует аккуратности.

Типовой service-layer паттерн:

```go
func callWithTimeout(ctx context.Context, fn func(context.Context) error) error {
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()

	done := make(chan error, 1)
	go func() {
		done <- fn(ctx)
	}()

	select {
	case err := <-done:
		return err
	case <-timer.C:
		return errors.New("operation timeout")
	case <-ctx.Done():
		return ctx.Err()
	}
}
```

Но в реальном коде для внешних вызовов чаще правильнее не создавать timer
вручную, а дать timeout через `context.WithTimeout` и передать `ctx` вниз:

```go
ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
defer cancel()

return client.Do(ctx, req)
```

Почему? Потому что ручной timer только сообщает тебе "время вышло". А context
может реально донести cancellation до HTTP/gRPC/SQL-клиента, если тот умеет
принимать `ctx`.

## 5. time.After: удобный одноразовый timeout

`time.After(d)` возвращает канал, который станет готов через `d`:

```go
select {
case v := <-resultCh:
	return v, nil
case <-time.After(300 * time.Millisecond):
	return "", errors.New("timeout")
}
```

Для одноразового `select` это нормально и читаемо.

Исторически в Go часто говорили: "не используй `time.After` в цикле, потому что
оно создает timer, который нельзя остановить". В современных Go, начиная с
Go 1.23, runtime/GC лучше умеют подбирать неиспользуемые timers/tickers. Но для
сервисного кода практическое правило все равно остается полезным:

- для одного timeout в одном `select` - `time.After` ок;
- для горячего цикла - лучше `time.NewTimer` или `time.NewTicker`;
- если нужно управлять жизненным циклом - создавай timer/ticker явно и вызывай
  `Stop`;
- если нужно отменять внешний вызов - чаще нужен `context.WithTimeout`, а не
  `time.After`.

Плохой вариант в горячем цикле:

```go
for {
	select {
	case item := <-items:
		process(item)
	case <-time.After(time.Second):
		flush()
	}
}
```

Лучше:

```go
ticker := time.NewTicker(time.Second)
defer ticker.Stop()

for {
	select {
	case item := <-items:
		process(item)
	case <-ticker.C:
		flush()
	}
}
```

## 6. Ticker: периодическая работа

`time.Ticker` нужен, когда событие должно происходить регулярно:

```go
ticker := time.NewTicker(10 * time.Second)
defer ticker.Stop()

for {
	select {
	case <-ticker.C:
		refresh()
	case <-ctx.Done():
		return ctx.Err()
	}
}
```

Ticker - это не "запустить функцию каждые 10 секунд параллельно". Это канал, из
которого ты читаешь ticks. Если обработка одного tick занимает дольше периода,
ticks не превращаются в бесконечную очередь задач. Поэтому важно понимать:
ticker подходит для "попробовать сделать периодическую работу", но не для
точного планировщика с гарантированной обработкой каждого интервала.

Где ticker уместен:

- периодически обновлять cache snapshot;
- чистить expired keys в TTL-cache;
- flush накопленных событий;
- polling внешней системы;
- health-check loop;
- background metrics collection.

Где ticker часто опасен:

- когда нет shutdown через `ctx.Done()`;
- когда refresh может зависнуть навсегда;
- когда каждый tick запускает новую goroutine без ограничения;
- когда периодическая работа пишет shared state без mutex/atomic;
- когда ошибка refresh стирает последнее хорошее значение.

## 7. Context: cancellation, deadline, timeout

`context.Context` переносит через границы API:

- cancellation signal;
- deadline;
- request-scoped values.

На собеседовании лучше говорить так:

> Context - это стандартный способ сообщить работе, что она больше не нужна:
> клиент отключился, deadline истек, родительская операция отменена, сервис
> завершает shutdown.

Базовые правила из официальной документации:

- `ctx` передают первым аргументом: `func Do(ctx context.Context, id string)`;
- `ctx` не хранят в struct как обычное поле сервиса;
- `nil` context не передают, если не знаешь что дать - `context.TODO()`;
- `CancelFunc` надо вызвать, обычно через `defer cancel()`;
- `context.Value` используют только для request-scoped данных, не для опций
  бизнес-логики.

Пример timeout:

```go
func (s *Service) GetUser(ctx context.Context, id string) (User, error) {
	ctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()

	return s.repo.GetUser(ctx, id)
}
```

Важный смысл `defer cancel()`:

- освобождает ресурсы дочернего context;
- останавливает связанный timer;
- убирает ссылку родителя на child context;
- делает код корректным на всех путях возврата.

Для SQL:

```go
ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
defer cancel()

rows, err := db.QueryContext(ctx, query, userID)
```

Для HTTP:

```go
ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
defer cancel()

req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
if err != nil {
	return err
}

resp, err := http.DefaultClient.Do(req)
```

Если внешний клиент не принимает `ctx`, timeout может не остановить реальную
работу. Тогда ты можешь вернуть ошибку вызывающему коду, но тяжелая операция
продолжит выполняться где-то в фоне. Это важное место для ревью.

## 8. Timeout vs deadline

`context.WithTimeout(parent, d)` означает "отмени через `d` от текущего момента".

```go
ctx, cancel := context.WithTimeout(parent, 500*time.Millisecond)
```

`context.WithDeadline(parent, t)` означает "отмени в конкретный момент `t`".

```go
deadline := time.Now().Add(500 * time.Millisecond)
ctx, cancel := context.WithDeadline(parent, deadline)
```

В обычном service layer чаще используют `WithTimeout`, потому что бизнесово
говорят: "на этот внешний вызов даем 300ms". `WithDeadline` удобен, когда общий
deadline уже известен заранее и его надо протащить дальше.

Если у parent context уже есть более ранний deadline, дочерний timeout не
сделает его длиннее. Более ранняя отмена победит.

## 9. TTL cache: expiresAt, mutex и cleanup

TTL-cache - классическая лайвкодинг-задача. Основная идея простая: значение
хранится вместе с моментом истечения.

```go
type entry struct {
	value     string
	expiresAt time.Time
}

type Cache struct {
	mu   sync.RWMutex
	data map[string]entry
	ttl  time.Duration
}
```

`Set` кладет значение и считает `expiresAt`:

```go
func (c *Cache) Set(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[key] = entry{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}
```

`Get` проверяет срок жизни:

```go
func (c *Cache) Get(key string) (string, bool) {
	now := time.Now()

	c.mu.RLock()
	e, ok := c.data[key]
	c.mu.RUnlock()
	if !ok {
		return "", false
	}

	if now.After(e.expiresAt) {
		c.mu.Lock()
		// Повторная проверка нужна, потому что между RUnlock и Lock
		// другой поток мог обновить key.
		if current, ok := c.data[key]; ok && now.After(current.expiresAt) {
			delete(c.data, key)
		}
		c.mu.Unlock()
		return "", false
	}

	return e.value, true
}
```

Почему нельзя удалить expired key прямо под `RLock`? Потому что `RLock` разрешает
только чтение. `map` нельзя писать под read lock. Для удаления нужен `Lock`.

Для периодической очистки можно добавить ticker:

```go
func (c *Cache) StartCleanup(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.deleteExpired(time.Now())
		case <-ctx.Done():
			return
		}
	}
}
```

Ревью-вопросы по TTL-cache:

- есть ли mutex вокруг map?
- удаляет ли код expired values под write lock?
- что происходит при `ttl <= 0`?
- не держим ли lock во время внешнего вызова?
- есть ли остановка cleanup goroutine?
- не сравниваем ли время через Unix seconds, теряя точность?
- не используем ли `time.Sleep` вместо ticker/context?

## 10. Background refresh для highload handler

Типичная задача:

```go
// aiWeatherForecast вычисляет прогноз за ~1 секунду.
// Есть highload RPC/HTTP ручка 10k RPS.
// Нужно реализовать handler.
```

Плохой вариант:

```go
func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, `{"temperature":%d}`, aiWeatherForecast())
}
```

При 10k RPS это означает 10k тяжелых вычислений в секунду. Если одно вычисление
занимает 1 секунду, система быстро захлебнется. Правильная архитектурная идея:
считать прогноз в фоне раз в N секунд, а handler должен только быстро читать
готовый snapshot.

Модель:

```text
background goroutine:
    ticker -> expensive call -> publish new snapshot

handler:
    read snapshot -> return JSON
```

Пример с `atomic.Value`:

```go
type WeatherSnapshot struct {
	Temperature int       `json:"temperature"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type WeatherService struct {
	current atomic.Value // stores WeatherSnapshot
}

func (s *WeatherService) Start(ctx context.Context, interval time.Duration) {
	s.refreshOnce(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.refreshOnce(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (s *WeatherService) refreshOnce(ctx context.Context) {
	// В реальном коде aiWeatherForecast должен принимать ctx.
	temp := aiWeatherForecast()
	s.current.Store(WeatherSnapshot{
		Temperature: temp,
		UpdatedAt:   time.Now(),
	})
}

func (s *WeatherService) Handler(w http.ResponseWriter, r *http.Request) {
	v := s.current.Load()
	if v == nil {
		http.Error(w, "forecast is not ready", http.StatusServiceUnavailable)
		return
	}

	snapshot := v.(WeatherSnapshot)
	_ = json.NewEncoder(w).Encode(snapshot)
}
```

Почему `atomic.Value` здесь уместен:

- handler делает очень частые чтения;
- snapshot заменяется целиком;
- нет сложной частичной мутации;
- читатели не блокируют друг друга;
- важно опубликовать согласованную копию значения.

Если snapshot сложный и содержит map/slice, надо быть осторожным. `atomic.Value`
безопасно публикует саму структуру, но если внутри лежит map или slice и кто-то
потом мутирует underlying data, получится shared mutable state. Для snapshot
лучше публиковать immutable copy.

Альтернатива с mutex:

```go
type WeatherService struct {
	mu      sync.RWMutex
	current WeatherSnapshot
	ready   bool
}
```

Mutex проще объяснить, `atomic.Value` часто лучше подходит для read-mostly
snapshot. На лайвкодинге можно выбрать mutex, если задача не требует
микрооптимизации.

## 11. Ошибка refresh: сохранять последнее хорошее значение

Очень частая бизнесовая ошибка: если новый refresh не удался, код стирает cache
или публикует нулевое значение.

Плохо:

```go
temp, err := client.Forecast(ctx)
if err != nil {
	s.current.Store(WeatherSnapshot{}) // теперь handler отдает мусор
	return
}
```

Лучше:

```go
temp, err := client.Forecast(ctx)
if err != nil {
	s.logger.Warn("forecast refresh failed", "err", err)
	return // оставляем last known good value
}

s.current.Store(WeatherSnapshot{
	Temperature: temp,
	UpdatedAt:   time.Now(),
})
```

На интервью это хороший senior-ish ответ: "при ошибке обновления я не ломаю
ручку для всех клиентов, а оставляю последнее валидное значение, помечаю
staleness и логирую/метричу проблему".

Можно добавить политику stale:

```go
if time.Since(snapshot.UpdatedAt) > 5*time.Minute {
	http.Error(w, "forecast is stale", http.StatusServiceUnavailable)
	return
}
```

Но это уже бизнес-решение. Иногда лучше отдать старые данные, чем 503. Иногда
нельзя отдавать старые данные вообще. Это надо проговаривать.

## 12. Context внутри background refresh

Фоновая goroutine должна уметь завершаться. Минимальный паттерн:

```go
func (s *Service) Start(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.refresh(ctx)
		case <-ctx.Done():
			return
		}
	}
}
```

Но есть нюанс: если `refresh(ctx)` завис на внешнем вызове, shutdown будет ждать,
пока этот вызов завершится. Поэтому на каждый refresh часто делают отдельный
короткий timeout:

```go
func (s *Service) refresh(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()

	value, err := s.client.Load(ctx)
	if err != nil {
		s.logger.Warn("refresh failed", "err", err)
		return
	}

	s.publish(value)
}
```

Здесь parent context отвечает за shutdown всего сервиса, а child timeout
ограничивает конкретную попытку refresh.

## 13. Delayed flush: когда нужен Timer, а не Ticker

Иногда надо не "каждые 10 секунд", а "через 10 секунд после первого события".
Например, батчить события:

- пришел первый event;
- запускаем timer на 10 секунд;
- собираем события в batch;
- если batch достиг размера N, flush сразу;
- если timer сработал раньше, flush по времени.

Здесь `Ticker` не идеален: он тикает независимо от того, есть ли работа. Лучше
`Timer`, который включается только когда batch становится непустым.

Схема:

```text
batch empty -> timer stopped/nil
first item  -> start timer
more items  -> append
batch full  -> stop timer, flush
timer fires -> flush by time
ctx done    -> final flush, exit
```

Такую задачу любят на лайвкодинге, потому что она проверяет сразу несколько
вещей: `select`, channel, timer lifecycle, slice, shutdown и инварианты.

Упрощенный каркас:

```go
func runBatcher(ctx context.Context, in <-chan Event, maxSize int, maxDelay time.Duration) {
	var batch []Event
	var timer *time.Timer
	var timerC <-chan time.Time

	startTimer := func() {
		timer = time.NewTimer(maxDelay)
		timerC = timer.C
	}

	stopTimer := func() {
		if timer != nil {
			timer.Stop()
			timer = nil
			timerC = nil
		}
	}

	flush := func() {
		if len(batch) == 0 {
			return
		}
		send(batch)
		batch = batch[:0]
		stopTimer()
	}

	for {
		select {
		case e := <-in:
			if len(batch) == 0 {
				startTimer()
			}
			batch = append(batch, e)
			if len(batch) >= maxSize {
				flush()
			}

		case <-timerC:
			flush()

		case <-ctx.Done():
			flush()
			return
		}
	}
}
```

В реальном коде надо обработать закрытие `in`:

```go
case e, ok := <-in:
	if !ok {
		flush()
		return
	}
	// ...
```

## 14. AfterFunc: callback по таймеру

`time.AfterFunc(d, f)` запускает функцию `f` в отдельной goroutine после `d`:

```go
timer := time.AfterFunc(time.Second, func() {
	fmt.Println("fired")
})
defer timer.Stop()
```

Это удобно для редких callback-сценариев, но в обычном service layer чаще
прозрачнее использовать `Timer` + `select`. У `AfterFunc` есть нюансы:

- callback выполняется отдельно;
- `Stop` не ждет завершения callback, если он уже начался;
- `Reset` может запланировать новый запуск, но не гарантирует, что прошлый
  callback уже завершился;
- shared state внутри callback требует обычной синхронизации.

На лайвкодинге `AfterFunc` редко нужен. Если не уверен, выбирай `Timer`.

## 15. Runtime под капотом: что происходит с timers

Когда ты вызываешь `time.Sleep`, `time.After`, `time.NewTimer` или
`time.NewTicker`, Go не создает отдельный OS thread на каждый timer. Runtime
регистрирует timer во внутренней структуре и паркует goroutine, если она ждет.
Когда время наступает, runtime делает соответствующее событие готовым:

- goroutine после `Sleep` может продолжить выполнение;
- канал timer/ticker становится готов к receive;
- callback `AfterFunc` запускается в goroutine.

Важно понимать именно это:

```text
time.Sleep / <-timer.C / <-ticker.C
не сжигают CPU в цикле ожидания
```

Но timers все равно не бесплатны. Если в горячем цикле создавать тысячи timers
без нужды, это увеличит нагрузку на runtime и GC. Поэтому в highload-коде
важнее проектировать поток данных, а не просто "поставить timeout везде".

## 16. Synchronization: time не делает данные безопасными

Timer/ticker решают вопрос "когда". Они не решают вопрос "как безопасно читать и
писать shared state".

Ошибка:

```go
var temperature int

go func() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for range ticker.C {
		temperature = aiWeatherForecast() // write
	}
}()

http.HandleFunc("/weather", func(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, temperature) // read без синхронизации
})
```

Это data race: одна goroutine пишет, другая читает.

Варианты исправления:

- `sync.RWMutex`;
- `atomic.Int64`/`atomic.Value`;
- ownership через одну goroutine и channel;
- immutable snapshot publication.

Для одного `int` можно:

```go
var temperature atomic.Int64

temperature.Store(int64(aiWeatherForecast()))
fmt.Fprintln(w, temperature.Load())
```

Для структуры:

```go
var snapshot atomic.Value // stores WeatherSnapshot
snapshot.Store(WeatherSnapshot{Temperature: 10, UpdatedAt: time.Now()})
```

Нельзя атомарно безопасно мутировать map/slice внутри snapshot после публикации.
Если данные сложные, публикуй копию и дальше ее не меняй.

## 17. Выбор инструмента

Короткая шпаргалка:

| Задача | Инструмент |
|---|---|
| Измерить latency | `start := time.Now(); time.Since(start)` |
| Подождать в демо-коде | `time.Sleep(d)` |
| Один timeout в `select` | `time.After(d)` |
| Управляемый одноразовый timeout | `time.NewTimer(d)` |
| Регулярная работа | `time.NewTicker(interval)` |
| Timeout внешнего HTTP/SQL/gRPC вызова | `context.WithTimeout` + передать `ctx` |
| Deadline уже известен | `context.WithDeadline` |
| TTL value | `expiresAt time.Time` |
| Background refresh | goroutine + ticker + ctx shutdown + synchronized snapshot |
| Отмена всей операции | `ctx.Done()` |
| Передать user id/logger trace id | осторожно `context.Value`, только request-scoped |

## 18. Что смотреть на ревью

Первый вопрос: "эта работа должна происходить один раз, периодически или до
отмены?" От этого зависит выбор `Timer`, `Ticker` или `context`.

Второй вопрос: "кто владеет жизненным циклом goroutine?" Если есть `go func`,
должен быть понятный путь остановки: `ctx.Done()`, закрытие channel, возврат по
ошибке, `WaitGroup` на shutdown.

Третий вопрос: "что происходит с shared state?" Если ticker обновляет map,
slice, struct или int, а handler читает, нужна синхронизация.

Четвертый вопрос: "есть ли timeout у внешнего вызова?" Внешний HTTP/SQL/gRPC
без context timeout может зависнуть дольше, чем живет запрос.

Пятый вопрос: "что происходит при ошибке refresh?" Часто правильная политика -
оставить last known good value, залогировать ошибку и пометить staleness.

Шестой вопрос: "не держится ли mutex во время внешнего вызова?" Это может
заблокировать всех читателей/писателей на время сети.

Плохой refresh:

```go
s.mu.Lock()
defer s.mu.Unlock()

value, err := s.client.Load(ctx) // внешний вызов под lock
if err != nil {
	return err
}
s.value = value
```

Лучше:

```go
value, err := s.client.Load(ctx)
if err != nil {
	return err
}

s.mu.Lock()
s.value = value
s.mu.Unlock()
```

## 19. Частые ошибки

`time.Sleep(10)` вместо `10*time.Second`.

`time.Tick` в долгоживущем service code без возможности остановки. В новых Go
GC лучше справляется с неиспользуемыми tickers, но `time.Tick` все равно не дает
явного `Stop`, поэтому для сервисного lifecycle обычно лучше `NewTicker`.

Ticker loop без `ctx.Done()`:

```go
for range ticker.C {
	refresh()
}
```

Такую goroutine трудно остановить.

`context.WithTimeout` без `defer cancel()`:

```go
ctx, _ := context.WithTimeout(ctx, time.Second)
```

Это утечка ресурсов дочернего context до отмены родителя или истечения timeout.

Передача `context.Background()` вниз вместо request `ctx`:

```go
s.repo.GetUser(context.Background(), id)
```

Так код отрывает работу от cancellation запроса.

Внешний вызов прямо в highload handler:

```go
fmt.Fprintf(w, "%d", aiWeatherForecast())
```

Если вызов дорогой и результат можно переиспользовать, нужен cache/background
refresh.

Обновление cache без синхронизации:

```go
current = value // write in background goroutine
return current  // read in handler
```

Это data race.

Полная перезапись cache нулем при ошибке:

```go
if err != nil {
	current = Snapshot{}
}
```

Часто лучше сохранить last known good value.

Использование `context.Value` для опций:

```go
ctx = context.WithValue(ctx, "timeout", 5*time.Second)
```

Timeout должен быть в `context.WithTimeout`, а не в value.

## 20. Как отвечать на собеседовании

Если спрашивают: "как сделать ручку, которая отдает результат тяжелого
вычисления при большой нагрузке?", хороший ответ:

> Я не буду делать тяжелое вычисление на каждый запрос. Если данные могут быть
> слегка устаревшими, вынесу вычисление в background goroutine, которая по
> `time.Ticker` обновляет snapshot. Handler будет только читать готовое значение
> под `RWMutex` или через `atomic.Value`. У background loop будет `ctx.Done()` для
> shutdown, у каждого внешнего вызова будет `context.WithTimeout`. При ошибке
> refresh я сохраню последнее хорошее значение и добавлю метрику/лог, а политику
> stale данных отдельно согласую с бизнесом.

Если спрашивают: "чем отличается Timer от Ticker?", можно:

> `Timer` - одноразовое событие через duration. Он нужен для timeout, delayed
> flush, debounce, ожидания одного события. `Ticker` - периодические ticks через
> interval, он подходит для cleanup, polling, periodic refresh. Оба надо
> привязывать к жизненному циклу: в сервисном коде обычно есть `ctx.Done()` и
> явный `Stop`.

Если спрашивают: "зачем context, если есть time.After?", можно:

> `time.After` только сообщает текущему select, что прошло время. Он не отменяет
> сам внешний запрос. `context.WithTimeout` передает cancellation вниз в HTTP,
> SQL, gRPC и другие API, которые принимают `ctx`. Поэтому для внешних вызовов
> timeout обычно делают через context.

## 21. Мини-чеклист для лайвкодинга

Перед тем как писать код со временем, проговори:

```text
1. Это one-shot timeout или periodic job?
2. Кто остановит goroutine?
3. Нужен ли context timeout для внешнего вызова?
4. Как защищены данные между background goroutine и handler?
5. Что делаем при ошибке refresh?
6. Можно ли отдавать stale данные и как долго?
7. Не держим ли mutex во время сети/диска?
8. Все ли durations указаны с единицами: time.Second, time.Millisecond?
```

Эта тема часто выглядит как "просто поставить ticker", но на самом деле
интервьюер проверяет инженерную зрелость: lifecycle, backpressure, shared state,
timeout, деградацию при ошибках и простую, объяснимую модель данных.
