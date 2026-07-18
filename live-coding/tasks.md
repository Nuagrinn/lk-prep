# Лайвкодинг: практические задачи

Условие, сигнатура функции и примеры с ожидаемым выводом лежат прямо в
`.go`-файле задачи как doc-комментарий, вместе с `main()`, который сразу
запускает все примеры и печатает `OK`/`MISMATCH`. Пиши решение прямо в этом
файле и запускай `go run <путь>/<файл>.go` - результат виден сразу, без
ручной сверки вывода.

Каждая задача - в своей поддиректории (`NN-slug/файл.go`), а не плоским
файлом прямо в `live-coding/`: все файлы задач - это `package main` с
собственным `func main()`, и несколько таких файлов в одной директории
конфликтуют при `go vet ./...`/`go build ./...` ("main redeclared"), даже
если `go run` на конкретном файле по отдельности работает нормально.

## Список задач

| # | Тема | Файл | Статус |
|---|---|---|---|
| LC06-1 | Строки + мапы + слайсы: топ-K частых слов | [01-top-k-frequent-words/top_k_frequent_words.go](01-top-k-frequent-words/top_k_frequent_words.go) | Решено |
| LC06-2 | Слайсы: сортировка структур по нескольким ключам | [02-sort-products/sort_products.go](02-sort-products/sort_products.go) | Не решено — оба примера дают OK, но это ложноположительно: первый ключ сравнивает Stock напрямую вместо булевого "в наличии/нет", баг не виден на текущих данных примеров |
| LC06-3 | Примитивы синхронизации + map/slice: конкурентная загрузка и детерминированный merge | [03-release-digest-sync/release_digest_sync.go](03-release-digest-sync/release_digest_sync.go) | Не решено |
| LC06-4 | Односвязный список: указатели, вставка, удаление, обход | [04-linked-list/linked_list.go](04-linked-list/linked_list.go) | Не решено |
| LC06-5 | Таймеры: delayed batch flusher через time.Timer/time.AfterFunc | [05-delayed-batch-flusher/delayed_batch_flusher.go](05-delayed-batch-flusher/delayed_batch_flusher.go) | Не решено |
| LC06-6 | Анонимные функции: простой callback для обхода активных пользователей | [06-anonymous-callback/anonymous_callback.go](06-anonymous-callback/anonymous_callback.go) | Решено |
| LC06-7 | Анонимные функции: pipeline через callbacks, closure и nil validation | [07-event-pipeline-callbacks/event_pipeline_callbacks.go](07-event-pipeline-callbacks/event_pipeline_callbacks.go) | Не решено |
| LC06-8 | Анонимные функции: обработка слайса через callback-реализации и локальную функцию | [08-task-report-callbacks/task_report_callbacks.go](08-task-report-callbacks/task_report_callbacks.go) | Не решено |

## Как сдать решение

Допиши функцию в файле задачи и запусти его. Когда все примеры печатают
`OK` - можно считать задачу решённой, статус в таблице выше обновляется на
"Решено".
