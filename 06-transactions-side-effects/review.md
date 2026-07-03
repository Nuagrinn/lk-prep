# Транзакции и внешние side effects

Эта тема про один из самых частых и самых неприятных классов ошибок в сервисном слое: код пытается сделать атомарной операцию, которая на самом деле атомарной не является.

Главная мысль:

> Транзакция в базе данных откатывает только изменения в базе данных. Она не откатывает внешний мир.

Если внутри DB-транзакции вызвать платежный сервис, отправить письмо, опубликовать событие в брокер, дернуть другой HTTP-сервис, записать файл или отправить push-уведомление, то `Rollback()` не вернет этот side effect назад. Деньги могут быть списаны, письмо может уйти, событие может быть прочитано другим сервисом, файл может остаться на диске, а транзакция в базе при этом откатится.

На code review это надо замечать особенно внимательно, потому что такие баги редко выглядят как простая синтаксическая ошибка. Код может быть красивым, аккуратным, с `defer tx.Rollback()`, с контекстами и интерфейсами, но бизнес-состояние системы все равно будет ломаться.

## Простое введение

Представь операцию создания заказа:

1. Создали заказ в базе.
2. Списали деньги через платежный сервис.
3. Поставили заказу статус `paid`.
4. Закоммитили транзакцию.
5. Отправили событие `order.created`.

На первый взгляд хочется обернуть это в одну транзакцию:

```go
tx, err := repo.Begin(ctx)
if err != nil {
	return err
}
defer tx.Rollback()

if err := repo.SaveOrder(ctx, tx, order); err != nil {
	return err
}

if err := payments.Charge(ctx, order.ID, order.Total); err != nil {
	return err
}

if err := repo.UpdateStatus(ctx, tx, order.ID, "paid"); err != nil {
	return err
}

return tx.Commit()
```

Проблема в строке с `payments.Charge`. Платежная система не участвует в транзакции базы данных. Если `Charge` успешно списал деньги, а потом `UpdateStatus` или `Commit` упали, база может откатиться, но деньги уже списаны.

Получается неприятное состояние:

- в базе заказа нет или он не `paid`;
- у пользователя деньги списаны;
- система не понимает, надо ли повторить операцию;
- повтор операции может списать деньги второй раз, если нет идемпотентности.

То же самое с событиями:

```go
tx, _ := repo.Begin(ctx)
defer tx.Rollback()

repo.SaveOrder(ctx, tx, order)
events.Publish(ctx, "order.created", order.ID)
tx.Commit()
```

Если событие опубликовано до `Commit`, другой сервис может получить `order.created`, пойти читать заказ из базы и не найти его, потому что транзакция еще не закоммичена или потом вообще откатилась.

Если событие опубликовано после `Commit`, возникает другая проблема:

```go
repo.SaveOrder(ctx, tx, order)
tx.Commit()
events.Publish(ctx, "order.created", order.ID)
```

Если процесс упал между `Commit` и `Publish`, заказ в базе уже есть, но событие никогда не уйдет.

Вот это и есть центр темы: у нас часто есть два действия, каждое из которых важно, но обычная DB-транзакция гарантирует атомарность только одного ресурса.

## Что такое side effect

Side effect - это любое действие, которое меняет состояние не только внутри локальных переменных текущей функции.

В контексте сервисного слоя чаще всего side effects такие:

- запись в базу данных;
- вызов платежного сервиса;
- отправка HTTP/gRPC-запроса в другой сервис;
- публикация сообщения в Kafka/RabbitMQ/NATS/SQS;
- отправка email, SMS, push;
- запись файла в S3 или локальное хранилище;
- изменение кеша Redis/Memcached/in-memory cache;
- инвалидация кеша;
- запись audit log;
- создание задачи в очереди;
- вызов внешней CRM/ERP/антифрода;
- изменение счетчиков, квот, лимитов;
- отправка метрик.

Не все side effects одинаково опасны. Метрика обычно best-effort: если она потерялась, бизнес-состояние не сломалось. А платеж, списание бонусов, публикация доменного события или изменение статуса заказа - это уже критичная часть бизнес-процесса.

На ревью полезно мысленно делить side effects на три группы:

1. Критичные для бизнес-состояния: платеж, списание баланса, изменение статуса, резерв товара.
2. Важные, но асинхронно восстанавливаемые: событие в брокер, индексирование в search, пересчет витрины.
3. Best-effort: метрики, часть логов, неважные аналитические события.

Чем критичнее side effect, тем строже нужны гарантии: идемпотентность, устойчивые ретраи, outbox, audit trail, статусная модель.

## Техническая база

### ACID

DB-транзакция обычно обещает ACID-свойства.

Atomicity: внутри транзакции изменения либо применяются вместе, либо не применяются. Но это относится к изменениям в этой базе данных, а не к вызовам наружу.

Consistency: транзакция переводит базу из одного валидного состояния в другое, если код и ограничения базы корректны.

Isolation: параллельные транзакции не должны ломать друг друга сильнее, чем разрешено уровнем изоляции.

Durability: после успешного `Commit` изменения должны сохраниться даже после сбоя.

Важно: ACID не говорит, что платежный сервис, брокер и база магически становятся одной общей транзакцией.

