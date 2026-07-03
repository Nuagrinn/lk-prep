# API и архитектура сервисного слоя

Эта тема про то, как устроен сервисный слой: какие методы он дает наружу, какие зависимости принимает, где проходят границы ответственности, какие side effects прячет или наоборот делает явными, как выглядит контракт метода и насколько этот контракт удобно сопровождать.

Это более теоретическая тема, чем data race или транзакции. Здесь часто нет одной строки, про которую можно сказать: “вот тут точно panic”. Но именно такие замечания хорошо показывают зрелость на code review. Ты видишь не только локальную ошибку, но и то, во что код превратится через полгода: будет ли его удобно тестировать, расширять, дебажить, повторять операции, менять бизнес-логику.

Главная мысль:

> Сервисный слой должен выражать бизнес-сценарии и их контракт, а не быть случайной смесью repository, HTTP-клиента, кеша, транзакций, горутин, метрик и бизнес-правил.

И вторая мысль:

> Хороший API сервиса делает важные вещи явными: что метод делает, какие у него side effects, что он возвращает, какие ошибки возможны, можно ли его повторить, кто владеет транзакцией и где проходит граница ответственности.

## Простое введение

Представь сервис:

```go
type OrderService struct {
	repo     Repository
	payments PaymentGateway
	events   EventPublisher
	cache    Cache
}
```

И метод:

```go
func (s *OrderService) GetOrders(ctx context.Context, userID string, limit int) ([]Order, error) {
	orders, err := s.repo.FindOrders(ctx, userID, limit)
	if err != nil {
		return nil, err
	}

	for _, order := range orders {
		go s.events.Publish(ctx, "order.viewed", order.ID)
	}

	return orders, nil
}
```

На первый взгляд это просто метод “получить заказы”. Но на самом деле он:

- читает из базы;
- запускает горутины;
- публикует события;
- использует request context внутри фоновой операции;
- игнорирует ошибки публикации;
- создает side effect “order viewed”;
- потенциально перегружает брокер при большом списке заказов.

Название `GetOrders` не говорит о таких последствиях. Вызывающий код думает, что это query, а метод ведет себя как command.

Вот это и есть архитектурная проблема API сервисного слоя. Не обязательно одна строка неправильная синтаксически. Неправильный сам контракт: метод обещает одно, делает другое, а важные последствия спрятаны внутри.

На ревью это можно сказать так:

> Метод называется как read-only операция, но внутри есть публикация события и запуск горутин. Я бы разделил получение данных и фиксацию факта просмотра, либо сделал side effect явным в названии и контракте.

## Что такое сервисный слой

В типичном backend-приложении можно грубо выделить слои:

```text
Transport / delivery layer
HTTP handlers, gRPC handlers, consumers, cron handlers

Application / service layer
Use cases, orchestration, transaction boundaries, policies

Domain layer
Entities, value objects, business rules, domain services

Infrastructure layer
Database, broker, external APIs, cache, filesystem
```

В Go это не всегда выражено папками `domain`, `application`, `infrastructure`. Маленькие сервисы часто проще. Но логические роли все равно существуют.

Сервисный слой обычно отвечает за:

- выполнение бизнес-сценария;
- координацию repository и внешних клиентов;
- управление транзакцией, если это часть use case;
- проверку бизнес-инвариантов;
- принятие решений по статусам и переходам;
- вызов доменной логики;
- формирование результата для верхнего слоя;
- публикацию событий через корректный механизм;
- observability: логические метрики, tracing, meaningful errors.

Сервисный слой обычно не должен:

- знать детали HTTP: status code, headers, cookies;
- напрямую формировать JSON;
- зависеть от конкретного SQL-запроса;
- прятать внутри себя случайные фоновые горутины без контракта;
- возвращать наружу ORM-модели как публичный API;
- превращаться в огромный god object;
- смешивать unrelated use cases в одном методе;
- делать непредсказуемые side effects в методах, похожих на чтение.

Важно: “обычно” не значит “никогда”. В реальных проектах бывают компромиссы. На ревью сила не в догматизме, а в способности объяснить риск и предложить соразмерное улучшение.

## Главная задача сервисного API

API сервисного слоя должен отвечать на вопросы:

- что делает метод;
- какие входные данные нужны;
- какие бизнес-правила применяются;
- что возвращается при успехе;
- какие ошибки значимы;
- какие side effects происходят;
- можно ли безопасно повторить вызов;
- какой context используется;
- где transaction boundary;
- кто отвечает за публикацию событий;
- какой уровень консистентности ожидается;
- что является source of truth.

Плохой API заставляет читать реализацию, чтобы понять базовые вещи.

Хороший API уже по сигнатуре и названию дает много информации.

Плохо:

```go
func (s *Service) Process(ctx context.Context, id string, mode string, sync bool) (any, error)
```

Лучше:

```go
func (s *OrderService) ConfirmOrder(ctx context.Context, cmd ConfirmOrderCommand) (ConfirmOrderResult, error)
```

Почему лучше:

- видно, что это command, а не query;
- видно, что речь про подтверждение заказа;
- входные данные можно расширять без ломки сигнатуры;
- результат типизирован;
- можно добавить idempotency key, actor id, reason;
- легче тестировать.

## Техническая база

### Application service

Application service - это слой use case-ов. Он не обязательно содержит всю бизнес-логику сам. Часто он:

1. Принимает команду.
2. Валидирует базовые входные данные.
3. Загружает нужные сущности.
4. Вызывает доменные методы или policy.
5. Сохраняет изменения.
6. Пишет outbox event.
7. Возвращает результат.

Пример:

```go
func (s *BookingService) ConfirmBooking(ctx context.Context, cmd ConfirmBookingCommand) (Booking, error) {
	booking, err := s.repo.GetBooking(ctx, cmd.BookingID)
	if err != nil {
		return Booking{}, err
	}

	if err := booking.Confirm(cmd.ActorID, s.clock.Now()); err != nil {
		return Booking{}, err
	}

	if err := s.repo.SaveBooking(ctx, booking); err != nil {
		return Booking{}, err
	}

	return booking, nil
}
```

Здесь сервис оркестрирует, а правило “можно ли подтвердить бронь” может жить внутри `booking.Confirm`.

### Domain service

Domain service - это бизнес-логика, которая неестественно помещается в одну entity.

Например:

- расчет цены с несколькими политиками;
- проверка доступности слота;
- выбор склада;
- распределение лимита;
- расчет комиссии.

Domain service обычно не должен знать про HTTP, SQL, Kafka. Он может быть чистой логикой, которую легко тестировать.

Пример:

