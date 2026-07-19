---
lk:
  source_role: official_reference
  source_refs:
    - "Go Spec: Errors"
    - "Go Spec: Handling panics"
    - "Package errors"
    - "Package fmt: Errorf"
    - "Go Blog: Error handling and Go"
    - "Go Blog: Errors are values"
    - "Go Blog: Working with Errors in Go 1.13"
    - "Go Wiki: Errors"
    - "Go Wiki: Go Code Review Comments"
    - "Effective Go: Errors, Panic, Recover"
  prompt_helper: |
    Это базовая Go-тема про error как interface value, явный возврат ошибок,
    errors.New, fmt.Errorf, wrapping через %w, errors.Is/As, sentinel errors,
    custom error types, errors.Join, panic/recover, границы API и сравнение с
    Java exceptions. Генерируй вопросы на механику, чтение кода и ревью
    ошибок в service/repository слоях.
  challenge_helper: |
    Давай небольшие задачи: добавить контекст к ошибкам без потери причины,
    спроектировать domain errors для repository/service, отличить not found от
    internal error, найти typed nil error, заменить сравнение строк на
    errors.Is/As, собрать multi-error для validation, объяснить panic boundary.
---

# Ошибки в Go: error interface, wrapping, Is/As, sentinel/custom errors и panic

Ошибки в Go сначала выглядят очень простыми:

```go
result, err := doWork()
if err != nil {
	return err
}
```

Новичок обычно видит только повторяющийся `if err != nil`. Но в Go это не
случайная многословность, а часть модели языка: ошибка - обычное значение.
Ее можно вернуть, обернуть, сравнить, сохранить, передать дальше, объединить с
другими ошибками или превратить в ответ HTTP на границе приложения.

Обычно спрашивают:

- что такое встроенный тип `error`;
- почему `nil` означает отсутствие ошибки;
- почему ошибки возвращают последним значением;
- чем `errors.New` отличается от `fmt.Errorf`;
- зачем писать `%w`, а не `%v`;
- как работают `errors.Is` и `errors.As`;
- что такое sentinel error;
- когда нужен свой тип ошибки;
- что такое error chain;
- когда использовать `panic`, а когда возвращать `error`;
- как не потерять контекст и при этом не раскрыть детали реализации;
- чем это отличается от Java exceptions.

Главная мысль:

> В Go ошибка - это значение интерфейсного типа `error`. Нормальная ошибка
> возвращается явно, обычно последним результатом функции. `nil` error значит,
> что операция успешна.

Вторая мысль:

> Wrapping через `%w` строит цепочку ошибок. `errors.Is` ищет в этой цепочке
> конкретную ошибку или категорию, а `errors.As` ищет ошибку нужного типа и дает
> достать из нее поля.

И третья:

> `panic` - не замена exceptions для обычного control flow. Для ожидаемых
> проблем возвращают `error`; `panic/recover` оставляют для programmer bugs,
> нарушенных инвариантов, fatal init-состояний или внутренних границ пакета.

## Primary Sources

