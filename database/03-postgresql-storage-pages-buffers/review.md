# PostgreSQL: физическое хранение, страницы, buffers, cache и I/O

Этот документ про то, как PostgreSQL физически хранит и читает данные: таблицы, страницы, heap, индексы, shared buffers, OS cache, WAL, sequential/random I/O, TOAST, сортировки и временные файлы.

Если предыдущая тема была про MVCC и версии строк, то здесь главный вопрос такой:

> Когда приложение отправило SQL-запрос, какие реальные куски данных PostgreSQL читает, где они лежат, через какие слои памяти проходят, почему один запрос быстрый, а другой внезапно начинает читать миллионы страниц.

Официальные источники:

- [PostgreSQL docs: Database File Layout](https://www.postgresql.org/docs/current/storage-file-layout.html)
- [PostgreSQL docs: Database Page Layout](https://www.postgresql.org/docs/current/storage-page-layout.html)
- [PostgreSQL docs: Free Space Map](https://www.postgresql.org/docs/current/storage-fsm.html)
- [PostgreSQL docs: Visibility Map](https://www.postgresql.org/docs/current/storage-vm.html)
- [PostgreSQL docs: TOAST](https://www.postgresql.org/docs/current/storage-toast.html)
- [PostgreSQL docs: Resource Consumption](https://www.postgresql.org/docs/current/runtime-config-resource.html)
- [PostgreSQL docs: Write-Ahead Logging](https://www.postgresql.org/docs/current/wal-intro.html)
- [PostgreSQL docs: Using EXPLAIN](https://www.postgresql.org/docs/current/using-explain.html)

## Главная мысль

База данных работает не со "строками" в вакууме.

Она работает с блоками/страницами.

Когда ты пишешь:

```sql
SELECT *
FROM orders
WHERE id = 42;
```

PostgreSQL не достает из диска "одну строку". Он:

1. Выбирает план.
2. Идет в индекс или таблицу.
3. Читает нужные страницы по 8 KB.
4. Загружает их в shared buffers.
5. Проверяет видимость строк.
6. Достает нужные tuple.
7. При необходимости идет в TOAST, индексы, временные файлы.
8. Возвращает результат.

Короткая формулировка для интервью:

> PostgreSQL физически хранит таблицы и индексы как relations, разбитые на страницы, обычно по 8 KB. Запросы читают и изменяют страницы через buffer manager и shared buffers, а ОС дополнительно держит свой page cache. Производительность часто определяется не количеством строк в SQL, а количеством страниц, которые нужно прочитать, их locality, cache hit ratio, random/sequential I/O, состоянием индексов, bloat, TOAST и тем, не выливаются ли сортировки/hash operations на диск.

## Простое введение

Представь книгу.

Логически ты хочешь найти одну фразу:

```text
"заказ 42"
```

Но физически книга состоит из страниц. Ты не можешь прочитать с полки ровно одну фразу. Ты открываешь страницу, читаешь кусок, ищешь нужное место.

База данных похожа:

- таблица - это не просто список строк;
- таблица физически лежит в файлах;
- файлы разбиты на страницы;
- страница - минимальная единица чтения/записи на уровне PostgreSQL;
- внутри страницы лежат версии строк;
- индексы тоже состоят из страниц;
- запрос читает страницы, а не абстрактные строки.

Если нужные данные лежат рядом, запрос быстрый.

Если нужные данные разбросаны по тысячам страниц, запрос медленнее.

Если страницы уже в cache, запрос быстрый.

Если их надо читать с диска, запрос медленнее.

Если запрос требует сортировки на 10 GB, а памяти мало, он польет временные файлы на диск.

## Логическое и физическое хранение

На уровне приложения ты видишь:

```sql
SELECT id, email
FROM users
WHERE email = 'a@example.com';
```

Это логический уровень:

- база;
- схема;
- таблица;
- строка;
- колонка;
- индекс;
- constraint;
- view;
- SQL-запрос.

Но PostgreSQL выполняет это через физический уровень:

- relation files;
- forks;
- pages/blocks;
- heap tuples;
- index tuples;
- TOAST chunks;
- WAL records;
- shared buffers;
- OS page cache;
- temp files.

Связь:

```text
SQL table users
  -> PostgreSQL relation
    -> files on disk
      -> 8 KB pages
        -> tuple versions
```

Для индекса:

```text
SQL index users_email_idx
  -> index relation
    -> files on disk
      -> 8 KB pages
        -> index tuples / tree nodes
```

Когда говорят "виртуальное хранение" в прикладном смысле, часто имеют в виду, что SQL-объекты скрывают физику. Ты работаешь с таблицами и строками, но БД сама отображает это на файлы, страницы, буферы и журналы.

## Relation и файлы на диске

В PostgreSQL таблица, индекс, TOAST-таблица - это relation.

У relation есть файлы на диске. Большие relation разбиваются на сегменты, чтобы один файл не становился бесконечно большим.

У relation есть несколько forks.

Главные:

- main fork - основные данные;
- FSM fork - free space map;
- VM fork - visibility map;
- init fork - для unlogged relations.

Например, таблица `orders` физически имеет основной файл с heap-страницами, а рядом могут быть файлы для FSM и VM.

Индекс тоже relation. У него свои страницы и свои FSM/VM-особенности.

Практический вывод:

> Когда таблица "весит 100 GB", это не один монолитный объект из SQL. Это набор файлов и страниц. Индексы - отдельные physical relations, которые могут весить еще 200 GB.

## Page / block: базовая единица

PostgreSQL page обычно 8 KB.

Это значение задается при сборке PostgreSQL, и в типичных установках оно 8192 bytes.

Страница - базовая единица:

- чтения из relation;
- записи в relation;
- кеширования в shared buffers;
- адресации tuple через `(block number, offset)`.

Почему это важно:

```sql
SELECT *
FROM users
WHERE id = 42;
```

Даже если строка занимает 200 bytes, PostgreSQL читает страницу 8 KB, где она лежит.

Если запросу нужно 100 строк:

- может хватить 2 страниц, если строки лежат рядом;
- может потребоваться 100 страниц, если строки разбросаны.

Это и есть locality.

## Page layout: что внутри страницы

Упрощенно heap page выглядит так:

```text
+-----------------------------+
| page header                 |
+-----------------------------+
| line pointers / item ids    |
| line pointer -> tuple A     |
| line pointer -> tuple B     |
+-----------------------------+
| free space                  |
+-----------------------------+
| tuple data                  |
| tuple B                     |
| tuple A                     |
+-----------------------------+
```

Внутри страницы есть:

- page header - служебная информация;
- line pointers - указатели на tuple внутри страницы;
- free space - свободное место;
- tuple data - сами версии строк.

Line pointer важен: индекс ссылается на tuple через TID, то есть `(block number, offset number)`. Offset number указывает на line pointer, а не просто на байт.

Это дает PostgreSQL некоторую гибкость внутри страницы: tuple можно двигать внутри страницы, пока line pointer сохраняет адресацию.

## Tuple: физическая версия строки

Логическая строка:

```text
id = 42, email = 'a@example.com'
```

Физически это tuple version.

Tuple содержит:

- служебный header;
- `xmin`;
- `xmax`;
- `ctid`;
- null bitmap, если есть nullable колонки;
- значения колонок;
- возможно ссылки на TOAST для больших значений.

Из-за MVCC одна логическая строка может иметь несколько физических tuple versions.

```text
users id=42
  version 1: email = old@example.com
  version 2: email = a@example.com
```

На странице могут лежать:

- live tuples;
- dead tuples;
- recently dead tuples;
- HOT chain;
- свободное место после pruning/vacuum.

## CTID

`ctid` - физический адрес версии строки.

Можно посмотреть:

```sql
SELECT ctid, id, email
FROM users
WHERE id = 42;
```

Пример результата:

```text
ctid    | id | email
--------+----+----------------
(12,5)  | 42 | a@example.com
```

Это значит:

- block/page 12;
- line pointer 5.

Важно:

> `ctid` меняется при UPDATE, потому что UPDATE обычно создает новую physical tuple version.

Поэтому `ctid` нельзя использовать как стабильный бизнесовый идентификатор.

## Heap table

Heap - основной способ хранения таблиц в PostgreSQL.

Heap не означает, что строки отсортированы по primary key.

Таблица:

```sql
CREATE TABLE orders (
	id bigint PRIMARY KEY,
	created_at timestamptz NOT NULL,
	user_id bigint NOT NULL,
	status text NOT NULL
);
```

Физически новые строки обычно добавляются туда, где есть место:

- в конец relation;
- или в страницу, где FSM показывает свободное место.

Если ты вставляешь `id = 1, 2, 3`, это не гарантирует вечную физическую сортировку по `id`, особенно после UPDATE/DELETE/VACUUM.

Для физической переупаковки по индексу есть `CLUSTER`, но порядок со временем снова деградирует.

## Индекс как отдельная структура страниц

B-tree индекс тоже состоит из страниц.

Упрощенно:

```text
root page
  -> internal page
    -> leaf page
      key = 42 -> TID (12,5)
```

Запрос:

```sql
SELECT *
FROM orders
WHERE id = 42;
```

по primary key:

1. Читает страницы индекса.
2. Находит ключ `42`.
3. Получает TID.
4. Читает heap page.
5. Проверяет MVCC видимость.
6. Возвращает строку.

Значит, даже "быстрый поиск по индексу" может читать несколько страниц:

- root/internal/leaf страницы индекса;
- heap страницу;
- TOAST страницы, если нужны большие значения.

Если индекс и heap page уже в shared buffers, все быстро.

Если каждый шаг требует диска, медленнее.

## Почему индекс может быть медленным

Индекс ускоряет поиск, но не отменяет физику страниц.

Плохой случай:

```sql
SELECT *
FROM orders
WHERE status = 'paid';
```

Если `paid` у 80% строк, индекс по `status` найдет огромное число TID.

Потом PostgreSQL должен сходить в heap за множеством страниц.

Получается много random I/O:

```text
index leaf -> heap page 100
index leaf -> heap page 52342
index leaf -> heap page 7
index leaf -> heap page 982344
...
```

Иногда дешевле `Seq Scan`: прочитать таблицу последовательно.

Хорошая формулировка:

> Индекс полезен, когда он сокращает количество читаемых страниц или помогает читать их в удобном порядке. Если индекс приводит к миллионам случайных heap fetches, он может проиграть последовательному скану.

## Sequential scan

`Seq Scan` читает таблицу последовательно.

```sql
EXPLAIN (ANALYZE, BUFFERS)
SELECT *
FROM orders
WHERE status = 'paid';
```

Если большая часть таблицы подходит, планировщик может выбрать:

```text
Seq Scan on orders
  Filter: (status = 'paid')
```

Почему это не всегда плохо:

- последовательное чтение эффективно;
- read-ahead на уровне ОС/диска помогает;
- страницы читаются подряд;
- если нужно много строк, индексные random fetches хуже.

Типичная ошибка на интервью:

> Видеть Seq Scan и автоматически говорить "нужен индекс".

Правильнее:

> Я бы посмотрел, сколько строк возвращается, сколько страниц читается, есть ли селективность, какие buffers hit/read, и почему планировщик счел sequential scan дешевле.

## Random I/O и sequential I/O

Sequential I/O:

```text
page 1000
page 1001
page 1002
page 1003
```

Random I/O:

```text
page 1000
page 52341
page 17
page 900002
```

На HDD random I/O был особенно дорогим из-за механики диска.

На SSD разница меньше, но все равно есть:

- random access хуже cache locality;
- больше IOPS;
- больше page misses;
- хуже prefetch/read-ahead;
- больше pressure на shared buffers.

PostgreSQL cost model учитывает это через параметры вроде:

- `seq_page_cost`;
- `random_page_cost`;
- `effective_cache_size`.

Эти параметры не ускоряют диск сами по себе. Они помогают планировщику оценить, какой план дешевле.

## Buffer manager

PostgreSQL не читает страницы напрямую в память каждого запроса как попало.

Есть buffer manager.

Он управляет shared buffers:

```text
query executor
  -> buffer manager
    -> shared buffers
      -> OS / disk
```

Когда executor просит страницу:

1. Buffer manager проверяет, есть ли page в shared buffers.
2. Если есть, возвращает buffer.
3. Если нет, выбирает victim buffer для вытеснения.
4. Если victim dirty, его нужно записать.
5. Читает нужную страницу с диска/OS cache.
6. Возвращает executor.

Shared buffers - общий кэш страниц PostgreSQL для всех backend-процессов.

## Shared buffers

`shared_buffers` - память PostgreSQL под страницы таблиц и индексов.

Если page есть в shared buffers, запросу не нужно идти в ОС за этой страницей.

Но есть нюанс:

> Даже если страницы нет в shared buffers, она может быть в OS page cache. Тогда физического чтения с диска тоже может не быть.

Слои:

```text
PostgreSQL shared buffers
OS page cache
disk / SSD / network storage
```

Поэтому "прочитано не из shared buffers" не всегда означает "с физического диска".

В `EXPLAIN (ANALYZE, BUFFERS)`:

- `shared hit` - страница найдена в shared buffers;
- `shared read` - PostgreSQL пришлось читать страницу в shared buffers;
- `shared dirtied` - страница стала dirty;
- `shared written` - страница была записана.

`shared read` может прийти из OS cache, но для PostgreSQL это все равно read operation.

## OS page cache

Операционная система тоже кэширует файлы.

PostgreSQL читает relation files через ОС, а ОС может держать часто используемые страницы в page cache.

Получается двойной cache:

- shared buffers внутри PostgreSQL;
- page cache внутри OS.

Почему это нормально:

- PostgreSQL контролирует свои буферы, locks, dirty state;
- ОС хорошо умеет read-ahead, page cache, writeback;
- планировщик учитывает ожидаемый cache через `effective_cache_size`.

Но для диагностики это усложнение:

- первый прогон запроса может быть медленным;
- второй быстрым;
- тесты на локальной машине могут врать;
- `EXPLAIN ANALYZE` после нескольких запусков может показывать уже прогретый cache.

## Что значит "прогреть cache"

Если один и тот же запрос выполняется несколько раз:

```sql
SELECT *
FROM orders
WHERE user_id = 42
ORDER BY created_at DESC
LIMIT 50;
```

Первый запуск:

- читает index pages;
- читает heap pages;
- возможно читает TOAST;
- кладет их в shared buffers/OS cache.

Второй запуск:

- многие страницы уже в памяти;
- latency ниже;
- buffers показывают больше `hit`.

Это не всегда означает, что запрос хорошо оптимизирован.

Он может быть быстрым только пока cache теплый.

Если workload большой и cache постоянно вытесняется, запрос снова станет дорогим.

## Dirty pages

Когда запрос меняет данные:

```sql
UPDATE orders
SET status = 'paid'
WHERE id = 42;
```

PostgreSQL меняет страницу в shared buffers. Такая страница становится dirty.

Dirty page - страница в памяти отличается от версии на диске.

Она будет записана на диск позже:

- background writer;
- checkpoint;
- eviction;
- explicit operations.

Commit не обязан ждать записи heap page на диск, потому что надежность обеспечивает WAL.

## WAL и страницы

WAL - Write-Ahead Log.

Правило:

> Сначала журналируем изменение, потом можно записывать измененную страницу данных.

Если сервер упал:

1. PostgreSQL читает последний checkpoint.
2. Проигрывает WAL после checkpoint.
3. Восстанавливает изменения.

Зачем это нужно:

- commit может быть быстрее;
- не надо сразу fsync каждой heap/index page;
- recovery возможно после crash;
- репликация передает WAL.

Но WAL - тоже I/O.

Массовый UPDATE:

```sql
UPDATE orders
SET status = 'archived'
WHERE created_at < now() - interval '2 years';
```

создает:

- heap page changes;
- index page changes;
- WAL;
- dirty pages;
- replication lag;
- будущую работу VACUUM.

## Checkpoint

Checkpoint - точка, до которой PostgreSQL гарантирует, что dirty pages сброшены так, чтобы recovery мог стартовать оттуда.

Если checkpoints слишком частые:

- больше write spikes;
- latency может прыгать;
- диск перегружается.

Если checkpoints слишком редкие:

- recovery после crash дольше;
- больше WAL нужно хранить.

Для прикладного разработчика важно:

> Большие записи в БД влияют не только на конкретный SQL. Они могут создавать WAL/checkpoint pressure и ухудшать latency всего сервиса.

## Background writer

Background writer пытается заранее сбрасывать dirty pages, чтобы пользовательские backend-процессы реже сами писали грязные страницы при вытеснении.

Если dirty pages слишком много, пользовательский запрос может внезапно начать ждать запись страницы.

Так появляются latency spikes.

## Temporary files

Не все данные запроса живут в shared buffers.

Сортировки, hash join, hash aggregate могут использовать `work_mem`.

Если памяти не хватает, PostgreSQL пишет временные файлы.

Пример:

```sql
SELECT user_id, count(*)
FROM events
GROUP BY user_id
ORDER BY count(*) DESC;
```

Если событий много:

- hash aggregate может не поместиться в `work_mem`;
- sort может не поместиться;
- PostgreSQL создаст temp files;
- запрос станет I/O-bound.

В `EXPLAIN` можно увидеть:

```text
Sort Method: external merge  Disk: 2048MB
```

или:

```text
Batches: 16  Memory Usage: ...
```

В логах при `log_temp_files` можно увидеть временные файлы.

Важный нюанс:

> `work_mem` выделяется не на весь сервер и не строго на один запрос, а на операции плана. Один запрос может использовать несколько `work_mem`, а много параллельных запросов могут резко увеличить память.

## Memory knobs: что за что отвечает

### shared_buffers

Кэш страниц PostgreSQL.

Влияет на:

- чтение таблиц;
- чтение индексов;
- dirty pages;
- cache locality.

### effective_cache_size

Не выделяет память.

Это подсказка планировщику: сколько памяти примерно доступно под cache с учетом shared buffers и OS cache.

Если значение слишком маленькое, planner может недооценивать индексные планы.

Если слишком большое, может переоценивать вероятность cache hit.

### work_mem

Память для операций:

- sort;
- hash join;
- hash aggregate;
- materialize;
- некоторые bitmap operations.

Опасность:

```text
work_mem * operations per query * concurrent queries
```

### maintenance_work_mem

Память для maintenance operations:

- VACUUM;
- CREATE INDEX;
- ALTER TABLE ADD FOREIGN KEY;
- maintenance tasks.

### temp_buffers

Буферы для temporary tables в рамках сессии.

## FSM: Free Space Map

FSM показывает, где в relation есть свободное место.

После DELETE/VACUUM внутри страниц появляется reusable space.

Когда PostgreSQL вставляет новую tuple, он может использовать FSM:

```text
нужна страница с >= 300 bytes свободного места
FSM подсказывает candidate page
```

Если FSM не помогла или места нет, вставка идет в конец relation.

Почему это важно:

- после VACUUM место может переиспользоваться;
- таблица может не уменьшиться физически, но новые строки будут занимать старые дырки;
- плохой fillfactor/частые updates влияют на свободное место;
- bloat означает, что свободного/мертвого места слишком много или оно плохо используется.

## VM: Visibility Map

Visibility Map хранит признаки по heap pages:

- all-visible;
- all-frozen.

All-visible значит:

> Все tuple на странице видимы всем транзакциям.

Это помогает:

- VACUUM пропускать страницы;
- Index Only Scan не ходить в heap.

Пример:

```sql
SELECT amount
FROM payments
WHERE user_id = 42
ORDER BY created_at DESC
LIMIT 20;
```

Если индекс содержит `amount` через `INCLUDE`, теоретически возможен Index Only Scan.

Но если VM не пометила heap pages как all-visible, PostgreSQL все равно может делать heap fetch для проверки видимости.

Поэтому VACUUM влияет не только на место, но и на эффективность чтения.

## TOAST

Страница PostgreSQL обычно 8 KB. А что делать с большим `text` или `jsonb` на 200 KB?

Для этого есть TOAST.

TOAST позволяет:

- сжимать большие значения;
- выносить большие значения в отдельную TOAST-таблицу;
- хранить в основной строке ссылку на chunks.

Пример:

```sql
CREATE TABLE events (
	id bigint PRIMARY KEY,
	payload jsonb NOT NULL
);
```

Если `payload` большой, физически:

```text
events heap tuple:
  id = 42
  payload = pointer to TOAST chunks

events toast table:
  chunk 1
  chunk 2
  chunk 3
```

Запрос:

```sql
SELECT id
FROM events
WHERE id = 42;
```

может не читать весь `payload`.

А запрос:

```sql
SELECT payload
FROM events
WHERE id = 42;
```

может пойти в TOAST.

Практический вывод:

> `SELECT *` по таблице с большим JSONB может быть намного дороже, чем выбор нескольких маленьких колонок.

## SELECT * и широкие строки

Плохой паттерн:

```sql
SELECT *
FROM users
WHERE id = $1;
```

Если таблица:

```sql
CREATE TABLE users (
	id bigint PRIMARY KEY,
	email text NOT NULL,
	name text NOT NULL,
	settings jsonb NOT NULL,
	avatar bytea,
	bio text
);
```

`SELECT *` может:

- читать TOAST;
- гонять большие данные по сети;
- мешать index-only scan;
- увеличивать CPU на deserialize;
- увеличивать latency API.

Лучше:

```sql
SELECT id, email, name
FROM users
WHERE id = $1;
```

На ревью:

> `SELECT *` особенно подозрителен на широких таблицах с JSONB/text/bytea. Он превращает маленький lookup в чтение лишних страниц и TOAST chunks.

## Row width и rows per page

Страница 8 KB.

Если строка маленькая:

```text
tuple ~100 bytes
```

на страницу помещается много строк.

Если строка широкая:

```text
tuple ~2 KB
```

на страницу помещается мало строк.

Для sequential scan это критично:

```text
1 миллион узких строк -> меньше страниц
1 миллион широких строк -> больше страниц
```

Даже если количество строк одинаковое, широкая таблица может читаться намного дольше.

Бизнесовый пример:

`orders` содержит:

- id;
- status;
- amount;
- user_id;
- created_at;
- большой `metadata jsonb`;
- большой `debug_payload`.

Большинство запросов ищут список заказов и показывают только id/status/amount.

Если все лежит в одной таблице, heap pages шире, cache хуже, scans дороже.

Варианты:

- вынести тяжелые поля в отдельную таблицу `order_details`;
- не делать `SELECT *`;
- использовать covering indexes для легких списков;
- хранить debug_payload отдельно/в object storage;
- продумать нормализацию.

## Data locality

Data locality - насколько нужные данные лежат рядом физически.

Хорошая locality:

```sql
SELECT *
FROM events
WHERE created_at >= now() - interval '1 hour';
```

если события вставляются по времени, новые данные лежат близко к концу таблицы.

Плохая locality:

```sql
SELECT *
FROM orders
WHERE user_id = 42;
```

если заказы пользователя разбросаны по всей истории таблицы.

Индекс найдет TID, но heap pages могут быть разбросаны.

Correlation в статистике помогает планировщику оценить, насколько порядок значений похож на физический порядок таблицы.

Можно смотреть:

```sql
SELECT attname, correlation
FROM pg_stats
WHERE tablename = 'events';
```

Если `created_at` сильно коррелирует с физическим порядком, range scan по времени может быть эффективнее.

## CLUSTER и физический порядок

Можно физически переписать таблицу по индексу:

```sql
CLUSTER events USING idx_events_created_at;
```

После этого строки будут физически упорядочены по `created_at`.

Плюсы:

- range scans читают более последовательные страницы;
- лучше cache locality;
- меньше random I/O.

Минусы:

- тяжелая операция;
- блокировки;
- порядок со временем деградирует;
- нужен maintenance.

Для OLTP обычно осторожно. Для больших append-mostly таблиц иногда полезно.

## BRIN и физический порядок

BRIN индекс использует идею физической locality.

Он хранит summary по диапазонам страниц:

```text
pages 0-127: created_at from Jan 1 to Jan 3
pages 128-255: created_at from Jan 3 to Jan 5
```

Запрос:

```sql
SELECT *
FROM events
WHERE created_at >= '2026-06-01'
  AND created_at < '2026-06-02';
```

BRIN быстро понимает, какие диапазоны страниц могут содержать нужные даты.

Плюсы:

- очень маленький индекс;
- хорошо для огромных append-only/time-series таблиц;
- дешевле B-tree по размеру.

Минусы:

- неточный;
- нужен recheck;
- плохо, если данные физически перемешаны.

## Bitmap Scan

Bitmap scan хорошо показывает механику страниц.

Запрос:

```sql
SELECT *
FROM orders
WHERE status = 'paid'
  AND created_at >= now() - interval '1 day';
```

PostgreSQL может сделать:

```text
Bitmap Index Scan on idx_orders_status
Bitmap Index Scan on idx_orders_created_at
BitmapAnd
Bitmap Heap Scan on orders
```

Идея:

1. По индексам собрать bitmap подходящих TID/pages.
2. Объединить условия.
3. Читать heap pages пачками, а не прыгать по одному TID.

Это компромисс между:

- чистым Index Scan;
- Seq Scan.

Bitmap Heap Scan часто хорош, когда строк не очень мало, но индекс все еще помогает.

В `EXPLAIN` можно увидеть:

```text
Recheck Cond: ...
Heap Blocks: exact=...
```

Если bitmap стал lossy из-за нехватки памяти, PostgreSQL хранит не точные tuple, а страницы целиком и потом делает recheck.

## Index Only Scan

Index Only Scan возможен, когда:

- все нужные колонки есть в индексе;
- visibility map позволяет не проверять heap для большинства страниц.

Пример:

```sql
CREATE INDEX idx_orders_user_created_include_amount
ON orders(user_id, created_at DESC)
INCLUDE (amount);
```

Запрос:

```sql
SELECT created_at, amount
FROM orders
WHERE user_id = 42
ORDER BY created_at DESC
LIMIT 20;
```

Может использовать только индекс.

Но:

```text
Heap Fetches: 1000
```

в плане означает, что PostgreSQL все равно ходил в heap, потому что visibility map не позволила доверять индексу полностью.

Связь с хранением:

> INCLUDE кладет данные в индексные страницы, а VACUUM/VM помогают избежать чтения heap pages.

## Почему count(*) может быть дорогим

В некоторых БД `count(*)` может быть быстрым за счет метаданных. В PostgreSQL MVCC мешает просто взять "число строк из таблицы".

Запрос:

```sql
SELECT count(*)
FROM orders;
```

PostgreSQL должен посчитать строки, видимые текущему snapshot.

Он может:

- сделать Seq Scan;
- иногда Index Only Scan, если есть подходящий индекс и VM хорошая;
- читать много страниц.

На большой таблице это дорого.

Варианты:

- approximate counts из статистики;
- материализованные счетчики;
- отдельная агрегатная таблица;
- аналитическое хранилище;
- кэширование.

Собеседовательная формулировка:

> В PostgreSQL `count(*)` по большой таблице не бесплатен из-за MVCC. Нужно проверить видимость строк, поэтому часто требуется скан таблицы или индекса.

## ORDER BY и сортировка

Запрос:

```sql
SELECT *
FROM orders
WHERE user_id = 42
ORDER BY created_at DESC
LIMIT 20;
```

Если есть индекс:

```sql
CREATE INDEX idx_orders_user_created
ON orders(user_id, created_at DESC);
```

PostgreSQL может читать уже в нужном порядке.

Если индекса нет:

1. Найти все строки пользователя.
2. Отсортировать.
3. Взять 20.

Если строк много, sort может уйти на диск.

В `EXPLAIN`:

```text
Sort Method: quicksort  Memory: ...
```

или:

```text
Sort Method: external merge  Disk: ...
```

Для API-пагинации индекс под `WHERE + ORDER BY` часто критичен.

## OFFSET и физика чтения

Плохой паттерн:

```sql
SELECT *
FROM orders
ORDER BY created_at DESC
LIMIT 50 OFFSET 100000;
```

PostgreSQL должен пройти первые 100000 строк, отбросить их, потом вернуть 50.

Даже если есть индекс по `created_at`, это все равно чтение большого количества index entries и, возможно, heap pages.

Лучше keyset pagination:

```sql
SELECT *
FROM orders
WHERE created_at < $last_seen_created_at
ORDER BY created_at DESC
LIMIT 50;
```

С индексом:

```sql
CREATE INDEX idx_orders_created_desc
ON orders(created_at DESC);
```

Бизнесовая формулировка:

> OFFSET-пагинация деградирует с глубиной страницы, потому что база физически должна пройти и отбросить предыдущие строки. Keyset pagination позволяет продолжить чтение с нужного места в индексе.

## Join и чтение страниц

Join - это не магия соединения таблиц. Это физическое чтение страниц из нескольких relations.

### Nested Loop

```text
for each row from A:
  find matching rows in B
```

Хорошо, если:

- A маленькая;
- по B есть индекс;
- совпадений мало.

Плохо, если:

- A большая;
- B ищется без индекса;
- получается scan B много раз.

### Hash Join

```text
build hash table from smaller input
scan larger input and probe hash
```

Хорошо для больших join без подходящего индекса.

Риск:

- hash table не помещается в work_mem;
- появляются batches/temp files;
- запрос льет диск.

### Merge Join

```text
оба входа отсортированы
идем двумя указателями
```

Хорошо, если данные уже отсортированы индексами или сортировка недорогая.

На интервью:

> Join performance зависит не только от количества строк, но и от того, какие страницы надо читать, есть ли индексы, помещается ли hash в память, не нужна ли сортировка на диск.

## N+1 как проблема страниц

Код:

```go
orders := repo.ListOrders(ctx, userID)
for _, order := range orders {
	items := repo.ListItemsByOrderID(ctx, order.ID)
	...
}
```

SQL:

```sql
SELECT * FROM orders WHERE user_id = $1;

-- потом 1000 раз:
SELECT * FROM order_items WHERE order_id = $1;
```

Даже если каждый запрос по индексу быстрый, суммарно:

- много round trips;
- много index page lookups;
- много heap fetches;
- плохая batch locality;
- лишняя нагрузка на connection pool.

Лучше:

```sql
SELECT *
FROM order_items
WHERE order_id = ANY($1);
```

или join/load в одном запросе.

С точки зрения страниц:

> N+1 заставляет БД много раз повторять похожие index traversals и heap fetches вместо более последовательного/batch чтения.

## Foreign key без индекса как проблема страниц

Родитель:

```sql
CREATE TABLE customers (
	id bigint PRIMARY KEY
);
```

Дочерняя таблица:

```sql
CREATE TABLE orders (
	id bigint PRIMARY KEY,
	customer_id bigint NOT NULL REFERENCES customers(id)
);
```

Если нет:

```sql
CREATE INDEX idx_orders_customer_id
ON orders(customer_id);
```

то:

```sql
DELETE FROM customers
WHERE id = 42;
```

может вынудить PostgreSQL проверить всю `orders`, чтобы понять, есть ли ссылки.

Физически:

```text
Seq Scan orders:
  read page 0
  read page 1
  read page 2
  ...
```

То есть удаление одной parent-строки превращается в чтение миллионов страниц child-таблицы.

С индексом:

```text
B-tree lookup customer_id=42
  read few index pages
  maybe read matching heap pages
```

Это один из самых практичных примеров, где понимание физического хранения сразу объясняет бизнесовую боль.

## Soft delete и широкая горячая таблица

Таблица:

```sql
CREATE TABLE documents (
	id bigint PRIMARY KEY,
	account_id bigint NOT NULL,
	title text NOT NULL,
	body text NOT NULL,
	metadata jsonb NOT NULL,
	deleted_at timestamptz,
	created_at timestamptz NOT NULL
);
```

API:

```sql
SELECT *
FROM documents
WHERE account_id = $1
  AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT 20;
```

Проблемы:

- `SELECT *` тащит `body` и `metadata`;
- `deleted_at IS NULL` без partial index читает лишнее;
- deleted rows остаются в той же таблице;
- широкие строки уменьшают rows per page;
- cache забивается тяжелыми страницами;
- сортировка может быть дорогой.

Лучше:

```sql
CREATE INDEX idx_documents_alive_account_created
ON documents(account_id, created_at DESC)
WHERE deleted_at IS NULL;
```

И запрос:

```sql
SELECT id, title, created_at
FROM documents
WHERE account_id = $1
  AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT 20;
```

А `body/metadata` доставать отдельным запросом по `id`, когда пользователь открывает документ.

## Time-series и retention

Таблица событий:

```sql
CREATE TABLE events (
	id bigserial,
	created_at timestamptz NOT NULL,
	payload jsonb NOT NULL
);
```

Проблемы:

- данные растут постоянно;
- старые данные удаляются;
- запросы часто по времени;
- payload большой;
- `DELETE old rows` создает dead tuples;
- indexes и heap пухнут.

Хороший дизайн:

- partition by time;
- BRIN или B-tree по времени в зависимости от запросов;
- не делать `SELECT payload` без необходимости;
- retention через drop partition;
- аналитика в ClickHouse/DWH, если нужны большие сканы.

Пример:

```sql
CREATE TABLE events (
	id bigserial,
	created_at timestamptz NOT NULL,
	payload jsonb NOT NULL
) PARTITION BY RANGE (created_at);
```

Удаление старого месяца:

```sql
DROP TABLE events_2026_01;
```

Это намного дешевле, чем массовый DELETE.

## Job queue

Таблица:

```sql
CREATE TABLE jobs (
	id bigserial PRIMARY KEY,
	state text NOT NULL,
	available_at timestamptz NOT NULL,
	payload jsonb NOT NULL,
	locked_at timestamptz,
	updated_at timestamptz NOT NULL
);
```

Воркер:

```sql
SELECT id
FROM jobs
WHERE state = 'ready'
  AND available_at <= now()
ORDER BY available_at
LIMIT 100
FOR UPDATE SKIP LOCKED;
```

Индекс:

```sql
CREATE INDEX idx_jobs_ready_available
ON jobs(available_at)
WHERE state = 'ready';
```

Физические риски:

- частые UPDATE state создают новые tuple versions;
- partial index постоянно меняется;
- payload jsonb широкий;
- HOT может быть невозможен;
- autovacuum должен успевать;
- done jobs надо архивировать/удалять.

Бизнесовый вывод:

> Таблица очереди - это не просто таблица. Это update-heavy структура с постоянным churn страниц, индексов и vacuum. Ее надо проектировать как горячий механизм, а не как обычный справочник.

## Dashboard count/sum

Бизнес хочет:

```sql
SELECT count(*), sum(amount)
FROM payments
WHERE account_id = $1
  AND created_at >= now() - interval '30 days';
```

Если платежей много:

- надо читать много index/heap pages;
- aggregate должен обработать много строк;
- index-only scan возможен только при хорошем covering index и VM;
- каждый refresh dashboard может грузить БД.

Варианты:

- covering index:

```sql
CREATE INDEX idx_payments_account_created_include_amount
ON payments(account_id, created_at)
INCLUDE (amount);
```

- предагрегаты;
- materialized view;
- cache;
- аналитическое хранилище;
- инкрементальные counters.

Ответ на собесе:

> Индекс может помочь сузить диапазон и дать index-only scan, но если бизнес постоянно считает агрегаты по большим объемам, часто нужен предагрегированный слой, а не бесконечная оптимизация OLTP-таблицы.

## Диагностика через EXPLAIN BUFFERS

Главный инструмент:

```sql
EXPLAIN (ANALYZE, BUFFERS)
SELECT ...
```

Смотреть:

- `shared hit`;
- `shared read`;
- `shared dirtied`;
- `shared written`;
- temp read/write;
- `Heap Fetches`;
- `Rows Removed by Filter`;
- actual rows vs estimated rows;
- sort method;
- hash batches.

Пример:

```text
Buffers: shared hit=100 read=5000
```

означает, что PostgreSQL нашел 100 страниц в shared buffers и прочитал 5000 страниц в shared buffers из нижнего слоя.

```text
Buffers: temp read=20000 written=20000
```

значит, запрос работал с временными файлами. Часто это sort/hash spill.

```text
Heap Fetches: 100000
```

у Index Only Scan означает, что он был не таким уж only.

## Как читать план через физику страниц

### Seq Scan

Вопрос:

- сколько страниц таблицы читаем?
- сколько строк удалено фильтром?
- таблица маленькая или огромная?
- данные в cache?

### Index Scan

Вопрос:

- сколько строк найдено?
- сколько heap pages fetched?
- не слишком ли много random I/O?
- селективен ли индекс?

### Bitmap Heap Scan

Вопрос:

- сколько heap blocks exact/lossy?
- хватает ли work_mem?
- нет ли recheck на огромном числе строк?

### Sort

Вопрос:

- in-memory или external merge?
- сколько disk?
- можно ли индексом дать порядок?

### Hash Join / Hash Aggregate

Вопрос:

- помещается ли hash в память?
- сколько batches?
- есть ли temp read/write?

## Что ускоряет запросы на уровне хранения

- селективные индексы;
- индекс под `WHERE + ORDER BY + LIMIT`;
- covering index / INCLUDE;
- хороший visibility map для index-only scan;
- узкие строки;
- отсутствие `SELECT *`;
- хорошая data locality;
- partition pruning;
- BRIN для больших упорядоченных таблиц;
- актуальная статистика;
- отсутствие bloat;
- достаточный cache;
- сортировка, которая помещается в память;
- батчинг вместо N+1;
- keyset pagination вместо OFFSET;
- partial indexes для горячих подмножеств;
- короткие транзакции, чтобы VACUUM работал.

## Что замедляет

- чтение большого числа страниц;
- random heap fetches;
- низкая селективность индекса;
- bloat таблиц и индексов;
- широкие строки;
- большие TOAST значения;
- `SELECT *`;
- сортировки/hash на диск;
- глубокий OFFSET;
- N+1 запросы;
- отсутствие индекса на FK child side;
- устаревшая статистика;
- плохая correlation;
- long-running transactions, мешающие VACUUM;
- hot update паттерны по indexed columns;
- слишком много индексов на write-heavy таблице;
- медленный storage;
- checkpoint/WAL pressure.

## Бизнесовые задачки с собеседований

### 1. Почему удаление одного пользователя зависло на 30 секунд?

Вероятный ответ:

- `DELETE FROM users WHERE id = ?`;
- есть дочерняя таблица `orders(user_id) REFERENCES users(id)`;
- нет индекса на `orders.user_id`;
- FK check сканирует всю `orders`;
- читает много страниц;
- держит locks;
- тормозит API.

Решение:

```sql
CREATE INDEX CONCURRENTLY idx_orders_user_id
ON orders(user_id);
```

### 2. Почему список заказов стал медленным, хотя есть индекс по user_id?

Запрос:

```sql
SELECT *
FROM orders
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT 50;
```

Есть:

```sql
CREATE INDEX idx_orders_user_id ON orders(user_id);
```

Проблема:

- индекс помогает найти user_id;
- но потом надо сортировать все заказы пользователя;
- `SELECT *` тащит широкие поля;
- heap pages разбросаны.

Лучше:

```sql
CREATE INDEX idx_orders_user_created
ON orders(user_id, created_at DESC);
```

И выбирать только нужные поля.

### 3. Почему `count(*)` по таблице на 100 млн строк долгий?

Потому что PostgreSQL должен проверить видимость строк для snapshot.

Решения:

- approximate count из статистики;
- cached counter;
- aggregate table;
- materialized view;
- index-only scan может помочь, но не всегда;
- аналитическое хранилище.

### 4. Почему dashboard кладет базу?

Запросы:

```sql
SELECT sum(amount)
FROM payments
WHERE created_at >= now() - interval '30 days';
```

каждые 5 секунд от сотен пользователей.

Проблема:

- большие range scans;
- aggregate много строк;
- повторное чтение страниц;
- cache вытесняется;
- OLTP база делает OLAP-работу.

Решение:

- предагрегация;
- cache;
- materialized view;
- ClickHouse/DWH;
- covering/partial indexes как временная помощь.

### 5. Почему поиск по `%text%` медленный?

Запрос:

```sql
SELECT *
FROM products
WHERE name ILIKE '%phone%';
```

B-tree не помогает для подстроки в середине.

База читает много страниц и фильтрует строки.

Решения:

- trigram GIN/GiST;
- full-text search;
- search engine;
- изменить тип поиска.

### 6. Почему после добавления JSONB все стало тяжелее?

Если `jsonb` большой:

- строки стали шире;
- TOAST;
- `SELECT *` читает больше;
- GIN index большой;
- update JSONB дорогой;
- WAL растет.

Решения:

- не выбирать JSONB без нужды;
- вынести payload;
- нормализовать горячие поля;
- осторожно с GIN;
- разделить OLTP и event payload.

### 7. Почему OFFSET-пагинация деградирует?

Потому что база физически проходит и отбрасывает строки.

```sql
LIMIT 50 OFFSET 500000
```

означает пройти 500000 index entries/rows.

Решение:

- keyset pagination.

### 8. Почему autovacuum и storage связаны с чтением?

Если VACUUM не успевает:

- dead tuples остаются;
- bloat растет;
- pages становятся менее плотными;
- index-only scan хуже из-за VM;
- scans читают больше страниц.

То есть VACUUM влияет не только на "очистку", но и на скорость чтения.

## Методология диагностики медленного запроса

1. Получить реальный запрос и параметры.

2. Выполнить:

   ```sql
   EXPLAIN (ANALYZE, BUFFERS)
   ...
   ```

3. Проверить:

   - какие scan types;
   - сколько actual rows;
   - сколько rows removed;
   - buffers hit/read;
   - temp read/write;
   - sort/hash spill;
   - heap fetches.

4. Посмотреть размер таблицы и индексов:

   ```sql
   SELECT
   	pg_size_pretty(pg_relation_size('orders')) AS table_size,
   	pg_size_pretty(pg_indexes_size('orders')) AS indexes_size,
   	pg_size_pretty(pg_total_relation_size('orders')) AS total_size;
   ```

5. Проверить статистику:

   ```sql
   SELECT attname, n_distinct, correlation, null_frac
   FROM pg_stats
   WHERE tablename = 'orders';
   ```

6. Проверить bloat/autovacuum косвенно:

   ```sql
   SELECT
   	n_live_tup,
   	n_dead_tup,
   	last_autovacuum,
   	last_autoanalyze
   FROM pg_stat_user_tables
   WHERE relname = 'orders';
   ```

7. Проверить, не широкая ли таблица и нет ли TOAST.

8. Проверить, не нужен ли другой индекс, partial index, covering index или partitioning.

9. Проверить бизнесовый уровень:

   - можно ли не делать `SELECT *`;
   - можно ли кэшировать;
   - можно ли предагрегировать;
   - можно ли батчить;
   - можно ли уйти от OFFSET;
   - не OLAP ли это на OLTP базе.

## Чек-лист для ревью

- Запрос выбирает только нужные колонки?
- Нет ли `SELECT *` из широкой таблицы?
- Есть ли `ORDER BY LIMIT`, и покрывает ли его индекс?
- Нет ли глубокого OFFSET?
- Нет ли N+1?
- Не читает ли API большой JSONB без необходимости?
- Нет ли FK child side без индекса?
- Не делается ли `count(*)` по огромной таблице на каждый request?
- Не делает ли dashboard тяжелые агрегаты по OLTP таблице?
- Не уходит ли sort/hash на диск?
- Есть ли bloat?
- Актуальна ли статистика?
- Нет ли long transaction, мешающей VACUUM?
- Подходит ли row width для частых list-запросов?
- Нужна ли отдельная details table для тяжелых полей?
- Нужен ли partitioning для retention/time-series?
- Не слишком ли много random heap fetches?
- Может ли помочь covering index?
- Может ли помочь BRIN из-за физического порядка?

## Что отвечать на интервью

### Что такое page/block в PostgreSQL?

Page или block - базовая единица хранения и кеширования relation. Обычно это 8 KB. Таблицы и индексы состоят из таких страниц. Даже если нужна одна строка, PostgreSQL читает страницу, где она лежит.

### Что такое shared buffers?

Shared buffers - общий кэш страниц PostgreSQL. Запросы читают heap/index pages через buffer manager. Если страница уже в shared buffers, это `shared hit`; если ее нужно загрузить, это `shared read`.

### Чем shared buffers отличается от OS cache?

Shared buffers - кэш внутри PostgreSQL, который понимает страницы, locks, dirty state. OS cache - кэш файлов на уровне операционной системы. Если PostgreSQL показывает `shared read`, это значит, что страницы не было в shared buffers, но физически она могла прийти из OS cache.

### Почему индекс не всегда быстрее Seq Scan?

Если условие возвращает много строк, индекс может привести к множеству random heap fetches. Seq Scan читает страницы последовательно и может быть дешевле, особенно если нужно прочитать большую часть таблицы.

### Почему широкая таблица медленнее?

На страницу помещается меньше строк, значит для того же числа строк нужно читать больше страниц. Плюс большие `text/jsonb/bytea` могут уходить в TOAST, а `SELECT *` будет читать лишние данные.

### Что такое TOAST?

TOAST - механизм хранения больших значений. PostgreSQL может сжимать или выносить большие `text/jsonb/bytea` в отдельную TOAST-таблицу, а в основной строке хранить ссылку.

### Что такое temp files?

Если sort/hash/aggregate не помещается в `work_mem`, PostgreSQL пишет временные файлы на диск. Это часто видно в `EXPLAIN` как external sort или temp read/write.

### Почему `count(*)` медленный?

Из-за MVCC PostgreSQL должен считать строки, видимые текущему snapshot. На большой таблице это требует скана таблицы или индекса, а не простого чтения метаданных.

### Что такое data locality?

Это насколько нужные данные лежат рядом физически. Хорошая locality уменьшает random I/O и улучшает cache. Например, append-only события часто хорошо лежат по `created_at`, а данные одного пользователя могут быть разбросаны по всей таблице.

### Зачем нужен EXPLAIN BUFFERS?

Он показывает не только план и время, но и работу со страницами: сколько страниц взяли из shared buffers, сколько прочитали, сколько временных файлов использовали, были ли heap fetches.

## Хороший рассказ на 2 минуты

PostgreSQL хранит таблицы и индексы как relation-файлы, разбитые на страницы, обычно по 8 KB. В heap-страницах лежат tuple versions, а индексы хранят ключи и ссылки на физические tuple через TID. Запрос не читает абстрактную строку, он читает страницы через buffer manager в shared buffers; ниже еще есть OS page cache и диск. Если страница уже в cache, чтение быстро, если надо читать много разбросанных страниц, запрос становится дорогим. Индекс помогает, когда сокращает количество читаемых страниц или дает нужный порядок, но при низкой селективности может привести к большому числу random heap fetches, и тогда Seq Scan дешевле. Широкие строки, TOAST, `SELECT *`, bloat, temp files от сортировок, deep OFFSET и N+1 увеличивают физический объем чтения. Для диагностики я бы смотрел `EXPLAIN (ANALYZE, BUFFERS)`: scan type, buffers hit/read, temp read/write, heap fetches, sort method и actual rows. А дальше решал бы на уровне запроса и модели: нужный индекс, covering index, partial index, keyset pagination, partitioning, предагрегация, отказ от SELECT *, вынос больших полей или настройка autovacuum.

## Мини-шпаргалка

`page/block`:

- обычно 8 KB;
- базовая единица чтения/кеша.

`heap`:

- физическое хранение таблицы;
- строки не обязательно отсортированы.

`tuple`:

- физическая версия строки;
- содержит MVCC header.

`ctid`:

- физический адрес tuple `(block, offset)`;
- меняется при UPDATE.

`index`:

- отдельная relation;
- key -> TID;
- ускоряет доступ, но может вести к heap fetches.

`shared buffers`:

- кэш страниц PostgreSQL.

`OS cache`:

- кэш файлов операционной системы.

`dirty page`:

- страница изменена в памяти и еще не сброшена.

`WAL`:

- журнал изменений перед записью страниц.

`FSM`:

- где есть свободное место.

`VM`:

- какие страницы all-visible/all-frozen.

`TOAST`:

- большие значения отдельно.

`work_mem`:

- сортировки/hash;
- нехватка -> temp files.

`Seq Scan`:

- читает таблицу последовательно;
- не всегда плохо.

`Index Scan`:

- индекс + heap fetch;
- может быть random I/O.

`Bitmap Heap Scan`:

- собирает bitmap и читает heap pages пачками.

`Index Only Scan`:

- может читать только индекс;
- зависит от visibility map.
