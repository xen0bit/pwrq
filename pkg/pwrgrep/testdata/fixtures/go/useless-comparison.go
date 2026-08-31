package fixture

func compare(a, b int) bool {
	// ruleid: go-useless-comparison
	if a == a {
		return true
	}
	// ruleid: go-useless-comparison
	if a != a {
		return false
	}
	// The original excludes this one: `1 == 1` is how a good deal of
	// generated and commented-out code spells "yes".
	// ok: go-useless-comparison
	if 1 == 1 {
		return true
	}
	// ok: go-useless-comparison
	if a == b {
		return true
	}
	// A test asserting something against itself is deliberate.
	// ok: go-useless-comparison
	assert(a == a)
	// ok: go-useless-comparison
	return a != b
}

func assert(bool) {}
