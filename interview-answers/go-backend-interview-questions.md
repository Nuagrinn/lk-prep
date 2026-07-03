# Ответы на вопросы Go/backend собеседования

Этот файл - готовый ответник по списку вопросов с собеседования. Его цель не в том, чтобы заучить текст слово в слово, а в том, чтобы у тебя была уверенная структура ответа: с чего начать, какие детали добавить, где привести пример из опыта, а где показать код и разобрать проблему.

Материал опирается на файл резюме [Резюме  Опыт Келлер Текстовый вариант.txt](<../Резюме  Опыт Келлер Текстовый вариант.txt>): ДомЛента/Лента, Лемана ПРО, Bell Integrator, IT_One, LuckySoft.

## Как пользоваться

На интервью почти никогда не надо отвечать максимально длинно. Хороший ответ обычно такой:

1. Сначала короткая уверенная формулировка.
2. Потом 1-2 технических уточнения.
3. Потом пример из опыта или кодовый пример.
4. Потом аккуратное ограничение: "если нужно глубже, могу разобрать подробнее".

В этом файле ответы длиннее, чем нужно говорить вслух. Это нормально: здесь конспект для подготовки.

## 0. Рассказ о себе

### Короткая версия на 40-60 секунд

Я backend Go-разработчик, в основном работаю с микросервисами, интеграциями и high-load-ish OLTP-сценариями вокруг e-commerce, платежей и внутренних операционных систем. Основной стек - Go, PostgreSQL, Kafka/RabbitMQ, Redis, gRPC/REST, Docker/Kubernetes, Prometheus/Grafana/OpenTelemetry.

Последний опыт - в ДомЛенте/Ленте. Там я занимался интеграционными Go-сервисами вокруг каталога, остатков, picker-процессов и клиентских коммуникаций: Kafka-потоки, BFF/service layer, обработка realtime и full snapshot остатков, DLQ, retry, circuit breaker, метрики, трассировка, подготовка сервисов к эксплуатации в Kubernetes.

До этого в Лемана ПРО делал сервис-диспетчер для браузерных ботов конкурентного мониторинга цен: RabbitMQ, Redis Pub/Sub, маршрутизация задач, retry, таймауты, observability. Еще раньше в Bell Integrator работал в эквайринге: платежные сервисы, интеграции с внешними платежными системами, PostgreSQL-оптимизации, Kafka, Redis. В IT_One делал сервисы для Госуслуг, REST/gRPC, Kafka, PostgreSQL, Redis и интеграции через СМЭВ.

Если коротко, моя сильная зона - backend на Go, надежная обработка данных и событий, работа с БД, конкурентностью, интеграциями и эксплуатационной частью: метрики, логи, retries, timeouts, graceful shutdown.

### Версия на 2-3 минуты

Я Go backend-разработчик с опытом в микросервисах, интеграциях и сервисах, где важны надежность обработки, база данных и асинхронные потоки. Последние проекты в основном были вокруг e-commerce, платежей, государственных интеграций и внутренних операционных систем.

Сейчас я работаю в команде ДомЛенты. Там моя зона - интеграционные Go-сервисы и service layer вокруг каталога, остатков, сборки заказов и клиентских коммуникаций. Например, я занимался `stocks-adapter`: чтение realtime-остатков и full snapshot из Kafka, фильтрация по SAP-кодам торговых комплексов, маппинг в контракт Offers, публикация в выходной топик, DLQ для некорректных сообщений. В таких сервисах для меня важны не только happy path, но и семантика обработки: когда коммитить Kafka offset, что отправлять в DLQ, где нужен skip, какие таймауты поставить, как корректно завершаться по shutdown.

Еще я дорабатывал основной Stocks-сервис: выносил периодическую выгрузку в отдельный one-shot job, занимался cron/supercronic, исправлял проблемы с шардированием, нулевыми остатками, гонками и дедлоками, оптимизировал расчет floor/container остатков. В picker-контуре работал с picker-service и picker-bff: смены сборщиков, переназначение заданий, проверка товара по barcode/ENSI/PIM, gRPC-контракты, Keycloak. BFF старался держать тонким, а бизнес-логику оставлять в сервисном слое.

До этого в Лемана ПРО я делал сервис-диспетчер для конкурентного мониторинга цен. Там задача была в том, чтобы централизованно раздавать задачи браузерным ботам, выбирать агента по сайту и региону, собирать результат в едином формате и доводить обработку до конца. Был RabbitMQ, Redis Pub/Sub для обновления конфигов на лету, HTTP/gRPC контракты с агентами, retries, таймауты, защита от повторной обработки, метрики и алерты.

В Bell Integrator я работал в эквайринге: платежные операции, внешние платежные системы, PostgreSQL, Kafka, Redis. Там много занимался производительностью и стабильностью: оптимизировал тяжелые запросы и JOIN-ы, правил индексы и миграции, добавлял retries с таймаутами, думал про идемпотентность и дубли во внешних интеграциях.

В IT_One работал над государственными цифровыми сервисами, в том числе интеграциями через СМЭВ 3. Делал REST/gRPC API, асинхронные callback-и через Kafka, оптимизировал PostgreSQL, а в механизме резервирования слотов решал конкурентные запросы и дубли через Redis-координацию.

По инженерным интересам мне ближе backend, где нужно не просто написать handler, а довести сервис до нормального продового состояния: контракты, БД, конкурентность, очереди, retries, observability, тесты, деплой. В Go мне нравится простота модели, явная обработка ошибок, goroutines/channels, хороший tooling и то, что язык заставляет держать код достаточно прямолинейным.

