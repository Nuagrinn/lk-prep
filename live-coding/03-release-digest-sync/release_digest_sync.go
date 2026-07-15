// Задача LC06-3: примитивы синхронизации + map/slice - релизный дайджест.
//
// Это не алгоритмическая задача и не "напиши worker pool". Здесь проверяется,
// умеешь ли ты правильно спроектировать маленький конкурентный сценарий:
// запустить независимые операции параллельно, собрать результат без data race,
// не зависеть от случайного порядка завершения goroutine и аккуратно обработать
// ошибки.
//
// Реализуй:
//
//	func BuildReleaseDigest(ctx context.Context, sources []Source, maxErrors int) (Digest, error)
//
// Есть несколько источников событий по релизу: web, mobile, api, support и т.д.
// Каждый Source умеет Load(ctx) и возвращает []Event или ошибку. Источники
// независимы, поэтому их нужно загружать конкурентно: по одной goroutine на
// source.
//
// После загрузки успешные источники нужно смержить ДЕТЕРМИНИРОВАННО:
// в порядке, в котором sources переданы во входном слайсе, а внутри каждого
// source - в порядке Events. Нельзя строить итог по порядку прихода результатов
// из goroutine, иначе результат будет зависеть от scheduler/delay.
//
// Правила merge:
//   - Event с пустым ID или пустым UserID игнорируется.
//   - Event.ID глобально дедуплицируется: если такой ID уже был принят из
//     более раннего source/event, повтор игнорируется.
//   - Kind == "like" увеличивает Likes пользователя.
//   - Kind == "bug" увеличивает Bugs пользователя.
//   - Kind == "label" добавляет Label пользователю.
//   - Labels у пользователя должны быть уникальными, но сохранять порядок
//     первого появления.
//   - Digest.AcceptedIDs должен хранить ID принятых событий в порядке merge.
//   - Digest.Users должен быть отсортирован по UserID по возрастанию.
//
// Обработка ошибок:
//   - Если количество source errors стало больше maxErrors, верни ошибку.
//   - При раннем fail желательно отменить оставшиеся загрузки через context,
//     но нельзя оставить goroutine висеть.
//   - Если ctx отменен снаружи, верни ошибку.
//
// Что здесь проверяется:
//   - goroutine + WaitGroup или channel для сбора результатов;
//   - отсутствие конкурентной записи в map/slice;
//   - детерминированный merge после конкурентной загрузки;
//   - правильное владение cancellation;
//   - аккуратная работа с map для dedup и slice для порядка.
package main

/*
Решение/проблемы:

Запуск каждого Source в отедльной горутине с контекстом, через wg

собрать слайс пар Item (порядок:эвент), его проще потом отсортировать и мержить результат по циклам
отдельный метод валидации по правилам мержа. Запись делаем через mutex

отдельные атомик счетчики для ивентов и ошибок

Валидация:

  - Event с пустым ID или пустым UserID игнорируется. (пропускаем через if)
  - Event.ID глобально дедуплицируется: если такой ID уже был принят из
    более раннего source/event, повтор игнорируется. (map[int]strunct{} - проверка через сет)
  - Kind == "like" увеличивает Likes пользователя. (через case)
  - Kind == "bug" увеличивает Bugs пользователя.
  - Kind == "label" добавляет Label пользователю. (держать сет и слайс, проверяем сет, либо аппендим, либо нет)
  - Labels у пользователя должны быть уникальными, но сохранять порядок
    первого появления.
  - Digest.AcceptedIDs должен хранить ID принятых событий в порядке merge.
  - Digest.Users должен быть отсортирован по UserID по возрастанию. (сортируем слайс по id юзеров sortFunc, cmp.Compare)

*/

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

type Event struct {
	ID     string
	UserID string
	Kind   string
	Label  string
}

type Source struct {
	Name   string
	Delay  time.Duration
	Events []Event
	Err    error
}

func (s Source) Load(ctx context.Context) ([]Event, error) {
	timer := time.NewTimer(s.Delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
	}

	if s.Err != nil {
		return nil, fmt.Errorf("%s: %w", s.Name, s.Err)
	}

	out := make([]Event, len(s.Events))
	copy(out, s.Events)
	return out, nil
}

type UserDigest struct {
	UserID string
	Likes  int
	Bugs   int
	Labels []string
}

type Digest struct {
	AcceptedIDs []string
	Users       []UserDigest
}

type DigAccum struct {
	ud     UserDigest
	labels map[string]struct{}
}

type Pair struct {
	id string
	ud UserDigest
}

