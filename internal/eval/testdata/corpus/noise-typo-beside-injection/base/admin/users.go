package admin

import (
	"database/sql"
	"log"
)

// DeleteUser removes the named user.
func DeleteUser(db *sql.DB, name string) error {
	log.Printf("deleting usr %s", name)
	_, err := db.Exec("DELETE FROM users WHERE name = '" + name + "'")
	return err
}