### Если спросят "с чем хочешь работать дальше"

Я бы говорил так:

> Мне интересно оставаться в backend Go, где есть реальные нагрузки, базы данных, очереди, интеграции и продовая ответственность. Нравятся задачи, где нужно не просто написать endpoint, а разобраться в бизнес-потоке, спроектировать контракт, правильно обработать ошибки и гонки, сделать наблюдаемость и тесты. Отдельно интересны темы PostgreSQL, Kafka/RabbitMQ, gRPC, надежная обработка событий и производительность.

## 1. Опыт, чем занимался, задачи. Работа с БД

Если вопрос общий, не надо пересказывать все резюме с датами. Лучше показать траекторию и 2-3 сильных примера.

Ответ:

> Основной опыт у меня в Go backend: микросервисы, интеграции, асинхронная обработка, PostgreSQL, Kafka/RabbitMQ, Redis, gRPC/REST. В последних проектах я работал в e-commerce и платежных доменах. В ДомЛенте занимался сервисами вокруг остатков, каталога, сборки заказов и коммуникаций. Например, делал Kafka-сервис для обработки realtime-остатков и full snapshot, маппил данные во внутренний контракт Offers, продумывал DLQ, commit offset, retry и graceful shutdown. В Bell Integrator работал в эквайринге: платежные операции, интеграции с внешними платежными системами, оптимизация PostgreSQL-запросов и миграций. В IT_One делал REST/gRPC API и интеграции с ведомственными системами, работал с PostgreSQL, Redis и Kafka.

По БД можно продолжить:

> С PostgreSQL работал на уровне прикладной разработки и оптимизации: писал запросы, миграции, индексы, разбирал тяжелые JOIN-ы, смотрел планы, думал про блокировки и конкурентные записи. В платежном контуре и e-commerce особенно важно, чтобы запросы не просто работали на тестовых данных, а нормально жили на продовых объемах. Поэтому я стараюсь смотреть на индексы, селективность, порядок колонок в составных индексах, миграции без долгих блокировок, а также на то, как транзакция защищает бизнес-инварианты.

Если интервьюер хочет пример:

> Один типичный пример - оптимизация тяжелых запросов в PostgreSQL. Сначала смотрел, какой запрос реально дает нагрузку, потом `EXPLAIN ANALYZE`, проверял JOIN-ы, фильтры, индексы, иногда миграции. Частая история - индекс есть, но не под тот порядок условий или не помогает с `ORDER BY LIMIT`; другая история - FK есть, а индекса на дочерней стороне нет, и удаление или обновление родителя внезапно сканирует большую таблицу.

## 2. Процесс и поток

Короткий ответ:

> Процесс - это экземпляр выполняющейся программы с собственным адресным пространством и ресурсами: память, file descriptors, открытые socket-и, переменные окружения. Поток - единица выполнения внутри процесса. Потоки одного процесса разделяют память процесса, но имеют свои stack, registers, instruction pointer. Поэтому процессы изолированы сильнее, а потоки дешевле для переключения и обмена данными, но требуют синхронизации при доступе к общей памяти.

Чуть подробнее:

Процесс можно представить как контейнер для программы. Когда запускается backend-сервис, операционная система создает процесс: выделяет виртуальное адресное пространство, загружает код, подключает библиотеки, дает process id, file descriptors, environment. Один процесс не может просто так читать память другого процесса.

Поток живет внутри процесса. Несколько потоков могут выполнять код параллельно на разных ядрах. Они видят одну и ту же heap-память процесса, поэтому обмен данными между потоками быстрее, чем между процессами. Но за это приходится платить синхронизацией: mutex, atomic, channels, lock-free структуры, очереди.

Что важно для Go:

> В Go я обычно не работаю напрямую с OS threads. Я создаю goroutines, а runtime сам планирует их на набор системных потоков. Но понимать процесс/поток полезно: блокирующий syscall, CPU-bound нагрузка, scheduler, `GOMAXPROCS`, race conditions - все это связано с тем, как goroutines исполняются поверх потоков ОС.

## 3. gRPC запросы: как отрабатываются

Ответ можно строить от клиента к серверу.

> gRPC обычно работает поверх HTTP/2 и использует protobuf для сериализации. На клиенте мы вызываем метод сгенерированного stub-а, например `client.GetOrder(ctx, req)`. Клиентский gRPC-код сериализует request в protobuf, добавляет metadata, deadline/cancellation из context, прогоняет interceptors и отправляет HTTP/2 stream на сервер.

На сервере:

> На сервере gRPC listener принимает соединения, HTTP/2 позволяет мультиплексировать несколько stream-ов по одному TCP-соединению. Для конкретного RPC сервер декодирует protobuf-сообщение, создает context с metadata/deadline/cancel, прогоняет server interceptors - например logging, tracing, auth, metrics - и вызывает handler. Handler обычно идет в service layer, репозитории, внешние клиенты, потом возвращает response или error. Error мапится в gRPC status code: `InvalidArgument`, `NotFound`, `Unauthenticated`, `Unavailable`, `DeadlineExceeded` и так далее.

Что добавить из опыта:

> В проектах я работал с gRPC-контрактами и protobuf, например в picker-контуре и интеграционных сервисах. Для меня важные вещи в gRPC - это совместимость контрактов, аккуратные status codes, deadline propagation, metadata для auth/trace context, interceptors для логов и метрик, а также понимание, где retry допустим, а где может создать дубль бизнесовой операции.

Нюансы:

- gRPC использует HTTP/2 streams, поэтому одно соединение может обслуживать много запросов.
- Deadline должен идти через `context.Context`.
- Если клиент отменил request, серверный context canceled, handler должен уметь остановиться.
- Ошибки лучше возвращать через `status.Error(codes.X, message)`, а не просто `fmt.Errorf`.
- Для idempotent read-запросов retry обычно проще, для write-запросов нужна идемпотентность.
- В streaming RPC надо внимательно работать с lifecycle stream-а и backpressure.

Пример кода:

```go
func (h *OrderHandler) GetOrder(ctx context.Context, req *pb.GetOrderRequest) (*pb.GetOrderResponse, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "order id is required")
	}

	order, err := h.service.GetOrder(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, domain.ErrOrderNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		return nil, status.Error(codes.Internal, "get order failed")
	}

	return mapOrderToProto(order), nil
}
```

Что сказать по этому коду:

> Handler тонкий: валидирует transport-level вход, вызывает service layer, маппит доменные ошибки в gRPC codes и не содержит бизнес-логики. Context пробрасывается вниз, чтобы deadline/cancel дошли до БД и внешних клиентов.

## 4. Горутины и системные потоки: как связаны

Коротко:

> Goroutine - легковесная единица конкурентного выполнения в Go. Она не равна системному потоку. Go runtime планирует много goroutines на меньшее или равное/большее число OS threads через модель M:N. `GOMAXPROCS` задает, сколько logical processors могут одновременно исполнять Go-код.

Модель Go scheduler часто объясняют через G/M/P:

- G - goroutine;
- M - machine, системный поток;
- P - processor, логический ресурс runtime, который позволяет M исполнять Go-код.

Когда ты пишешь:

```go
go processOrder(order)
```

создается goroutine. Она получает маленький начальный stack, который может расти. Runtime ставит ее в очередь. Дальше scheduler назначает goroutine на выполнение через P на конкретном OS thread M.

Почему goroutines дешевле потоков:

- маленький растущий stack;
- создание дешевле;
- переключение дешевле, потому что управляется runtime;
- удобно создавать тысячи и десятки тысяч goroutines для IO-bound задач.

Но goroutines не магия:

- CPU-bound goroutines конкурируют за `GOMAXPROCS`;
- shared memory все равно требует синхронизации;
- goroutine может утечь, если она навсегда ждет channel/read/context;
- блокирующие syscalls и cgo имеют свои нюансы;
- слишком много goroutines может давить памятью, scheduler overhead и внешними ресурсами.

Пример хорошего ответа:

> Я воспринимаю goroutine как дешевую конкурентную задачу, а не как поток. Runtime сам маппит goroutines на threads. Поэтому я могу запускать goroutine на обработку Kafka-сообщения или фонового worker-а, но должен контролировать lifecycle: context, WaitGroup, закрытие каналов, лимиты параллелизма и backpressure.

## 5. Проблемы с многопоточностью в Go, с чем сталкивался

Этот вопрос лучше отвечать через реальные классы проблем.

Ответ:

> В Go чаще всего сталкивался не с "потоками" напрямую, а с конкурентностью: data race, некорректная работа с map/cache, goroutine leaks, блокировки на каналах, неправильный shutdown worker-ов, дедлоки вокруг mutex-ов и неатомарные read-modify-write операции.

Из опыта можно привязать так:

> В ДомЛенте в контуре остатков и jobs были задачи вокруг Kafka consumers, graceful shutdown, commit offset после успешной обработки, дедлоки и гонки в расчетах остатков. В Лемана ПРО в dispatcher-е были очереди, retries, зависшие задачи, ограничение времени обработки и защита от повторной обработки. Там важно было не просто запустить goroutines, а ограничить параллелизм, корректно завершать worker-ы и не терять сообщения.

Типы проблем:

Data race:

```go
var processed int

for _, msg := range messages {
	go func() {
		processed++
	}()
}
```

`processed++` не атомарен. Это read-modify-write. Нужен `sync.Mutex`, `atomic`, channel aggregation или другая архитектура.

Concurrent map writes:

```go
cache := map[string]Order{}

go func() { cache["1"] = order }()
go func() { cache["2"] = order }()
```

Обычная map не потокобезопасна для конкурентной записи.

Goroutine leak:

```go
func startWorker(in <-chan Job) {
	go func() {
		for job := range in {
			process(job)
		}
	}()
}
```

Если `in` никогда не закрывается и нет context, worker может жить вечно.

Channel deadlock:

```go
ch := make(chan int)
ch <- 1
fmt.Println(<-ch)
```

В одной goroutine отправка в unbuffered channel заблокируется навсегда, потому что receiver еще не запущен.

Как диагностировать:

- `go test -race`;
- pprof goroutine dump;
- `runtime.NumGoroutine`;
- логи lifecycle worker-ов;
- метрики очередей и in-flight задач;
- проверка context cancellation;
- code review на ownership channel close.

## 6. Код-ревью блок: каналы, блокировки, неатомарность, синхронизация, слайсы

На интервью это часто выглядит так: дают небольшой код и просят "что тут может пойти не так".

### 6.1 Каналы: какие бывают

Каналы в Go бывают:

- unbuffered: `make(chan T)`;
- buffered: `make(chan T, n)`;
- nil channel: `var ch chan T`;
- closed channel;
- directional: receive-only `<-chan T`, send-only `chan<- T`.

Поведение:

- send в unbuffered channel блокируется, пока нет receiver;
- receive из unbuffered channel блокируется, пока нет sender;
- send в full buffered channel блокируется;
- receive из empty buffered channel блокируется;
- receive из closed channel сразу возвращает zero value и `ok=false`, если буфер пуст;
- send в closed channel panic;
- close closed channel panic;
- операции с nil channel блокируются навсегда;
- закрывать канал должен sender/owner, а не receiver.

### 6.2 Задача: найти блокировку в коде

Код:

```go
func collectEven(ctx context.Context, in <-chan int) []int {
	out := make(chan int)

	go func() {
		for v := range in {
			if v%2 == 0 {
				out <- v
			}
		}
	}()

	var result []int
	for v := range out {
		result = append(result, v)
	}

	return result
}
```

Что здесь не так:

1. `out` никогда не закрывается.

   Цикл:

   ```go
   for v := range out
   ```

   завершится только после `close(out)`. Но goroutine после окончания `in` просто выходит и не закрывает `out`. Получаем deadlock.

2. `ctx` не используется.

   Если caller отменил операцию, worker продолжит ждать `in` или пытаться отправлять в `out`.

3. Отправка `out <- v` может заблокироваться, если receiver уже ушел.

4. Не определен owner закрытия `out`.

Исправление:

```go
func collectEven(ctx context.Context, in <-chan int) []int {
	out := make(chan int)

	go func() {
		defer close(out)

		for {
			select {
			case <-ctx.Done():
				return
			case v, ok := <-in:
				if !ok {
					return
				}
				if v%2 != 0 {
					continue
				}

				select {
				case <-ctx.Done():
					return
				case out <- v:
				}
			}
		}
	}()

	var result []int
	for v := range out {
		result = append(result, v)
	}

	return result
}
```

Что сказать на интервью:

> Я бы в первую очередь проверил lifecycle каналов: кто пишет, кто читает, кто закрывает, что происходит при отмене context, может ли send/receive заблокироваться навсегда. Здесь `out` не закрывается, поэтому range по нему зависнет. Еще context передан, но не используется.

### 6.3 Неатомарная ситуация

Код:

```go
type Counter struct {
	value int
}

func (c *Counter) Inc() {
	c.value++
}

func (c *Counter) Value() int {
	return c.value
}
```

Проблема:

`c.value++` - не атомарная операция. Это примерно:

1. прочитать `value`;
2. прибавить 1;
3. записать обратно.

Если две goroutine делают это одновременно, одна запись может потеряться.

Исправление через mutex:

```go
type Counter struct {
	mu    sync.Mutex
	value int
}

func (c *Counter) Inc() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.value++
}

func (c *Counter) Value() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.value
}
```

Исправление через atomic:

```go
type Counter struct {
	value atomic.Int64
}

func (c *Counter) Inc() {
	c.value.Add(1)
}

func (c *Counter) Value() int64 {
	return c.value.Load()
}
```

Как выбрать:

- `atomic` хорош для простых счетчиков/флагов;
- `mutex` лучше, когда защищаем несколько полей или инвариант;
- channel aggregation может быть хороша, если нужна событийная модель.

### 6.4 Срезы и массивы: как проверить пустоту

Массив:

```go
var a [3]int
```

Размер массива - часть типа. `[3]int` и `[4]int` - разные типы.

Срез:

```go
var s []int
```

Срез - это header:

```text
pointer to array
len
cap
```

Проверка пустоты:

```go
if len(s) == 0 {
	// empty
}
```

Так корректно и для nil slice, и для empty non-nil slice.

```go
var a []int        // nil, len=0
b := []int{}       // non-nil, len=0
c := make([]int, 0) // non-nil, len=0
```

Почти всегда для "пустой или нет" надо использовать `len(s) == 0`, а не `s == nil`.

### 6.5 Как слайсы передаются в функцию

Слайс передается по значению. Но значение слайса - это header, который указывает на underlying array.

```go
func changeFirst(values []int) {
	values[0] = 100
}

func main() {
	x := []int{1, 2, 3}
	changeFirst(x)
	fmt.Println(x) // [100 2 3]
}
```

Почему изменилось? Потому что header скопировали, но pointer внутри header указывает на тот же underlying array.

Но если внутри функции сделать append, внешний header не изменится:

```go
func appendValue(values []int) {
	values = append(values, 4)
}

func main() {
	x := []int{1, 2, 3}
	appendValue(x)
	fmt.Println(x) // [1 2 3]
}
```

Внутренний `values` получил новый len, но внешний `x` остался со старым len. Если append поместился в тот же underlying array, массив мог измениться под капотом, но внешний слайс этого не покажет, потому что его len не изменился.

Поэтому `append` возвращает слайс:

```go
x = append(x, 4)
```

### 6.6 Задача: что выведет код со слайсами

Код:

```go
func main() {
	a := []int{1, 2, 3}
	b := a[:2]
	c := append(b, 9)

	fmt.Println("a:", a, len(a), cap(a))
	fmt.Println("b:", b, len(b), cap(b))
	fmt.Println("c:", c, len(c), cap(c))

	d := append(c, 10, 11)
	d[0] = 100

	fmt.Println("a2:", a)
	fmt.Println("b2:", b)
	fmt.Println("c2:", c)
	fmt.Println("d:", d)
}
```

Разбор:

Изначально:

```text
a = [1 2 3], len=3, cap=3
b = a[:2] -> [1 2], len=2, cap=3
```

`append(b, 9)` помещается в capacity `b`, поэтому используется тот же underlying array. Третий элемент массива становится `9`.

```text
a = [1 2 9]
b = [1 2]
c = [1 2 9]
```

Потом:

```go
d := append(c, 10, 11)
```

У `c` len=3 cap=3. Добавить два элемента нельзя без перевыделения. Поэтому `d` получает новый underlying array.

`d[0] = 100` меняет только новый массив `d`, а `a/b/c` остаются на старом массиве.

Вывод:

```text
a: [1 2 9] 3 3
b: [1 2] 2 3
c: [1 2 9] 3 3
a2: [1 2 9]
b2: [1 2]
c2: [1 2 9]
d: [100 2 9 10 11]
```

Что сказать:

> Главный принцип: slice header копируется, underlying array может быть общим. Append либо пишет в тот же массив, если хватает cap, либо выделяет новый массив. Поэтому после append всегда надо использовать возвращенный slice.

## 7. Unit tests: писал ли и как понимать, что программа не сломалась

Ответ с опорой на опыт:

> Да, писал unit и integration tests. В последних проектах тестировал критичную бизнес-логику и интеграционные куски: фильтрацию остатков, обработку Kafka-сообщений, producer-consumer сценарии, маппинг данных, ошибки внешних клиентов, маршрутизацию задач в dispatcher-е, retry/timeout поведение. Для меня тесты - это не просто проверка функций, а способ безопасно менять код, особенно когда есть интеграции и много edge cases.

Что такое unit test:

> Unit test проверяет небольшой модуль изолированно: функцию, service method, mapper, validator. Внешние зависимости обычно заменяются fake/mock/stub. Хороший unit test быстрый, детерминированный и проверяет поведение, а не внутреннюю реализацию.

Как понимать, что программа не сломалась:

- unit tests на бизнес-логику;
- integration tests на БД/очереди/контракты;
- contract tests для внешних API/gRPC/protobuf;
- regression tests на найденные баги;
- CI pipeline;
- code review;
- static analysis и linters;
- race detector для concurrent кода;
- observability после релиза: метрики, алерты, логи, traces;
- canary/rolling update.

Хорошая формулировка:

> Полной гарантии, что ничего не сломалось, тесты не дают. Но они снижают риск. Я стараюсь покрывать тестами места, где высокая цена ошибки: маппинг контрактов, бизнес-инварианты, обработка ошибок, retries, конкурентные сценарии, работа с БД. А после выката смотрю метрики и алерты, потому что интеграционные проблемы иногда видны только на реальной нагрузке.

Пример unit test:

```go
func TestMapStockMessage_SkipsUnknownStore(t *testing.T) {
	msg := StockMessage{
		StoreID: "unknown",
		SKU:     "123",
		Qty:     10,
	}

	got, ok := MapStockMessage(msg, AllowedStores{"spb-1": true})

	require.False(t, ok)
	require.Empty(t, got)
}
```

## 8. Интерфейсы: использовал ли, для чего, утиная типизация

Ответ:

> Да, интерфейсы использовал постоянно: для разделения service layer и внешних зависимостей, для репозиториев, клиентов внешних сервисов, Kafka producers/consumers, clock/id generator в тестах. В Go интерфейс задает поведение, а тип реализует его неявно. Это и называют duck typing: если тип имеет нужные методы, он удовлетворяет интерфейсу, явно писать `implements` не надо.

Пример:

```go
type OrderRepository interface {
	GetByID(ctx context.Context, id string) (Order, error)
	Save(ctx context.Context, order Order) error
}

type PaymentClient interface {
	Charge(ctx context.Context, req ChargeRequest) (ChargeResult, error)
}

type Service struct {
	orders   OrderRepository
	payments PaymentClient
}
```

Зачем:

- тестируемость;
- слабая связность;
- возможность заменить реализацию;
- отделение бизнес-логики от transport/storage;
- удобная архитектура service/repository/client.

Нюанс Go:

> В Go часто лучше принимать интерфейс там, где он нужен, а возвращать конкретную структуру. И не стоит плодить интерфейсы заранее. Если есть одна реализация и нет тестовой/архитектурной необходимости, интерфейс может быть лишним.

Пример из опыта:

> В интеграционных сервисах удобно закрывать внешние HTTP/gRPC клиенты интерфейсами. Тогда service layer можно тестировать без реального Akeneo, OMS, платежного провайдера или Kafka producer-а. В тесте подставляем fake-клиент и проверяем бизнесовое поведение.

## 9. С какими языками работал, кроме Go

Из резюме основной стек - Go, есть Java в навыках, Linux/scripts, опыт аналитика с пайплайнами и парсерами.

Ответ лучше честный:

> Основной production-язык у меня Go. Кроме него работал/сталкивался с Java на уровне понимания backend-кода и экосистемы, писал скрипты и парсеры в аналитических задачах, работал с SQL достаточно много. C/C++ не является моим основным production-стеком, но базовый синтаксис и идеи вроде указателей, ручного управления памятью, stack/heap, struct, header/source files могу читать и обсуждать.

Если хотят сравнить:

> С точки зрения backend-разработки я сильнее всего в Go. По другим языкам могу читать код, понимать общие принципы, но если задача требует глубокой экспертизы C++, я бы честно сказал, что это не мой основной коммерческий опыт.

## 10. C++ и Go: что больше нравится, плюсы. Чтение C-кода

Здесь важно не ругать C++ и не притворяться C++-сеньором.

Ответ:

> Для backend-сервисов мне больше нравится Go. Он проще по модели, быстрее читается, имеет хороший стандартный tooling, удобную конкурентность через goroutines/channels, явную обработку ошибок, хорошую экосистему для микросервисов, gRPC, HTTP, observability. В Go проще держать сервис поддерживаемым командой.

