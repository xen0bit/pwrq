// Spans of time: measuring them, parsing them and writing them out.
package duration

import (
	"fmt"
	"strings"
	"time"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterDurationBetween registers duration_between, the number of seconds
// between two timestamps or dates (second minus first).
func RegisterDurationBetween() gojq.CompilerOption {
	return gojq.WithFunction("duration_between", 1, 1, func(v any, args []any) any {
		a, err := parseTime(v)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("duration_between: %v", err), nil)
		}
		b, err := parseTime(args[0])
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("duration_between: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(b.Sub(a).Seconds(), nil)
	})
}

// RegisterHumanDuration registers human_duration, rendering seconds as a
// compact "1d 2h 3m 4s" string.
func RegisterHumanDuration() gojq.CompilerOption {
	return gojq.WithFunction("human_duration", 0, 0, func(v any, args []any) any {
		sec, ok := common.ToFloat64(common.BindValue(v))
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("human_duration: expected a number of seconds, got %T", v), nil)
		}
		return common.MakeUDFSuccessResult(humanDuration(sec), nil)
	})
}

func humanDuration(sec float64) string {
	if sec < 0 {
		return "-" + humanDuration(-sec)
	}
	total := int64(sec)
	if total < 60 {
		return fmt.Sprintf("%ds", total)
	}
	days := total / 86400
	hours := (total % 86400) / 3600
	mins := (total % 3600) / 60
	secs := total % 60
	parts := make([]string, 0, 4)
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if mins > 0 {
		parts = append(parts, fmt.Sprintf("%dm", mins))
	}
	if secs > 0 {
		parts = append(parts, fmt.Sprintf("%ds", secs))
	}
	return strings.Join(parts, " ")
}

// RegisterParseDuration registers parse_duration, turning a Go-style duration
// like "2h30m" into seconds.
func RegisterParseDuration() gojq.CompilerOption {
	return gojq.WithFunction("parse_duration", 0, 0, func(v any, args []any) any {
		s, ok := common.BindValue(v).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("parse_duration: expected a duration string, got %T", v), nil)
		}
		d, err := time.ParseDuration(strings.TrimSpace(s))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("parse_duration: cannot parse %q: %v", s, err), nil)
		}
		return common.MakeUDFSuccessResult(d.Seconds(), nil)
	})
}

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

// RegisterTimeAgo registers time_ago, a timestamp rendered relative to now.
func RegisterTimeAgo() gojq.CompilerOption {
	return gojq.WithFunction("time_ago", 0, 0, func(v any, args []any) any {
		t, err := parseTime(v)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("time_ago: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(relative(time.Since(t)), nil)
	})
}

func relative(d time.Duration) string {
	if d < 0 {
		d = -d
		return inAWhile(d)
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return plural(int(d/time.Minute), "minute") + " ago"
	case d < 24*time.Hour:
		return plural(int(d/time.Hour), "hour") + " ago"
	case d < 7*24*time.Hour:
		return plural(int(d/(24*time.Hour)), "day") + " ago"
	case d < 30*24*time.Hour:
		return plural(int(d/(7*24*time.Hour)), "week") + " ago"
	case d < 365*24*time.Hour:
		return plural(int(d/(30*24*time.Hour)), "month") + " ago"
	default:
		return plural(int(d/(365*24*time.Hour)), "year") + " ago"
	}
}

func inAWhile(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "in a moment"
	case d < time.Hour:
		return "in " + plural(int(d/time.Minute), "minute")
	case d < 24*time.Hour:
		return "in " + plural(int(d/time.Hour), "hour")
	default:
		return "in " + plural(int(d/(24*time.Hour)), "day")
	}
}

func plural(n int, unit string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", unit)
	}
	return fmt.Sprintf("%d %ss", n, unit)
}