```go
type PricingPolicy struct {
	discounts DiscountRules
	taxes     TaxRules
}

func (p PricingPolicy) Calculate(cart Cart, user User) (Money, error) {
	// pure business logic
}
```

### Repository

Repository - это абстракция доступа к хранилищу. В хорошем варианте сервис не знает, SQL там, Mongo, Redis или API другого сервиса.

Но repository не должен превращаться в свалку бизнес-методов:

Плохо:

```go
type OrderRepository interface {
	CreateOrderAndChargePaymentAndSendEmail(ctx context.Context, order Order) error
}
```

Так repository уже не repository, а скрытый service layer.

Лучше:

```go
type OrderRepository interface {
	GetOrder(ctx context.Context, id string) (Order, error)
	SaveOrder(ctx context.Context, order Order) error
}
```

Но тоже важно не скатиться в слишком низкоуровневый DAO:

```go
type OrderRepository interface {
	InsertIntoOrdersTable(ctx context.Context, row OrderRow) error
	UpdateOrdersSetStatusWhereID(ctx context.Context, id string, status string) error
}
```

Repository API должен быть в терминах домена или use case-а, а не в терминах конкретной таблицы.

### Ports and adapters

В Go часто удобно думать так:

- service layer определяет, что ему нужно;
- infrastructure layer реализует это.

Например сервису нужен порт:

```go
type PaymentClient interface {
	Charge(ctx context.Context, req ChargeRequest) (ChargeResult, error)
}
```

А конкретная реализация может быть:

```go
type StripePaymentClient struct {
	http *http.Client
}
```

На ревью важно смотреть, где объявлен interface. Часто в Go хороший паттерн:

> Interface lives where it is consumed, not where it is implemented.

То есть если `OrderService` нуждается только в `Charge`, ему не нужен огромный интерфейс всего payment SDK.

Плохо:

```go
type PaymentGateway interface {
	Charge(...)
	Refund(...)
	Capture(...)
	Void(...)
	ListTransactions(...)
	GetBalance(...)
	UpdateWebhook(...)
}
```

Если сервис использует только `Charge`, лучше:

```go
type Charger interface {
	Charge(ctx context.Context, req ChargeRequest) (ChargeResult, error)
}
```

Это снижает связность и упрощает тесты.

### Public API метода

Сигнатура метода - это контракт.

```go
func (s *Service) Do(ctx context.Context, a string, b int, c bool) error
```

Контракт слабый: непонятно, что такое `a`, что такое `b`, что включает `c`.

Часто лучше command struct:

```go
type ConfirmBookingCommand struct {
	BookingID      string
	ActorID        string
	IdempotencyKey string
	RequestedAt    time.Time
}

func (s *BookingService) ConfirmBooking(ctx context.Context, cmd ConfirmBookingCommand) (ConfirmBookingResult, error)
```

Плюсы command struct:

- можно валидировать как единый объект;
- можно расширять без длинной сигнатуры;
- названия полей документируют смысл;
- удобно тестировать;
- можно логировать operation id;
- можно отделить external DTO от internal command.

Но command struct не надо делать огромным мешком `Params` на все случаи. Он должен соответствовать конкретному use case.

### Command и Query

Очень полезное разделение:

- Query читает данные и не меняет бизнес-состояние.
- Command меняет состояние или запускает side effects.

Примеры query:

```go
GetOrder
ListUserOrders
FindAvailableRooms
GetBalance
```

Примеры command:

```go
CreateOrder
ConfirmBooking
CancelSubscription
ReserveStock
ApplyPromoCode
```

Если query публикует событие, меняет кеш, инкрементит счетчики или создает audit entry, надо понять, является ли это допустимым техническим side effect или уже нарушение контракта.

Не всякий side effect в query запрещен. Например метрика или read-through cache обычно допустимы. Но доменное событие `order.viewed`, изменение `last_seen_at`, создание записи просмотра - это уже бизнес-изменение. Его лучше делать явно.

Плохо:

```go
func (s *OrderService) ListUserOrders(ctx context.Context, userID string) ([]Order, error) {
	orders, err := s.repo.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	s.events.Publish(ctx, "orders.viewed", userID)
	return orders, nil
}
```

Лучше варианты:

```go
func (s *OrderService) ListUserOrders(ctx context.Context, query ListUserOrdersQuery) ([]Order, error)
func (s *OrderService) MarkOrdersViewed(ctx context.Context, cmd MarkOrdersViewedCommand) error
```

Или если бизнес хочет именно “получить и отметить просмотр”:

```go
func (s *OrderService) ViewUserOrders(ctx context.Context, cmd ViewUserOrdersCommand) (ViewUserOrdersResult, error)
```

Название `View` уже честнее, чем `List`.

### DTO, domain model и persistence model

Одна из частых архитектурных ошибок - использовать одну структуру для всего:

```go
type Order struct {
	ID        string `json:"id" db:"id"`
	UserID    string `json:"user_id" db:"user_id"`
	Status    string `json:"status" db:"status"`
	CreatedAt string `json:"created_at" db:"created_at"`
}
```

Эта структура одновременно:

- HTTP JSON DTO;
- DB row;
- domain entity;
- service response.

В маленьком проекте это может быть приемлемо. Но по мере роста возникают проблемы:

- наружу случайно утекают внутренние поля;
- DB-теги и JSON-теги начинают диктовать доменную модель;
- формат времени выбирается ради API, а страдает бизнес-логика;
- нельзя независимо менять таблицу и public API;
- сложно валидировать разные сценарии создания/обновления;
- появляется куча nullable-полей “на все случаи”.

Более здоровая схема:

```text
HTTP request DTO -> service command -> domain model -> repository model
domain model -> service result -> HTTP response DTO
```

Не всегда нужно заводить все пять типов. Но на ревью стоит видеть, где границы уже начинают смешиваться.

Пример:

```go
type CreateOrderRequest struct {
	Items []CreateOrderItemRequest `json:"items"`
}

type CreateOrderCommand struct {
	UserID string
	Items []CreateOrderItem
}

type Order struct {
	ID     OrderID
	UserID UserID
	Items  []OrderItem
	Status OrderStatus
	Total  Money
}

type OrderRow struct {
	ID        string
	UserID    string
	Status    string
	TotalCents int64
}
```

Смысл не в том, чтобы плодить типы ради типов. Смысл в том, чтобы разные контракты не мешали друг другу.

### Context в API сервиса

В Go почти всегда сервисные методы, которые делают I/O, должны принимать `context.Context` первым аргументом:

```go
func (s *OrderService) CreateOrder(ctx context.Context, cmd CreateOrderCommand) (Order, error)
```