Про C++:

> C++ сильнее там, где нужен максимальный контроль над памятью, latency, low-level, embedded, high-performance вычисления, игровые движки, системное программирование. Но за контроль платишь сложностью: memory management, undefined behavior, сложная модель языка, headers/templates, lifetime объектов, больше способов ошибиться.

Если дадут C-код читать:

> Я бы смотрел на типы, указатели, владение памятью, `malloc/free`, выходы за границы массива, null pointer, lifetime буферов, integer overflow, работу со строками через `char*`, возвращаемые коды ошибок. Даже если я не пишу C каждый день, базовые проблемы безопасности и памяти понимаю.

Пример C-фрагмента:

```c
char *make_name(const char *first, const char *last) {
    char buf[128];
    sprintf(buf, "%s %s", first, last);
    return buf;
}
```

Что сказать:

> Здесь возвращается указатель на локальный stack buffer `buf`. После выхода из функции память уже недействительна. Плюс `sprintf` небезопасен из-за возможного переполнения. Нужно выделять память снаружи/через malloc и использовать snprintf, либо передавать буфер и размер.

## 11. Мапы в Go: что выведет функция main

Пример задачи:

```go
func main() {
	m := map[string]int{"a": 1}

	fmt.Println(m["b"])

	v, ok := m["b"]
	fmt.Println(v, ok)

	m["b"]++
	fmt.Println(m["b"], len(m))

	delete(m, "c")
	fmt.Println(len(m))
}
```

Вывод:

```text
0
0 false
1 2
2
```

Разбор:

Чтение отсутствующего ключа возвращает zero value типа значения. Для `int` это `0`.

Чтобы отличить "ключа нет" от "ключ есть, но значение 0", используют comma ok:

```go
v, ok := m["b"]
```

`m["b"]++` для отсутствующего ключа работает так: читается zero value `0`, прибавляется 1, записывается `1`. После этого в map уже два ключа: `"a"` и `"b"`.

`delete(m, "c")` безопасен, даже если ключа нет.

Nil map:

```go
var m map[string]int

fmt.Println(m["x"]) // 0
m["x"] = 1          // panic: assignment to entry in nil map
```

Важно:

- читать из nil map можно;
- писать в nil map нельзя;
- порядок обхода map через `range` не гарантирован;
- обычная map не потокобезопасна для конкурентной записи;
- ключами могут быть только comparable-типы.

## 12. Всегда ли хэширование быстрее, чем связанная таблица/список

Ответ:

> Нет, не всегда. Hash table обычно дает средний доступ O(1), но это не значит, что она всегда быстрее любой другой структуры. Реальная скорость зависит от размера данных, cache locality, стоимости hash-функции, количества коллизий, аллокаций, характера запросов и того, что именно мы делаем: поиск, range scan, сортировка, итерация.

Если сравнивать hash map и linked list:

- hash map хорош для поиска по ключу;
- linked list плох для поиска, потому что O(n);
- но linked list может быть уместен для частых вставок/удалений при уже известном node;
- linked list плохо дружит с CPU cache, потому что узлы разбросаны по памяти;
- маленький slice иногда быстрее map, потому что линейный проход по компактной памяти дешевле hash overhead.

Пример:

```go
func containsSmall(xs []int, target int) bool {
	for _, x := range xs {
		if x == target {
			return true
		}
	}
	return false
}
```

Для 5-10 элементов такой slice может быть быстрее `map[int]struct{}` из-за отсутствия hash overhead и хорошей cache locality.

Для БД похожая идея:

> Индекс/hash-структура не всегда быстрее последовательного чтения. Если нужно прочитать большую часть таблицы, sequential scan может быть дешевле, чем много random index lookups.

## 13. SQL и NoSQL: что это, чем отличаются, когда выбирать

Ответ:

> SQL-базы - это обычно реляционная модель: таблицы, строки, колонки, SQL, связи, JOIN, транзакции, constraints, строгая схема. Пример - PostgreSQL. NoSQL - широкий набор нереляционных хранилищ: key-value, document, column-family, graph. Например Redis, MongoDB, Cassandra. Они часто выбираются под конкретный профиль данных и нагрузки.

SQL выбирать, когда:

- важны транзакции;
- есть связи между сущностями;
- нужны JOIN;
- нужна сильная консистентность;
- важны constraints;
- данные имеют понятную модель;
- нужны сложные запросы и отчеты в рамках OLTP.

NoSQL выбирать, когда:

- нужна очень простая модель key-value с высокой скоростью;
- нужны TTL и cache - Redis;
- документная модель удобнее таблиц;
- нужно горизонтальное масштабирование под огромный write throughput;
- данные слабо связаны;
- допустима eventual consistency;
- нужен специализированный движок.

Из опыта:

> Я чаще работал с PostgreSQL как основной OLTP-БД, а Redis использовал как кэш, TTL-хранилище, механизм координации/блокировок или техническое состояние. Kafka/RabbitMQ - не БД, но важная часть асинхронной обработки. ClickHouse я бы рассматривал для аналитических запросов и агрегаций по большим объемам, а не как замену PostgreSQL для транзакционного контура.

Хороший вывод:

> Я бы выбирал не по моде SQL/NoSQL, а по требованиям: транзакции, модель данных, тип запросов, объем, latency, consistency, эксплуатация и команда.

## 14. PostgreSQL: индексы, всегда ли улучшают, составной индекс, INCLUDE

Короткий ответ:

> Индекс - это дополнительная структура данных, которая ускоряет некоторые чтения, но замедляет запись и занимает место. Индекс не всегда улучшает запрос: если таблица маленькая, условие не селективное или запрос читает большую часть таблицы, PostgreSQL может выбрать Seq Scan. Индекс полезен, когда он резко сокращает количество читаемых строк/страниц, помогает JOIN, `ORDER BY LIMIT`, уникальности или index-only scan.

Какие индексы использовал:

> На практике в основном B-tree: primary key, unique, индексы по FK, составные индексы под фильтры и сортировку, partial indexes для активных/неудаленных записей, иногда GIN для JSONB/text-поиска. Еще важно не только создавать индексы, но и смотреть лишние/дублирующие, потому что они ухудшают запись и VACUUM.

Составной индекс:

```sql
CREATE INDEX idx_orders_user_status_created
ON orders(user_id, status, created_at DESC);
```

Он хорошо подходит для:

```sql
SELECT *
FROM orders
WHERE user_id = $1
  AND status = 'paid'
ORDER BY created_at DESC
LIMIT 20;
```

Порядок колонок важен. Индекс `(user_id, status, created_at)` не равен `(status, user_id, created_at)`. Обычно думают про equality-условия, range, sort и селективность.

`INCLUDE`:

```sql
CREATE INDEX idx_orders_user_created_include_amount
ON orders(user_id, created_at DESC)
INCLUDE (amount, status);
```

`INCLUDE` добавляет неключевые колонки в индекс. Они не участвуют в поиске и сортировке как key columns, но могут позволить ответить из индекса без похода в heap.

Пример:

```sql
SELECT created_at, amount, status
FROM orders
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT 20;
```

Если все нужные поля есть в индексе и visibility map позволяет, возможен Index Only Scan.

Нюанс:

> `INCLUDE` увеличивает размер индекса. Его не надо использовать как "добавлю все поля на всякий случай".

## 15. Задача: функция замокана, неизменяема. Мьютексы. Как модернизировать для тестирования

Пример плохого кода:

```go
var fetchRate = externalFetchRate

var rates = map[string]float64{}

func GetRate(ctx context.Context, currency string) (float64, error) {
	if v, ok := rates[currency]; ok {
		return v, nil
	}

	v, err := fetchRate(ctx, currency)
	if err != nil {
		return 0, err
	}

	rates[currency] = v
	return v, nil
}
```

Что здесь может быть проблемой:

1. Глобальная map `rates` не защищена mutex-ом.

   При параллельных запросах будет data race или `fatal error: concurrent map writes`.

2. Глобальная переменная `fetchRate` как mock hook.

   В тестах ее могут переопределять, но это ломает параллельные тесты и делает порядок тестов важным.

3. Check-then-act не атомарен.

   Две goroutine могут одновременно не найти ключ и обе пойти во внешний сервис.

4. Нет инкапсуляции состояния.

   Cache и dependency живут глобально.

5. Труднее тестировать.

   Нужно менять global state и чистить его после тестов.

Минимальное исправление mutex-ом:

```go
var (
	ratesMu sync.RWMutex
	rates   = map[string]float64{}
)

func GetRate(ctx context.Context, currency string) (float64, error) {
	ratesMu.RLock()
	v, ok := rates[currency]
	ratesMu.RUnlock()
	if ok {
		return v, nil
	}

	v, err := fetchRate(ctx, currency)
	if err != nil {
		return 0, err
	}

	ratesMu.Lock()
	rates[currency] = v
	ratesMu.Unlock()

	return v, nil
}
```

Но это все еще неидеально: глобальная зависимость и возможные duplicate external calls.

Лучше модернизировать:

```go
type RateClient interface {
	FetchRate(ctx context.Context, currency string) (float64, error)
}

type RateService struct {
	client RateClient

	mu    sync.RWMutex
	cache map[string]float64
}

func NewRateService(client RateClient) *RateService {
	return &RateService{
		client: client,
		cache:  make(map[string]float64),
	}
}

func (s *RateService) GetRate(ctx context.Context, currency string) (float64, error) {
	s.mu.RLock()
	v, ok := s.cache[currency]
	s.mu.RUnlock()
	if ok {
		return v, nil
	}

	v, err := s.client.FetchRate(ctx, currency)
	if err != nil {
		return 0, err
	}

	s.mu.Lock()
	s.cache[currency] = v
	s.mu.Unlock()

	return v, nil
}
```

Еще лучше для duplicate calls можно использовать `singleflight`, но на интервью достаточно сказать:

> Я бы убрал глобальное состояние, завернул зависимость в интерфейс, состояние cache положил внутрь service struct, защитил map mutex-ом и в тестах подставлял fake client. Если важно не делать несколько одинаковых внешних вызовов параллельно, добавил бы singleflight или блокировку на ключ.

Пример fake для теста:

```go
type fakeRateClient struct {
	value float64
	err   error
}

func (f fakeRateClient) FetchRate(ctx context.Context, currency string) (float64, error) {
	return f.value, f.err
}
```

## 16. Инструменты тестирования

Ответ:

> В Go основной инструмент - стандартный пакет `testing`: `go test`, table-driven tests, subtests через `t.Run`, benchmarks, examples. Дополнительно использовал assertion-библиотеки вроде `testify/require`, mocks/fakes, race detector, интеграционные тесты с Docker/testcontainers или поднятыми зависимостями на стенде, coverage, linters в CI.

Что перечислить:

- `go test ./...`;
- `go test -race ./...`;
- `go test -cover ./...`;
- table-driven tests;
- subtests;
- benchmarks: `go test -bench=.`;
- `httptest` для HTTP;
- fake gRPC/HTTP clients;
- `gomock`/`mockery` при необходимости;
- `testify/require`;
- integration tests с PostgreSQL/Kafka/RabbitMQ/Redis;
- contract tests для API/protobuf;
- CI: GitLab CI/Jenkins;
- linters: `golangci-lint`;
- pprof для performance.

Хороший нюанс:

> Я стараюсь не мокать все подряд. Для бизнес-логики удобно использовать fake реализации интерфейсов. Для репозитория иногда лучше integration test с реальной PostgreSQL, потому что SQL, транзакции и индексы моками нормально не проверишь.

## 17. Большой файл на несколько ГБ надо отсортировать побайтно

Здесь важно уточнить: "побайтно" означает отсортировать отдельные байты файла, а не строки. Если именно байты, задача проще, потому что возможных значений всего 256.

Ответ:

> Если нужно отсортировать файл именно по байтам, я бы использовал counting sort. Читаем файл потоково чанками, считаем частоту каждого byte value в массиве `[256]uint64`, потом создаем выходной файл и записываем байты от 0 до 255 нужное число раз. Память O(1), время O(n), файл можно читать хоть на десятки гигабайт.

Псевдокод:

```go
func SortBytes(inputPath, outputPath string) error {
	in, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer in.Close()

	var counts [256]uint64
	buf := make([]byte, 4*1024*1024)

	for {
		n, readErr := in.Read(buf)
		for _, b := range buf[:n] {
			counts[b]++
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()

	writeBuf := make([]byte, 4*1024*1024)
	for value, count := range counts {
		b := byte(value)
		for count > 0 {
			n := len(writeBuf)
			if uint64(n) > count {
				n = int(count)
			}
			for i := 0; i < n; i++ {
				writeBuf[i] = b
			}
			if _, err := out.Write(writeBuf[:n]); err != nil {
				return err
			}
			count -= uint64(n)
		}
	}

	return out.Sync()
}
```

Что важно проговорить:

- не загружаем файл целиком в память;
- читаем streaming chunks;
- массив счетчиков всего на 256 значений;
- запись тоже chunked;
- обрабатываем ошибки read/write;
- если важна атомарность результата, пишем во временный файл и потом rename;
- если входной файл может быть больше 2^64 байт, счетчики должны быть больше, но на практике `uint64` достаточно.

Если интервьюер имел в виду "отсортировать строки большого файла", ответ другой:

> Тогда нужен external merge sort: читаем файл кусками, каждый кусок сортируем в памяти, пишем sorted run во временный файл, потом k-way merge через heap. Память ограничена размером chunk-а и heap-ом по числу run-ов.

## Redis и ClickHouse: пару слов

### Redis

> Redis использовал как кэш, TTL-хранилище, техническое состояние и иногда механизм координации. Например, для разгрузки БД, хранения временных состояний, Pub/Sub для обновления конфигов, блокировок/координации в сценариях резервирования. Важно помнить, что Redis - не замена транзакционной БД по умолчанию. Нужно думать про TTL, eviction policy, consistency, idempotency и то, что будет при падении Redis.

Примеры:

- cache frequently used data;
- distributed-ish lock with TTL, осторожно;
- rate limiting;
- Pub/Sub для конфигов;
- session/technical state;
- deduplication keys.

### ClickHouse

> ClickHouse я бы рассматривал как колоночную аналитическую БД для больших объемов событий, логов, метрик и агрегатов. Она хороша для OLAP: быстро читать отдельные колонки, считать агрегации по большим объемам. Но для OLTP, транзакций, частых точечных update/delete и сложных constraints PostgreSQL подходит лучше.

Когда выбирать:

- отчеты;
- dashboard-и;
- event analytics;
- продуктовые метрики;
- большие scan/aggregate запросы.

Когда не выбирать как основную:

- платежная транзакция;
- баланс пользователя;
- заказ с ACID-инвариантами;
- частые update по одной строке.

## Финальная шпаргалка коротких ответов

Процесс/поток:

> Процесс - изолированное адресное пространство и ресурсы. Поток - единица выполнения внутри процесса, потоки делят память процесса.

Goroutine:

> Легковесная задача Go runtime, не равна OS thread. Scheduler маппит много goroutines на threads.

gRPC:

> HTTP/2 + protobuf + generated stubs + context/deadline/metadata + interceptors + status codes.

Channels:

> Unbuffered, buffered, nil, closed, directional. Главные риски - deadlock, send to closed, leak, неверный owner close.

Slice:

> Header `{ptr,len,cap}` передается по значению, underlying array общий. Append возвращает новый slice, потому что header мог измениться.

Map:

> Hash table, порядок range не гарантирован, read absent key дает zero value, comma ok отличает отсутствие, concurrent writes unsafe.

Interfaces:

> Поведение задается методами, реализация неявная. Использую для зависимостей и тестируемости, но не плодить заранее.

PostgreSQL index:

> Ускоряет не все. Помогает селективным условиям, JOIN, ORDER BY LIMIT, uniqueness, index-only scan. Платим записью, местом и обслуживанием.

INCLUDE:

> Неключевые колонки в индексе для covering/index-only scan. Не участвуют в поиске как key columns.

SQL vs NoSQL:

> Выбор по модели данных, транзакциям, запросам, consistency и нагрузке. PostgreSQL для OLTP и связей, Redis для cache/TTL, ClickHouse для аналитики.

Big file byte sort:

> Counting sort на 256 счетчиков, streaming read/write, O(n) time, O(1) memory.