### Begin, Commit, Rollback

Типовой жизненный цикл:

```go
tx, err := db.BeginTx(ctx, nil)
if err != nil {
	return err
}

committed := false
defer func() {
	if !committed {
		_ = tx.Rollback()
	}
}()

if err := doWork(ctx, tx); err != nil {
	return err
}

if err := tx.Commit(); err != nil {
	return err
}
committed = true

return nil
```

Зачем `committed`:

- после успешного `Commit` `Rollback` уже не нужен;
- многие драйверы вернут ошибку при rollback после commit, и это может шуметь;
- такой код явно показывает границу: до commit можно откатиться, после commit уже нельзя.

На практике часто пишут:

```go
defer tx.Rollback()
```

Это не всегда катастрофа, потому что rollback после commit обычно просто вернет ошибку, которую игнорируют. Но на ревью можно улучшить: rollback должен быть fallback только до успешного commit.

### Транзакция держит ресурсы

Пока транзакция открыта, она может держать:

- соединение с базой из пула;
- row locks;
- gap locks или predicate locks, зависит от базы и уровня изоляции;
- MVCC snapshots;
- версии строк, которые мешают vacuum/cleanup;
- память и внутренние структуры базы.

Если внутри транзакции сделать медленный HTTP-запрос наружу, транзакция будет висеть все это время. Это плохо даже если логически все завершилось успешно.

Последствия:

- другие запросы ждут lock;
- растет latency;
- истощается connection pool;
- увеличивается риск deadlock;
- база держит старые версии строк;
- под нагрузкой система начинает деградировать каскадно.

Хорошая эвристика:

> В транзакции должны быть только короткие операции с базой. Сетевые вызовы, ожидания, тяжелые вычисления и внешние side effects лучше выносить за ее пределы.

### Уровни изоляции не решают проблему внешнего мира

Можно поставить `SERIALIZABLE`, можно использовать `SELECT FOR UPDATE`, можно взять advisory lock. Это помогает против конкурирующих транзакций в базе. Но это не делает внешний payment API частью транзакции.

Уровень изоляции отвечает на вопросы вроде:

- увидит ли транзакция чужие незакоммиченные данные;
- может ли случиться non-repeatable read;
- возможны ли phantom reads;
- как база разрешает конкурентные обновления.

Он не отвечает на вопрос:

- откатится ли HTTP-запрос;
- отменится ли письмо;
- исчезнет ли опубликованное сообщение из Kafka;
- вернутся ли деньги после rollback.

### Commit error - отдельная боль

Ошибка на `Commit` не всегда означает, что данные точно не записались. Иногда клиент потерял соединение в момент commit, и приложение не знает, успела база применить изменения или нет.

Это называется ambiguous commit result.

Пример:

```go
if err := tx.Commit(); err != nil {
	return err
}
```

Если commit вернул ошибку из-за network timeout, есть два варианта:

- база не закоммитила;
- база закоммитила, но клиент не получил ответ.

Поэтому опасно после такой ошибки просто повторить весь бизнес-оператор, особенно если внутри уже был внешний side effect. Надо проектировать операции идемпотентными: использовать уникальные ключи, request id, idempotency key, business operation id.

### Exactly once почти всегда иллюзия

В распределенных системах обычно реально строят не exactly once, а комбинацию:

- at-least-once delivery;
- idempotent processing;
- deduplication;
- durable state;
- retry with backoff;
- reconciliation.

То есть сообщение может прийти два раза, запрос может повториться, worker может упасть после выполнения действия, но до отметки “готово”. Система должна уметь безопасно пережить повтор.

Для ревью это важная мысль:

> Я бы не полагался на то, что side effect выполнится ровно один раз. Лучше заложить at-least-once и сделать обработчик идемпотентным.

## Базовые паттерны

### 1. Transactional outbox

Transactional outbox - один из главных паттернов для связки “изменить базу и опубликовать событие”.

Идея:

1. В одной DB-транзакции меняем бизнес-таблицы.
2. В этой же транзакции пишем запись в таблицу `outbox`.
3. Коммитим транзакцию.
4. Отдельный worker читает `outbox` и публикует события в брокер.
5. После успешной публикации помечает запись как отправленную.

Схематично:

```go
tx, err := repo.Begin(ctx)
if err != nil {
	return err
}

if err := repo.SaveOrder(ctx, tx, order); err != nil {
	_ = tx.Rollback()
	return err
}

event := OutboxEvent{
	ID:        newID(),
	Topic:     "order.created",
	Payload:   order.ID,
	CreatedAt: time.Now(),
}
if err := repo.SaveOutboxEvent(ctx, tx, event); err != nil {
	_ = tx.Rollback()
	return err
}

if err := tx.Commit(); err != nil {
	return err
}

return nil
```

Почему это хорошо:

- заказ и “намерение отправить событие” коммитятся атомарно;
- если процесс упал после commit, запись в outbox осталась;
- worker сможет повторить публикацию;
- событие не появится без заказа, потому что outbox пишется в той же транзакции;
- можно наблюдать backlog и ошибки отправки.

