package string

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

var (
	alphanumericPattern = regexp.MustCompile(`^[A-Za-z0-9]+$`)
	alphabeticPattern   = regexp.MustCompile(`^[A-Za-z]+$`)
	digitPattern        = regexp.MustCompile(`^[0-9]+$`)
)

// registerPredicate registers a 0-2 arity string-in, boolean-out cmdlet.
func registerPredicate(name string, fn func(string) bool) gojq.CompilerOption {
	return gojq.WithFunction(name, 0, 2, func(v any, args []any) any {
		inputVal, _, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: %v", name, err), nil)
		}
		input, err := stringInput(common.BindValue(inputVal))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: %v", name, err), nil)
		}
		return common.MakeUDFSuccessResult(fn(input), nil)
	})
}

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

// RegisterWordCount registers word_count, how many whitespace-separated words a
// string has.
func RegisterWordCount() gojq.CompilerOption {
	return gojq.WithFunction("word_count", 0, 2, func(v any, args []any) any {
		inputVal, _, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("word_count: %v", err), nil)
		}
		input, err := stringInput(common.BindValue(inputVal))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("word_count: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(len(strings.Fields(input)), nil)
	})
}

// RegisterNormalizeWhitespace registers normalize_whitespace, collapsing every
// run of whitespace to a single space and trimming the ends.
func RegisterNormalizeWhitespace() gojq.CompilerOption {
	return registerCaseConverter("normalize_whitespace", func(s string) string {
		return strings.Join(strings.Fields(s), " ")
	})
}

// RegisterAcronym registers acronym, the uppercase initials of the words in a
// string.
func RegisterAcronym() gojq.CompilerOption {
	return registerCaseConverter("acronym", func(s string) string {
		words := splitWords(s)
		var b strings.Builder
		for _, w := range words {
			if w != "" {
				b.WriteRune(unicode.ToUpper(rune(w[0])))
			}
		}
		return b.String()
	})
}

// RegisterEscapeRegex registers escape_regex, a string with every regex
// metacharacter quoted, so it can be used literally inside a pattern.
func RegisterEscapeRegex() gojq.CompilerOption {
	return registerCaseConverter("escape_regex", regexp.QuoteMeta)
}

// RegisterIsRegexValid registers is_regex_valid, whether a string compiles as a
// regular expression.
func RegisterIsRegexValid() gojq.CompilerOption {
	return registerPredicate("is_regex_valid", func(s string) bool {
		_, err := regexp.Compile(s)
		return err == nil
	})
}

// globToRegex translates a shell-style glob into an anchored regular
// expression. * becomes .*, ? becomes ., and character classes pass through.
func globToRegex(glob string) string {
	var b strings.Builder
	b.WriteByte('^')
	for i := 0; i < len(glob); i++ {
		c := glob[i]
		switch c {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteByte('.')
		case '[':
			// Copy the character class through, handling a leading ! and a
			// trailing ].
			j := i + 1
			if j < len(glob) && glob[j] == '!' {
				b.WriteString("[^")
				j++
			} else {
				b.WriteByte('[')
			}
			if j < len(glob) && glob[j] == ']' {
				b.WriteString(`\]`)
				j++
			}
			for j < len(glob) && glob[j] != ']' {
				if glob[j] == '\\' {
					b.WriteString(`\\`)
				} else {
					b.WriteByte(glob[j])
				}
				j++
			}
			b.WriteByte(']')
			i = j
		case '\\':
			if i+1 < len(glob) {
				b.WriteString(regexp.QuoteMeta(string(glob[i+1])))
				i++
			} else {
				b.WriteString(`\\`)
			}
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteByte('$')
	return b.String()
}

// RegisterGlobToRegex registers glob_to_regex, turning a glob like "*.txt"
// into the anchored regular expression that matches it.
func RegisterGlobToRegex() gojq.CompilerOption {
	return registerCaseConverter("glob_to_regex", globToRegex)
}

// RegisterMatchGlob registers match_glob, whether a string matches a glob
// pattern.
func RegisterMatchGlob() gojq.CompilerOption {
	return gojq.WithFunction("match_glob", 1, 2, func(v any, args []any) any {
		glob, ok := common.BindValue(args[0]).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("match_glob: pattern must be a string, got %T", args[0]), nil)
		}
		isFile := false
		if len(args) > 1 {
			if b, ok := args[1].(bool); ok {
				isFile = b
			}
		}
		in, err := bindInput(v, isFile)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("match_glob: %v", err), nil)
		}
		input, err := stringInput(in)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("match_glob: %v", err), nil)
		}
		re, err := regexp.Compile(globToRegex(glob))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("match_glob: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(re.MatchString(input), nil)
	})
}
