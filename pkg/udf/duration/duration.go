// Package duration provides human-friendly time handling: durations rendered
// and parsed, relative times, and weekday helpers.
package duration

import (
	"fmt"
	"strings"
	"time"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterAll registers every duration cmdlet.
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterHumanDuration(),
		RegisterParseDuration(),
		RegisterTimeAgo(),
		RegisterWeekday(),
		RegisterIsWeekend(),
		RegisterDurationBetween(),
		RegisterAddSeconds(),
		RegisterAddDays(),
		RegisterStartOfDay(),
		RegisterEndOfDay(),
		RegisterIsLeapYear(),
		RegisterDaysInMonth(),
		RegisterMonthName(),
	}
}

// parseTime accepts a Unix timestamp (number) or an ISO date/time string.
func parseTime(v any) (time.Time, error) {
	switch val := common.BindValue(v).(type) {
	case string:
		for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
			if t, err := time.Parse(layout, val); err == nil {
				return t, nil
			}
		}
		return time.Time{}, fmt.Errorf("cannot parse %q as a date", val)
	default:
		if f, ok := common.ToFloat64(val); ok {
			return time.Unix(int64(f), 0), nil
		}
		return time.Time{}, fmt.Errorf("expected a Unix timestamp or a date string, got %T", val)
	}
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

// RegisterWeekday registers weekday, the name of the day for a timestamp or
// date string.
func RegisterWeekday() gojq.CompilerOption {
	return gojq.WithFunction("weekday", 0, 0, func(v any, args []any) any {
		t, err := parseTime(v)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("weekday: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(t.Weekday().String(), nil)
	})
}

// RegisterIsWeekend registers is_weekend, whether the day is Saturday or
// Sunday.
func RegisterIsWeekend() gojq.CompilerOption {
	return gojq.WithFunction("is_weekend", 0, 0, func(v any, args []any) any {
		t, err := parseTime(v)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("is_weekend: %v", err), nil)
		}
		d := t.Weekday()
		return common.MakeUDFSuccessResult(d == time.Saturday || d == time.Sunday, nil)
	})
}
