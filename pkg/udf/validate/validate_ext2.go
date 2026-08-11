package validate

import (
	"encoding/hex"
	"fmt"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

var (
	slugPattern        = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	isoDatePattern     = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
	iso8601Pattern     = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}([T ][0-9]{2}:[0-9]{2}(:[0-9]{2})?([.,][0-9]+)?(Z|[+-][0-9]{2}:?[0-9]{2})?)?$`)
	extractDatePattern = regexp.MustCompile(`\b[0-9]{4}-[0-9]{2}-[0-9]{2}\b`)
)

// RegisterSemverCompare registers semver_compare, -1, 0 or 1 comparing two
// semantic versions: semver_compare(a; b), or a from the pipeline.
func RegisterSemverCompare() gojq.CompilerOption {
	return gojq.WithFunction("semver_compare", 1, 2, func(v any, args []any) any {
		a, okA := common.BindValue(v).(string)
		b, okB := common.BindValue(args[len(args)-1]).(string)
		if len(args) > 1 {
			a, okA = common.BindValue(args[0]).(string)
		}
		if !okA || !okB {
			return common.MakeUDFErrorResult(fmt.Errorf("semver_compare: two version strings are required"), nil)
		}
		cmp, err := compareSemver(a, b)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("semver_compare: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(cmp, nil)
	})
}

func compareSemver(a, b string) (int, error) {
	pa, err := parseSemver(a)
	if err != nil {
		return 0, err
	}
	pb, err := parseSemver(b)
	if err != nil {
		return 0, err
	}
	for i := 0; i < 3; i++ {
		if pa[i] < pb[i] {
			return -1, nil
		}
		if pa[i] > pb[i] {
			return 1, nil
		}
	}
	// Pre-release compares lower than release; no pre-release wins.
	if pa[3] == 0 && pb[3] == 0 {
		return 0, nil
	}
	if pa[3] == 0 {
		return 1, nil
	}
	if pb[3] == 0 {
		return -1, nil
	}
	return comparePrerelease(a, b), nil
}

// parseSemver returns [major, minor, patch, hasPrerelease].
func parseSemver(s string) ([4]int, error) {
	var out [4]int
	core := strings.TrimSpace(s)
	if i := strings.IndexAny(core, "-+"); i >= 0 {
		if strings.HasPrefix(core[i:], "-") {
			out[3] = 1
		}
		core = core[:i]
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return out, fmt.Errorf("%q is not a semantic version", s)
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, fmt.Errorf("%q is not a semantic version", s)
		}
		out[i] = n
	}
	return out, nil
}

func comparePrerelease(a, b string) int {
	prereleaseOf := func(s string) string {
		rest := strings.TrimPrefix(s, s[:strings.IndexAny(s, "-+")])
		if strings.HasPrefix(rest, "-") {
			rest = rest[1:]
		}
		if i := strings.Index(rest, "+"); i >= 0 {
			rest = rest[:i]
		}
		return rest
	}
	pa := strings.Split(prereleaseOf(a), ".")
	pb := strings.Split(prereleaseOf(b), ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		// Numeric identifiers compare numerically, alphanumeric lexically.
		na, errA := strconv.Atoi(pa[i])
		nb, errB := strconv.Atoi(pb[i])
		switch {
		case errA == nil && errB == nil:
			if na < nb {
				return -1
			}
			if na > nb {
				return 1
			}
		default:
			if pa[i] < pb[i] {
				return -1
			}
			if pa[i] > pb[i] {
				return 1
			}
		}
	}
	if len(pa) < len(pb) {
		return -1
	}
	if len(pa) > len(pb) {
		return 1
	}
	return 0
}

// RegisterSemverParts registers semver_parts, a semantic version split into
// {major, minor, patch, prerelease, build}.
func RegisterSemverParts() gojq.CompilerOption {
	return gojq.WithFunction("semver_parts", 0, 1, func(v any, args []any) any {
		s, err := strInput(v, args, "semver_parts")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		s = strings.TrimSpace(s)
		if !semverPattern.MatchString(s) {
			return common.MakeUDFErrorResult(fmt.Errorf("semver_parts: %q is not a semantic version", s), nil)
		}
		out := map[string]any{}
		core := s
		if i := strings.Index(core, "+"); i >= 0 {
			out["build"] = core[i+1:]
			core = core[:i]
		}
		if i := strings.Index(core, "-"); i >= 0 {
			out["prerelease"] = core[i+1:]
			core = core[:i]
		}
		nums := strings.Split(core, ".")
		for i, name := range []string{"major", "minor", "patch"} {
			n, _ := strconv.Atoi(nums[i])
			out[name] = n
		}
		return common.MakeUDFSuccessResult(out, nil)
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
	return gojq.WithFunction("is_port", 0, 1, func(v any, args []any) any {
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

// RegisterExtractDates registers extract_dates, every YYYY-MM-DD date in a
// string.
func RegisterExtractDates() gojq.CompilerOption {
	return registerFindAll("extract_dates", extractDatePattern)
}
