package fixture

import (
	"database/sql"
	"fmt"
)

func lookup(db *sql.DB, name string) (*sql.Rows, error) {
	// ruleid: go-sql-string-concat
	return db.Query("SELECT * FROM users WHERE name = '" + name + "'")
}

func remove(db *sql.DB, name string) error {
	// ruleid: go-sql-string-concat
	_, err := db.Exec(fmt.Sprintf("DELETE FROM users WHERE name = '%s'", name))
	return err
}

func lookupCtx(db *sql.DB, name string) (*sql.Rows, error) {
	// ruleid: go-sql-string-concat
	return db.QueryContext(nil, "SELECT * FROM users WHERE name = '"+name+"'")
}

func parameterised(db *sql.DB, name string) (*sql.Rows, error) {
	// ok: go-sql-string-concat
	return db.Query("SELECT * FROM users WHERE name = ?", name)
}

func constantQuery(db *sql.DB) (*sql.Rows, error) {
	// A query built only from literals carries nothing an attacker supplied,
	// and the original excludes it by name.
	// ok: go-sql-string-concat
	return db.Query("SELECT * FROM " + "users")
}
