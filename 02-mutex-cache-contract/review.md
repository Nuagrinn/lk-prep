# Mutex, инкапсуляция shared state, cache access contract

Эта тема продолжает разговор про data race, но фокус здесь другой. В теме 01 мы говорили: если несколько горутин одновременно читают и пишут общую память без синхронизации, будет data race. Здесь мы идем глубже:

- как правильно проектировать доступ к shared state;
- почему одного наличия `sync.Mutex` в структуре недостаточно;
- что такое контракт доступа к кешу;
- почему прямой доступ к `map` надо прятать за методы;
- где границы lock-а должны быть маленькими, а где наоборот нужна атомарность операции;
- чем техническая потокобезопасность отличается от логической консистентности;
- как ревьюить сервисный кеш, счетчики, локальное состояние и вспомогательные map-ы.

Главная мысль:

> Mutex - это не просто “добавить Lock/Unlock вокруг map”. Mutex должен быть частью понятного контракта: какие поля он защищает, кто имеет право к ним обращаться, какие операции должны быть атомарными, и что нельзя делать под lock.

И вторая мысль:

> Если shared state доступен напрямую из разных методов, синхронизация держится на дисциплине каждого автора. Это хрупко. Лучше инкапсулировать доступ так, чтобы нарушить правило было трудно.

## Простое введение

Допустим, есть сервис:

```go
type Service struct {
	cache map[string]Order
	mu    sync.RWMutex
}
```

В одном методе автор аккуратно делает:

```go
s.mu.RLock()
order, ok := s.cache[id]
s.mu.RUnlock()
```

А в другом методе:

```go
s.cache[order.ID] = order
```

На ревью это красный флаг. Само наличие `mu` говорит: поле `cache` надо защищать. Но правило применяется не везде.

Проблема не только в конкретной пропущенной блокировке. Проблема в том, что у поля `cache` нет жесткого контракта доступа. Любой метод может написать напрямую `s.cache[...]`, и компилятор не остановит. Сегодня ошибся один автор, завтра другой.

Более устойчивый вариант:

```go
func (s *Service) getCachedOrder(id string) (Order, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	order, ok := s.cache[id]
	return order, ok
}

func (s *Service) setCachedOrder(order Order) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cache[order.ID] = order
}
```

Теперь остальные методы сервиса вызывают `getCachedOrder` и `setCachedOrder`, а не трогают `s.cache` напрямую.

Это не делает систему идеально консистентной во всех бизнес-смыслах, но решает важную инженерную проблему: правило синхронизации становится локализованным.

На ревью можно сказать так:

> Здесь mutex используется непоследовательно. Я бы спрятал прямой доступ к cache за методы, чтобы контракт был “к этому полю нельзя обращаться без lock”. Иначе каждый новый метод может случайно обойти синхронизацию.

## Что такое shared state

Shared state - это состояние, которое может быть доступно нескольким горутинам.

В сервисном слое это часто:

- in-memory cache;
- map с сессиями;
- map с подписчиками;
- счетчики;
- локальные лимиты;
- last seen timestamps;
- набор pending jobs;
- состояние background worker-а;
- флаги shutdown/start;
- last successful sync time;
- snapshot конфигурации;
- локальный rate limiter;
- дедупликационные структуры;
- буфер событий;
- connection/client state, если клиент не потокобезопасен.

Важно: поле структуры становится shared state не потому, что оно “глобальное”, а потому что один экземпляр структуры используется несколькими горутинами.

Например HTTP-сервис:

```go
svc := NewService(...)
http.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {
	svc.GetOrder(r.Context(), id)
})
```

Каждый HTTP-запрос обрабатывается в своей горутине. Значит все поля `svc` потенциально разделяются между запросами.

## Что защищает mutex

`sync.Mutex` и `sync.RWMutex` решают две задачи.

Первая: взаимное исключение. Пока одна горутина находится между `Lock()` и `Unlock()`, другая горутина не может войти в секцию под тем же mutex.

Вторая: видимость памяти. `Unlock` в одной горутине синхронизируется с последующим `Lock` в другой. Изменения, сделанные до `Unlock`, становятся корректно видимыми после `Lock`.

Для `RWMutex` есть два режима:

- `RLock/RUnlock` для чтения;
- `Lock/Unlock` для записи.

Много читателей могут держать `RLock` одновременно. Писатель с `Lock` получает эксклюзивный доступ.

Для `map` это типичный паттерн:

```go
s.mu.RLock()
value, ok := s.cache[key]
s.mu.RUnlock()

s.mu.Lock()
s.cache[key] = value
s.mu.Unlock()
```

Но mutex полезен только если все обращения к защищаемому состоянию идут через него. Один прямой доступ без lock ломает модель.

## Mutex защищает данные, а не код по магии

Частая ошибка мышления: “в структуре есть mutex, значит структура потокобезопасна”.

Нет. Потокобезопасность появляется только если:

