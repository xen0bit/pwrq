package number

import (
	"fmt"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

var (
	smallWords = []string{"zero", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine",
		"ten", "eleven", "twelve", "thirteen", "fourteen", "fifteen", "sixteen", "seventeen",
		"eighteen", "nineteen"}
	tensWords = []string{"", "", "twenty", "thirty", "forty", "fifty", "sixty", "seventy", "eighty", "ninety"}
)

func below1000(n int64) string {
	var parts []string
	if n >= 100 {
		parts = append(parts, smallWords[n/100], "hundred")
		n %= 100
	}
	switch {
	case n >= 20:
		tens := tensWords[n/10]
		if n%10 != 0 {
			tens += "-" + smallWords[n%10]
		}
		parts = append(parts, tens)
	case n > 0:
		parts = append(parts, smallWords[n])
	}
	return strings.Join(parts, " ")
}

func numberToWords(n int64) string {
	if n == 0 {
		return "zero"
	}
	scale := []string{"", " thousand", " million", " billion", " trillion", " quadrillion"}
	var groups []string
	for n > 0 {
		groups = append(groups, below1000(n%1000))
		n /= 1000
	}
	var out []string
	for i := len(groups) - 1; i >= 0; i-- {
		if groups[i] == "" {
			continue
		}
		out = append(out, groups[i]+scale[i])
	}
	return strings.Join(out, " ")
}

func toRoman(n int64) string {
	values := []struct {
		val int64
		sym string
	}{
		{1000, "M"}, {900, "CM"}, {500, "D"}, {400, "CD"},
		{100, "C"}, {90, "XC"}, {50, "L"}, {40, "XL"},
		{10, "X"}, {9, "IX"}, {5, "V"}, {4, "IV"}, {1, "I"},
	}
	var out strings.Builder
	for _, v := range values {
		for n >= v.val {
			out.WriteString(v.sym)
			n -= v.val
		}
	}
	return out.String()
}

// RegisterGroupDigits registers group_digits, an integer with thousands
// separators: 1234567 | group_digits -> "1,234,567".
func RegisterGroupDigits() gojq.CompilerOption {
	return gojq.WithFunction("group_digits", 0, 1, func(v any, args []any) any {
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

// RegisterFormatCurrency registers format_currency, a number as a currency
// string: format_currency(n; [symbol]) -> "$1,234.57".
func RegisterFormatCurrency() gojq.CompilerOption {
	return gojq.WithFunction("format_currency", 0, 1, func(v any, args []any) any {
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
