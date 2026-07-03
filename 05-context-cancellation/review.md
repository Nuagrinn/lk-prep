# Context: cancellation, deadline, request scope

Эта тема про `context.Context` в Go: отмену операций, deadline, timeout, request scope, propagation между слоями, graceful shutdown и типичные ошибки в сервисном коде.

На первый взгляд `context` выглядит просто: передали `ctx` первым аргументом, где-то сделали `select { case <-ctx.Done(): ... }`, и готово. Но на code review context почти всегда раскрывает более глубокие вопросы:

- кто владеет жизненным циклом операции;
- должна ли операция отмениться вместе с HTTP-запросом;
- есть ли deadline у внешних вызовов;
- не потеряли ли мы cancellation через `context.Background()`;
- не тащим ли request context в фоновую goroutine;
- возвращаем ли `ctx.Err()`;
- вызываем ли `cancel`;
- не храним ли context в struct;
- не используем ли context как мешок для параметров;
- умеет ли worker остановиться;
- не обрываем ли критичный бизнес-процесс из-за закрытого клиентского соединения.

Главная мысль:

> Context - это контракт жизненного цикла операции. Он отвечает на вопрос: “когда эта работа больше не нужна или больше не должна продолжаться?”

И вторая мысль:

> В сервисном слое важно различать request scope, operation scope, worker scope и process shutdown scope. Ошибки часто появляются, когда один scope случайно используют вместо другого.

## Простое введение

Представим HTTP handler:

```go
func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	order, err := h.orders.CreateOrder(r.Context(), cmd)
	...
}
```

`r.Context()` отменится, если:

- клиент закрыл соединение;
- HTTP server завершает request;
- истек deadline, если он задан middleware/server-ом;
- server shutdown отменил request.

Это хорошо: если клиент ушел, нам часто не нужно дальше читать БД, считать ответ и писать в сеть.

Но теперь представь, что внутри `CreateOrder` мы делаем:

```go
func (s *OrderService) CreateOrder(ctx context.Context, cmd CreateOrderCommand) error {
	if err := s.repo.SaveOrder(ctx, order); err != nil {
		return err
	}

	go s.events.Publish(ctx, "order.created", order.ID)
	return nil
}
```

Здесь request context попал в goroutine. Handler вернул ответ, request завершился, context отменился. Публикация события может оборваться. Если событие важно, мы получим странный баг: заказ создан, но событие иногда не уходит.

Другой плохой вариант:

```go
func (s *OrderService) CreateOrder(ctx context.Context, cmd CreateOrderCommand) error {
	return s.repo.SaveOrder(context.Background(), order)
}
```

Теперь входной `ctx` игнорируется. Клиент отменил запрос или deadline истек, но операция продолжает держать БД, транзакцию, locks, соединения.

Обе ошибки противоположные:

- в первом случае мы слишком сильно привязали фоновую работу к request scope;
- во втором случае мы полностью потеряли cancellation.

Хороший ревью-комментарий:

> Здесь надо явно выбрать scope операции. Синхронные DB/HTTP вызовы в рамках request должны использовать входной `ctx`, но долгую фоновую работу нельзя запускать на request context. Для нее нужен worker/outbox/service context и отдельный timeout.

## Что такое context

`context.Context` - это интерфейс из стандартной библиотеки:

```go
type Context interface {
	Deadline() (deadline time.Time, ok bool)
	Done() <-chan struct{}
	Err() error
	Value(key any) any
}
```

Он передает:

- cancellation signal через `Done()`;
- deadline через `Deadline()`;
- причину отмены через `Err()`;
- request-scoped values через `Value()`.

Context обычно создается наверху:

- HTTP server создает request context;
- gRPC создает RPC context;
- worker создает context жизненного цикла;
- CLI/root process создает root context;
- тест создает context с timeout.

И дальше context прокидывается вниз:

```go
handler -> service -> repository -> db driver
handler -> service -> external client -> http request
worker -> service -> repository
```

Context не должен быть “глобальной переменной”. Он описывает конкретную операцию.

## Базовые правила Go context

### 1. Context первым аргументом

Обычно:

```go
func (s *Service) Do(ctx context.Context, req Request) error
```

Не так:

```go
func (s *Service) Do(req Request, ctx context.Context) error
```

Это общепринятая Go-конвенция. Она облегчает чтение и поиск.

### 2. Не хранить context в struct

Плохо:

```go
type Service struct {
	ctx context.Context
}
```

Почему плохо:

- непонятно, к какой операции относится ctx;
- можно случайно переиспользовать отмененный request context;
- разные методы сервиса получают один и тот же scope;
- появляются data/lifecycle bugs;
- тесты становятся странными.

Исключение: long-lived component может хранить context жизненного цикла внутри, если это явно part of lifecycle:

