# Слайсы и массивы в Go

Эта тема кажется базовой, но на собеседовании по Go она очень быстро становится глубокой. Обычно начинают с простых вопросов:

- что такое массив;
- что такое слайс;
- чем `len` отличается от `cap`;
- как проверить, что слайс пустой;
- как слайс передается в функцию;
- как работает `append`;
- почему `append` возвращает новый слайс.

А потом переходят к более интересному:

- почему изменение слайса в функции иногда видно снаружи, а иногда нет;
- когда `append` портит другой слайс;
- почему subslice может удерживать большой массив в памяти;
- чем `nil` slice отличается от empty slice;
- почему `range` копирует значение;
- что происходит при удалении элементов;
- как правильно копировать слайс;
- когда нужен `s[:len(s):len(s)]`;
- можно ли конкурентно писать в слайс;
- что ревьюить в коде, где активно используются слайсы.

Главная мысль:

> Слайс в Go - это маленький header, который ссылается на массив. Сам header копируется по значению, но элементы лежат в общем underlying array.

И вторая мысль:

> Почти все подвохи со слайсами возникают из-за aliasing: несколько слайсов могут смотреть на один и тот же массив.

## Простое введение

Массив в Go - это значение фиксированной длины:

```go
var a [3]int
```

У массива длина является частью типа:

```go
var a [3]int
var b [4]int
```

`[3]int` и `[4]int` - разные типы.

Слайс - это динамическое окно поверх массива:

```go
s := []int{1, 2, 3}
```

Слайс сам не хранит элементы внутри себя. Упрощенно он хранит:

```go
type sliceHeader struct {
	ptr *Element
	len int
	cap int
}
```

То есть:

- `ptr` указывает на первый доступный элемент underlying array;
- `len` - сколько элементов видно через слайс;
- `cap` - сколько элементов можно использовать от `ptr` до конца capacity.

Важно: это не точное API для ручного использования, а ментальная модель. В обычном коде не надо работать с `reflect.SliceHeader` или `unsafe`.

Пример:

```go
a := [5]int{10, 20, 30, 40, 50}
s := a[1:4]

fmt.Println(s)      // [20 30 40]
fmt.Println(len(s)) // 3
fmt.Println(cap(s)) // 4
```

Почему `cap(s) == 4`? Потому что `s` начинается с `a[1]`, а до конца массива есть элементы:

```text
a[1], a[2], a[3], a[4]
```

То есть capacity от начала слайса до конца массива.

## Массивы

### Создание массива

```go
var a [3]int
fmt.Println(a) // [0 0 0]
```

Массив инициализируется zero values.

```go
a := [3]int{1, 2, 3}
b := [...]int{1, 2, 3}
```

`...` просит компилятор вывести длину.

### Длина массива

```go
fmt.Println(len(a))
```

У массива нет `cap` как отдельной динамической характеристики: `cap(a)` тоже работает и равен `len(a)`, потому что массив фиксированный.

```go
a := [3]int{1, 2, 3}
fmt.Println(len(a)) // 3
fmt.Println(cap(a)) // 3
```

### Массив передается по значению

```go
func change(a [3]int) {
	a[0] = 100
}

func main() {
	a := [3]int{1, 2, 3}
	change(a)
	fmt.Println(a) // [1 2 3]
}
```

Массив скопировался. Изменение внутри функции не поменяло оригинал.

Чтобы изменить оригинал:

```go
func change(a *[3]int) {
	a[0] = 100
}
```

Но в обычном Go-коде чаще используют слайсы, а не указатели на массивы.

### Почему массивы важны для понимания слайсов

Потому что слайс всегда опирается на underlying array. Когда ты пишешь:

```go
s := []int{1, 2, 3}
```

где-то есть массив с элементами `1, 2, 3`, а `s` - descriptor этого массива.

## Слайсы

### Создание слайса literal-ом

```go
s := []int{1, 2, 3}
```

Это не массив. У массива в типе есть длина:

```go
a := [3]int{1, 2, 3}
```

У слайса длины в типе нет:

```go
var s []int
```

### Создание через make

```go
s := make([]int, 3)
fmt.Println(s)      // [0 0 0]
fmt.Println(len(s)) // 3
fmt.Println(cap(s)) // 3
```

С `len` и `cap`:

```go
s := make([]int, 0, 10)
fmt.Println(len(s)) // 0
fmt.Println(cap(s)) // 10
```

Так часто делают, если заранее знают примерный размер:

```go
users := make([]User, 0, len(rows))
for _, row := range rows {
	users = append(users, mapRow(row))
}
```

Это уменьшает количество reallocations.

### Nil slice

```go
var s []int

fmt.Println(s == nil) // true
fmt.Println(len(s))   // 0
fmt.Println(cap(s))   // 0
```

Nil slice можно читать по `len`, можно делать `append`:

```go
s = append(s, 1)
fmt.Println(s) // [1]
```

