---
lk:
  source_role: primary_source_artifact
  source_refs:
    - "Benjamin Peirce, Linear Associative Algebra, 1870/1881: historical origin of the mathematical term idempotent"
    - "MacTutor History of Mathematics: Earliest Known Uses of Some of the Words of Mathematics, entry IDEMPOTENT"
    - "Online Etymology Dictionary: idempotent"
    - "RFC 9110, section 9.2.2: Idempotent Methods"
    - "Roy Fielding, Architectural Styles and the Design of Network-based Software Architectures, chapter 5: REST"
    - "Martin Kleppmann, Designing Data-Intensive Applications: reliability, partial failures, replication, transactions, messaging"
    - "Gregor Hohpe and Bobby Woolf, Enterprise Integration Patterns: Idempotent Receiver"
    - "Chris Richardson, Microservices Patterns: Idempotent Consumer"
    - "AWS Builders' Library: Making retries safe with idempotent APIs"
    - "AWS Well-Architected Framework REL04-BP04: Make all responses idempotent"
    - "Microsoft Azure Architecture Center: Retry pattern"
    - "Stripe API docs: Idempotent requests"
    - "PayPal REST API docs: Idempotency"
    - "Apache Kafka documentation: producer enable.idempotence and transactional.id"
  prompt_helper: |
    Проверять понимание идемпотентности как контракта повторного выполнения:
    математическая идея, HTTP semantics, idempotency keys, retry safety,
    consumer deduplication, outbox/inbox, at-least-once и границы exactly-once.
  challenge_helper: |
    Давай мини-кейс: платеж, создание заказа, consumer события или HTTP retry
    после timeout. Проси спроектировать ключи, хранилище, TTL, replay response,
    обработку параллельных дублей, payload mismatch, observability и recovery.
---

# Идемпотентность

Идемпотентность - это свойство операции: если выполнить ее один раз или несколько раз с тем же намерением, итоговый эффект будет тем же.

Самая короткая формулировка:

> Повтор не должен создавать новый бизнес-эффект.

Для backend-разработчика это не абстрактная математика, а один из главных инструментов надежности. Сети теряют ответы. Клиенты повторяют запросы. SDK делают автоматические ретраи. Брокеры сообщений доставляют одно и то же сообщение повторно. Worker может упасть после выполнения действия, но до записи статуса. Пользователь может нажать кнопку оплаты два раза. Без идемпотентности повтор превращается в двойной платеж, два заказа, два письма, два списания, два созданных сервера, два инкремента счетчика.

Главная мысль:

> Идемпотентность нужна не потому, что разработчики любят красивые термины. Она нужна потому, что в распределенной системе отправитель часто не знает, было ли действие уже выполнено.

## Простое введение

Представь кнопку "выключить свет".

Если свет включен, первое нажатие выключит его. Второе, третье и десятое нажатие не сделают комнату "еще более выключенной". Итоговое состояние одно:

```text
light = off
```

Это идемпотентная операция.

Теперь представь кнопку "добавить товар в корзину".

Нажал один раз - в корзине один товар. Нажал второй раз - два товара. Нажал десять раз - десять товаров.

```text
cart.items += 1
```

Это неидемпотентная операция.

Разница не в том, что первая операция "простая", а вторая "сложная". Разница в смысле команды:

- `set light = off` задает конечное состояние;
- `add one item` добавляет новый эффект при каждом повторе.

То же самое в коде:

```sql
UPDATE users
SET email = 'new@example.com'
WHERE id = 42;
```

Если выполнить этот запрос несколько раз, email останется `new@example.com`. Операция похожа на "установить состояние".

А вот это другое:

```sql
UPDATE accounts
SET balance = balance - 100
WHERE id = 42;
```

Если выполнить один раз, баланс уменьшится на 100. Если выполнить три раза, баланс уменьшится на 300. Это "добавить изменение", а не "установить состояние".

На собеседовании можно сказать так:

> Идемпотентная операция допускает безопасный повтор: после первого успешного применения дополнительные повторы не создают нового бизнес-эффекта. Например, `set status = cancelled` обычно идемпотентен, а `charge card` или `increment balance` без ключа операции - нет.

## Что именно должно быть одинаковым

Новички часто думают: идемпотентность значит, что каждый вызов возвращает одинаковый ответ.

Это не совсем так.

Идемпотентность прежде всего про эффект на состояние системы. Ответы могут отличаться.

Например:

```http
DELETE /users/42
```

Первый вызов может вернуть:

```http
204 No Content
```

Повторный вызов может вернуть:

```http
404 Not Found
```

Но состояние после первого и после второго вызова одно и то же: пользователя `42` нет. Поэтому `DELETE` в HTTP считается идемпотентным по смыслу метода, хотя конкретные ответы могут различаться.

Однако для платежей, создания ресурсов и автоматических ретраев часто хочется более сильный контракт: не только "эффект тот же", но и "повтор возвращает понятный результат первой операции". AWS называет это семантически эквивалентным ответом: клиент повторяет запрос с тем же ключом и получает ответ, который означает тот же исход, а не неожиданный `already exists`.

Поэтому есть два уровня:

1. Базовая идемпотентность: повтор не меняет состояние дополнительно.
2. Идемпотентный API-контракт: повтор еще и возвращает предсказуемый ответ, удобный для клиента.

В реальных API второй уровень часто важнее для developer experience.

## Историческая справка

Слово `idempotent` пришло не из веба и не из микросервисов. Оно старше современного программирования.

Этимологически слово собирается из латинских корней:

- `idem` - "тот же самый";
- `potens` / `potent` - "имеющий силу", "способный", "мощный".

Грубо: "имеющий ту же силу".

