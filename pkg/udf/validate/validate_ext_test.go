package validate

import (
	"fmt"
	"testing"
)

func TestIsSemver(t *testing.T) {
	if got := run(t, `"1.2.3" | is_semver`); got != true {
		t.Error("1.2.3 is_semver = false")
	}
	if got := run(t, `"2.0.0-rc.1+build.5" | is_semver`); got != true {
		t.Error("prerelease is_semver = false")
	}
	if got := run(t, `"v1.2.3" | is_semver`); got != false {
		t.Error("v1.2.3 is_semver = true")
	}
}

func TestIsCreditCard(t *testing.T) {
	// 4111 1111 1111 1111 passes Luhn.
	if got := run(t, `"4111111111111111" | is_credit_card`); got != true {
		t.Error("valid card = false")
	}
	if got := run(t, `"4111111111111112" | is_credit_card`); got != false {
		t.Error("invalid card = true")
	}
	if got := fmt.Sprint(run(t, `"4111 1111 1111 1111" | is_credit_card`)); got != "true" {
		t.Errorf("spaced card = %s", got)
	}
}