Но нельзя обращаться по индексу:

```go
var s []int
fmt.Println(s[0]) // panic: index out of range
```

### Empty non-nil slice

```go
s := []int{}

fmt.Println(s == nil) // false
fmt.Println(len(s))   // 0
fmt.Println(cap(s))   // 0
```

Или:

```go
s := make([]int, 0)
```

### Как проверить, что слайс пустой

Обычно:

```go
if len(s) == 0 {
	// empty
}
```

Не так:

```go
if s == nil {
	// only nil, not all empty slices
}
```

`nil` slice и empty slice оба пустые по смыслу длины:

```go
var a []int
b := []int{}

fmt.Println(len(a) == 0) // true
fmt.Println(len(b) == 0) // true
fmt.Println(a == nil)    // true
fmt.Println(b == nil)    // false
```

На собесе:

> Пустоту слайса обычно проверяют через `len(s) == 0`. Проверка `s == nil` отвечает на другой вопрос: был ли слайс nil, а не пустой ли он логически.

## Nil vs empty в JSON

В Go:

```go
var a []int
b := []int{}
```

Для большинства операций они одинаково удобны. Но при JSON encoding разница видна:

```go
type Response struct {
	Items []int `json:"items"`
}
```

Nil slice обычно кодируется как:

```json
{"items":null}
```

Empty non-nil slice:

```json
{"items":[]}
```

Это важно для API-контрактов. Если клиент ожидает `[]`, надо инициализировать empty slice.

На ревью:

> Для внутренней логики `nil` slice обычно нормален, но для JSON/API ответа стоит проверить контракт: хотим `null` или `[]`.

Если поле с `omitempty`:

```go
Items []int `json:"items,omitempty"`
```

то и nil, и empty slice часто будут опущены, потому что `len == 0`.

## len и cap

`len(s)` - сколько элементов доступно по индексу.

```go
s := []int{1, 2, 3}
fmt.Println(len(s)) // 3
```

`cap(s)` - сколько элементов можно вместить в текущий underlying array начиная от начала слайса, прежде чем понадобится новый массив.

```go
a := [5]int{1, 2, 3, 4, 5}
s := a[1:3]

fmt.Println(s)      // [2 3]
fmt.Println(len(s)) // 2
fmt.Println(cap(s)) // 4
```

Индексы:

```go
s[0] // ok
s[1] // ok
s[2] // panic, потому что len == 2
```

Хотя `cap == 4`, обращаться напрямую можно только до `len-1`.

Расширить до capacity можно slicing-ом:

```go
s = s[:cap(s)]
fmt.Println(s) // [2 3 4 5]
```

Но это работает только если элементы находятся в capacity и индексы валидны.

## Slicing

Базовый синтаксис:

```go
s[a:b]
```

Это элементы с индекса `a` включительно до `b` не включительно.

```go
s := []int{10, 20, 30, 40}

fmt.Println(s[1:3]) // [20 30]
fmt.Println(s[:2])  // [10 20]
fmt.Println(s[2:])  // [30 40]
fmt.Println(s[:])   // [10 20 30 40]
```

Срез не копирует элементы. Новый слайс смотрит на тот же underlying array.

```go
s := []int{1, 2, 3, 4}
t := s[1:3]
t[0] = 100

fmt.Println(s) // [1 100 3 4]
```

Вот это один из самых важных gotchas.

## Full slice expression

Есть форма:

```go
s[a:b:c]
```

Где:

- `a` - начало;
- `b` - конец len;
- `c` - конец cap.

Новая длина:

```go
b - a
```

Новая capacity:

```go
c - a
```

Пример:

```go
s := []int{1, 2, 3, 4, 5}
t := s[1:3:3]

fmt.Println(t)      // [2 3]
fmt.Println(len(t)) // 2
fmt.Println(cap(t)) // 2
```

Зачем это нужно? Чтобы ограничить capacity и защититься от `append`, который может перезаписать хвост исходного массива.

Без ограничения:

```go
s := []int{1, 2, 3, 4, 5}
t := s[1:3]
t = append(t, 99)

fmt.Println(s) // [1 2 3 99 5]
fmt.Println(t) // [2 3 99]
```

С ограничением:

```go
s := []int{1, 2, 3, 4, 5}
t := s[1:3:3]
t = append(t, 99)

fmt.Println(s) // [1 2 3 4 5]
fmt.Println(t) // [2 3 99]
```

Потому что `cap(t) == len(t)`, и `append` вынужден выделить новый массив.

На ревью:

> Если subslice передается дальше и его будут append-ить, стоит подумать о full slice expression `s[i:j:j]` или явной копии, чтобы не изменить исходный массив.

## Передача слайса в функцию

Слайс передается по значению. Но значение слайса - это header, который указывает на общий underlying array.

```go
func change(s []int) {
	s[0] = 100
}

func main() {
	s := []int{1, 2, 3}
	change(s)
	fmt.Println(s) // [100 2 3]
}
```

