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

	categoryStats := make([]CategoryStat, 0, len(purchases))

	uniqueCategory := make(map[string]struct{}, len(purchases))
	for _, p := range purchases {
		if _, ok := uniqueCategory[p.Category]; !ok {
			uniqueCategory[p.Category] = struct{}{}
		}
	}

	for k, _ := range uniqueCategory {
		catStatEntry := CategoryStat{}
		catStatEntry.Category = k
		catStatEntryUniqueUsers := make(map[string]struct{}, len(uniqueCategory))
		for _, purchase := range purchases {
			if purchase.Category == k {
				catStatEntry.Revenue += purchase.Amount
				catStatEntryUniqueUsers[purchase.UserID] = struct{}{}
			}
		}
		catStatEntry.Buyers = len(catStatEntryUniqueUsers)
		categoryStats = append(categoryStats, catStatEntry)
	}

	return categoryStats
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
