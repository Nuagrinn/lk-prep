# Мапы в Go: прикладное использование и ревью кода

Мапы в Go - одна из тех базовых тем, где на собеседовании легко проверить и синтаксис, и понимание устройства языка, и практическую инженерную аккуратность. Обычно спрашивают:

- что такое map;
- как создать map;
- что будет при чтении из nil map;
- что будет при записи в nil map;
- как проверить наличие ключа;
- почему порядок обхода не гарантирован;
- какие типы могут быть ключами;
- можно ли конкурентно читать/писать map;
- что выведет код с map;
- всегда ли hash map быстрее списка/массива/таблицы;
- когда map не лучший выбор.

Главная мысль:

> Map в Go - это встроенная hash table: структура для связи ключа и значения, с быстрым средним доступом по ключу, но без гарантии порядка обхода и без потокобезопасности для конкурентных записей.

И вторая мысль:

> Map выглядит простой, но в ревью важно проверять nil map, zero value ambiguity, concurrent access, mutable values, порядок обхода и то, подходит ли map как структура данных для задачи.

## Подкапотная база

Глубокий разбор устройства map вынесен в [internals.md](internals.md): hash, коллизии, старая bucket/overflow модель, Swiss Table в Go 1.24+, probing, tombstones, рост map, память и вопрос `hash != address`.

Этот файл остается прикладным конспектом: синтаксис, nil map, `ok`, порядок обхода, ключи, конкурентный доступ, типовые ошибки на ревью и готовые ответы для собеседования.

## Простое введение

Map хранит пары ключ-значение:

```go
ages := map[string]int{
	"Alice": 30,
	"Bob":   25,
}

fmt.Println(ages["Alice"]) // 30
```

Тип:

```go
map[KeyType]ValueType
```

Примеры:

```go
map[string]int
map[int]string
map[UserID]User
map[string][]Order
map[string]struct{}
```

Map удобна, когда нужно быстро находить значение по ключу:

- пользователь по ID;
- настройки по имени;
- счетчик по категории;
- множество уникальных значений;
- группировка объектов;
- cache;
- lookup table.

## Создание map

### Literal

```go
m := map[string]int{
	"a": 1,
	"b": 2,
}
```

### make

```go
m := make(map[string]int)
m["a"] = 1
```

С hint capacity:

```go
m := make(map[string]int, 1000)
```

Второй аргумент - не фиксированная capacity как у slice, а подсказка runtime, сколько элементов примерно ожидается. Map может расти дальше.

### var nil map

```go
var m map[string]int
```

Это nil map.

```go
fmt.Println(m == nil) // true
fmt.Println(len(m))   // 0
```

Читать из nil map можно:

```go
fmt.Println(m["x"]) // 0
```

Записывать нельзя:

```go
m["x"] = 1 // panic: assignment to entry in nil map
```

Чтобы писать:

```go
m = make(map[string]int)
m["x"] = 1
```

На собесе:

> Nil map можно читать, `len` работает, `delete` безопасен, но запись в nil map вызывает panic.

## Базовые операции

### Запись

```go
m["name"] = 10
```

### Чтение

```go
value := m["name"]
```

Если ключа нет, вернется zero value типа значения.

```go
m := map[string]int{}
fmt.Println(m["missing"]) // 0
```

### Проверка наличия ключа

```go
value, ok := m["name"]
if !ok {
	// key not found
}
```

Это называется comma-ok idiom.

Важно:

```go
m := map[string]int{"a": 0}

fmt.Println(m["a"]) // 0
fmt.Println(m["b"]) // 0
```

По одному значению нельзя понять, есть ключ или нет. Нужен `ok`.

На ревью:

> Если zero value является валидным значением, чтение из map без `ok` может скрыть отсутствие ключа.

### Удаление

```go
delete(m, "name")
```

Если ключа нет - ничего не произойдет.

`delete` по nil map безопасен:

```go
var m map[string]int
delete(m, "x") // ok
```

### Длина

```go
len(m)
```

Возвращает количество пар ключ-значение.

## Map как reference-like type

Map передается в функцию по значению, но значение map - это descriptor/runtime header, который указывает на внутреннюю hash table.