- [Go Spec: Errors](https://go.dev/ref/spec#Errors)
- [Go Spec: Handling panics](https://go.dev/ref/spec#Handling_panics)
- [Package errors](https://pkg.go.dev/errors)
- [Package fmt: Errorf](https://pkg.go.dev/fmt#Errorf)
- [Go Blog: Error handling and Go](https://go.dev/blog/error-handling-and-go)
- [Go Blog: Errors are values](https://go.dev/blog/errors-are-values)
- [Go Blog: Working with Errors in Go 1.13](https://go.dev/blog/go1.13-errors)
- [Go Wiki: Errors](https://go.dev/wiki/Errors)
- [Go Wiki: Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)
- [Effective Go: Errors](https://go.dev/doc/effective_go#errors)
- [Effective Go: Panic and Recover](https://go.dev/doc/effective_go#panic)

## 1. Простая модель: функция возвращает результат и ошибку

В Go функция часто возвращает два значения:

```go
user, err := repo.GetUser(ctx, userID)
if err != nil {
	return User{}, err
}

return user, nil
```

Смысл:

- `user` - полезный результат;
- `err` - объяснение, почему результата нет;
- `nil` в `err` - ошибки нет, результат можно использовать.

Типичная сигнатура:

```go
func GetUser(ctx context.Context, id string) (User, error)
```

Почему `error` обычно последний:

```go
value, err := do()
```

Так код читается одинаково во всем Go-коде: сначала результат, в конце статус
операции. Если операция ничего не возвращает, кроме ошибки:

```go
func SaveUser(ctx context.Context, user User) error
```

На собеседовании можно формулировать так:

> В Go нет обычного `try/catch` для штатных ошибок. Функция явно возвращает
> `error`, а вызывающий код явно решает: обработать, обернуть и вернуть выше,
> проигнорировать только осознанно или завершить выполнение на границе.

## 2. Что такое `error` под капотом

В спецификации Go `error` определен как интерфейс:

```go
type error interface {
	Error() string
}
```

То есть любой тип, у которого есть метод `Error() string`, реализует `error`.
Например:

```go
type ParseError struct {
	Line int
	Col  int
	Msg  string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("parse at %d:%d: %s", e.Line, e.Col, e.Msg)
}
```

Теперь `*ParseError` можно вернуть как `error`:

```go
func parse(input string) error {
	if input == "" {
		return &ParseError{Line: 1, Col: 1, Msg: "empty input"}
	}
	return nil
}
```

Важно: `Error()` - это строковое представление для человека. Не надо строить
логику приложения на парсинге этой строки. Если вызывающему коду нужно принять
решение, нужна проверяемая форма: sentinel error, custom type, интерфейс или
документированный predicate.

## 3. `nil` error и typed nil ловушка

Интерфейсное значение в Go можно мысленно представить как пару:

```text
interface value
  dynamic type
  dynamic value
```

`error == nil` только когда обе части пустые:

```text
err == nil
  type:  nil
  value: nil
```

Ловушка:

```go
type ValidationError struct{}

func (*ValidationError) Error() string {
	return "validation failed"
}

func validate(ok bool) error {
	var e *ValidationError

	if !ok {
		e = &ValidationError{}
	}

	return e
}

func main() {
	err := validate(true)
	fmt.Println(err == nil) // false
}
```

Почему `false`? Потому что возвращается не пустой интерфейс, а интерфейс с
динамическим типом `*ValidationError` и динамическим значением `nil`:

```text
err
  dynamic type:  *ValidationError
  dynamic value: nil
```

Исправление простое: если ошибки нет, возвращай настоящий `nil`.

```go
func validate(ok bool) error {
	if ok {
		return nil
	}

	return &ValidationError{}
}
```

На ревью:

> Если функция возвращает `error`, нельзя возвращать typed nil pointer как
> `error`. Нужно явно вернуть `nil`, иначе `err != nil` сработает, хотя внутри
> динамическое значение nil.

## 4. Создание ошибок: `errors.New` и `fmt.Errorf`

Самый простой способ создать ошибку:

```go
return errors.New("empty user id")
```

`errors.New` возвращает новое значение ошибки с текстом. Даже одинаковый текст
не означает, что это одна и та же ошибка:

```go
fmt.Println(errors.New("not found") == errors.New("not found")) // false
```

Если нужно добавить данные в сообщение, используют `fmt.Errorf`:

```go
return fmt.Errorf("user %q not found", userID)
```

Стиль сообщений в Go:

- с маленькой буквы;
- без точки в конце;
- с контекстом операции, если ошибка пойдет выше;
- без лишних слов вроде `error occurred`;
- без секретов, токенов, паролей, raw PII.

Плохо:

```go
return errors.New("User not found.")
```

Лучше:

```go
return fmt.Errorf("user %q not found", userID)
```

Почему с маленькой буквы и без точки? Ошибки часто склеиваются с внешним
контекстом:

```go
return fmt.Errorf("create order: %w", err)
```

Если внутренняя ошибка начинается с большой буквы и заканчивается точкой,
итоговое сообщение выглядит как случайная строка из двух предложений.

## 5. Базовая идиома: handle, wrap or return

У каждого `err != nil` должен быть смысл. Обычно есть четыре варианта.

Первый: обработать ошибку здесь.

```go
if errors.Is(err, ErrUserNotFound) {
	return emptyProfile(), nil
}
```

Второй: добавить контекст и вернуть выше.

```go
if err != nil {
	return fmt.Errorf("load user profile: %w", err)
}
```

Третий: заменить низкоуровневую ошибку на ошибку своего слоя.

```go
if errors.Is(err, sql.ErrNoRows) {
	return User{}, fmt.Errorf("user %q: %w", id, ErrUserNotFound)
}
```

Четвертый: явно проигнорировать, если это правда допустимо.

```go
if err := metrics.Inc("cache_miss"); err != nil {
	logger.Warn("record metric", "err", err)
}
```

Плохо:

```go
_ = repo.Save(ctx, user)
return nil
```

Так ошибка потерялась. По Code Review Comments: если функция возвращает ошибку,
ее надо проверить. Игнорирование через `_` должно быть редким и объяснимым.

## 6. Почему `if err != nil` не так страшен

В Go ошибки проверяются рядом с операцией, которая могла сломаться:

```go
file, err := os.Open(path)
if err != nil {
	return fmt.Errorf("open config %q: %w", path, err)
}
defer file.Close()

cfg, err := parseConfig(file)
if err != nil {
	return fmt.Errorf("parse config %q: %w", path, err)
}
```

Это делает control flow локальным:

```text
open file
  ok  -> parse file
  err -> return "open config ..."

parse file
  ok  -> use config
  err -> return "parse config ..."
```

В Java похожая ошибка может вылететь из глубины вызова и быть поймана далеко
выше. В Go место проверки видно прямо в функции. Цена - больше строк. Польза -
меньше скрытого control flow.

Идиоматичный Go также держит happy path без лишнего отступа:

```go
if err != nil {
	return err
}

// normal path continues here
```

Плохо, когда весь полезный код уезжает внутрь `else`:

```go
if err != nil {
	return err
} else {
	doMore()
}
```

Лучше:

```go
if err != nil {
	return err
}

doMore()
```

## 7. Контекст ошибки: что писать в message

Ошибка должна отвечать на вопрос: какая операция не получилась?

Плохо:

```go
return err
```

Если наверху напечатают:

```text
no rows in result set
```

непонятно, что искали: пользователя, заказ, настройку, промокод?

Лучше:

```go
return fmt.Errorf("load user %q: %w", userID, err)
```

Теперь наверху:

```text
load user "42": no rows in result set
```

Схема движения ошибки:

```text
db driver:
  "connection refused"

repository:
  "select user 42: connection refused"

service:
  "build profile: select user 42: connection refused"

http handler:
  status 500, log full error with request id
```

Но контекст не должен превращаться в шум:

```go
return fmt.Errorf("error while failed to process operation doWork with err: %w", err)
```

Лучше коротко и глаголом:

```go
return fmt.Errorf("process payment: %w", err)
```

## 8. Wrapping: `%w` против `%v`

`fmt.Errorf` умеет добавить строковый контекст:

```go
return fmt.Errorf("read config: %v", err)
```

И умеет обернуть исходную ошибку:

```go
return fmt.Errorf("read config: %w", err)
```

В обоих случаях текст для человека почти одинаковый. Разница для программы:

- `%v` добавляет только текст, исходная ошибка недоступна через `errors.Is/As`;
- `%w` строит цепочку, исходную ошибку можно найти программно.

Схема:

```text
fmt.Errorf("read config: %w", os.ErrNotExist)

outer error:
  message: "read config: file does not exist"
  unwrap -> os.ErrNotExist
```

Проверка:

```go
err := fmt.Errorf("read config: %w", os.ErrNotExist)

fmt.Println(errors.Is(err, os.ErrNotExist)) // true
```

С `%v`:

```go
err := fmt.Errorf("read config: %v", os.ErrNotExist)

fmt.Println(errors.Is(err, os.ErrNotExist)) // false
```

На ревью:

> `%w` - это не "более новый `%v`". `%w` делает нижнюю ошибку частью контракта
> для вызывающего кода. Если хочешь дать caller-у возможность проверить причину
> через `errors.Is/As`, используй `%w`. Если нижняя ошибка - implementation
> detail, оставь `%v` или преобразуй ее в ошибку своего слоя.

## 9. Error chain и `Unwrap`

Ошибка может содержать другую ошибку. В Go это называется wrapping.

```go
var ErrPermission = errors.New("permission denied")

func load() error {
	return fmt.Errorf("load user settings: %w", ErrPermission)
}
```

Цепочка:

```text
"load user settings: permission denied"
  unwrap -> ErrPermission
```

Если обернуть несколько раз:

```go
func handler() error {
	if err := service(); err != nil {
		return fmt.Errorf("handle request: %w", err)
	}
	return nil
}

func service() error {
	if err := repo(); err != nil {
		return fmt.Errorf("build profile: %w", err)
	}
	return nil
}

func repo() error {
	return fmt.Errorf("select user: %w", ErrPermission)
}
```

Цепочка:

```text
handle request
  -> build profile
       -> select user
            -> ErrPermission
```

`errors.Unwrap` снимает один слой, но обычно руками его не вызывают:

```go
inner := errors.Unwrap(err)
```

Для реального кода почти всегда лучше:

```go
if errors.Is(err, ErrPermission) {
	// ...
}
```

`errors.Is` и `errors.As` проходят по цепочке сами.

## 10. `errors.Is`: проверка конкретной ошибки или категории

Sentinel error - это заранее объявленное значение ошибки:

```go
var ErrUserNotFound = errors.New("user not found")
```

Такую ошибку можно вернуть:

```go
func FindUser(ctx context.Context, id string) (User, error) {
	user, ok := cache[id]
	if !ok {
		return User{}, ErrUserNotFound
	}
	return user, nil
}
```

Но если ошибку обернули, прямое сравнение ломается:

```go
err := fmt.Errorf("get user 42: %w", ErrUserNotFound)

fmt.Println(err == ErrUserNotFound)             // false
fmt.Println(errors.Is(err, ErrUserNotFound))    // true
```

Правило:

```go
if errors.Is(err, ErrUserNotFound) {
	// handle not found
}
```

`errors.Is` проверяет:

- саму ошибку;
- ошибки внутри `Unwrap() error`;
- ошибки внутри `Unwrap() []error`, например из `errors.Join`;
- custom `Is(error) bool`, если тип ошибки его реализует.

Когда sentinel error хорош:

- у ошибки нет важных полей;
- вызывающему коду нужна категория;
- категория стабильна как часть API;
- пример: `io.EOF`, `context.Canceled`, `context.DeadlineExceeded`,
  `sql.ErrNoRows`, `fs.ErrNotExist`.

Когда sentinel error опасен:

- нужно передать много деталей;
- разные случаи имеют один текст, но разные параметры;
- публичный sentinel привязывает клиентов к твоей реализации;
- переменную можно случайно переопределить, если она exported var.

## 11. `errors.As`: проверка типа и доступ к полям

Если нужно не просто понять категорию, а достать данные, нужен custom error type.

```go
type ValidationError struct {
	Field string
	Rule  string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("field %q violates %s", e.Field, e.Rule)
}
```

Возвращаем:

```go
func validateEmail(email string) error {
	if email == "" {
		return &ValidationError{Field: "email", Rule: "required"}
	}
	return nil
}
```

Проверяем:

```go
err := validateEmail("")

var validationErr *ValidationError
if errors.As(err, &validationErr) {
	fmt.Println(validationErr.Field) // email
	fmt.Println(validationErr.Rule)  // required
}
```

Почему `&validationErr`, то есть pointer to pointer? Потому что
`errors.As` должен записать найденное значение в переменную. Переменная имеет
тип `*ValidationError`, значит передать надо адрес этой переменной.

Схема:

```text
err interface
  -> outer wrapper
       -> *ValidationError{Field: "email", Rule: "required"}

errors.As(err, &validationErr)
  validationErr now points to the found *ValidationError
```

Важно: target должен соответствовать реальному типу ошибки. Если функция
возвращает `*ValidationError`, ищем `*ValidationError`. Если возвращает
`ValidationError` как value, ищем `ValidationError`.

Обычно для структурных ошибок используют pointer type:

```go
return &ValidationError{Field: "email", Rule: "required"}
```

Так не копируются поля, и `errors.As` выглядит привычно:

```go
var e *ValidationError
if errors.As(err, &e) {
	// ...
}
```

## 12. Custom error type с `Unwrap`

Свой тип ошибки может хранить нижнюю ошибку:

```go
type QueryError struct {
	Query string
	Err   error
}

func (e *QueryError) Error() string {
	return fmt.Sprintf("query %q: %v", e.Query, e.Err)
}

func (e *QueryError) Unwrap() error {
	return e.Err
}
```

Теперь `errors.Is/As` видят не только `QueryError`, но и то, что внутри:

```go
err := &QueryError{
	Query: "select * from users",
	Err:   context.DeadlineExceeded,
}

fmt.Println(errors.Is(err, context.DeadlineExceeded)) // true

var qerr *QueryError
fmt.Println(errors.As(err, &qerr)) // true
```

Такой тип полезен, когда нужно одновременно:

- дать человеку понятный message;
- сохранить машинно-читаемые поля;
- не потерять нижнюю причину.

Пример из стандартной библиотеки в этой же идее - ошибки вида `*fs.PathError`:
там есть operation, path и вложенная ошибка.

## 13. Категории ошибок: sentinel плюс custom type

Часто хочется оба свойства:

- `errors.Is(err, ErrNotFound)` для общей логики;
- `errors.As(err, *NotFoundError)` для деталей.

Можно сделать так:

```go
var ErrNotFound = errors.New("not found")

type NotFoundError struct {
	Resource string
	ID       string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s %q not found", e.Resource, e.ID)
}

func (e *NotFoundError) Is(target error) bool {
	return target == ErrNotFound
}
```

Использование:

```go
func FindUser(ctx context.Context, id string) (User, error) {
	return User{}, &NotFoundError{Resource: "user", ID: id}
}

err := FindUser(ctx, "42")

if errors.Is(err, ErrNotFound) {
	// общая обработка not found
}

var nf *NotFoundError
if errors.As(err, &nf) {
	// можно достать nf.Resource и nf.ID
}
```

Это удобная модель для service layer:

```text
ErrNotFound              category
*NotFoundError           details
errors.Is                category check
errors.As                details extraction
```

Но не надо усложнять каждую ошибку. Если caller ничего не может сделать с
полями, обычного `fmt.Errorf` достаточно.

## 14. Граница API: wrapping становится контрактом

Официальный Go Blog по error wrapping формулирует важную мысль: если ты
оборачиваешь ошибку через `%w`, caller может начать на нее полагаться.

Пример:

```go
func (r *UserRepo) Find(ctx context.Context, id string) (User, error) {
	err := r.db.QueryRowContext(ctx, query, id).Scan(&user.ID, &user.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, fmt.Errorf("user %q: %w", id, ErrUserNotFound)
	}
	if err != nil {
		return User{}, fmt.Errorf("select user %q: %w", id, err)
	}
	return user, nil
}
```

Если `UserRepo` - внутренний компонент приложения, wrapping database error может
быть нормальным: выше логирование увидит реальную причину.

Если `UserRepo` - публичный package API, нужно подумать. Когда ты возвращаешь:

```go
return fmt.Errorf("select user %q: %w", id, sql.ErrNoRows)
```

caller может написать:

```go
if errors.Is(err, sql.ErrNoRows) {
	// ...
}
```

Теперь `sql.ErrNoRows` стал частью твоего API. Если завтра ты заменишь SQL на
HTTP storage, совместимость может сломаться.

Более стабильный вариант:

```go
if errors.Is(err, sql.ErrNoRows) {
	return User{}, fmt.Errorf("user %q: %w", id, ErrUserNotFound)
}
```

И задокументировать:

```go
// Find returns an error matching ErrUserNotFound when the user does not exist.
func (r *UserRepo) Find(ctx context.Context, id string) (User, error)
```

На ревью:

> Я проверяю, какие ошибки слой обещает наружу. `%w` раскрывает нижнюю ошибку
> программно, поэтому это API decision, а не только вопрос красивого текста.

## 15. Перевод низкоуровневых ошибок в domain errors

В service code часто есть несколько уровней:

```text
database/sql
  -> repository
       -> service
            -> transport: HTTP/gRPC/CLI
```

На нижнем уровне могут быть технические ошибки:

- `sql.ErrNoRows`;
- network timeout;
- duplicate key;
- context deadline;
- broken connection.

На уровне сервиса часто нужны бизнесовые категории:

- user not found;
- order already paid;
- insufficient balance;
- invalid transition;
- operation unavailable.

Пример:

```go
var (
	ErrUserNotFound = errors.New("user not found")
	ErrConflict     = errors.New("conflict")
)

func (r *UserRepo) Find(ctx context.Context, id string) (User, error) {
	var user User
	err := r.db.QueryRowContext(ctx, findUserSQL, id).Scan(&user.ID, &user.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, fmt.Errorf("user %q: %w", id, ErrUserNotFound)
	}
	if err != nil {
		return User{}, fmt.Errorf("select user %q: %w", id, err)
	}
	return user, nil
}

func (s *ProfileService) GetProfile(ctx context.Context, id string) (Profile, error) {
	user, err := s.users.Find(ctx, id)
	if err != nil {
		return Profile{}, fmt.Errorf("get profile: %w", err)
	}

	return Profile{User: user}, nil
}
```

На HTTP-границе:

```go
profile, err := service.GetProfile(r.Context(), userID)
if err != nil {
	switch {
	case errors.Is(err, ErrUserNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, context.Canceled):
		return
	default:
		logger.Error("get profile", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
	return
}
```

Обрати внимание: пользователь не получает весь внутренний error chain. Полная
ошибка нужна логам и диагностике, а внешний ответ должен быть стабильным,
безопасным и понятным.

## 16. Логирование ошибок: не логируй на каждом уровне

Плохой паттерн:

```go
func repo() error {
	if err := query(); err != nil {
		logger.Error("query failed", "err", err)
		return err
	}
	return nil
}

func service() error {
	if err := repo(); err != nil {
		logger.Error("service failed", "err", err)
		return err
	}
	return nil
}

func handler() {
	if err := service(); err != nil {
		logger.Error("handler failed", "err", err)
	}
}
```

Одна ошибка даст три похожих log entry. В проде это шумит, усложняет алерты и
мешает найти место, где ошибка реально обработана.

Обычно лучше:

- внутри слоев добавлять контекст и возвращать ошибку;
- логировать на границе, где есть request id, user id, route, status code;
- если ошибка обработана локально и не возвращается, логировать там;
- если ошибка best-effort, явно выбрать log/metric/retry/drop policy.

Пример:

```go
func repo() error {
	if err := query(); err != nil {
		return fmt.Errorf("query user: %w", err)
	}
	return nil
}

func service() error {
	if err := repo(); err != nil {
		return fmt.Errorf("build profile: %w", err)
	}
	return nil
}

func handler() {
	if err := service(); err != nil {
		logger.Error("request failed", "err", err, "route", "/profile")
	}
}
```

На ревью:

> Ошибка должна либо обрабатываться, либо возвращаться с контекстом. Логировать
> и возвращать на каждом уровне обычно плохо: получится дублирование. Исключение
> - когда уровень реально принимает решение: retry, fallback, compensation,
> metric, audit event.

## 17. `errors.Join` и несколько ошибок

Иногда ошибка не одна. Например, validation нашла несколько проблем:

```go
func ValidateUser(u User) error {
	var errs []error

	if u.Email == "" {
		errs = append(errs, errors.New("email is required"))
	}
	if u.Age < 18 {
		errs = append(errs, errors.New("age must be at least 18"))
	}

	return errors.Join(errs...)
}
```

`errors.Join`:

- отбрасывает `nil`;
- возвращает `nil`, если все ошибки `nil`;
- возвращает ошибку, которая содержит несколько children;
- работает с `errors.Is` и `errors.As`.

Пример:

```go
var ErrInvalidEmail = errors.New("invalid email")
var ErrTooYoung = errors.New("too young")

err := errors.Join(
	fmt.Errorf("email: %w", ErrInvalidEmail),
	fmt.Errorf("age: %w", ErrTooYoung),
)

fmt.Println(errors.Is(err, ErrInvalidEmail)) // true
fmt.Println(errors.Is(err, ErrTooYoung))     // true
```

Схема:

```text
joined error
  -> "email: invalid email"
       -> ErrInvalidEmail
  -> "age: too young"
       -> ErrTooYoung
```

Важно: `errors.Unwrap(err)` работает только с `Unwrap() error`, а не с
`Unwrap() []error`. Для multi-error не ходи руками через `errors.Unwrap`; для
проверок используй `errors.Is/As`.

Где `errors.Join` уместен:

- validation errors;
- cleanup, где несколько ресурсов могли закрыться с ошибкой;
- fan-out, где нужно вернуть набор независимых ошибок;
- batch processing, где часть элементов не прошла.

Где не уместен:

- когда caller должен увидеть только первую ошибку;
- когда ошибки имеют порядок причинности, а не независимый набор;
- когда нужна rich validation structure для UI, лучше вернуть typed error с
  полями.

## 18. Ошибки `defer` и `Close`

Частый Go-код:

```go
f, err := os.Open(path)
if err != nil {
	return fmt.Errorf("open %q: %w", path, err)
}
defer f.Close()
```

Для read-only файла часто нормально проигнорировать ошибку `Close`. Но для
writer-а ошибка `Close` может быть важной: данные могли не сброситься на диск
или в сеть.

Плохой вариант:

```go
func WriteFile(path string, data []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return err
	}
	return nil
}
```

Лучше явно учесть `Close`:

```go
func WriteFile(path string, data []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %q: %w", path, err)
	}

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write %q: %w", path, err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("close %q: %w", path, err)
	}

	return nil
}
```

Если есть несколько cleanup-ошибок, можно использовать `errors.Join`, но надо
не потерять главную ошибку.

## 19. Context errors: cancellation и deadline

`context` использует обычные error values:

```go
context.Canceled
context.DeadlineExceeded
```

Проверять их лучше через `errors.Is`, потому что ошибка могла быть обернута:

```go
if errors.Is(err, context.Canceled) {
	return nil
}

if errors.Is(err, context.DeadlineExceeded) {
	return fmt.Errorf("request timeout: %w", err)
}
```

Не надо превращать отмену запроса в обычную internal error метрику:

```go
if errors.Is(err, context.Canceled) {
	// клиент ушел, обычно это не server error
	return
}
```

И не надо терять причину отмены:

```go
select {
case <-ctx.Done():
	return ctx.Err()
case result := <-resultCh:
	return result.Err
}
```

С Go 1.20 есть `context.WithCancelCause` и `context.Cause`, когда нужно
отменить work tree с конкретной причиной. Это уже более продвинутая тема, но
идея та же: причина - обычная ошибка, которую можно передавать и проверять.

## 20. `panic`: что происходит механически

`panic` запускает unwinding текущей goroutine:

```go
func f() {
	defer fmt.Println("defer in f")
	panic("boom")
}
```

Механика:

```text
f starts
  panic("boom")
    run defers in f
    return to caller while panicking
    run caller defers
    ...
    if nobody recovers -> program crashes with stack trace
```

`defer` выполняется при panic:

```go
func main() {
	defer fmt.Println("cleanup")
	panic("boom")
}
```

Вывод будет содержать `cleanup`, а затем stack trace.

Runtime тоже может вызвать panic:

```go
s := []int{1, 2}
fmt.Println(s[10]) // panic: index out of range
```

Другие примеры:

- send в закрытый channel;
- close закрытого channel;
- запись в nil map;
- failed type assertion без comma-ok;
- nil pointer dereference.

## 21. `recover`: где работает и где нет

`recover` останавливает panic только если вызван прямо внутри deferred function
в той же goroutine.

Работает:

```go
func safeCall(fn func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	fn()
	return nil
}
```

Не работает:

```go
func badRecover() {
	recover() // nil, потому что не в deferred function during panic
}
```

Не ловит panic из другой goroutine:

```go
func main() {
	defer func() {
		_ = recover()
	}()

	go func() {
		panic("boom")
	}()

	time.Sleep(time.Second)
}
```

`recover` в `main` не поймает panic в отдельной goroutine. Если goroutine
может panic-нуть и это не должно уронить процесс, recover должен стоять внутри
этой goroutine:

```go
go func() {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("worker panic", "panic", r)
		}
	}()

	runWorker()
}()
```

Но recover - не волшебная кнопка. После panic состояние может быть частично
изменено. Часто лучше дать процессу упасть и перезапуститься, чем продолжать в
непонятном состоянии.

## 22. Когда `panic`, а когда `error`

Обычные ошибки:

- пользователь ввел плохие данные;
- файл не найден;
- сеть недоступна;
- БД вернула deadlock;
- context отменен;
- внешний сервис ответил 503;
- валидация не прошла.

Для этого возвращают `error`.

`panic` уместен, когда:

- нарушен инвариант, который означает bug в программе;
- невозможно продолжать startup, например нет обязательной конфигурации;
- API вызвали с явно недопустимым programmer input, и так принято для этого API;
- внутри пакета `panic/recover` используется как локальная техника, но наружу
  пакет возвращает `error`.

Пример constructor validation:

```go
func NewService(repo Repo, logger Logger) (*Service, error) {
	if repo == nil {
		return nil, errors.New("nil repo")
	}
	if logger == nil {
		return nil, errors.New("nil logger")
	}

	return &Service{repo: repo, logger: logger}, nil
}
```

Это лучше, чем:

```go
func NewService(repo Repo, logger Logger) *Service {
	if repo == nil {
		panic("nil repo")
	}
	return &Service{repo: repo, logger: logger}
}
```

Но бывают библиотеки, где `MustX` явно паникует:

```go
var userIDPattern = regexp.MustCompile(`^[a-z0-9_]+$`)
```

`MustCompile` удобен для package-level initialization: если регулярка в коде
сломана, это bug, и приложение не должно стартовать.

Правило:

```text
expected failure from environment or user input -> error
programmer bug / impossible invariant          -> panic
startup cannot continue                         -> panic or fatal at main boundary
```

## 23. Panic boundary в пакете

Effective Go показывает паттерн: внутри сложного parser-а можно использовать
panic, чтобы быстро выйти из глубокой рекурсии, но exported function должна
превратить это в обычный `error`.

Упрощенная схема:

```go
type parseError string

func (e parseError) Error() string {
	return string(e)
}

func Parse(input string) (tree *Tree, err error) {
	defer func() {
		if r := recover(); r != nil {
			if pe, ok := r.(parseError); ok {
				tree = nil
				err = pe
				return
			}
			panic(r)
		}
	}()

	tree = parseInternal(input) // may panic(parseError(...))
	return tree, nil
}
```

Важная часть - re-panic для неожиданных panic:

```go
panic(r)
```

Если поймали только свой `parseError`, это контролируемая внутренняя техника.
Если поймали `runtime error: index out of range`, это bug, его нельзя молча
превращать в "parse failed".

На собеседовании:

> `recover` обычно ставят на границе goroutine, request boundary или внутри
> пакета, который сам использует panic как внутренний механизм. Нельзя
> превращать все panic подряд в `nil` или обычные ответы: так можно скрыть
> corruption и programmer bugs.

## 24. Ошибки и стек

В Java exception обычно содержит stack trace с момента создания/throw. В Go
обычный `error` stack trace не содержит.

```go
return fmt.Errorf("save user: %w", err)
```

Это добавляет message и chain, но не записывает стек.

Что с этим делать:

- добавлять полезный контекст по пути вверх;
- логировать ошибку на границе с request id и важными полями;
- для panic runtime напечатает stack trace;
- если нужен stack trace для обычных errors, использовать observability tooling
  или специальную библиотеку осознанно.

Не надо пытаться заменить контекст стеком. Stack trace показывает "где", но
часто не показывает "что именно пытались сделать":

```text
good error message:
  create order "A-42": reserve stock SKU-7: deadline exceeded

stack only:
  repo.go:71
  service.go:42
  handler.go:33
```

В сервисной диагностике нужны оба измерения: операция и место.

## 25. Ошибки в тестах

Не тестируй error string, если есть структурный контракт.

Плохо:

```go
if err.Error() != "user not found" {
	t.Fatalf("unexpected error: %v", err)
}
```

Лучше:

```go
if !errors.Is(err, ErrUserNotFound) {
	t.Fatalf("expected ErrUserNotFound, got %v", err)
}
```

Для custom type:

```go
var validationErr *ValidationError
if !errors.As(err, &validationErr) {
	t.Fatalf("expected ValidationError, got %v", err)
}

if validationErr.Field != "email" {
	t.Fatalf("field = %q, want email", validationErr.Field)
}
```

Строку можно проверять, когда именно текст является контрактом: например CLI
output или конкретное сообщение для пользователя. Для внутренней ошибки лучше
проверять категорию и поля.

## 26. Сравнение с Java: механика и концептуальность

В Java обычная модель ошибок - exceptions:

```java
try {
    User user = repo.getUser(id);
    return profile(user);
} catch (UserNotFoundException e) {
    return emptyProfile();
}
```

В Go:

```go
user, err := repo.GetUser(ctx, id)
if err != nil {
	if errors.Is(err, ErrUserNotFound) {
		return emptyProfile(), nil
	}
	return Profile{}, fmt.Errorf("get user: %w", err)
}

return profile(user), nil
```

### Механика control flow

| Вопрос | Go | Java |
|---|---|---|
| Как сообщается штатная ошибка | Функция возвращает `error` | Код делает `throw` exception |
| Где видно, что функция может ошибиться | Обычно в сигнатуре `(T, error)` | Checked exceptions видны в `throws`, unchecked - не видны |
| Что происходит при ошибке | Обычный return value, caller сам проверяет | Стек разматывается до ближайшего `catch` |
| Нужно ли явно проверять | Да, если хочешь корректность | Checked заставляет compiler, unchecked нет |
| Можно ли пропустить обработку | Да, но тогда ошибка теряется или идет выше явно | Да, exception может улететь выше неявно |
| Нормальный cleanup | `defer` | `finally` или try-with-resources |
| Stack trace | У обычного `error` нет автоматически | Обычно есть у `Throwable` |
| Multi-error | `errors.Join`, custom aggregate | Suppressed exceptions, custom aggregate |
| Аналог panic | `panic` | unchecked exception/error, но в Java exceptions шире используются |

Главная механическая разница:

```text
Go:
  call -> returns (value, error) -> local branch

Java:
  call -> throws -> non-local jump to catch
```

В Go ошибка не перепрыгивает через код сама. Это обычное значение, и каждый
уровень явно решает, что с ним делать.

### Checked и unchecked

Java делит исключения на checked и unchecked:

- checked exception должен быть объявлен в `throws` или пойман;
- unchecked exception (`RuntimeException`) может вылететь без объявления.

В Go нет checked exceptions. Но сигнатура с `error` делает похожую вещь более
прямолинейно:

```go
func LoadConfig(path string) (Config, error)
```

Она говорит: "у результата есть состояние ошибки". Compiler не заставляет
обработать `err`, но Go tooling и code review считают потерю ошибки багом.

Концептуально:

- Java checked exceptions - часть type-level контракта, но могут разрастаться и
  протекать через слои;
- Java unchecked exceptions удобны, но легко скрывают failure paths;
- Go `error` проще: это просто значение, но дисциплина обработки лежит на коде,
  тестах, линтерах и ревью.

### Ошибка как value против exception как event

В Go:

```go
err := do()
if err != nil {
	// err можно сохранить, обернуть, сравнить, объединить
}
```

Ошибка похожа на данные.

В Java:

```java
throw new PaymentFailedException(orderId, cause);
```

Exception больше похож на событие управления потоком: оно прерывает текущий ход
выполнения и ищет обработчик.

Это влияет на стиль архитектуры.

В Go часто пишут:

```go
if err := tx.Commit(); err != nil {
	return fmt.Errorf("commit order transaction: %w", err)
}
```

В Java часто пишут:

```java
try {
    tx.commit();
} catch (SQLException e) {
    throw new OrderCommitException(orderId, e);
}
```

Обе модели умеют хранить cause. Но Go заставляет каждый слой явно выбрать:

- вернуть как есть;
- добавить контекст;
- заменить на domain error;
- обработать и продолжить.

### Cause chain

В Java:

```java
throw new ServiceException("build profile", cause);
```

Есть cause chain через `Throwable.getCause()`.

В Go:

```go
return fmt.Errorf("build profile: %w", err)
```

Есть error chain через `Unwrap`.

Схема похожа:

```text
Java:
  ServiceException
    cause -> SQLException

Go:
  "build profile: ..."
    unwrap -> lower error
```

Разница в том, что Go chain - это интерфейсная конвенция (`Unwrap`, `Is`, `As`),
а не обязательный базовый класс. Любой тип может реализовать нужное поведение.

### Типы и matching

В Java часто ловят по классу:

```java
catch (UserNotFoundException e) {
    ...
}
```

В Go:

```go
var e *UserNotFoundError
if errors.As(err, &e) {
	// ...
}
```

Для категории без полей:

```go
if errors.Is(err, ErrUserNotFound) {
	// ...
}
```

Java class hierarchy часто толкает к иерархии exception-классов:

```text
AppException
  ValidationException
  ConflictException
  NotFoundException
```

Go обычно не строит большую error hierarchy. Часто достаточно:

- несколько sentinel categories;
- несколько custom types с полями;
- interface matching там, где нужна capability.

Например:

```go
type Temporary interface {
	Temporary() bool
}

var temp Temporary
if errors.As(err, &temp) && temp.Temporary() {
	// retry
}
```

Это ближе к "ошибка имеет свойство", а не "ошибка является потомком класса".

### Null и nil

В Java:

```java
return null;
```

может означать "нет результата", "ошибка", "не найдено" или bug, если контракт
плохо описан. Exceptions частично решают это: вместо `null` можно бросить
исключение.

В Go для "значения нет, потому что ошибка" обычно возвращают zero value плюс
`error`:

```go
user, err := FindUser(id)
if err != nil {
	return err
}
```

Для "значения нет, но это нормальная ветка" часто возвращают `ok`:

```go
user, ok := cache.Get(id)
if !ok {
	// cache miss
}
```

Это важное разделение:

```text
error -> операция не смогла успешно выполниться
ok    -> результата нет, но это ожидаемая альтернатива без объяснения
```

### Performance и allocation

Нельзя выбирать модель ошибок только из-за performance, но механика разная.

В Java создание exception обычно собирает stack trace, это может быть дорогим.
Поэтому exception для обычного ветвления считается плохим стилем.

В Go обычный `error` может быть очень дешевым:

```go
var ErrNotFound = errors.New("not found")
```

Это одно значение, которое можно возвращать много раз. Но `fmt.Errorf` обычно
создает новую ошибку и строку. Это нормально для failure path, но не надо
использовать ошибки как горячий path для ожидаемого результата, где лучше `ok`.

Пример:

```go
value, ok := m[key] // cache miss is not necessarily an error
```

## 27. Типовые ошибки на ревью

### Ошибка 1. Проверяют строку ошибки

```go
if err != nil && err.Error() == "user not found" {
	// ...
}
```

Проблема: текст поменяется, локализуется, получит контекст, и проверка
сломается.

Лучше:

```go
if errors.Is(err, ErrUserNotFound) {
	// ...
}
```

### Ошибка 2. Теряют причину через `%v`

```go
return fmt.Errorf("load user: %v", err)
```

Если caller должен уметь проверить нижнюю ошибку:

```go
return fmt.Errorf("load user: %w", err)
```

Но если нижняя ошибка не должна быть API, `%v` может быть правильнее. Вопрос
не в синтаксисе, а в контракте.

### Ошибка 3. Возвращают слишком общую ошибку

```go
return errors.New("failed")
```

Лучше:

```go
return fmt.Errorf("reserve stock for sku %q: %w", sku, err)
```

### Ошибка 4. Логируют и возвращают на каждом уровне

```go
logger.Error("repo failed", "err", err)
return err
```

Если выше тоже залогирует, получится дубль. Добавь контекст и верни, а логируй
на границе.

### Ошибка 5. `panic` вместо обычной ошибки

```go
if userID == "" {
	panic("empty user id")
}
```

Для request input лучше:

```go
if userID == "" {
	return errors.New("empty user id")
}
```

### Ошибка 6. Скрывают panic через пустой recover

```go
defer func() {
	_ = recover()
}()
```

Так можно потерять серьезный bug. Если recover нужен, минимум логируй, метри и
понимай, можно ли продолжать.

### Ошибка 7. Typed nil error

```go
var e *MyError
return e // err != nil у caller-а
```

Нужно:

```go
if e == nil {
	return nil
}
return e
```

### Ошибка 8. Непонятный API contract

```go
func FindUser(id string) (User, error)
```

Неясно:

- что вернется, если пользователя нет;
- можно ли проверять `ErrUserNotFound`;
- может ли быть `context.Canceled`;
- какие ошибки считаются retryable.

Лучше документировать exported behavior:

```go
// FindUser returns an error matching ErrUserNotFound when the user does not exist.
func FindUser(ctx context.Context, id string) (User, error)
```

## 28. Что ревьюить в коде с ошибками

1. Все ли returned errors проверяются?
2. Есть ли контекст у ошибок, которые идут вверх?
3. Используется ли `%w` только там, где нижняя ошибка должна быть доступна caller-у?
4. Проверяют ли ошибки через `errors.Is/As`, а не через строки?
5. Есть ли стабильные domain errors для ожидаемых бизнесовых случаев?
6. Не протекают ли наружу implementation details вроде `sql.ErrNoRows`, если это не часть API?
7. Не логируется ли одна и та же ошибка на каждом слое?
8. Не используется ли `panic` для обычной валидации/user input/network errors?
9. Если есть `recover`, не скрывает ли он programmer bugs?
10. Не возвращается ли typed nil error?
11. Обрабатываются ли `context.Canceled` и `context.DeadlineExceeded` отдельно, если это важно?
12. Для validation/batch/fan-out понятно ли, нужна первая ошибка или aggregate?
13. Ошибка `Close/Flush/Commit/Rollback` не теряется там, где она важна?
14. Тесты проверяют категорию/тип, а не fragile message?

## 29. Готовые ответы для собеседования

### Что такое `error` в Go?

> `error` - встроенный интерфейс `interface { Error() string }`. Любой тип с
> методом `Error() string` может быть ошибкой. По конвенции функции возвращают
> `error` последним значением, а `nil` означает отсутствие ошибки.

### Почему в Go ошибки возвращаются, а не бросаются?

> Go делает штатные ошибки явными значениями. Это делает failure path видимым в
> сигнатуре и в теле функции: caller рядом с вызовом решает, обработать ошибку,
> вернуть выше или добавить контекст. Для неожиданных programmer bugs есть
> `panic`, но это не замена обычной обработке ошибок.

### Чем `%w` отличается от `%v`?

> `%v` добавляет текст ошибки в новое сообщение. `%w` тоже добавляет текст, но
> дополнительно сохраняет исходную ошибку в цепочке `Unwrap`, поэтому ее можно
> найти через `errors.Is` или `errors.As`. Выбор `%w` - это API decision:
> нижняя ошибка становится доступна вызывающему коду.

### Когда использовать `errors.Is`?

> Когда нужно проверить конкретную ошибку или категорию, включая wrapped errors:
> `errors.Is(err, ErrNotFound)`. Прямое `err == ErrNotFound` работает только
> без wrapping и поэтому в современном коде обычно хуже.

### Когда использовать `errors.As`?

> Когда нужно найти ошибку конкретного типа и достать из нее поля:
> `var e *ValidationError; if errors.As(err, &e) { ... }`. `As` проходит по
> error chain, поэтому работает даже если ошибка была обернута.

### Что такое sentinel error?

> Это заранее объявленное значение ошибки, например `var ErrNotFound =
> errors.New("not found")`. Caller может проверять его через `errors.Is`.
> Sentinel хорош для стабильной категории без полей, но exported sentinel
> становится частью API.

### Когда нужен custom error type?

> Когда ошибка должна нести структурные данные: поле валидации, resource/id,
> operation/path, retry metadata. Тогда тип реализует `Error() string`, иногда
> `Unwrap() error` или `Is(error) bool`, а caller использует `errors.As`.

### Когда использовать `panic`?

> Для обычных проблем окружения, пользовательского ввода, сети и БД нужно
> возвращать `error`. `panic` уместен для programmer bugs, невозможных
> инвариантов, fatal startup-состояний или как внутренняя техника пакета, если
> exported API превращает panic обратно в `error`.

### Можно ли recover-нуть panic из другой goroutine?

> Нет. `recover` работает только в deferred function той же goroutine, которая
> panicking. Если worker goroutine должна переживать panic, recover должен быть
> внутри этой goroutine.

### Чем Go errors отличаются от Java exceptions?

> В Java exception делает non-local jump к `catch` и обычно несет stack trace. В
> Go обычная ошибка - return value. Она не разматывает стек сама, не содержит
> stack trace автоматически и требует явной проверки. Зато failure path виден
> локально, а ошибка остается обычным значением, которое можно оборачивать,
> сравнивать, объединять и передавать.

## 30. Мини-шпаргалка

```go
// Simple error
return errors.New("empty user id")

// Error with data in message
return fmt.Errorf("user %q not found", id)

// Wrap cause
return fmt.Errorf("load user %q: %w", id, err)

// Check category
if errors.Is(err, ErrUserNotFound) {
	// ...
}

// Extract typed error
var validationErr *ValidationError
if errors.As(err, &validationErr) {
	// ...
}

// Join independent errors
return errors.Join(err1, err2)

// Preserve cancellation
select {
case <-ctx.Done():
	return ctx.Err()
}
```

Короткая итоговая формула:

> Ошибки в Go - это не "исключения без синтаксического сахара", а отдельная
> модель: явные значения, явные контракты, контекст через wrapping, проверка
> через `errors.Is/As`, `panic` только для исключительных границ.