```go
type Worker struct {
	ctx    context.Context
	cancel context.CancelFunc
}
```

Но это не request context. Это context самого worker-а/service-а.

На ревью:

> Я бы не хранил request context в struct. Context должен передаваться в конкретный вызов. Если нужен lifecycle worker-а, это должен быть отдельный internal context с cancel.

### 3. Не передавать nil context

Плохо:

```go
s.repo.GetOrder(nil, id)
```

Если нет context-а, лучше:

```go
context.Background()
```

или в тестах:

```go
context.TODO()
```

Но `Background()` в середине бизнес-операции почти всегда подозрителен. Он уместен как root context, а не как способ “отвязаться от отмены”.

### 4. Не использовать context для обычных параметров

Плохо:

```go
ctx = context.WithValue(ctx, "limit", 100)
ctx = context.WithValue(ctx, "userID", userID)
```

Обычные параметры должны быть параметрами или command struct:

```go
type ListOrdersQuery struct {
	UserID string
	Limit  int
}
```

Context values подходят для request-scoped metadata, которая проходит через много слоев:

- trace id;
- request id;
- auth principal, если в проекте так принято;
- logger fields;
- locale, иногда;
- tenant id, если это архитектурно принято.

Но даже auth/tenant часто лучше передавать явно в command, если это бизнес-значимые данные.

На ревью:

> Context.Value не должен быть скрытым API метода. Если userID нужен бизнес-логике, лучше передать его явно.

### 5. Всегда вызывать cancel

Плохо:

```go
ctx, cancel := context.WithTimeout(parent, time.Second)
do(ctx)
```

Нужно:

```go
ctx, cancel := context.WithTimeout(parent, time.Second)
defer cancel()

do(ctx)
```

Почему:

- освобождаются timer resources;
- дочерние context-и получают сигнал;
- хорошая привычка даже если deadline истечет сам.

На ревью:

> После `context.WithTimeout/WithCancel/WithDeadline` cancel должен быть вызван, обычно через `defer cancel()`.

## Cancellation

Cancellation означает: работа больше не нужна или не должна продолжаться.

Проверка:

```go
select {
case <-ctx.Done():
	return ctx.Err()
default:
}
```

В долгих циклах:

```go
for _, item := range items {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := process(ctx, item); err != nil {
		return err
	}
}
```

Для блокирующих операций:

```go
select {
case jobs <- job:
	return nil
case <-ctx.Done():
	return ctx.Err()
}
```

Важно: context не убивает goroutine магически. Код сам должен:

- передавать ctx в I/O функции;
- проверять `ctx.Done()` в loops/selects;
- завершаться при `ctx.Err()`.

Если функция не смотрит на ctx, cancellation не сработает.

## Deadline и Timeout

Deadline - абсолютное время:

```go
ctx, cancel := context.WithDeadline(parent, deadline)
```

Timeout - относительная длительность:

```go
ctx, cancel := context.WithTimeout(parent, 3*time.Second)
```

Обычно используют `WithTimeout`.

Пример внешнего вызова:

```go
func (s *Service) callProvider(ctx context.Context, req Request) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	return s.provider.Call(ctx, req)
}
```

Зачем отдельный timeout, если у request уже есть deadline?

- внешний сервис может иметь свой SLA;
- не хотим тратить весь request budget на один вызов;
- хотим быстро fallback-нуться;
- хотим ограничить worker job.

Но timeout надо выбирать осознанно. Плохо:

```go
context.WithTimeout(ctx, 10*time.Minute)
```

в request path без причины.

И плохо:

```go
context.WithTimeout(ctx, 50*time.Millisecond)
```

если downstream обычно отвечает 100 ms. Это даст ложные ошибки.

На ревью:

> Здесь нужен понятный timeout budget: сколько времени весь use case может выполняться, и сколько мы готовы ждать конкретный внешний dependency.

## Request scope

Request scope - это жизненный цикл одного входящего запроса.

HTTP:

```go
ctx := r.Context()
```

gRPC:

```go
func (s *Server) Method(ctx context.Context, req *pb.Request) (*pb.Response, error)
```

Request context хорошо подходит для:

- чтения входных данных;
- обращения в БД для ответа клиенту;
- синхронных проверок;
- внешних вызовов, результат которых нужен для ответа;
- отмены работы, если клиент ушел.

Request context плохо подходит для:

- фоновой публикации события после ответа;
- долгой генерации отчета;
- retry worker-а;
- periodic task;
- операции, которая должна завершиться независимо от клиента;
- cleanup процесса.

Плохой пример:

```go
func (s *Service) CreateReport(ctx context.Context, cmd Cmd) error {
	go s.generateReport(ctx, cmd.ReportID)
	return nil
}
```

