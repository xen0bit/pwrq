package fixture

// ruleid: go-variables
const limit = 10

// ruleid: go-variables
var global = 3

func f() int {
	// ruleid: go-variables
	x := 1
	// ruleid: go-variables
	var y int
	// ok: go-variables
	y = x
	// ruleid: go-variables
	a, b := 1, 2
	return y + a + b + global + limit
}
