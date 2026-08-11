package string

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterBeforeFirst registers before_first, the part of a string before the
// first occurrence of a separator (the whole string when there is none).
func RegisterBeforeFirst() gojq.CompilerOption {
	return gojq.WithFunction("before_first", 1, 1, func(v any, args []any) any {
		sep, ok := common.BindValue(args[0]).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("before_first: separator must be a string, got %T", args[0]), nil)
		}
		input, err := strFromPipeline(v, "before_first")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		if i := strings.Index(input, sep); i >= 0 {
			return common.MakeUDFSuccessResult(input[:i], nil)
		}
		return common.MakeUDFSuccessResult(input, nil)
	})
}

// RegisterAfterFirst registers after_first, the part of a string after the
// first occurrence of a separator (the empty string when there is none).
func RegisterAfterFirst() gojq.CompilerOption {
	return gojq.WithFunction("after_first", 1, 1, func(v any, args []any) any {
		sep, ok := common.BindValue(args[0]).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("after_first: separator must be a string, got %T", args[0]), nil)
		}
		input, err := strFromPipeline(v, "after_first")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		if i := strings.Index(input, sep); i >= 0 {
			return common.MakeUDFSuccessResult(input[i+len(sep):], nil)
		}
		return common.MakeUDFSuccessResult("", nil)
	})
}

// RegisterSurround registers surround, a string wrapped in a prefix and suffix:
// surround("x"; "[ "; " ]") -> "[ x ]".
func RegisterSurround() gojq.CompilerOption {
	return gojq.WithFunction("surround", 2, 2, func(v any, args []any) any {
		prefix, okP := common.BindValue(args[0]).(string)
		suffix, okS := common.BindValue(args[1]).(string)
		if !okP || !okS {
			return common.MakeUDFErrorResult(fmt.Errorf("surround: prefix and suffix must be strings"), nil)
		}
		input, err := strFromPipeline(v, "surround")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		return common.MakeUDFSuccessResult(prefix+input+suffix, nil)
	})
}

// RegisterSoundex registers soundex, the four-character Soundex code of a
// word, for matching names that sound alike.
func RegisterSoundex() gojq.CompilerOption {
	return registerTextFn("soundex", func(s string) any {
		return soundex(s)
	})
}

func soundex(s string) string {
	// Map letters to Soundex groups; 0 marks a dropped letter.
	code := map[rune]byte{
		'b': '1', 'f': '1', 'p': '1', 'v': '1',
		'c': '2', 'g': '2', 'j': '2', 'k': '2', 'q': '2', 's': '2', 'x': '2', 'z': '2',
		'd': '3', 't': '3',
		'l': '4',
		'm': '5', 'n': '5',
		'r': '6',
		'a': 0, 'e': 0, 'i': 0, 'o': 0, 'u': 0, 'y': 0, 'h': 0, 'w': 0,
	}
	var out []byte
	prev := byte(0)
	for _, r := range strings.ToLower(s) {
		c, ok := code[r]
		if !ok {
			continue // non-letter
		}
		if len(out) == 0 {
			out = append(out, byte(unicode.ToUpper(r)))
			prev = c
			continue
		}
		if c != 0 && c != prev {
			out = append(out, c)
			if len(out) == 4 {
				break
			}
		}
		prev = c
	}
	for len(out) < 4 {
		out = append(out, '0')
	}
	return string(out[:4])
}

// RegisterCountVowels registers count_vowels, how many vowel letters a string
// has.
func RegisterCountVowels() gojq.CompilerOption {
	return registerTextFn("count_vowels", func(s string) any {
		n := 0
		for _, r := range strings.ToLower(s) {
			switch r {
			case 'a', 'e', 'i', 'o', 'u':
				n++
			}
		}
		return n
	})
}

// RegisterCountConsonants registers count_consonants, how many consonant
// letters a string has.
func RegisterCountConsonants() gojq.CompilerOption {
	return registerTextFn("count_consonants", func(s string) any {
		n := 0
		for _, r := range strings.ToLower(s) {
			if !unicode.IsLetter(r) {
				continue
			}
			switch r {
			case 'a', 'e', 'i', 'o', 'u':
			default:
				n++
			}
		}
		return n
	})
}

// RegisterCapitalizeFirst registers capitalize_first, the first letter
// uppercased with the rest left untouched.
func RegisterCapitalizeFirst() gojq.CompilerOption {
	return registerTextFn("capitalize_first", func(s string) any {
		for i, r := range s {
			if unicode.IsLetter(r) {
				runes := []rune(s)
				runes[i] = unicode.ToUpper(r)
				return string(runes)
			}
		}
		return s
	})
}

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

// RegisterDiffLines registers diff_lines, which lines are in only one of two
// texts: {added, removed}. The sets respect multiplicity.
func RegisterDiffLines() gojq.CompilerOption {
	return gojq.WithFunction("diff_lines", 1, 2, func(v any, args []any) any {
		other, ok := common.BindValue(args[len(args)-1]).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("diff_lines: the other text must be a string, got %T", args[len(args)-1]), nil)
		}
		a, err := strFromPipeline(v, "diff_lines")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		return common.MakeUDFSuccessResult(lineDiff(a, other), nil)
	})
}

func lineDiff(a, b string) map[string]any {
	counts := func(s string) map[string]int {
		m := make(map[string]int)
		if s == "" {
			return m
		}
		for _, line := range strings.Split(s, "\n") {
			m[line]++
		}
		return m
	}
	ac := counts(a)
	bc := counts(b)
	var added, removed []any
	for _, line := range strings.Split(b, "\n") {
		if line == "" && b == "" {
			continue
		}
		if n := bc[line]; n > ac[line] {
			added = append(added, line)
			bc[line]--
		}
	}
	for _, line := range strings.Split(a, "\n") {
		if line == "" && a == "" {
			continue
		}
		if n := ac[line]; n > bc[line] {
			removed = append(removed, line)
			ac[line]--
		}
	}
	return map[string]any{"added": added, "removed": removed}
}
