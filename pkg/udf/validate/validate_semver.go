// Semantic versions: validating, parsing and ordering them.
package validate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

var semverPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)

// RegisterIsSemver registers is_semver, whether a string is a semantic version
// like 1.2.3 or 2.0.0-rc.1+build.
func RegisterIsSemver() gojq.CompilerOption {
	return registerBool("is_semver", func(s string) bool {
		return semverPattern.MatchString(strings.TrimSpace(s))
	})
}

// RegisterSemverCompare registers semver_compare, -1, 0 or 1 comparing two
// semantic versions: semver_compare(a; b), or a from the pipeline.
func RegisterSemverCompare() gojq.CompilerOption {
	return common.WithFunction("semver_compare", 1, 2, func(v any, args []any) any {
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
		rest = strings.TrimPrefix(rest, "-")
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
	return common.WithFunction("semver_parts", 0, 1, func(v any, args []any) any {
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
