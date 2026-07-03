package main

import "fmt"

func dumpSlice(name string, s []int) {
	if cap(s) == 0 {
		fmt.Printf("%s: visible=%v len=%d cap=%d cap-window=%v data=<nil>\n", name, s, len(s), cap(s), s[:cap(s)])
		return
	}
	
	full := s[:cap(s)]
	fmt.Printf("%s: visible=%v len=%d cap=%d cap-window=%v data=%p\n", name, s, len(s), cap(s), full, &full[0])
}

func touch(values []int) {
	dumpSlice("touch before append", values)
	values = append(values, 7)
	dumpSlice("touch after append", values)
	values[0] = 100
	dumpSlice("touch after values[0]=100", values)
	fmt.Println("touch:", values, len(values), cap(values))
}

func grow(values []int) []int {
	values = append(values, 8, 9)
	values[1] = 200
	return values
}

func main() {
	x := make([]int, 3, 5) // 1 2 3 len 3 cap 5
	x[0], x[1], x[2] = 1, 2, 3
	
	dumpSlice("x before touch", x)
	touch(x)
	dumpSlice("x after touch", x)
	fmt.Println("x1:", x, len(x), cap(x)) // 100 2 3 7 len 4 cap 5
	
	y := grow(x)
	fmt.Println("x2:", x, len(x), cap(x)) // 100 2 3 7 len 4 cap 5
	fmt.Println("y:", y, len(y), cap(y))  // 100 200 3 7 8 9 len 6 cap 10
	
}
