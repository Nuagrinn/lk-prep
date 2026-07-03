# Каналы, worker lifecycle, shutdown

Эта тема про каналы, фоновые worker-ы, очереди задач внутри процесса и корректное завершение. Она стоит рядом с многопоточкой, mutex и context, но у нее свой набор вопросов:

- кто отправляет в канал;
- кто читает из канала;
- кто имеет право закрывать канал;
- может ли send заблокировать request;
- что происходит при shutdown;
- ждём ли мы завершения worker-а;
- теряем ли задачи;
- есть ли backpressure;
- куда деваются ошибки;
- можно ли вызвать `Close` два раза;
- не остаются ли горутины жить после теста или остановки сервиса.

Главная мысль:

> Канал - это не просто “очередь”. Канал задает протокол взаимодействия между горутинами. У этого протокола должен быть владелец, правила закрытия, политика переполнения, обработка ошибок и lifecycle.

И вторая мысль:

> Worker без понятного Start/Stop/Wait контракта почти всегда становится источником зависаний, утечек горутин, потерянных задач или паник на закрытом канале.

## Простое введение

Представим сервис:

```go
type Service struct {
	jobs chan string
	stop chan struct{}
}

func NewService() *Service {
	s := &Service{
		jobs: make(chan string),
		stop: make(chan struct{}),
	}
	go s.worker()
	return s
}

func (s *Service) CreateOrder(ctx context.Context, id string) error {
	s.jobs <- id
	return nil
}

func (s *Service) worker() {
	for {
		select {
		case id := <-s.jobs:
			s.publish(id)
		case <-s.stop:
			return
		}
	}
}

func (s *Service) Close() {
	close(s.stop)
}
```

На первый взгляд выглядит нормально: есть канал задач, есть worker, есть stop channel. Но на ревью тут много вопросов.

Первый вопрос: `jobs` unbuffered. Значит `s.jobs <- id` заблокируется, пока worker не прочитает из канала. Если worker занят, завис, еще не стартовал или уже остановился, `CreateOrder` может зависнуть.

Второй вопрос: send не учитывает `ctx.Done()`. Если HTTP-запрос отменился или deadline истек, метод все равно может ждать отправки в канал.

Третий вопрос: `Close()` закрывает `stop`, но не ждет завершения worker-а. Метод вернулся, а worker еще может выполнять `publish`.

Четвертый вопрос: `Close()` нельзя вызвать два раза. Повторный `close(s.stop)` приведет к panic.

Пятый вопрос: что происходит с задачами, которые уже отправлены в `jobs`, но еще не обработаны? При `stop` worker может выйти и бросить их.

Шестой вопрос: если `publish` вернул ошибку, куда она девается? Логируется? Ретраится? Метрика? Dead letter?

Вот поэтому тема не сводится к синтаксису каналов. Надо ревьюить весь протокол.

Хороший комментарий на ревью:

> Здесь канал используется как внутренняя очередь, но у очереди не определен lifecycle: отправка может заблокировать request, shutdown не ждёт worker-а, повторный Close паникует, а политика обработки уже поставленных задач неясна.

## Что такое канал в Go

Канал в Go - это средство синхронизации и передачи значений между горутинами.

```go
ch := make(chan Job)
ch <- job
job := <-ch
```

Канал одновременно:

- передает значение;
- синхронизирует отправителя и получателя;
- может использоваться как очередь;
- может использоваться как сигнал;
- может ограничивать concurrency;
- может передавать ownership объекта от одной горутины к другой.

Каналы бывают unbuffered и buffered.

Unbuffered:

```go
ch := make(chan Job)
```

Send блокируется, пока кто-то не начнет receive. Receive блокируется, пока кто-то не сделает send. Это rendezvous: отправитель и получатель встречаются.

Buffered:

```go
ch := make(chan Job, 100)
```

Send блокируется только когда буфер заполнен. Receive блокируется, когда буфер пуст.

Буфер не делает систему “асинхронной без ограничений”. Он просто дает ограниченную емкость. Когда буфер заполнится, backpressure вернется к отправителю.

## Базовые свойства каналов

### Send в открытый канал

```go
ch <- value
```

Если канал unbuffered, send ждет receiver.

Если канал buffered, send ждет свободного места.

Если канал закрыт, send вызывает panic:

```text
panic: send on closed channel
```

### Receive из открытого канала

```go
value := <-ch
```

Если значения нет, receive блокируется.

Для проверки закрытия:

```go
value, ok := <-ch
if !ok {
	// channel closed and drained
}
```

### Receive из закрытого канала

Receive из закрытого и уже drained канала возвращает zero value сразу:

```go
value := <-ch // zero value
```

Поэтому если zero value валиден, всегда нужен `ok`:

```go
value, ok := <-ch
if !ok {
	return
}
```

### Close

```go
close(ch)
```

Close сообщает receiver-ам: новых значений больше не будет.

Правила:

- закрывать канал должен отправитель или владелец отправки;
- receiver обычно не закрывает канал;
- закрывать канал дважды нельзя, будет panic;
- отправка в закрытый канал приводит к panic;
- закрытие nil channel приводит к panic;
- receive из nil channel блокируется навсегда;
- send в nil channel блокируется навсегда.

Очень важная формулировка:

> Channel close is a broadcast to receivers, not a cancellation magic for senders.

Если отправитель делает `ch <- job`, а кто-то закрыл канал, отправитель получит panic. Поэтому надо четко понимать, кто владеет закрытием.

## Канал как очередь задач

Типичный паттерн:

```go
type Worker struct {
	jobs chan Job
}

func (w *Worker) Submit(ctx context.Context, job Job) error {
	select {
	case w.jobs <- job:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *Worker) run(ctx context.Context) {
	for {
		select {
		case job := <-w.jobs:
			w.handle(ctx, job)
		case <-ctx.Done():
			return
		}
	}
}
```

Что уже лучше:

- отправка в канал не висит бесконечно;
- caller получает ошибку при отмене context;
- worker может остановиться по context.

Но это еще не полный production lifecycle. Надо решить:

- кто создает context worker-а;
- кто вызывает cancel;
- что делать с jobs при shutdown;
- будет ли drain;
- как ждать завершения;
- как закрывать `jobs`;
- как не отправить после close;
- что делать с ошибками `handle`.

## Worker lifecycle

Lifecycle worker-а обычно включает состояния:

```text
created -> started -> running -> stopping -> stopped
```

Минимальные вопросы:

- когда worker стартует;
- можно ли стартовать дважды;
- как отправлять задачи до start;
- как остановить;
- можно ли остановить дважды;
- `Close` блокирующий или нет;
- `Close` дожидается завершения текущей задачи;
- что происходит с очередью;
- можно ли отправлять после `Close`;
- как caller узнает, что сервис закрыт.

В простом сервисе worker можно стартовать в constructor-е:

```go
func NewService(...) *Service {
	s := &Service{...}
	go s.worker()
	return s
}
```

Это удобно, но есть минусы:

- constructor сразу запускает goroutine;
- в тестах надо не забыть `Close`;
- если constructor вернул ошибку после старта части worker-ов, нужно cleanup;
- невозможно настроить сервис после создания, но до start;
- lifecycle привязан к созданию объекта.

Более явный вариант:

```go
func NewService(...) *Service
func (s *Service) Start(ctx context.Context) error
func (s *Service) Close(ctx context.Context) error
```

Для маленьких задач constructor-start нормально, но на ревью стоит смотреть, есть ли `Close` и `Wait`.

## Stop channel

Классический stop channel:

```go
type Service struct {
	stop chan struct{}
}

func (s *Service) worker() {
	for {
		select {
		case <-s.stop:
			return
		}
	}
}

func (s *Service) Close() {
	close(s.stop)
}
```

Плюсы:

- close канала будит всех receiver-ов;
- можно остановить несколько worker-ов одним сигналом;
- не нужно отправлять N сообщений для N worker-ов.

Минусы:

- повторный close вызывает panic;
- send-сторона задач может не знать, что сервис закрыт;
- сам по себе stop не ждет завершения;
- не задает timeout shutdown;
- надо решить, drain или immediate stop.

Повторный close чинится `sync.Once`:

```go
type Service struct {
	stop     chan struct{}
	stopOnce sync.Once
}

func (s *Service) Close() {
	s.stopOnce.Do(func() {
		close(s.stop)
	})
}
```

Но `sync.Once` решает только panic на повторном close. Он не решает ожидание worker-а и обработку очереди.

## Context как сигнал остановки

Вместо stop channel можно использовать context:

```go
type Service struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func NewService(parent context.Context) *Service {
	ctx, cancel := context.WithCancel(parent)
	return &Service{ctx: ctx, cancel: cancel}
}

func (s *Service) Close() {
	s.cancel()
}
```

В worker-е:

```go
func (s *Service) worker() {
	for {
		select {
		case job := <-s.jobs:
			s.handle(s.ctx, job)
		case <-s.ctx.Done():
			return
		}
	}
}
```

Плюсы context:

- можно передавать cancellation во внешние операции;
- можно задавать timeout/deadline;
- хорошо интегрируется с HTTP/gRPC/DB;
- `cancel()` можно вызвать несколько раз безопасно.

Минусы:

- context сам не закрывает `jobs`;
- context сам не ждет goroutine;
- надо не перепутать request context и worker lifecycle context;
- `ctx.Done()` не говорит, обработать ли оставшиеся jobs.

Важно:

> Request context обычно не должен управлять жизнью background worker-а сервиса.

Плохо:

```go
func (s *Service) CreateOrder(ctx context.Context, order Order) error {
	go s.publishLater(ctx, order)
	return nil
}
```

После ответа HTTP request context отменится, и background work может оборваться.

Лучше:

- положить задачу в durable queue/outbox;
- или отправить в worker, который живет на service context;
- а `ctx` request-а использовать только для submit с timeout.

## WaitGroup

Чтобы `Close` дождался завершения worker-ов:

```go
type Service struct {
	stop chan struct{}
	wg   sync.WaitGroup
}

func (s *Service) Start() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.worker()
	}()
}

func (s *Service) Close() {
	close(s.stop)
	s.wg.Wait()
}
```

Это уже лучше:

- `Close` не возвращается, пока worker не вышел;
- тесты могут надежно завершаться;
- ресурсы освобождаются предсказуемо.

Но надо быть аккуратным:

- `wg.Add` должен происходить до запуска goroutine;
- нельзя делать `Add` конкурентно с `Wait` без строгого lifecycle;
- `Close` может зависнуть, если worker не реагирует на stop;
- нужен `sync.Once` для повторного Close.

Более полный вариант:

```go
type Service struct {
	jobs      chan Job
	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}
```

`done` можно использовать, чтобы `Submit` понимал, что сервис закрывается:

```go
func (s *Service) Submit(ctx context.Context, job Job) error {
	select {
	case s.jobs <- job:
		return nil
	case <-s.done:
		return ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}
```

Но тут нужно правильно закрывать `done` в `Close`.

## Закрывать jobs или stop?

Есть два распространенных подхода.

### Подход 1. Закрыть stop, jobs не закрывать

```go
select {
case job := <-s.jobs:
	handle(job)
case <-s.stop:
	return
}
```

Плюсы:

- `stop` как broadcast;
- не рискуем send on closed jobs, если submit еще возможен;
- подходит для immediate shutdown.

Минусы:

- уже поставленные jobs могут остаться необработанными;
- Submit после Close может зависнуть или отправить задачу, которую никто не прочитает, если не защищен;
- нужна отдельная защита от отправки после close.

### Подход 2. Закрыть jobs, worker drain-ит канал

```go
func (s *Service) worker() {
	for job := range s.jobs {
		handle(job)
	}
}

func (s *Service) Close() {
	close(s.jobs)
	s.wg.Wait()
}
```

Плюсы:

- worker обработает все jobs, которые уже были в канале;
- простой worker loop;
- `Close` дожидается drain.

Минусы:

- если кто-то отправит после close, будет panic;
- надо гарантировать, что после начала close новых send не будет;
- нужен mutex/atomic state или submit channel owner;
- если queue большая и handle медленный, shutdown может быть долгим.

Правило:

> Закрывать `jobs` безопасно только тому, кто гарантирует, что больше никто не будет отправлять в `jobs`.

В сервисном API это обычно значит: `Submit` должен проверять closed state под lock или через отдельный done channel.

## Graceful shutdown и immediate shutdown

Перед реализацией shutdown надо выбрать политику.

### Immediate shutdown

Сервис прекращает брать новые задачи и пытается остановиться быстро. Текущая задача может прерваться по context.

Подходит для:

- best-effort событий;
- периодических refresh-задач;
- задач, которые можно безопасно повторить позже;
- worker-ов, читающих durable queue, где сообщение вернется в очередь.

### Graceful drain

Сервис перестает принимать новые задачи, но обрабатывает уже принятые.

Подходит для:

- in-memory очереди, где задачи иначе потеряются;
- коротких задач;
- shutdown в тестах;
- controlled deployments с timeout.

### Graceful with timeout

Сервис пытается drain-ить, но не бесконечно:

```go
func (s *Service) Close(ctx context.Context) error {
	s.closeOnce.Do(func() {
		close(s.jobs)
	})

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
```

Тут есть нюанс: запускать goroutine в каждом `Close` не всегда красиво, и надо аккуратно проектировать повторные вызовы. Но идея важна: `Close` не должен вечно висеть без возможности timeout.

На ревью:

> Надо явно выбрать shutdown policy: бросаем очередь, drain-им ее или drain-им с timeout. Сейчас из кода непонятно, какие гарантии у уже принятых задач.

## Backpressure

Backpressure - это когда система не бесконечно принимает работу, а заставляет отправителя ждать или получать ошибку, если downstream не успевает.

Unbuffered channel дает сильный backpressure:

```go
jobs := make(chan Job)
```

Каждый submit ждет worker-а.

Buffered channel дает ограниченный буфер:

```go
jobs := make(chan Job, 100)
```

Первые 100 задач влезут быстро, потом submit начнет ждать.

Без backpressure часто делают плохо:

```go
go handle(job)
```

на каждый request. Под нагрузкой горутин становится тысячи, память растет, внешние сервисы перегружаются.

На ревью:

> Буфер канала - это часть capacity planning. Нужно понимать, почему выбран такой размер, что происходит при заполнении и какую ошибку видит caller.

Пример submit с backpressure и context:

```go
func (s *Service) Submit(ctx context.Context, job Job) error {
	select {
	case s.jobs <- job:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return ErrClosed
	}
}
```

Если не хотим ждать, можно сделать non-blocking submit:

```go
func (s *Service) TrySubmit(job Job) error {
	select {
	case s.jobs <- job:
		return nil
	default:
		return ErrQueueFull
	}
}
```

Но `default` означает: если очередь прямо сейчас заполнена, задача отклоняется. Это нормальная политика, если caller умеет обработать `ErrQueueFull`.

## Worker pool

Один worker:

```go
go s.worker()
```

Несколько worker-ов:

```go
for i := 0; i < n; i++ {
	s.wg.Add(1)
	go func(workerID int) {
		defer s.wg.Done()
		s.worker(workerID)
	}(i)
}
```

Worker pool нужен, если:

- задач много;
- каждая задача делает I/O;
- можно обрабатывать параллельно;
- надо ограничить concurrency.

Но worker pool требует ответов:

- сохраняется ли порядок задач;
- можно ли одну и ту же сущность обрабатывать параллельно;
- нужны ли per-key locks;
- как обрабатываются ошибки;
- как shutdown влияет на всех worker-ов;
- что делать с panic внутри worker-а;
- как метриками видеть backlog и processing time.

Если важен порядок, несколько worker-ов могут нарушить его.

Пример:

```text
job1: set status paid
job2: set status canceled
```

Если job2 выполнится раньше job1, состояние может сломаться.

Для таких сценариев нужна:

- одна очередь на aggregate;
- partitioning by key;
- per-key serialization;
- optimistic version check;
- идемпотентные state transitions.

## Ошибки worker-а

Очень частая проблема:

```go
_ = s.events.Publish(ctx, topic, payload)
```

Ошибка пропадает. Worker вроде “обработал” задачу, но результат неизвестен.

Надо решить:

- логируем и забываем;
- ретраим;
- возвращаем задачу в очередь;
- пишем dead letter;
- обновляем статус в БД;
- метрика failure;
- останавливаем сервис;
- возвращаем ошибку caller-у.

Для in-memory worker-а без durable storage ретраи опасны:

```go
for {
	err := handle(job)
	if err == nil {
		return
	}
}
```

Так можно зависнуть на одной задаче навсегда.

Лучше:

- ограниченный retry count;
- backoff;
- context timeout;
- логирование;
- failure status;
- durable retry queue/outbox для критичных задач.

На ревью:

> Worker игнорирует ошибку обработки. Нужно понять, задача best-effort или критичная. Если критичная, нужен retry/outbox/status, если best-effort - хотя бы лог и метрика.

## Panic внутри worker-а

Если worker panic-нул, горутина умерла.

Пример:

```go
go func() {
	for job := range jobs {
		handle(job)
	}
}()
```

Если `handle` panic-нул:

- worker завершился;
- новые jobs могут навсегда зависнуть в очереди;
- `Submit` может начать блокироваться;
- сервис может выглядеть живым, но фоновые задачи не работают.

Можно использовать recover на границе worker-а:

```go
func (s *Service) worker() {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("worker panic", "panic", r)
		}
	}()

	for job := range s.jobs {
		s.handle(job)
	}
}
```

Но просто recover и выход тоже может оставить сервис без worker-а. Варианты:

- recover вокруг каждой job, чтобы worker продолжал;
- supervisor перезапускает worker;
- panic считается fatal, процесс падает;
- job уходит в failed.

Для большинства service-layer задач лучше не допускать panic и обрабатывать ошибки явно. Но на ревью можно отметить:

> Если обработчик job может panic-нуть, один worker умрет и очередь перестанет разгребаться. Нужна стратегия: recover per job, supervisor или fail-fast.

## Канал как сигнал

`chan struct{}` часто используется как сигнал:

```go
done := make(chan struct{})

go func() {
	defer close(done)
	doWork()
}()

<-done
```

`struct{}` не занимает памяти для значения и говорит: нам важен сам факт сигнала.

Stop channel:

```go
stop := make(chan struct{})
close(stop)
```

Все receive из `stop` разблокируются.

Важно:

- не отправлять много значений в stop channel;
- для broadcast использовать close;
- закрывать один раз;
- не закрывать канал из receiver-а.

## select

`select` выбирает один готовый case.

```go
select {
case job := <-jobs:
	handle(job)
case <-ctx.Done():
	return ctx.Err()
}
```

Если готово несколько case, Go выбирает псевдослучайно.

Это важно:

```go
select {
case job := <-jobs:
	handle(job)
case <-stop:
	return
}
```

Если `stop` закрыт и одновременно в `jobs` есть задача, select может выбрать любую ветку. То есть shutdown может не быть немедленным, а может обработать еще одну задачу. Или наоборот выйти, оставив задачи.

Если нужна строгая политика drain, надо писать явно:

```go
for {
	select {
	case job, ok := <-jobs:
		if !ok {
			return
		}
		handle(job)
	case <-stop:
		for {
			select {
			case job := <-jobs:
				handle(job)
			default:
				return
			}
		}
	}
}
```

