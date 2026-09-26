package fixture

import (
	"database/sql"
	"net/http"
	"os"
	"os/exec"
)

func f(db *sql.DB) {
	// ruleid: go-side-effects
	exec.Command("ls").Run()
	// ruleid: go-side-effects
	os.WriteFile("out.txt", nil, 0o644)
	// ruleid: go-side-effects
	os.RemoveAll("tmp")
	// ruleid: go-side-effects
	http.Get("https://example.com")
	// ruleid: go-side-effects
	db.Query("SELECT 1")
	// ok: go-side-effects
	os.Getenv("HOME")
}
