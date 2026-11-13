package inventory

// Item is a stock item.
type Item struct {
	SKU   string
	Count int
}

// InStock reports whether at least n units of sku are available.
func InStock(items map[string]*Item, sku string, n int) bool {
	it, ok := items[sku]
	return ok && it.Count >= n
}

// Remove decrements the stock count.
func Remove(items map[string]*Item, sku string, n int) {
	// get the item
	it := items[sku]
	// subtract n
	it.Count -= n
}
