// Задача LC06-1: строки + мапы + слайсы - топ-K частых слов.
//
// Реализуй:
//
//	func TopKFrequentWords(text string, k int) []string
//
// Дано произвольный текст text (заглавные и строчные буквы, знаки
// препинания, цифры, пробелы, возможны переносы строк) и число k.
//
// Слово - максимальная последовательность unicode-букв (unicode.IsLetter).
// Всё остальное (пробелы, знаки препинания, цифры) - разделитель, в слова
// не входит. Сравнение регистронезависимое: перед подсчётом приведи слово
// к нижнему регистру.
//
// Верни k самых часто встречающихся слов по убыванию частоты. При равной
// частоте - по алфавиту (по возрастанию). Если различных слов меньше k -
// верни все, что есть.
//
// Что здесь проверяется:
//   - подсчёт частот через map[string]int;
//   - аккуратная работа со строками: приведение регистра, отделение слов
//     от пунктуации без пробелов (strings.Fields в лоб оставит пунктуацию
//     приклеенной к слову - "fast." вместо "fast");
//   - сборка результата в []string и сортировка с двумя ключами
//     (sort.Slice: сначала по убыванию частоты, потом по алфавиту);
//   - обработку k, который больше числа уникальных слов.
package main

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode"
)

type Item struct {
	Word  string
	Count int
}

func TopKFrequentWords(text string, l int) []string {
	strRune := []rune(text)

	competeWord := false
	var word []rune
	wrdCnt := make(map[string]int)

	for _, r := range strRune {
		if unicode.IsLetter(r) {
			word = append(word, r)
			competeWord = true
		} else if competeWord {
			wrdCnt[strings.ToLower(string(word))]++
			competeWord = false
			word = []rune{}
		}
	}

	if competeWord {
		wrdCnt[strings.ToLower(string(word))]++
	}

	items := make([]Item, 0, len(wrdCnt))
	for k, v := range wrdCnt {
		items = append(items, Item{k, v})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Count != items[j].Count {
			return items[i].Count > items[j].Count
		}

		return items[i].Word < items[j].Word
	})

	if l > len(items) {
		l = len(items)
	}

	result := make([]string, 0, l)
	for i := 0; i < l; i++ {
		result = append(result, items[i].Word)
	}

	return result
}

func main() {

	runExample(
		"Пример 1",
		"Go is simple. Go is fast. Go is fun! Concurrency in Go is easy, and tooling in Go is great.",
		3,
		[]string{"go", "is", "in"},
	)
	runExample(
		"Пример 2 (k больше числа уникальных слов)",
		"A a a B b C",
		10,
		[]string{"a", "b", "c"},
	)
}

func runExample(name, text string, k int, expected []string) {
	got := TopKFrequentWords(text, k)
	status := "MISMATCH"
	if slices.Equal(got, expected) {
		status = "OK"
	}
	fmt.Printf("%s [%s]\n", name, status)
	fmt.Printf("  got:      %v\n", got)
	fmt.Printf("  expected: %v\n", expected)
}
