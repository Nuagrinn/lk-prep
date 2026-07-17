// Задача LC06-5: delayed batch flusher через time.Timer.
//
// Реализуй структуру, которая накапливает строки и сбрасывает их пачкой,
// если с момента первого элемента прошло maxDelay.
//
// Это маленькая прикладная задача на работу с таймером:
// например, сервис получает события часто, но писать наружу хочет пачками.
//
// Нужно реализовать:
//
//	type BatchFlusher struct { ... }
//
//	func NewBatchFlusher(maxDelay time.Duration, flush func([]string)) *BatchFlusher
//	func (b *BatchFlusher) Add(value string)
//	func (b *BatchFlusher) Stop()
//
// Правила:
//   - Add добавляет value во внутренний buffer.
//   - Когда в пустой buffer добавлен первый элемент, должен стартовать timer.
//   - Когда timer срабатывает, все накопленные значения передаются в flush.
//   - После flush buffer должен стать пустым.
//   - Если после flush приходит новый Add, timer должен стартовать заново.
//   - Stop должен сбросить оставшиеся значения, если они есть.
//   - Stop должен быть безопасен для повторного вызова.
//   - flush нельзя вызывать под mutex, если ты используешь mutex внутри.
//
// Для тренировки можешь не делать отдельную goroutine на каждый Add. Достаточно
// простой структуры с mutex + time.AfterFunc или mutex + time.Timer.
//
// Подсказка:
// Самая короткая реализация обычно получается через time.AfterFunc:
//   - при первом элементе сохранить timer;
//   - callback таймера вызывает внутренний flush;
//   - Stop останавливает timer и вручную flush-ит остаток.
//
// Проверь себя:
//   - Add("a"), Add("b"), подождать maxDelay -> flush(["a", "b"])
//   - Add("c") после первого flush -> отдельный flush(["c"])
//   - Add("x"), Stop() до maxDelay -> flush(["x"])
//   - Stop(), Stop() -> второй вызов ничего не ломает и не flush-ит повторно.
package main

import (
	"fmt"
	"sync"
	"time"
)

/*
я не знаю, как сделать по тому тз с функцией в конструкторе,
поэтому сделаю свою реализацию, как умею, а потом уже буду доделывать
*/

/*
Отчет после ревью первой попытки.

Главный пробел был не в timer, а в понимании callback. В условии параметр
flush func([]string) означает: "когда пачка готова, вызови переданную снаружи
функцию и отдай ей batch". BatchFlusher не должен сам решать, куда отправлять
данные: в консоль, БД, Kafka или тестовый slice. Он только копит элементы,
выбирает момент сброса и вызывает flush(batch).

Что пошло не так:
  - конструктор получился без flush, поэтому структура не умеет отдавать batch
    наружу;
  - timer очищает buffer через f.buffer = f.buffer[:0], но это просто потеря
    данных, а не flush;
  - Stop тоже очищает остаток, хотя по условию должен сбросить его через flush;
  - f.timer записывается в startFlush без mutex, а читается в stop под mutex -
    это shared state и потенциальная data race;
  - string возвращает внутренний slice наружу, из-за чего вызывающий код может
    получить alias на внутренний buffer.

Как решать такие задачи в следующий раз:
  1. Сначала расшифровать сигнатуру. Если видишь параметр вида func(...), это
     поведение, которое передали снаружи. Его обычно надо сохранить в struct.
  2. Разделить ответственности: структура управляет состоянием и временем,
     callback делает внешний эффект.
  3. Для flush под lock только забрать batch и очистить внутренний buffer.
     Сам callback вызывать уже после unlock.
  4. Если batch основан на slice, перед передачей наружу часто безопаснее
     сделать копию: batch := append([]string(nil), f.buffer...).
  5. Все поля, которые читают/пишут разные goroutine, защищать одним и тем же
     mutex или другой явной синхронизацией.
*/

type BatchFlusher struct {
	buffer   []string
	maxDelay time.Duration
	mu       sync.Mutex
	timer    *time.Timer
}

/*
Логика: мы добавляем новый элемент. Если элемент - первый, то он запускает таймер,
по истечению которого запускается flush.
*/

func NewBathFlusher(maxDelay time.Duration) *BatchFlusher {
	return &BatchFlusher{
		maxDelay: maxDelay,
	}
}

func (f *BatchFlusher) startFlush() {
	f.timer = time.NewTimer(f.maxDelay)
	
	select {
	case <-f.timer.C:
		f.mu.Lock()
		f.buffer = f.buffer[:0]
		f.mu.Unlock()
	}
}

func (f *BatchFlusher) add(s string) {
	f.mu.Lock()
	if len(f.buffer) == 0 {
		f.buffer = append(f.buffer, s)
		go f.startFlush()
	} else {
		f.buffer = append(f.buffer, s)
	}
	f.mu.Unlock()
}

func (f *BatchFlusher) stop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.timer != nil {
		f.timer.Stop()
	}
	f.buffer = f.buffer[:0]
}

func (f *BatchFlusher) string() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.buffer
}

func main() {
	batchFlusher := NewBathFlusher(3 * time.Second)
	batchFlusher.add("a")
	batchFlusher.add("b")
	fmt.Println(batchFlusher.string())
	time.Sleep(4 * time.Second)
	fmt.Println(batchFlusher.string())
	batchFlusher.add("c")
	batchFlusher.add("d")
	fmt.Println(batchFlusher.string())
	batchFlusher.stop()
	batchFlusher.stop()
	fmt.Println(batchFlusher.string())
}
