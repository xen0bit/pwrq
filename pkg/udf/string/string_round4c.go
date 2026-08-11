package string

import (
	"fmt"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

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

// RegisterPrefixLines registers prefix_lines, every line prefixed: prefix_lines
// ("a\nb"; "> ") -> "> a\n> b".
func RegisterPrefixLines() gojq.CompilerOption {
	return gojq.WithFunction("prefix_lines", 1, 1, func(v any, args []any) any {
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

// RegisterFirstLines registers first_lines, the first n lines of a string.
func RegisterFirstLines() gojq.CompilerOption {
	return gojq.WithFunction("first_lines", 1, 1, func(v any, args []any) any {
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
	return gojq.WithFunction("last_lines", 1, 1, func(v any, args []any) any {
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