Важно: outbox обычно дает at-least-once publication. Значит потребители должны быть идемпотентными. Одно и то же событие может быть опубликовано повторно, например если worker отправил сообщение, но упал до отметки `published_at`.

Типичная таблица:

```sql
CREATE TABLE outbox_events (
    id            UUID PRIMARY KEY,
    aggregate_id  TEXT NOT NULL,
    event_type    TEXT NOT NULL,
    payload       JSONB NOT NULL,
    created_at    TIMESTAMP NOT NULL,
    published_at  TIMESTAMP NULL,
    attempts      INT NOT NULL DEFAULT 0,
    last_error    TEXT NULL
);
```

На ревью стоит смотреть:

- пишется ли outbox в той же транзакции, что и бизнес-изменения;
- есть ли уникальный `id` события;
- есть ли ретраи;
- не потеряется ли событие при падении процесса;
- идемпотентны ли consumer-ы;
- есть ли мониторинг stuck outbox.

### 2. Inbox / deduplication на стороне consumer-а

Если сообщения доставляются at-least-once, consumer должен уметь переживать дубликаты.

Паттерн inbox:

1. Consumer получает сообщение с `message_id`.
2. В транзакции пытается вставить `message_id` в таблицу `inbox`.
3. Если такой `message_id` уже есть, сообщение уже обработано, можно ack.
4. Если вставка прошла, выполняет бизнес-изменения.
5. Коммитит.

Примерно:

```go
tx, err := repo.Begin(ctx)
if err != nil {
	return err
}

inserted, err := repo.TrySaveInboxMessage(ctx, tx, msg.ID)
if err != nil {
	_ = tx.Rollback()
	return err
}
if !inserted {
	_ = tx.Rollback()
	return nil
}

if err := repo.ApplyEvent(ctx, tx, msg.Payload); err != nil {
	_ = tx.Rollback()
	return err
}

return tx.Commit()
```

Главная идея: уникальный ключ на `message_id` делает повтор безопасным.

### 3. Idempotency key

Idempotency key - это ключ операции, по которому повторный запрос не выполняет действие второй раз, а возвращает результат предыдущей попытки.

Классика для платежей:

```go
payments.Charge(ctx, ChargeRequest{
	OrderID:        order.ID,
	Amount:         order.Total,
	IdempotencyKey: order.PaymentAttemptID,
})
```

Если сервис повторит запрос с тем же ключом, платежный провайдер должен не списать деньги второй раз, а вернуть тот же результат или состояние операции.

Где брать ключ:

- `order_id`, если по заказу может быть только один платеж;
- `payment_attempt_id`, если допускаются повторные попытки;
- `operation_id`, если это отдельная бизнес-операция;
- request id, если он сохраняется в базе и живет дольше HTTP-запроса.

На ревью надо проверять:

- есть ли idempotency key у внешнего вызова;
- сохраняется ли он до вызова наружу;
- одинаковый ли ключ используется при ретраях;
- что будет при timeout внешнего сервиса;
- можно ли безопасно повторить операцию.

### 4. Saga

Saga - это разбиение большого бизнес-процесса на несколько локальных транзакций и внешних шагов.

Например заказ:

1. Создать заказ со статусом `pending_payment`.
2. Закоммитить.
3. Запустить оплату.
4. Если оплата успешна, перевести заказ в `paid`.
5. Если оплата неуспешна, перевести заказ в `payment_failed`.
6. Если нужно, запустить компенсацию: отменить резерв, вернуть бонусы, отправить уведомление.

В saga нет иллюзии одной общей транзакции. Вместо этого есть:

- явные статусы;
- шаги процесса;
- компенсационные действия;
- retry policy;
- audit log;
- идемпотентные переходы.

Пример статусов:

```text
new -> pending_payment -> paid -> fulfilled
                    |
                    v
              payment_failed
```

Для собеса хорошая формулировка:

> Я бы не пытался держать DB-транзакцию вокруг платежа. Я бы сохранил заказ в pending-статусе, закоммитил локальное состояние, а платеж сделал отдельным шагом с идемпотентным ключом и явными переходами статусов.

### 5. Compensation

Компенсация - это действие, которое логически исправляет уже выполненный side effect.

Например:

- списали деньги, но заказ не удалось собрать - делаем refund;
- зарезервировали товар, но оплата не прошла - снимаем резерв;
- начислили бонусы ошибочно - создаем обратную операцию списания;
- отправили задачу на доставку, но заказ отменен - отправляем cancellation.

Компенсация не равна rollback. Rollback делает вид, что изменения не было. Компенсация создает новое действие, которое приводит бизнес к приемлемому состоянию.

Для денег это особенно важно: часто нельзя “удалить” финансовую операцию. Нужно создать обратную операцию, чтобы сохранялся audit trail.

### 6. State machine

Если процесс состоит из нескольких шагов, полезно явно моделировать состояния.

Плохо:

```go
if paid {
	// do something
}
```

Лучше:

```go
const (
	OrderStatusNew            = "new"
	OrderStatusPendingPayment = "pending_payment"
	OrderStatusPaid           = "paid"
	OrderStatusPaymentFailed  = "payment_failed"
	OrderStatusCanceled       = "canceled"
)
```

