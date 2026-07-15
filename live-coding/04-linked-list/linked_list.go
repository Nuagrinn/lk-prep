// Задача LC06-4: односвязный список.
//
// Реализуй с нуля простой linked list для int.
//
// Нужно самостоятельно описать:
//   - структуру узла списка;
//   - структуру самого списка;
//   - методы списка.
//
// Методы, которые должны появиться:
//   - PushFront(value int)
//   - PushBack(value int)
//   - InsertAfter(target int, value int) bool
//   - Remove(value int) bool
//   - Contains(value int) bool
//   - ToSlice() []int
//
// Правила:

//   - ToSlice возвращает значения списка в текущем порядке.
//
// Проверь себя на сценариях:
//   - пустой список;
//   - вставка в начало;
//   - вставка в конец;
//   - вставка в середину через InsertAfter;
//   - удаление head-узла;
//   - удаление узла из середины;
//   - удаление отсутствующего значения.
package main

import "fmt"

type Node struct {
	next, prev *Node
	value      int
}

type LinkedList struct {
	head *Node
}

func NewLL() *LinkedList {
	return &LinkedList{}
}

// PushFront - добавляет новый узел в начало списка.
func (l *LinkedList) PushFront(value int) {
	n := &Node{
		value: value,
	}

	if l.head == nil {
		l.head = n
	} else {
		l.head.prev = n
		n.next = l.head
		l.head = n
	}
}

// PushBack - добавляет новый узел в конец списка.
func (l *LinkedList) PushBack(value int) {
	n := &Node{
		value: value,
	}
	last := l.Last()
	if last == nil {
		l.head = n
	} else {
		last.next = n
		n.prev = last
	}
}

// InsertAfter - ищет первый узел со значением target и вставляет value
//
//	сразу после него. Если target найден - возвращает true, иначе false.
func (l *LinkedList) InsertAfter(target int, value int) bool {
	curr := l.head

	for curr != nil {
		if curr.value == target {
			n := &Node{
				value: value,
			}

			nextNode := curr.next
			if nextNode != nil {
				n.next = nextNode
				nextNode.prev = n
			}
			n.prev = curr
			curr.next = n
			return true
		}
		curr = curr.next
	}
	return false
}

// Remove - удаляет первое найденное значение и возвращает true. Если
//
//	значения нет - возвращает false.
func (l *LinkedList) Remove(value int) bool {
	curr := l.head

	for curr != nil {
		if curr.value == value {
			prevNode := curr.prev
			nextNode := curr.next

			if prevNode == nil {
				l.head = nextNode
			} else {
				prevNode.next = nextNode
			}

			if nextNode != nil {
				nextNode.prev = prevNode
			}

			return true
		}

		curr = curr.next
	}

	return false
}

func (l *LinkedList) ToSlice() []int {
	var slice []int
	curr := l.head
	for curr != nil {
		slice = append(slice, curr.value)
		curr = curr.next
	}
	return slice
}

// Contains - возвращает true, если значение есть в списке.
func (l *LinkedList) Contains(value int) bool {
	for curr := l.head; curr != nil; curr = curr.next {
		if curr.value == value {
			return true
		}
	}
	return false
}

func (l *LinkedList) Last() *Node {

	curr := l.head
	if curr == nil {
		return nil
	}
	for {
		if curr.next == nil {
			return curr
		}

		curr = curr.next
	}
}

func main() {

	ll := NewLL()
	ll.PushBack(1)
	//ll.PushBack(2)
	//ll.PushBack(3)
	//ll.PushBack(4)
	//ll.PushBack(5)
	//ll.InsertAfter(3, 99)
	//ll.Remove(3)
	//ll.Remove(5)
	//ll.InsertAfter(4, 99)
	//ll.PushFront(100)
	//ll.Remove(100)
	//ll.Remove(1)

	fmt.Println(ll.ToSlice())
}