- понятно, какие поля защищены каким mutex;
- все чтения и записи этих полей проходят под этим mutex;
- наружу не возвращаются изменяемые ссылки на внутренние структуры;
- операции, которые должны быть атомарными, действительно выполняются под одной критической секцией или через другой механизм;
- нет копирования структуры с mutex;
- нет вызовов внешнего неизвестного кода под lock, если это может привести к deadlock или долгому удержанию.

Плохо:

```go
type Service struct {
	mu    sync.RWMutex
	cache map[string]Order
	stats map[string]int
}
```

Неясно:

- `mu` защищает только `cache` или еще `stats`;
- можно ли читать `stats` без lock;
- можно ли заменять `cache` целиком;
- нужно ли держать lock при чтении `Order.Items`;
- можно ли возвращать ссылку на значение из cache.

Лучше явно зафиксировать правило:

```go
type Service struct {
	cacheMu sync.RWMutex
	cache   map[string]Order
}
```

Или коротким комментарием:

```go
type Service struct {
	mu    sync.RWMutex // protects cache and stats
	cache map[string]Order
	stats map[string]int
}
```

Комментарии не заменяют код, но помогают ревьюеру и следующему автору.

## Cache access contract

Cache access contract - это правило, как код имеет право работать с кешем.

Примеры контрактов:

1. `cache` доступен только через методы `getCachedOrder`, `setCachedOrder`, `deleteCachedOrder`.
2. `cache` защищен `cacheMu`; любое чтение требует `RLock`, любая запись требует `Lock`.
3. Значения в cache считаются immutable после записи.
4. Метод `GetOrder` может вернуть stale значение до TTL.
5. `RebuildCache` заменяет map целиком, не мутирует ее по одной записи без lock.
6. Кеш не является source of truth; при сомнении читаем из repository.
7. Ошибка cache write не должна ломать бизнес-операцию, если кеш best-effort.
8. Внешний код не получает ссылку на внутреннюю map/slice.

Контракт может быть простым, но он должен быть.

Плохой признак:

```go
s.cache[id] = order
```

разбросано по десяти методам.

Хороший признак:

```go
s.setCachedOrder(order)
```

и прямые обращения к `s.cache` встречаются только в маленьких helper-ах.

На ревью это можно формулировать:

> Я бы сделал cache private не только по имени поля, но и по способу доступа: все операции через helper methods. Сейчас любое новое место может нарушить lock discipline.

## Инкапсуляция shared state

Инкапсуляция - это не “спрятать ради красоты”. Это способ сделать неправильное использование труднее.

Плохо:

```go
func (s *Service) GetOrder(ctx context.Context, id string) (Order, error) {
	s.mu.RLock()
	order, ok := s.cache[id]
	s.mu.RUnlock()
	if ok {
		return order, nil
	}
	...
}

func (s *Service) CreateOrder(ctx context.Context, order Order) error {
	...
	s.cache[order.ID] = order
	return nil
}

func (s *Service) RebuildCache(ctx context.Context) error {
	...
	s.cache[order.ID] = order
	return nil
}
```

Лучше:

```go
func (s *Service) getCachedOrder(id string) (Order, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	order, ok := s.cache[id]
	return order, ok
}

func (s *Service) setCachedOrder(order Order) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cache[order.ID] = order
}

func (s *Service) replaceCache(next map[string]Order) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cache = next
}
```

Теперь в бизнес-методах:

```go
if order, ok := s.getCachedOrder(id); ok {
	return order, nil
}

order, err := s.repo.GetOrder(ctx, id)
if err != nil {
	return Order{}, err
}

s.setCachedOrder(order)
return order, nil
```

Чтение стало проще. Ревьюеру не надо каждый раз проверять `Lock/Unlock`: он проверяет helper один раз.

## Маленькие helper-методы

Полезные helper-ы для кеша:

```go
func (s *Service) getFromCache(id string) (Order, bool)
func (s *Service) setCache(order Order)
func (s *Service) deleteCache(id string)
func (s *Service) replaceCache(next map[string]Order)
func (s *Service) snapshotCache() map[string]Order
```

Для счетчиков:

```go
func (s *Service) incViewed()
func (s *Service) statsSnapshot() Stats
```

Для состояния worker-а:

```go
func (s *Service) markStarted()
func (s *Service) isClosed() bool
```

Но helper-ы должны оставаться маленькими и честными. Если `setCache` вдруг начинает публиковать событие, писать в БД и отправлять метрику, он перестает быть helper-ом доступа к state.

## Важная граница: техническая безопасность и логическая консистентность

Посмотрим на код:

```go
func (s *Service) GetOrder(ctx context.Context, id string) (Order, error) {
	if order, ok := s.getCachedOrder(id); ok {
		return order, nil
	}

	order, err := s.repo.GetOrder(ctx, id)
	if err != nil {
		return Order{}, err
	}

	s.setCachedOrder(order)
	return order, nil
}
```

Этот код технически безопаснее, чем прямой доступ к map. Но между `getCachedOrder` и `setCachedOrder` есть окно:

```text
cache miss
repo.GetOrder
another goroutine updates cache
setCachedOrder overwrites cache
```

То есть data race нет, но возможна логическая гонка: мы можем перезаписать более свежее значение старым.

Это важнейшее различие:

- mutex вокруг map защищает структуру данных от data race;
- он не всегда гарантирует бизнес-консистентность всей операции;
- если нужна атомарность “check then set”, ее надо проектировать отдельно.

Если кеш best-effort, это может быть нормально. Если нельзя перетирать более свежие данные, нужно другое правило:

```go
func (s *Service) setCacheIfAbsent(order Order) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.cache[order.ID]; !ok {
		s.cache[order.ID] = order
	}
}
```

Или с версией:

```go
func (s *Service) setCacheIfNewer(order Order) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.cache[order.ID]
	if !ok || order.UpdatedAt.After(current.UpdatedAt) {
		s.cache[order.ID] = order
	}
}
```

На ревью хорошо сказать:

> Этот вариант убирает data race, но не делает read-through cache атомарным в бизнес-смысле. Если кеш может обновляться из других методов, надо решить, допустим ли stale write или нужен compare-by-version/set-if-absent.

## Не держать lock во время долгой операции

Плохой вариант:

```go
func (s *Service) GetOrder(ctx context.Context, id string) (Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if order, ok := s.cache[id]; ok {
		return order, nil
	}

	order, err := s.repo.GetOrder(ctx, id)
	if err != nil {
		return Order{}, err
	}

	s.cache[id] = order
	return order, nil
}
```

Тут нет data race. Но lock держится во время `repo.GetOrder`, то есть во время I/O.

Последствия:

- все другие читатели/писатели cache ждут БД;
- один медленный запрос блокирует весь кеш;
- растет latency;
- может появиться цепочка ожиданий;
- если внутри repo вызовется код, которому тоже нужен `s.mu`, можно получить deadlock.

Обычно лучше:

1. быстро проверить кеш под `RLock`;
2. отпустить lock;
3. сходить в БД;
4. коротко взять `Lock` и обновить кеш.

Но это возвращает логическое окно между miss и set. Поэтому надо решить, что важнее:

- не держать lock на I/O;
- или обеспечить single-flight/атомарное заполнение.

Для read-through cache часто используют `singleflight`, чтобы несколько запросов на один ключ не били в БД одновременно.

## Singleflight

Проблема cache stampede:

```text
100 запросов одновременно не нашли key в cache
100 запросов одновременно пошли в DB
100 запросов записали один и тот же key
```

Mutex вокруг map не решит это полностью, если мы отпускаем lock перед DB. Держать lock во время DB тоже плохо.

Для этого есть паттерн singleflight:

```go
type Service struct {
	mu    sync.RWMutex
	cache map[string]Order
	group singleflight.Group
}
```

Идея:

- первый запрос по ключу идет в БД;
- остальные ждут результат того же запроса;
- после результата все получают одно значение.

Концептуально:

```go
value, err, _ := s.group.Do(id, func() (any, error) {
	if order, ok := s.getCachedOrder(id); ok {
		return order, nil
	}

	order, err := s.repo.GetOrder(ctx, id)
	if err != nil {
		return Order{}, err
	}

	s.setCachedOrder(order)
	return order, nil
})
```

На собесе не обязательно писать `singleflight` с нуля. Достаточно назвать проблему:

> Даже если map защищена mutex-ом, при cache miss несколько горутин могут одновременно пойти в БД. Если это важно, я бы использовал singleflight или per-key locking.

## Per-key locks

Иногда глобальный mutex слишком грубый. Например кеш большой, ключей много, и операции по разным ключам не должны блокировать друг друга.

Можно использовать per-key lock:

```go
type Service struct {
	cacheMu sync.RWMutex
	cache   map[string]Order

	locksMu sync.Mutex
	locks   map[string]*sync.Mutex
}
```

Но это усложняет код:

- надо чистить старые lock-и;
- можно получить leak map-ы lock-ов;
- легко ошибиться с порядком lock-ов;
- сложнее ревьюить.

Для middle/middle+ чаще достаточно сказать: “если будет проблема stampede или contention, можно рассмотреть singleflight/per-key locking, но базово я бы начал с простого cache contract”.

## RWMutex: когда полезен, когда нет

`RWMutex` полезен, если:

- чтений сильно больше, чем записей;
- чтения короткие;
- записи редкие;
- critical section не содержит долгого I/O;
- нет постоянного write contention.

`RWMutex` может не дать пользы, если:

- операций записи много;
- critical sections маленькие, и overhead важнее;
- логика сложная и легко перепутать RLock/Lock;
- под `RLock` случайно делается запись;
- данные нужно часто заменять целиком.

В простых случаях `sync.Mutex` может быть лучше:

```go
type Service struct {
	mu    sync.Mutex
	cache map[string]Order
}
```

На ревью не надо автоматически требовать `RWMutex`. Лучше спросить:

> Здесь действительно много параллельных чтений и мало записей? Если нет, обычный Mutex будет проще.