Но такой код уже сложнее. Часто проще закрывать `jobs` и `range`.

## Nil channels в select

Nil channel блокируется навсегда.

В `select` это можно использовать, чтобы отключить case:

```go
var jobs <-chan Job
if enabled {
	jobs = s.jobs
}

select {
case job := <-jobs:
	handle(job)
case <-ctx.Done():
	return
}
```

Если `enabled == false`, case с jobs никогда не выберется.

Это продвинутый прием. На ревью надо быть осторожным: nil channel может быть и случайным багом.

Плохо:

```go
var jobs chan Job
jobs <- job // blocks forever
```

## Закрытый jobs канал и ok

Плохо:

```go
for {
	select {
	case job := <-s.jobs:
		s.handle(job)
	case <-s.stop:
		return
	}
}
```

Если `s.jobs` закрыть, receive будет постоянно возвращать zero value. Worker начнет бесконечно обрабатывать пустые job.

Лучше:

```go
case job, ok := <-s.jobs:
	if !ok {
		return
	}
	s.handle(job)
```

Или:

```go
for job := range s.jobs {
	s.handle(job)
}
```

## Отправка в канал с context

Плохо:

```go
s.jobs <- job
```

Если канал заполнен, метод зависнет.

Лучше:

```go
select {
case s.jobs <- job:
	return nil
case <-ctx.Done():
	return ctx.Err()
}
```

Если сервис может закрываться:

```go
select {
case s.jobs <- job:
	return nil
case <-s.done:
	return ErrClosed
case <-ctx.Done():
	return ctx.Err()
}
```

Но надо думать о гонке между `done` и `jobs`. Если `jobs` закрывается, send может panic. Поэтому один из вариантов - не закрывать `jobs`, а закрывать `done` и давать worker-у выйти по `done`. Другой вариант - защищать submit/close mutex-ом.

## Submit after Close

Типичная ошибка:

```go
func (s *Service) Close() {
	close(s.jobs)
}

func (s *Service) Submit(job Job) {
	s.jobs <- job
}
```

Если `Submit` вызвали после `Close`, будет panic.

Нужно определить контракт:

- после `Close` `Submit` возвращает `ErrClosed`;
- или `Submit` запрещен и caller обязан синхронизироваться;
- или сервис не закрывает `jobs`, а использует `done`;
- или `Submit` и `Close` защищены mutex-ом.

Пример с mutex:

```go
type Service struct {
	mu     sync.Mutex
	closed bool
	jobs   chan Job
}

func (s *Service) Submit(ctx context.Context, job Job) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrClosed
	}
	jobs := s.jobs
	s.mu.Unlock()

	select {
	case jobs <- job:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) Close() {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		close(s.jobs)
	}
	s.mu.Unlock()

	s.wg.Wait()
}
```

Но тут есть тонкая гонка: `Submit` может взять `jobs`, отпустить mutex, затем `Close` закроет канал, а `Submit` попытается отправить и panic-нет.

Чтобы закрывать `jobs` безопасно, нужно либо держать lock на время send, что может заблокировать `Close`, либо использовать другой протокол.

Поэтому часто выбирают:

- `jobs` не закрывается;
- закрывается `done`;
- `Submit` select-ит по `done`;
- worker выходит по `done`;
- accepted jobs могут быть потеряны при immediate shutdown.

Или:

- `Close` сначала запрещает новые submit-ы;
- ждет активные submit-ы;
- потом закрывает jobs;
- worker drain-ит jobs.

Это сложнее, но дает graceful drain.

## Протокол graceful close с закрытием jobs

Один из вариантов:

```go
type Service struct {
	mu       sync.Mutex
	closed   bool
	submits  sync.WaitGroup
	jobs     chan Job
	wg       sync.WaitGroup
	closeOnce sync.Once
}
```

Идея:

1. `Submit` под lock проверяет `closed`.
2. Если не closed, увеличивает `submits`.
3. Отпускает lock и делает send.
4. После send делает `submits.Done`.
5. `Close` ставит `closed = true`.
6. Ждет `submits.Wait`.
7. Закрывает `jobs`.
8. Ждет worker `wg.Wait`.

Это уже довольно сложный протокол. Для собеса на middle+ чаще достаточно не писать его полностью, а проговорить проблему:

> Если мы закрываем jobs, надо гарантировать, что ни один Submit не отправит после close. Это требует явного протокола закрытия. Иначе будет panic.

## In-memory queue vs durable queue

Канал внутри процесса - это in-memory очередь.

Свойства:

- задачи теряются при crash процесса;
- задачи теряются при restart/deploy, если не drain-ить;
- очередь живет только внутри одного instance;
- при нескольких replicas у каждой свой канал;
- нет built-in retry после падения;
- нет visibility timeout;
- нет dead letter;
- нет persistence.

Для best-effort фоновой работы канал нормален.

Для критичных бизнес-событий лучше:

- transactional outbox;
- Kafka/RabbitMQ/SQS/NATS/Redis stream;
- durable job table;
- external scheduler.

На ревью:

> Если задача критична, in-memory channel недостаточен: при падении процесса она потеряется. Лучше outbox/durable queue. Канал подходит, если потеря допустима или задачу можно восстановить.

## Горутина на каждую задачу vs worker pool

Плохо:

```go
for _, item := range items {
	go process(item)
}
```

Если items много:

- всплеск горутин;
- нет backpressure;
- нет лимита внешних запросов;
- ошибки теряются;
- shutdown сложный;
- тесты flaky.

Лучше:

```go
jobs := make(chan Item)

for i := 0; i < workers; i++ {
	go func() {
		for item := range jobs {
			process(item)
		}
	}()
}

for _, item := range items {
	jobs <- item
}
close(jobs)
```

Но это тоже надо доработать:

- context;
- error aggregation;
- WaitGroup;
- закрытие results;
- cancellation on first error, если нужно.

## Fan-out / fan-in

Fan-out: распределяем задачи между несколькими worker-ами.

Fan-in: собираем результаты из нескольких worker-ов в один канал.

Пример:

```go
results := make(chan Result)

for i := 0; i < n; i++ {
	wg.Add(1)
	go func() {
		defer wg.Done()
		for job := range jobs {
			results <- handle(job)
		}
	}()
}

go func() {
	wg.Wait()
	close(results)
}()
```

Важные правила:

- results закрывает тот, кто знает, что все senders завершились;
- обычно это goroutine, которая ждет `wg.Wait`;
- receiver не закрывает results;
- если receiver перестал читать results, workers могут зависнуть на send;
- нужен context cancellation.

