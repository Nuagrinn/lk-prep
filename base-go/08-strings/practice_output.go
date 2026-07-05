package main

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// dump печатает три уровня "длины" строки: байты, руны и валидность UTF-8.
func dump(label, s string) {
	fmt.Printf("%-10s %-12q bytes=%d runes=%d valid=%t\n",
		label, s, len(s), utf8.RuneCountInString(s), utf8.ValidString(s))
}

func reverseRunes(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

func main() {
	s := "Hello, 世界"

	// len считает байты, RuneCount считает руны.
	dump("s", s)         // bytes=13 runes=9 valid=true
	dump("s[7:]", s[7:]) // "世界"  bytes=6 runes=2 valid=true
	dump("s[:8]", s[:8]) // режем руну 世 пополам: bytes=8 runes=8 valid=false

	// Индексация возвращает байт, а не символ.
	fmt.Println("s[0] =", s[0])             // 72  (тип byte, код 'H')
	fmt.Printf("s[0] as char = %c\n", s[0]) // H

	ru := "Привет"
	fmt.Println("ru[0] =", ru[0])             // 208 (первый байт руны 'П', 0xD0)
	fmt.Printf("ru[0] as char = %c\n", ru[0]) // Ð  (половина руны -> мусор)

	// range декодирует UTF-8: индекс это байтовое смещение, значение это руна.
	fmt.Println("range indices of \"aé\":")
	for i, r := range "aé" {
		fmt.Printf("  i=%d rune=%c\n", i, r) // i=0 a ; i=1 é (не 2!)
	}

	// Индексный цикл ходит по байтам, range по рунам.
	byteCount, runeCount := 0, 0
	for i := 0; i < len(ru); i++ {
		byteCount++
	}
	for range ru {
		runeCount++
	}
	fmt.Printf("Привет: byteCount=%d runeCount=%d\n", byteCount, runeCount) // 12 и 6

	// []byte(s) это КОПИЯ: изменение слайса не трогает исходную строку.
	orig := "hello"
	b := []byte(orig)
	b[0] = 'H'
	fmt.Printf("orig=%q modified=%q\n", orig, string(b)) // orig="hello" modified="Hello"

	// Разворот по рунам сохраняет символы (в отличие от разворота по байтам).
	fmt.Println("reverse =", reverseRunes(ru)) // тевирП

	// Построение строки: Builder вместо += в цикле.
	var sb strings.Builder
	for _, part := range []string{"go", "p", "her"} {
		sb.WriteString(part)
	}
	fmt.Println("built =", sb.String()) // gopher
}
