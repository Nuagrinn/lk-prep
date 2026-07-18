# LK Prep

Репозиторий материалов LearnKeeper для подготовки к code review секции на Go middle/middle+ и backend-собеседованиям.

Основной тренировочный файл:

- [review_task.go](review_task.go) - фрагмент service layer для ревью.

Для локальных агентов: перед правками прочитать [Agent contract](AGENTS.md).

## Быстрый срез: готово

| Раздел | Готовые материалы |
|---|---:|
| Code review Go | 6 |
| Базовый Go | 6 |
| Базы данных | 3 |
| Книги / DDIA | 3 |
| Лайвкодинг и практика | 4 |
| Компьютерные основы | 1 |

## Быстрый срез: планируется

| Раздел | Планируемые материалы |
|---|---:|
| Code review Go | 5 |
| Базовый Go | 5 |
| Базы данных | 5 |
| Лайвкодинг и практика | 2 |
| System Design | 2 |
| Компьютерные основы | 1 |
| Идиомы и паттерны Go | 1 |
| Книги | 1 |

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
| B09 | Базовые примитивы конкурентности: goroutines, channels, mutex, WaitGroup, select, atomic | [review.md](base-go/09-concurrency-primitives/review.md) | [practice_output.go](base-go/09-concurrency-primitives/practice_output.go) | Готово |
| B10 | Время в Go: time, context, timer/ticker, TTL, timeout и background refresh | [review.md](base-go/10-time-context-timers/review.md) | - | Готово |
| B11 | Анонимные функции, callbacks, closures и передача поведения в конструктор | [review.md](base-go/11-anonymous-functions/review.md) | - | Готово |

## Базы данных

| # | Тема | Материал | Практика | Статус |
|---|---|---|---|---|
| DB01 | PostgreSQL: индексы — устройство, типы и цена владения | - | - | Планируется |
| DB02 | PostgreSQL: MVCC, VACUUM, версии строк, память и bloat | [review.md](database/02-postgresql-mvcc-vacuum-bloat/review.md) | - | Готово |
| DB03 | PostgreSQL: физическое хранение, страницы, buffers, cache и I/O | [review.md](database/03-postgresql-storage-pages-buffers/review.md) | - | Готово |
| DB04 | PostgreSQL: уровни изоляции, аномалии транзакций и бизнесовые гонки | [review.md](database/04-postgresql-isolation-anomalies/review.md) | - | Готово |
| DB06 | Cassandra, Redis, ClickHouse: когда и зачем использовать | - | - | Планируется |
| DB07 | Массовые операции: delete/update, bloat, locks, батчи, partitioning | - | - | Планируется |
| DB08 | PostgreSQL: планировщик запросов, статистика и EXPLAIN | - | - | Планируется |
| DB09 | PostgreSQL: диагностика и эксплуатация индексов в проде | - | - | Планируется |

## Книги

Книжные разборы устроены как верхнеуровневые индексы: строка книги ведет в
отдельный `index.md`, а уже внутри него лежит оглавление готовых разборов глав.
Строки книг держим в статусе "Планируется": это навигация, а не тема для
квиза. LearnKeeper раскрывает поддержанные книжные индексы и тренирует уже
готовые строки глав внутри `index.md`.

| # | Тема | Материал | Практика | Статус |
|---|---|---|---|---|
| BK01 | Martin Kleppmann - Designing Data-Intensive Applications | [index.md](books/ddia/index.md) | - | Планируется |

## Лайвкодинг и практика

| # | Тема | Файл | Статус |
|---|---|---|---|
| LC01 | Базовый тренировочный service layer для ревью | [review_task.go](review_task.go) | Готово |
| LC02 | Практика по конкурентному service layer: cache, channels, WaitGroup, RWMutex | [main/aaa.go](main/aaa.go) | Готово |
| LC03 | Highload RPC handler: тяжелое вычисление, кэш, ticker/timer, atomic | [weather_handler_solution.go](live-coding/highload-weather-cache/weather_handler_solution.go) | Готово |
| LC04 | Что выведет код: slices/maps/runtime snippets | - | Планируется |
| LC05 | Большой файл на несколько ГБ: внешняя сортировка и стратегия решения | - | Планируется |
| LC06 | Список лайвкодинг-задач с трекингом решено/не решено (строки, мапы, слайсы и далее) | [tasks.md](live-coding/tasks.md) | Готово |

## System Design

| # | Тема | Материал | Практика | Статус |
|---|---|---|---|---|
| SD01 | System Design: Transactional Outbox | - | - | Планируется |
| SD02 | System Design: rate limiting — token bucket, leaky bucket, распределённые счётчики, backpressure | - | - | Планируется |

## Компьютерные основы

Общая computer-science база не привязанная к конкретному языку: как устроены низкоуровневые механизмы, которые потом всплывают в любом языке и системе. Сюда не заводим прикладные Go-темы (для них есть "Базовый Go") — только подкапотные основы.

| # | Тема | Материал | Практика | Статус |
|---|---|---|---|---|
| CS01 | Кодировки текста: UTF-8 под капотом, побитовая арифметика, overlong encoding, byte/rune/grapheme, сравнение с другими языками | [internals.md](cs-fundamentals/01-text-encoding/internals.md) | - | Готово |
| CS02 | Процессы и потоки на уровне ОС: контекст-свитчинг, разделяемая память, IPC | - | - | Планируется |

## Идиомы и паттерны Go

Справочник приёмов написания идиоматичного Go-кода (мапы, слайсы,
сортировка и далее) - не темы для собеседования и не материал для тестов,
а reference-гайды "как это принято делать и почему", к которым возвращаешься
по ходу решения задач. Темы этого раздела намеренно держим со статусом
"Планируется" даже когда материал уже готов - LearnKeeper подбирает темы
для тестов и напоминаний только среди тем со статусом "Готово", так этот
раздел гарантированно не попадёт в квиз-ротацию. Обновлять статус на
"Готово" здесь не нужно.

| # | Тема | Материал | Практика | Статус |
|---|---|---|---|---|
| GI01 | Мапы, слайсы, сортировка: comma-ok, map[K]struct{}, sort.Interface/sort.Slice/slices.SortFunc, SoA vs AoS | [guide.md](go-idioms/01-slices-maps-sorting/guide.md) | - | Планируется |