Плохой вариант:

```go
for result := range results {
	if result.Err != nil {
		return result.Err
	}
}
```

Если вернуться раньше, worker-ы могут заблокироваться на `results <- ...`.

Нужно cancel:

```go
ctx, cancel := context.WithCancel(ctx)
defer cancel()
```

и send:

```go
select {
case results <- result:
case <-ctx.Done():
	return
}
```

## Каналы и ошибки

Иногда делают отдельный канал ошибок:

```go
errCh := make(chan error, 1)
```

Важно:

- если errCh unbuffered, worker может зависнуть при отправке ошибки;
- если ошибок много, буфер 1 сохранит только первую при правильном select;
- errCh надо закрывать только когда все senders завершились;
- receiver должен отменять context, если дальше результаты не нужны.

Пример:

```go
select {
case errCh <- err:
default:
}
```

Так сохраняем первую ошибку, не блокируя worker.

Но в реальном коде часто удобнее `errgroup.Group` из `golang.org/x/sync/errgroup`, если dependency допустима:

```go
g, ctx := errgroup.WithContext(ctx)

for _, item := range items {
	item := item
	g.Go(func() error {
		return process(ctx, item)
	})
}

if err := g.Wait(); err != nil {
	return err
}
```

Для worker pool с лимитом у errgroup есть `SetLimit` в современных версиях:

```go
g.SetLimit(10)
```

На собесе можно сказать:

> Для fan-out с ошибками я бы не писал вручную сложную связку каналов, если можно использовать errgroup с context и лимитом concurrency.

## Context в worker-е

Нужны два разных context-а:

1. Context submit-а: сколько caller готов ждать постановку задачи.
2. Context worker-а: жизненный цикл фоновой обработки.

Пример:

```go
func (s *Service) Submit(ctx context.Context, job Job) error {
	select {
	case s.jobs <- job:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return ErrClosed
	}
}
```

А worker:

```go
func (s *Service) worker(ctx context.Context) {
	for {
		select {
		case job := <-s.jobs:
			s.handle(ctx, job)
		case <-ctx.Done():
			return
		}
	}
}
```

Если обработка одной job должна иметь timeout:

```go
func (s *Service) handle(parent context.Context, job Job) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()

	_ = s.publisher.Publish(ctx, job.Event)
}
```

Не стоит использовать `context.Background()` внутри worker-а без причины. Если service shutdown начался, внешние операции тоже должны получить cancellation или timeout.

## Channel direction types

В Go можно ограничить направление канала:

```go
func producer(out chan<- Job)
func consumer(in <-chan Job)
```

Это улучшает API:

- producer не может читать;
- consumer не может писать;
- легче понять ownership.

Пример:

```go
func startWorkers(ctx context.Context, jobs <-chan Job, n int) {
	for i := 0; i < n; i++ {
		go func() {
			for {
				select {
				case job, ok := <-jobs:
					if !ok {
						return
					}
					handle(job)
				case <-ctx.Done():
					return
				}
			}
		}()
	}
}
```

На ревью:

> Здесь функция только читает из канала, можно принять `<-chan Job`, чтобы контракт был явным.

## Semaphore через канал

Канал можно использовать как семафор:

```go
sem := make(chan struct{}, 10)

for _, item := range items {
	sem <- struct{}{}
	go func(item Item) {
		defer func() { <-sem }()
		process(item)
	}(item)
}
```

Так ограничиваем число параллельных горутин.

Но нужно:

- ждать завершения всех goroutine через WaitGroup;
- учитывать context;
- не забыть release в defer;
- не зависнуть при отправке в sem, если context отменился.

Более аккуратно:

```go
select {
case sem <- struct{}{}:
case <-ctx.Done():
	return ctx.Err()
}
```

## Тикеры и таймеры в worker-ах

Worker часто делает periodic task:

```go
ticker := time.NewTicker(time.Minute)
defer ticker.Stop()

for {
	select {
	case <-ticker.C:
		s.refresh()
	case <-ctx.Done():
		return
	}
}
```

Ошибки:

- забыли `ticker.Stop()`;
- `refresh` выполняется дольше интервала, задачи накладываются;
- нет context внутри refresh;
- panic убивает worker;
- shutdown ждет долгий refresh без timeout.

Если нельзя накладывать refresh, один worker loop нормален. Если можно пропускать tick-и при занятости, надо явно выбрать политику.

## Типовые ошибки

### Ошибка 1. Unbuffered channel блокирует request

```go
func (s *Service) Create(ctx context.Context, id string) error {
	s.jobs <- id
	return nil
}
```

Если worker занят или остановлен, request зависнет.

Как лучше:

```go
select {
case s.jobs <- id:
	return nil
case <-ctx.Done():
	return ctx.Err()
}
```

И решить, нужен ли buffer.

### Ошибка 2. Send не учитывает shutdown

```go
select {
case s.jobs <- job:
	return nil
case <-ctx.Done():
	return ctx.Err()
}
```

Если service закрывается, submit может ждать до request deadline.

Лучше добавить done:

```go
case <-s.done:
	return ErrClosed
```

### Ошибка 3. Close не idempotent

```go
func (s *Service) Close() {
	close(s.stop)
}
```

Повторный вызов panic.

Лучше:

```go
s.closeOnce.Do(func() { close(s.stop) })
```

Но помнить, что это только часть решения.

### Ошибка 4. Close не ждет worker-а

```go
func (s *Service) Close() {
	close(s.stop)
}
```

Метод вернулся, worker еще может работать.

Нужно:

```go
close(s.stop)
s.wg.Wait()
```

или `Close(ctx)` с timeout.

### Ошибка 5. Закрыли jobs, но Submit может отправить

```go
close(s.jobs)
```

и где-то:

```go
s.jobs <- job
```

Паника `send on closed channel`.

Нужен ownership и протокол закрытия.

### Ошибка 6. Receive из закрытого jobs без ok

```go
job := <-s.jobs
s.handle(job)
```

После close будет zero value.

Нужно:

```go
job, ok := <-s.jobs
if !ok {
	return
}
```

### Ошибка 7. Worker использует context.Background()

```go
_ = s.publisher.Publish(context.Background(), event)
```

Shutdown/deadline не управляет publish. Worker может зависнуть надолго.

Лучше worker context или per-job timeout.

### Ошибка 8. Ошибки обработки игнорируются

```go
_ = s.events.Publish(ctx, topic, payload)
```

Непонятно, потеряна задача или нет.

