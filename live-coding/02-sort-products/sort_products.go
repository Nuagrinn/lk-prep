// Задача LC06-2: сортировка слайса структур по нескольким ключам.
//
// Реализуй:
//
//	func SortProducts(products []Product)
//
// Функция сортирует слайс на месте (in place), ничего не возвращает - как
// это принято для sort.Slice/slices.SortFunc в стандартной библиотеке.
//
//	type Product struct {
//		Name  string
//		Price float64
//		Stock int
//	}
//
// Порядок сортировки, в порядке приоритета ключей:
//  1. Товары в наличии (Stock > 0) идут раньше товаров не в наличии
//     (Stock == 0).
//  2. Внутри одной группы (в наличии / не в наличии) - по возрастанию Price.
//  3. При равной Price - по возрастанию Name (по алфавиту).
//
// Что здесь проверяется:
//   - сортировка слайса структур (не строк и не чисел напрямую) через
//     sort.Slice или slices.SortFunc;
//   - составной компаратор больше чем с двумя ключами, где первый ключ -
//     не поле само по себе, а вычисленное из поля условие (Stock > 0);
//   - что функция мутирует переданный слайс на месте, а не возвращает новый.
//
// Если не помнишь, чем sort.Slice отличается от slices.SortFunc и почему
// второй в общем случае быстрее - см. go-idioms/01-slices-maps-sorting/guide.md,
// там ровно этот вопрос разобран подробно.
package main

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

type Product struct {
	Name  string
	Price float64
	Stock int
}

func SortProducts(products []Product) {
	slices.SortFunc(products, func(a Product, b Product) int {
		return cmp.Or(
			cmp.Compare(inStockRank(a), inStockRank(b)),
			cmp.Compare(a.Price, b.Price),
			strings.Compare(a.Name, b.Name),
		)
		
	})
}

func inStockRank(p Product) int {
	if p.Stock > 0 {
		return 0
	}
	return 1
}

func main() {
	runExample(
		"Пример 1",
		[]Product{
			{"Mouse", 25.0, 0},
			{"Keyboard", 45.0, 10},
			{"Monitor", 199.99, 0},
			{"Webcam", 45.0, 5},
			{"Headset", 60.0, 0},
			{"Mousepad", 10.0, 20},
		},
		[]Product{
			{"Mousepad", 10.0, 20},
			{"Keyboard", 45.0, 10},
			{"Webcam", 45.0, 5},
			{"Mouse", 25.0, 0},
			{"Headset", 60.0, 0},
			{"Monitor", 199.99, 0},
		},
	)
	runExample(
		"Пример 2 (все в наличии, одинаковая цена - чистый алфавит)",
		[]Product{
			{"Charlie", 10.0, 1},
			{"Alpha", 10.0, 1},
			{"Bravo", 10.0, 1},
		},
		[]Product{
			{"Alpha", 10.0, 1},
			{"Bravo", 10.0, 1},
			{"Charlie", 10.0, 1},
		},
	)
}

func runExample(name string, products, expected []Product) {
	SortProducts(products)
	status := "MISMATCH"
	if slices.Equal(products, expected) {
		status = "OK"
	}
	fmt.Printf("%s [%s]\n", name, status)
	fmt.Printf("  got:      %v\n", products)
	fmt.Printf("  expected: %v\n", expected)
}
