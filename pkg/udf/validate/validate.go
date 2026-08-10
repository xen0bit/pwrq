// Package validate provides pragmatic validators and extractors for the data
// you meet in logs and configs: emails, URLs, domains, JSON, IP addresses, and
// HTML-to-text.
package validate

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

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
	}
}

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
	return gojq.WithFunction(name, 0, 2, func(v any, args []any) any {
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
	return gojq.WithFunction(name, 0, 2, func(v any, args []any) any {
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

// The regexes are deliberately pragmatic. Email validation is famously a
// rabbit hole; the goal here is to separate obvious addresses from everything
// else, not to implement RFC 5322.
var (
	emailPattern = regexp.MustCompile(`^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$`)
	domainPattern = regexp.MustCompile(`^(?i)[a-z0-9]([a-z0-9\-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9\-]{0,61}[a-z0-9])?)*$`)
	extractEmailPattern = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	extractURLPattern   = regexp.MustCompile(`https?://[^\s"'<>]+`)
	extractIPPattern    = regexp.MustCompile(`\b(25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])(\.(25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])){3}\b`)
	tagPattern          = regexp.MustCompile(`<\s*\/?\s*[^>]+>`)
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

// RegisterExtractEmails registers extract_emails, every email-looking token in
// a string.
func RegisterExtractEmails() gojq.CompilerOption {
	return registerFindAll("extract_emails", extractEmailPattern)
}

// RegisterExtractURLs registers extract_urls, every http(s) URL in a string.
func RegisterExtractURLs() gojq.CompilerOption {
	return registerFindAll("extract_urls", extractURLPattern)
}

// RegisterExtractIPs registers extract_ips, every IPv4 address in a string.
func RegisterExtractIPs() gojq.CompilerOption {
	return registerFindAll("extract_ips", extractIPPattern)
}

// RegisterStripTags registers strip_tags, HTML tags removed from a string,
// leaving the text.
func RegisterStripTags() gojq.CompilerOption {
	return gojq.WithFunction("strip_tags", 0, 2, func(v any, args []any) any {
		s, err := strInput(v, args, "strip_tags")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("strip_tags: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(strings.TrimSpace(tagPattern.ReplaceAllString(s, "")), nil)
	})
}
