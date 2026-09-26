package fixture

import "os"

func f(xs []int) {
	// ruleid: go-calls
	os.Exit(1)
	// ruleid: go-calls
	g()
	// ok: go-calls
	_ = xs[0]
}

func g() {}