Правила:

- `ctx` первым параметром;
- не хранить `context.Context` в struct;
- не передавать nil context;
- не заменять входной ctx на `context.Background()` без явной причины;
- уважать cancellation/deadline;
- для фоновых процессов иметь отдельный worker context;
- не использовать request context для работы, которая должна пережить HTTP-запрос.

Архитектурный нюанс:

Если метод запускает асинхронный процесс, надо явно определить, чей context управляет этим процессом.

Плохо:

```go
func (s *Service) Create(ctx context.Context, cmd Cmd) error {
	go s.publisher.Publish(ctx, event)
	return nil
}
```

Если HTTP-запрос завершился, `ctx` отменится, и публикация может оборваться. Если заменить на `context.Background()`, публикация переживет запрос, но появится неуправляемая фоновая работа.

Лучше:

- outbox;
- worker с lifecycle;
- очередь задач;
- явный timeout;
- наблюдаемость ошибок.

### Ошибки как часть API

Ошибка - тоже часть контракта.

Плохо:

```go
return err
```

если вызывающий слой не может понять:

- это not found или internal error;
- можно ли retry;
- это validation error или conflict;
- пользователь виноват или система;
- надо ли вернуть HTTP 400, 404, 409, 500.

В service layer полезно иметь доменные ошибки:

```go
var (
	ErrOrderNotFound      = errors.New("order not found")
	ErrOrderAlreadyPaid   = errors.New("order already paid")
	ErrInsufficientStock  = errors.New("insufficient stock")
	ErrInvalidOrderStatus = errors.New("invalid order status")
)
```

И оборачивать инфраструктурные ошибки контекстом:

```go
return fmt.Errorf("get order %s: %w", orderID, err)
```

Но не надо протаскивать SQL-ошибки прямо до HTTP handler-а как бизнес-контракт.

На ревью вопрос:

> Может ли вызывающий код принять правильное решение по этой ошибке?

### Dependency injection

Сервис должен получать зависимости явно:

```go
func NewOrderService(repo OrderRepository, payments PaymentClient, events EventBus) *OrderService
```

Плохо:

```go
func NewOrderService() *OrderService {
	return &OrderService{
		repo: NewPostgresRepo(os.Getenv("DB_DSN")),
	}
}
```

Почему плохо:

- сложно тестировать;
- сервис сам создает инфраструктуру;
- нельзя подменить mock/fake;
- нарушается separation of concerns;
- конфигурация размазывается по коду.

Но и DI можно испортить:

```go
func NewService(repo Repo, payments Payments, events Events, cache Cache, logger Logger, metrics Metrics, clock Clock, config Config, featureFlags FeatureFlags, validator Validator, notifier Notifier, search Search, audit Audit) *Service
```

Если зависимостей слишком много, возможно сервис делает слишком много.

Эвристика:

> Если у сервиса 12 зависимостей, это повод проверить его cohesion. Возможно там несколько разных use case-ов, которые надо разделить.

### Interface size

В Go маленькие интерфейсы обычно лучше больших.

Плохо:

```go
type UserService interface {
	CreateUser(...)
	UpdateUser(...)
	DeleteUser(...)
	GetUser(...)
	ListUsers(...)
	Login(...)
	Logout(...)
	SendPasswordReset(...)
	VerifyEmail(...)
	UploadAvatar(...)
}
```

Если конкретному use case нужен только `GetUser`, ему лучше зависеть от:

```go
type UserGetter interface {
	GetUser(ctx context.Context, id string) (User, error)
}
```

Но не надо фанатично делать интерфейс на каждый метод, если это не дает пользы. На ревью важен баланс:

- интерфейс должен соответствовать потребности потребителя;
- не должен включать лишние методы;
- не должен быть абстракцией “на будущее” без реального сценария;
- должен упрощать тестирование и снижать связность.

### Transaction boundary

Транзакция часто находится в service layer, потому что именно сервис знает use case целиком.

Пример:

```go
func (s *OrderService) CreateOrder(ctx context.Context, cmd CreateOrderCommand) error {
	return s.txRunner.Run(ctx, func(ctx context.Context, tx Tx) error {
		if err := s.orders.Save(ctx, tx, order); err != nil {
			return err
		}
		if err := s.outbox.Save(ctx, tx, event); err != nil {
			return err
		}
		return nil
	})
}
```

Что плохо:

- когда HTTP handler открывает транзакцию и передает ее в сервис;
- когда repository сам открывает транзакцию вокруг кусочка, не зная use case;
- когда транзакция протекает наружу в public API;
- когда внутри транзакции спрятаны внешние side effects.

В service API обычно не хочется видеть:

```go
func (s *Service) CreateOrder(ctx context.Context, tx Tx, cmd CreateOrderCommand) error
```

Если это публичный use case метод, вызывающий слой не должен думать о tx. Исключения бывают для внутренних helper-ов, но тогда лучше сделать это явно непубличным методом:

```go
func (s *Service) createOrderInTx(ctx context.Context, tx Tx, cmd CreateOrderCommand) error
```

### Validation и invariants

Валидация бывает разных уровней.

Transport validation:

- JSON корректный;
- обязательные поля пришли;
- строка не длиннее лимита;
- формат email;
- limit не отрицательный.

Service validation:

- пользователь имеет право выполнить действие;
- заказ в нужном статусе;
- сумма сходится;
- операция идемпотентна;
- бизнес-переход разрешен.

Domain invariants:

- order cannot be paid twice;
- quantity must be positive;
- booking end must be after start;
- money cannot be negative.

Плохой признак:

- в handler-е проверяется бизнес-статус заказа;
- domain entity допускает невозможное состояние;
- service принимает полусырые данные и надеется, что repository разберется;
- validation размазана по пяти местам и конфликтует.

Хороший API помогает не создать некорректное состояние.

Например вместо:

```go
order.Status = "paid"
```

лучше:

```go
if err := order.MarkPaid(paymentID, now); err != nil {
	return err
}
```

### Authorization как часть use case

Иногда authorization ошибочно держат только в HTTP middleware.

Middleware может проверить:

- пользователь аутентифицирован;
- токен валиден;
- роль есть.

Но service layer часто должен проверить object-level authorization:

- этот пользователь владеет этим заказом;
- этот менеджер может менять именно этот филиал;
- этот оператор может видеть только свои регионы;
- этот admin не может менять superadmin.

На ревью стоит смотреть:

> Где проверяется право выполнить именно это действие над именно этим объектом?

Если метод сервиса публичный для разных transport-ов, он не должен полагаться только на один HTTP handler.

### Time, clock и nondeterminism