Handler вернул ответ, request context отменился, report generation может остановиться.

Лучше:

- создать job в БД;
- отправить outbox event;
- worker под своим context заберет job;
- request context используется только для записи job.

## Operation scope

Иногда надо создать context не на весь request, а на конкретную операцию.

Пример:

```go
func (s *Service) EnrichProfile(ctx context.Context, userID string) (Profile, error) {
	user, err := s.users.Get(ctx, userID)
	if err != nil {
		return Profile{}, err
	}

	providerCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	score, err := s.scoring.GetScore(providerCtx, userID)
	if err != nil {
		score = DefaultScore
	}

	return Profile{User: user, Score: score}, nil
}
```

Весь request может иметь deadline 2 секунды, но scoring мы готовы ждать только 500 ms.

Важно: дочерний context не может жить дольше родительского. Если `ctx` уже отменен, `providerCtx` тоже отменится.

## Worker scope

Worker scope - это context жизненного цикла фонового worker-а.

```go
type Worker struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}
```

Worker должен:

- завершаться при `ctx.Done()`;
- передавать ctx или per-job timeout вниз;
- освобождать ресурсы;
- быть ожидаемым через `WaitGroup`;
- не зависеть от request context.

Пример:

```go
func (w *Worker) run() {
	defer w.wg.Done()

	for {
		select {
		case job := <-w.jobs:
			w.handleJob(w.ctx, job)
		case <-w.ctx.Done():
			return
		}
	}
}
```

Для конкретной job:

```go
func (w *Worker) handleJob(parent context.Context, job Job) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()

	_ = w.sender.Send(ctx, job)
}
```

На ревью:

> Для фоновой работы нужен context жизненного цикла worker-а, а request context должен использоваться только для постановки задачи.

## Process shutdown scope

При остановке приложения root context отменяется:

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()
```

Дальше:

- HTTP server получает shutdown;
- workers получают cancel;
- queues перестают принимать новые задачи;
- сервисы drain-ят или останавливают jobs;
- `Close(ctx)` получает отдельный timeout.

Пример:

```go
shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

if err := app.Shutdown(shutdownCtx); err != nil {
	logger.Error("shutdown failed", "error", err)
}
```

Здесь `context.Background()` как root для shutdown timeout нормален: это новая верхнеуровневая операция завершения процесса.

## `context.Background()` и `context.TODO()`

`context.Background()`:

- root context;
- никогда не отменяется;
- без deadline;
- без values.

Уместен:

- в `main`;
- в tests как root;
- для initialization root;
- для process-level shutdown context;
- как parent для long-lived service context.

Подозрителен:

- внутри service method, который уже получил `ctx`;
- внутри repository;
- внутри external client;
- внутри transaction;
- внутри worker вместо lifecycle context.

`context.TODO()`:

- временная заглушка;
- означает “здесь context пока не продуман”.

В production-коде лучше не оставлять без причины.

На ревью:

> `context.Background()` посреди use case обычно ломает propagation cancellation/deadline. Лучше передать входной ctx или явно создать отдельный worker/lifecycle context.

## Propagation между слоями

Правильный поток:

```text
handler ctx
  -> service ctx
    -> repository ctx
      -> db.QueryContext(ctx, ...)
    -> external client ctx
      -> http.NewRequestWithContext(ctx, ...)
```

Плохо:

```go
func (r *Repo) GetOrder(ctx context.Context, id string) (Order, error) {
	return r.db.QueryRow("select ...", id).Scan(...)
}
```

Если используется `QueryRow`, а не `QueryRowContext`, database call может не отмениться по ctx.

Лучше:

```go
r.db.QueryRowContext(ctx, "select ...", id)
```

HTTP client:

```go
req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
```

gRPC обычно принимает ctx первым аргументом:

```go
client.GetUser(ctx, req)
```

На ревью:

> Метод принимает ctx, но дальше вызывает API без context-aware варианта. Тогда cancellation фактически теряется.

## Context и транзакции

Транзакционные методы должны получать ctx:

```go
tx, err := db.BeginTx(ctx, nil)
```

И queries внутри:

```go
tx.ExecContext(ctx, ...)
```

Плохо:

```go
repo.SaveOrder(context.Background(), tx, order)
```

Если request отменился, транзакция продолжит работать.

Но есть тонкость: если контекст отменился после того, как операция прошла важную бизнес-точку, иногда нельзя просто бросить все в неопределенном состоянии. Это решается не `Background()` случайно, а архитектурой:

- статусная модель;
- outbox;
- worker;
- идемпотентность;
- reconciliation.

На ревью:

> Если операция должна пережить отмену HTTP-запроса, это должен быть явный async workflow. Просто заменить ctx на Background внутри транзакции - плохой способ.

## Context и external calls

Каждый внешний вызов должен иметь deadline/timeout.

Плохо:

```go
resp, err := http.DefaultClient.Do(req)
```

если `req` без context и client без timeout.

Лучше:

```go
ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
defer cancel()

