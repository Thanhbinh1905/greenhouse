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

// ByEmail returns the orders placed with the email address from the search form.
func ByEmail(db *sql.DB, email string) ([]Order, error) {
	rows, err := db.Query("SELECT id, total FROM orders WHERE email = '" + email + "'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Order
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.ID, &o.Total); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