## Правило: не копировать mutex

`sync.Mutex` и `sync.RWMutex` нельзя копировать после первого использования.

Плохо:

```go
type Cache struct {
	mu sync.Mutex
	m  map[string]Order
}

func (c Cache) Set(id string, order Order) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[id] = order
}
```

Метод с value receiver копирует `Cache`, включая mutex.

Нужно:

```go
func (c *Cache) Set(id string, order Order) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[id] = order
}
```

Также плохо:

```go
cache2 := cache1
```

если `cache1` уже использовался.

Инструмент `go vet` часто ловит copylocks.

На ревью:

> У структуры есть mutex, поэтому методы должны быть pointer receiver, и саму структуру нельзя копировать после использования.

## Не возвращать внутренние map/slice наружу

Даже если доступ к map защищен внутри, можно случайно отдать наружу изменяемую ссылку.

Плохо:

```go
func (s *Service) AllOrders() map[string]Order {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.cache
}
```

Caller теперь может сделать:

```go
orders := s.AllOrders()
orders["x"] = order
```

без lock-а.

Нужно возвращать копию:

```go
func (s *Service) CacheSnapshot() map[string]Order {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]Order, len(s.cache))
	for k, v := range s.cache {
		result[k] = v
	}
	return result
}
```

Со slice похожая история:

```go
func (s *Service) Items(id string) []Item {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.cache[id].Items
}
```

Если `Items` - slice, caller может изменить underlying array.

Лучше копировать:

```go
items := append([]Item(nil), s.cache[id].Items...)
```

## Значения в cache: immutable или mutable

Если в cache хранится структура с slice/map/pointer внутри:

```go
type Order struct {
	ID    string
	Items []Item
	Meta  map[string]string
}
```

Даже если саму `map[string]Order` защищать mutex-ом, внутренние поля могут оставаться изменяемыми.

Пример:

```go
order, _ := s.getCachedOrder(id)
order.Items[0].Quantity = 100
```

Если `Items` ссылается на тот же underlying array, который лежит в cache, внешняя мутация может изменить кеш без lock.

Контракт должен сказать:

- значения immutable после записи;
- или getter возвращает deep copy;
- или все мутации значений происходят под lock;
- или в cache хранятся value objects без изменяемых вложенных структур.

На ревью:

> Мы защищаем map, но нужно проверить, не возвращаем ли наружу mutable содержимое значения. Если Order содержит slices/maps, shallow copy может быть недостаточен.

## Заменять map целиком при rebuild

Плохой вариант:

```go
func (s *Service) RebuildCache(ctx context.Context) error {
	orders, err := s.repo.ListOrders(ctx)
	if err != nil {
		return err
	}

	for _, order := range orders {
		s.mu.Lock()
		s.cache[order.ID] = order
		s.mu.Unlock()
	}

	return nil
}
```

Проблемы:

- читатели видят частично перестроенный кеш;
- старые записи, которых больше нет, могут остаться;
- много lock/unlock;
- если rebuild должен быть атомарным, контракт нарушен.

Лучше:

```go
func (s *Service) RebuildCache(ctx context.Context) error {
	orders, err := s.repo.ListOrders(ctx)
	if err != nil {
		return err
	}

	next := make(map[string]Order, len(orders))
	for _, order := range orders {
		next[order.ID] = order
	}

	s.mu.Lock()
	s.cache = next
	s.mu.Unlock()

	return nil
}
```

Так читатели либо видят старый кеш, либо новый. Не видят промежуточное состояние.

Если rebuild только для одного пользователя, можно собрать `nextUserOrders`, затем под lock удалить старые записи пользователя и вставить новые одной критической секцией.

## Lock ordering и deadlock

Если в сервисе несколько mutex-ов, важен порядок.

Плохо:

```go
func (s *Service) A() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	s.statsMu.Lock()
	defer s.statsMu.Unlock()
}

func (s *Service) B() {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()

	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
}
```

Горутина 1 держит `cacheMu` и ждет `statsMu`.
Горутина 2 держит `statsMu` и ждет `cacheMu`.

Deadlock.

Контракт должен включать порядок:

```go
// Lock order: cacheMu before statsMu.
```

Но лучше вообще избегать необходимости держать несколько lock-ов одновременно.

## Не вызывать чужой код под lock

Опасно:

```go
s.mu.Lock()
defer s.mu.Unlock()

s.events.Publish(ctx, event)
```

Почему:

- внешний вызов может быть долгим;
- может вызвать callback, который снова зайдет в сервис;
- может зависнуть;
- может создать deadlock;
- lock будет блокировать unrelated операции.

Под lock лучше делать только быстрые операции с памятью:

```go
s.mu.Lock()
event := s.buildEventLocked()
s.mu.Unlock()

s.events.Publish(ctx, event)
```

Если нужно снять snapshot:

```go
s.mu.RLock()
snapshot := s.snapshotLocked()
s.mu.RUnlock()

return s.render(snapshot)
```

## defer Unlock: хорошо, но не всегда идеально