`time.Now()` внутри сервиса - нормально для простого кода, но ухудшает тестируемость и воспроизводимость.

Для важной логики удобно внедрять clock:

```go
type Clock interface {
	Now() time.Time
}
```

Это полезно для:

- истечения сроков;
- TTL;
- дедлайнов бизнес-операций;
- тестов retry/backoff;
- state transitions.

Не надо везде фанатично добавлять clock, но если метод завязан на время, на ревью можно отметить тестируемость.

### Idempotency как часть API

Команды, которые могут быть повторены клиентом или worker-ом, должны иметь idempotency contract.

Пример:

```go
type CreatePayoutCommand struct {
	UserID         string
	Amount         Money
	IdempotencyKey string
}
```

Без этого повтор HTTP-запроса после timeout может создать две выплаты.

Idempotency особенно важна для:

- платежей;
- списаний;
- начислений;
- создания заказов;
- бронирований;
- отправки внешних заявок;
- worker processing.

На ревью вопрос:

> Если клиент не получил ответ и повторил запрос, что произойдет?

### Observability как часть архитектуры

Сервисный слой - хорошее место для бизнес-метрик и структурного логирования:

- order_created_total;
- payment_failed_total;
- booking_confirm_duration;
- outbox_backlog;
- pending_status_age;
- external_provider_errors.

Но метрики и логи не должны превращаться в бизнес-логику.

Плохо:

```go
if err != nil {
	s.metrics.Inc("error")
	return err
}
```

Слишком общее. Лучше:

```go
s.metrics.Inc("booking_confirm_failed_total", "reason", "calendar_error")
```

Для ревью:

- есть ли correlation id;
- логируются ли важные переходы;
- не логируются ли персональные данные;
- можно ли понять, на каком шаге упал use case;
- не является ли логика зависимой от успешности метрики.

## Хорошие приемы проектирования

### 1. Use-case oriented methods

Методы сервиса лучше называть по бизнес-сценариям:

```go
ConfirmBooking
CancelBooking
CreateOrder
ApplyPromoCode
ChangeDeliveryAddress
StartPasswordReset
```

Менее удачно:

```go
Update
Process
Handle
Execute
Save
Sync
```

Не всегда плохие имена запрещены. `Handle` может быть нормальным в handler-е команды. Но если в сервисе все называется `Process`, читать API тяжело.

### 2. Command/result structs

Для методов с несколькими параметрами лучше command struct.

Плохо:

```go
func (s *Service) Create(ctx context.Context, userID string, productID string, qty int, promo string, sync bool, notify bool) error
```

Лучше:

```go
type CreateOrderCommand struct {
	UserID         string
	Items          []CreateOrderItem
	PromoCode      string
	IdempotencyKey string
	NotifyUser     bool
}

type CreateOrderResult struct {
	OrderID string
	Status  string
	Total   Money
}
```

Но boolean flags часто сами по себе smell. `NotifyUser bool` может быть нормально, но `sync bool`, `force bool`, `skipValidation bool` часто говорят, что метод делает несколько разных сценариев.

### 3. Explicit side effects

Если метод публикует событие, отправляет уведомление или запускает процесс, это должно быть видно.

Плохо:

```go
func (s *UserService) GetProfile(ctx context.Context, userID string) (Profile, error)
```

а внутри:

```go
s.email.Send("welcome back")
s.repo.UpdateLastSeen(...)
```

Лучше:

```go
func (s *UserService) ViewProfile(ctx context.Context, cmd ViewProfileCommand) (Profile, error)
```

или разделить:

```go
GetProfile
MarkProfileViewed
```

### 4. Thin transport, meaningful service

HTTP handler должен быть тонким:

```go
func (h *Handler) ConfirmBooking(w http.ResponseWriter, r *http.Request) {
	cmd, err := decodeConfirmBookingRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	result, err := h.bookings.ConfirmBooking(r.Context(), cmd)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, result)
}
```

Если handler сам открывает транзакцию, дергает 3 repository, публикует событие и решает статус, service layer фактически отсутствует.

### 5. Cohesion over convenience

Сервис должен быть связным. `OrderService` может управлять заказами, но если он также занимается пользователями, email-шаблонами, warehouse sync, pricing admin и отчетами, он распухнет.

Признаки низкой связности:

- много зависимостей;
- методы не используют общие зависимости;
- тесты сервиса огромные;
- изменение одной фичи ломает unrelated методы;
- сложно назвать, за что сервис отвечает.

Разделение может быть по use case:

```go
OrderCreator
OrderCanceler
OrderReader
PaymentProcessor
```

Не обязательно делать отдельный struct на каждый метод. Но если сервис стал “комбайном”, это вариант.

### 6. Policies and domain methods

Бизнес-правила можно выносить из application service:

```go
if !user.IsPremium() && order.Total.GreaterThan(limit) && !featureFlags.Enabled("large_orders") {
	return ErrLimitExceeded
}
```

Такой код в сервисе быстро растет.

Лучше:

```go
if err := s.orderPolicy.CanCreateOrder(user, order); err != nil {
	return err
}
```

Или:

```go
if err := order.Submit(user, s.clock.Now()); err != nil {
	return err
}
```

### 7. Anti-corruption layer

Если внешний API имеет неудобную модель, не надо тащить ее в домен.

Плохо:

```go
type Order struct {
	ExternalPaymentStatus string
	ProviderPayload       map[string]any
}
```

Лучше:

```go
type PaymentStatus string

const (
	PaymentStatusPending PaymentStatus = "pending"
	PaymentStatusPaid    PaymentStatus = "paid"
	PaymentStatusFailed  PaymentStatus = "failed"
)
```

А mapping оставить в adapter:

```go
func mapProviderStatus(status string) PaymentStatus
```

Сервисный слой должен говорить на языке приложения, а не на языке каждого внешнего провайдера.

### 8. Explicit ownership of consistency

API должен показывать, кто отвечает за консистентность.

Например:

```go
func (s *Service) UpdateOrder(ctx context.Context, order Order) error
```

Непонятно:

- это полная замена или patch;
- проверяется ли версия;
- что с конкурентными обновлениями;
- можно ли перезаписать чужие изменения;
- какие поля разрешено менять.

Лучше:

```go
type ChangeDeliveryAddressCommand struct {
	OrderID         string
	ExpectedVersion int64
	NewAddress      Address
	ActorID         string
}
```

Тут видно, что есть optimistic locking и конкретный сценарий.

### 9. Separate read and write models when needed

Для сложных систем read path и write path могут отличаться.

Write model:

- проверяет инварианты;
- меняет состояние;
- пишет события;
- работает с транзакцией.

Read model:

- оптимизирована под выдачу;
- может читать denormalized view;
- может иметь pagination/filtering/sorting;
- не должна случайно менять бизнес-состояние.

Не надо вводить CQRS в каждый маленький сервис. Но если один метод пытается одновременно быть “поиском”, “изменением статуса”, “логированием просмотра” и “публикацией события”, разделение read/write может помочь.

### 10. Stable contracts for external callers

Если service API используется разными transport-ами, важно не протаскивать HTTP-детали:

Плохо:

```go
func (s *Service) CreateOrder(ctx context.Context, r *http.Request) error
```

Лучше:

```go
func (s *Service) CreateOrder(ctx context.Context, cmd CreateOrderCommand) (CreateOrderResult, error)
```

Иначе service layer становится привязанным к HTTP и его сложнее использовать из:

- gRPC;
- worker-а;
- CLI;
- cron job;
- tests.

## Типовые ошибки

### Ошибка 1. Метод называется как query, но имеет business side effects

Пример:

```go
func (s *OrderService) ListUserOrders(ctx context.Context, userID string) ([]Order, error) {
	orders, err := s.repo.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	for _, order := range orders {
		_ = s.events.Publish(ctx, "order.viewed", order.ID)
	}

	return orders, nil
}
```

Что не так:

- `List` звучит как read-only;
- внутри публикуется доменное событие;
- ошибки события игнорируются;
- вызывающий код не ожидает side effects;
- повторный вызов может создать повторные события;
- тесты query должны теперь учитывать broker.

Как лучше:

- разделить `ListUserOrders` и `MarkOrdersViewed`;
- переименовать в `ViewUserOrders`;
- сделать event через outbox, если событие критично;
- явно описать идемпотентность просмотра.

### Ошибка 2. Service API слишком общий

Пример:

```go
func (s *Service) Process(ctx context.Context, req Request) (Response, error)
```

И внутри:

```go
switch req.Type {
case "create":
case "cancel":
case "refund":
case "sync":
}
```

Проблемы:

- непонятный контракт;
- много ветвлений;
- разные validation rules;
- разные dependencies;
- разные ошибки;
- сложно тестировать;
- легко сломать один сценарий при изменении другого.

Лучше отдельные use case methods:

```go
CreateOrder
CancelOrder
RefundOrder
SyncOrder
```

### Ошибка 3. Long parameter list

Пример:

```go
func (s *Service) CreateBooking(ctx context.Context, userID, roomID string, startsAt, endsAt time.Time, notify bool, source string, comment string) error
```

Проблемы:

- легко перепутать параметры одного типа;
- сложно расширять;
- не видно группировку данных;
- boolean flags часто меняют сценарий;
- вызов плохо читается.

Лучше:

```go
type CreateBookingCommand struct {
	UserID    string
	RoomID    string
	Interval  TimeInterval
	Source    BookingSource
	Comment   string
}
```

### Ошибка 4. Boolean flags управляют разными сценариями

Пример:

```go
func (s *Service) UpdateUser(ctx context.Context, user User, notify bool, skipValidation bool, force bool) error
```

Проблемы:

- один метод делает много режимов;
- комбинации flags плохо тестируются;
- `force=true` часто ломает инварианты;
- непонятно, какие flags можно сочетать;
- метод трудно читать на call site.

Лучше:

- разные методы для разных use case-ов;
- explicit command;
- policy object;
- enum с понятным смыслом, если режим действительно один из нескольких.

Например:

```go
UpdateUserProfile
AdminForceUpdateUser
UpdateUserEmail
```

### Ошибка 5. Утечка transport details в service layer

Плохо:

```go
func (s *Service) CreateOrder(w http.ResponseWriter, r *http.Request)
```

или:

```go
func (s *Service) CreateOrder(ctx context.Context, headers http.Header, body []byte) error
```

Сервис теперь нельзя нормально вызвать из worker-а или теста без HTTP-обвязки.

Лучше decode/encode оставить transport layer-у, а сервису дать typed command.

### Ошибка 6. Утечка persistence details наружу

Плохо:

```go
func (s *Service) GetOrder(ctx context.Context, id string) (OrderRow, error)
```

`OrderRow` может содержать:

- DB-specific nullable types;
- internal columns;
- технические статусы;
- поля для миграций;
- storage tags.

Лучше возвращать service result/domain projection:

```go
func (s *Service) GetOrder(ctx context.Context, id string) (OrderDetails, error)
```

### Ошибка 7. Repository прячет бизнес-сценарий

Плохо:

```go
func (r *Repo) ConfirmBooking(ctx context.Context, id string) error {
	tx := r.db.Begin()
	...
	r.calendar.CreateEvent(...)
	r.events.Publish(...)
}
```

Repository внезапно:

- открывает транзакцию;
- вызывает calendar;
- публикует событие;
- знает бизнес-сценарий.

Это усложняет тестирование и ломает layering. Сервисный слой должен оркестрировать use case, repository - сохранять и загружать данные.

### Ошибка 8. Сервис напрямую знает слишком много инфраструктуры

Пример:

```go
type Service struct {
	db          *sql.DB
	redis       *redis.Client
	kafka       *kafka.Writer
	httpClient   *http.Client
	s3          *s3.Client
}
```

Иногда это допустимо в маленьком приложении, но часто лучше иметь порты:

```go
type OrderRepository interface { ... }
type EventPublisher interface { ... }
type FileStorage interface { ... }
```

Проблемы прямых клиентов:

- бизнес-код смешивается с инфраструктурным;
- сложнее мокать;
- сложнее менять provider;
- сервис знает детали retry/serialization/protocol.

### Ошибка 9. Слишком большой интерфейс зависимости

Плохо:

```go
type UserClient interface {
	Create(...)
	Update(...)
	Delete(...)
	Get(...)
	Search(...)
	Ban(...)
	Unban(...)
	ResetPassword(...)
}
```

Если сервису нужен только `Get`, интерфейс завышен. Mock-и становятся огромными, а связность растет.

Лучше интерфейс под потребность.

### Ошибка 10. Циклические зависимости сервисов

Плохо:

```text
OrderService -> PaymentService -> UserService -> OrderService
```

Последствия:

- сложно инициализировать;
- трудно тестировать;
- непонятные side effects;
- риск рекурсивных сценариев;
- невозможность отделить модули.

Решения:

- выделить общий domain service/policy;
- заменить прямой вызов событием;
- вынести shared dependency;
- пересмотреть границы bounded context.

### Ошибка 11. God service

Признаки:

- struct на 1000+ строк;
- десятки методов;
- 10+ зависимостей;
- unrelated use cases;
- огромные тесты;
- приватные helper-ы вызываются хаотично;
- изменения постоянно конфликтуют.

Что делать:

- разделить read/write;
- выделить use case services;
- вынести policy/domain logic;
- выделить adapters;
- уменьшить public API;
- убрать неиспользуемые зависимости.

### Ошибка 12. Неявная идемпотентность

Плохо:

```go
func (s *PayoutService) CreatePayout(ctx context.Context, userID string, amount int64) error
```

Что если клиент повторит запрос после timeout?

Может создаться две выплаты. API не содержит operation id или idempotency key.

Лучше:

```go
type CreatePayoutCommand struct {
	UserID         string
	Amount         Money
	IdempotencyKey string
}
```

### Ошибка 13. Неопределенный partial success

Пример:

```go
func (s *Service) SyncUser(ctx context.Context, userID string) error {
	s.crm.Update(...)
	s.email.Send(...)
	s.repo.MarkSynced(...)
	return nil
}
```

Если email упал после успешного CRM update, что значит ошибка метода? Надо ли retry? Что будет при retry? Будет ли второй email?

API должен иметь стратегию:

- all-or-nothing в рамках одного ресурса;
- saga;
- outbox;
- best-effort side effect;
- partial result;
- retryable status.

### Ошибка 14. Ошибки слишком инфраструктурные или слишком общие

Плохо:

```go
return pq.ErrNoRows
```

или:

```go
return errors.New("failed")
```

Первое протаскивает DB detail наружу. Второе не дает принять решение.

Лучше:

```go
return ErrBookingNotFound
```

или:

```go
return fmt.Errorf("confirm booking: %w", ErrInvalidBookingStatus)
```

### Ошибка 15. Валидация размазана и противоречива

Пример:

- handler проверяет `amount > 0`;
- service иногда тоже проверяет;
- repository молча приводит отрицательное значение к нулю;
- domain entity допускает отрицательную сумму.

Нужен понятный ownership:

- формат и обязательность - transport;
- бизнес-инварианты - service/domain;
- storage constraints - база как последняя линия защиты.

### Ошибка 16. Неясное владение временем

Плохо:

```go
if time.Now().After(order.ExpiresAt) {
	...
}
```

В тестах сложно проверить границы. В разных частях метода может быть разное `now`.

Лучше:

```go
now := s.clock.Now()
if order.IsExpired(now) {
	...
}
```

### Ошибка 17. Сервис мутирует входной объект неожиданно

Пример:

```go
func (s *Service) CreateOrder(ctx context.Context, order *Order) error {
	order.ID = newID()
	order.Status = "new"
	return s.repo.Save(ctx, *order)
}
```

Если caller не ожидает мутацию, это может привести к багам. В Go мутация указателя может быть нормальной, но контракт должен быть понятен.

Часто лучше:

```go
func (s *Service) CreateOrder(ctx context.Context, cmd CreateOrderCommand) (Order, error)
```

### Ошибка 18. Возврат внутренних slices/maps без защиты

Если service возвращает объект с map/slice, который является внутренним состоянием кеша, caller может его изменить.

Пример:

```go
func (s *Service) GetRules() []Rule {
	return s.rules
}
```

Лучше вернуть копию, если это shared internal state.

### Ошибка 19. Сервис запускает goroutine без lifecycle

Плохо:

```go
func (s *Service) Create(ctx context.Context, cmd Cmd) error {
	go s.doAfterCreate(ctx, cmd)
	return nil
}
```

Проблемы:

- ошибка теряется;
- context может отмениться;
- нет shutdown;
- нет backpressure;
- тесты нестабильны;
- retry неочевиден.

Лучше:

- outbox;
- очередь;
- worker pool с lifecycle;
- явный async API.

### Ошибка 20. Cache становится частью бизнес-контракта

Кеш должен ускорять, но не становиться source of truth, если это не отдельное осознанное хранилище.

Плохо:

```go
if cached, ok := cache.Get(id); ok {
	return cached, nil
}
// если в базе ошибка, возвращаем старый кеш как будто все нормально
```

Иногда stale cache допустим. Но API должен понимать:

- можно ли вернуть stale data;
- сколько она может жить;
- кто инвалидирует;
- что происходит при write;
- что является source of truth.

## Как избегать проблем

### Делать API честным

Название метода должно соответствовать последствиям.

- `Get` не должен подтверждать, отправлять, списывать.
- `List` не должен менять бизнес-состояние без явного контракта.
- `Create` должен говорить, что именно создается.
- `Confirm` должен проверять статус и переход.
- `Sync` должен иметь понятный источник и направление.

Если метод делает side effect, это должно быть понятно по названию, command-типу или документации.

### Разделять сценарии

Если метод содержит много `if mode`, `if force`, `if sync`, `if notify`, возможно там несколько use case-ов.

Разделение методов часто упрощает:

- validation;
- errors;
- authorization;
- tests;
- observability;
- transaction boundaries.

### Держать зависимости узкими

Сервис должен зависеть от того, что ему нужно, а не от всего мира.

Хороший вопрос:

> Можно ли протестировать этот use case простыми fake-зависимостями?

Если для теста `ConfirmBooking` надо поднимать Redis, Kafka, Postgres, HTTP server и feature flags, возможно границы выбраны плохо.

### Явно моделировать бизнес-операции

Для важных команд полезны:

- command struct;
- result struct;
- operation id;
- idempotency key;
- actor id;
- expected version;
- reason/comment;
- source/channel.

Это делает API чуть более многословным, зато надежным.

### Не смешивать внешние DTO и domain

В handler-е:

```go
req := CreateOrderRequest{}
cmd := CreateOrderCommand{
	UserID: userIDFromAuth,
	Items: mapItems(req.Items),
}
result, err := service.CreateOrder(ctx, cmd)
```

Так service не зависит от JSON, а domain не зависит от transport.

### Выносить business rules в domain/policy

Если service становится набором огромных `if`, подумать:

- это метод entity?
- это policy?
- это state machine?
- это domain service?
- это authorization rule?

Сервис должен быть читаемой orchestration, а не стеной условий на 300 строк.

### Явно проектировать async

Если операция асинхронная, API должен это показывать.

Например:

```go
func (s *ReportService) StartReportGeneration(ctx context.Context, cmd StartReportCommand) (ReportJob, error)
func (s *ReportService) GetReportJob(ctx context.Context, id string) (ReportJob, error)
```

Плохо, когда `GenerateReport` вроде синхронный, но внутри запускает goroutine и возвращает nil.

### Делать ошибки пригодными для caller-а

Сервис должен возвращать ошибки, по которым transport может принять решение:

- 400 validation;
- 401/403 authorization;
- 404 not found;
- 409 conflict;
- 422 business rule violation;
- 429 rate limit;
- 500 internal;
- 503 external dependency unavailable.

Не обязательно привязывать сервис к HTTP-кодам. Но семантика ошибки должна быть ясной.

## Как диагностировать архитектурные проблемы на ревью

### Шаг 1. Сначала читать сигнатуры

До реализации посмотри на public API:

```go
func (s *Service) Method(ctx context.Context, ...)
```

Спроси:

- понятно ли из названия, что делает метод;
- это command или query;
- нет ли слишком общего имени;
- нет ли длинного списка параметров;
- нужны ли command/result types;
- возвращается ли осмысленный тип;
- можно ли расширить API без ломки вызовов.

### Шаг 2. Найти side effects

Внутри метода ищи:

- repository writes;
- events.Publish;
- email.Send;
- payments.Charge;
- cache.Set/Delete;
- goroutine;
- queue.Enqueue;
- external HTTP/gRPC;
- audit log;
- metrics/logging.

Потом сравни с названием метода. Если side effects неожиданны, это замечание.

### Шаг 3. Нарисовать зависимости

Прямо мысленно:

```text
OrderService
  -> OrderRepository
  -> PaymentClient
  -> EventPublisher
  -> Cache
  -> UserService
  -> PricingService
```

Вопросы:

- все ли зависимости нужны именно этому сервису;
- нет ли циклов;
- нет ли слишком широких интерфейсов;
- не зависит ли service от concrete infra clients;
- легко ли заменить dependency в тесте.

### Шаг 4. Проследить data flow

Откуда приходит объект и куда уходит:

```text
HTTP request -> command -> domain -> repository -> row
row -> domain/projection -> result -> HTTP response
```

Ищи:

- JSON DTO протек в domain;
- DB row возвращается наружу;
- domain entity содержит provider payload;
- internal fields уходят клиенту;
- service мутирует входной pointer.

### Шаг 5. Проверить ownership бизнес-правил

Спроси:

- где проверяется статус;
- где проверяется право доступа;
- где проверяется лимит;
- где считается сумма;
- где запрещается невозможный переход;
- может ли другой caller обойти правило.

Если правило только в HTTP handler-е, а сервис можно вызвать из worker-а, это риск.

### Шаг 6. Проверить failure contract

Для каждого метода:

- что значит ошибка;
- могла ли часть операции уже выполниться;
- можно ли retry;
- есть ли idempotency;
- есть ли partial result;
- есть ли компенсация;
- что должен делать caller.

Если это непонятно, API слабый.

### Шаг 7. Проверить testability

Спроси:

- можно ли протестировать метод без настоящей базы;
- можно ли подменить clock;
- можно ли проверить external failure;
- можно ли проверить authorization;
- не нужно ли мокать 15 методов огромного интерфейса;
- не запускаются ли скрытые goroutine, из-за которых тесты flaky.

### Шаг 8. Проверить naming

Имена - это архитектура в миниатюре.

Плохие признаки:

- `Process`;
- `Handle`;
- `Do`;
- `Run`;
- `Sync` без направления;
- `Update` для разных сценариев;
- `Manager`;
- `Helper`;
- `Util`;
- `Data`;
- `Info`;
- `Payload` в domain.

Не все эти слова запрещены. Но они часто требуют уточнения.

Хорошие имена:

- `ConfirmBooking`;
- `CancelBooking`;
- `GetBookingDetails`;
- `FindAvailableRooms`;
- `CreatePaymentAttempt`;
- `MarkPaymentPaid`;
- `PublishOutboxEvents`;
- `ReserveStock`.

### Шаг 9. Проверить boundaries

Спроси:

- transport не делает business logic?
- service не делает SQL/JSON/protocol details?
- repository не вызывает внешние сервисы?
- domain не зависит от infrastructure?
- async не спрятан в случайной goroutine?
- transaction boundary находится в правильном слое?

### Шаг 10. Проверить эволюцию

Хороший ревьюер думает на пару изменений вперед:

- что если появится второй payment provider;
- что если будет gRPC рядом с HTTP;
- что если нужен retry;
- что если нужно добавить audit;
- что если появится idempotency;
- что если userID надо брать из actor-а;
- что если нужно запретить переход статуса.

Не надо заранее строить космическую архитектуру. Но если текущее API явно не выдержит ближайшее очевидное требование, стоит сказать.

## Как говорить на собеседовании

Про query с side effects:

> Метод выглядит как read-only query, но внутри меняет внешнее состояние. Я бы сделал side effect явным: либо разделил чтение и mark viewed, либо переименовал метод в бизнес-действие, например `ViewOrders`.

Про service API:

> Сигнатура с большим количеством примитивов плохо выражает контракт. Я бы ввел command struct, чтобы поля имели имена, можно было добавить idempotency key/actor id и централизованно валидировать вход.

Про смешение слоев:

> Здесь service layer знает слишком много деталей transport/infrastructure. Я бы оставил HTTP decoding в handler-е, а сервису передавал typed command. Для внешних систем завел бы узкие интерфейсы-порты.

Про repository:

> Repository должен отвечать за загрузку и сохранение данных. Если внутри repository публикуются события или вызывается payment API, бизнес-сценарий становится спрятанным и его сложнее ревьюить.

Про интерфейсы:

> Интерфейс шире, чем нужно этому сервису. В Go лучше объявлять маленький интерфейс на стороне потребителя, чтобы снизить связность и упростить тесты.

Про god service:

> У сервиса слишком много зависимостей и unrelated методов. Это сигнал низкой связности. Я бы посмотрел, можно ли разделить read/write или выделить use case services/policies.

Про ошибки:

> Ошибки являются частью API. Сейчас наружу возвращается либо слишком общая ошибка, либо инфраструктурная. Вызывающий слой не сможет корректно отличить not found, validation, conflict и internal failure.

Про идемпотентность:

> Для команды, которая создает внешний эффект, в API нет idempotency key. Если клиент повторит запрос после timeout, можно выполнить операцию дважды.

Про бизнес-правила:

> Проверка статуса/прав доступа должна быть в service/domain layer, а не только в HTTP handler-е, иначе другой caller сможет обойти правило.

## Пример плохого сервиса

