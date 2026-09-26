package fixture

import (
	"flag"
	"net/http"
	"os"
)

func f(w http.ResponseWriter, r *http.Request) {
	// ruleid: go-external-input
	home := os.Getenv("HOME")
	// ruleid: go-external-input
	first := os.Args[1]
	// ruleid: go-external-input
	port := flag.Int("port", 80, "")
	// ruleid: go-external-input
	body, _ := os.ReadFile("config.json")
	// ruleid: go-external-input
	name := r.FormValue("name")
	// ruleid: go-external-input
	q := r.URL.Query()
	// ok: go-external-input
	local := "constant"
	_, _, _, _, _, _, _ = home, first, port, body, name, q, local
}
