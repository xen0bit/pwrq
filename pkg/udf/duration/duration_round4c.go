package duration

import (
	"fmt"
	"time"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterIsoDuration registers iso_duration, seconds as an ISO 8601 duration
// like "P1DT2H3M4S".
func RegisterIsoDuration() gojq.CompilerOption {
	return gojq.WithFunction("iso_duration", 0, 0, func(v any, args []any) any {
		sec, ok := common.ToFloat64(common.BindValue(v))
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("iso_duration: expected a number of seconds, got %T", v), nil)
		}
		return common.MakeUDFSuccessResult(isoDuration(int64(sec)), nil)
	})
}

func isoDuration(total int64) string {
	if total == 0 {
		return "PT0S"
	}
	d := time.Duration(total) * time.Second
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	hours := d / time.Hour
	d -= hours * time.Hour
	mins := d / time.Minute
	d -= mins * time.Minute
	secs := d / time.Second

	out := "P"
	if days > 0 {
		out += fmt.Sprintf("%dD", days)
	}
	if hours > 0 || mins > 0 || secs > 0 {
		out += "T"
	}
	if hours > 0 {
		out += fmt.Sprintf("%dH", hours)
	}
	if mins > 0 {
		out += fmt.Sprintf("%dM", mins)
	}
	if secs > 0 {
		out += fmt.Sprintf("%dS", secs)
	}
	return out
}
