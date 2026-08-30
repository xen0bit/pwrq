package fixture

func gate(enabled bool) string {
	// ruleid: go-hardcoded-if-condition
	if true {
		return "always"
	}
	// ruleid: go-hardcoded-if-condition
	if false {
		return "never"
	}
	// ruleid: go-hardcoded-if-condition
	if (true) {
		return "still always"
	}
	// ok: go-hardcoded-if-condition
	if enabled {
		return "sometimes"
	}
	// ok: go-hardcoded-if-condition
	if !enabled {
		return "the other times"
	}
	return "no"
}
