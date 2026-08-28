// Words and characters: counting, padding, masking and templating.
package string

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
	"golang.org/x/text/unicode/norm"
)

// RegisterReverseWords registers reverse_words, the words of a string in
// reverse order: "the quick brown" -> "brown quick the".
func RegisterReverseWords() gojq.CompilerOption {
	return registerTextFn("reverse_words", func(s string) any {
		words := strings.Fields(s)
		for i, j := 0, len(words)-1; i < j; i, j = i+1, j-1 {
			words[i], words[j] = words[j], words[i]
		}
		return strings.Join(words, " ")
	})
}

// RegisterTruncateWords registers truncate_words, a string cut to at most n
// words with an ellipsis when it had to give anything up.
func RegisterTruncateWords() gojq.CompilerOption {
	return common.WithFunction("truncate_words", 1, 2, func(v any, args []any) any {
		n, ok := common.ToInt(args[0])
		if !ok || n < 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("truncate_words: count must be a non-negative integer, got %v", args[0]), nil)
		}
		input, err := strFromPipeline(v, "truncate_words")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		words := strings.Fields(input)
		if len(words) <= n {
			return common.MakeUDFSuccessResult(input, nil)
		}
		return common.MakeUDFSuccessResult(strings.Join(words[:n], " ")+"…", nil)
	})
}

// RegisterTruncate registers truncate, cutting a string to a length and
// appending a suffix (an ellipsis by default).
func RegisterTruncate() gojq.CompilerOption {
	return common.WithFunction("truncate", 1, 3, func(v any, args []any) any {
		maxLen, ok := common.ToInt(args[0])
		if !ok || maxLen < 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("truncate: length must be a non-negative number, got %v", args[0]), nil)
		}
		suffix := "…"
		isFile := false
		if len(args) > 1 {
			switch s := common.BindValue(args[1]).(type) {
			case bool:
				isFile = s
			case string:
				suffix = s
			default:
				return common.MakeUDFErrorResult(fmt.Errorf("truncate: suffix must be a string, got %T", args[1]), nil)
			}
		}
		if len(args) > 2 {
			if b, ok := args[2].(bool); ok {
				isFile = b
			}
		}
		in, err := bindInput(v, isFile)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("truncate: %v", err), nil)
		}
		input, err := stringInput(in)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("truncate: %v", err), nil)
		}
		runes := []rune(input)
		if len(runes) <= maxLen {
			return common.MakeUDFSuccessResult(input, nil)
		}
		return common.MakeUDFSuccessResult(string(runes[:maxLen])+suffix, nil)
	})
}

// RegisterRemoveAccents registers remove_accents, diacritics stripped from a
// string: "café" -> "cafe".
func RegisterRemoveAccents() gojq.CompilerOption {
	return registerTextFn("remove_accents", func(s string) any {
		var b strings.Builder
		for _, r := range norm.NFD.String(s) {
			if unicode.Is(unicode.Mn, r) {
				continue
			}
			b.WriteRune(r)
		}
		return b.String()
	})
}

// RegisterCharFrequencies registers char_frequencies, how often each character
// appears in a string, as {char: count}.
func RegisterCharFrequencies() gojq.CompilerOption {
	return registerTextFnOf("char_frequencies", CharFrequencies, func(s string) any {
		counts := make(map[string]any)
		order := []string{}
		for _, r := range s {
			key := string(r)
			if _, seen := counts[key]; !seen {
				order = append(order, key)
			}
			if n, ok := counts[key].(int); ok {
				counts[key] = n + 1
			} else {
				counts[key] = 1
			}
		}
		out := make(map[string]any, len(order))
		for _, key := range order {
			out[key] = counts[key]
		}
		return out
	})
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

// RegisterCountOccurrences registers count_occurrences, counting non-overlapping
// occurrences of a substring.
func RegisterCountOccurrences() gojq.CompilerOption {
	return common.WithFunction("count_occurrences", 1, 2, func(v any, args []any) any {
		sub, ok := common.BindValue(args[0]).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("count_occurrences: substring must be a string, got %T", args[0]), nil)
		}
		isFile := false
		if len(args) > 1 {
			if b, ok := args[1].(bool); ok {
				isFile = b
			}
		}
		in, err := bindInput(v, isFile)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("count_occurrences: %v", err), nil)
		}
		input, err := stringInput(in)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("count_occurrences: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(strings.Count(input, sub), nil)
	})
}

// RegisterWordCount registers word_count, how many whitespace-separated words a
// string has.
func RegisterWordCount() gojq.CompilerOption {
	return common.WithFunction("word_count", 0, 2, func(v any, args []any) any {
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

// RegisterPadLeft registers pad_left, left-padding a string to a width with a
// repeated padding character.
func RegisterPadLeft() gojq.CompilerOption {
	return common.WithFunction("pad_left", 1, 3, func(v any, args []any) any {
		width, ok := common.ToInt(args[0])
		if !ok || width < 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("pad_left: width must be a non-negative number, got %v", args[0]), nil)
		}
		pad, isFile, err := padArgs(args, 1)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("pad_left: %v", err), nil)
		}
		in, err := bindInput(v, isFile)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("pad_left: %v", err), nil)
		}
		input, err := stringInput(in)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("pad_left: %v", err), nil)
		}
		need := width - len([]rune(input))
		if need <= 0 {
			return common.MakeUDFSuccessResult(input, nil)
		}
		return common.MakeUDFSuccessResult(strings.Repeat(pad, need)+input, nil)
	})
}

