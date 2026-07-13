package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

/*
Перед запуском ответь:

1. Что выведет receive из closed channel?
2. Почему select с nil channel уйдет в default?
3. Почему unbuffered send не завершается до receive?
4. Что доказывает WaitGroup, а что он не защищает?
5. Почему ValueCounter не меняется, хотя Inc вызывается два раза?
6. Чем channel ownership отличается от общей map/slice под mutex?
7. Почему atomic counter здесь корректен, а две связанные atomic-переменные
   не были бы полноценным snapshot-ом?
*/

type ValueCounter struct {
	n int
}

func (c ValueCounter) Inc() {
	c.n++
}

type GoodCounter struct {
	mu sync.Mutex
	n  int
}

func (c *GoodCounter) Inc() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
}

func demoClosedChannel() {
	ch := make(chan int, 1)
	ch <- 7
	close(ch)

	a, ok1 := <-ch
	b, ok2 := <-ch

	fmt.Println("closed:", a, ok1, b, ok2)
}

func demoNilChannelSelect() {
	var ch chan int

	select {
	case ch <- 1:
		fmt.Println("nil select: sent")
	case v := <-ch:
		fmt.Println("nil select: received", v)
	default:
		fmt.Println("nil select: default")
	}
}

func demoBufferedChannel() {
	ch := make(chan string, 2)
	ch <- "a"
	ch <- "b"

	first := <-ch

	fmt.Println("buffered:", first, len(ch), cap(ch))
}

func demoUnbufferedHandoff() {
	ch := make(chan string)
	done := make(chan string)

	go func() {
		ch <- "value"
		done <- "send returned"
	}()

	v := <-ch
	msg := <-done

	fmt.Println("unbuffered:", v, msg)
}

func demoWaitGroupWithMutex() {
	var wg sync.WaitGroup
	var mu sync.Mutex
	count := 0

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			mu.Lock()
			count++
			mu.Unlock()
		}()
	}

	wg.Wait()
	fmt.Println("mutex count:", count)
}

func demoValueReceiverCopy() {
	var value ValueCounter
	value.Inc()
	value.Inc()

	var good GoodCounter
	good.Inc()
	good.Inc()

	fmt.Println("receiver:", value.n, good.n)
}

func demoChannelOwnership() {
	jobs := make(chan int)
	results := make(chan int)

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				results <- job * job
			}
		}()
	}

	go func() {
		for _, job := range []int{1, 2, 3, 4} {
			jobs <- job
		}
		close(jobs)

		wg.Wait()
		close(results)
	}()

	sum := 0
	for result := range results {
		sum += result
	}

	fmt.Println("ownership sum:", sum)
}

func demoAtomicCounter() {
	var counter atomic.Int64
	var wg sync.WaitGroup

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			counter.Add(1)
		}()
	}

	wg.Wait()
	fmt.Println("atomic:", counter.Load())
}

func main() {
	demoClosedChannel()
	demoNilChannelSelect()
	demoBufferedChannel()
	demoUnbufferedHandoff()
	demoWaitGroupWithMutex()
	demoValueReceiverCopy()
	demoChannelOwnership()
	demoAtomicCounter()
}
