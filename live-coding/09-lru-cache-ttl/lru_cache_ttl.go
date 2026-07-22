// Задача LC06-9: in-memory LRU cache с TTL.
//
// Реализуй небольшой потокобезопасный кэш строковых значений.
//
// Нужно реализовать:
//
//	type Cache struct { ... }
//
//	func NewCache(capacity int, now func() time.Time) *Cache
//	func (c *Cache) Set(key, value string, ttl time.Duration)
//	func (c *Cache) Get(key string) (string, bool)
//	func (c *Cache) Delete(key string)
//	func (c *Cache) Len() int
//

// Подсказка по реализации:
//   - для быстрых операций обычно берут map[string]*list.Element;
//   - порядок LRU удобно хранить в container/list;
//   - в list.Element.Value можно положить struct с key, value и expiresAt;
//   - все операции с map/list защищай одним mutex;
//   - now передается в конструктор, чтобы тестировать TTL без настоящего sleep.
//
// Что стоит проверить вручную:
//   - Get возвращает сохраненное значение до истечения TTL.
//   - Истекший ключ удаляется и больше не считается в Len.
//   - При переполнении capacity удаляется least recently used ключ.
//   - Успешный Get меняет recency.
//   - Повторный Set обновляет value, TTL и recency.
//   - ttl <= 0 удаляет существующий ключ.
//   - Delete удаляет существующий ключ и спокойно обрабатывает отсутствующий.
//   - Некорректный capacity и nil clock приводят к panic.
package main

import (
	"container/list"
	"fmt"
	"sync"
	"time"
)

type Item struct {
	key       string
	value     string
	expiresAt time.Time
}

type Cache struct {
	capacity     int
	fastStore    map[string]*list.Element
	orderedStore *list.List
	now          func() time.Time
	mu           *sync.RWMutex
}

// Правила:
//   - capacity задает максимальное количество живых элементов.
//   - NewCache должен panic-нуть, если capacity <= 0 или now == nil.
//   - Set добавляет новый ключ или обновляет существующий.
//   - ttl <= 0 означает "не хранить": такой Set должен удалить существующий ключ,
//     если он был, и не добавлять новый.
//   - Get возвращает value, true, если ключ есть и срок жизни не истек.
//   - Если ключ истек, Get удаляет его и возвращает "", false.
//   - Успешный Get делает элемент самым недавно использованным.
//   - Set нового или существующего ключа тоже делает элемент самым недавно
//     использованным.
//   - Если после Set размер больше capacity, нужно удалить least recently used
//     элемент.Key
//   - Delete удаляет ключ, если он есть.
//   - Len возвращает количество живых элементов. Истекшие элементы можно чистить
//     лениво внутри Len.
//   - Все публичные методы должны быть безопасны для конкурентного вызова.
//

func main() {
	
	current := time.Now()
	cache := NewCache(10, func() time.Time {
		return current
	})
	
	cache.Set("a", "100", 20*time.Second)
	current = current.Add(30 * time.Second)
	v, ok := cache.Get("a")
	fmt.Println(v, ok)
}

func NewCache(capacity int, now func() time.Time) *Cache {
	if capacity <= 0 || now == nil {
		panic("invalid capacity or nil time")
	}
	
	return &Cache{
		capacity:     capacity,
		fastStore:    make(map[string]*list.Element, capacity),
		orderedStore: list.New(),
		now:          now,
		mu:           &sync.RWMutex{},
	}
}

/*
проверяет ttl - дальше или делит
проверяем фаст стор. Проверяем наличие в мап:
есть - удаляем из оС старый элемент, переносим в конец и обновляем в фС
нет - добавляем в конец оС и добавляем в фС
*/

func (c *Cache) Set(key string, value string, ttl time.Duration) {
	if ttl <= 0 {
		c.Delete(key)
	}
	
	c.mu.Lock()
	item := &Item{}
	if v, ok := c.fastStore[key]; ok {
		c.orderedStore.Remove(v)
		item.expiresAt = c.now().Add(ttl)
		item.key = key
		item.value = value
		v.Value = item
		e := c.orderedStore.PushBack(item)
		c.fastStore[key] = e
	} else {
		v = &list.Element{}
		item.expiresAt = c.now().Add(ttl)
		item.key = key
		item.value = value
		v.Value = item
		e := c.orderedStore.PushBack(item)
		c.fastStore[key] = e
	}
	c.mu.Unlock()
	if c.Len() > c.orderedStore.Len() {
		c.orderedStore.Remove(c.orderedStore.Front())
	}
	
}

/*
/ ищем элемент в фС
есть - проверяем ttl если естек - возвраем фолс, удаляем из фс и ос, если не истек - вощвраем, перемещаем в конец Ос. обнолвяем ТТл
нет - возараем фолс
*/
func (c *Cache) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if v, ok := c.fastStore[key]; ok {
		item, ok := v.Value.(*Item)
		if !ok {
			return "", false
		}
		if item.expiresAt.Before(c.now()) {
			delete(c.fastStore, key)
			c.orderedStore.Remove(v)
			return "", false
		}
		c.orderedStore.MoveToBack(v)
		return item.value, true
	}
	return "", false
}

func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if v, ok := c.fastStore[key]; ok {
		delete(c.fastStore, key)
		c.orderedStore.Remove(v)
	}
	
}

func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.fastStore)
}