Нужно определить failure policy.

### Ошибка 9. Нет backpressure

```go
go s.handle(job)
```

на каждый request.

Под нагрузкой неограниченные goroutine.

Лучше bounded queue / worker pool / semaphore.

### Ошибка 10. Горутина может зависнуть на отправке результата

```go
results <- result
```

Если receiver ушел по ошибке, worker завис.

Лучше:

```go
select {
case results <- result:
case <-ctx.Done():
	return
}
```

### Ошибка 11. results закрывает receiver

Receiver не знает, все ли senders закончили.

Закрывать results должен координатор, который дождался всех senders.

### Ошибка 12. `for range ch` без закрытия ch

```go
for job := range jobs {
	handle(job)
}
```

Если jobs никогда не закрывается, worker никогда не выйдет. Это нормально для процесса, который живет до shutdown, но тогда нужен другой сигнал остановки.

### Ошибка 13. `select` с `default` крутит CPU

```go
for {
	select {
	case job := <-jobs:
		handle(job)
	default:
	}
}
```

Busy loop, CPU 100%.

Если нечего делать, лучше блокироваться на channel или ticker.

### Ошибка 14. `time.After` в бесконечном цикле

```go
for {
	select {
	case <-time.After(time.Second):
		do()
	}
}
```

Часто лучше `time.NewTicker`, особенно если цикл долгоживущий. `time.After` создает новый timer каждый раз.

### Ошибка 15. Worker pool нарушает порядок

Если несколько worker-ов читают один канал, порядок завершения задач не гарантирован.

Если бизнесу важен порядок, нужен другой дизайн.

### Ошибка 16. In-memory channel используется для критичной задачи

Падение процесса потеряет задачу.

Для критичных событий лучше durable queue/outbox.

### Ошибка 17. Add внутри goroutine

Плохо:

```go
go func() {
	wg.Add(1)
	defer wg.Done()
	work()
}()
wg.Wait()
```

`Wait` может выполниться до `Add`.

Правильно:

```go
wg.Add(1)
go func() {
	defer wg.Done()
	work()
}()
```

### Ошибка 18. Захват loop variable

В современных Go часть кейсов улучшена, но на ревью все равно стоит быть внимательным, особенно если проект на старой версии или переменная переиспользуется:

```go
for _, job := range jobs {
	go func() {
		handle(job)
	}()
}
```

Безопаснее явно:

```go
for _, job := range jobs {
	job := job
	go func() {
		handle(job)
	}()
}
```

### Ошибка 19. Канал используется там, где проще mutex

Не надо использовать channel только потому, что “Go любит каналы”.

Если нужно защитить map, mutex проще.

Если нужно передать ownership задачи worker-у, channel подходит.

Формулировка:

> Share memory by communicating, when communication is the model. But if the model is shared cache, mutex is often simpler and clearer.

## Как проектировать worker правильно

### Шаг 1. Определить назначение задач

Вопросы:

- задача критичная или best-effort;
- можно ли потерять задачу при crash;
- можно ли обработать повторно;
- нужна ли идемпотентность;
- важен ли порядок;
- можно ли параллелить;
- сколько задач в секунду ожидается;
- что делать при ошибке.

### Шаг 2. Выбрать хранение очереди

In-memory channel подходит:

- для best-effort;
- для short-lived background work;
- для локального worker pool;
- для ограничения concurrency;
- для задач, которые можно восстановить другим путем.

Durable queue/outbox нужен:

- для бизнес-критичных событий;
- для платежей;
- для email, если delivery критична;
- для интеграций;
- если задача не должна теряться при deploy/crash.

### Шаг 3. Выбрать capacity и backpressure policy

Варианты:

- unbuffered: caller ждет worker-а;
- small buffer: сглаживает короткие пики;
- large buffer: риск памяти и долгого drain;
- non-blocking submit: возвращает `ErrQueueFull`;
- blocking with context: ждет до deadline.

Нужно определить:

- что видит caller при full queue;
- логируется ли overflow;
- есть ли метрика backlog;
- не скрываем ли перегрузку.

### Шаг 4. Определить shutdown policy

Варианты:

- immediate stop;
- drain accepted jobs;
- drain with timeout;
- stop accepting new jobs, current job finishes;
- cancel current job.

Нужно явно выбрать.

### Шаг 5. Определить error policy

Варианты:

- log and drop;
- retry N times;
- exponential backoff;
- mark failed in DB;
- dead letter;
- stop service;
- return error through result channel;
- outbox retry worker.

### Шаг 6. Добавить observability

Метрики:

- queue length;
- queue capacity;
- submit count;
- submit rejected count;
- worker active count;
- job processing duration;
- job success/failure count;
- retries;
- oldest queued job age;
- shutdown duration.

Логи:

- worker started/stopped;
- submit rejected;
- job failed;
- panic recovered;
- shutdown timeout;
- queue full.

## Хороший базовый шаблон

Это не универсальный идеал, но хороший ориентир для in-memory worker-а с immediate shutdown и безопасным `Close`.

```go
var ErrClosed = errors.New("service closed")

type Service struct {
	jobs chan Job

	ctx    context.Context
	cancel context.CancelFunc

	wg        sync.WaitGroup
	closeOnce sync.Once
	done      chan struct{}
}

func NewService(parent context.Context, workers int, queueSize int) *Service {
	ctx, cancel := context.WithCancel(parent)
	s := &Service{
		jobs:   make(chan Job, queueSize),
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
	}

	for i := 0; i < workers; i++ {
		s.wg.Add(1)
		go func(workerID int) {
			defer s.wg.Done()
			s.worker(workerID)
		}(i)
	}

	return s
}

func (s *Service) Submit(ctx context.Context, job Job) error {
	select {
	case s.jobs <- job:
		return nil
	case <-s.done:
		return ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) worker(workerID int) {
	for {
		select {
		case job := <-s.jobs:
			s.handle(s.ctx, job)
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *Service) Close() {
	s.closeOnce.Do(func() {
		close(s.done)
		s.cancel()
		s.wg.Wait()
	})
}
```

Что тут хорошо:

- `Close` idempotent;
- worker получает cancellation;
- `Submit` учитывает caller context и closed state;
- есть WaitGroup;
- очередь bounded.

Что тут не идеально:

- если `jobs` уже содержит задачи, при cancel worker может выйти и оставить их;
- select может выбрать job даже после ctx cancel, если оба case готовы;
- `Submit` может успеть отправить job одновременно с close, если select выберет send;
- задачи in-memory и потеряются при crash;
- ошибки handle надо отдельно проектировать.