Обычно:

```go
s.mu.Lock()
defer s.mu.Unlock()
```

Это безопасно и читаемо.

Но в горячих местах или в функциях с долгим хвостом лучше явно сужать critical section:

```go
s.mu.Lock()
value := s.cache[id]
s.mu.Unlock()

return expensiveFormat(value)
```

Не надо держать lock дольше нужного только из-за удобства `defer`.

На ревью:

> Lock scope можно сузить: под mutex надо только прочитать/записать shared state, а форматирование/внешние вызовы лучше делать после unlock.

## sync.Map

`sync.Map` иногда используют вместо `map + mutex`.

Она полезна, когда:

- ключи пишутся один раз, читаются много раз;
- разные горутины работают с независимыми ключами;
- хочется избежать глобального lock contention;
- паттерн похож на cache of immutable entries.

Но `sync.Map` не является универсальной заменой.

Минусы:

- слабее типизация: key/value как `any`;
- сложнее поддерживать инварианты между несколькими ключами;
- нельзя легко сделать атомарную замену всей map;
- логика часто становится менее читаемой;
- не решает бизнес-консистентность.

Для обычного сервисного кеша `map + mutex` чаще проще и понятнее.

На ревью:

> Я бы не заменял это автоматически на sync.Map. Если нужен простой cache с понятным контрактом, map+mutex читается лучше. sync.Map стоит рассматривать при специфическом access pattern.

## Atomic primitives

Для простых счетчиков можно использовать `sync/atomic`:

```go
var requests atomic.Int64
requests.Add(1)
count := requests.Load()
```

Но atomic не подходит, если нужно согласованно обновить несколько полей:

```go
viewCount++
lastSeen = now
```

Если эти значения должны быть согласованы, лучше один mutex и snapshot:

```go
type Stats struct {
	ViewCount int64
	LastSeen  time.Time
}

func (s *Service) markViewed(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stats.ViewCount++
	s.stats.LastSeen = now
}

func (s *Service) Stats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.stats
}
```

На ревью:

> Atomic подойдет для независимого счетчика, но если несколько полей должны читаться как консистентный snapshot, нужен mutex или отдельная immutable snapshot-модель.

## Copy-on-write и atomic.Value

Для конфигурации или редко обновляемых snapshot-ов полезна модель copy-on-write:

```go
type Service struct {
	config atomic.Value // stores Config
}
```

При обновлении создается новый `Config`, затем атомарно публикуется. Читатели получают immutable snapshot.

Это хорошо для:

- runtime config;
- routing tables;
- feature flags snapshot;
- lookup tables;
- редко обновляемых справочников.

Но для обычного mutable cache с частыми точечными обновлениями это может быть неудобно.

На ревью достаточно понимать:

> Если нужно много читателей и редкие атомарные замены всего snapshot-а, можно рассмотреть copy-on-write/atomic.Value. Для обычного map cache проще mutex.

## Cache-aside contract

Распространенный паттерн:

```go
func (s *Service) Get(ctx context.Context, id string) (Order, error) {
	if order, ok := s.cache.Get(id); ok {
		return order, nil
	}

	order, err := s.repo.Get(ctx, id)
	if err != nil {
		return Order{}, err
	}

	s.cache.Set(id, order)
	return order, nil
}
```

Вопросы контракта:

- что делать при ошибке cache get;
- что делать при ошибке cache set;
- можно ли вернуть stale значение;
- есть ли TTL;
- кто инвалидирует при write;
- что делать при concurrent miss;
- можно ли cache stampede;
- что является source of truth;
- содержит ли кеш персональные/секретные данные;
- нужно ли копировать значения.

Для in-memory map cache внутри одного сервиса особенно важно:

- каждый instance имеет свой кеш;
- после restart кеш пустой;
- в multi-instance deployment кеши расходятся;
- invalidation между instance-ами не происходит без отдельного механизма.

На ревью:

> Этот in-memory cache работает только в рамках одного процесса. Если сервис масштабирован в несколько replicas, надо понимать, допустима ли рассинхронизация между instance-ами.

## Типовые ошибки

### Ошибка 1. Mutex есть, но используется не везде

Пример:

```go
func (s *Service) Get(id string) (Order, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache[id], true
}

func (s *Service) Set(order Order) {
	s.cache[order.ID] = order
}
```

Что не так:

- чтение защищено, запись нет;
- race detector найдет data race;
- map может упасть с `fatal error: concurrent map read and map write`;
- наличие mutex создает ложное чувство безопасности.

Как лучше:

- все операции через helper-ы;
- прямой доступ только в helper-ах;
- добавить ревью-правило: `s.cache` не трогать напрямую.

### Ошибка 2. Lock защищает чтение map, но возвращает mutable внутренность

Пример:

```go
func (s *Service) Get(id string) ([]Item, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	order, ok := s.cache[id]
	return order.Items, ok
}
```

Caller может изменить `Items`.

Как лучше:

- вернуть копию slice;
- хранить immutable value;
- документировать и обеспечить immutability.

