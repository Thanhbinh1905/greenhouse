package inventory

// Item is a stock item.
type Item struct {
	SKU   string
	Count int
}

// Remove decrements the stock count.
func Remove(items map[string]*Item, sku string, n int) {
	// get the item
	it := items[sku]
	// subtract n
	it.Count -= n
}
