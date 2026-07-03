# PostgreSQL: уровни изоляции, аномалии транзакций и бизнесовые гонки

Этот документ про то, как база данных ведет себя, когда несколько транзакций одновременно читают и меняют одни и те же данные. Это одна из самых важных тем для backend-собеседований, потому что почти все реальные бизнесовые баги вокруг денег, остатков, бронирований, лимитов, статусов и очередей упираются не просто в "транзакции есть", а в вопрос:

> Что именно моя транзакция видит, чего она не видит, с кем она конфликтует, какие аномалии возможны на выбранном уровне изоляции и кто обязан повторить операцию при конфликте.

Официальные источники:

- [PostgreSQL docs: Transaction Isolation](https://www.postgresql.org/docs/current/transaction-iso.html)
- [PostgreSQL docs: Explicit Locking](https://www.postgresql.org/docs/current/explicit-locking.html)
- [PostgreSQL docs: MVCC](https://www.postgresql.org/docs/current/mvcc.html)
- [PostgreSQL docs: SET TRANSACTION](https://www.postgresql.org/docs/current/sql-set-transaction.html)

## Главная мысль

Транзакция дает не магическую безопасность, а конкретный набор гарантий.

Когда мы пишем:

```sql
BEGIN;

SELECT balance
FROM accounts
WHERE id = 1;

UPDATE accounts
SET balance = balance - 100
WHERE id = 1;

COMMIT;
```

мы должны понимать не только то, что `COMMIT` атомарно фиксирует изменения. Нужно понимать, что параллельно могла делать другая транзакция. Она могла читать тот же баланс. Она могла обновлять ту же строку. Она могла вставить новую строку, которая подходит под наш `WHERE`. Она могла изменить набор строк, из которого мы считали сумму. И выбранный уровень изоляции определяет, будет ли это допустимо, будет ли наша транзакция ждать, увидит ли новые данные, получит ли ошибку сериализации или молча зафиксирует результат, который бизнес считает неправильным.

Короткая формулировка для интервью:

> Изоляция транзакций определяет, как параллельные транзакции видят изменения друг друга. В PostgreSQL это построено на MVCC: транзакции читают snapshot данных, а записи создают новые версии строк. На `READ COMMITTED` каждый statement видит новый snapshot, на `REPEATABLE READ` вся транзакция видит один snapshot, а `SERIALIZABLE` добавляет обнаружение опасных read/write зависимостей и может завершать транзакции ошибкой serialization failure. Поэтому корректность бизнес-операции зависит не только от `BEGIN/COMMIT`, но и от уровня изоляции, блокировок, уникальных ограничений, атомарных SQL-операций и retry-логики.

## Простое введение

Представь интернет-магазин. На складе остался один товар:

```text
product_id = 10
stock = 1
```

Два пользователя одновременно нажимают "купить".

Оба запроса приходят почти одновременно.

Первый запрос читает:

```sql
SELECT stock
FROM products
WHERE id = 10;
```

Получает `1`.

Второй запрос делает то же самое и тоже получает `1`.

Если приложение потом отдельно решает "раз stock > 0, можно списать" и делает update, возникает вопрос: что будет при параллельности? Продадим ли мы один товар двум людям? Уйдет ли остаток в `-1`? Получит ли один запрос ошибку? Будет ли второй ждать? Нужно ли брать lock? Достаточно ли `READ COMMITTED`?

Наивное ощущение часто такое: "мы же внутри транзакции, значит безопасно". Но транзакция сама по себе означает только атомарность набора операций и некоторые гарантии видимости. Если код сначала читает состояние, потом на стороне приложения принимает решение, потом пишет, то между чтением и записью мир мог измениться. Именно здесь начинаются аномалии.

## ACID и место изоляции

Транзакции часто объясняют через ACID.

Atomicity означает, что транзакция фиксируется целиком или откатывается целиком. Если мы создали заказ, списали внутренний баланс и записали движение по счету в одной DB-транзакции, то база не оставит половину этих изменений.

Consistency означает, что база переходит из одного согласованного состояния в другое. Но здесь есть тонкость: база обеспечивает свои constraints, например `NOT NULL`, `UNIQUE`, `FOREIGN KEY`, `CHECK`. А бизнесовую согласованность вроде "у пользователя не больше трех активных подписок" нужно выразить ограничениями, блокировками, корректным SQL или транзакционной логикой.

Isolation означает, насколько параллельные транзакции отделены друг от друга. Именно это тема документа.

Durability означает, что после commit изменения переживут сбой, обычно через WAL.

Очень важная мысль: изоляция не равна атомарности. Можно иметь атомарные транзакции, которые при параллельном выполнении дают бизнесово неправильный результат. Например, две транзакции каждая атомарно создают бронирование, но вместе создают пересечение интервалов, если мы не защитились от такой гонки.

## MVCC как основа изоляции PostgreSQL

PostgreSQL не строит изоляцию только на грубых блокировках "читатель заблокировал писателя" или "писатель заблокировал читателя". Он использует MVCC, multiversion concurrency control.

Когда строка обновляется, PostgreSQL обычно не перезаписывает ее на месте. Он создает новую версию строки, а старая версия остается, пока может быть видна старым транзакциям. У версий есть служебные поля вроде `xmin` и `xmax`: какая транзакция создала версию и какая ее заменила или удалила.

Когда транзакция читает данные, она читает не "самое свежее физическое состояние файла", а состояние, видимое ее snapshot. Snapshot отвечает на вопрос: какие транзакции на момент снимка уже были committed, какие еще выполнялись, какие начались позже.

Именно snapshot объясняет большинство нюансов:

```text
T1 start
T1 snapshot sees balance = 100

T2 update balance = 50
T2 commit

T1 повторно читает balance
```

На `READ COMMITTED` второй statement T1 может увидеть `50`, потому что каждый statement получает новый snapshot.

На `REPEATABLE READ` T1 продолжит видеть `100`, потому что snapshot один на всю транзакцию.

На `SERIALIZABLE` T1 тоже работает со snapshot, но PostgreSQL дополнительно отслеживает зависимости между транзакциями и может сказать: "такой набор параллельных действий невозможно представить как корректное последовательное выполнение", после чего одна транзакция получит ошибку.

## ANSI-уровни и PostgreSQL

SQL-стандарт описывает четыре уровня:

- `READ UNCOMMITTED`;
- `READ COMMITTED`;
- `REPEATABLE READ`;
- `SERIALIZABLE`.

Но PostgreSQL реализует их со своими важными особенностями.

В PostgreSQL `READ UNCOMMITTED` фактически работает как `READ COMMITTED`. Грязных чтений PostgreSQL не допускает из-за MVCC.

`READ COMMITTED` - уровень по умолчанию. На нем каждый statement видит snapshot на начало этого statement.

`REPEATABLE READ` в PostgreSQL сильнее, чем минимально требуется стандартом: он не допускает phantom reads в обычном смысле, потому что транзакция видит один snapshot.

`SERIALIZABLE` в PostgreSQL - это не просто "REPEATABLE READ плюс больше lock-ов на все". Он реализован через Serializable Snapshot Isolation. Транзакции читают snapshot, но PostgreSQL отслеживает опасные зависимости и может завершить одну из транзакций ошибкой `could not serialize access...`. Приложение должно повторить транзакцию.

## Аномалии: зачем они нужны

Аномалия - это ситуация, когда параллельное выполнение транзакций дает результат, который не соответствует ожидаемой бизнесовой логике или не эквивалентен некоторому последовательному порядку выполнения.

На собеседовании часто спрашивают названия: dirty read, non-repeatable read, phantom read, lost update, write skew. Но знать названия мало. Нужно уметь увидеть это в коде:

```go
balance := repo.GetBalance(ctx, userID)
if balance < amount {
	return ErrInsufficientFunds
}
repo.UpdateBalance(ctx, userID, balance-amount)
```

Это выглядит логично, пока не появятся два параллельных запроса.

Ниже будем идти от простых аномалий к более неприятным.

## Dirty read

Dirty read - это когда транзакция читает изменения другой транзакции, которые еще не закоммичены.

Представим:

```sql
-- T1
BEGIN;
UPDATE accounts
SET balance = 0
WHERE id = 1;
-- commit еще не было
```

Если T2 могла бы увидеть `balance = 0`, а потом T1 сделала бы rollback, T2 приняла бы решение на основе данных, которых в реальности никогда не было.

В PostgreSQL dirty read нет даже на `READ UNCOMMITTED`, потому что из-за MVCC незакоммиченные версии других транзакций не видны.

Для интервью:

> Dirty read в PostgreSQL практически не возникает: `READ UNCOMMITTED` мапится на `READ COMMITTED`, незакоммиченные изменения других транзакций не видны.

## Non-repeatable read

Non-repeatable read - это когда внутри одной транзакции мы дважды читаем одну и ту же строку и получаем разные значения, потому что другая транзакция между чтениями обновила и закоммитила строку.

Пример:

```sql
-- T1
BEGIN ISOLATION LEVEL READ COMMITTED;

SELECT balance
FROM accounts
WHERE id = 1;
-- 100
```

Параллельно:

```sql
-- T2
BEGIN;
UPDATE accounts
SET balance = 50
WHERE id = 1;
COMMIT;
```

T1 продолжает:

```sql
SELECT balance
FROM accounts
WHERE id = 1;
-- 50 на READ COMMITTED

COMMIT;
```

На `READ COMMITTED` это нормально, потому что каждый statement получает новый snapshot.

На `REPEATABLE READ` T1 продолжила бы видеть `100`.

Бизнесовый пример: сервис строит счет-фактуру. В начале транзакции он прочитал тариф клиента, потом прочитал его еще раз при расчете скидки, а между чтениями тариф поменялся. На `READ COMMITTED` два чтения могут увидеть разные версии. Это не всегда баг, но если операция требует консистентной картины, нужно либо `REPEATABLE READ`, либо один запрос, либо явные lock-и, либо версионирование бизнесовых данных.

## Phantom read

Phantom read - это когда транзакция повторяет запрос по условию и во второй раз получает другой набор строк, потому что другая транзакция вставила, удалила или изменила строки, подходящие под условие.

Пример:

```sql
-- T1
BEGIN ISOLATION LEVEL READ COMMITTED;

SELECT count(*)
FROM appointments
WHERE doctor_id = 7
  AND starts_at >= '2026-07-01 10:00'
  AND starts_at <  '2026-07-01 11:00';
-- 0
```

Параллельно:

```sql
-- T2
INSERT INTO appointments(doctor_id, starts_at, ends_at)
VALUES (7, '2026-07-01 10:15', '2026-07-01 10:45');
COMMIT;
```

T1 повторяет:

```sql
SELECT count(*)
FROM appointments
WHERE doctor_id = 7
  AND starts_at >= '2026-07-01 10:00'
  AND starts_at <  '2026-07-01 11:00';
-- 1 на READ COMMITTED
```

Это phantom.

В PostgreSQL на `REPEATABLE READ` транзакция видит один snapshot, поэтому такой повторный query не увидит новую строку. В документации PostgreSQL прямо важно: его `REPEATABLE READ` не допускает phantom reads, хотя SQL-стандарт в таблице аномалий допускает phantom на Repeatable Read как минимальную модель.

Но "не увидеть phantom" не значит "бизнес-инвариант защищен". Если две транзакции одновременно видят, что пересечений нет, и обе вставляют новые строки, они могут создать конфликтный набор. Это уже ближе к write skew и predicate problem.

## Lost update

Lost update - это когда две транзакции читают одно значение, обе вычисляют новое значение, и одна запись затирает результат другой.

Плохой код:

```sql
-- T1
SELECT balance FROM accounts WHERE id = 1; -- 100
-- приложение считает 100 - 10 = 90

-- T2
SELECT balance FROM accounts WHERE id = 1; -- 100
-- приложение считает 100 - 20 = 80

-- T1
UPDATE accounts SET balance = 90 WHERE id = 1;
COMMIT;

-- T2
UPDATE accounts SET balance = 80 WHERE id = 1;
COMMIT;
```

Итог `80`, хотя если оба списания должны примениться, должен быть `70`. Обновление T1 потеряно.

В PostgreSQL поведение зависит от формы update и уровня изоляции. Если писать атомарно:

```sql
UPDATE accounts
SET balance = balance - 10
WHERE id = 1;
```

то второй update будет работать с актуальной версией строки после ожидания lock-а, и оба списания применятся.

А если приложение сначала читает значение, считает в памяти и потом пишет абсолютное значение, оно само создает риск.

Правильнее:

```sql
UPDATE accounts
SET balance = balance - $1
WHERE id = $2
  AND balance >= $1
RETURNING balance;
```

Это одна атомарная операция. Если строка не вернулась, денег не хватило.

На интервью:

> Lost update часто лечится не повышением изоляции в первую очередь, а правильной формой SQL: атомарный `UPDATE ... SET x = x + delta WHERE condition RETURNING ...`, optimistic locking через version column или `SELECT FOR UPDATE`, если нужна сложная логика.

## Write skew

Write skew - одна из самых неприятных аномалий, потому что каждая транзакция меняет разные строки, прямого write/write конфликта нет, но вместе они ломают инвариант.

Классический пример: в больнице должен быть хотя бы один дежурный врач.

Таблица:

```sql
CREATE TABLE doctor_shifts (
	doctor_id bigint PRIMARY KEY,
	on_call boolean NOT NULL
);
```

Данные:

```text
doctor 1 on_call = true
doctor 2 on_call = true
```

Бизнес-правило:

> В любой момент хотя бы один врач должен остаться `on_call = true`.

Две транзакции одновременно хотят снять врачей с дежурства.

T1:

```sql
BEGIN ISOLATION LEVEL REPEATABLE READ;

SELECT count(*)
FROM doctor_shifts
WHERE on_call = true;
-- 2

UPDATE doctor_shifts
SET on_call = false
WHERE doctor_id = 1;

COMMIT;
```

T2 одновременно:

```sql
BEGIN ISOLATION LEVEL REPEATABLE READ;

SELECT count(*)
FROM doctor_shifts
WHERE on_call = true;
-- 2

UPDATE doctor_shifts
SET on_call = false
WHERE doctor_id = 2;

COMMIT;
```

Каждая транзакция видела двух дежурных и решила, что снять одного безопасно. Они обновили разные строки, поэтому прямого row-level конфликта нет. Итог:

```text
doctor 1 on_call = false
doctor 2 on_call = false
```

Инвариант сломан.

`REPEATABLE READ` не обязательно спасает от write skew, потому что snapshot стабилен, но он не превращает автоматически все предикаты в блокировки будущих изменений.

`SERIALIZABLE` в PostgreSQL должен обнаружить опасную структуру зависимостей и одну транзакцию завершить serialization failure. Но тогда приложение должно повторить транзакцию.

Другие решения:

- выразить инвариант через constraint, если возможно;
- взять явный lock на общий ресурс;
- использовать advisory lock на `shift_id`;
- сделать одну строку-агрегат, которую обе транзакции обновляют;
- использовать `SERIALIZABLE` с retry.

## Read skew

Read skew - это когда транзакция читает несколько связанных значений, но они относятся к разным моментам времени.

Пример: перевод между счетами.

Есть два счета:

```text
A = 100
B = 100
total = 200
```

T2 переводит 50 с A на B:

```sql
BEGIN;
UPDATE accounts SET balance = balance - 50 WHERE id = 'A';
UPDATE accounts SET balance = balance + 50 WHERE id = 'B';
COMMIT;
```

T1 на `READ COMMITTED` строит отчет:

```sql
BEGIN ISOLATION LEVEL READ COMMITTED;

SELECT balance FROM accounts WHERE id = 'A'; -- может увидеть 50 после первого изменения/commit
SELECT balance FROM accounts WHERE id = 'B'; -- может увидеть старое или новое в зависимости от timing

COMMIT;
```

В PostgreSQL изменения внутри другой транзакции видны только после commit, но на `READ COMMITTED` разные statements могут видеть разные snapshots. Если отчет состоит из нескольких запросов, он может получить не единую картину времени, а смесь committed-состояний.

Для консистентного отчета лучше:

- один SQL-запрос;
- `REPEATABLE READ READ ONLY`;
- replica/analytics;
- materialized snapshot.

## Уровень READ COMMITTED под капотом

`READ COMMITTED` - дефолт в PostgreSQL.

Его удобно понимать так:

> Каждый SQL statement видит снимок базы на начало этого statement.

Если внутри транзакции три запроса:

```sql
BEGIN;
SELECT ...; -- snapshot A
SELECT ...; -- snapshot B
UPDATE ...; -- snapshot C для поиска строк + особые правила update conflicts
COMMIT;
```

то каждый может видеть новые committed-изменения других транзакций.

Для простых CRUD-операций это удобно. Мы не держим длинный устаревший snapshot, меньше конфликтов, меньше serialization failures, поведение обычно понятно.

Но есть важный нюанс с `UPDATE`, `DELETE`, `SELECT FOR UPDATE`. Если statement нашел строку, но она уже обновляется другой транзакцией, PostgreSQL будет ждать. После того как другая транзакция commit-ится, PostgreSQL перепроверит условие `WHERE` на новой версии строки. Если новая версия все еще подходит, операция применится к новой версии.

Пример:

```sql
-- T1
BEGIN;
UPDATE products
SET stock = stock - 1
WHERE id = 10
  AND stock > 0;
-- держит lock, еще не commit
```

T2:

```sql
BEGIN;
UPDATE products
SET stock = stock - 1
WHERE id = 10
  AND stock > 0;
-- ждет T1
```

Если T1 закоммитила и stock стал `0`, T2 перепроверит `stock > 0` и не обновит строку.

Вот почему хороший паттерн списания склада:

```sql
UPDATE products
SET stock = stock - 1
WHERE id = $1
  AND stock > 0
RETURNING stock;
```

работает надежнее, чем "сначала SELECT, потом UPDATE абсолютным значением".

## Где READ COMMITTED хорош

`READ COMMITTED` хорош для большинства коротких OLTP-операций, если сами SQL-запросы написаны атомарно.

Например:

```sql
UPDATE accounts
SET balance = balance - $amount
WHERE id = $account_id
  AND balance >= $amount
RETURNING balance;
```

Это безопаснее, чем повышать изоляцию и оставлять read-modify-write в приложении.

Хороший backend-код часто строится не на максимальной изоляции, а на том, что бизнесовая операция выражена одним SQL или набором SQL с правильными constraints/locks.

## Где READ COMMITTED опасен

Он опасен, когда операция делает несколько чтений и принимает решение на основе "картины мира", которая должна быть цельной.

Примеры:

- проверить сумму заказов пользователя за день и создать новый заказ, если лимит не превышен;
- проверить, что в интервале нет бронирований, и вставить новое;
- проверить, что у врача остается другой дежурный, и снять текущего;
- проверить, что активных подписок меньше трех, и добавить новую;
- построить отчет несколькими SELECT-ами;
- проверить отсутствие строки, потом вставить, если нет unique constraint.

Во всех этих случаях нужно думать: какой именно инвариант защищаем? Можно ли выразить его constraint-ом? Нужен ли lock? Нужен ли `SERIALIZABLE`? Можно ли сделать один атомарный `INSERT ... ON CONFLICT`?

## REPEATABLE READ под капотом

В PostgreSQL `REPEATABLE READ` означает:

> Первый statement транзакции устанавливает snapshot, и вся транзакция читает этот snapshot.

Пример:

```sql
BEGIN ISOLATION LEVEL REPEATABLE READ;

SELECT balance FROM accounts WHERE id = 1; -- 100

-- другая транзакция обновила balance до 50 и commit

SELECT balance FROM accounts WHERE id = 1; -- все еще 100

COMMIT;
```

Это удобно, когда нужна консистентная картина данных. Например, отчет должен считать несколько таблиц на один момент времени.

Но при попытке обновить строку, которую параллельно обновила другая транзакция, можно получить ошибку:

```text
ERROR: could not serialize access due to concurrent update
```

Название ошибки может смущать: она может возникать и на `REPEATABLE READ`. Смысл в том, что транзакция не может корректно применить update к строке, версия которой изменилась после ее snapshot.

На `READ COMMITTED` PostgreSQL чаще перепроверит новую версию и продолжит. На `REPEATABLE READ` он не может просто переключиться на новую committed-версию, потому что это нарушило бы единый snapshot транзакции.

## Где REPEATABLE READ хорош

Он хорош для консистентных read-only операций.

Например, нужно построить отчет:

```sql
BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;

SELECT ... FROM orders ...;
SELECT ... FROM payments ...;
SELECT ... FROM refunds ...;

COMMIT;
```

Все запросы видят одну картину базы.

Еще он полезен, если бизнес-операция должна работать с неизменной картиной, но нужно быть готовым к конфликтам при записи.

Однако `REPEATABLE READ` не является универсальной заменой `SERIALIZABLE`. Он не гарантирует, что результат параллельных транзакций эквивалентен некоторому последовательному порядку. Write skew остается важным примером.

## SERIALIZABLE под капотом

`SERIALIZABLE` - самый сильный уровень изоляции в PostgreSQL.

Идея:

> Результат успешно закоммиченных serializable-транзакций должен быть таким, как будто они выполнились последовательно в некотором порядке.

Но PostgreSQL не блокирует грубо все чтения и записи. Он использует Serializable Snapshot Isolation. Транзакции читают snapshot, как при snapshot isolation, но база дополнительно отслеживает read/write зависимости.

Если T1 прочитала набор строк, T2 изменила то, что могло повлиять на это чтение, T3 еще как-то связана с T1/T2, может возникнуть dangerous structure. PostgreSQL понимает, что все эти транзакции нельзя безопасно упорядочить последовательно, и abort-ит одну из них.

Ошибка обычно выглядит так:

```text
ERROR: could not serialize access due to read/write dependencies among transactions
```

или похожим образом.

Для приложения это не "фатальная ошибка". Это сигнал:

> Транзакцию нужно повторить целиком.

На `SERIALIZABLE` retry - часть нормального дизайна.

## Predicate locks в PostgreSQL

В документации PostgreSQL для serializable говорится о predicate locking. Важно понимать осторожно: это не всегда блокировки в привычном смысле, которые мешают другим писать. В PostgreSQL они часто используются как SIREAD locks для отслеживания зависимостей.

Когда serializable-транзакция читает:

```sql
SELECT *
FROM appointments
WHERE doctor_id = 7
  AND starts_at < $end
  AND ends_at > $start;
```

PostgreSQL должен помнить не только конкретные прочитанные строки, но и сам факт чтения диапазона/предиката. Если другая транзакция вставит строку, которая попала бы в этот диапазон, это может создать read/write dependency.

Если есть хороший индекс под предикат, PostgreSQL может точнее отслеживать диапазон. Если индекса нет, tracking может быть грубее, например на уровне relation/page, что повышает шанс serialization failures.

Инженерный вывод:

> На SERIALIZABLE хорошие индексы важны не только для скорости, но и для точности отслеживания конфликтов. Плохие планы и seq scan могут приводить к более грубым predicate locks и большему числу abort-ов.

## Почему SERIALIZABLE не надо включать бездумно

Кажется, что самый сильный уровень всегда лучше. На практике нет.

`SERIALIZABLE` дает сильные гарантии, но:

- транзакции могут чаще падать с serialization failure;
- приложение обязано иметь retry;
- длинные транзакции повышают риск конфликтов;
- плохие индексы могут ухудшить ситуацию;
- нагрузка на отслеживание зависимостей выше;
- пользователю нужно аккуратно проектировать idempotency и side effects.

Особенно опасно делать внешние side effects внутри serializable-транзакции. Если транзакция потом получила serialization failure, DB-изменения откатились, а внешний мир уже изменился. Поэтому платежи, письма, брокеры, HTTP-вызовы нужно проектировать через outbox/idempotency, а не просто "обернем в SERIALIZABLE".

## READ ONLY и DEFERRABLE

В PostgreSQL есть полезный режим для serializable read-only транзакций:

```sql
BEGIN ISOLATION LEVEL SERIALIZABLE READ ONLY DEFERRABLE;
```

Идея: транзакция может подождать в начале, пока PostgreSQL не сможет дать ей snapshot, который безопасен с точки зрения сериализации. После этого read-only транзакция может выполняться без риска serialization failure.

Это полезно для долгих консистентных отчетов, но не для обычных коротких API-запросов.

## SELECT FOR UPDATE

Часто бизнесовую гонку решают не повышением уровня изоляции, а явным row lock.

```sql
BEGIN;

SELECT balance
FROM accounts
WHERE id = 1
FOR UPDATE;

UPDATE accounts
SET balance = balance - 100
WHERE id = 1;

COMMIT;
```

`FOR UPDATE` блокирует выбранные строки для конкурентных update/delete/select for update. Если другая транзакция попытается взять тот же lock, она будет ждать.

Это хорошо, когда инвариант привязан к конкретной существующей строке.

Например:

- списать баланс одного аккаунта;
- изменить статус одного заказа;
- взять задачу из очереди;
- обновить профиль пользователя.

Но `FOR UPDATE` не блокирует "отсутствие строки".

Если ты делаешь:

```sql
SELECT *
FROM bookings
WHERE room_id = 10
  AND starts_at < $end
  AND ends_at > $start
FOR UPDATE;
```

и строк нет, ты ничего не заблокировал. Другая транзакция может вставить пересекающееся бронирование. Для таких случаев нужны constraint-ы, predicate protection через SERIALIZABLE, advisory locks или lock на родительскую строку/ресурс.

## SELECT FOR SHARE, FOR NO KEY UPDATE, FOR KEY SHARE

В PostgreSQL есть несколько режимов row-level locks.

`FOR UPDATE` - самый привычный: хотим обновлять/удалять строку.

`FOR NO KEY UPDATE` слабее: используется, когда обновление не меняет key columns, влияющие на foreign key.

`FOR SHARE` и `FOR KEY SHARE` используются для более мягких сценариев, например когда нужно защититься от удаления/изменения ключа, но не обязательно блокировать все update.

На собеседовании обычно достаточно уверенно знать `FOR UPDATE`, но для middle+ полезно сказать:

> В PostgreSQL row-level locks имеют несколько режимов. Я бы выбирал минимально достаточный, но в прикладном коде чаще всего явно встречается `FOR UPDATE`, `FOR UPDATE SKIP LOCKED` для очередей и иногда `FOR SHARE`.

## SKIP LOCKED и очереди

Очереди часто делают так:

```sql
BEGIN;

SELECT id
FROM jobs
WHERE state = 'ready'
ORDER BY available_at
LIMIT 100
FOR UPDATE SKIP LOCKED;

-- update selected jobs to processing

COMMIT;
```

`SKIP LOCKED` говорит: если строка уже заблокирована другой транзакцией, не ждать ее, а пропустить.

Это удобно для worker pool: несколько воркеров не берут одни и те же job.

Но есть нюансы:

- порядок может быть не идеально честным;
- залоченные старые job могут пропускаться;
- транзакция должна быть короткой;
- нельзя держать lock во время внешней работы;
- нужен lease/timeout, если worker умер.

Хороший паттерн: быстро взять jobs и пометить `processing`, commit, потом выполнять работу вне транзакции.

## Optimistic locking

Optimistic locking часто делают через колонку `version`.

```sql
CREATE TABLE accounts (
	id bigint PRIMARY KEY,
	balance numeric NOT NULL,
	version bigint NOT NULL DEFAULT 1
);
```

Приложение читает:

```sql
SELECT balance, version
FROM accounts
WHERE id = $1;
```

Потом обновляет:

```sql
UPDATE accounts
SET balance = $new_balance,
    version = version + 1
WHERE id = $id
  AND version = $old_version;
```

Если `rows affected = 0`, кто-то изменил строку между чтением и записью. Нужно перечитать и повторить или вернуть conflict.

Это хорошо для:

- UI editing;
- документов;
- профилей;
- настроек;
- API, где пользователь редактирует старую версию.

Но для счетчиков и балансов часто лучше атомарный `UPDATE balance = balance + delta`.

## Constraints как защита от гонок

Лучший lock - тот, который не нужно писать руками, потому что инвариант выражен в базе.

### Уникальность

Плохо:

```sql
SELECT 1 FROM users WHERE email = $1;
-- если нет, insert
INSERT INTO users(email) VALUES ($1);
```

Две транзакции могут одновременно увидеть, что email свободен.

Правильно:

```sql
ALTER TABLE users ADD CONSTRAINT users_email_key UNIQUE (email);
```

И:

```sql
INSERT INTO users(email)
VALUES ($1)
ON CONFLICT (email) DO NOTHING;
```

### Неотрицательный баланс

Можно добавить:

```sql
ALTER TABLE accounts
ADD CONSTRAINT accounts_balance_non_negative
CHECK (balance >= 0);
```

Но одного check может быть мало для красивого UX. Лучше списывать атомарно:

```sql
UPDATE accounts
SET balance = balance - $amount
WHERE id = $id
  AND balance >= $amount
RETURNING balance;
```

### Непересекающиеся интервалы

Для бронирований PostgreSQL умеет exclusion constraints:

```sql
CREATE EXTENSION IF NOT EXISTS btree_gist;

ALTER TABLE room_bookings
ADD CONSTRAINT room_bookings_no_overlap
EXCLUDE USING gist (
	room_id WITH =,
	tsrange(starts_at, ends_at) WITH &&
);
```

Это намного надежнее, чем "SELECT проверить, потом INSERT", потому что база сама защищает инвариант при параллельных insert.

## Advisory locks

Advisory lock - прикладная блокировка, которую PostgreSQL предоставляет, но смысл ключа определяет приложение.

Пример:

```sql
SELECT pg_advisory_xact_lock(12345);
```

`pg_advisory_xact_lock` живет до конца транзакции.

Можно lock-ать бизнесовый ресурс:

```sql
SELECT pg_advisory_xact_lock(hashtext('room:10'));
```

И потом проверять/вставлять бронирование.

Плюсы:

- удобно для ресурса, который не представлен одной строкой;
- можно сериализовать операции по бизнес-ключу;
- lock автоматически отпустится в конце транзакции для xact-варианта.

Минусы:

- база не понимает бизнес-смысл lock-а;
- все участники должны дисциплинированно брать тот же lock;
- можно получить bottleneck;
- нужно аккуратно выбирать ключи;
- не заменяет constraints там, где constraints возможны.

## Бизнес-кейс: списание баланса

Плохой вариант:

```sql
BEGIN;

SELECT balance
FROM accounts
WHERE id = $account_id;

-- приложение проверяет balance >= amount

UPDATE accounts
SET balance = $new_balance
WHERE id = $account_id;

COMMIT;
```

Проблема: read-modify-write в приложении. Две транзакции могут прочитать один баланс и перезаписать друг друга.

Лучше:

```sql
UPDATE accounts
SET balance = balance - $amount
WHERE id = $account_id
  AND balance >= $amount
RETURNING balance;
```

Это один statement. На `READ COMMITTED` PostgreSQL корректно обработает конкурирующие updates одной строки через row lock и перепроверку условия.

Если нужно еще писать ledger:

```sql
BEGIN;

UPDATE accounts
SET balance = balance - $amount
WHERE id = $account_id
  AND balance >= $amount
RETURNING balance;

INSERT INTO account_entries(account_id, amount, type)
VALUES ($account_id, -$amount, 'withdrawal');

COMMIT;
```

Но важно, чтобы `INSERT ledger` происходил только если `UPDATE ... RETURNING` вернул строку.

## Бизнес-кейс: лимит активных подписок

Правило:

> У пользователя не может быть больше трех активных подписок.

Наивно:

```sql
BEGIN;

SELECT count(*)
FROM subscriptions
WHERE user_id = $1
  AND status = 'active';

-- если count < 3
INSERT INTO subscriptions(user_id, status)
VALUES ($1, 'active');

COMMIT;
```

Две транзакции одновременно видят `count = 2` и обе вставляют. Итог `4`.

Решения зависят от модели.

Можно взять lock на строку пользователя:

```sql
BEGIN;

SELECT id
FROM users
WHERE id = $1
FOR UPDATE;

SELECT count(*)
FROM subscriptions
WHERE user_id = $1
  AND status = 'active';

INSERT ...

COMMIT;
```

Теперь операции по одному пользователю сериализуются.

Можно использовать advisory lock по user_id.

Можно держать счетчик активных подписок в `users` и обновлять его атомарно:

```sql
UPDATE users
SET active_subscriptions = active_subscriptions + 1
WHERE id = $1
  AND active_subscriptions < 3
RETURNING active_subscriptions;
```

После успешного update вставить подписку в той же транзакции.

`SERIALIZABLE` тоже может помочь, но потребует retry и все равно нужно думать о UX.

## Бизнес-кейс: бронирование комнаты

Правило:

> В одной комнате не должно быть пересекающихся бронирований.

Плохой вариант:

```sql
BEGIN;

SELECT 1
FROM room_bookings
WHERE room_id = $room_id
  AND starts_at < $new_end
  AND ends_at > $new_start;

-- если строк нет
INSERT INTO room_bookings(room_id, starts_at, ends_at)
VALUES ($room_id, $new_start, $new_end);

COMMIT;
```

Если две транзакции одновременно проверяют пустой интервал, обе могут вставить пересекающиеся бронирования.

Решения:

1. Exclusion constraint - часто лучший вариант в PostgreSQL.

2. `SERIALIZABLE` + retry.

3. Advisory lock по `room_id`, если constraint не подходит.

4. Lock на строку `rooms`:

   ```sql
   SELECT id FROM rooms WHERE id = $room_id FOR UPDATE;
   ```

   Тогда все бронирования одной комнаты идут последовательно.

## Бизнес-кейс: складской остаток

Плохой вариант:

```sql
SELECT stock
FROM products
WHERE id = $product_id;

-- если stock > 0

UPDATE products
SET stock = $stock - 1
WHERE id = $product_id;
```

Лучше:

```sql
UPDATE products
SET stock = stock - 1
WHERE id = $product_id
  AND stock > 0
RETURNING stock;
```

Если нужно списать несколько товаров из заказа, сложнее. Можно:

- блокировать строки товаров `FOR UPDATE`;
- обновлять в стабильном порядке по `product_id`, чтобы снизить deadlock risk;
- проверять affected rows;
- использовать отдельную таблицу движений склада;
- закладывать retry на deadlock/serialization errors.

## Бизнес-кейс: создание пользователя по email

Плохой вариант:

```sql
BEGIN;

SELECT id
FROM users
WHERE email = $email;

-- если нет
INSERT INTO users(email)
VALUES ($email);

COMMIT;
```

При параллельных запросах два потока могут увидеть отсутствие пользователя.

Правильно:

```sql
ALTER TABLE users
ADD CONSTRAINT users_email_unique UNIQUE (email);
```

И:

```sql
INSERT INTO users(email)
VALUES ($email)
ON CONFLICT (email) DO UPDATE
SET email = EXCLUDED.email
RETURNING id;
```

Или `DO NOTHING` + последующий select.

Это пример, где база должна защищать инвариант, а не приложение через предварительный SELECT.

## Бизнес-кейс: очередь задач

Нужно, чтобы несколько воркеров не взяли одну job.

Плохой вариант:

```sql
SELECT id
FROM jobs
WHERE state = 'ready'
ORDER BY available_at
LIMIT 1;

UPDATE jobs
SET state = 'processing'
WHERE id = $id;
```

Два воркера могут выбрать один id.

Лучше:

```sql
BEGIN;

WITH picked AS (
	SELECT id
	FROM jobs
	WHERE state = 'ready'
	  AND available_at <= now()
	ORDER BY available_at
	LIMIT 1
	FOR UPDATE SKIP LOCKED
)
UPDATE jobs
SET state = 'processing',
    locked_at = now()
FROM picked
WHERE jobs.id = picked.id
RETURNING jobs.id;

COMMIT;
```

Это берет lock на выбранную строку, пропускает уже заблокированные и атомарно переводит job в processing.

## Бизнес-кейс: дневной лимит операций

Правило:

> Пользователь может сделать переводов максимум на 100 000 рублей в день.

Наивно:

```sql
BEGIN;

SELECT coalesce(sum(amount), 0)
FROM transfers
WHERE user_id = $1
  AND created_at >= current_date
  AND created_at < current_date + interval '1 day';

-- если sum + amount <= 100000
INSERT INTO transfers(user_id, amount)
VALUES ($1, $amount);

COMMIT;
```

Две транзакции одновременно могут увидеть сумму 90 000 и обе добавить по 10 000. Итог 110 000.

Варианты:

1. Агрегатная строка лимита:

   ```sql
   CREATE TABLE daily_limits (
   	user_id bigint NOT NULL,
   	day date NOT NULL,
   	used_amount numeric NOT NULL,
   	PRIMARY KEY (user_id, day)
   );
   ```

   Атомарно:

   ```sql
   UPDATE daily_limits
   SET used_amount = used_amount + $amount
   WHERE user_id = $user_id
     AND day = current_date
     AND used_amount + $amount <= 100000
   RETURNING used_amount;
   ```

2. Lock на строку пользователя или лимита.

3. `SERIALIZABLE` + retry.

Для финансового домена часто лучше иметь явную агрегатную строку/ledger и constraints, чем каждый раз считать сумму по истории.

## Deadlocks

Deadlock - это не уровень изоляции, но рядом.

Пример:

T1:

```sql
BEGIN;
UPDATE accounts SET balance = balance - 10 WHERE id = 1;
UPDATE accounts SET balance = balance + 10 WHERE id = 2;
```

T2:

```sql
BEGIN;
UPDATE accounts SET balance = balance - 20 WHERE id = 2;
UPDATE accounts SET balance = balance + 20 WHERE id = 1;
```

T1 держит lock на account 1 и ждет account 2. T2 держит account 2 и ждет account 1. PostgreSQL обнаружит deadlock и abort-ит одну транзакцию.

Практика:

- брать locks в стабильном порядке;
- например всегда сортировать account ids;
- держать транзакции короткими;
- иметь retry на deadlock detected;
- не делать внешние вызовы внутри транзакции.

## Retry-логика

Для `SERIALIZABLE`, deadlocks и иногда lock timeouts нужна retry-логика.

Важно повторять всю транзакцию, а не последний statement.

Псевдокод:

```go
for attempt := 0; attempt < maxAttempts; attempt++ {
	err := runTx(ctx, db, func(tx Tx) error {
		// all reads and writes here
		return nil
	})
	if err == nil {
		return nil
	}
	if !isRetryableTxError(err) {
		return err
	}
	backoff(attempt)
}
return ErrTooMuchContention
```

Retryable:

- serialization failure;
- deadlock detected;
- иногда lock timeout, если бизнес допускает.

Но нельзя бездумно повторять транзакцию, если внутри нее был внешний side effect. Поэтому side effects нужно выносить через outbox/idempotency.

## Как выбирать уровень изоляции

Для большинства коротких OLTP-команд `READ COMMITTED` нормален, если:

- SQL выражает операцию атомарно;
- есть constraints;
- есть правильные уникальные индексы;
- для row-level инвариантов используется `FOR UPDATE` или атомарный `UPDATE`;
- нет сложных predicate-инвариантов.

`REPEATABLE READ` выбирают, когда нужна консистентная картина в рамках транзакции, особенно для read-only отчетов или операций, где повторное чтение должно быть стабильным. Но нужно помнить про конфликты при записи и про то, что write skew не исчезает как класс бизнесовой проблемы.

`SERIALIZABLE` выбирают, когда нужно защитить сложный инвариант, который трудно выразить constraints/locks, и приложение готово к retry. Это сильный инструмент, но он требует дисциплины.

Явные locks выбирают, когда понятно, какой ресурс нужно сериализовать: строка аккаунта, строка пользователя, строка комнаты, агрегатная строка лимита.

Constraints выбирают всегда, когда инвариант можно выразить декларативно. База надежнее приложения в гонках.

## Как ревьюить транзакционный код

Когда на ревью видишь транзакцию, нужно задать не "есть ли BEGIN", а другие вопросы.

Какие строки или диапазоны защищает операция? Если код сначала делает `SELECT`, потом в приложении принимает решение, потом делает `INSERT/UPDATE`, это место нужно читать особенно внимательно.

Если инвариант про одну строку, можно ли заменить read-modify-write на атомарный `UPDATE ... WHERE condition RETURNING`?

Если инвариант про отсутствие строки, есть ли unique/exclusion constraint? Если нет, предварительный `SELECT` не защищает от параллельного `INSERT`.

Если инвариант про набор строк, например "не больше трех активных подписок", что сериализует операции по одному пользователю? Lock на `users`? Advisory lock? Aggregation row? Serializable?

Если используется `SERIALIZABLE`, есть ли retry всей транзакции?

Если есть `SELECT FOR UPDATE`, что происходит, когда строк нет? Не пытаемся ли мы заблокировать пустоту?

Нет ли внешних HTTP/gRPC/payment/email/broker вызовов внутри транзакции?

Берутся ли locks в стабильном порядке?

Есть ли timeout-ы?

Обрабатываются ли deadlock/serialization errors?

## Частые ошибки

### "Транзакция значит безопасно"

Нет. Транзакция дает atomicity, но isolation зависит от уровня и от формы SQL. Read-modify-write в приложении может быть небезопасен даже внутри транзакции.

### "READ COMMITTED всегда достаточно"

Он часто достаточно хорош, но только если операции выражены атомарно или защищены constraints/locks. Для predicate-инвариантов он легко пропускает гонки.

### "REPEATABLE READ защищает от всех гонок"

Нет. Он дает стабильный snapshot и в PostgreSQL предотвращает phantom reads в смысле повторного чтения, но write skew возможен.

### "SERIALIZABLE просто делает все правильно"

Он дает сильную гарантию только для успешно закоммиченных транзакций. Некоторые транзакции будут abort-иться, и приложение обязано повторять их.

### "SELECT FOR UPDATE защищает отсутствие строки"

Нет. Он блокирует найденные строки. Если строк нет, ничего не заблокировано.

### "Сначала проверю, потом вставлю"

Без unique/exclusion constraint или lock-а это гонка.

### "Retry можно сделать только для последнего запроса"

Нет. При serialization failure нужно повторить всю транзакцию, потому что snapshot и все решения внутри нее устарели.

### "Можно отправить письмо внутри транзакции, потом если что retry"

Нельзя без idempotency/outbox. DB rollback не откатывает внешний мир.

## Таблица аномалий в PostgreSQL

Упрощенно для PostgreSQL:

| Уровень | Dirty read | Non-repeatable read | Phantom read | Write skew | Serialization failures |
|---|---|---|---|---|---|
| Read Uncommitted | Нет, работает как Read Committed | Возможен | Возможен | Возможен | Обычно нет как механизм SSI |
| Read Committed | Нет | Возможен | Возможен | Возможен | Возможны отдельные concurrent update/deadlock ошибки, но не SSI-гарантия |
| Repeatable Read | Нет | Нет | Нет в PostgreSQL | Возможен | Возможны ошибки при concurrent update |
| Serializable | Нет | Нет | Нет | Предотвращается через abort одной транзакции | Да, нормальная часть работы |

Важно: таблица помогает помнить, но на ревью нужно думать не названиями аномалий, а бизнесовым инвариантом.

## Диагностика в продакшене

Если есть подозрение на проблемы изоляции, часто в логах и метриках видны:

- `could not serialize access due to concurrent update`;
- `could not serialize access due to read/write dependencies among transactions`;
- `deadlock detected`;
- `canceling statement due to lock timeout`;
- долгие ожидания locks;
- рост latency транзакций;
- повторяющиеся duplicate key errors;
- неожиданные отрицательные остатки;
- превышенные лимиты;
- пересекающиеся бронирования;
- две активные "уникальные" сущности без unique constraint.

Полезные запросы:

```sql
SELECT
	pid,
	state,
	wait_event_type,
	wait_event,
	xact_start,
	now() - xact_start AS xact_age,
	query
FROM pg_stat_activity
WHERE xact_start IS NOT NULL
ORDER BY xact_start;
```

Блокировки:

```sql
SELECT *
FROM pg_locks
WHERE NOT granted;
```

Долгие idle транзакции:

```sql
SELECT
	pid,
	state,
	xact_start,
	now() - xact_start AS xact_age,
	query
FROM pg_stat_activity
WHERE state = 'idle in transaction';
```

Но самые неприятные изоляционные баги часто не выглядят как ошибки БД. Они выглядят как бизнесовые несостыковки: два бронирования на один слот, отрицательный остаток, превышенный лимит, задвоенная заявка. Поэтому лучший способ диагностики - проверять инварианты и искать места, где они защищены только предварительным `SELECT`.

## Практические паттерны

### Атомарный update вместо read-modify-write

```sql
UPDATE accounts
SET balance = balance - $amount
WHERE id = $id
  AND balance >= $amount
RETURNING balance;
```

### Unique constraint вместо check-then-insert

```sql
INSERT INTO users(email)
VALUES ($email)
ON CONFLICT (email) DO NOTHING;
```

### Exclusion constraint для интервалов

```sql
EXCLUDE USING gist (
	room_id WITH =,
	tsrange(starts_at, ends_at) WITH &&
);
```

### Lock родительского ресурса

```sql
SELECT id
FROM users
WHERE id = $user_id
FOR UPDATE;
```

### Advisory lock по бизнес-ключу

```sql
SELECT pg_advisory_xact_lock($resource_key);
```

### Serializable с retry

```sql
BEGIN ISOLATION LEVEL SERIALIZABLE;
-- read predicate
-- write
COMMIT;
-- if serialization failure: retry whole transaction
```

## Ответы для интервью

### Что такое уровень изоляции?

Это набор гарантий о том, как параллельные транзакции видят изменения друг друга. Он определяет, возможны ли грязные чтения, неповторяющиеся чтения, фантомы и более сложные аномалии вроде write skew.

### Как работает READ COMMITTED в PostgreSQL?

Каждый statement видит snapshot на начало statement. Поэтому внутри одной транзакции два SELECT могут увидеть разные committed-данные. Dirty read нет. Для update/delete PostgreSQL ждет конкурентный update и перепроверяет условие на новой версии строки.

### Как работает REPEATABLE READ?

Транзакция видит один snapshot на протяжении всей транзакции. Повторные чтения стабильны, phantom reads в PostgreSQL не возникают в обычном смысле. Но write skew возможен, потому что транзакции могут менять разные строки, опираясь на один и тот же старый snapshot.

### Как работает SERIALIZABLE?

PostgreSQL использует Serializable Snapshot Isolation. Транзакции читают snapshot, но база отслеживает read/write dependencies через predicate/SIREAD locks. Если параллельные транзакции нельзя сериализовать, одна получает serialization failure. Приложение должно повторить всю транзакцию.

### Что такое lost update?

Это когда две транзакции читают одно значение, каждая считает новое значение и одна перезаписывает результат другой. Лечится атомарным update, optimistic locking, row locks или более строгой изоляцией с retry.

### Что такое write skew?

Это когда две транзакции читают общий инвариант, но обновляют разные строки, и вместе ломают правило. Например, две транзакции снимают двух врачей с дежурства, каждая видела, что врачей двое, а в итоге не осталось никого. На Repeatable Read это возможно, Serializable должен abort-ить одну транзакцию.

### Когда нужен SELECT FOR UPDATE?

Когда нужно заблокировать конкретные существующие строки, на основе которых принимается решение. Например, аккаунт перед списанием или заказ перед сменой статуса. Но `FOR UPDATE` не блокирует отсутствие строк.

### Что делать с serialization failure?

Повторить всю транзакцию с самого начала. Это штатная часть работы на `SERIALIZABLE`, а не неожиданный сбой.

## Хороший рассказ на 2 минуты

PostgreSQL строит изоляцию на MVCC: изменения создают новые версии строк, а транзакции читают snapshot. На `READ COMMITTED` каждый statement получает новый snapshot, поэтому повторные чтения внутри транзакции могут видеть разные committed-данные. Это дефолтный и часто нормальный уровень для OLTP, если операции выражены атомарными SQL-командами и constraints. На `REPEATABLE READ` snapshot один на всю транзакцию, поэтому повторные чтения стабильны и phantom reads в PostgreSQL не возникают, но возможен write skew, когда две транзакции читают один инвариант и меняют разные строки. На `SERIALIZABLE` PostgreSQL использует Serializable Snapshot Isolation: отслеживает read/write зависимости и, если параллельное выполнение нельзя представить как последовательное, abort-ит одну транзакцию. Поэтому при `SERIALIZABLE` обязательно нужна retry-логика всей транзакции. В реальном коде я бы защищал инварианты в первую очередь constraints, атомарными `UPDATE ... WHERE ... RETURNING`, `INSERT ... ON CONFLICT`, `SELECT FOR UPDATE` для существующих строк, exclusion constraints для интервалов, advisory locks для бизнес-ресурсов и только там, где нужно, serializable-транзакциями с retry.

## Чек-лист ревью

- Есть ли read-modify-write в приложении?
- Можно ли заменить его атомарным SQL?
- Есть ли unique/exclusion/check constraint для бизнес-инварианта?
- Если код проверяет отсутствие строки, что мешает другой транзакции вставить ее параллельно?
- Если используется `SELECT FOR UPDATE`, что происходит, когда строк нет?
- Если инвариант по набору строк, какой ресурс сериализует операции?
- Есть ли retry для serialization failure/deadlock?
- Нет ли внешних side effects внутри транзакции?
- Берутся ли locks в стабильном порядке?
- Не держится ли транзакция слишком долго?
- Есть ли индексы под predicate locks/serializable-запросы?
- Не используется ли `REPEATABLE READ` как ложная защита от write skew?
- Не полагается ли код на предварительный `SELECT` вместо constraint?
- Есть ли timeout-ы: `statement_timeout`, `lock_timeout`, `idle_in_transaction_session_timeout`?
- Проверяется ли `rows affected` после conditional update?
- Обрабатываются ли duplicate key errors как нормальный конкурентный сценарий?