Почему изменение видно? Потому что header скопировался, но оба header-а указывают на один массив.

Теперь append:

```go
func add(s []int) {
	s = append(s, 4)
}

func main() {
	s := []int{1, 2, 3}
	add(s)
	fmt.Println(s) // [1 2 3]
}
```

Почему `4` не видно? Потому что `append` изменил локальную копию header-а: у нее новый `len`, возможно новый `ptr`. Внешний header остался прежним.

Чтобы изменение длины было видно, нужно вернуть слайс:

```go
func add(s []int) []int {
	return append(s, 4)
}

func main() {
	s := []int{1, 2, 3}
	s = add(s)
	fmt.Println(s) // [1 2 3 4]
}
```

Именно поэтому `append` возвращает слайс.

На собесе:

> Слайс передается по значению, копируется только header. Изменение элементов видно через общий underlying array, но изменение самого header-а: len/cap/ptr - снаружи не видно, если не вернуть новый слайс.

## Почему append возвращает слайс

`append` может:

1. Записать элементы в тот же underlying array, если capacity хватает.
2. Выделить новый массив, скопировать старые элементы, добавить новые, вернуть header на новый массив.

Поэтому обязательно:

```go
s = append(s, x)
```

а не:

```go
append(s, x) // compile error: result of append not used
```

Функция `append` возвращает новый slice header, потому что:

- `len` точно меняется;
- `cap` может измениться;
- `ptr` может измениться, если была reallocation.

## Как работает append концептуально

Упрощенная логика:

```go
func appendConceptually(s []T, values ...T) []T {
	newLen := len(s) + len(values)

	if newLen <= cap(s) {
		// используем тот же array
		result := s[:newLen]
		copy(result[len(s):], values)
		return result
	}

	// выделяем новый array побольше
	newCap := grow(cap(s), newLen)
	result := make([]T, newLen, newCap)
	copy(result, s)
	copy(result[len(s):], values)
	return result
}
```

Реальный алгоритм роста capacity - implementation detail runtime. Важно не завязываться на точные числа. Обычно capacity растет с запасом, чтобы последовательные append были амортизированно эффективными.

На собесе:

> Точный коэффициент роста capacity - деталь реализации runtime и может меняться. Гарантия для нас: append вернет слайс с нужной длиной, при нехватке capacity выделит новый underlying array и скопирует элементы.

## Append и aliasing

Это одна из самых частых задачек.

```go
a := []int{1, 2, 3, 4}
b := a[:2]
c := append(b, 99)

fmt.Println(a)
fmt.Println(b)
fmt.Println(c)
```

Разбор:

```go
a = [1 2 3 4], len=4 cap=4
b = a[:2] -> [1 2], len=2 cap=4
append(b, 99) capacity хватает
```

`99` записывается в тот же underlying array на позицию `2`.

Результат:

```text
a = [1 2 99 4]
b = [1 2]
c = [1 2 99]
```

То есть append в `b` изменил `a`.

Защита:

```go
b := a[:2:2]
c := append(b, 99)
```

или:

```go
b := append([]int(nil), a[:2]...)
```

## Append внутри функции

Пример 1: capacity не хватает.

```go
func f(s []int) {
	s = append(s, 4)
	s[0] = 100
}

func main() {
	s := []int{1, 2, 3}
	f(s)
	fmt.Println(s)
}
```

Часто результат:

```text
[1 2 3]
```

Почему? У `s := []int{1,2,3}` обычно `len=3 cap=3`. `append` выделил новый массив, локальная `s` теперь смотрит туда, `s[0]=100` меняет новый массив, внешний слайс остался на старом.

Пример 2: capacity хватает.

```go
func f(s []int) {
	s = append(s, 4)
	s[0] = 100
}

func main() {
	s := make([]int, 3, 10)
	s[0], s[1], s[2] = 1, 2, 3
	f(s)
	fmt.Println(s)
}
```

Результат:

```text
[100 2 3]
```

Почему? `append` остался в том же underlying array. Изменение элемента `s[0]` видно.

Но длина внешнего слайса не изменилась: внешний `len` всё еще 3.

На ревью:

> Если функция append-ит в переданный слайс, она должна вернуть новый слайс или принимать указатель на слайс. Иначе caller не увидит изменение len/ptr.

Обычно лучше вернуть:

```go
func add(s []int, v int) []int {
	return append(s, v)
}
```

Указатель на слайс нужен редко:

```go
func add(s *[]int, v int) {
	*s = append(*s, v)
}
```

Чаще это ухудшает читаемость.

## Copy

`copy(dst, src)` копирует элементы из `src` в `dst` и возвращает количество скопированных элементов.

```go
src := []int{1, 2, 3}
dst := make([]int, len(src))
n := copy(dst, src)

fmt.Println(dst) // [1 2 3]
fmt.Println(n)   // 3
```

