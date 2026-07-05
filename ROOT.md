# Interview Review Prep

Репозиторий для подготовки к code review секции на Go middle/middle+ и backend-собеседованиям.

Основной тренировочный файл:

- [review_task.go](review_task.go) - фрагмент service layer для ревью.

Для локальных агентов: перед правками прочитать [Agent contract](AGENTS.md).

## Быстрый срез: готово

| Раздел | Готовые материалы |
|---|---:|
| Code review Go | 6 |
| Базовый Go | 3 |
| Базы данных | 4 |
| Лайвкодинг и практика | 2 |
| Собеседования | 1 |

## Быстрый срез: планируется

| Раздел | Планируемые материалы |
|---|---:|
| Code review Go | 5 |
| Базовый Go | 5 |
| Базы данных | 3 |
| Лайвкодинг и практика | 3 |
| System Design | 1 |

## Code Review Go

| # | Тема | Материал | Практика | Статус |
|---|---|---|---|---|
| CR01 | Многопоточность, общий доступ к памяти, data race | [review.md](01-concurrency-data-race/review.md) | [practice_service.go](01-concurrency-data-race/practice_service.go) | Готово |
| CR02 | Mutex, инкапсуляция shared state, cache access contract | [review.md](02-mutex-cache-contract/review.md) | - | Готово |
| CR03 | Каналы, worker lifecycle, shutdown | [review.md](03-channels-workers-shutdown/review.md) | [practice_service.go](03-channels-workers-shutdown/practice_service.go) | Готово |
| CR04 | Goroutines, backpressure, fan-out, утечки | - | - | Планируется |
| CR05 | Context: cancellation, deadline, request scope | [review.md](05-context-cancellation/review.md) | [practice_service.go](05-context-cancellation/practice_service.go) | Готово |
| CR06 | Транзакции и внешние side effects | [review.md](06-transactions-side-effects/review.md) | [practice_service.go](06-transactions-side-effects/practice_service.go) | Готово |
| CR07 | API и архитектура сервисного слоя | [review.md](07-service-api-architecture/review.md) | [practice_service.go](07-service-api-architecture/practice_service.go) | Готово |
| CR08 | Обработка ошибок и observability | - | - | Планируется |
| CR09 | Паники, nil dependencies, constructor validation | - | - | Планируется |
| CR10 | Валидация входных данных и бизнес-инварианты | - | - | Планируется |
| CR11 | Кеш, память, TTL, eviction | - | - | Планируется |

## Базовый Go

| # | Тема | Материал | Практика | Статус |
|---|---|---|---|---|
| B01 | Слайсы и массивы: len/cap, nil/empty, append, передача в функции | [review.md](base-go/01-slices/review.md) | [practice_output.go](base-go/01-slices/practice_output.go) | Готово |
| B02 | Мапы: устройство, nil map, порядок обхода, конкурентный доступ, ключи | [internals.md](base-go/02-maps/internals.md), [review.md](base-go/02-maps/review.md) | [practice_task.go](base-go/02-maps/practice_task.go) | Готово |
| B03 | Runtime Go: GMP, GOMAXPROCS, syscalls, scheduler | - | - | Планируется |
| B04 | GC и аллокатор: heap/stack, escape analysis, pauses | - | - | Планируется |
| B05 | Интерфейсы, nil interface, method set, duck typing | - | - | Планируется |
| B06 | Указатели: value vs pointer, nil, escape analysis, receiver, shared mutable state | - | - | Планируется |
| B07 | Ошибки в Go: error interface, wrapping, errors.Is/As, sentinel/custom errors, panic vs error | - | - | Планируется |
| B08 | Строки: immutable bytes, rune/UTF-8, len, slicing, conversions, strings.Builder | [internals.md](base-go/08-strings/internals.md), [review.md](base-go/08-strings/review.md) | [practice_output.go](base-go/08-strings/practice_output.go) | Готово |

## Базы данных

| # | Тема | Материал | Практика | Статус |
|---|---|---|---|---|
| DB01 | PostgreSQL: индексы, планировщик и диагностика проблем | [review.md](database/01-postgresql-indexes-diagnostics/review.md) | - | Готово |
| DB02 | PostgreSQL: MVCC, VACUUM, версии строк, память и bloat | [review.md](database/02-postgresql-mvcc-vacuum-bloat/review.md) | - | Готово |
| DB03 | PostgreSQL: физическое хранение, страницы, buffers, cache и I/O | [review.md](database/03-postgresql-storage-pages-buffers/review.md) | - | Готово |
| DB04 | PostgreSQL: уровни изоляции, аномалии транзакций и бизнесовые гонки | [review.md](database/04-postgresql-isolation-anomalies/review.md) | - | Готово |
| DB05 | SQL vs NoSQL: выбор хранилища под задачу | - | - | Планируется |
| DB06 | Cassandra, Redis, ClickHouse: когда и зачем использовать | - | - | Планируется |
| DB07 | Массовые операции: delete/update, bloat, locks, батчи, partitioning | - | - | Планируется |

## Лайвкодинг и практика

| # | Тема | Файл | Статус |
|---|---|---|---|
| LC01 | Базовый тренировочный service layer для ревью | [review_task.go](review_task.go) | Готово |
| LC02 | Практика по конкурентному service layer: cache, channels, WaitGroup, RWMutex | [main/aaa.go](main/aaa.go) | Готово |
| LC03 | Highload RPC handler: тяжелое вычисление, кэш, ticker/timer, atomic | - | Планируется |
| LC04 | Что выведет код: slices/maps/runtime snippets | - | Планируется |
| LC05 | Большой файл на несколько ГБ: внешняя сортировка и стратегия решения | - | Планируется |

## Собеседования

| # | Тема | Материал | Статус |
|---|---|---|---|
| A01 | Ответы на вопросы Go/backend собеседования | [go-backend-interview-questions.md](interview-answers/go-backend-interview-questions.md) | Готово |

## System Design

| # | Тема | Материал | Практика | Статус |
|---|---|---|---|---|
| SD01 | System Design: Transactional Outbox | - | - | Планируется |
