package main

import (
	"fmt"
)

func main() {
	s := "Hello"
	//fmt.Println(s[0])
	//fmt.Println(len(s))
	//sb := []byte(s)
	//sr := []rune(s)
	//fmt.Println(sb)
	//fmt.Println(sr)
	//fmt.Println(string(sb))
	//fmt.Println(string(sr))
	
	for _, b := range s {
		fmt.Println(string(b))
	}
	
	//for i := 0; i < len(s); i++ {
	//	fmt.Println(string(s[i]))
	//}
	
}