### Ошибка 3. Долгая операция под lock

Пример:

```go
s.mu.Lock()
defer s.mu.Unlock()

order, err := s.repo.GetOrder(ctx, id)
```

Что не так:

- lock держится на I/O;
- блокирует другие операции;
- риск latency spike;
- может привести к deadlock при обратных вызовах.

Как лучше:

- lock только вокруг map;
- для cache miss использовать singleflight, если важно.

### Ошибка 4. Check-then-act не атомарен

Пример:

```go
if _, ok := s.cache[id]; !ok {
	s.cache[id] = order
}
```

Если это без lock - data race.

Если check и set в разных helper-ах:

```go
if _, ok := s.get(id); !ok {
	s.set(order)
}
```

data race нет, но операция не атомарна. Между get и set могло измениться состояние.

Как лучше:

```go
func (s *Service) setIfAbsent(order Order) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.cache[order.ID]; ok {
		return false
	}
	s.cache[order.ID] = order
	return true
}
```

### Ошибка 5. Rebuild мутирует кеш по одной записи

Пример:

```go
for _, item := range items {
	s.set(item)
}
```

Если rebuild должен быть атомарным, читатели увидят промежуточное состояние.

Как лучше:

- собрать новую map локально;
- заменить ссылку под lock;
- удалить старые записи, если rebuild частичный.

### Ошибка 6. Ошибка в lock/unlock pair

Плохо:

```go
s.mu.Lock()
if err != nil {
	return err
}
s.mu.Unlock()
```

При раннем `return` lock не освободится.

Лучше:

```go
s.mu.Lock()
defer s.mu.Unlock()
```

или явно с маленьким scope:

```go
s.mu.Lock()
value := s.cache[id]
s.mu.Unlock()
```

### Ошибка 7. RLock используется для записи

Плохо:

```go
s.mu.RLock()
defer s.mu.RUnlock()

s.cache[id] = order
```

`RLock` разрешает параллельных читателей, но не защищает запись. Для записи нужен `Lock`.

### Ошибка 8. Несколько mutex-ов без порядка

Проблема deadlock при разном порядке захвата.

Как лучше:

- избегать вложенных lock-ов;
- если нужно - документировать порядок;
- сделать один mutex для связанных полей;
- использовать snapshot и отпускать lock до второй операции.

### Ошибка 9. Copying lock

Плохо:

```go
func (s Service) Get(id string) Order
```

если `Service` содержит mutex.

Как лучше:

```go
func (s *Service) Get(id string) Order
```

и не копировать саму структуру.

### Ошибка 10. Наружу отдается pointer на внутреннее состояние

Плохо:

```go
func (s *Service) GetConfig() *Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}
```

Caller может изменить `config` без lock.

Лучше:

- вернуть копию;
- сделать `Config` immutable;
- вернуть read-only interface, если применимо.

### Ошибка 11. Кеш обновляется без связи с write path

Пример:

```go
func (s *Service) UpdateOrder(ctx context.Context, order Order) error {
	if err := s.repo.Save(ctx, order); err != nil {
		return err
	}
	return nil
}
```

А `GetOrder` читает из cache. После update кеш может остаться старым.

Как лучше:

- invalidate cache после успешного write;
- update cache after commit;
- использовать TTL;
- version check;
- event-driven invalidation.

Это уже не data race, а cache consistency.

### Ошибка 12. Cache write считается критичным без решения

Пример:

```go
if err := s.cache.Set(ctx, key, value); err != nil {
	return err
}
```

Если кеш best-effort, операция не должна падать из-за cache miss/set failure.

Но если кеш - часть бизнес-логики, это уже не просто кеш, а хранилище состояния, и контракт должен быть другим.

На ревью вопрос:

> Cache failure должен ломать use case или только логироваться? Что является source of truth?

## Как избегать проблем

### Правило 1. Назвать, что защищает mutex

Плохо:

```go
mu sync.Mutex
```

и рядом 10 полей.

Лучше:

```go
cacheMu sync.RWMutex
cache   map[string]Order
```

или:

```go
mu sync.RWMutex // protects cache and stats
```

### Правило 2. Прятать shared state за helper-ами

Не размазывать:

```go
s.cache[id] = order
```

по всему сервису.

Ввести:

```go
s.setCachedOrder(order)
```

и считать прямой доступ smell-ом.

### Правило 3. Держать lock scope коротким

Под lock:

- прочитать map;
- записать map;
- скопировать snapshot;
- заменить ссылку;
- обновить маленький счетчик.

Не под lock:

- DB;
- HTTP;
- Publish;
- SendEmail;
- time.Sleep;
- тяжелое форматирование;
- вызов неизвестного callback-а.

### Правило 4. Отдельно проектировать атомарные операции

Если нужна операция “если нет - вставить”, она должна быть одним helper-ом под одним lock:

```go
setIfAbsent
setIfNewer
compareAndSwap
replaceAll
deleteAndSet
```