И проверять допустимые переходы:

```go
func canMove(from, to string) bool {
	switch from {
	case OrderStatusNew:
		return to == OrderStatusPendingPayment || to == OrderStatusCanceled
	case OrderStatusPendingPayment:
		return to == OrderStatusPaid || to == OrderStatusPaymentFailed
	default:
		return false
	}
}
```

State machine помогает:

- восстанавливаться после падений;
- видеть, на каком шаге застрял процесс;
- безопасно повторять worker;
- не делать невозможные переходы;
- писать понятные алерты.

### 7. Reconciliation

Даже с outbox, идемпотентностью и saga иногда состояние может разойтись. Например платежный провайдер подтвердил платеж, но callback потерялся.

Reconciliation - это периодическая сверка:

- берем заказы в `pending_payment` старше N минут;
- спрашиваем payment provider о статусе;
- приводим локальное состояние к внешнему;
- логируем и алертим странные расхождения.

Это важная часть зрелого дизайна. На ревью можно спросить:

> А что будет, если внешний сервис успешно выполнит операцию, но наш процесс упадет до обновления локального статуса? Есть ли reconciliation или callback processing?

## Типовые ошибки

### Ошибка 1. Внешний вызов внутри DB-транзакции

Пример:

```go
tx, err := repo.Begin(ctx)
if err != nil {
	return err
}
defer tx.Rollback()

if err := repo.SaveOrder(ctx, tx, order); err != nil {
	return err
}

payment, err := payments.Charge(ctx, order.ID, order.Total)
if err != nil {
	return err
}

if err := repo.UpdateStatus(ctx, tx, order.ID, "paid"); err != nil {
	return err
}

return tx.Commit()
```

Что не так:

- транзакция держится во время сетевого вызова;
- платеж нельзя откатить rollback-ом;
- при падении после платежа, но до commit, деньги списаны, а заказ не paid;
- при retry можно списать деньги повторно;
- если внешний сервис медленный, база страдает от долгих транзакций.

Как лучше:

- сохранить заказ в `pending_payment`;
- закоммитить;
- вызвать платеж отдельно;
- обновить статус отдельной транзакцией;
- использовать idempotency key;
- иметь reconciliation или обработку callback-а.

### Ошибка 2. Публикация события до commit

Пример:

```go
repo.SaveOrder(ctx, tx, order)
events.Publish(ctx, "order.created", order.ID)
tx.Commit()
```

Проблемы:

- событие может уйти, а транзакция потом откатится;
- consumer может прочитать несуществующие или старые данные;
- нарушается причинность: внешний мир узнал о событии раньше, чем состояние стало durable.

Как лучше:

- outbox в той же транзакции;
- publish после commit только для некритичных best-effort событий;
- если publish после commit критичен, нужен durable retry mechanism.

### Ошибка 3. Публикация события после commit без outbox

Пример:

```go
repo.SaveOrder(ctx, tx, order)
tx.Commit()
events.Publish(ctx, "order.created", order.ID)
```

На первый взгляд это лучше, чем publish до commit. Но если процесс упадет между `Commit` и `Publish`, событие потеряется.

Подходит только если событие не критично или есть другой механизм восстановления.

Для доменных событий обычно лучше outbox.

### Ошибка 4. Обновление failure-статуса внутри транзакции, которая потом откатится

Пример:

```go
if err := payments.Charge(ctx, order.ID, order.Total); err != nil {
	_ = repo.UpdateStatus(ctx, tx, order.ID, "payment_failed")
	return err
}
```

Если выше стоит `defer tx.Rollback()`, то при `return err` транзакция откатится. Статус `payment_failed`, записанный в эту же транзакцию, исчезнет.

Это очень хороший пункт для ревью, потому что выглядит как аккуратная обработка ошибки, но фактически она не сохраняет failure-state.

Как лучше:

- либо commit-ить заказ в `pending_payment` до платежа;
- либо после ошибки платежа открыть отдельную транзакцию и сохранить failure-state;
- либо хранить payment attempt как отдельную запись с идемпотентным ключом и статусом.

### Ошибка 5. `defer tx.Rollback()` без понимания жизненного цикла

Пример:

```go
defer tx.Rollback()
...
if err := tx.Commit(); err != nil {
	return err
}
return nil
```

Это не всегда баг уровня production incident, но на ревью стоит уточнить:

- будет ли rollback после commit возвращать шумную ошибку;
- не скрывается ли ошибка rollback;
- понятно ли из кода, где транзакция уже завершена;
- не будет ли после commit случайно еще использоваться tx.

Более аккуратный вариант:

```go
committed := false
defer func() {
	if !committed {
		_ = tx.Rollback()
	}
}()

if err := tx.Commit(); err != nil {
	return err
}
committed = true
```

### Ошибка 6. Ретраи неидемпотентных операций

Пример:

```go
for i := 0; i < 3; i++ {
	err := payments.Charge(ctx, orderID, amount)
	if err == nil {
		return nil
	}
}
```

Если `Charge` упал по timeout, неизвестно, списались деньги или нет. Повтор может списать второй раз.

Как лучше:

- использовать idempotency key;
- хранить payment attempt;
- различать retryable и non-retryable ошибки;
- проверять статус операции у provider-а;
- не делать blind retry для денежных операций.

### Ошибка 7. Длинная транзакция вокруг лишней работы

Плохо:

```go
tx, _ := repo.Begin(ctx)
defer tx.Rollback()

payload := buildHugePayload(order)
score := antifraud.Check(ctx, payload)
price := pricing.Calculate(ctx, order)

repo.Save(ctx, tx, order, score, price)
tx.Commit()
```

Если `buildHugePayload`, `antifraud.Check` и `pricing.Calculate` не требуют открытой транзакции, их лучше сделать до `Begin`.

Принцип:

> Begin должен быть как можно ближе к первой DB-операции, Commit должен быть как можно ближе к последней DB-операции.

### Ошибка 8. Запуск goroutine внутри транзакции

Плохо:

```go
tx, _ := repo.Begin(ctx)

go func() {
	_ = repo.UpdateSomething(ctx, tx)
}()

return tx.Commit()
```

Проблемы:

- `Tx` обычно не надо использовать конкурентно;
- commit может случиться раньше goroutine;
- rollback может закрыть tx, пока goroutine еще работает;
- ошибки теряются;
- порядок операций становится неочевидным.

Внутри транзакции лучше держать линейный, понятный, короткий код.

### Ошибка 9. Передача `tx` слишком глубоко и неявно

Иногда код выглядит так:

```go
func (s *Service) CreateOrder(ctx context.Context, order Order) error {
	tx, _ := s.repo.Begin(ctx)
	defer tx.Rollback()

	return s.orderManager.Create(ctx, tx, order)
}
```

А внутри `orderManager.Create` может быть:

```go
events.Publish(ctx, "order.created", order.ID)
payments.Charge(ctx, order.ID, order.Total)
```

На верхнем уровне кажется, что транзакция только про базу, но глубоко внутри внезапно появляются внешние вызовы.

На ревью стоит смотреть не только на метод, где начинается tx, но и на все вызовы внутри транзакционного блока.

Хорошая практика:

- явно отделять методы, которые работают только с БД;
- не прятать HTTP/broker/payment внутри repository-методов;
- именовать функции так, чтобы side effects были видны;
- держать транзакционный блок коротким и читаемым.

### Ошибка 10. Игнорирование ошибки commit

Плохо:

```go
_ = tx.Commit()
return nil
```

Если commit упал, вызывающий код думает, что операция успешна. Для бизнес-операций это почти всегда критично.

Ошибка commit должна быть обработана, залогирована и возвращена. Если результат commit неоднозначен, нужен способ восстановить состояние по operation id.

### Ошибка 11. Ошибка rollback полностью теряется

Rollback обычно не должен перебивать основную ошибку, но его failure полезно логировать.

Пример:

```go
defer func() {
	if !committed {
		if rbErr := tx.Rollback(); rbErr != nil {
			logger.Error("rollback failed", "error", rbErr)
		}
	}
}()
```

Если rollback не сработал, соединение или транзакция могли остаться в плохом состоянии. Обычно это проблема инфраструктурного уровня, но для диагностики она важна.

### Ошибка 12. `context.Background()` внутри транзакционного сценария

Плохо:

```go
repo.SaveOrder(context.Background(), tx, order)
```

Если входной `ctx` отменился, операция все равно продолжит работать. Это ломает deadline запроса и может держать транзакцию дольше нужного.

Для бизнес-операции обычно надо передавать входной `ctx`. Исключения бывают в фоновых cleanup-задачах, но их лучше делать явно и с собственным timeout.

### Ошибка 13. Кеш обновляется как будто он часть транзакции

Пример:

```go
repo.UpdateOrder(ctx, tx, order)
cache.Set(order.ID, order)
tx.Commit()
```

Если commit откатится, кеш уже содержит новое значение, которого нет в базе.

Если cache.Set после commit, есть риск, что процесс упадет между commit и cache update. Это обычно менее страшно, потому что кеш можно инвалидировать или восстановить из базы, но это надо понимать.

Для кеша часто лучше:

- invalidate after commit;
- использовать TTL;
- не считать кеш источником истины;
- делать cache-aside;
- при критичных данных опираться на БД.

## Как избегать проблем

### Правило 1. Не держать транзакцию вокруг внешних вызовов

Если внутри транзакции есть:

- `httpClient.Do`;
- `grpcClient.SomeCall`;
- `payments.Charge`;
- `events.Publish`;
- `email.Send`;
- `time.Sleep`;
- ожидание канала;
- запуск worker-а;
- большой CPU-bound расчет;

это красный флаг.

Не всегда это автоматически запрещено, но нужно очень сильное обоснование.

### Правило 2. Делать транзакцию короткой

Хорошая транзакция:

- начинается прямо перед DB-изменениями;
- содержит только нужные запросы;
- не делает внешних I/O;
- быстро коммитится;
- возвращает ошибку commit;
- аккуратно делает rollback при ошибке.

### Правило 3. Сначала проектировать бизнес-состояния

Для сложных процессов надо задать вопросы:

- какие есть статусы;
- какой статус сохраняется до внешнего вызова;
- что происходит при успехе;
- что происходит при failure;
- что происходит при timeout;
- что происходит при retry;
- что происходит, если процесс упал между шагами;
- кто и как продолжит процесс;
- можно ли безопасно повторить шаг.

Если ответов нет, код почти наверняка будет хрупким.

### Правило 4. Использовать idempotency для внешних операций

Любой внешний side effect, который может быть повторен из-за retry, timeout или падения процесса, должен иметь идемпотентность.

Особенно:

- платежи;
- списание бонусов;
- резерв товара;
- создание доставки;
- отправка критичных сообщений;
- начисление баланса.

### Правило 5. Для событий использовать outbox

Если событие является частью бизнес-контракта, простого `Publish` рядом с `Commit` мало.

Лучше:

- сохранять доменное изменение и outbox event в одной транзакции;
- публиковать из отдельного worker-а;
- делать consumer-ы идемпотентными;
- мониторить backlog.

### Правило 6. Не пытаться скрыть распределенную систему за одной функцией

Метод может называться `CreateOrder`, но на самом деле он делает распределенный процесс:

- БД;
- платежный провайдер;
- брокер;
- warehouse;
- delivery.

Если это распределенный процесс, ему нужны распределенные гарантии: статусы, retries, idempotency, compensation, reconciliation.

Обычная функция с `return err` не покрывает все варианты отказа.

## Как диагностировать проблемы

### На code review

Идешь по функции сверху вниз и отмечаешь:

1. Где начинается транзакция.
2. Где она заканчивается.
3. Какие операции выполняются внутри.
4. Есть ли внутри вызовы наружу.
5. Какие изменения rollback реально откатит.
6. Какие side effects rollback не откатит.
7. Что будет, если процесс упадет после каждого важного шага.
8. Что будет, если внешний вызов выполнится, но вернет timeout.
9. Что будет при повторном вызове метода.
10. Есть ли idempotency key или уникальный operation id.
11. Есть ли outbox для событий.
12. Есть ли обработка commit error.
13. Есть ли логирование и метрики для stuck-состояний.

Очень полезная техника:

> Мысленно ставь crash point между каждой парой строк.

Например:

```text
SaveOrder
CRASH?
Charge
CRASH?
UpdateStatus
CRASH?
Commit
CRASH?
PublishEvent
CRASH?
```

И для каждого crash point отвечай:

- что уже успело измениться;
- что не успело;
- кто продолжит;
- можно ли повторить;
- не будет ли дублирования.

### В логах

Для таких сценариев нужны связующие идентификаторы:

- `order_id`;
- `operation_id`;
- `payment_attempt_id`;
- `idempotency_key`;
- `outbox_event_id`;
- `trace_id`;
- `request_id`.

Логи должны позволять собрать историю:

```text
order created pending_payment
payment attempt created
payment charge requested
payment provider timeout
payment status reconciliation started
payment confirmed
order marked paid
outbox event published
```

Если в логах нет operation id, расследовать такие баги крайне трудно.

### В метриках

Полезные метрики:

- количество заказов в `pending_payment` старше N минут;
- количество failed payment attempts;
- outbox backlog;
- outbox oldest unpublished age;
- retry attempts count;
- commit latency;
- transaction duration;
- DB lock wait time;
- deadlock count;
- external API latency внутри бизнес-процессов;
- количество ambiguous commit / timeout cases;
- reconciliation corrections count.

Особенно полезны age-метрики:

```text
oldest_pending_payment_age_seconds
oldest_outbox_event_age_seconds
```

Они показывают, что процесс застрял.

### В трассировке

Trace должен показывать:

- границу транзакции;
- DB-запросы;
- внешние вызовы;
- время между begin и commit;
- retry attempts;
- worker processing.

Если видно, что транзакция открыта 800 ms, а внутри 700 ms занял HTTP payment call, это сильный сигнал.

### В тестах

Обычные happy-path тесты эту тему почти не ловят. Нужны fault-injection сценарии:

- платеж успешен, но `UpdateStatus` вернул ошибку;
- платеж успешен, но `Commit` вернул ошибку;
- `Commit` успешен, но `Publish` упал;
- процесс упал после commit, но до publish;
- worker отправил событие, но упал до отметки `published_at`;
- внешний вызов timeout, но операция у провайдера на самом деле выполнена;
- повторный запрос с тем же idempotency key;
- consumer получил одно сообщение два раза.

Даже если не писать сложную инфраструктуру, на ревью можно сказать:

> Я бы добавил тесты на failure points между шагами, потому что happy path не показывает, как система ведет себя при частичном успехе.

## Методология ревью

### Шаг 1. Найти границу транзакции

Ищи:

- `Begin`;
- `BeginTx`;
- `WithTransaction`;
- `RunInTx`;
- `tx :=`;
- `Commit`;
- `Rollback`.

Определи точный блок кода, который выполняется внутри транзакции.

### Шаг 2. Выписать все side effects внутри этой границы

Ищи:

- payment;
- publish;
- send;
- notify;
- upload;
- cache;
- HTTP/gRPC clients;
- goroutines;
- queue;
- sleep/wait;
- locks не из БД;
- любые вызовы, которые могут зависнуть или изменить внешний мир.