То есть даже хороший шаблон требует осознанной политики. Это нормально. Главное - уметь проговорить trade-offs.

## Шаблон graceful drain через close jobs

Подходит, если есть один владелец submit-а или можно гарантировать, что после `Close` submit не вызывается.

```go
type Service struct {
	jobs      chan Job
	wg        sync.WaitGroup
	closeOnce sync.Once
}

func NewService(workers int, queueSize int) *Service {
	s := &Service{
		jobs: make(chan Job, queueSize),
	}

	for i := 0; i < workers; i++ {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			for job := range s.jobs {
				s.handle(job)
			}
		}()
	}

	return s
}

func (s *Service) Submit(ctx context.Context, job Job) error {
	select {
	case s.jobs <- job:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) Close() {
	s.closeOnce.Do(func() {
		close(s.jobs)
		s.wg.Wait()
	})
}
```

Главный caveat:

> Этот вариант безопасен только если после начала Close никто не вызывает Submit.

Если такого контракта нет, нужен более сложный протокол или другой подход.

## Диагностика проблем

### На code review

Ищи:

- `make(chan` без размера;
- `jobs <-` без `select`;
- `<-jobs` без `ok`, если канал может закрываться;
- `close(...)` без `sync.Once`;
- `go func` без WaitGroup;
- `go func` внутри request path;
- `_ = worker.Handle(...)`;
- `context.Background()` внутри worker-а;
- `for range ch`, но непонятно, кто закрывает ch;
- `select { default: }` в бесконечном цикле;
- `time.After` внутри долгого loop;
- channel close из receiver-а;
- send на channel, который может быть closed;
- отсутствие `Close`;
- отсутствие `wg.Wait`;
- отсутствие queue capacity/backpressure policy.

### В тестах

Полезные тесты:

- `Close` можно вызвать дважды;
- `Submit` после `Close` возвращает ошибку, а не panic;
- `Submit` с canceled context возвращает `context.Canceled`;
- заполненная очередь не вешает тест навсегда;
- worker завершается после Close;
- обработчик получает cancellation;
- ошибки worker-а логируются/метрятся;
- jobs drain-ятся или не drain-ятся согласно политике;
- нет утечки горутин после теста.

Для leak-тестов можно использовать внешние библиотеки вроде `goleak`, если проект это допускает. Без зависимостей можно грубо проверять `runtime.NumGoroutine`, но это менее надежно.

### В runtime

Инструменты:

- goroutine profile через pprof;
- block profile;
- mutex profile;
- execution trace;
- metrics queue length;
- logs worker start/stop;
- stack dump при зависании.

Признаки проблем:

- много горутин в состоянии `chan send`;
- много горутин в состоянии `chan receive`;
- goroutine висит на `<-time.After`;
- worker stack показывает зависший external call;
- queue length постоянно растет;
- shutdown duration растет;
- `Submit` latency растет.

Пример stack:

```text
goroutine 123 [chan send]:
Service.Submit(...)
```

Это сигнал: отправка в канал блокируется. Надо смотреть, есть ли worker, жив ли он, заполнен ли буфер.

## Методология ревью

### Шаг 1. Найти все каналы

Поиск:

```text
make(chan
chan
<- 
close(
```

Спроси:

- канал для данных или для сигнала;
- buffered или unbuffered;
- кто пишет;
- кто читает;
- кто закрывает;
- может ли быть несколько writers/readers.

### Шаг 2. Найти lifecycle

Ищи:

- `go s.worker`;
- `Start`;
- `Close`;
- `Stop`;
- `Wait`;
- `sync.Once`;
- `sync.WaitGroup`;
- `context.WithCancel`.

Спроси:

- когда worker стартует;
- можно ли стартовать дважды;
- как остановить;
- ждём ли завершения;
- что если Close вызвать дважды;
- что если Close вызвать во время Submit.

### Шаг 3. Проверить send path

Для каждого send:

- может ли он заблокироваться;
- есть ли `select` с `ctx.Done`;
- есть ли обработка service closed;
- что происходит при full buffer;
- может ли канал быть закрыт;
- не отправляем ли под mutex;
- не отправляем ли в nil channel.

### Шаг 4. Проверить receive path

Для каждого receive:

- проверяется ли `ok`;
- что если канал закрыт;
- что если stop пришел одновременно с job;
- не будет ли worker висеть forever;
- есть ли context cancellation;
- не игнорируются ли ошибки обработки.

### Шаг 5. Проверить shutdown policy

Спроси:

- immediate или drain;
- что с queued jobs;
- что с in-flight job;
- есть ли timeout;
- есть ли метрики shutdown;
- можно ли потерять критичную задачу.

### Шаг 6. Проверить concurrency limit

Спроси:

- сколько worker-ов;
- почему именно столько;
- что с внешним сервисом при параллельной обработке;
- нужен ли rate limit;
- важен ли порядок.

### Шаг 7. Проверить error/retry policy

Спроси:

- куда идет ошибка job;
- есть ли retry;
- retry идемпотентный;
- есть ли backoff;
- есть ли dead letter;
- ошибка влияет на caller-а или нет.

## Готовые формулировки для собеседования

Про unbuffered channel:

> Здесь отправка в unbuffered channel находится в request path. Если worker занят или остановлен, метод может зависнуть. Я бы сделал buffered queue и отправку через `select` с `ctx.Done()` и сигналом закрытия сервиса.

Про shutdown:

> `Close` только закрывает stop channel, но не ждет завершения worker-а. После Close фоновые операции еще могут выполняться. Нужен `WaitGroup`, а сам Close лучше сделать idempotent через `sync.Once`.

Про повторный Close:

> Повторный `close(stop)` приведет к panic. Если Close может вызываться из разных мест или в тестах через defer, нужен `sync.Once` или другой guard.

Про закрытие jobs:

> Если мы закрываем `jobs`, надо гарантировать, что больше никто не отправляет в этот канал. Иначе будет `send on closed channel`. Обычно закрывает канал владелец отправки, а не receiver.

Про ошибки worker-а:

> Worker игнорирует ошибку обработки. Нужно определить, является задача best-effort или критичной. Для критичных задач нужен retry/outbox/status, для best-effort хотя бы лог и метрика.

Про context:

> В worker-е используется `context.Background()`, поэтому shutdown не отменит внешний вызов. Лучше использовать context жизненного цикла worker-а и per-job timeout.

Про in-memory queue:

> Канал внутри процесса не является durable queue. При crash или restart задачи в нем потеряются. Для бизнес-критичных событий лучше transactional outbox или внешняя очередь.

