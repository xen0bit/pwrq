package validate

import (
	"strconv"
	"strings"

	"github.com/itchyny/gojq"
)

// RegisterIsNumeric registers is_numeric, whether a string parses as an
// integer or floating-point number.
func RegisterIsNumeric() gojq.CompilerOption {
	return registerBool("is_numeric", func(s string) bool {
		s = strings.TrimSpace(s)
		if s == "" {
			return false
		}
		_, err := strconv.ParseFloat(s, 64)
		return err == nil
	})
}
