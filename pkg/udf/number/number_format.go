// Rendering numbers for people: radices, byte sizes, currency and ordinals.
package number

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterToBase registers to_base, rendering a number in any base from 2 to 36.
func RegisterToBase() gojq.CompilerOption {
	return common.WithFunction("to_base", 1, 1, func(v any, args []any) any {
		base, ok := common.ToInt(args[0])
		if !ok || base < 2 || base > 36 {
			return common.MakeUDFErrorResult(fmt.Errorf("to_base: base must be 2-36, got %v", args[0]), nil)
		}
		n, err := num(v, "to_base")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		return common.MakeUDFSuccessResult(strconv.FormatInt(int64(n), base), nil)
	})
}

// RegisterFromBase registers from_base, parsing a number written in any base
// from 2 to 36.
func RegisterFromBase() gojq.CompilerOption {
	return common.WithFunction("from_base", 1, 1, func(v any, args []any) any {
		base, ok := common.ToInt(args[0])
		if !ok || base < 2 || base > 36 {
			return common.MakeUDFErrorResult(fmt.Errorf("from_base: base must be 2-36, got %v", args[0]), nil)
		}
		s, ok := common.BindValue(v).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("from_base: expected a string, got %T", v), nil)
		}
		n, err := strconv.ParseInt(strings.TrimSpace(s), base, 64)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("from_base: cannot parse %q in base %d: %v", s, base, err), nil)
		}
		return common.MakeUDFSuccessResult(n, nil)
	})
}

// RegisterToHexNumber registers to_hex_number, a number to a hex string (the
// byte-oriented hex_encode is the sibling for text).
func RegisterToHexNumber() gojq.CompilerOption {
	return common.WithFunction("to_hex_number", 0, 0, func(v any, args []any) any {
		n, err := num(v, "to_hex_number")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		return common.MakeUDFSuccessResult(strconv.FormatInt(int64(n), 16), nil)
	})
}

// RegisterFromHexNumber registers from_hex_number, a hex string to a number.
func RegisterFromHexNumber() gojq.CompilerOption {
	return common.WithFunction("from_hex_number", 0, 0, func(v any, args []any) any {
		s, ok := common.BindValue(v).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("from_hex_number: expected a string, got %T", v), nil)
		}
		n, err := strconv.ParseInt(strings.TrimPrefix(strings.TrimSpace(s), "0x"), 16, 64)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("from_hex_number: cannot parse %q as hex: %v", s, err), nil)
		}
		return common.MakeUDFSuccessResult(n, nil)
	})
}

// RegisterHumanBytes registers human_bytes, rendering a byte count in binary
// units (KiB, MiB, GiB, ...).
func RegisterHumanBytes() gojq.CompilerOption {
	return common.WithFunction("human_bytes", 0, 0, func(v any, args []any) any {
		n, err := num(v, "human_bytes")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		return common.MakeUDFSuccessResult(humanBytes(n), nil)
	})
}

func humanBytes(n float64) string {
	if n < 0 {
		return "-" + humanBytes(-n)
	}
	const units = "KMGTPE"
	if n < 1024 {
		return fmt.Sprintf("%d B", int64(n))
	}
	val := n / 1024
	i := 0
	for val >= 1024 && i < len(units)-1 {
		val /= 1024
		i++
	}
	s := fmt.Sprintf("%.1f", val)
	return s + " " + string(units[i]) + "iB"
}

// RegisterHumanNumber registers human_number, a count rendered compactly with
// k, M, B and T suffixes.
func RegisterHumanNumber() gojq.CompilerOption {
	return common.WithFunction("human_number", 0, 0, func(v any, args []any) any {
		n, ok := common.ToFloat64(common.BindValue(v))
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("human_number: expected a number, got %T", v), nil)
		}
		return common.MakeUDFSuccessResult(humanNumber(n), nil)
	})
}

func humanNumber(n float64) string {
	if n < 0 {
		return "-" + humanNumber(-n)
	}
	abs := math.Abs(n)
	switch {
	case abs < 1000:
		return fmt.Sprintf("%.0f", n)
	case abs < 1e6:
		return trim1(n/1e3) + "k"
	case abs < 1e9:
		return trim1(n/1e6) + "M"
	case abs < 1e12:
		return trim1(n/1e9) + "B"
	default:
		return trim1(n/1e12) + "T"
	}
}

func trim1(f float64) string {
	s := fmt.Sprintf("%.1f", f)
	for len(s) > 1 && s[len(s)-1] == '0' {
		s = s[:len(s)-1]
	}
	if s[len(s)-1] == '.' {
		s = s[:len(s)-1]
	}
	return s
}

// RegisterGroupDigits registers group_digits, an integer with thousands
// separators: 1234567 | group_digits -> "1,234,567".
func RegisterGroupDigits() gojq.CompilerOption {
	return common.WithFunction("group_digits", 0, 1, func(v any, args []any) any {
		input := common.BindValue(v)
		if len(args) > 0 {
			input = common.BindValue(args[0])
		}
		var s string
		switch val := input.(type) {
		case string:
			s = val
		default:
			f, ok := common.ToFloat64(val)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("group_digits: expected a number, got %T", input), nil)
			}
			s = fmt.Sprintf("%.0f", f)
		}
		neg := strings.HasPrefix(s, "-")
		if neg {
			s = s[1:]
		}
		if len(s) <= 3 {
			if neg {
				s = "-" + s
			}
			return common.MakeUDFSuccessResult(s, nil)
		}
		var b strings.Builder
		first := len(s) % 3
		if first == 0 {
			first = 3
		}
		b.WriteString(s[:first])
		for i := first; i < len(s); i += 3 {
			b.WriteByte(',')
			b.WriteString(s[i : i+3])
		}
		if neg {
			return common.MakeUDFSuccessResult("-"+b.String(), nil)
		}
		return common.MakeUDFSuccessResult(b.String(), nil)
	})
}

func groupDigits(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	first := len(s) % 3
	if first == 0 {
		first = 3
	}
	b.WriteString(s[:first])
	for i := first; i < len(s); i += 3 {
		b.WriteByte(',')
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

// RegisterFormatCurrency registers format_currency, a number as a currency
// string: format_currency(n; [symbol]) -> "$1,234.57".
func RegisterFormatCurrency() gojq.CompilerOption {
	return common.WithFunction("format_currency", 0, 1, func(v any, args []any) any {
		symbol := "$"
		if len(args) > 0 {
			if s, ok := common.BindValue(args[0]).(string); ok {
				symbol = s
			}
		}
		f, ok := common.ToFloat64(common.BindValue(v))
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("format_currency: expected a number, got %T", v), nil)
		}
		neg := f < 0
		if neg {
			f = -f
		}
		whole := int64(f)
		cents := int64((f-float64(whole))*100 + 0.5)
		if cents >= 100 {
			whole++
			cents -= 100
		}
		grouped := groupDigits(whole)
		out := symbol + grouped + fmt.Sprintf(".%02d", cents)
		if neg {
			out = "-" + out
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterOrdinal registers ordinal, an integer as "1st", "2nd", "3rd", ...
func RegisterOrdinal() gojq.CompilerOption {
	return common.WithFunction("ordinal", 0, 0, func(v any, args []any) any {
		n, err := intInput(v, "ordinal")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		return common.MakeUDFSuccessResult(ordinal(n), nil)
	})
}

func ordinal(n int64) string {
	abs := n
	if abs < 0 {
		abs = -abs
	}
	suffix := "th"
	switch abs % 100 {
	case 11, 12, 13:
		suffix = "th"
	default:
		switch abs % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return fmt.Sprintf("%d%s", n, suffix)
}
