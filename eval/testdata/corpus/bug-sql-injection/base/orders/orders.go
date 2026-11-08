package orders

import "database/sql"

type Order struct {
	ID    int64
	Total int64
}

// ByID returns one order.
func ByID(db *sql.DB, id int64) (Order, error) {
	var o Order
	err := db.QueryRow("SELECT id, total FROM orders WHERE id = ?", id).Scan(&o.ID, &o.Total)
	return o, err
}