```go
func change(m map[string]int) {
	m["a"] = 100
}

func main() {
	m := map[string]int{"a": 1}
	change(m)
	fmt.Println(m["a"]) // 100
}
```

Почему изменение видно? Потому что копируется header map, но он указывает на ту же runtime-структуру.

Если внутри функции присвоить новую map:

```go
func replace(m map[string]int) {
	m = map[string]int{"x": 1}
}

func main() {
	m := map[string]int{"a": 1}
	replace(m)
	fmt.Println(m) // map[a:1]
}
```

Внешняя переменная не изменилась, потому что изменили только локальную копию map header.

Если нужно заменить map целиком:

```go
func replace(m *map[string]int) {
	*m = map[string]int{"x": 1}
}
```

Но указатель на map нужен редко. Обычно лучше вернуть новую map:

```go
func replaced() map[string]int {
	return map[string]int{"x": 1}
}
```

На собесе:

> Map, как slice, передается по значению, но содержит ссылку на runtime hash table. Поэтому изменение записей видно снаружи, а присваивание новой map локальной переменной - нет.

## Скорость доступа

Обычно операции map:

- lookup: O(1) average;
- insert: O(1) average;
- delete: O(1) average;
- iteration: O(n).

Но “O(1)” не значит “всегда быстрее всего”.

Hash map имеет overhead:

- вычисление hash;
- доступ к внутренним table/group/slots;
- возможные коллизии;
- probe sequence;
- реальные сравнения ключей после H2-кандидатов;
- возможный pointer chasing для больших/непрямых ключей и значений;
- плохая cache locality по сравнению со slice/array;
- память на groups/control bytes;
- рост, rehash или split table.

Для маленького количества элементов линейный поиск по slice может быть быстрее:

```go
items := []Pair{{"a", 1}, {"b", 2}, {"c", 3}}
```

Почему?

- slice лежит компактно в памяти;
- CPU cache работает хорошо;
- нет hash overhead;
- всего 3-10 элементов проще пройти циклом.

На собесе на вопрос “всегда ли хэширование быстрее связанной таблицы/списка/массива”:

> Нет. В среднем map дает быстрый доступ по ключу, но для маленьких наборов или когда важен порядок/локальность, slice/array может быть быстрее и проще. Нужно учитывать размер данных, тип ключа, частоту операций, порядок, память и cache locality.

## Порядок обхода map

Порядок `range` по map не гарантирован.

```go
m := map[string]int{
	"a": 1,
	"b": 2,
	"c": 3,
}

for k, v := range m {
	fmt.Println(k, v)
}
```

Нельзя ожидать:

```text
a 1
b 2
c 3
```

Go специально не обещает порядок. Он может отличаться:

- между запусками;
- между итерациями;
- после вставок/удалений;
- между версиями Go.

Если нужен стабильный порядок:

```go
keys := make([]string, 0, len(m))
for k := range m {
	keys = append(keys, k)
}
sort.Strings(keys)

for _, k := range keys {
	fmt.Println(k, m[k])
}
```

На ревью:

> Если результат зависит от порядка обхода map, это баг. Нужно явно собрать ключи и отсортировать или использовать другую структуру.

## Какие типы могут быть ключами

Ключ map должен быть comparable.

Можно:

- bool;
- int, int64, uint;
- string;
- pointers;
- channels;
- interfaces, если динамическое значение comparable;
- structs, если все поля comparable;
- arrays, если элементы comparable;
- custom types на базе comparable типов.

Нельзя:

- slices;
- maps;
- functions;
- structs с slice/map/function полями;
- arrays с non-comparable элементами.

Пример:

```go
type Point struct {
	X int
	Y int
}

m := map[Point]string{
	{1, 2}: "a",
}
```

Можно, потому что `Point` comparable.

Нельзя:

```go
type User struct {
	ID   string
	Tags []string
}

m := map[User]int{} // compile error
```

Потому что `Tags []string` не comparable.

### Interface ключи

```go
m := map[any]string{}
m["x"] = "ok"
m[10] = "ok"
```

Но если динамическое значение не comparable:

```go
m[[]int{1, 2}] = "boom"
```