// RegisterPadRight registers pad_right, right-padding a string to a width.
func RegisterPadRight() gojq.CompilerOption {
	return common.WithFunction("pad_right", 1, 3, func(v any, args []any) any {
		width, ok := common.ToInt(args[0])
		if !ok || width < 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("pad_right: width must be a non-negative number, got %v", args[0]), nil)
		}
		pad, isFile, err := padArgs(args, 1)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("pad_right: %v", err), nil)
		}
		in, err := bindInput(v, isFile)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("pad_right: %v", err), nil)
		}
		input, err := stringInput(in)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("pad_right: %v", err), nil)
		}
		need := width - len([]rune(input))
		if need <= 0 {
			return common.MakeUDFSuccessResult(input, nil)
		}
		return common.MakeUDFSuccessResult(input+strings.Repeat(pad, need), nil)
	})
}

// RegisterPadCenter registers pad_center, a string centered in a field of n
// characters, padded with a repeated character (default space).
func RegisterPadCenter() gojq.CompilerOption {
	return common.WithFunction("pad_center", 1, 2, func(v any, args []any) any {
		width, ok := common.ToInt(args[0])
		if !ok || width < 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("pad_center: width must be a non-negative integer, got %v", args[0]), nil)
		}
		pad := " "
		if len(args) > 1 {
			if p, ok := common.BindValue(args[1]).(string); ok && p != "" {
				pad = p
			}
		}
		input, err := strFromPipeline(v, "pad_center")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		size := utf8.RuneCountInString(input)
		if size >= width {
			return common.MakeUDFSuccessResult(input, nil)
		}
		total := width - size
		left := total / 2
		right := total - left
		leftPad, rightPad := "", ""
		for i := 0; i < left; i++ {
			leftPad += pad
		}
		for i := 0; i < right; i++ {
			rightPad += pad
		}
		return common.MakeUDFSuccessResult(leftPad+input+rightPad, nil)
	})
}

// padArgs reads the optional padding string (and trailing file flag) that
// starts at args[start].
func padArgs(args []any, start int) (string, bool, error) {
	pad := " "
	isFile := false
	if len(args) > start {
		switch p := common.BindValue(args[start]).(type) {
		case bool:
			isFile = p
		case string:
			if p == "" {
				pad = " "
			} else {
				pad = string([]rune(p)[0])
			}
		default:
			return "", false, fmt.Errorf("padding must be a string, got %T", args[start])
		}
	}
	if len(args) > start+1 {
		if b, ok := args[start+1].(bool); ok {
			isFile = b
		}
	}
	return pad, isFile, nil
}

// RegisterMask registers mask, hiding the middle of a string and keeping the
// first and last visible characters, so credentials keep their shape.
func RegisterMask() gojq.CompilerOption {
	return common.WithFunction("mask", 0, 2, func(v any, args []any) any {
		visible := 0
		isFile := false
		if len(args) > 0 {
			if b, ok := args[0].(bool); ok {
				isFile = b
			} else if n, ok := common.ToInt(args[0]); ok {
				visible = n
			}
		}
		if len(args) > 1 {
			if b, ok := args[1].(bool); ok {
				isFile = b
			}
		}
		if visible < 0 {
			visible = 0
		}
		in, err := bindInput(v, isFile)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("mask: %v", err), nil)
		}
		input, err := stringInput(in)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("mask: %v", err), nil)
		}
		runes := []rune(input)
		n := len(runes)
		if n <= visible*2 {
			return common.MakeUDFSuccessResult(input, nil)
		}
		var b strings.Builder
		b.Grow(n)
		for i, r := range runes {
			if i < visible || i >= n-visible {
				b.WriteRune(r)
			} else {
				b.WriteRune('*')
			}
		}
		return common.MakeUDFSuccessResult(b.String(), nil)
	})
}

// RegisterTemplate registers template, replacing {{key}} placeholders in a
// string with values from an object.
func RegisterTemplate() gojq.CompilerOption {
	return common.WithFunction("template", 1, 1, func(v any, args []any) any {
		vars, ok := common.BindValue(args[0]).(map[string]any)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("template: variables must be an object, got %T", args[0]), nil)
		}
		in, err := bindInput(v, false)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("template: %v", err), nil)
		}
		input, err := stringInput(in)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("template: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(applyTemplate(input, vars), nil)
	})
}

var placeholder = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_.]+)\s*\}\}`)

func applyTemplate(s string, vars map[string]any) string {
	return placeholder.ReplaceAllStringFunc(s, func(match string) string {
		groups := placeholder.FindStringSubmatch(match)
		key := groups[1]
		if value, ok := vars[key]; ok {
			if str, isString := value.(string); isString {
				return str
			}
			encoded, err := json.Marshal(value)
			if err == nil {
				return string(encoded)
			}
			return fmt.Sprint(value)
		}
		return match
	})
}

// RegisterNormalizeWhitespace registers normalize_whitespace, collapsing every
// run of whitespace to a single space and trimming the ends.
func RegisterNormalizeWhitespace() gojq.CompilerOption {
	return registerCaseConverter("normalize_whitespace", func(s string) string {
		return strings.Join(strings.Fields(s), " ")
	})
}
