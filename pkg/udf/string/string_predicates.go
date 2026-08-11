// Predicates over a string s shape.
package string

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/itchyny/gojq"
)

var (
	alphanumericPattern = regexp.MustCompile(`^[A-Za-z0-9]+$`)
	alphabeticPattern   = regexp.MustCompile(`^[A-Za-z]+$`)
	digitPattern        = regexp.MustCompile(`^[0-9]+$`)
)

// RegisterIsBlank registers is_blank, whether a string is empty or whitespace.
func RegisterIsBlank() gojq.CompilerOption {
	return registerPredicate("is_blank", func(s string) bool {
		return strings.TrimSpace(s) == ""
	})
}

// RegisterIsAlphanumeric registers is_alphanumeric, whether every character is
// a letter or a digit.
func RegisterIsAlphanumeric() gojq.CompilerOption {
	return registerPredicate("is_alphanumeric", func(s string) bool {
		return alphanumericPattern.MatchString(s)
	})
}

// RegisterIsAlphabetic registers is_alphabetic, whether every character is a
// letter.
func RegisterIsAlphabetic() gojq.CompilerOption {
	return registerPredicate("is_alphabetic", func(s string) bool {
		return alphabeticPattern.MatchString(s)
	})
}

// RegisterIsNumericString registers is_numeric_string, whether every character
// is a digit.
func RegisterIsNumericString() gojq.CompilerOption {
	return registerPredicate("is_numeric_string", func(s string) bool {
		return digitPattern.MatchString(s)
	})
}

// RegisterIsUppercase registers is_uppercase, whether a string contains a
// letter and every letter is uppercase.
func RegisterIsUppercase() gojq.CompilerOption {
	return registerPredicate("is_uppercase", func(s string) bool {
		return hasLetter(s) && s == strings.ToUpper(s)
	})
}

// RegisterIsLowercase registers is_lowercase, whether a string contains a
// letter and every letter is lowercase.
func RegisterIsLowercase() gojq.CompilerOption {
	return registerPredicate("is_lowercase", func(s string) bool {
		return hasLetter(s) && s == strings.ToLower(s)
	})
}

func hasLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// RegisterIsAscii registers is_ascii, whether every byte is in the ASCII range.
func RegisterIsAscii() gojq.CompilerOption {
	return registerPredicate("is_ascii", func(s string) bool {
		for i := 0; i < len(s); i++ {
			if s[i] >= 0x80 {
				return false
			}
		}
		return true
	})
}

// RegisterIsBalanced registers is_balanced, whether (), [] and {} nest without
// mismatching.
func RegisterIsBalanced() gojq.CompilerOption {
	return registerTextFn("is_balanced", func(s string) any {
		stack := []byte{}
		pairs := map[byte]byte{')': '(', ']': '[', '}': '{'}
		for i := 0; i < len(s); i++ {
			c := s[i]
			switch c {
			case '(', '[', '{':
				stack = append(stack, c)
			case ')', ']', '}':
				if len(stack) == 0 || stack[len(stack)-1] != pairs[c] {
					return false
				}
				stack = stack[:len(stack)-1]
			}
		}
		return len(stack) == 0
	})
}