req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
if err != nil {
	return err
}

resp, err := s.http.Do(req)
```

В Go еще есть `http.Client{Timeout: ...}`. Он ограничивает весь request. Это полезно, но context все равно нужен для propagation отмены от parent.

На ревью:

> Внешний вызов должен быть ограничен deadline-ом. Иначе один зависший dependency может держать goroutine, соединение, транзакцию или worker бесконечно.

## Возвращать `ctx.Err()`

Плохо:

```go
select {
case <-ctx.Done():
	return nil
}
```

Caller не поймет, операция успешно завершилась или была отменена.

Лучше:

```go
case <-ctx.Done():
	return ctx.Err()
```

`ctx.Err()` возвращает:

- `context.Canceled`;
- `context.DeadlineExceeded`.

Это важно для:

- логирования;
- HTTP/gRPC mapping;
- retry decisions;
- метрик.

На ревью:

> При cancellation надо возвращать `ctx.Err()`, а не nil/общую ошибку, чтобы caller мог отличить cancel от deadline.

## `context.Canceled` vs `context.DeadlineExceeded`

`context.Canceled`:

- кто-то явно отменил context;
- клиент ушел;
- parent cancel вызван.

`context.DeadlineExceeded`:

- истек deadline/timeout.

Для observability это разные причины.

Например:

- `Canceled` от клиента не всегда ошибка сервера;
- `DeadlineExceeded` может означать latency issue;
- при shutdown `Canceled` ожидаем.

На ревью:

> Не все context errors надо логировать как internal error. Client cancellation и deadline требуют разной интерпретации.

## `context.WithCancelCause`

В новых Go версиях есть cancellation cause:

```go
ctx, cancel := context.WithCancelCause(parent)
cancel(ErrQueueClosed)

cause := context.Cause(ctx)
```

Это полезно, когда хочется передать причину отмены глубже, чем просто `context.Canceled`.

Например:

- service shutting down;
- queue closed;
- dependency failed;
- parent workflow failed.

Но в простом сервисном коде обычный `WithCancel/WithTimeout` чаще достаточно.

На собесе можно упомянуть как дополнительный инструмент, но не обязательно тащить в каждый ответ.

## Context values

Правильные ключи:

```go
type requestIDKey struct{}

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}
```

Плохо:

```go
context.WithValue(ctx, "request_id", id)
```

Строковые ключи могут конфликтовать между пакетами.

Что можно хранить:

- request id;
- trace/span;
- logger;
- auth principal, если команда проекта так делает;
- tenant metadata, если это инфраструктурный context.

Что не надо:

- limit;
- page;
- order id;
- payment amount;
- business flags;
- optional behavior mode.

Это должно быть в request/command struct.

## Context и logger/tracing

Часто logger/tracer привязан к ctx:

```go
logger := log.FromContext(ctx)
span := trace.SpanFromContext(ctx)
```

Это нормально: observability metadata следует за request.

Важно:

- не заменять ctx на Background и не терять trace;
- не создавать новый root span без причины;
- не логировать context cancellation как panic-level;
- пробрасывать ctx в external clients, чтобы trace propagation работал.

На ревью:

> Если здесь используется `context.Background()`, мы теряем не только cancellation, но и tracing/request id.

## Типовые ошибки

### Ошибка 1. Метод принимает ctx, но игнорирует его

```go
func (s *Service) Save(ctx context.Context, item Item) error {
	return s.repo.Save(context.Background(), item)
}
```

Проблемы:

- deadline не работает;
- client cancellation не работает;
- tracing теряется;
- операция может держать ресурсы после отмены.

Как лучше:

```go
return s.repo.Save(ctx, item)
```

### Ошибка 2. Repository использует не context-aware DB methods

```go
row := db.QueryRow("select ...")
```

Вместо:

```go
row := db.QueryRowContext(ctx, "select ...")
```

### Ошибка 3. HTTP request без context

```go
req, _ := http.NewRequest(http.MethodPost, url, body)
resp, err := client.Do(req)
```

Лучше:

```go
req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
```

### Ошибка 4. Request context используется в goroutine после ответа

```go
go s.sendEmail(ctx, email)
```

После завершения request ctx отменится. Email может не уйти.

Как лучше:

- outbox;
- queue;
- worker context;
- explicit async job.

### Ошибка 5. Background используется для “чтобы не отменилось”

```go
go s.sendEmail(context.Background(), email)
```

Теперь работа вообще не управляется lifecycle-ом:

- shutdown ее не остановит;
- timeout нет;
- ошибки теряются;
- trace теряется.

Лучше service/worker context + per-job timeout.

### Ошибка 6. Не вызвали cancel

```go
ctx, cancel := context.WithTimeout(ctx, time.Second)
return s.client.Call(ctx)
```

Нужно:

```go
ctx, cancel := context.WithTimeout(ctx, time.Second)
defer cancel()
return s.client.Call(ctx)
```

### Ошибка 7. Возврат nil при cancellation

```go
case <-ctx.Done():
	return nil
