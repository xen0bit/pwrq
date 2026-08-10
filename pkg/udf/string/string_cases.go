package string

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// The word-splitting rule shared by the case converters. A word boundary is any
// run of non-alphanumerics, or a camelCase transition (lowercase followed by
// uppercase), so "helloWorld foo_bar" splits into hello, world, foo, bar.
var (
	camelBoundary = regexp.MustCompile(`([a-z0-9])([A-Z])`)
	wordBoundary  = regexp.MustCompile(`[^A-Za-z0-9]+`)
)

func splitWords(s string) []string {
	s = camelBoundary.ReplaceAllString(s, `${1} ${2}`)
	parts := wordBoundary.Split(s, -1)
	words := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			words = append(words, strings.ToLower(p))
		}
	}
	return words
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	return strings.ToUpper(string(runes[:1])) + string(runes[1:])
}

// stringInput coerces the bound pipeline value to a string, the way the string
// cmdlets' switch does everywhere else in this package.
func stringInput(inputVal any) (string, error) {
	switch val := inputVal.(type) {
	case string:
		return val, nil
	case []byte:
		return string(val), nil
	default:
		if str, ok := val.(fmt.Stringer); ok {
			return str.String(), nil
		}
		return "", fmt.Errorf("argument must be a string, got %T", val)
	}
}

// bindInput resolves the pipeline value, honouring the file flag the way
// ParseFileArgs does for the single-input cmdlets.
func bindInput(v any, isFile bool) (any, error) {
	if !isFile {
		return common.BindValue(v), nil
	}
	path, ok := common.BindPath(v)
	if !ok {
		return nil, fmt.Errorf("file argument requires a string path, got %T", v)
	}
	data, _, _, err := common.ReadFileFromPath(path)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

// registerCaseConverter builds a 0-2 arity string-in, string-out cmdlet that
// applies transform to the pipeline input, mirroring upper/lower.
func registerCaseConverter(name string, transform func(string) string) gojq.CompilerOption {
	return gojq.WithFunction(name, 0, 2, func(v any, args []any) any {
		inputVal, _, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: %v", name, err), nil)
		}
		input, err := stringInput(common.BindValue(inputVal))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: %v", name, err), nil)
		}
		return common.MakeUDFSuccessResult(transform(input), nil)
	})
}

// RegisterSlugify registers slugify, which turns any string into a
// lowercase-hyphenated identifier fit for URLs and file names.
func RegisterSlugify() gojq.CompilerOption {
	return registerCaseConverter("slugify", func(s string) string {
		return strings.Join(splitWords(s), "-")
	})
}

// RegisterSnakeCase registers snake_case, joining words with underscores.
func RegisterSnakeCase() gojq.CompilerOption {
	return registerCaseConverter("snake_case", func(s string) string {
		return strings.Join(splitWords(s), "_")
	})
}

// RegisterKebabCase registers kebab_case, joining words with hyphens.
func RegisterKebabCase() gojq.CompilerOption {
	return registerCaseConverter("kebab_case", func(s string) string {
		return strings.Join(splitWords(s), "-")
	})
}

// RegisterCamelCase registers camel_case, joining words with the first word
// lower and the rest capitalized.
func RegisterCamelCase() gojq.CompilerOption {
	return registerCaseConverter("camel_case", func(s string) string {
		words := splitWords(s)
		for i := 1; i < len(words); i++ {
			words[i] = capitalize(words[i])
		}
		return strings.Join(words, "")
	})
}

// RegisterPascalCase registers pascal_case, joining words each capitalized.
func RegisterPascalCase() gojq.CompilerOption {
	return registerCaseConverter("pascal_case", func(s string) string {
		words := splitWords(s)
		for i := range words {
			words[i] = capitalize(words[i])
		}
		return strings.Join(words, "")
	})
}

// RegisterTitleCase registers title_case, capitalizing the first letter of
// every word.
func RegisterTitleCase() gojq.CompilerOption {
	return registerCaseConverter("title_case", func(s string) string {
		words := splitWords(s)
		for i := range words {
			words[i] = capitalize(words[i])
		}
		return strings.Join(words, " ")
	})
}

// RegisterTruncate registers truncate, cutting a string to a length and
// appending a suffix (an ellipsis by default).
func RegisterTruncate() gojq.CompilerOption {
	return gojq.WithFunction("truncate", 1, 3, func(v any, args []any) any {
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

// RegisterPadLeft registers pad_left, left-padding a string to a width with a
// repeated padding character.
func RegisterPadLeft() gojq.CompilerOption {
	return gojq.WithFunction("pad_left", 1, 3, func(v any, args []any) any {
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
	return gojq.WithFunction("pad_right", 1, 3, func(v any, args []any) any {
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

// RegisterMask registers mask, hiding the middle of a string and keeping the
// first and last visible characters, so credentials keep their shape.
func RegisterMask() gojq.CompilerOption {
	return gojq.WithFunction("mask", 0, 2, func(v any, args []any) any {
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

// RegisterCountOccurrences registers count_occurrences, counting non-overlapping
// occurrences of a substring.
func RegisterCountOccurrences() gojq.CompilerOption {
	return gojq.WithFunction("count_occurrences", 1, 2, func(v any, args []any) any {
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
