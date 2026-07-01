package main

// Product is one item on the coffee menu. Unlike the image service's random
// products, the coffee shop sells a fixed menu at fixed prices.
type Product struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Emoji      string `json:"emoji"`
	Price      string `json:"price"`      // decimal string (money math is string-based)
	CurrencyID int64  `json:"currencyId"` // 1 = the demo currency
}

// menu is the coffee shop's fixed catalog.
var menu = []Product{
	{ID: "espresso", Title: "Espresso", Emoji: "☕", Price: "2.50", CurrencyID: 1},
	{ID: "latte", Title: "Latte", Emoji: "🥛", Price: "3.50", CurrencyID: 1},
	{ID: "cappuccino", Title: "Cappuccino", Emoji: "☕", Price: "3.00", CurrencyID: 1},
	{ID: "mocha", Title: "Mocha", Emoji: "🍫", Price: "4.00", CurrencyID: 1},
	{ID: "cold-brew", Title: "Cold Brew", Emoji: "🧊", Price: "3.75", CurrencyID: 1},
}

func productByID(id string) (Product, bool) {
	for _, p := range menu {
		if p.ID == id {
			return p, true
		}
	}
	return Product{}, false
}