Не собирать атомарную операцию из двух безопасных helper-ов, если между ними может вклиниться другая горутина.

### Правило 5. Не отдавать внутренности наружу

Возвращать:

- value copy;
- snapshot map;
- copied slice;
- immutable object.

Не возвращать:

- внутреннюю map;
- slice на внутренний array;
- pointer на mutable внутренний объект;
- структуру с mutable вложенными map/slice без копирования.

### Правило 6. Разделять thread safety и consistency

Вопросы:

- нет ли data race?
- может ли кеш стать устаревшим?
- может ли stale write перезаписать fresh write?
- видят ли читатели частичный rebuild?
- что происходит при concurrent update?
- допустима ли eventual consistency?

Mutex отвечает только на часть этих вопросов.

### Правило 7. Документировать cache policy

Даже коротко:

```go
// cache is best-effort and may contain stale orders for up to orderCacheTTL.
```

или:

```go
// cache is updated only after successful DB commit.
```

или:

```go
// values stored in cache must be immutable.
```

Такие комментарии полезны, потому что они фиксируют архитектурное решение.

## Как диагностировать на ревью

### Шаг 1. Найти все shared fields

Ищи в struct:

```go
map
slice
pointer
counter
time.Time
bool flags
channels
worker state
cache
stats
```

Спроси:

- может ли этот service жить дольше одного запроса;
- вызывается ли он параллельно;
- какие поля меняются после constructor-а.

### Шаг 2. Найти mutex и понять ownership

Спроси:

- какой mutex защищает какое поле;
- есть ли несколько mutex-ов;
- есть ли комментарий;
- используются ли pointer receivers;
- не копируется ли struct.

### Шаг 3. Найти все обращения к shared state

Поиск:

```text
s.cache
s.stats
s.lastSeen
s.started
s.closed
```

Для каждого места:

- чтение или запись;
- под lock или нет;
- правильный lock mode;
- не возвращается ли ссылка наружу;
- нет ли long operation под lock.

### Шаг 4. Проверить helper contract

Если helper-ы есть:

- все ли используют их;
- достаточно ли они атомарны;
- не делают ли лишние side effects;
- не возвращают ли mutable internal state.

Если helper-ов нет:

- стоит ли предложить их.

### Шаг 5. Проверить logical races

Ищи паттерны:

```go
if exists {
	...
}
set(...)
```

```go
get under lock
external call
set under lock
```

```go
check status
do something
update status
```

Спроси:

- что если другая горутина изменит состояние между шагами;
- это допустимо;
- нужна ли версия;
- нужен ли compare-and-set;
- нужен ли singleflight.

### Шаг 6. Проверить rebuild/invalidation

Вопросы:

- rebuild атомарный или частичный;
- старые записи удаляются;
- читатели видят промежуточное состояние;
- write path инвалидирует кеш;
- TTL есть;
- multi-instance рассинхронизация допустима.

### Шаг 7. Проверить lock scope

Внутри lock не должно быть:

- repository calls;
- network;
- publish;
- logging с потенциально медленным writer, если это hot path;
- callback;
- sleep;
- wait group wait;
- channel send, который может заблокироваться.

Канал под lock тоже опасен:

```go
s.mu.Lock()
s.jobs <- job
s.mu.Unlock()
```

Если send заблокируется, lock останется удержанным.

## Как говорить на собеседовании

Про непоследовательный mutex:

> Здесь есть `mu`, и часть доступа к cache идет под lock, но в других местах map пишется напрямую. Это нарушает lock discipline: один незащищенный доступ уже ломает потокобезопасность всей структуры.

Про инкапсуляцию:

> Я бы спрятал cache за helper methods: `getCached`, `setCached`, `replaceCache`. Тогда правило синхронизации проверяется в одном месте, а бизнес-методы не трогают map напрямую.

Про technical vs logical race:

> Раздельные get/set helper-ы убирают data race, но не всегда убирают логическую гонку. Между cache miss и записью результата другая горутина может положить более свежее значение. Если это важно, нужен `setIfAbsent`, версия или singleflight.

Про lock scope:

> Не стоит держать mutex во время DB/HTTP вызова. Lock должен защищать короткую работу с памятью. Иначе один медленный внешний вызов блокирует все операции с кешем.

Про mutable values:

> Даже если map защищена, надо проверить, не возвращаем ли наружу slice/map/pointer из значения. Иначе caller сможет изменить внутреннее состояние без lock.

Про rebuild:

> Если rebuild должен быть атомарным, лучше собрать новую map локально и заменить ссылку под lock. Иначе читатели могут увидеть частично перестроенный кеш, а старые записи могут остаться.

Про source of truth:

> Надо явно решить, кеш best-effort или часть бизнес-контракта. Если source of truth - БД, cache failure обычно не должен ломать use case, а stale values должны быть ограничены TTL/invalidations.

## Пример плохого кода