```

Лучше:

```go
return ctx.Err()
```

### Ошибка 8. Context хранится в struct

```go
type Service struct {
	ctx context.Context
}
```

Для request context - плохо.

Для worker lifecycle context - возможно, если это явно часть lifecycle.

### Ошибка 9. Context используется для business params

```go
userID := ctx.Value("userID").(string)
```

Если userID нужен бизнес-логике, передать явно.

### Ошибка 10. Создается timeout поверх уже истекшего context-а без проверки

```go
ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
defer cancel()

do(ctx)
```

Если parent уже canceled, child тоже canceled. Это нормально, но иногда код не ожидает немедленной отмены.

На ревью можно спросить:

> Что должно быть, если caller передал уже canceled ctx?

### Ошибка 11. Долгий CPU-loop не проверяет ctx

```go
for _, row := range hugeRows {
	process(row)
}
```

Если `process` CPU-bound и долго работает, cancellation не сработает.

Нужно периодически проверять:

```go
if err := ctx.Err(); err != nil {
	return err
}
```

### Ошибка 12. Blocking channel send/receive без ctx

```go
jobs <- job
```

или:

```go
result := <-results
```

Если никто не читает/пишет, зависание.

Лучше:

```go
select {
case jobs <- job:
case <-ctx.Done():
	return ctx.Err()
}
```

### Ошибка 13. Timeout создается слишком низко в стеке

Если repository сам делает:

```go
ctx, cancel := context.WithTimeout(context.Background(), time.Second)
```

то service не может управлять budget-ом операции. Лучше timeout задавать на уровне use case/external client policy, а repository уважает полученный ctx.

### Ошибка 14. Все ошибки context логируются как server error

```go
logger.Error("failed", "err", err)
```

Если `errors.Is(err, context.Canceled)`, это может быть нормальный client disconnect.

Надо классифицировать.

### Ошибка 15. Слишком общий timeout

Один timeout на весь сервис:

```go
const timeout = 30 * time.Second
```

и везде одинаково.

У разных операций разные budgets. Read query, payment call, report generation, cache get - это разные сценарии.

## Как избегать проблем

### Правило 1. Передавать ctx вниз без подмены

Если функция получила ctx, обычно она передает этот же ctx дальше:

```go
s.repo.Save(ctx, item)
s.client.Call(ctx, req)
```

Подмена на Background должна быть редким и явно объяснимым решением.

### Правило 2. Разделять scopes

Запомнить:

- request ctx - для работы, нужной ответу;
- operation ctx - для конкретного шага с timeout;
- worker ctx - для фоновой обработки;
- shutdown ctx - для завершения процесса;
- Background - root, а не escape hatch.

### Правило 3. Для async использовать очередь/worker/outbox

Плохо:

```go
go doAsync(ctx)
```

Лучше:

- поставить durable job;
- записать outbox event;
- отправить в worker с его lifecycle context;
- сделать retry/idempotency.

### Правило 4. Всегда закрывать cancel

После `WithCancel/WithTimeout/WithDeadline`:

```go
defer cancel()
```

Если cancel надо вызвать раньше, можно:

```go
cancel()
```

но важно не забыть.

### Правило 5. Возвращать `ctx.Err()`

При отмене:

```go
return ctx.Err()
```

Не скрывать cancellation за nil или generic error.

### Правило 6. Контролировать timeout budgets

На уровне use case:

```go
whole operation <= 2s
db read <= parent ctx
provider A <= 500ms
provider B <= 300ms
cache <= 50ms
```

Не обязательно везде руками создавать timeout. Но надо понимать, где они задаются: server, client, middleware, config.

### Правило 7. Проверять ctx в long loops

Для CPU или batch:

```go
for i, item := range items {
	if i%100 == 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	process(item)
}
```

Не обязательно проверять на каждой итерации, если итерации очень дешевые. Но cancellation должна быть отзывчивой.

### Правило 8. Не использовать context as optional params

Если метод меняет поведение от значения в ctx, это скрытый API. Лучше command struct.

## Диагностика на ревью

### Шаг 1. Найти все context usage

Поиск:

```text
context.Background
context.TODO
WithTimeout
WithCancel
WithDeadline
ctx.Done
ctx.Err
context.WithValue
```

Особенно подозрительны:

- `Background()` внутри service/repo;
- `TODO()` в production;
- `WithTimeout` без `cancel`;
- `Done()` с return nil;
- `WithValue` для business params.

### Шаг 2. Проследить propagation

От handler-а до repo/external client:

```text
r.Context()
 -> service.Method(ctx)
 -> repo.Query(ctx)
 -> db.QueryContext(ctx)