```go
type UserService struct {
	db       *sql.DB
	cache    Cache
	events   EventPublisher
	email    EmailClient
	payments PaymentClient
}

func (s *UserService) GetUser(ctx context.Context, userID string, includeOrders bool, notify bool) (UserResponse, error) {
	row := s.db.QueryRowContext(ctx, "select id, email, status from users where id = $1", userID)

	var user UserResponse
	if err := row.Scan(&user.ID, &user.Email, &user.Status); err != nil {
		return UserResponse{}, err
	}

	if includeOrders {
		orders, err := s.loadOrders(ctx, userID)
		if err != nil {
			return UserResponse{}, err
		}
		user.Orders = orders
	}

	if notify {
		go s.email.Send(ctx, user.Email, "profile viewed")
	}

	_ = s.events.Publish(ctx, "user.viewed", userID)
	_ = s.cache.Set(ctx, "user:"+userID, user, time.Minute)

	return user, nil
}
```

Проблемы:

- `GetUser` выглядит как query, но отправляет email и событие;
- boolean flags меняют сценарий;
- service напрямую использует SQL;
- возвращается response-модель прямо из service;
- goroutine без lifecycle;
- ошибки email/event/cache игнорируются;
- context запроса уходит в goroutine;
- нет явного контракта, является ли просмотр бизнес-событием;
- метод смешивает чтение профиля, чтение заказов, уведомления, кеш и аналитику.

Более здоровое направление:

```go
type GetUserProfileQuery struct {
	UserID string
}

type ViewUserProfileCommand struct {
	UserID string
	ActorID string
}

func (s *UserReader) GetUserProfile(ctx context.Context, query GetUserProfileQuery) (UserProfile, error)
func (s *UserActivityService) ViewUserProfile(ctx context.Context, cmd ViewUserProfileCommand) (UserProfile, error)
```

Если просмотр должен создавать событие, `ViewUserProfile` может явно:

- проверить права;
- получить профиль;
- сохранить факт просмотра;
- записать outbox event;
- вернуть профиль.

## Пример хорошего направления

```go
type ConfirmBookingCommand struct {
	BookingID      string
	ActorID        string
	IdempotencyKey string
	ExpectedVersion int64
}

type ConfirmBookingResult struct {
	BookingID string
	Status    BookingStatus
	Version   int64
}

func (s *BookingService) ConfirmBooking(ctx context.Context, cmd ConfirmBookingCommand) (ConfirmBookingResult, error) {
	if err := cmd.Validate(); err != nil {
		return ConfirmBookingResult{}, err
	}

	var result ConfirmBookingResult

	err := s.tx.Run(ctx, func(ctx context.Context, tx Tx) error {
		booking, err := s.bookings.GetForUpdate(ctx, tx, cmd.BookingID)
		if err != nil {
			return err
		}

		if err := s.authz.CanConfirmBooking(ctx, cmd.ActorID, booking); err != nil {
			return err
		}

		if err := booking.Confirm(cmd.ActorID, s.clock.Now(), cmd.ExpectedVersion); err != nil {
			return err
		}

		if err := s.bookings.Save(ctx, tx, booking); err != nil {
			return err
		}

		if err := s.outbox.Save(ctx, tx, BookingConfirmedEvent(booking)); err != nil {
			return err
		}

		result = ConfirmBookingResult{
			BookingID: booking.ID.String(),
			Status:    booking.Status,
			Version:   booking.Version,
		}
		return nil
	})
	if err != nil {
		return ConfirmBookingResult{}, err
	}

	return result, nil
}
```

Почему направление лучше:

- method name выражает use case;
- command содержит actor/idempotency/version;
- transaction boundary внутри service;
- authorization на уровне use case;
- state transition внутри domain;
- event пишется через outbox;
- result типизирован;
- нет transport details;
- dependencies выражены через порты;
- код можно тестировать fake-зависимостями.

Это не “единственно правильный” шаблон. Но он показывает, какие вопросы закрывает хороший service API.

## Мини-чеклист для ревью

Можно держать перед глазами:

- Название метода честно описывает действие?
- Это command или query?
- Есть ли неожиданные side effects?
- Не слишком ли общий метод (`Process`, `Update`, `Sync`)?
- Нет ли длинного списка параметров?
- Нужен ли command/result struct?
- Не протекли ли HTTP/JSON/SQL детали в service API?
- Не возвращает ли service DB row или ORM model наружу?
- Где проверяются business invariants?
- Где проверяется authorization на объект?
- Кто владеет transaction boundary?
- Нет ли внешних вызовов, спрятанных в repository?
- Не слишком ли широкие interfaces?
- Нет ли циклических зависимостей сервисов?
- Не слишком ли много dependencies у сервиса?
- Можно ли метод протестировать без реальной инфраструктуры?
- Что означает ошибка метода?
- Можно ли безопасно retry?
- Нужен ли idempotency key?
- Не запускаются ли goroutine без lifecycle?
- Явно ли описана async-часть?
- Не стал ли cache source of truth случайно?
- Есть ли observability для важных бизнес-переходов?

## Вопросы автору кода

Полезные вопросы на ревью:

- Этот метод должен быть read-only или он намеренно меняет состояние?
- Почему side effect находится внутри query-метода?
- Что произойдет при повторном вызове после timeout?
- Как caller должен отличить validation error от conflict/not found/internal?
- Где проверяется право пользователя выполнить это действие?
- Почему сервис принимает HTTP request/DB row/concrete client?
- Можно ли использовать этот service из worker-а или gRPC?
- Почему repository публикует событие?
- Что является source of truth: база или кеш?
- Есть ли expected version/optimistic lock для обновления?
- Все ли dependencies сервиса нужны каждому use case?
- Можно ли выделить отдельный policy/domain service?
- Как протестировать этот метод без настоящего брокера/почты/платежки?
- Если событие не отправилось, операция считается успешной?
- Если cache update упал, надо ли фейлить весь use case?

## Короткое резюме

Сервисный слой - это место, где приложение говорит на языке use case-ов. Хороший сервисный API делает бизнес-сценарий понятным и честным:

- command/query разделены;
- side effects явные;
- зависимости узкие;
- transport и infrastructure детали не протекают в контракт;
- ошибки имеют смысл для caller-а;
- transaction boundary находится в правильном месте;
- business rules нельзя обойти другим caller-ом;
- retry/idempotency продуманы для команд;
- async и события не спрятаны в случайных goroutine;
- код можно тестировать без тяжелой инфраструктуры.

На собеседовании сильный ответ по этой теме звучит не как “мне не нравится архитектура”, а как:

> Этот API скрывает важные последствия. По названию это query, но внутри есть business side effect. Вызывающий код не понимает, можно ли метод безопасно повторить, какие ошибки ожидать и что будет при частичном failure. Я бы сделал use case явным: разделил command/query, ввел command struct с actor/idempotency, вынес внешние side effects в явный механизм и сузил зависимости сервиса.