Про worker pool:

> Несколько worker-ов ограничивают concurrency, но могут нарушить порядок обработки. Если порядок по сущности важен, нужен partitioning/per-key serialization или проверка версий.

Про backpressure:

> Размер буфера канала - это не просто техническая деталь. Это политика backpressure: что происходит, когда downstream не успевает. Нужно возвращать ошибку, ждать context deadline или использовать durable queue.

## Пример плохого кода

```go
type NotificationService struct {
	jobs chan Notification
	stop chan struct{}
}

func NewNotificationService() *NotificationService {
	s := &NotificationService{
		jobs: make(chan Notification),
		stop: make(chan struct{}),
	}
	go s.worker()
	return s
}

func (s *NotificationService) Send(ctx context.Context, n Notification) error {
	s.jobs <- n
	return nil
}

func (s *NotificationService) worker() {
	for {
		select {
		case n := <-s.jobs:
			_ = sendEmail(context.Background(), n.Email, n.Text)
		case <-s.stop:
			return
		}
	}
}

func (s *NotificationService) Close() {
	close(s.stop)
}
```

Проблемы:

- `jobs` unbuffered;
- `Send` может зависнуть;
- `Send` не учитывает `ctx`;
- нет реакции на Close в Send;
- worker использует `context.Background`;
- ошибка sendEmail игнорируется;
- Close не idempotent;
- Close не ждет worker-а;
- queued jobs могут потеряться;
- in-memory channel может потерять критичные уведомления при crash;
- если worker panic-нет, сервис останется без обработки jobs.

## Более здоровое направление

```go
var ErrClosed = errors.New("notification service closed")

type NotificationService struct {
	jobs chan Notification

	ctx    context.Context
	cancel context.CancelFunc

	done      chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

func NewNotificationService(parent context.Context, workers int, queueSize int) *NotificationService {
	ctx, cancel := context.WithCancel(parent)

	s := &NotificationService{
		jobs:   make(chan Notification, queueSize),
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
	}

	for i := 0; i < workers; i++ {
		s.wg.Add(1)
		go func(workerID int) {
			defer s.wg.Done()
			s.worker(workerID)
		}(i)
	}

	return s
}

func (s *NotificationService) Submit(ctx context.Context, n Notification) error {
	select {
	case s.jobs <- n:
		return nil
	case <-s.done:
		return ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *NotificationService) worker(workerID int) {
	for {
		select {
		case n := <-s.jobs:
			s.handle(n)
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *NotificationService) handle(n Notification) {
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()

	if err := sendEmail(ctx, n.Email, n.Text); err != nil {
		// log, metric, retry policy, dead letter, or status update
	}
}

func (s *NotificationService) Close() {
	s.closeOnce.Do(func() {
		close(s.done)
		s.cancel()
		s.wg.Wait()
	})
}
```

Что стало лучше:

- bounded queue;
- Submit учитывает caller context;
- есть closed signal;
- Close idempotent;
- Close ждет worker-ов;
- external call получает timeout;
- ошибки можно обработать централизованно.

Что все еще надо решить:

- drain или immediate shutdown;
- retry policy;
- durable queue для критичных уведомлений;
- метрики queue length;
- Submit одновременно с Close может иметь trade-off;
- порядок обработки при нескольких worker-ах.

Это хороший момент для собеса: не надо продавать шаблон как идеальный. Надо показать, что ты понимаешь его гарантии и ограничения.

## Мини-чеклист

- Канал buffered или unbuffered?
- Почему выбран такой buffer size?
- Send может заблокировать request?
- Send учитывает `ctx.Done()`?
- Send учитывает shutdown?
- Кто закрывает канал?
- Может ли быть send after close?
- Close можно вызвать дважды?
- Close ждет worker-ов?
- Есть ли `WaitGroup`?
- `wg.Add` вызывается до `go`?
- Worker выходит по shutdown?
- Worker использует правильный context?
- Есть ли per-job timeout?
- Ошибки worker-а обрабатываются?
- Panic worker-а убьет обработку?
- Очередь drain-ится или бросается?
- Что происходит с queued jobs при Close?
- In-memory queue подходит для критичности задач?
- Есть ли backpressure или unbounded goroutines?
- Нужен ли worker pool?
- Worker pool не ломает порядок?
- Results channel закрывает правильная сторона?
- Receive из закрытого channel проверяет `ok`?
- Нет ли busy loop с `default`?
- Нет ли `context.Background()` внутри фоновой работы?
- Есть ли метрики backlog/failures/duration?
- Тестируется ли shutdown?

## Вопросы автору кода

- Что должно произойти, если очередь заполнена?
- Должен ли caller ждать, получить ошибку или задача должна уйти в durable queue?
- Можно ли потерять задачу при shutdown?
- Эта задача критична для бизнеса?
- Почему используется in-memory channel, а не outbox/broker?
- Кто владеет закрытием jobs?
- Что произойдет при Submit после Close?
- Почему Close не ждет worker-а?
- Почему worker использует request context или Background?
- Как обрабатываются ошибки обработки job?
- Нужны ли ретраи?
- Ретраи идемпотентны?
- Сколько worker-ов оптимально и почему?
- Важен ли порядок обработки?
- Что будет, если worker panic-нет?
- Есть ли тест на double Close?
- Есть ли тест, что worker не протекает после Close?

## Короткое резюме

Каналы и worker-ы надо ревьюить как протокол, а не как отдельные строки кода.

Сильный ответ:

> Здесь канал используется как очередь задач, но не определены гарантии очереди: send может заблокировать request, shutdown не idempotent, worker не ожидается через WaitGroup, ошибки обработки теряются, а in-memory jobs могут пропасть при остановке процесса. Я бы явно задал lifecycle: bounded queue, submit через select с context и closed signal, Close через sync.Once + cancel + wg.Wait, и отдельно выбрал политику drain/retry/backpressure.

Главные идеи:

- unbuffered channel в request path может зависнуть;
- buffered channel требует capacity и overflow policy;
- закрывать канал должен владелец отправки;
- close дважды и send after close приводят к panic;
- worker-ы должны иметь lifecycle: start, stop, wait;
- shutdown должен быть idempotent и понятен;
- ошибки worker-а нельзя молча терять;
- context request-а и context worker-а не одно и то же;
- in-memory channel не заменяет durable queue;
- worker pool ограничивает concurrency, но может ломать порядок;
- `go test`, pprof и goroutine stack помогают диагностировать зависания и утечки.