В математике термин связывают с Бенджамином Пирсом и его работой `Linear Associative Algebra`. В исторических справках указывается 1870 год: Пирс использовал `idempotent` для выражений, которые при возведении в квадрат или более высокую степень дают самих себя. В современной записи базовый вид такой:

```text
A^2 = A
```

Примеры из математики и computer science:

```text
0 * 0 = 0
1 * 1 = 1
```

Числа `0` и `1` идемпотентны относительно умножения.

Функция `abs` идемпотентна относительно повторного применения:

```text
abs(abs(x)) = abs(x)
```

Операция "отсортировать список" тоже идемпотентна в бытовом смысле:

```text
sort(sort(xs)) = sort(xs)
```

После первой сортировки повторная сортировка уже не меняет порядок.

Математическая идея постепенно переехала в программирование: если операция приводит систему к состоянию, которое уже не меняется от повторного применения той же операции, ее удобно безопасно повторять.

## Как идея попала в HTTP и REST

В вебе идемпотентность стала важной из-за масштаба и промежуточной инфраструктуры: браузеры, прокси, кеши, поисковые роботы, балансировщики, повторные подключения, обрывы соединений.

HTTP определяет методы не просто как "глаголы", а как контракты с семантикой. В актуальном стандарте [RFC 9110](https://datatracker.ietf.org/doc/html/rfc9110#section-9.2.2) идемпотентность метода определяется через предполагаемый эффект на сервере при нескольких одинаковых запросах.

Типичная таблица:

| Метод | Safe | Idempotent | Смысл |
|---|---:|---:|---|
| `GET` | да | да | получить представление ресурса |
| `HEAD` | да | да | получить headers без body |
| `OPTIONS` | да | да | узнать доступные опции |
| `PUT` | нет | да | заменить ресурс заданным представлением |
| `DELETE` | нет | да | удалить ресурс |
| `POST` | нет | нет по умолчанию | передать данные для обработки, часто создать новый эффект |
| `PATCH` | нет | нет по умолчанию | частично изменить ресурс |

`Safe` и `idempotent` - разные свойства.

Safe-метод означает, что клиент не просит сервер менять состояние. `GET` должен быть безопасным: перейти по ссылке, открыть страницу, проиндексировать URL поисковым роботом не должно создавать заказ или списывать деньги.

Idempotent-метод может менять состояние, но повтор не должен менять его дополнительно. `DELETE` не safe, потому что он удаляет ресурс. Но он idempotent, потому что после удаления повторное удаление оставляет ресурс удаленным.

REST в диссертации Роя Филдинга делает акцент на uniform interface: компоненты общаются через общий интерфейс и видимую семантику сообщений. Именно поэтому свойства методов важны. Если метод называется `GET`, инфраструктура и люди ожидают одно поведение. Если `GET /orders/123?pay=true` списывает деньги, система нарушает ожидания и становится опасной для автоматических переходов, prefetch, crawlers и retry logic.

Коротко:

> HTTP-идемпотентность - это не магия протокола. Это обещание владельца API соблюдать семантику метода.

## Почему идемпотентность стала архитектурной темой

В монолитном локальном коде часто есть иллюзия простоты:

```go
err := service.CreateOrder(ctx, input)
if err != nil {
	return err
}
```

Мы вызвали функцию, получили ошибку или успех. Кажется, все понятно.

В распределенной системе вызов выглядит иначе:

```text
client -> network -> load balancer -> service -> database -> payment provider
```

И теперь есть несколько неприятных сценариев:

1. Запрос не дошел до сервера.
2. Запрос дошел, но сервер упал до обработки.
3. Сервер обработал запрос, но упал до ответа.
4. Сервер обработал запрос, отправил ответ, но ответ потерялся в сети.
5. Клиент получил timeout, хотя сервер продолжает работу.
6. Клиент, SDK, worker или gateway повторяет запрос.

С точки зрения клиента сценарии 1, 3 и 4 могут выглядеть одинаково: "я не получил успешный ответ". Но последствия разные. В одном случае действие не произошло, в другом произошло, а в третьем произошло и ответ просто потерялся.

Если операция неидемпотентна, retry становится опасным:

```text
POST /payments
timeout
POST /payments
```

Без идемпотентности это может быть два списания.

Если операция идемпотентна:

```text
POST /payments
Idempotency-Key: pay_attempt_123
timeout

POST /payments
Idempotency-Key: pay_attempt_123
```

сервер может понять: это не новая покупка, а повтор той же попытки.

Именно поэтому AWS Builders' Library связывает идемпотентные API с безопасными ретраями: ретраи сильно упрощают клиентский код и повышают надежность, но только если повтор не создает второй эффект.

## Идемпотентность и retry

Retry - естественный ответ на transient failure.

Если зависимость временно перегружена, сеть на секунду оборвалась или сервер вернул `503`, повтор через backoff может успешно завершить операцию. Microsoft Azure Architecture Center прямо выделяет retry pattern как способ переживать временные сбои, но отдельно предупреждает: перед повтором нужно понимать, идемпотентна ли операция.

На практике хороший retry policy задает:

- какие ошибки retryable;
- сколько попыток делать;
- с каким backoff;
- есть ли jitter;
- есть ли общий deadline;
- не усилит ли retry перегрузку;
- можно ли повторять именно эту операцию.

Идемпотентность отвечает на последний вопрос.

Если операция идемпотентна, retry может быть обычной частью надежности.

Если операция неидемпотентна, retry должен быть либо запрещен, либо превращен в идемпотентный через operation id, idempotency key, unique constraint, state machine, outbox/inbox или другой механизм.

Главная ловушка:

> Timeout не означает, что операция не выполнилась.

Timeout означает только то, что вызывающая сторона не получила ответ за отведенное время.

## Идемпотентность не равна exactly-once

Это один из самых важных моментов.

Идемпотентность не обещает, что система физически выполнит код один раз. Она обещает, что повторное получение того же намерения не создаст повторный эффект.

В распределенных системах часто различают:

- `at-most-once`: отправили один раз, если потерялось - потерялось;
- `at-least-once`: повторяем, пока не будет подтверждения, возможны дубли;
- `exactly-once`: хочется, чтобы эффект был ровно один раз.

`Exactly-once` в бытовом смысле обычно достигается не тем, что все компоненты никогда не повторяют работу, а комбинацией:

- durable state;
- transactional boundaries;
- deduplication;
- idempotent operations;
- careful acknowledgement;
- reconciliation.

Например, Kafka имеет `enable.idempotence` для producer-а. Это помогает не записывать дубликаты в stream при внутренних retry producer-а, если соблюдены условия по `acks`, `retries` и `max.in.flight.requests.per.connection`. Но это не значит, что весь бизнес-процесс стал exactly-once. Если consumer прочитал сообщение, списал деньги во внешнем API и упал до commit offset, сообщение может быть обработано повторно. Внешний side effect все равно должен быть идемпотентным.

Короткая формулировка:

> Exactly-once delivery не заменяет идемпотентность бизнес-эффектов.

## Виды идемпотентности

Полезно различать несколько уровней.

### 1. Математическая идемпотентность

Функция повторно применяется к своему результату:

```text
f(f(x)) = f(x)
```

Примеры:

- `abs(abs(x)) = abs(x)`;
- `sort(sort(xs)) = sort(xs)`;
- `normalize(normalize(text)) = normalize(text)`, если normalization deterministic.

### 2. Идемпотентность состояния

Операция задает конечное состояние:

```sql
UPDATE orders
SET status = 'cancelled'
WHERE id = 100;
```

Повтор оставляет заказ cancelled.

### 3. Идемпотентность API

API принимает повтор того же запроса и не создает второй ресурс или второй side effect:

```http
POST /payment-attempts
Idempotency-Key: 36f2f1bb-8e19-4a25-82c0-3ad145cb53f5
```

### 4. Идемпотентность consumer-а

Consumer может получить одно и то же сообщение несколько раз, но обработает бизнес-эффект один раз:

```text
message_id = evt_123
consumer = billing_projection
```

Если пара `(consumer, message_id)` уже обработана, дубликат игнорируется или возвращает сохраненный результат.

### 5. Идемпотентность workflow

Целый процесс можно безопасно продолжать после падения:

```text
order pending -> payment started -> payment confirmed -> order paid -> event published
```

Повтор worker-а не начинает оплату заново, а смотрит на статус и продолжает с нужного шага.

## Естественная и спроектированная идемпотентность

Некоторые операции идемпотентны естественно.

```sql
UPDATE users
SET blocked = true
WHERE id = 42;
```

Повтор не создает новый эффект.

```sql
INSERT INTO user_settings (user_id, timezone)
VALUES (42, 'Europe/Moscow')
ON CONFLICT (user_id)
DO UPDATE SET timezone = EXCLUDED.timezone;
```

Повтор приводит к тому же значению.

Другие операции надо специально проектировать.

```sql
INSERT INTO payments (user_id, amount)
VALUES (42, 1000);
```

Если выполнить дважды, будет две записи.

Но можно добавить business operation id:

```sql
CREATE TABLE payment_attempts (
	id bigserial PRIMARY KEY,
	merchant_id bigint NOT NULL,
	idempotency_key text NOT NULL,
	order_id bigint NOT NULL,
	amount_cents bigint NOT NULL,
	status text NOT NULL,
	provider_payment_id text,
	created_at timestamptz NOT NULL DEFAULT now(),
	updated_at timestamptz NOT NULL DEFAULT now(),
	UNIQUE (merchant_id, idempotency_key)
);
```

Теперь повтор с тем же `(merchant_id, idempotency_key)` не создаст вторую попытку.

## Почему hash payload не всегда ключ идемпотентности

Иногда хочется сказать: "если два запроса имеют одинаковый body, считаем их дублями".

Это опасно.

Представь API:

```http
POST /instances
{
  "image": "ubuntu-24.04",
  "type": "small"
}
```

Два одинаковых запроса могут означать разные намерения:

- пользователь хочет создать одну instance, а второй запрос - retry;
- пользователь реально хочет создать две одинаковые instance.

Сервер не может надежно угадать намерение по payload. Поэтому AWS Builders' Library рекомендует caller-provided client request identifier: клиент явно говорит, какие повторы относятся к одной и той же операции.

Hash payload полезен как защита от misuse:

- первый запрос с ключом `abc` был на `amount=1000`;
- повтор с ключом `abc` пришел на `amount=9999`;
- сервер должен не выполнять новую операцию, а вернуть ошибку mismatch.

То есть ключ выражает намерение, а hash payload проверяет, что намерение не переиспользовали для другого действия.

## Idempotency key

`Idempotency key` - это уникальный идентификатор бизнес-попытки, который клиент передает при повторяемой операции.

Пример:

```http
POST /payments
Idempotency-Key: 7d68b4fd-0df6-4518-9f42-b1cb6e0f19a1
Content-Type: application/json

{
  "order_id": "ord_100",
  "amount_cents": 150000,
  "currency": "RUB"
}
```

Сервер использует ключ, чтобы отличить:

- новый платеж;
- повтор того же платежа после timeout;
- случайное повторное нажатие кнопки;
- ошибочное переиспользование ключа с другим payload.

В Stripe похожий механизм применяется для `POST`-запросов: результат первой начавшейся обработки сохраняется на ключ и повтор с тем же ключом возвращает сохраненный результат. PayPal использует свой header `PayPal-Request-Id` для поддерживаемых REST API и возвращает статус предыдущей операции, если повтор пришел с тем же идентификатором.

Важно: общий header `Idempotency-Key` долго существовал как распространенная практика и как IETF draft, но по состоянию на 2026-07-22 draft `draft-ietf-httpapi-idempotency-key-header-07` помечен в Datatracker как expired, а не как опубликованный RFC. Поэтому в реальном проекте надо смотреть документацию конкретного API: `Idempotency-Key`, `PayPal-Request-Id`, `ClientToken`, `request_id` и другие имена могут означать похожую идею, но детали отличаются.

## Как проектировать idempotency key

Ключ должен быть:

- уникальным для одной бизнес-попытки;
- стабильным между retry одной попытки;
- достаточно случайным или глобально уникальным;
- scoped по caller-у, tenant-у или аккаунту;
- не содержащим персональные данные и секреты;
- логируемым и пригодным для расследования.

Плохой ключ:

```text
user@example.com
```

Проблемы:

- персональные данные в логах;
- возможные коллизии;
- непонятно, какая именно операция имеется в виду.

Лучше:

```text
payment_attempt_id = 7d68b4fd-0df6-4518-9f42-b1cb6e0f19a1
```

Еще лучше, если ключ связан с бизнесовой записью:

```text
order_id = ord_100
payment_attempt_id = payatt_001
provider_idempotency_key = payatt_001
```

Тогда один и тот же идентификатор виден:

- в базе;
- в логах;
- в trace;
- во внешнем платежном provider-е;
- в support tooling.

## Серверный алгоритм

Типичная таблица:

```sql
CREATE TABLE idempotency_records (
	scope text NOT NULL,
	idempotency_key text NOT NULL,
	request_hash text NOT NULL,
	status text NOT NULL,
	response_status int,
	response_body jsonb,
	resource_type text,
	resource_id text,
	error_code text,
	created_at timestamptz NOT NULL DEFAULT now(),
	updated_at timestamptz NOT NULL DEFAULT now(),
	expires_at timestamptz,
	PRIMARY KEY (scope, idempotency_key)
);
```

`scope` нужен, чтобы ключ одного клиента не конфликтовал с ключом другого:

```text
scope = merchant_id + endpoint + operation_type
```

Упрощенный flow:

1. Клиент присылает mutating request с idempotency key.
2. Сервер валидирует базовый формат.
3. Сервер считает canonical request hash.
4. Сервер пытается создать `idempotency_record` со статусом `processing`.
5. Если запись уже есть, сервер сравнивает hash.
6. Если hash отличается, возвращает ошибку payload mismatch.
7. Если статус `completed`, сервер возвращает сохраненный response или семантически эквивалентный результат.
8. Если статус `processing`, сервер возвращает `409 Conflict`, `202 Accepted` или ждет завершения, в зависимости от контракта.
9. Если запись новая, сервер выполняет бизнес-операцию.
10. Сервер атомарно сохраняет результат и переводит запись в `completed`.

Псевдокод:

```go
type CreatePaymentCommand struct {
	MerchantID      string
	OrderID         string
	AmountCents     int64
	Currency        string
	IdempotencyKey string
}

func (s *PaymentService) CreatePayment(ctx context.Context, cmd CreatePaymentCommand) (PaymentResult, error) {
	hash := canonicalHash(cmd.OrderID, cmd.AmountCents, cmd.Currency)
	scope := "merchant:" + cmd.MerchantID + ":payments:create"

	return s.tx.Run(ctx, func(ctx context.Context, tx Tx) (PaymentResult, error) {
		record, created, err := s.idempotency.Begin(ctx, tx, scope, cmd.IdempotencyKey, hash)
		if err != nil {
			return PaymentResult{}, err
		}

		if !created {
			return s.handleExistingRecord(ctx, tx, record, hash)
		}

		payment, err := s.payments.CreateAttempt(ctx, tx, cmd)
		if err != nil {
			_ = s.idempotency.MarkFailed(ctx, tx, scope, cmd.IdempotencyKey, err)
			return PaymentResult{}, err
		}

		result := PaymentResult{PaymentID: payment.ID, Status: payment.Status}
		if err := s.idempotency.MarkCompleted(ctx, tx, scope, cmd.IdempotencyKey, result); err != nil {
			return PaymentResult{}, err
		}

		return result, nil
	})
}
```

Этот пример намеренно упрощен. В реальном платежном flow внешний вызов к provider-у не стоит делать внутри долгой DB-транзакции. Часто лучше создать локальную `payment_attempt` в `pending`, закоммитить, а сам provider call выполнить отдельным шагом с тем же idempotency key и явной статусной моделью. Но идея остается той же: duplicate identity должна быть сохранена durable и проверяться атомарно с критичными изменениями.

## Самое опасное место: check-then-act

Наивная реализация:

```go
exists, _ := repo.IdempotencyKeyExists(ctx, key)
if exists {
	return previousResult, nil
}

payment, err := provider.Charge(ctx, amount)
if err != nil {
	return err
}

repo.SaveIdempotencyKey(ctx, key, payment.ID)
```

Проблема: два одинаковых запроса могут прийти параллельно.

```text
request A: exists=false
request B: exists=false
request A: Charge
request B: Charge
request A: SaveKey
request B: SaveKey
```

Получили двойное списание.

Нужно не "сначала проверить, потом сделать", а атомарно захватить право на выполнение:

```sql
INSERT INTO idempotency_records (scope, idempotency_key, request_hash, status)
VALUES ($1, $2, $3, 'processing')
ON CONFLICT DO NOTHING;
```

Если insert сработал, этот request владеет обработкой. Если нет, это duplicate или concurrent retry.

Для критичных операций важно, чтобы unique constraint был в надежном хранилище, а не только в локальной памяти процесса.

## Что возвращать на повтор

Есть несколько стратегий.

### 1. Replay сохраненного ответа

Первый запрос:

```http
201 Created
{
  "payment_id": "pay_123",
  "status": "succeeded"
}
```

Повтор:

```http
201 Created
{
  "payment_id": "pay_123",
  "status": "succeeded"
}
```

Плюсы:

- клиентский код простой;
- retry прозрачен;
- хорошая совместимость с SDK.

Минусы:

- надо хранить response;
- надо решить, хранить ли ошибки;
- response может содержать устаревшие поля, если ресурс позже изменился.

Stripe, например, документирует сохранение status code и body результата первой начавшейся обработки, включая ошибки `500`. Это строгий контракт, но он требует дисциплины на сервере.

### 2. Вернуть текущий статус ресурса

Первый запрос создал payment attempt. Повтор возвращает текущий статус:

```http
200 OK
{
  "payment_id": "pay_123",
  "status": "succeeded"
}
```

Плюсы:

- клиент видит актуальное состояние;
- удобно для long-running операций.

Минусы:

- ответ не идентичен первому;
- клиент должен понимать семантику.

PayPal в документации описывает возврат latest status предыдущего запроса для повторов с тем же `PayPal-Request-Id`.

### 3. Вернуть ссылку на status resource

Для долгих операций:

```http
202 Accepted
Location: /operations/op_123
```

Повтор с тем же ключом возвращает ту же operation:

```http
202 Accepted
Location: /operations/op_123
```

Это часто хороший вариант для тяжелых workflow: создание отчета, запуск VM, импорт файла, ML job, массовая операция.

## Payload mismatch

Один из главных edge cases:

```http
POST /payments
Idempotency-Key: abc

{ "amount": 1000 }
```

Потом:

```http
POST /payments
Idempotency-Key: abc

{ "amount": 9999 }
```

Это не retry. Это новый intent со старым ключом.

Сервер не должен молча возвращать старый payment и не должен выполнять новый payment. Обычно нужно вернуть ошибку вроде:

```http
409 Conflict
```

или:

```http
422 Unprocessable Entity
```

Смысл:

> Этот idempotency key уже использовался для другого payload.

Для этого нужен canonical hash. Важно канонизировать payload так, чтобы порядок полей JSON, пробелы и несущественные детали не ломали сравнение.

## TTL и хранение ключей

Idempotency records нельзя хранить бесконечно во всех системах. Обычно задают retention:

- 24 часа для обычных API retries;
- несколько дней для платежей и финансовых операций;
- дольше для бизнес-критичных workflow;
- навсегда для ledger-like операций, если ключ является частью бухгалтерского следа.

TTL - это часть контракта.

Если ключ удален, повтор с тем же ключом может быть принят как новая операция. Это нормально только если клиент и сервер понимают окно гарантий.

В документации Stripe указано, что ключи могут удаляться после минимального окна хранения, и повтор после pruning может создать новую операцию. Поэтому клиент не должен считать idempotency key вечным, если API обещает только ограниченное окно.

## Где хранить ключи

Варианты:

### Реляционная база

Лучший default для операций, которые уже меняют бизнесовые строки в этой базе.

Плюсы:

- unique constraint;
- транзакции;
- можно атомарно связать idempotency record и business state;
- легко расследовать.

Минусы:

- дополнительная запись на каждый mutating request;
- надо чистить старые записи;
- надо аккуратно проектировать индексы.

### Redis или другой быстрый key-value store

Хорош для коротких окон и высоконагруженных API, где бизнесовый риск ниже.

Плюсы:

- быстро;
- TTL из коробки;
- снижает нагрузку на основную БД.

Минусы:

- durability слабее;
- сложнее атомарно связать с DB-изменениями;
- потеря Redis state может открыть окно дублей.

Для платежей и критичных бизнес-операций Redis-only часто недостаточен. Его можно использовать как cache поверх durable record, но не как единственный источник истины.

### Business table

Иногда отдельная таблица не нужна: idempotency key хранится прямо в бизнесовой сущности.

```sql
CREATE TABLE orders (
	id bigserial PRIMARY KEY,
	merchant_id bigint NOT NULL,
	client_order_id text NOT NULL,
	status text NOT NULL,
	UNIQUE (merchant_id, client_order_id)
);
```

Если `client_order_id` и есть идемпотентный business id, повтор `create order` вернет тот же order.

Это часто самый чистый дизайн: вместо технического ключа есть настоящий business operation id.

## State machine как основа идемпотентности

Идемпотентность легче проектировать, когда бизнес-процесс выражен статусами.

Плохо:

```go
func PayOrder(ctx context.Context, orderID string) error {
	chargeCard()
	sendEmail()
	publishEvent()
	return nil
}
```

Непонятно:

- что уже случилось;
- что можно повторить;
- где остановиться после падения;
- как отличить retry от новой операции.

Лучше:

```text
created
payment_pending
payment_started
payment_succeeded
paid
payment_failed
cancelled
```

И переходы:

```sql
UPDATE orders
SET status = 'payment_started'
WHERE id = $1
  AND status = 'payment_pending';
```

Если запрос повторился, а заказ уже `payment_started` или `paid`, worker не начинает все заново, а смотрит текущее состояние и продолжает или возвращает результат.

Идемпотентные state transitions обычно выглядят так:

- перейти в конкретный статус;
- создать ресурс с уникальным business id;
- записать результат для конкретной попытки;
- пропустить шаг, если он уже завершен;
- повторить только тот шаг, который явно retryable.

Неидемпотентные transitions:

- `balance = balance - amount` без operation id;
- `attempt_count = attempt_count + 1`, если счетчик влияет на бизнес-решение;
- `send email` без dedup;
- `publish event` без event id;
- `create row` с новым auto-increment id на каждый retry.

## Idempotent consumer

В messaging идемпотентность нужна еще чаще, чем в HTTP.

Большинство брокеров в практических системах дают at-least-once delivery или похожую семантику. Это значит:

> Сообщение будет доставлено, но иногда может быть доставлено больше одного раза.

Дубли возникают, например, так:

1. Consumer получил сообщение.
2. Consumer обновил базу.
3. Consumer упал до ack или commit offset.
4. Broker считает сообщение необработанным.
5. Broker доставляет его снова.

Если handler неидемпотентен:

```go
func HandleAccountDebited(event AccountDebited) error {
	return repo.DecreaseBalance(ctx, event.AccountID, event.Amount)
}
```

дубликат уменьшит баланс дважды.

Idempotent consumer добавляет message identity:

```sql
CREATE TABLE processed_messages (
	consumer_name text NOT NULL,
	message_id text NOT NULL,
	processed_at timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (consumer_name, message_id)
);
```

Обработка:

```sql
BEGIN;

INSERT INTO processed_messages (consumer_name, message_id)
VALUES ('billing_projection', 'evt_123');

-- если duplicate key, значит это повтор: rollback/commit и ничего не делаем

UPDATE account_projection
SET balance = balance - 100
WHERE account_id = 42;

COMMIT;
```

Ключевой момент: запись `processed_messages` и бизнес-изменение должны быть в одной транзакции. Иначе снова появится окно:

- бизнес-изменение применили, но message id не записали;
- message id записали, но бизнес-изменение не применили.

Если отдельная транзакция невозможна, нужны другие механизмы: idempotent business state, external dedup, transactional consumer API, reconciliation.

## Event design для идемпотентности

Consumer-у проще быть идемпотентным, если событие несет правильную информацию.

Плохо:

```json
{
  "type": "BalanceChanged",
  "account_id": "acc_1",
  "delta": -100
}
```

Если применить дважды, баланс уменьшится дважды.

Лучше:

```json
{
  "event_id": "evt_123",
  "type": "AccountDebited",
  "account_id": "acc_1",
  "operation_id": "op_777",
  "amount": 100,
  "balance_after": 900,
  "version": 12
}
```

Теперь consumer может:

- дедуплицировать по `event_id`;
- дедуплицировать бизнес-операцию по `operation_id`;
- проверить `version`;
- построить projection через `balance_after`, если это подходит модели;
- расследовать повтор в логах.

Не всегда можно включить `balance_after`; для ledger-систем часто важнее append-only записи. Но event id, operation id и version почти всегда полезны.

## Outbox и inbox

Идемпотентность тесно связана с transactional outbox.

Проблема:

```go
tx.SaveOrder(order)
tx.Commit()
broker.Publish(OrderCreated{OrderID: order.ID})
```

Если процесс упал после commit, но до publish, заказ есть, события нет.

Outbox:

```sql
BEGIN;

INSERT INTO orders (...);

INSERT INTO outbox_events (event_id, aggregate_id, event_type, payload, status)
VALUES ('evt_123', 'ord_100', 'OrderCreated', '{...}', 'pending');

COMMIT;
```

Отдельный worker читает outbox и публикует события.

Но outbox обычно дает at-least-once publication:

1. Worker публикует событие.
2. Broker принимает.
3. Worker падает до отметки `published`.
4. Worker публикует снова.

Поэтому consumer-ы должны быть идемпотентными.

Inbox - симметричный паттерн на стороне приема: сохранить входящее сообщение или operation id и обработать его один раз.

Вместе:

- outbox защищает "локальная транзакция + публикация";
- inbox защищает consumer от повторной доставки;
- idempotency key защищает synchronous API от повторного вызова;
- reconciliation чинит то, что все равно разошлось.

## Платежи

Платежи - классический пример, потому что ошибка сразу дорогая.

Плохой flow:

```text
POST /pay
server -> provider.Charge(card, 1000)
timeout
client retries POST /pay
server -> provider.Charge(card, 1000)
```

Риск: два списания.

Более надежный flow:

```text
client creates payment_attempt_id
client POST /payments with Idempotency-Key = payment_attempt_id
server creates local payment_attempt pending
server calls provider with provider idempotency key = payment_attempt_id
server stores provider result
client retries with same key if timeout
server returns existing attempt/result
```

Важные детали:

- ключ должен быть одинаковым при всех retries одной попытки;
- новая попытка оплаты должна получить новый ключ;
- сумма, валюта, order id и merchant id должны проверяться на mismatch;
- provider idempotency key должен быть связан с локальной записью;
- callback/webhook от provider-а тоже должен быть идемпотентным;
- ручная reconciliation должна уметь сопоставить локальный attempt и provider transaction.

На ревью платежного кода вопрос номер один:

> Что будет, если timeout случился после успешного списания, но до ответа клиенту?

Если ответа нет, нужна идемпотентность.

## HTTP examples

### `PUT` как замена состояния

```http
PUT /users/42/email
Content-Type: application/json

{ "email": "new@example.com" }
```

Повтор задает тот же email.

### `PATCH` может быть идемпотентным или нет

Неидемпотентный patch:

```http
PATCH /accounts/42
Content-Type: application/json

{ "op": "increment", "field": "balance", "value": 100 }
```

Повтор увеличит баланс снова.

Идемпотентный patch:

```http
PATCH /users/42
Content-Type: application/json

{ "email": "new@example.com" }
```

Если контракт именно "установить email в значение", повтор безопасен.

### `POST` может быть идемпотентным по контракту

HTTP не гарантирует идемпотентность `POST` по умолчанию, но конкретный endpoint может сделать ее через ключ:

```http
POST /orders
Idempotency-Key: ord-client-778
```

Важно не говорить "POST всегда неидемпотентен". Точнее так:

> `POST` не считается идемпотентным по стандартной семантике HTTP, но конкретный API может определить идемпотентный контракт для `POST` через idempotency key или business id.

## Database patterns

### Unique constraint

Самый простой и надежный инструмент.

```sql
CREATE UNIQUE INDEX uniq_order_client_id
ON orders (merchant_id, client_order_id);
```

Повтор:

```sql
INSERT INTO orders (merchant_id, client_order_id, amount_cents)
VALUES (1, 'client-ord-123', 1000)
ON CONFLICT (merchant_id, client_order_id)
DO UPDATE SET updated_at = orders.updated_at
RETURNING id, status;
```

Можно вернуть существующий order.

### Idempotent update

```sql
UPDATE orders
SET status = 'cancelled'
WHERE id = $1
  AND status IN ('created', 'payment_failed');
```

Повтор либо ничего не изменит, либо вернет текущий статус.

### Operation ledger

Для денег часто лучше хранить операции append-only:

```sql
CREATE TABLE ledger_entries (
	id bigserial PRIMARY KEY,
	account_id bigint NOT NULL,
	operation_id text NOT NULL,
	amount_cents bigint NOT NULL,
	created_at timestamptz NOT NULL DEFAULT now(),
	UNIQUE (account_id, operation_id)
);
```

Повтор той же `operation_id` не создаст вторую ledger entry.

Важно: ledger entry сама по себе append-only, но операция создания entry идемпотентна по `operation_id`.

## Ошибки проектирования

### Ошибка 1. "У нас retry только один раз"

Один повтор уже достаточно, чтобы создать двойной эффект.

### Ошибка 2. Генерировать idempotency key на сервере после получения запроса

Если клиент при retry получает новый key, сервер не сможет связать повторы.

Сервер может сгенерировать operation id только если первый ответ гарантированно дошел до клиента, а это как раз не гарантировано при timeout. Для внешнего retry ключ обычно должен быть у клиента или у верхнего orchestrator-а.

### Ошибка 3. Использовать новый key при каждом retry

```text
attempt 1: Idempotency-Key = uuid-1
attempt 2: Idempotency-Key = uuid-2
```

Для сервера это две разные операции.

### Ошибка 4. Не сравнивать payload

Повтор с тем же ключом, но другой суммой, нельзя считать тем же запросом.

### Ошибка 5. Сохранять ключ после side effect

Если side effect случился, а процесс упал до сохранения ключа, повтор создаст второй side effect.

### Ошибка 6. Делать dedup в памяти процесса

In-memory map защищает только один процесс и только до рестарта.

```go
seen := map[string]bool{}
```

Для production-дублей из сети, брокера или нескольких replicas этого недостаточно.

### Ошибка 7. Считать `GET` безопасным автоматически

Если handler на `GET` меняет состояние, отправляет письма или запускает операции, он нарушает ожидания HTTP.

### Ошибка 8. Думать, что идемпотентность отменяет observability

Дубликаты надо видеть:

- сколько duplicate requests;
- сколько payload mismatches;
- сколько concurrent duplicates;
- сколько expired key reuse;
- сколько retries у provider-а;
- сколько сообщений отброшено consumer dedup-ом.

Идемпотентность без наблюдаемости превращается в тихое проглатывание проблем.

### Ошибка 9. Не описать TTL

Если API хранит ключи 24 часа, это часть контракта. Клиент не должен повторять старую операцию через неделю и ожидать тот же результат.

### Ошибка 10. Путать request id и idempotency key

`Request ID` часто нужен для трассировки конкретного HTTP-вызова. При retry он может быть новым.

`Idempotency key` нужен для бизнес-попытки. При retry он должен быть тем же.

Иногда один идентификатор может играть обе роли, но это должно быть осознанно.

## Code review checklist

Когда видишь mutating operation, пройди по вопросам:

1. Может ли вызывающая сторона повторить запрос после timeout?
2. Есть ли retry в SDK, gateway, queue consumer, cron или worker-е?
3. Что является идентификатором бизнес-намерения?
4. Где хранится idempotency key или operation id?
5. Есть ли unique constraint?
6. Атомарна ли запись ключа и бизнес-изменение?
7. Что будет при двух параллельных запросах с одним ключом?
8. Что будет при том же ключе и другом payload?
9. Какой response вернется на повтор?
10. Как долго хранится ключ?
11. Есть ли связь ключа с tenant/user/merchant scope?
12. Не содержит ли ключ PII или секреты?
13. Что происходит с внешними side effects?
14. Идемпотентны ли webhooks и consumers?
15. Есть ли метрики и логи по duplicate/mismatch/replay?
16. Есть ли manual reconciliation для критичных операций?

На ревью можно сказать:

> У этой команды есть внешний side effect и она может быть повторена после timeout, но в контракте нет operation id или idempotency key. Если первый вызов успел выполниться, а ответ потерялся, retry может создать второй эффект. Я бы добавил идемпотентный ключ, unique constraint на уровне бизнес-попытки и явно описал поведение повторов.

## Как отвечать на интервью

### Что такое идемпотентность?

> Идемпотентность - свойство операции, при котором повторное выполнение с тем же намерением не меняет итоговый эффект после первого успешного применения. В backend это нужно для безопасных retries, потому что timeout или lost response не говорят клиенту, была ли операция выполнена.

### Пример идемпотентной и неидемпотентной операции

> `set status = cancelled` обычно идемпотентен: повтор оставляет статус cancelled. `increment balance by 100` или `charge card` без operation id неидемпотентны: каждый повтор создает новый эффект.

### Чем safe отличается от idempotent?

> Safe означает, что клиент не просит менять состояние, например `GET`. Idempotent означает, что повтор не создает дополнительного эффекта. `DELETE` не safe, потому что удаляет, но idempotent, потому что повторное удаление оставляет ресурс удаленным.

### Как сделать `POST /payments` идемпотентным?

> Клиент генерирует idempotency key для payment attempt и повторяет retries с тем же ключом. Сервер хранит ключ в scope клиента или merchant-а, сравнивает payload hash, атомарно связывает ключ с payment attempt, возвращает сохраненный или семантически эквивалентный результат на повтор, а для другого payload с тем же ключом возвращает ошибку mismatch.

### Почему идемпотентность не равна exactly-once?

> Exactly-once в распределенной системе трудно гарантировать end-to-end. Код может физически получить запрос или сообщение больше одного раза. Идемпотентность делает повтор безопасным: дубликат распознается и не создает второй бизнес-эффект.

### Как сделать consumer идемпотентным?

> Добавить message id или operation id и durable dedup. Например, в одной транзакции вставить `(consumer_name, message_id)` в `processed_messages` с unique constraint и выполнить бизнес-изменение. Если insert упал по duplicate key, сообщение уже обработано.

## Историческая шкала

Примерная линия развития:

- 1847: Джордж Буль использует алгебраические идеи, где выражения могут давать себя при степенях.
- 1870: Бенджамин Пирс вводит термин `idempotent` в контексте linear associative algebra.
- Конец XIX - XX век: термин становится обычным в алгебре, матрицах, кольцах, проекторах, closure operations.
- Вторая половина XX века: идея переходит в computer science, functional programming, storage operations, distributed systems.
- 1990-е: HTTP/1.1 закрепляет семантику safe/idempotent/cacheable methods для веба.
- 2000: диссертация Роя Филдинга описывает REST как архитектурный стиль с uniform interface, statelessness, cache и видимой семантикой сообщений.
- 2003: Enterprise Integration Patterns формулирует Idempotent Receiver для messaging.
- 2010-е: микросервисы, cloud SDK retries, message brokers и payment APIs делают идемпотентность повседневной reliability-практикой.
- 2020-е: Stripe, PayPal, AWS, Azure, Kafka и другие платформы явно документируют retry/idempotency contracts; `Idempotency-Key` оформлялся как IETF draft, но по состоянию на 2026-07-22 соответствующий draft истек и не является RFC.

## Связь с другими темами

Идемпотентность почти никогда не живет одна. Она связана с:

- retries and backoff;
- timeouts;
- context cancellation;
- transactions;
- isolation levels;
- unique constraints;
- outbox/inbox;
- sagas;
- message delivery semantics;
- distributed locks;
- optimistic concurrency;
- reconciliation;
- observability.

Если совсем коротко:

> Retry без идемпотентности опасен. Идемпотентность без durable state часто иллюзорна. Durable state без понятного business intent может дедуплицировать не то.

## Источники для дальнейшего чтения

- [MacTutor History of Mathematics: Earliest Known Uses of Some of the Words of Mathematics, IDEMPOTENT](https://mathshistory.st-andrews.ac.uk/Miller/mathword/i/)
- [Online Etymology Dictionary: idempotent](https://www.etymonline.com/word/idempotent)
- [RFC 9110: HTTP Semantics, section 9.2.2 Idempotent Methods](https://datatracker.ietf.org/doc/html/rfc9110#section-9.2.2)
- [Roy Fielding: Chapter 5, Representational State Transfer](https://ics.uci.edu/~fielding/pubs/dissertation/rest_arch_style.htm)
- [AWS Builders' Library: Making retries safe with idempotent APIs](https://aws.amazon.com/builders-library/making-retries-safe-with-idempotent-APIs/)
- [AWS Well-Architected Framework: REL04-BP04 Make all responses idempotent](https://docs.aws.amazon.com/wellarchitected/2024-06-27/framework/rel_prevent_interaction_failure_idempotent.html)
- [Microsoft Azure Architecture Center: Retry pattern](https://learn.microsoft.com/en-us/azure/architecture/patterns/retry)
- [Stripe API docs: Idempotent requests](https://docs.stripe.com/api/idempotent_requests)
- [PayPal REST API docs: Idempotency](https://developer.paypal.com/api/rest/reference/idempotency/)
- [Enterprise Integration Patterns: Idempotent Receiver](https://www.enterpriseintegrationpatterns.com/patterns/messaging/IdempotentReceiver.html)
- [Microservices.io: Idempotent Consumer](https://microservices.io/patterns/communication-style/idempotent-consumer.html)
- [Martin Fowler / Patterns of Distributed Systems: Idempotent Receiver](https://martinfowler.com/articles/patterns-of-distributed-systems/idempotent-receiver.html)
- [Apache Kafka docs: Producer configs, enable.idempotence](https://kafka.apache.org/39/configuration/producer-configs/#producerconfigs_enable.idempotence)
- [IETF Datatracker: expired draft Idempotency-Key HTTP Header Field](https://datatracker.ietf.org/doc/draft-ietf-httpapi-idempotency-key-header/)