func BuildReleaseDigest(ctx context.Context, sources []Source, maxErrors int) (Digest, error) {
	// TODO: реализуй.

	mu := sync.Mutex{}
	wg := sync.WaitGroup{}
	var errCnt atomic.Int64
	sourceResults := make([][]Event, len(sources))
	childCtx, cancelChild := context.WithCancel(ctx)
	defer cancelChild()

	for i, source := range sources {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := source.Load(childCtx)
			if err != nil {
				errCnt.Add(1)
				if errCnt.Load() > int64(maxErrors) {
					cancelChild()
					return
				}
			}
			mu.Lock()
			defer mu.Unlock()
			sourceResults[i] = result
		}()
	}

	wg.Wait()

	eventsIDs := make(map[string]struct{})

	dig := Digest{
		AcceptedIDs: make([]string, 0, len(sources)),
		Users:       make([]UserDigest, 0, len(sources)),
	}

	userDigAccum := make(map[string]*DigAccum)
	for _, es := range sourceResults {
		for _, e := range es {
			if len(e.ID) == 0 || len(e.UserID) == 0 {
				continue
			}
			if _, ok := eventsIDs[e.ID]; ok {
				continue
			}
			eventsIDs[e.ID] = struct{}{}
			eventsIDs[e.UserID] = struct{}{}
			dig.AcceptedIDs = append(dig.AcceptedIDs, e.ID)

			if _, ok := userDigAccum[e.UserID]; !ok {
				userDigAccum[e.UserID] = &DigAccum{
					labels: make(map[string]struct{}),
				}
			}
			uda := userDigAccum[e.UserID]
			uda.ud.UserID = e.UserID
			switch e.Kind {
			case "like":
				uda.ud.Likes++
			case "bug":
				uda.ud.Bugs++
			case "label":
				if _, ok := uda.labels[e.Label]; !ok {
					uda.labels[e.Label] = struct{}{}
					uda.ud.Labels = append(uda.ud.Labels, e.Label)
				}
			}

		}
	}

	slToSort := make([]Pair, 0, len(userDigAccum))
	for _, uda := range userDigAccum {
		p := Pair{
			id: uda.ud.UserID,
			ud: uda.ud,
		}
		slToSort = append(slToSort, p)
	}

	slices.SortFunc(slToSort, func(a, b Pair) int {
		return cmp.Compare(a.id, b.id)
	})

	usersDigests := make([]UserDigest, 0, len(slToSort))
	for _, sl := range slToSort {
		usersDigests = append(usersDigests, sl.ud)
	}

	dig.Users = usersDigests

	return dig, nil
}

func main() {
	runDigestExample(
		"Пример 1: успешный deterministic merge",
		[]Source{
			{
				Name:  "mobile",
				Delay: 30 * time.Millisecond,
				Events: []Event{
					{ID: "e1", UserID: "u1", Kind: "like"},
					{ID: "e2", UserID: "u2", Kind: "bug"},
					{ID: "e3", UserID: "u1", Kind: "label", Label: "ios"},
				},
			},
			{
				Name:  "web",
				Delay: 5 * time.Millisecond,
				Events: []Event{
					{ID: "e2", UserID: "u3", Kind: "like"}, // duplicate ID, ignored
					{ID: "e4", UserID: "u2", Kind: "label", Label: "checkout"},
					{ID: "e5", UserID: "u1", Kind: "like"},
				},
			},
			{
				Name:  "api",
				Delay: 10 * time.Millisecond,
				Events: []Event{
					{ID: "", UserID: "u9", Kind: "like"}, // invalid, ignored
					{ID: "e6", UserID: "u3", Kind: "bug"},
					{ID: "e7", UserID: "u2", Kind: "label", Label: "api"},
					{ID: "e8", UserID: "u2", Kind: "label", Label: "checkout"}, // duplicate label
				},
			},
		},
		2,
		Digest{
			AcceptedIDs: []string{"e1", "e2", "e3", "e4", "e5", "e6", "e7", "e8"},
			Users: []UserDigest{
				{UserID: "u1", Likes: 2, Bugs: 0, Labels: []string{"ios"}},
				{UserID: "u2", Likes: 0, Bugs: 1, Labels: []string{"checkout", "api"}},
				{UserID: "u3", Likes: 0, Bugs: 1, Labels: nil},
			},
		},
	)

	runErrorExample(
		"Пример 2: ошибок больше maxErrors",
		[]Source{
			{
				Name:  "mobile",
				Delay: 5 * time.Millisecond,
				Events: []Event{
					{ID: "e1", UserID: "u1", Kind: "like"},
				},
			},
			{Name: "web", Delay: 10 * time.Millisecond, Err: errors.New("timeout")},
			{Name: "api", Delay: 15 * time.Millisecond, Err: errors.New("bad gateway")},
		},
		1,
	)
}

func runDigestExample(name string, sources []Source, maxErrors int, expected Digest) {
	got, err := BuildReleaseDigest(context.Background(), sources, maxErrors)
	status := "MISMATCH"
	if err == nil && reflect.DeepEqual(got, expected) {
		status = "OK"
	}

	fmt.Printf("%s [%s]\n", name, status)
	fmt.Printf("  err:      %v\n", err)
	fmt.Printf("  got:      %+v\n", got)
	fmt.Printf("  expected: %+v\n", expected)
}

func runErrorExample(name string, sources []Source, maxErrors int) {
	got, err := BuildReleaseDigest(context.Background(), sources, maxErrors)
	status := "MISMATCH"
	if err != nil {
		status = "OK"
	}

	fmt.Printf("%s [%s]\n", name, status)
	fmt.Printf("  err:    %v\n", err)
	fmt.Printf("  digest: %+v\n", got)
}
