package fixture

import (
	"net/http"
	"testing"
)

// ruleid: go-entry-points
func main() {
	// ruleid: go-entry-points
	http.HandleFunc("/hello", hello)
	// ruleid: go-entry-points
	r.GET("/users/:id", getUser)
	// ok: go-entry-points
	cache.Get("/not-a-route", 1)
}

// ruleid: go-entry-points
func init() {}

// ruleid: go-entry-points
func TestHello(t *testing.T) {}

// ok: go-entry-points
func hello(w http.ResponseWriter, req *http.Request) {}