Будет panic:

```text
panic: runtime error: hash of unhashable type []int
```

На ревью:

> Map с ключом `any` опасна: compile-time не проверит comparable динамического значения.

## Почему нельзя взять адрес элемента map

Плохо:

```go
m := map[string]User{"a": {Name: "Alice"}}
p := &m["a"] // compile error
```

Нельзя, потому что runtime может менять внутреннее расположение элементов map при росте, rehash или split table. Адрес элемента не стабилен.

Если надо изменить поле struct в map:

```go
m["a"].Name = "Bob" // compile error
```

Нужно:

```go
u := m["a"]
u.Name = "Bob"
m["a"] = u
```

Или хранить указатели:

```go
m := map[string]*User{"a": {Name: "Alice"}}
m["a"].Name = "Bob"
```

Но map указателей имеет свои trade-offs:

- легче мутировать;
- можно получить nil pointer;
- сложнее ownership;
- объекты живут отдельно в памяти;
- может быть больше pressure на GC.

На собесе:

> Элементы map не addressable, потому что runtime может менять внутреннее расположение элементов при росте map. Поэтому поле struct-значения в map нельзя изменить напрямую: надо достать, изменить копию и записать обратно, либо хранить pointer.

## Map value: struct vs pointer

### Значение

```go
map[string]User
```

Плюсы:

- значения хранятся как values;
- нет nil pointer;
- проще immutable-модель;
- меньше случайных мутаций через shared pointer.

Минусы:

- при чтении получаем копию;
- чтобы изменить поле, надо записать обратно;
- большие struct копируются;
- нельзя взять адрес элемента.

### Указатель

```go
map[string]*User
```

Плюсы:

- можно менять поля напрямую;
- не копируем большую struct;
- удобно для shared entities.

Минусы:

- nil values;
- shared mutable state;
- race risks;
- больше объектов для GC;
- сложнее понимать ownership.

На ревью:

> Если map хранит pointers, надо проверить ownership и concurrency. Если map хранит values, надо помнить, что чтение возвращает копию и изменения полей надо записывать обратно.

## Nil map

Nil map:

```go
var m map[string]int
```

Можно:

```go
len(m)
value := m["x"]
value, ok := m["x"]
delete(m, "x")
for k, v := range m {}
```

Нельзя:

```go
m["x"] = 1
```

panic.

Частый баг:

```go
type Cache struct {
	items map[string]Item
}

func (c *Cache) Set(id string, item Item) {
	c.items[id] = item // panic if items nil
}
```

Нужно инициализировать:

```go
func NewCache() *Cache {
	return &Cache{
		items: make(map[string]Item),
	}
}
```

Или lazy init:

```go
func (c *Cache) Set(id string, item Item) {
	if c.items == nil {
		c.items = make(map[string]Item)
	}
	c.items[id] = item
}
```

Lazy init в concurrent-коде требует mutex.

## Empty map vs nil map

```go
var nilMap map[string]int
emptyMap := map[string]int{}
madeMap := make(map[string]int)
```

Все имеют `len == 0`.

```go
fmt.Println(nilMap == nil)   // true
fmt.Println(emptyMap == nil) // false
fmt.Println(madeMap == nil)  // false
```

Запись:

```go
emptyMap["x"] = 1 // ok
madeMap["x"] = 1  // ok
nilMap["x"] = 1   // panic
```

Для JSON:

```go
type Response struct {
	Items map[string]int `json:"items"`
}
```

Nil map обычно кодируется как:

```json
{"items":null}
```

Empty map:

```json
{"items":{}}
```

Для API-контрактов это может быть важно.

## Map и JSON

Map с string keys хорошо кодируется в JSON object:

```go
map[string]int{"a": 1}
```

```json
{"a":1}
```

Если ключи не строки, encoding/json имеет правила для некоторых типов, но для API лучше явно думать о контракте.

Важно:

- порядок полей JSON object не должен быть смысловым;
- если нужен массив в определенном порядке, map не подходит;
- nil map vs empty map может дать `null` vs `{}`.

## Map и set

В Go нет встроенного set, часто используют map.

