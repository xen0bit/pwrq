package validate

import (
	"regexp"
	"strings"

	"github.com/itchyny/gojq"
)

var semverPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)

// RegisterIsSemver registers is_semver, whether a string is a semantic version
// like 1.2.3 or 2.0.0-rc.1+build.
func RegisterIsSemver() gojq.CompilerOption {
	return registerBool("is_semver", func(s string) bool {
		return semverPattern.MatchString(strings.TrimSpace(s))
	})
}

// RegisterIsCreditCard registers is_credit_card, whether a string of digits
// passes the Luhn checksum.
func RegisterIsCreditCard() gojq.CompilerOption {
	return registerBool("is_credit_card", func(s string) bool {
		digits := ""
		for _, r := range s {
			if r >= '0' && r <= '9' {
				digits += string(r)
			}
		}
		if len(digits) < 13 || len(digits) > 19 {
			return false
		}
		return luhn(digits)
	})
}

func luhn(digits string) bool {
	sum := 0
	double := false
	for i := len(digits) - 1; i >= 0; i-- {
		d := int(digits[i] - '0')
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return sum%10 == 0
}