```

Проверь:

- не заменили ли ctx;
- не потеряли ли в helper-е;
- использует ли downstream context-aware API;
- сохраняется ли trace/request id.

### Шаг 3. Найти goroutines

Поиск:

```text
go func
go s.
```

Для каждой:

- какой ctx используется;
- переживает ли goroutine request;
- кто ее отменяет;
- кто ждет завершения;
- куда идут ошибки;
- есть ли timeout.

### Шаг 4. Проверить blocking operations

Ищи:

- channel send/receive;
- select без ctx;
- external HTTP;
- DB query;
- lock wait;
- long loop;
- WaitGroup wait;
- sleep/ticker.

Спроси:

- можно ли выйти при cancel;
- есть ли deadline;
- не зависнет ли shutdown.

### Шаг 5. Проверить error semantics

Если операция отменена:

- возвращается ли `context.Canceled`;
- возвращается ли `context.DeadlineExceeded`;
- не логируется ли как panic/internal;
- правильно ли HTTP/gRPC маппит ошибку.

### Шаг 6. Проверить async business operation

Спроси:

- должна ли операция завершиться, если клиент ушел;
- если да, почему она на request ctx;
- если нет, почему она на Background без lifecycle;
- есть ли durable state/job/outbox.

## Диагностика в runtime

Признаки context-проблем:

- goroutines висят после отмены запроса;
- DB queries продолжаются после client disconnect;
- HTTP calls висят без timeout;
- worker не завершается при shutdown;
- много ошибок `context deadline exceeded`;
- много `context canceled` логируется как 500;
- события иногда не публикуются после успешного ответа;
- фоновые задачи обрываются при завершении request.

Инструменты:

- pprof goroutine profile;
- tracing spans с deadlines;
- logs с request_id/trace_id;
- metrics по timeout/canceled;
- integration tests с canceled context;
- fake clients, которые ждут ctx.Done.

Полезный тест:

```go
ctx, cancel := context.WithCancel(context.Background())
cancel()

err := service.Do(ctx, cmd)
if !errors.Is(err, context.Canceled) {
	t.Fatalf("expected canceled, got %v", err)
}
```

Тест на timeout:

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
defer cancel()

err := service.SlowOperation(ctx)
if !errors.Is(err, context.DeadlineExceeded) {
	t.Fatalf("expected deadline exceeded, got %v", err)
}
```

## Context и HTTP/gRPC mapping

HTTP:

- `context.Canceled` часто означает клиент ушел. Можно не логировать как 500.
- `context.DeadlineExceeded` может маппиться в 504 Gateway Timeout или 503/500 в зависимости от слоя.

gRPC:

- `context.Canceled` -> `codes.Canceled`;
- `context.DeadlineExceeded` -> `codes.DeadlineExceeded`.

На сервисном уровне лучше не возвращать HTTP status напрямую, но сохранять ошибку так, чтобы transport мог корректно замаппить.

## Context и retries

Retry loop должен уважать ctx:

```go
for attempt := 0; attempt < maxAttempts; attempt++ {
	if err := ctx.Err(); err != nil {
		return err
	}

	err := call(ctx)
	if err == nil {
		return nil
	}

	timer := time.NewTimer(backoff(attempt))
	select {
	case <-timer.C:
	case <-ctx.Done():
		timer.Stop()
		return ctx.Err()
	}
}
```

Плохо:

```go
time.Sleep(backoff)
```

без возможности отмены.

На ревью:

> Retry/backoff должен быть context-aware, иначе отмененный запрос продолжит спать и ретраить.

## Context и locks

`sync.Mutex.Lock()` не принимает ctx. Если горутина ждет mutex, context ее не разбудит.

Поэтому:

- не держать mutex долго;
- не делать I/O под lock;
- для cancelable acquisition иногда нужен channel/semaphore или другой дизайн;
- не думать, что ctx отменит ожидание обычного mutex.

На ревью:

> Context не отменяет ожидание mutex. Поэтому важно не держать lock во время внешних операций.

## Context и каналы

Для send:

```go
select {
case ch <- value:
	return nil
case <-ctx.Done():
	return ctx.Err()
}
```

Для receive:

```go
select {
case value := <-ch:
	return value, nil
case <-ctx.Done():
	return zero, ctx.Err()
}
```

