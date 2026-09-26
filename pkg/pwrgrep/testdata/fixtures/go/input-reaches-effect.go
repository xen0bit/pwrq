package fixture

import (
	"database/sql"
	"net/http"
	"os"
	"os/exec"
)

func run() {
	cmd := os.Getenv("CMD")
	full := cmd + " --verbose"
	// ruleid: go-input-reaches-effect
	exec.Command(full).Run()
	// ok: go-input-reaches-effect
	exec.Command("ls").Run()
}

func handle(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	// ruleid: go-input-reaches-effect
	db.Query("SELECT * FROM users WHERE name = '" + name + "'")
	// ok: go-input-reaches-effect
	db.Query("SELECT 1")
	// ruleid: go-input-reaches-effect
	os.WriteFile(os.Args[1], nil, 0o644)
}

func query(w http.ResponseWriter, r *http.Request) {
	// ok: go-input-reaches-effect
	q := r.URL.Query()
	_ = q
}