```go
set := make(map[string]struct{})
set["a"] = struct{}{}

if _, ok := set["a"]; ok {
	// exists
}
```

Почему `struct{}`?

```go
map[string]struct{}
```

Zero-size value, не несет данных.

Иногда используют:

```go
map[string]bool
```

Плюсы:

- проще читать;
- можно писать `if set[x]`.

Минус:

- `false` и отсутствие ключа различаются только через `ok`;
- занимает значение bool;
- семантически слабее.

Сравнение:

```go
seen := map[string]bool{}
if seen[id] {
	...
}
```

Это нормально, если `false` никогда не записывается. Для строгого set лучше `struct{}`.

## Группировка через map

```go
groups := make(map[string][]Order)
for _, order := range orders {
	groups[order.UserID] = append(groups[order.UserID], order)
}
```

Это идиоматично.

Почему работает без проверки ключа?

Если `groups[order.UserID]` отсутствует, вернется nil slice. В nil slice можно append-ить.

На ревью:

> Для map[K][]V паттерн `m[k] = append(m[k], v)` нормален: missing key дает nil slice, append работает.

Но если value - map:

```go
groups := map[string]map[string]int{}
groups[userID][status]++ // panic if inner map nil
```

Нужно:

```go
if groups[userID] == nil {
	groups[userID] = make(map[string]int)
}
groups[userID][status]++
```

## Счетчики

```go
counts := make(map[string]int)
for _, item := range items {
	counts[item.Category]++
}
```

Это работает, потому что missing key дает zero value `0`.

Но если нужно отличить “нет ключа” от “значение 0”, используем `ok`.

## Map of slices, map of maps

### map[K][]V

Идиоматично:

```go
m[k] = append(m[k], v)
```

### map[K]map[K2]V

Нужно инициализировать inner map:

```go
if m[k] == nil {
	m[k] = make(map[K2]V)
}
m[k][k2] = v
```

### map[K]*Struct

Нужно проверять nil:

```go
user := users[id]
if user == nil {
	return ErrNotFound
}
user.Name = "new"
```

## Удаление во время range

Удалять из map во время range можно.

```go
for k := range m {
	if shouldDelete(k) {
		delete(m, k)
	}
}
```

Это разрешено.

Добавлять во время range технически тоже не запрещено, но поведение обхода новых элементов не гарантировано: новый элемент может быть посещен или нет.

```go
for k := range m {
	m[newKey(k)] = value
}
```

На ревью лучше избегать добавления в map, по которой сейчас range-имся, если результат важен.

Формулировка:

> Delete during range is ok. Insert during range makes iteration behavior hard to reason about: newly inserted entries may or may not be visited.

## Очистка map

Можно удалить ключи:

```go
for k := range m {
	delete(m, k)
}
```

В новых Go есть built-in `clear`:

```go
clear(m)
```

`clear` удаляет все entries из map. Для nil map безопасен.

```go
var m map[string]int
clear(m) // ok
```

Важно: очистка map не обязательно сразу отдаст всю память ОС. Runtime может сохранить внутренние структуры хранения для reuse. Если нужно позволить GC освободить память, можно сбросить map:

```go
m = nil
```

или создать новую:

```go
m = make(map[string]int)
```

На ревью:

> Если map была огромной и дальше нужна маленькая, `clear` может оставить внутреннюю емкость для reuse. Если память важнее reuse, лучше заменить map на nil/new map.

## Конкурентный доступ

Обычная map не потокобезопасна.

Опасно:

```go
m := map[string]int{}

go func() {
	m["a"] = 1
}()

go func() {
	fmt.Println(m["a"])
}()
```

Если есть конкурентная запись и чтение/запись без синхронизации, это data race. Runtime может упасть:

```text
fatal error: concurrent map read and map write
```

или:

```text
fatal error: concurrent map writes
```

Но не надо полагаться на panic. Data race уже делает поведение некорректным.

### Безопасно ли много concurrent reads?

Если map больше не изменяется, параллельные чтения обычно безопасны.

```go
m := buildMap()
// после этого только read-only access
```

Но если хотя бы одна горутина пишет, нужна синхронизация.

### Mutex

