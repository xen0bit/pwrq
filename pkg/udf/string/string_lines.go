// Line-oriented operations: counting, slicing, reordering and reflowing.
package string

import (
	"fmt"
	"sort"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// linesOf splits a string into lines, a lone trailing newline not adding an
// empty final line.
func linesOf(s string) []string {
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
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

// RegisterFirstLines registers first_lines, the first n lines of a string.
func RegisterFirstLines() gojq.CompilerOption {
	return common.WithFunction("first_lines", 1, 1, func(v any, args []any) any {
		n, ok := common.ToInt(args[0])
		if !ok || n < 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("first_lines: count must be a non-negative integer, got %v", args[0]), nil)
		}
		input, err := strFromPipeline(v, "first_lines")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		lines := linesOf(input)
		if n > len(lines) {
			n = len(lines)
		}
		return common.MakeUDFSuccessResult(strings.Join(lines[:n], "\n"), nil)
	})
}

// RegisterLastLines registers last_lines, the last n lines of a string.
func RegisterLastLines() gojq.CompilerOption {
	return common.WithFunction("last_lines", 1, 1, func(v any, args []any) any {
		n, ok := common.ToInt(args[0])
		if !ok || n < 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("last_lines: count must be a non-negative integer, got %v", args[0]), nil)
		}
		input, err := strFromPipeline(v, "last_lines")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		lines := linesOf(input)
		if n > len(lines) {
			n = len(lines)
		}
		return common.MakeUDFSuccessResult(strings.Join(lines[len(lines)-n:], "\n"), nil)
	})
}

// RegisterPrefixLines registers prefix_lines, every line prefixed: prefix_lines
// ("a\nb"; "> ") -> "> a\n> b".
func RegisterPrefixLines() gojq.CompilerOption {
	return common.WithFunction("prefix_lines", 1, 1, func(v any, args []any) any {
		prefix, ok := common.BindValue(args[0]).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("prefix_lines: prefix must be a string, got %T", args[0]), nil)
		}
		input, err := strFromPipeline(v, "prefix_lines")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		if input == "" {
			return common.MakeUDFSuccessResult("", nil)
		}
		lines := strings.Split(input, "\n")
		for i := range lines {
			lines[i] = prefix + lines[i]
		}
		return common.MakeUDFSuccessResult(strings.Join(lines, "\n"), nil)
	})
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

// RegisterIndent registers indent, prefixing every line with n spaces.
func RegisterIndent() gojq.CompilerOption {
	return common.WithFunction("indent", 1, 2, func(v any, args []any) any {
		n, ok := common.ToInt(args[0])
		if !ok || n < 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("indent: width must be a non-negative integer, got %v", args[0]), nil)
		}
		isFile := false
		if len(args) > 1 {
			if b, ok := args[1].(bool); ok {
				isFile = b
			}
		}
		in, err := bindInput(v, isFile)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("indent: %v", err), nil)
		}
		input, err := stringInput(in)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("indent: %v", err), nil)
		}
		pad := strings.Repeat(" ", n)
		lines := strings.Split(input, "\n")
		for i, l := range lines {
			if l != "" {
				lines[i] = pad + l
			}
		}
		return common.MakeUDFSuccessResult(strings.Join(lines, "\n"), nil)
	})
}

// RegisterWrapText registers wrap_text, greedy word-wrapping a string to a
// width, returned as an array of lines.
func RegisterWrapText() gojq.CompilerOption {
	return common.WithFunction("wrap_text", 1, 2, func(v any, args []any) any {
		width, ok := common.ToInt(args[0])
		if !ok || width <= 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("wrap_text: width must be a positive integer, got %v", args[0]), nil)
		}
		isFile := false
		if len(args) > 1 {
			if b, ok := args[1].(bool); ok {
				isFile = b
			}
		}
		in, err := bindInput(v, isFile)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("wrap_text: %v", err), nil)
		}
		input, err := stringInput(in)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("wrap_text: %v", err), nil)
		}
		lines := wrapText(input, width)
		out := make([]any, len(lines))
		for i, l := range lines {
			out[i] = l
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

func wrapText(s string, width int) []string {
	words := strings.Fields(s)
	var lines []string
	var current strings.Builder
	for _, w := range words {
		if current.Len() > 0 && current.Len()+1+len(w) > width {
			lines = append(lines, current.String())
			current.Reset()
		}
		if current.Len() > 0 {
			current.WriteByte(' ')
		}
		current.WriteString(w)
	}
	if current.Len() > 0 {
		lines = append(lines, current.String())
	}
	return lines
}

// RegisterDiffLines registers diff_lines, which lines are in only one of two
// texts: {added, removed}. The sets respect multiplicity.
func RegisterDiffLines() gojq.CompilerOption {
	return common.WithFunction("diff_lines", 1, 2, func(v any, args []any) any {
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
