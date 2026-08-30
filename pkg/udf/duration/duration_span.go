// Spans of time: measuring them, parsing them and writing them out.
package duration

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterDurationBetween registers duration_between, the number of seconds
// between two timestamps or dates (second minus first).
func RegisterDurationBetween() gojq.CompilerOption {
	return common.WithFunction("duration_between", 1, 1, func(v any, args []any) any {
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
	return common.WithFunction("human_duration", 0, 0, func(v any, args []any) any {
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

// RegisterParseDuration registers parse_duration, turning a duration string
// into seconds.
//
// It reads all three of the forms pwrq itself writes. It used to read only
// Go's, which left this category not closed under its own round trip: of the
// three duration cmdlets, two produced strings the third could not read.
//
//	93784 | iso_duration   | parse_duration  ->  cannot parse "P1DT2H3M4S"
//	93784 | human_duration | parse_duration  ->  unknown unit "d "
//
// Both messages came from Go's parser and named Go's grammar, so a caller
// staring at a string pwrq had just handed them was told, in effect, that pwrq
// does not accept its own output. A cmdlet and its inverse have to agree, and
// here the inverse is whichever of the two writers produced the string.
func RegisterParseDuration() gojq.CompilerOption {
	return common.WithFunction("parse_duration", 0, 0, func(v any, args []any) any {
		s, ok := common.BindValue(v).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("parse_duration: expected a duration string, got %T", v), nil)
		}
		seconds, err := parseDurationSeconds(s)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("parse_duration: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(seconds, nil)
	})
}

// isoDurationPattern matches the ISO 8601 durations iso_duration writes, plus
// the week designator, which it does not write but which is the one other form
// a caller is likely to arrive with.
//
// Years and months are deliberately absent. "P1M" is a calendar month, whose
// length depends on which month, and answering it in seconds would mean
// picking one silently.
var isoDurationPattern = regexp.MustCompile(
	`^(-?)P(?:(\d+(?:\.\d+)?)W)?(?:(\d+(?:\.\d+)?)D)?(?:T(?:(\d+(?:\.\d+)?)H)?(?:(\d+(?:\.\d+)?)M)?(?:(\d+(?:\.\d+)?)S)?)?$`)

// humanDayPattern matches the day component human_duration writes, which Go's
// parser has no unit for.
var humanDayPattern = regexp.MustCompile(`^(-?)(\d+(?:\.\d+)?)d`)

// parseDurationSeconds reads a duration in any of the three forms pwrq speaks:
// Go's own ("2h30m"), the ISO 8601 that iso_duration writes ("P1DT2H3M4S"),
// and the spaced form that human_duration writes ("1d 2h 3m 4s").
func parseDurationSeconds(text string) (float64, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0, fmt.Errorf("cannot parse %q: it is empty", text)
	}

	if seconds, ok, err := parseISODuration(trimmed); ok {
		return seconds, err
	}

	// human_duration separates its components with spaces and counts days;
	// Go's parser accepts neither. Removing the spaces and turning the days
	// into hours leaves something it does accept, so the two forms share one
	// implementation rather than two that can disagree.
	compact := strings.ReplaceAll(trimmed, " ", "")
	negative := false
	if match := humanDayPattern.FindStringSubmatch(compact); match != nil {
		days, err := strconv.ParseFloat(match[2], 64)
		if err != nil {
			return 0, fmt.Errorf("cannot parse %q: %v", text, err)
		}
		negative = match[1] == "-"
		compact = fmt.Sprintf("%gh", days*24) + compact[len(match[0]):]
	}

	d, err := time.ParseDuration(compact)
	if err != nil {
		return 0, fmt.Errorf("cannot parse %q: expected a duration like 2h30m, "+
			"an ISO 8601 duration like P1DT2H3M4S, or a spaced one like 1d 2h 3m 4s", text)
	}
	if negative {
		return -d.Seconds(), nil
	}
	return d.Seconds(), nil
}

// parseISODuration reads an ISO 8601 duration, reporting whether the text was
// one at all so the caller can fall through to the other grammars.
func parseISODuration(text string) (float64, bool, error) {
	upper := strings.ToUpper(text)
	if !strings.HasPrefix(upper, "P") && !strings.HasPrefix(upper, "-P") {
		return 0, false, nil
	}
	match := isoDurationPattern.FindStringSubmatch(upper)
	if match == nil {
		return 0, true, fmt.Errorf("cannot parse %q: it starts like an ISO 8601 duration "+
			"but is not one; pwrq reads weeks, days, hours, minutes and seconds, as in P1DT2H3M4S", text)
	}

	units := []float64{604800, 86400, 3600, 60, 1} // W D H M S
	var seconds float64
	for i, unit := range units {
		field := match[i+2]
		if field == "" {
			continue
		}
		n, err := strconv.ParseFloat(field, 64)
		if err != nil {
			return 0, true, fmt.Errorf("cannot parse %q: %v", text, err)
		}
		seconds += n * unit
	}
	if match[1] == "-" {
		seconds = -seconds
	}
	return seconds, true, nil
}

// RegisterIsoDuration registers iso_duration, seconds as an ISO 8601 duration
// like "P1DT2H3M4S".
func RegisterIsoDuration() gojq.CompilerOption {
	return common.WithFunction("iso_duration", 0, 0, func(v any, args []any) any {
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
	return common.WithFunction("time_ago", 0, 0, func(v any, args []any) any {
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
