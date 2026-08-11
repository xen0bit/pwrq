// Pulling structured values back out of free text.
package validate

import (
	"fmt"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

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

// RegisterExtractDates registers extract_dates, every YYYY-MM-DD date in a
// string.
func RegisterExtractDates() gojq.CompilerOption {
	return registerFindAll("extract_dates", extractDatePattern)
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