```go
type SafeCache struct {
	mu sync.RWMutex
	m  map[string]Item
}

func (c *SafeCache) Get(key string) (Item, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	v, ok := c.m[key]
	return v, ok
}

func (c *SafeCache) Set(key string, value Item) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.m[key] = value
}
```

### sync.Map

`sync.Map` - специальная concurrent map.

```go
var m sync.Map
m.Store("a", 1)
v, ok := m.Load("a")
m.Delete("a")
```

Когда полезна:

- много goroutine читают, записи редкие;
- ключи в основном пишутся один раз и читаются много раз;
- независимые keys;
- cache-like сценарии.

Минусы:

- key/value типа `any`, слабее типизация;
- сложнее поддерживать инварианты;
- неудобно делать операции над несколькими ключами атомарно;
- не всегда быстрее `map+mutex`.

На ревью:

> `sync.Map` не универсальная замена обычной map. Для большинства доменных cache-ей `map+RWMutex` проще и типобезопаснее.

## Неатомарные операции

Даже если отдельные операции защищены, check-then-act может быть логически неатомарным.

Плохо:

```go
if _, ok := m[key]; !ok {
	m[key] = value
}
```

В concurrent-коде это должно быть под одним lock:

```go
mu.Lock()
if _, ok := m[key]; !ok {
	m[key] = value
}
mu.Unlock()
```

То же для счетчика:

```go
m[key]++
```

Это read-modify-write. Без lock в concurrent-коде нельзя.

## Map values и mutable contents

Даже если map защищена mutex-ом, value может быть mutable.

```go
type Cache struct {
	mu sync.RWMutex
	m  map[string][]int
}

func (c *Cache) Get(key string) []int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.m[key]
}
```

Caller получает slice, который смотрит на внутренний underlying array, и может изменить его без lock:

```go
items := cache.Get("x")
items[0] = 100
```

Лучше вернуть копию:

```go
func (c *Cache) Get(key string) []int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	items := c.m[key]
	return append([]int(nil), items...)
}
```

То же с pointer values:

```go
map[string]*User
```

Выдали pointer - выдали право мутировать объект.

На ревью:

> Mutex защищает саму map, но надо проверить, не возвращаем ли наружу mutable value: slice, map, pointer.

## Map и память

Map может расти. При удалении элементов память может не возвращаться сразу.

```go
m := make(map[string]BigValue)
// добавили миллион элементов
// удалили миллион элементов
```

Map может сохранить внутренние структуры хранения для будущего reuse.

Если это долгоживущий объект и память важна:

```go
m = make(map[string]BigValue)
```

или:

```go
m = nil
```

Если values содержат pointers, удаление ключей освобождает ссылки из entries, но детали освобождения памяти зависят от runtime/GC.

Для больших cache-ей часто лучше использовать:

- TTL;
- max size;
- LRU/LFU;
- готовую cache-библиотеку;
- external cache вроде Redis.

Обычная map без eviction в долгоживущем сервисе может стать memory leak по смыслу: она бесконечно растет.

## Map и архитектура

Map - не всегда правильная структура.

Подходит:

- быстрый lookup по уникальному ключу;
- дедупликация;
- счетчики;
- группировка;
- cache;
- индекс по ID.

Может не подходить:

- нужен порядок вставки;
- нужен sorted order;
- нужны range queries;
- нужны top-N/min/max;
- много маленьких наборов, где slice проще;
- нужен concurrent access без внешней синхронизации;
- нужен TTL/eviction;
- нужен persistence;
- нужен predictable memory usage.

Альтернативы:

- slice + linear scan для маленьких наборов;
- sorted slice + binary search;
- heap для priority queue;
- tree/B-tree для ordered/range operations;
- list для частых вставок/удалений при известных nodes;
- Redis/DB для shared/distributed state;
- `sync.Map` для специфических concurrent паттернов.

На собесе:

> Map хороша для key-value lookup, но выбор структуры зависит от операций. Если нужен порядок или range query, hash map не лучший вариант. Если данных мало, slice может быть быстрее и проще.

## Частые задачи “что выведет”

### Задача 1: missing key