Если `dst` короче:

```go
src := []int{1, 2, 3}
dst := make([]int, 2)
n := copy(dst, src)

fmt.Println(dst) // [1 2]
fmt.Println(n)   // 2
```

`copy` безопасно работает с перекрывающимися слайсами:

```go
s := []int{1, 2, 3, 4}
copy(s[1:], s)
fmt.Println(s) // [1 1 2 3]
```

## Как скопировать слайс

Вариант 1:

```go
dst := make([]T, len(src))
copy(dst, src)
```

Вариант 2:

```go
dst := append([]T(nil), src...)
```

Вариант 3, если нужно сохранить empty non-nil:

```go
dst := append([]T{}, src...)
```

Разница:

```go
var src []int = nil

a := append([]int(nil), src...)
b := append([]int{}, src...)

fmt.Println(a == nil) // true
fmt.Println(b == nil) // false
```

В новых версиях Go есть пакет `slices` в стандартной библиотеке:

```go
clone := slices.Clone(src)
```

Он удобен, но на собесе полезно понимать базовые `make + copy` и `append`.

## Удаление элемента

Удалить элемент с индексом `i`, порядок не важен:

```go
s[i] = s[len(s)-1]
s = s[:len(s)-1]
```

Быстро, O(1), но меняет порядок.

Удалить с сохранением порядка:

```go
s = append(s[:i], s[i+1:]...)
```

Это O(n), потому что элементы после `i` сдвигаются.

Или:

```go
copy(s[i:], s[i+1:])
s = s[:len(s)-1]
```

### Важный момент про GC

Если слайс хранит pointers или большие объекты через pointers, удаленный элемент может оставаться в underlying array и удерживать объект от сборки мусора.

Плохо:

```go
s = append(s[:i], s[i+1:]...)
```

Для `[]*BigObject` последний элемент все еще может ссылаться на объект.

Лучше:

```go
copy(s[i:], s[i+1:])
s[len(s)-1] = nil
s = s[:len(s)-1]
```

Для generic:

```go
var zero T
s[len(s)-1] = zero
s = s[:len(s)-1]
```

В стандартном пакете `slices` есть helpers вроде `slices.Delete`, и современные реализации учитывают зануление освобожденных элементов для GC. Но на ревью всё равно важно понимать проблему удержания ссылок.

## Очистка слайса

Сбросить длину до нуля, сохранить underlying array для переиспользования:

```go
s = s[:0]
```

Это полезно для буферов:

```go
buf = buf[:0]
for ... {
	buf = append(buf, ...)
}
```

Но если элементы содержат pointers, старые ссылки остаются в underlying array и могут удерживать память.

Если нужно помочь GC:

```go
for i := range s {
	s[i] = nil
}
s = s[:0]
```

Или полностью отпустить массив:

```go
s = nil
```

Разница:

- `s = s[:0]` переиспользует память;
- `s = nil` позволяет GC освободить underlying array, если больше нет ссылок.

На ревью:

> Если это долгоживущий буфер с pointer-элементами, `s[:0]` может удерживать память. Надо занулить элементы или сбросить слайс в nil, если память важнее reuse.

## Subslice и утечка памяти

Классическая ловушка:

```go
func firstKB(data []byte) []byte {
	return data[:1024]
}
```

Если `data` - файл на несколько гигабайт, возвращенный маленький слайс удерживает весь underlying array.

Почему? Потому что `data[:1024]` смотрит на тот же массив.

Лучше скопировать:

```go
func firstKB(data []byte) []byte {
	result := make([]byte, 1024)
	copy(result, data[:1024])
	return result
}
```

Или:

```go
return append([]byte(nil), data[:1024]...)
```

На собесе:

> Маленький subslice может удерживать большой underlying array. Если нужно вернуть маленький фрагмент из большого буфера надолго, надо скопировать.

## Range по слайсу

Базово:

```go
for i, v := range s {
	fmt.Println(i, v)
}
```

`v` - копия элемента.

```go
s := []User{{Name: "a"}, {Name: "b"}}

for _, user := range s {
	user.Name = "x"
}

fmt.Println(s) // [{a} {b}]
```

Чтобы изменить элементы:

```go
for i := range s {
	s[i].Name = "x"
}
```

Если элементы - pointers:

```go
users := []*User{{Name: "a"}, {Name: "b"}}
for _, user := range users {
	user.Name = "x"
}
```

Тут меняется объект, на который указывает pointer.

На ревью:

> В `for _, v := range s` переменная `v` - копия. Если хотим изменить элемент слайса, надо обращаться по индексу.

## Указатель на range variable

Классическая старая ловушка:

```go
var ptrs []*User
for _, user := range users {
	ptrs = append(ptrs, &user)
}
```

В старых версиях Go все указатели могли указывать на одну и ту же переменную цикла. В Go 1.22 семантика loop variables была изменена для модулей с соответствующей версией, и многие старые ловушки с замыканиями стали безопаснее. Но на ревью всё равно лучше писать явно и понятно:

```go
for i := range users {
	ptrs = append(ptrs, &users[i])
}
```

Это еще и семантически точнее: нам нужен адрес элемента слайса, а не адрес копии.

Если `users` - `[]User`, то `&users[i]` указывает на элемент underlying array. Нужно быть уверенным, что слайс не будет reallocate-нут так, что ожидания по адресам нарушатся.

## Изменение слайса во время range

`range` вычисляет слайс один раз в начале: header копируется.

```go
s := []int{1, 2, 3}
for _, v := range s {
	s = append(s, v)
}
fmt.Println(s)
```

Цикл выполнится 3 раза, а не бесконечно, потому что range идет по исходной длине.

Но изменять элементы можно:

```go
s := []int{1, 2, 3}
for i := range s {
	s[i] *= 10
}
fmt.Println(s) // [10 20 30]
```

Удалять элементы внутри `range` часто опасно и запутанно:

```go
for i, v := range s {
	if shouldDelete(v) {
		s = append(s[:i], s[i+1:]...)
	}
}
```

Индексы начинают не соответствовать текущему содержимому. Лучше:

```go
dst := s[:0]
for _, v := range s {
	if keep(v) {
		dst = append(dst, v)
	}
}
s = dst
```

Если элементы с pointers и удаленные надо отпустить для GC, после фильтрации занулить хвост:

```go
for i := len(dst); i < len(s); i++ {
	s[i] = nil
}
s = dst
```

## Фильтрация in-place

Идиоматичный паттерн:

```go
func filterEven(s []int) []int {
	result := s[:0]
	for _, v := range s {
		if v%2 == 0 {
			result = append(result, v)
		}
	}
	return result
}
```

Плюсы:

- не выделяет новый массив;
- сохраняет порядок;
- O(n).

Минусы:

- мутирует исходный underlying array;
- caller должен знать, что исходные данные будут перезаписаны;
- для pointer elements надо подумать о занулении хвоста.

Если нельзя мутировать входной слайс:

```go
func filterEven(s []int) []int {
	result := make([]int, 0, len(s))
	for _, v := range s {
		if v%2 == 0 {
			result = append(result, v)
		}
	}
	return result
}
```

На ревью:

> `s[:0]` - хороший in-place filter, но он меняет underlying array входного слайса. Это должно быть допустимо по контракту функции.

## Сравнение слайсов

Слайсы нельзя сравнивать через `==`, кроме сравнения с `nil`.

```go
a := []int{1, 2}
b := []int{1, 2}

fmt.Println(a == b) // compile error
```

Можно:

```go
fmt.Println(a == nil)
```

Для сравнения:

```go
reflect.DeepEqual(a, b)
```

Но `reflect.DeepEqual` различает nil slice и empty slice:

```go
var a []int
b := []int{}

fmt.Println(reflect.DeepEqual(a, b)) // false
```

В стандартном пакете `slices`:

```go
slices.Equal(a, b)
```

Для многих задач `slices.Equal` удобнее и типобезопаснее. Для nil и empty слайсов она сравнивает элементы, поэтому оба с длиной 0 считаются равными.

На собесе:

> Слайсы не comparable, потому что это descriptor изменяемой последовательности. Можно сравнить только с nil. Для сравнения содержимого используют `slices.Equal`, ручной цикл или `reflect.DeepEqual` с пониманием отличий.

## Слайс как параметр API

Если функция не должна менять вход:

```go
func Sum(values []int) int
```

Обычно нормально. Но если функция может сохранить слайс внутри структуры, это важно:

```go
type Store struct {
	items []Item
}

func (s *Store) SetItems(items []Item) {
	s.items = items
}
```

Теперь caller может изменить `items`, и `Store` увидит изменения.

Если нужен ownership:

```go
func (s *Store) SetItems(items []Item) {
	s.items = append([]Item(nil), items...)
}
```

Если возвращаем внутренний слайс:

```go
func (s *Store) Items() []Item {
	return s.items
}
```

Caller может изменить внутреннее состояние:

```go
items := store.Items()
items[0] = bad
```

Лучше:

```go
func (s *Store) Items() []Item {
	return append([]Item(nil), s.items...)
}
```

На ревью:

> Если структура сохраняет переданный слайс или возвращает свой внутренний слайс, надо явно решить ownership. Иначе caller и объект будут разделять underlying array.

## Слайсы и concurrency

Слайс сам по себе не потокобезопасен.

Опасно:

```go
var s []int

go func() {
	s = append(s, 1)
}()

go func() {
	s = append(s, 2)
}()
```

Тут race:

- меняется slice header `s`;
- могут одновременно писаться элементы underlying array;
- может быть reallocation.

Нужен mutex/channel/другая синхронизация.

Если разные горутины пишут в разные индексы заранее выделенного слайса:

```go
s := make([]int, n)
var wg sync.WaitGroup

for i := range s {
	i := i
	wg.Add(1)
	go func() {
		defer wg.Done()
		s[i] = compute(i)
	}()
}
wg.Wait()
```

Это может быть допустимо, если:

- длина слайса не меняется;
- каждая горутина пишет в свой индекс;
- никто одновременно не читает эти элементы без синхронизации;
- после `wg.Wait` есть synchronization point.

Но concurrent append в общий слайс - почти всегда ошибка.

На ревью:

> Параллельный `append` в один и тот же слайс небезопасен. Если нужно собирать результаты из goroutine, лучше заранее выделить slice и писать по уникальному индексу, либо отправлять результаты в канал, либо защищать append mutex-ом.

## Сбор результатов из goroutine

Плохо:

```go
var results []Result
for _, item := range items {
	go func(item Item) {
		results = append(results, process(item))
	}(item)
}
```

Data race и concurrent append.

Вариант 1: заранее выделить:

```go
results := make([]Result, len(items))
var wg sync.WaitGroup

for i, item := range items {
	i, item := i, item
	wg.Add(1)
	go func() {
		defer wg.Done()
		results[i] = process(item)
	}()
}

wg.Wait()
```

Вариант 2: mutex:

```go
var (
	mu sync.Mutex
	results []Result
)

for _, item := range items {
	item := item
	wg.Add(1)
	go func() {
		defer wg.Done()
		result := process(item)

		mu.Lock()
		results = append(results, result)
		mu.Unlock()
	}()
}
```

Вариант 3: channel:

```go
resultsCh := make(chan Result)
```

И один collector append-ит.

## Performance: preallocation

Плохо:

```go
var users []User
for _, row := range rows {
	users = append(users, mapRow(row))
}
```

Если `len(rows)` известно:

```go
users := make([]User, 0, len(rows))
for _, row := range rows {
	users = append(users, mapRow(row))
}
```

Это уменьшает количество allocations и копирований.

Но не надо preallocate огромную capacity без необходимости:

```go
buf := make([]byte, 0, 1<<30)
```

Это может сразу зарезервировать много памяти.

Если нужна длина сразу:

```go
users := make([]User, len(rows))
for i, row := range rows {
	users[i] = mapRow(row)
}
```

Выбор:

- `make([]T, 0, n)` + `append` удобно, когда добавляем не все элементы;
- `make([]T, n)` + assignment удобно, когда результат ровно n элементов.

## []byte и string

Конвертация:

```go
b := []byte("hello")
s := string(b)
```

Обычно это копирует данные, потому что string immutable, а []byte mutable.

Важно:

- `string` нельзя изменить;
- `[]byte` можно изменить;
- частые конвертации могут давать allocations;
- в performance-sensitive коде надо профилировать.

Для собеса достаточно:

> `string` immutable, `[]byte` mutable. Конвертация между ними обычно создает копию, чтобы сохранить безопасность неизменяемости.

## Слайсы и variadic функции

Функция:

```go
func sum(values ...int) int
```

Вызов:

```go
sum(1, 2, 3)
```

Если есть слайс:

```go
s := []int{1, 2, 3}
sum(s...)
```

Внутри `values` - слайс.

Важно:

```go
func appendOne(values ...int) {
	values[0] = 100
}

s := []int{1, 2, 3}
appendOne(s...)
fmt.Println(s) // [100 2 3]
```

Передан слайс на тот же underlying array.

Если передаем отдельные аргументы:

```go
appendOne(1, 2, 3)
```

компилятор создаст временный слайс для variadic args.

## Многомерные слайсы

```go
matrix := make([][]int, rows)
for i := range matrix {
	matrix[i] = make([]int, cols)
}
```

Каждая строка - отдельный слайс, возможно отдельный underlying array.

Можно сделать один плоский массив:

```go
data := make([]int, rows*cols)
matrix := make([][]int, rows)
for i := range matrix {
	matrix[i] = data[i*cols : (i+1)*cols]
}
```

Плюсы:

- меньше allocations;
- лучше locality;
- удобно для численных задач.

Минусы:

- сложнее менять размеры строк;
- строки разделяют один underlying array.

## Частые задачки: что выведет

### Задача 1

```go
s := []int{1, 2, 3}
t := s
t[0] = 100
fmt.Println(s)
```

Ответ:

```text
[100 2 3]
```

Потому что `t` и `s` смотрят на один array.

### Задача 2

```go
s := []int{1, 2, 3}
t := append(s, 4)
t[0] = 100
fmt.Println(s)
fmt.Println(t)
```

Часто:

```text
[1 2 3]
[100 2 3 4]
```

Потому что у literal обычно cap == len, append выделил новый array. Но на собесе лучше не упираться в “обычно”. Если capacity была бы больше, результат мог бы разделять array.

Чтобы сделать поведение явным:

```go
s := make([]int, 3, 4)
s[0], s[1], s[2] = 1, 2, 3
t := append(s, 4)
t[0] = 100
fmt.Println(s) // [100 2 3]
```

### Задача 3

```go
s := []int{1, 2, 3, 4}
a := s[:2]
b := append(a, 9)
fmt.Println(s)
fmt.Println(b)
```

Ответ:

```text
[1 2 9 4]
[1 2 9]
```

Capacity у `a` хватает, append пишет в исходный array.

### Задача 4

```go
func f(s []int) {
	s = append(s, 10)
}

func main() {
	s := []int{1, 2}
	f(s)
	fmt.Println(s)
}
```

Ответ:

```text
[1 2]
```

Header изменился только внутри функции.

### Задача 5

```go
func f(s []int) {
	s[0] = 10
}

func main() {
	s := []int{1, 2}
	f(s)
	fmt.Println(s)
}
```

Ответ:

```text
[10 2]
```

Элементы общего array изменились.

### Задача 6

```go
var s []int
s = append(s, 1)
fmt.Println(s, len(s), cap(s), s == nil)
```

Ответ:

```text
[1] 1 ... false
```

Точная capacity не важна.

## Типовые ошибки на ревью

### Ошибка 1. Проверка пустоты через nil

```go
if s == nil {
	return
}
```

Если надо проверить пустой ли слайс:

```go
if len(s) == 0 {
	return
}
```

### Ошибка 2. Функция append-ит, но не возвращает слайс

```go
func add(items []Item, item Item) {
	items = append(items, item)
}
```

Caller не увидит новый len/ptr.

Лучше:

```go
func add(items []Item, item Item) []Item {
	return append(items, item)
}
```

### Ошибка 3. Subslice неожиданно меняет исходный слайс

```go
part := items[:10]
part = append(part, newItem)
```

Может перезаписать `items[10]`.

Защита:

```go
part := items[:10:10]
```

или copy.

### Ошибка 4. Возврат маленького subslice от большого буфера

```go
return data[:100]
```

Если `data` большой и результат живет долго, удерживаем весь массив.

Лучше скопировать.

### Ошибка 5. Возврат внутреннего слайса структуры

```go
func (s *Store) Items() []Item {
	return s.items
}
```

Caller может изменить internal state.

Лучше вернуть копию.

### Ошибка 6. Сохранение входного слайса без копии

```go
func (s *Store) SetItems(items []Item) {
	s.items = items
}
```

Caller и Store разделяют array. Если нужен ownership - копировать.

### Ошибка 7. Изменение range value

```go
for _, item := range items {
	item.Status = "done"
}
```

Если `items` - `[]Item`, это меняет копию.

Лучше:

```go
for i := range items {
	items[i].Status = "done"
}
```

### Ошибка 8. Удаление pointer elements без зануления

```go
items = append(items[:i], items[i+1:]...)
```

Может удерживать ссылку в хвосте underlying array.

Лучше занулить освобожденный слот.

### Ошибка 9. Concurrent append

```go
results = append(results, result)
```

из нескольких goroutine без mutex.

Нужна синхронизация или другая стратегия.

### Ошибка 10. Неосознанный `s[:0]`

```go
result := input[:0]
```

Это мутирует input. Хорошо, если контракт in-place. Плохо, если caller ожидает неизменность.

### Ошибка 11. Неверный preallocation

```go
result := make([]Item, len(items))
for _, item := range items {
	if keep(item) {
		result = append(result, item)
	}
}
```

Будет `len(items)` zero values в начале, потом append.

Правильно:

```go
result := make([]Item, 0, len(items))
```

если используем append.

Или:

```go
result := make([]Item, len(items))
for i, item := range items {
	result[i] = item
}
```

если заполняем по индексу.

### Ошибка 12. Сравнение слайсов через reflect без понимания nil/empty

```go
reflect.DeepEqual(nilSlice, emptySlice) // false
```

Для содержимого чаще лучше `slices.Equal`.

## Практики использования

### Для накопления неизвестного числа элементов

```go
var result []T
for ... {
	if ok {
		result = append(result, v)
	}
}
```

Нормально. Nil slice можно append-ить.

### Для накопления с известным максимумом

```go
result := make([]T, 0, len(input))
for _, v := range input {
	if keep(v) {
		result = append(result, v)
	}
}
```

### Для точного размера результата

```go
result := make([]T, len(input))
for i, v := range input {
	result[i] = convert(v)
}
```

### Для защиты от мутаций

```go
safe := append([]T(nil), input...)
```

### Для API response с `[]` вместо `null`

```go
items := make([]Item, 0)
```

или гарантировать перед marshal:

```go
if items == nil {
	items = []Item{}
}
```

### Для in-place filter

```go
out := s[:0]
for _, v := range s {
	if keep(v) {
		out = append(out, v)
	}
}
s = out
```

### Для удаления без сохранения порядка

