package fixture

import (
	"os"
	"strconv"
)

func f() {
	// ruleid: go-swallowed-errors
	n, _ := strconv.Atoi("1")
	// ruleid: go-swallowed-errors
	_ = os.Remove("x")
	err := os.Remove("y")
	// ruleid: go-swallowed-errors
	if err != nil {
	}
	// ok: go-swallowed-errors
	m, err := strconv.Atoi("2")
	if err != nil {
		panic(err)
	}
	_, _ = n, m
}