```go
m := map[string]int{"a": 0}

fmt.Println(m["a"])
fmt.Println(m["b"])

_, oka := m["a"]
_, okb := m["b"]
fmt.Println(oka, okb)
```

Вывод:

```text
0
0
true false
```

Без `ok` не отличить отсутствующий ключ от zero value.

### Задача 2: nil map

```go
var m map[string]int

fmt.Println(m == nil)
fmt.Println(len(m))
fmt.Println(m["x"])
delete(m, "x")
m["x"] = 1
```

До записи:

```text
true
0
0
```

Потом panic:

```text
panic: assignment to entry in nil map
```

### Задача 3: map передана в функцию

```go
func change(m map[string]int) {
	m["a"] = 100
}

func main() {
	m := map[string]int{"a": 1}
	change(m)
	fmt.Println(m)
}
```

Вывод:

```text
map[a:100]
```

### Задача 4: replace map внутри функции

```go
func replace(m map[string]int) {
	m = map[string]int{"b": 2}
}

func main() {
	m := map[string]int{"a": 1}
	replace(m)
	fmt.Println(m)
}
```

Вывод:

```text
map[a:1]
```

Потому что локальная переменная `m` стала смотреть на другую map, внешний header не изменился.

### Задача 5: struct value

```go
type User struct {
	Name string
}

func main() {
	m := map[string]User{"a": {Name: "Alice"}}
	u := m["a"]
	u.Name = "Bob"
	fmt.Println(m["a"].Name)
}
```

Вывод:

```text
Alice
```

Потому что `u` - копия значения из map. Чтобы изменить:

```go
u := m["a"]
u.Name = "Bob"
m["a"] = u
```

### Задача 6: pointer value

```go
type User struct {
	Name string
}

func main() {
	m := map[string]*User{"a": {Name: "Alice"}}
	u := m["a"]
	u.Name = "Bob"
	fmt.Println(m["a"].Name)
}
```

Вывод:

```text
Bob
```

Потому что скопировался pointer, но объект общий.

## Типовые ошибки на ревью

### Ошибка 1. Запись в nil map

```go
var m map[string]int
m["x"] = 1
```

Нужно `make`.

### Ошибка 2. Не проверяют `ok`

```go
limit := limits[userID]
```

Если `0` - валидный limit, нельзя понять, ключ отсутствует или limit равен 0.

### Ошибка 3. Полагаться на порядок range

```go
for k := range m {
	result = append(result, k)
}
return result
```

Если API/test ожидает порядок - нужно сортировать.

### Ошибка 4. Concurrent read/write без mutex

```go
cache[id] = value
return cache[id]
```

в разных goroutine.

Нужен mutex/sync.Map/channel ownership.

### Ошибка 5. Возврат внутренней map наружу

```go
func (s *Store) All() map[string]Item {
	return s.items
}
```

Caller может мутировать внутреннее состояние.

Лучше вернуть копию:

```go
result := make(map[string]Item, len(s.items))
for k, v := range s.items {
	result[k] = v
}
return result
```

Если values mutable, может потребоваться deep copy.

### Ошибка 6. Сохранение входной map без копии

```go
func (s *Store) Set(items map[string]Item) {
	s.items = items
}
```

Caller и Store разделяют одну map.

Если нужен ownership:

```go
s.items = cloneMap(items)
```

В новых Go есть `maps.Clone` в стандартной библиотеке:

```go
s.items = maps.Clone(items)
```

Но shallow copy: values не deep-copy.

### Ошибка 7. Мутация struct value в map

```go
m[id].Status = "active"
```

Не компилируется. Нужно достать-copy-writeback или хранить pointer.

### Ошибка 8. Map as set через bool с неоднозначностью

```go
if seen[id] {
	...
}
```

Нормально, если `false` не записывается. Если `false` имеет смысл, нужен `ok`.

### Ошибка 9. Inner map не инициализирована

```go
m[userID][key] = value
```

если `m[userID] == nil`, будет panic.

### Ошибка 10. Бесконечный cache

```go
cache[key] = value
```

без TTL/eviction/limit в долгоживущем сервисе. Это может стать memory leak.

### Ошибка 11. Использование map там, где нужен порядок

