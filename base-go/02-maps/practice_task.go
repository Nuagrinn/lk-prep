package main

import "fmt"

type Purchase struct {
	UserID   string
	Category string
	Amount   int
}

type CategoryStat struct {
	Category string
	Buyers   int
	Revenue  int
}

func BuildCategoryStats(purchases []Purchase) []CategoryStat {
	// TODO:
	// 1. For each category, count total revenue.
	// 2. For each category, count unique buyers by UserID.
	// 3. Return one CategoryStat per category.
	// Result order is not important.
	return nil
}

func main() {
	purchases := []Purchase{
		{UserID: "u1", Category: "books", Amount: 100},
		{UserID: "u2", Category: "books", Amount: 70},
		{UserID: "u1", Category: "books", Amount: 30},
		{UserID: "u2", Category: "games", Amount: 200},
		{UserID: "u3", Category: "games", Amount: 50},
	}
	
	fmt.Printf("%+v\n", BuildCategoryStats(purchases))
}