Для worker:

```go
for {
	select {
	case job := <-jobs:
		handle(ctx, job)
	case <-ctx.Done():
		return
	}
}
```

Но если нужно drain jobs, простое `ctx.Done()` может бросить очередь. Это уже shutdown policy.

## Context и `defer cancel` в loops

Плохо:

```go
for _, item := range items {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	process(ctx, item)
}
```

`defer cancel()` выполнится только в конце всей функции, а не итерации. Если items много, timers живут дольше нужного.

Лучше:

```go
for _, item := range items {
	if err := processOne(ctx, item); err != nil {
		return err
	}
}

func processOne(parent context.Context, item Item) error {
	ctx, cancel := context.WithTimeout(parent, time.Second)
	defer cancel()

	return process(ctx, item)
}
```

или явно:

```go
ctx, cancel := context.WithTimeout(parent, time.Second)
err := process(ctx, item)
cancel()
if err != nil {
	return err
}
```

## Context и nil/zero values

Функции не должны принимать nil context. Если это публичный API внутри проекта, можно panic? Обычно лучше не panic, а считать это programmer error и исправить caller.

В тестах:

```go
ctx := context.Background()
```

Не:

```go
var ctx context.Context
```

## Context и бизнес-критичные операции

Самая тонкая часть.

Есть операции, которые нельзя просто оборвать, если клиент ушел:

- создать payment attempt;
- списать деньги;
- завершить транзакционный workflow;
- отправить критичное событие;
- изменить статус после внешнего callback-а.

Но это не значит, что нужно везде использовать `Background()`.

Правильный подход:

1. Синхронная часть request-а делает минимальное durable изменение под request ctx.
2. Дальше процесс продолжается через outbox/worker.
3. Worker имеет свой lifecycle context.
4. Каждый внешний вызов имеет timeout.
5. Операции идемпотентны.

Пример:

```go
func (s *Service) StartPayment(ctx context.Context, cmd Cmd) error {
	return s.tx.Run(ctx, func(ctx context.Context, tx Tx) error {
		if err := s.payments.SaveAttempt(ctx, tx, attempt); err != nil {
			return err
		}
		return s.outbox.Save(ctx, tx, PaymentRequestedEvent(attempt.ID))
	})
}
```

Worker:

```go
func (w *PaymentWorker) Handle(parent context.Context, event Event) error {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()

	return w.gateway.Charge(ctx, ...)
}
```

На ревью:

> Если бизнес-операция должна пережить request cancellation, надо вынести ее в durable async workflow. `context.Background()` внутри request path скрывает проблему, но не решает lifecycle.

## Примеры плохого и хорошего кода

### Плохой вариант

```go
func (s *ReportService) GenerateReport(ctx context.Context, userID string) error {
	report, err := s.repo.CreateReport(context.Background(), userID)
	if err != nil {
		return err
	}

	go func() {
		data, err := s.analytics.LoadData(ctx, userID)
		if err != nil {
			return
		}

		file, err := s.renderer.Render(context.Background(), data)
		if err != nil {
			return
		}

		_ = s.storage.Upload(context.Background(), report.ID, file)
		_ = s.repo.MarkReady(context.Background(), report.ID)
	}()

	return nil
}
```

Проблемы:

- `CreateReport` игнорирует входной ctx;
- goroutine использует request ctx для долгой работы;
- часть вызовов использует `Background`;
- нет lifecycle worker-а;
- ошибки теряются;
- upload/render без управляемого timeout;
- report может навсегда остаться в промежуточном состоянии;
- shutdown не контролирует goroutine;
- tracing теряется.

### Более здоровое направление

```go
func (s *ReportService) StartReport(ctx context.Context, cmd StartReportCommand) (ReportID, error) {
	var reportID ReportID

	err := s.tx.Run(ctx, func(ctx context.Context, tx Tx) error {
		report, err := NewReport(cmd.UserID, s.clock.Now())
		if err != nil {
			return err
		}

		if err := s.reports.Save(ctx, tx, report); err != nil {
			return err
		}

		if err := s.outbox.Save(ctx, tx, ReportGenerationRequested(report.ID)); err != nil {
			return err
		}

		reportID = report.ID
		return nil
	})
	if err != nil {
		return "", err
	}

	return reportID, nil
}
```

Worker:

```go
func (w *ReportWorker) Handle(parent context.Context, event ReportGenerationRequested) error {
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()

	data, err := w.analytics.LoadData(ctx, event.UserID)
	if err != nil {
		return err
	}

	file, err := w.renderer.Render(ctx, data)
	if err != nil {
		return err
	}

	if err := w.storage.Upload(ctx, event.ReportID, file); err != nil {
		return err
	}

	return w.reports.MarkReady(ctx, event.ReportID)
}
```

