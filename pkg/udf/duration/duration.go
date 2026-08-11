// Package duration provides date arithmetic and duration formatting.
package duration

import (
	"fmt"
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
		RegisterDaysBetween(),
		RegisterDayOfYear(),
		RegisterWeekOfYear(),
		RegisterStartOfWeek(),
		RegisterAddMonths(),
		RegisterAddYears(),
		RegisterAgeInYears(),
		RegisterIsoDuration(),
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
