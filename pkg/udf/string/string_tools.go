package string

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// ANSI escapes, matched so strip_ansi can clear terminal colour from logs.
var (
	ansiCSI = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
	ansiOSC = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)
)

// RegisterStripANSI registers strip_ansi, removing ANSI terminal escape
// sequences from a string.
func RegisterStripANSI() gojq.CompilerOption {
	return registerCaseConverter("strip_ansi", func(s string) string {
		s = ansiOSC.ReplaceAllString(s, "")
		s = ansiCSI.ReplaceAllString(s, "")
		return s
	})
}

// RegisterTemplate registers template, replacing {{key}} placeholders in a
// string with values from an object.
func RegisterTemplate() gojq.CompilerOption {
	return gojq.WithFunction("template", 1, 1, func(v any, args []any) any {
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

// RegisterWrapText registers wrap_text, greedy word-wrapping a string to a
// width, returned as an array of lines.
func RegisterWrapText() gojq.CompilerOption {
	return gojq.WithFunction("wrap_text", 1, 2, func(v any, args []any) any {
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

// RegisterIndent registers indent, prefixing every line with n spaces.
func RegisterIndent() gojq.CompilerOption {
	return gojq.WithFunction("indent", 1, 2, func(v any, args []any) any {
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

// RegisterPluralize registers pluralize, an integer with a pluralized noun:
// 2 | pluralize("item") is "2 items".
func RegisterPluralize() gojq.CompilerOption {
	return gojq.WithFunction("pluralize", 1, 2, func(v any, args []any) any {
		n, ok := common.ToInt(v)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("pluralize: expected a number, got %T", v), nil)
		}
		singular, ok := common.BindValue(args[0]).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("pluralize: noun must be a string, got %T", args[0]), nil)
		}
		if n == 1 {
			return common.MakeUDFSuccessResult(fmt.Sprintf("1 %s", singular), nil)
		}
		plural := singular + "s"
		if len(args) > 1 {
			if p, ok := common.BindValue(args[1]).(string); ok && p != "" {
				plural = p
			}
		}
		return common.MakeUDFSuccessResult(fmt.Sprintf("%d %s", n, plural), nil)
	})
}