Здесь:

- request ctx управляет только стартом job;
- дальнейшая работа идет в worker scope;
- timeout задан на job;
- ошибки возвращаются worker-у;
- можно добавить retry/status/outbox;
- tracing можно передать через event metadata, если нужно.

## Мини-чеклист для ревью

- `ctx` идет первым аргументом?
- Метод принимает ctx и реально его использует?
- Нет ли `context.Background()` внутри service/repository?
- Нет ли `context.TODO()` в production path?
- Все DB calls используют `QueryContext/ExecContext`?
- HTTP calls создаются через `NewRequestWithContext`?
- gRPC/external calls получают ctx?
- После `WithTimeout/WithCancel/WithDeadline` вызывается `cancel`?
- Нет ли `defer cancel()` внутри большого loop?
- При `ctx.Done()` возвращается `ctx.Err()`?
- Не хранится ли request ctx в struct?
- Не используется ли ctx для business params?
- Не запускается ли goroutine на request ctx?
- У background worker-а есть свой lifecycle ctx?
- У внешних вызовов есть timeout?
- Retry/backoff уважает ctx?
- Long loop проверяет ctx?
- Channel send/receive учитывает ctx?
- Shutdown context отделен от request context?
- Context errors правильно классифицируются?
- Не теряется ли tracing/request id через Background?
- Если операция должна пережить request cancel, есть ли durable async workflow?

## Вопросы автору кода

- Какой scope у этого context: request, operation, worker или shutdown?
- Что должно произойти, если клиент отменил запрос?
- Должна ли эта операция продолжиться после client disconnect?
- Почему здесь используется `context.Background()`?
- Почему goroutine получает request ctx?
- Где задается deadline для внешнего сервиса?
- Что будет, если downstream зависнет?
- Почему cancel не вызывается?
- Что возвращаем при cancellation?
- Нужно ли отличать `Canceled` от `DeadlineExceeded`?
- Где проверяется ctx в долгом цикле?
- Может ли эта операция оставить частично созданное состояние при отмене?
- Нужен ли здесь outbox/worker вместо goroutine?
- Context.Value здесь metadata или скрытый бизнес-параметр?

## Готовые формулировки для собеседования

Про `Background`:

> Метод получает `ctx`, но внутри заменяет его на `context.Background()`. Так мы теряем cancellation, deadline и tracing. Если это синхронная часть use case-а, надо передавать входной ctx. Если операция должна жить дольше request-а, нужен явный worker/outbox, а не случайный Background.

Про goroutine:

> Здесь request context передается в goroutine. После возврата handler-а context может отмениться, и фоновая работа оборвется. Для async лучше поставить задачу в очередь/outbox, а worker должен работать на своем lifecycle context.

Про timeout:

> У внешнего вызова нет deadline-а. Один зависший provider может держать goroutine и ресурсы бесконечно. Я бы добавил context timeout на этот dependency и прокинул ctx в client.

Про cancel:

> После `context.WithTimeout` нужно вызвать cancel, обычно через defer, чтобы освободить timer resources и корректно завершить дочерний context.

Про `ctx.Err()`:

> При отмене лучше возвращать `ctx.Err()`, а не nil или generic error. Тогда caller сможет отличить client cancellation от deadline exceeded и правильно замаппить ошибку.

Про request scope:

> Request context подходит для работы, нужной ответу клиенту. Но если бизнес-процесс должен завершиться независимо от клиента, это должен быть отдельный durable workflow.

Про values:

> `Context.Value` не должен быть скрытым API для бизнес-параметров. Если userID/orderID/limit нужны методу, лучше передать их явно в command struct.

Про loops:

> В долгом цикле надо периодически проверять `ctx.Err()`, иначе cancellation будет срабатывать только после завершения всей обработки.

## Короткое резюме

Сильный ответ по теме context:

> Я смотрю не только на наличие параметра `ctx`, а на его scope и propagation. Синхронная работа должна уважать request context, внешние вызовы должны иметь deadline, cancellation должна возвращать `ctx.Err()`, а фоновая работа не должна жить на request context или случайном `Background`. Если операция должна пережить отмену запроса, это отдельный async workflow с worker context, outbox, timeout и идемпотентностью.

Главные идеи:

- context описывает жизненный цикл операции;
- request context не равен worker context;
- `Background()` внутри use case-а часто ломает cancellation/tracing;
- request ctx нельзя бездумно тащить в goroutine;
- timeout/deadline нужны внешним вызовам;
- cancel надо вызывать;
- `ctx.Err()` надо возвращать;
- context values не для бизнес-параметров;
- cancellation не убивает goroutine магически;
- async business work требует явного lifecycle и durable state.
