// Escaping, unescaping, and turning globs into regular expressions.
package string

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterUnicodeEscape registers unicode_escape, every non-ASCII and control
// character rendered as \uXXXX, so any text is ASCII-only.
func RegisterUnicodeEscape() gojq.CompilerOption {
	return registerTextFn("unicode_escape", func(s string) any {
		var b strings.Builder
		for _, r := range s {
			if r < 0x20 || r == 0x7f || r > 0x7e {
				fmt.Fprintf(&b, "\\u%04x", r)
				continue
			}
			b.WriteRune(r)
		}
		return b.String()
	})
}

// RegisterUnicodeUnescape registers unicode_unescape, \uXXXX escapes rendered
// back into their characters.
func RegisterUnicodeUnescape() gojq.CompilerOption {
	return registerTextFn("unicode_unescape", func(s string) any {
		re := regexp.MustCompile(`\\u([0-9a-fA-F]{4})`)
		return re.ReplaceAllStringFunc(s, func(m string) string {
			hex := m[2:]
			n, err := strconv.ParseUint(hex, 16, 32)
			if err != nil {
				return m
			}
			return string(rune(n))
		})
	})
}

// RegisterQuotedPrintableEncode registers quoted_printable_encode, text as MIME
// quoted-printable, the encoding email bodies travel in. It operates on UTF-8
// bytes, so é encodes as =C3=A9.
func RegisterQuotedPrintableEncode() gojq.CompilerOption {
	return registerTextFn("quoted_printable_encode", func(s string) any {
		var b strings.Builder
		lineLen := 0
		for i := 0; i < len(s); i++ {
			c := s[i]
			// A hard break keeps lines at most 76 characters.
			if lineLen+1 > 75 {
				b.WriteString("=\n")
				lineLen = 0
			}
			switch {
			case c == '=':
				b.WriteString("=3D")
				lineLen += 3
			case c == '\n' || c == '\r':
				// A literal newline stays raw as a soft break.
				b.WriteByte(c)
				lineLen = 0
			case (c >= 33 && c <= 126) || c == 9 || c == 32:
				b.WriteByte(c)
				lineLen++
			default:
				fmt.Fprintf(&b, "=%02X", c)
				lineLen += 3
			}
		}
		return b.String()
	})
}

// RegisterQuotedPrintableDecode registers quoted_printable_decode, the inverse
// of quoted_printable_encode.
func RegisterQuotedPrintableDecode() gojq.CompilerOption {
	return registerTextFn("quoted_printable_decode", func(s string) any {
		var b strings.Builder
		for i := 0; i < len(s); i++ {
			switch {
			case s[i] == '=' && i+1 < len(s) && s[i+1] == '\n':
				// Soft line break: drop the = and the newline.
				i++
			case s[i] == '=' && i+2 < len(s):
				var n int
				if _, err := fmt.Sscanf(s[i+1:i+3], "%02x", &n); err == nil {
					b.WriteByte(byte(n))
					i += 2
				} else {
					b.WriteByte(s[i])
				}
			default:
				b.WriteByte(s[i])
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

// RegisterIsRegexValid registers is_regex_valid, whether a string compiles as a
// regular expression.
func RegisterIsRegexValid() gojq.CompilerOption {
	return registerPredicate("is_regex_valid", func(s string) bool {
		_, err := regexp.Compile(s)
		return err == nil
	})
}