Если бизнес-логика зависит от порядка, map не подходит без дополнительной структуры.

### Ошибка 12. Использование map с `any` ключом без контроля

Может быть panic на non-comparable dynamic type.

## Практики использования

### Clone map

```go
func cloneMap[K comparable, V any](src map[K]V) map[K]V {
	if src == nil {
		return nil
	}

	dst := make(map[K]V, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
```

С `maps.Clone`:

```go
dst := maps.Clone(src)
```

Помнить: это shallow copy.

### Stable ordered output

```go
keys := make([]string, 0, len(m))
for k := range m {
	keys = append(keys, k)
}
sort.Strings(keys)

for _, k := range keys {
	process(k, m[k])
}
```

### Counter

```go
counts := make(map[string]int)
for _, event := range events {
	counts[event.Type]++
}
```

### Group by

```go
byUser := make(map[string][]Order)
for _, order := range orders {
	byUser[order.UserID] = append(byUser[order.UserID], order)
}
```

### Set

```go
seen := make(map[string]struct{})
seen[id] = struct{}{}

if _, ok := seen[id]; ok {
	...
}
```

### Cache with mutex

```go
type Cache[K comparable, V any] struct {
	mu sync.RWMutex
	m  map[K]V
}

func NewCache[K comparable, V any]() *Cache[K, V] {
	return &Cache[K, V]{m: make(map[K]V)}
}

func (c *Cache[K, V]) Get(k K) (V, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	v, ok := c.m[k]
	return v, ok
}

func (c *Cache[K, V]) Set(k K, v V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.m[k] = v
}
```

## Как отвечать на собеседовании

### Что такое map?

> Map - это встроенная hash table: структура `key -> value`. Ключ должен быть comparable. В среднем lookup/insert/delete O(1), но порядок обхода не гарантирован, и обычная map не потокобезопасна для конкурентных записей.

### Как map работает под капотом?

> Runtime считает hash ключа, по hash выбирает место поиска, проверяет кандидатов и обязательно сравнивает реальные ключи. В Go 1.24+ стандартная реализация основана на Swiss Table: данные лежат группами по 8 slots, рядом есть control bytes, в которых хранится состояние slot-а и 7 бит hash. Lookup быстро проверяет control bytes группы, находит candidate slots, потом сравнивает реальные ключи. Коллизии решаются через open addressing и quadratic probing.

### Что такое коллизия и как с ней работают?

> Коллизия - это когда разные ключи дают одинаковый hash или попадают в одну область хранения. Это нормальная ситуация для hash table. Runtime сначала использует hash-фрагмент как быстрый фильтр, но затем сравнивает реальные ключи. В современной Swiss Table реализации при коллизиях lookup идет по probe sequence к следующим группам. Коллизии не ломают корректность, но могут ухудшать скорость.

### Что изменилось со Swiss Table?

> Раньше Go map обычно объясняли через bucket-ы по 8 пар и overflow bucket-ы. В Go 1.24+ builtin map перешла на Swiss Table: group из 8 slots плюс control word, H1/H2 hash split, быстрый поиск по control bytes, open addressing, quadratic probing и tombstones после delete. Языковые правила при этом не изменились: порядок range не гарантирован, ключи comparable, обычная map не concurrent-safe для read/write.

### Что будет при чтении отсутствующего ключа?

> Вернется zero value типа значения. Чтобы отличить отсутствующий ключ от zero value, используют comma-ok: `v, ok := m[k]`.

### Что такое nil map?

> Nil map - это неинициализированная map. Ее можно читать, брать len, range-ить и delete-ить, но запись в nil map вызывает panic.

### Как проверить наличие ключа?

```go
v, ok := m[k]
```

### Гарантирован ли порядок range?

> Нет. Порядок обхода map не гарантирован. Если нужен стабильный порядок, надо собрать ключи и отсортировать.

### Какие типы могут быть ключами?

> Только comparable типы: строки, числа, bool, pointers, arrays/structs из comparable полей. Slices, maps, functions ключами быть не могут.

### Можно ли конкурентно работать с map?

