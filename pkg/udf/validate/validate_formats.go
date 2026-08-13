// Predicates: does this string have the shape of an email, a URL, a port?
package validate

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterIsEmail registers is_email, a pragmatic email check.
func RegisterIsEmail() gojq.CompilerOption {
	return registerBool("is_email", func(s string) bool {
		return emailPattern.MatchString(strings.TrimSpace(s))
	})
}

// RegisterIsURL registers is_url, whether a string is an http(s) URL.
func RegisterIsURL() gojq.CompilerOption {
	return registerBool("is_url", func(s string) bool {
		u, err := url.Parse(s)
		if err != nil {
			return false
		}
		return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
	})
}

// RegisterIsDomain registers is_domain, a hostname-style string check.
func RegisterIsDomain() gojq.CompilerOption {
	return registerBool("is_domain", func(s string) bool {
		return domainPattern.MatchString(strings.TrimSpace(s))
	})
}

// RegisterIsJSON registers is_json, whether a string parses as JSON.
func RegisterIsJSON() gojq.CompilerOption {
	return registerBool("is_json", func(s string) bool {
		var v any
		return json.Unmarshal([]byte(s), &v) == nil
	})
}

// RegisterIsHex registers is_hex, whether a string is a hexadecimal number.
func RegisterIsHex() gojq.CompilerOption {
	return registerBool("is_hex", func(s string) bool {
		s = strings.TrimSpace(s)
		if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
			s = s[2:]
		}
		if s == "" {
			return false
		}
		_, err := hex.DecodeString(s)
		return err == nil
	})
}

// RegisterIsCIDR registers is_cidr, whether a string is an IP/CIDR block.
func RegisterIsCIDR() gojq.CompilerOption {
	return registerBool("is_cidr", func(s string) bool {
		_, err := netip.ParsePrefix(strings.TrimSpace(s))
		return err == nil
	})
}

// RegisterIsPort registers is_port, whether a number is a valid port (1-65535).
func RegisterIsPort() gojq.CompilerOption {
	return common.WithFunction("is_port", 0, 1, func(v any, args []any) any {
		input := v
		if len(args) > 0 {
			input = args[0]
		}
		if f, ok := common.ToFloat64(common.BindValue(input)); ok {
			return common.MakeUDFSuccessResult(f >= 1 && f <= 65535 && f == float64(int(f)), nil)
		}
		if s, ok := common.BindValue(input).(string); ok {
			n, err := strconv.Atoi(strings.TrimSpace(s))
			return common.MakeUDFSuccessResult(err == nil && n >= 1 && n <= 65535, nil)
		}
		return common.MakeUDFErrorResult(fmt.Errorf("is_port: expected a number or port string, got %T", input), nil)
	})
}

// RegisterIsDate registers is_date, whether a string is a YYYY-MM-DD date.
func RegisterIsDate() gojq.CompilerOption {
	return registerBool("is_date", func(s string) bool {
		if !isoDatePattern.MatchString(strings.TrimSpace(s)) {
			return false
		}
		_, err := time.Parse("2006-01-02", strings.TrimSpace(s))
		return err == nil
	})
}

// RegisterIsISO8601 registers is_iso8601, whether a string is an ISO 8601
// timestamp or date.
func RegisterIsISO8601() gojq.CompilerOption {
	return registerBool("is_iso8601", func(s string) bool {
		s = strings.TrimSpace(s)
		if !iso8601Pattern.MatchString(s) {
			return false
		}
		for _, layout := range []string{
			time.RFC3339, time.RFC3339Nano, "2006-01-02",
			"2006-01-02T15:04:05", "2006-01-02 15:04:05",
		} {
			if _, err := time.Parse(layout, s); err == nil {
				return true
			}
		}
		return false
	})
}

// RegisterIsSlug registers is_slug, whether a string is lowercase letters,
// digits and hyphens, the shape slugify produces.
func RegisterIsSlug() gojq.CompilerOption {
	return registerBool("is_slug", func(s string) bool {
		return slugPattern.MatchString(s)
	})
}

// RegisterIsNumeric registers is_numeric, whether a string parses as an
// integer or floating-point number.
func RegisterIsNumeric() gojq.CompilerOption {
	return registerBool("is_numeric", func(s string) bool {
		s = strings.TrimSpace(s)
		if s == "" {
			return false
		}
		_, err := strconv.ParseFloat(s, 64)
		return err == nil
	})
}

// RegisterIsCreditCard registers is_credit_card, whether a string of digits
// passes the Luhn checksum.
func RegisterIsCreditCard() gojq.CompilerOption {
	return registerBool("is_credit_card", func(s string) bool {
		digits := ""
		for _, r := range s {
			if r >= '0' && r <= '9' {
				digits += string(r)
			}
		}
		if len(digits) < 13 || len(digits) > 19 {
			return false
		}
		return luhn(digits)
	})
}

func luhn(digits string) bool {
	sum := 0
	double := false
	for i := len(digits) - 1; i >= 0; i-- {
		d := int(digits[i] - '0')
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return sum%10 == 0
}