```go
type ProductService struct {
	repo  ProductRepository
	mu    sync.RWMutex
	cache map[string]Product
	stats map[string]int
}

func (s *ProductService) GetProduct(ctx context.Context, id string) (Product, error) {
	s.mu.RLock()
	product, ok := s.cache[id]
	s.mu.RUnlock()
	if ok {
		s.stats["hits"]++
		return product, nil
	}

	product, err := s.repo.GetProduct(ctx, id)
	if err != nil {
		return Product{}, err
	}

	s.mu.Lock()
	s.cache[id] = product
	s.mu.Unlock()

	return product, nil
}

func (s *ProductService) UpdateProduct(ctx context.Context, product Product) error {
	if err := s.repo.SaveProduct(ctx, product); err != nil {
		return err
	}

	s.cache[product.ID] = product
	s.stats["updates"]++
	return nil
}

func (s *ProductService) AllCached() map[string]Product {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.cache
}
```

Проблемы:

- `stats["hits"]++` без lock;
- `UpdateProduct` пишет `cache` без lock;
- `stats["updates"]++` без lock;
- `AllCached` возвращает внутреннюю map наружу;
- неясно, `mu` защищает только cache или stats тоже;
- `GetProduct` может перезаписать свежий update устаревшим значением после cache miss;
- нет cache invalidation/version policy.

## Более здоровое направление

```go
type ProductService struct {
	repo ProductRepository

	mu    sync.RWMutex // protects cache and stats
	cache map[string]Product
	stats CacheStats
}

type CacheStats struct {
	Hits    int
	Misses  int
	Updates int
}

func (s *ProductService) getCachedProduct(id string) (Product, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	product, ok := s.cache[id]
	return product, ok
}

func (s *ProductService) setCachedProduct(product Product) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cache[product.ID] = product
}

func (s *ProductService) recordHit() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stats.Hits++
}

func (s *ProductService) statsSnapshot() CacheStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.stats
}

func (s *ProductService) cachedSnapshot() map[string]Product {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]Product, len(s.cache))
	for id, product := range s.cache {
		result[id] = product
	}
	return result
}
```

Это не идеальный финальный дизайн для любого проекта. Но он показывает главное:

- контракт доступа локализован;
- внешнему коду не отдается map;
- stats тоже защищены;
- по коду видно, что mutex защищает;
- ревьюер проверяет helper-ы, а не весь сервис на 500 строк.

## Мини-чеклист для ревью

- Какие поля являются shared state?
- Какой mutex защищает какие поля?
- Все чтения и записи идут под lock?
- Правильный режим: `RLock` для чтения, `Lock` для записи?
- Нет ли прямого доступа к map в обход helper-ов?
- Не копируется ли struct с mutex?
- Все методы с mutex имеют pointer receiver?
- Не возвращается ли внутренняя map/slice/pointer наружу?
- Значения в cache immutable или копируются?
- Не держится ли lock во время DB/HTTP/broker/cache external call?
- Нет ли channel send/receive под lock?
- Нет ли nested locks с разным порядком?
- Rebuild cache атомарный?
- Старые записи удаляются при rebuild?
- Не может ли stale write перезаписать fresh value?
- Нужен ли `setIfAbsent`, `setIfNewer`, version или singleflight?
- Что является source of truth?
- Cache failure критичен или best-effort?
- Есть ли TTL/eviction/invalidation?
- Работает ли контракт при нескольких service instances?
- Можно ли поймать проблему через `go test -race`?

## Вопросы автору кода

- Какой контракт доступа к этому cache?
- Какие поля защищает этот mutex?
- Почему здесь прямой доступ к `s.cache`, а не helper?
- Может ли этот метод вызываться параллельно из нескольких запросов?
- Можно ли вернуть stale значение из cache?
- Что произойдет при concurrent update и cache miss?
- Нужно ли защищать не только map, но и значения внутри нее?
- Можно ли caller изменить возвращенный slice/map?
- Почему lock держится во время внешнего вызова?
- Rebuild должен быть атомарным или частичное состояние допустимо?
- Что будет при cache set failure?
- Надо ли инвалидировать cache после write?
- Этот in-memory cache допустим при нескольких replicas?
- Нужен ли здесь RWMutex или обычный Mutex проще?
- Нужен ли singleflight от cache stampede?

## Короткое резюме

Сильный ответ по этой теме звучит не как “надо добавить mutex”, а так:

> Сейчас у shared state нет надежного access contract. Часть обращений к cache идет под mutex, часть напрямую. Я бы инкапсулировал cache за helper methods, явно указал, какие поля защищает mutex, не возвращал mutable внутренности наружу и отдельно решил вопрос логической консистентности: допустимы ли stale writes, нужен ли version/set-if-newer/singleflight, как работает invalidation.

Главные идеи:

- mutex должен защищать конкретные поля по понятному правилу;
- все обращения должны соблюдать одно правило;
- helper-методы уменьшают шанс обхода lock-а;
- lock scope должен быть коротким;
- нельзя отдавать наружу внутренние map/slice/pointer;
- техническая thread safety не равна бизнес-консистентности;
- cache contract должен описывать source of truth, stale data, invalidation и failure policy.