> Параллельные read-only обращения допустимы, если map не меняется. Если есть запись параллельно с чтением или другой записью, нужен mutex/sync.Map/другой механизм. Иначе data race и возможен runtime panic.

### Почему нельзя взять адрес элемента map?

> Элементы map не addressable, потому что runtime может менять внутреннее расположение элементов при росте, rehash или split table. Поэтому нельзя `&m[k]` и нельзя напрямую менять поле struct-значения `m[k].Field`.

### Всегда ли map быстрее?

> Нет. Map быстрее в среднем для lookup по ключу на достаточно больших наборах. Но для маленьких данных slice с линейным поиском может быть быстрее из-за cache locality и отсутствия hash overhead. Также map не подходит, если нужен порядок или range queries.

## Методология ревью кода с map

1. Проверить инициализацию: нет ли записи в nil map.
2. Проверить чтение: нужен ли `ok`, если zero value неоднозначен.
3. Проверить порядок: нет ли зависимости от `range` order.
4. Проверить concurrent access: есть ли mutex/sync.Map.
5. Проверить ownership: не возвращается ли внутренняя map наружу.
6. Проверить copy: не сохраняется ли входная map без копии.
7. Проверить values: не возвращаются ли mutable slices/maps/pointers.
8. Проверить key type: comparable ли он, нет ли `any` с риском panic.
9. Проверить map of maps: инициализирована ли inner map.
10. Проверить cache policy: TTL, eviction, max size.
11. Проверить memory behavior: если map огромная, надо ли заменить/clear.
12. Проверить, подходит ли map по задаче: порядок, small data, range query.

## Мини-чеклист

- Map инициализирована перед записью?
- Чтение missing key не путается с zero value?
- Используется `value, ok := m[key]`, где нужно?
- Нет зависимости от порядка range?
- Если нужен порядок, ключи сортируются?
- Key type comparable?
- Нет map с `any` keys без контроля?
- Нет concurrent read/write без lock?
- Не возвращается internal map наружу?
- Не сохраняется external map без clone?
- Values immutable или копируются?
- Нужен ли map вообще, или slice проще?
- Есть ли TTL/eviction для cache?
- Inner maps создаются перед записью?
- Нужно ли `clear`, `delete`, новая map или `nil` для памяти?
- Не пытаются ли изменить поле struct value в map напрямую?

## Короткое резюме

Map в Go - это удобная hash table, но с важными условиями:

- ключ должен быть comparable;
- missing key возвращает zero value;
- наличие ключа проверяется через comma-ok;
- nil map читается, но запись в нее panic;
- порядок range не гарантирован;
- элементы map не addressable;
- map передается как reference-like value;
- обычная map не безопасна для concurrent writes/read-writes;
- map не всегда быстрее slice, особенно на маленьких наборах;
- в Go 1.24+ runtime использует Swiss Table вместо старой bucket/overflow модели;
- для API/cache важно думать об ownership, копиях, TTL и памяти.

Сильная формулировка:

> На ревью map я проверяю не только синтаксис, но и контракт: кто владеет map, может ли она быть nil, не путаем ли missing key с zero value, не зависит ли код от порядка обхода, нет ли конкурентного доступа без mutex, не возвращаем ли внутреннюю map наружу и действительно ли hash map подходит под операции этой задачи.

## Источники

- [Go 1.24 Release Notes: Runtime](https://go.dev/doc/go1.24#runtime) - переход builtin map на Swiss Tables и `GOEXPERIMENT=noswissmap`.
- [Go source: internal/runtime/maps/map.go](https://go.dev/src/internal/runtime/maps/map.go) - комментарий к современной Swiss Table реализации Go map.
- [Go source: internal/runtime/maps/group.go](https://go.dev/src/internal/runtime/maps/group.go) - control bytes, group layout и slots.
- [Go source: runtime/map_noswiss.go](https://go.dev/src/runtime/map_noswiss.go) - старая bucket/overflow реализация без Swiss map.
- [Go spec: range over maps](https://go.dev/ref/spec#For_statements_with_range_clause) - порядок обхода map не специфицирован.
- [Go spec: map types](https://go.dev/ref/spec#Map_types) - базовые языковые правила для map.
