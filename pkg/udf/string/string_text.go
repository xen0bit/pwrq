package string

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
	"golang.org/x/text/unicode/norm"
)

// registerTextFn registers a 0-2 arity string-in, value-out cmdlet.
func registerTextFn(name string, fn func(string) any) gojq.CompilerOption {
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
	return gojq.WithFunction("truncate_words", 1, 2, func(v any, args []any) any {
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

// RegisterSentenceCase registers sentence_case, first letter capitalized and
// the rest lowercased.
func RegisterSentenceCase() gojq.CompilerOption {
	return registerTextFn("sentence_case", func(s string) any {
		s = strings.ToLower(s)
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

// RegisterLineCount registers line_count, how many lines a string has (0 for
// an empty string).
func RegisterLineCount() gojq.CompilerOption {
	return registerTextFn("line_count", func(s string) any {
		if s == "" {
			return 0
		}
		return 1 + strings.Count(s, "\n")
	})
}

// RegisterDedent registers dedent, the common leading whitespace removed from
// every line.
func RegisterDedent() gojq.CompilerOption {
	return registerTextFn("dedent", func(s string) any {
		lines := strings.Split(s, "\n")
		indent := -1
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			width := len(line) - len(strings.TrimLeft(line, " \t"))
			if indent == -1 || width < indent {
				indent = width
			}
		}
		if indent <= 0 {
			return s
		}
		for i, line := range lines {
			if len(line) >= indent {
				lines[i] = line[indent:]
			}
		}
		return strings.Join(lines, "\n")
	})
}

// RegisterSwapCase registers swap_case, uppercase and lowercase letters
// exchanged.
func RegisterSwapCase() gojq.CompilerOption {
	return registerTextFn("swap_case", func(s string) any {
		var b strings.Builder
		for _, r := range s {
			switch {
			case unicode.IsUpper(r):
				b.WriteRune(unicode.ToLower(r))
			case unicode.IsLower(r):
				b.WriteRune(unicode.ToUpper(r))
			default:
				b.WriteRune(r)
			}
		}
		return b.String()
	})
}

// RegisterCharFrequencies registers char_frequencies, how often each character
// appears in a string, as {char: count}.
func RegisterCharFrequencies() gojq.CompilerOption {
	return registerTextFn("char_frequencies", func(s string) any {
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

func sortString(s string) string {
	runes := []rune(s)
	sort.Slice(runes, func(i, j int) bool { return runes[i] < runes[j] })
	return string(runes)
}

// linesOf splits a string into lines, a lone trailing newline not adding an
// empty final line.
func linesOf(s string) []string {
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// RegisterReverseLines registers reverse_lines, the lines of a string reversed.
func RegisterReverseLines() gojq.CompilerOption {
	return registerTextFn("reverse_lines", func(s string) any {
		if s == "" {
			return ""
		}
		lines := linesOf(s)
		for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
			lines[i], lines[j] = lines[j], lines[i]
		}
		return strings.Join(lines, "\n")
	})
}

// RegisterUniqueLines registers unique_lines, duplicate lines removed keeping
// first-occurrence order.
func RegisterUniqueLines() gojq.CompilerOption {
	return registerTextFn("unique_lines", func(s string) any {
		seen := make(map[string]bool)
		out := []string{}
		for _, line := range linesOf(s) {
			if !seen[line] {
				seen[line] = true
				out = append(out, line)
			}
		}
		return strings.Join(out, "\n")
	})
}

// RegisterSortLines registers sort_lines, the lines of a string sorted.
func RegisterSortLines() gojq.CompilerOption {
	return registerTextFn("sort_lines", func(s string) any {
		lines := linesOf(s)
		sort.Strings(lines)
		return strings.Join(lines, "\n")
	})
}

// RegisterStripQuotes registers strip_quotes, one layer of matching quotes
// ('…', "…", or `…`) removed from each end.
func RegisterStripQuotes() gojq.CompilerOption {
	return registerTextFn("strip_quotes", func(s string) any {
		s = strings.TrimSpace(s)
		if utf8.RuneCountInString(s) < 2 {
			return s
		}
		runes := []rune(s)
		first, last := runes[0], runes[len(runes)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') || (first == '`' && last == '`') {
			return string(runes[1 : len(runes)-1])
		}
		return s
	})
}

// RegisterPadCenter registers pad_center, a string centered in a field of n
// characters, padded with a repeated character (default space).
func RegisterPadCenter() gojq.CompilerOption {
	return gojq.WithFunction("pad_center", 1, 2, func(v any, args []any) any {
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