### Шаг 3. Разделить изменения на откатываемые и неоткатываемые

Откатываемые:

- insert/update/delete в той же DB-транзакции.

Неоткатываемые:

- платеж;
- отправленное сообщение;
- email;
- cache update;
- внешний HTTP-запрос;
- файл;
- событие в брокер;
- изменение в другой базе без общей транзакции.

### Шаг 4. Проверить порядок операций

Типовые вопросы:

- можно ли публиковать событие до commit;
- что будет, если commit упадет после внешнего success;
- есть ли durable запись о намерении сделать side effect;
- можно ли повторить операцию;
- не держим ли lock слишком долго.

### Шаг 5. Проверить идемпотентность

Вопросы:

- есть ли idempotency key;
- где он хранится;
- одинаковый ли он при повторе;
- есть ли unique constraint;
- как обрабатывается duplicate request;
- что делает внешний provider при повторе.

### Шаг 6. Проверить восстановление

Вопросы:

- кто продолжит процесс после падения;
- есть ли worker;
- есть ли outbox/inbox;
- есть ли reconciliation;
- есть ли stuck statuses;
- есть ли ручной repair path.

### Шаг 7. Проверить наблюдаемость

Вопросы:

- можно ли понять, на каком шаге процесс упал;
- есть ли correlation id;
- есть ли метрики очередей и pending-статусов;
- есть ли алерты на старые pending records;
- логируется ли rollback/commit failure.

## Как говорить на собеседовании

Хорошая короткая формулировка:

> Здесь внешний вызов находится внутри DB-транзакции. Это опасно, потому что rollback откатит только изменения в БД, но не откатит платеж или опубликованное событие. Плюс транзакция будет держать locks и соединение, пока мы ждем сеть.

Про payment:

> Для платежей я бы не делал `Charge` внутри транзакции. Я бы сохранил заказ и payment attempt в pending-статусе, закоммитил, а затем выполнял оплату отдельным шагом с idempotency key. После результата платежа отдельной транзакцией обновлял бы статус заказа.

Про events:

> Если событие критично, publish после commit без outbox может потерять событие при падении процесса. Publish до commit может отправить событие о данных, которые еще не закоммичены или потом откатятся. Поэтому здесь лучше transactional outbox.

Про retry:

> Retry внешних операций должен быть идемпотентным. Timeout не означает, что операция не выполнилась, поэтому слепой повтор платежа может привести к двойному списанию.

Про commit:

> Ошибку commit нельзя игнорировать. Более того, при некоторых сбоях результат commit может быть неоднозначным, поэтому нужна возможность восстановиться по operation id.

Про transaction duration:

> Я бы сократил транзакционный блок: все вычисления и внешние проверки по возможности до `Begin`, только DB-записи внутри, `Commit` сразу после последнего запроса.

## Пример плохого дизайна и улучшения

### Плохой вариант

```go
func (s *Service) CreateOrder(ctx context.Context, order Order) error {
	tx, err := s.repo.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := s.repo.SaveOrder(ctx, tx, order); err != nil {
		return err
	}

	payment, err := s.payments.Charge(ctx, order.ID, order.Total)
	if err != nil {
		_ = s.repo.UpdateStatus(ctx, tx, order.ID, "payment_failed")
		return err
	}

	if payment.Status != "paid" {
		_ = s.repo.UpdateStatus(ctx, tx, order.ID, "payment_failed")
		return errors.New("payment failed")
	}

	if err := s.repo.UpdateStatus(ctx, tx, order.ID, "paid"); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return s.events.Publish(ctx, "order.created", order.ID)
}
```

Проблемы:

- `Charge` внутри транзакции;
- rollback не откатывает платеж;
- `payment_failed` пишется в транзакции, которая откатится при return;
- publish после commit может потеряться;
- нет idempotency key;
- нет payment attempt;
- нет outbox;
- долгий transaction duration;
- rollback после commit вызывается через defer;
- возвращаемая ошибка publish может сказать клиенту, что заказ не создан, хотя заказ уже создан.

### Более здоровый вариант

Это не единственный правильный дизайн, но он показывает направление.

```go
func (s *Service) CreateOrder(ctx context.Context, order Order) error {
	order.Status = "pending_payment"
	paymentAttempt := PaymentAttempt{
		ID:             newID(),
		OrderID:        order.ID,
		Amount:         order.Total,
		Status:         "new",
		IdempotencyKey: newID(),
	}

	tx, err := s.repo.Begin(ctx)
	if err != nil {
		return err
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err := s.repo.SaveOrder(ctx, tx, order); err != nil {
		return err
	}

	if err := s.repo.SavePaymentAttempt(ctx, tx, paymentAttempt); err != nil {
		return err
	}

	if err := s.repo.SaveOutboxEvent(ctx, tx, OutboxEvent{
		ID:          newID(),
		Type:        "payment.requested",
		AggregateID: order.ID,
		Payload:     paymentAttempt,
	}); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true

	return nil
}
```

Дальше отдельный worker:

```go
func (w *PaymentWorker) Handle(ctx context.Context, event PaymentRequested) error {
	attempt, err := w.repo.GetPaymentAttempt(ctx, event.PaymentAttemptID)
	if err != nil {
		return err
	}
	if attempt.Status == "paid" {
		return nil
	}

	result, err := w.payments.Charge(ctx, ChargeRequest{
		OrderID:        attempt.OrderID,
		Amount:         attempt.Amount,
		IdempotencyKey: attempt.IdempotencyKey,
	})
	if err != nil {
		return err
	}

	tx, err := w.repo.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if result.Status == "paid" {
		if err := w.repo.MarkPaymentPaid(ctx, tx, attempt.ID); err != nil {
			return err
		}
		if err := w.repo.UpdateOrderStatus(ctx, tx, attempt.OrderID, "paid"); err != nil {
			return err
		}
		if err := w.repo.SaveOutboxEvent(ctx, tx, OrderPaidEvent(attempt.OrderID)); err != nil {
			return err
		}
	} else {
		if err := w.repo.MarkPaymentFailed(ctx, tx, attempt.ID); err != nil {
			return err
		}
		if err := w.repo.UpdateOrderStatus(ctx, tx, attempt.OrderID, "payment_failed"); err != nil {
			return err
		}
	}

	return tx.Commit()
}
```

Да, кода больше. Но зато система честно признает, что платеж - внешний процесс, а не часть DB-транзакции.

## На что обращать внимание именно в Go

### `database/sql.Tx`

`sql.Tx` представляет одну транзакцию, привязанную к конкретному соединению. После `Commit` или `Rollback` транзакцию использовать нельзя.

Для ревью:

- не передавать `tx` в goroutine;
- не использовать `tx` после commit/rollback;
- не держать `tx` открытым во время сетевых вызовов;
- передавать `ctx`, а не `context.Background()`;
- задавать timeout на уровне входной операции или worker-а.

### Repository interface

Если repository методы принимают `tx`, хорошо, когда это явно:

```go
SaveOrder(ctx context.Context, tx Tx, order Order) error
```

Но надо следить, чтобы внутри repository не было внешних side effects. Repository обычно должен работать с хранилищем, а не отправлять события или дергать платежи.

Если есть `RunInTx`, важно смотреть, что передается внутрь callback-а:

```go
err := repo.RunInTx(ctx, func(ctx context.Context, tx Tx) error {
	return service.DoSomething(ctx, tx)
})
```

Внутри callback-а легко случайно спрятать внешний вызов.

### Context

У транзакционного кода должен быть понятный context:

- request context для синхронного запроса;
- worker context для фоновой обработки;
- отдельный timeout для внешнего вызова, если он нужен;
- не `context.Background()` в середине бизнес-операции.

Но есть нюанс: если клиент отменил HTTP-запрос, а сервер уже начал критичный payment workflow, иногда бизнес хочет довести процесс до консистентного состояния. Тогда это должно быть явно спроектировано как асинхронный процесс, а не случайный `context.Background()`.

## Мини-чеклист для ревью

Можно прямо держать в голове:

- Где начинается и заканчивается транзакция?
- Есть ли внутри транзакции внешний I/O?
- Какие действия rollback реально откатит?
- Какие действия rollback не откатит?
- Что будет, если процесс упадет после каждого шага?
- Что будет, если внешний вызов успешен, но ответ потерялся?
- Можно ли безопасно повторить операцию?
- Есть ли idempotency key?
- Есть ли outbox для событий?
- Есть ли inbox/dedup у consumer-а?
- Есть ли явные статусы процесса?
- Есть ли compensation или reconciliation?
- Не держим ли DB locks во время сети?
- Обрабатывается ли commit error?
- Логируется ли rollback failure?
- Можно ли по логам восстановить историю операции?

## Сильные вопросы интервьюеру или автору кода

На ревью можно не только утверждать, но и задавать точные вопросы:

- Что будет, если платеж прошел, а commit не прошел?
- Что будет, если payment API вернул timeout?
- Есть ли idempotency key для повторной попытки?
- Можно ли получить двойное списание?
- Почему внешний вызов находится внутри DB-транзакции?
- Можно ли сократить время жизни транзакции?
- Что будет, если процесс упадет после commit, но до publish?
- Это событие best-effort или часть бизнес-контракта?
- Нужен ли здесь outbox?
- Кто обработает заказ, застрявший в `pending_payment`?
- Есть ли reconciliation с платежным провайдером?
- Как consumer защищен от повторной доставки события?
- Можно ли повторно вызвать этот endpoint с тем же request id?

## Короткое резюме

Самая сильная мысль темы:

> DB-транзакция не делает атомарным внешний мир.

Из этого следуют почти все ревью-замечания:

- не делать сетевые side effects внутри транзакции;
- не публиковать критичные события напрямую рядом с commit;
- использовать transactional outbox;
- делать внешние операции идемпотентными;
- явно моделировать статусы процесса;
- проектировать retry, compensation и reconciliation;
- держать транзакции короткими;
- проверять failure points между шагами.

Если на собеседовании ты видишь `payments.Charge`, `events.Publish`, `email.Send`, `httpClient.Do` или `cache.Set` внутри транзакции, это почти всегда место, где стоит остановиться и вслух разобрать последствия.