```go
s[i] = s[len(s)-1]
s[len(s)-1] = zero
s = s[:len(s)-1]
```

## Как отвечать на собеседовании

### Что такое слайс?

> Слайс - это descriptor поверх массива: указатель на underlying array, длина и capacity. Сам слайс копируется по значению, но элементы могут быть общими с другими слайсами.

### Чем массив отличается от слайса?

> Массив имеет фиксированную длину, и длина входит в тип: `[3]int` и `[4]int` разные типы. Массив передается по значению и копируется. Слайс динамический, длина не входит в тип, он ссылается на underlying array.

### Как проверить, что слайс пустой?

> Через `len(s) == 0`. `s == nil` проверяет только nil slice, но empty non-nil slice тоже имеет длину 0.

### Как слайс передается в функцию?

> По значению копируется slice header: pointer, len, cap. Изменение элементов видно снаружи, потому что underlying array общий. Но изменение header-а, например после append, снаружи не видно, если не вернуть новый слайс.

### Почему append возвращает слайс?

> Потому что append меняет длину и может поменять capacity и указатель на underlying array, если понадобится reallocation. Поэтому надо присваивать результат: `s = append(s, x)`.

### Как работает append?

> Если capacity хватает, append записывает новые элементы в тот же underlying array и возвращает слайс с большей длиной. Если capacity не хватает, runtime выделяет новый массив, копирует старые элементы, добавляет новые и возвращает новый slice header.

### Может ли append изменить другой слайс?

> Да, если другой слайс разделяет тот же underlying array и capacity позволяет append без reallocation. Тогда append может перезаписать элементы, видимые через другой слайс.

### Как защититься от этого?

> Сделать копию или ограничить capacity через full slice expression: `s[i:j:j]`.

### Nil slice и empty slice отличаются?

> У обоих `len == 0`, оба можно append-ить. Отличаются сравнением с nil и некоторыми внешними контрактами, например JSON: nil может стать `null`, empty - `[]`.

### Слайсы потокобезопасны?

> Нет. Параллельный append или чтение/запись без синхронизации дают data race. Можно параллельно писать в разные индексы заранее выделенного слайса при строгом разделении индексов и синхронизации завершения, но общий append должен быть защищен.

## Методология ревью кода со слайсами

1. Смотри, где слайс создается: `nil`, literal, `make(len)`, `make(0, cap)`.
2. Если используется `append`, проверь, присваивается ли результат.
3. Если функция append-ит в параметр, проверь, возвращает ли она слайс.
4. Если есть subslice, проверь, не будет ли append портить исходный array.
5. Если возвращается subslice от большого буфера, подумай о memory retention.
6. Если структура сохраняет входной слайс, проверь ownership/copy.
7. Если метод возвращает внутренний слайс, проверь, не отдаёт ли mutable state.
8. Если используется `range`, проверь, не пытаются ли менять копию value.
9. Если удаляются pointer elements, проверь зануление хвоста.
10. Если слайс используется из goroutine, проверь data race и concurrent append.
11. Если проверяется пустота, лучше `len(s) == 0`.
12. Если API возвращает JSON, проверь `nil` vs `[]` контракт.
13. Если есть `make([]T, len)` и потом append, проверь, не появились ли zero values.
14. Если есть `s[:0]`, проверь, допустима ли мутация входного underlying array.

## Мини-чеклист

- Это массив или слайс?
- Длина массива случайно не является частью API?
- Пустота проверяется через `len`?
- Нужно ли различать nil и empty?
- `append` результат присвоен?
- Функция после append возвращает слайс?
- Может ли append переиспользовать capacity и изменить другой слайс?
- Нужен ли full slice expression?
- Нужна ли копия перед сохранением/возвратом?
- Нет ли memory retention через маленький subslice?
- Не возвращается ли internal slice наружу?
- `range` value не мутируется по ошибке?
- Удаление элементов учитывает GC?
- Concurrent append защищен?
- Preallocation сделан правильно: `len` vs `cap`?
- JSON contract требует `[]`, а не `null`?

## Короткое резюме

Слайс - это не “динамический массив” в бытовом смысле, а descriptor над массивом. Он дешевый для передачи, но из-за общего underlying array появляются главные ловушки:

- изменение элементов видно через другие слайсы;
- append может переиспользовать capacity и изменить исходный массив;
- append может выделить новый массив, поэтому надо возвращать результат;
- subslice может удерживать большой массив в памяти;
- nil и empty slice оба пустые, но отличаются в API-контрактах;
- range дает копию значения;
- concurrent append небезопасен.

Сильная формулировка для собеса:

> Я всегда смотрю на слайс как на header: pointer, len, cap. Поэтому при ревью проверяю не только индексы, но и ownership underlying array: кто еще на него смотрит, может ли append его переиспользовать, не возвращаем ли мы внутренний mutable slice, и не удерживаем ли лишнюю память через маленький subslice.
