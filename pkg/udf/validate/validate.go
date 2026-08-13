// Package validate provides format predicates and extractors over text.
package validate

import (
	"fmt"
	"regexp"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterAll registers every validation cmdlet.
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterIsEmail(),
		RegisterIsURL(),
		RegisterIsDomain(),
		RegisterIsJSON(),
		RegisterExtractEmails(),
		RegisterExtractURLs(),
		RegisterExtractIPs(),
		RegisterStripTags(),
		RegisterIsSemver(),
		RegisterIsCreditCard(),
		RegisterSemverCompare(),
		RegisterSemverParts(),
		RegisterIsHex(),
		RegisterIsCIDR(),
		RegisterIsPort(),
		RegisterIsDate(),
		RegisterIsISO8601(),
		RegisterIsSlug(),
		RegisterExtractDates(),
		RegisterIsNumeric(),
	}
}

// The shapes the format predicates and extractors match against, kept together
// so a change to one is visible next to the others.
var (
	emailPattern        = regexp.MustCompile(`^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$`)
	domainPattern       = regexp.MustCompile(`^(?i)[a-z0-9]([a-z0-9\-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9\-]{0,61}[a-z0-9])?)*$`)
	slugPattern         = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	isoDatePattern      = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
	iso8601Pattern      = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}([T ][0-9]{2}:[0-9]{2}(:[0-9]{2})?([.,][0-9]+)?(Z|[+-][0-9]{2}:?[0-9]{2})?)?$`)
	extractEmailPattern = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	extractURLPattern   = regexp.MustCompile(`https?://[^\s"'<>]+`)
	extractIPPattern    = regexp.MustCompile(`\b(25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])(\.(25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])){3}\b`)
	extractDatePattern  = regexp.MustCompile(`\b[0-9]{4}-[0-9]{2}-[0-9]{2}\b`)
	tagPattern          = regexp.MustCompile(`<\s*\/?\s*[^>]+>`)
)

// strInput resolves a string from the pipeline or first argument.
func strInput(v any, args []any, name string) (string, error) {
	inputVal, _, err := common.ParseFileArgs(v, args)
	if err != nil {
		return "", err
	}
	switch val := common.BindValue(inputVal).(type) {
	case string:
		return val, nil
	case []byte:
		return string(val), nil
	default:
		return "", fmt.Errorf("%s: expected a string, got %T", name, inputVal)
	}
}

// registerBool registers a 0-2 arity predicate over a string.
func registerBool(name string, fn func(string) bool) gojq.CompilerOption {
	return common.WithFunction(name, 0, 2, func(v any, args []any) any {
		s, err := strInput(v, args, name)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: %v", name, err), nil)
		}
		return common.MakeUDFSuccessResult(fn(s), nil)
	})
}

// registerFindAll registers a 0-2 arity cmdlet that returns the regex matches
// in a string as an array.
func registerFindAll(name string, re *regexp.Regexp) gojq.CompilerOption {
	return common.WithFunction(name, 0, 2, func(v any, args []any) any {
		s, err := strInput(v, args, name)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: %v", name, err), nil)
		}
		matches := re.FindAllString(s, -1)
		out := make([]any, len(matches))
		for i, m := range matches {
			out[i] = m
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}
