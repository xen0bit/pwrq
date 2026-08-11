package duration

import (
	"fmt"
	"time"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterDaysBetween registers days_between, the number of calendar days
// between two dates or timestamps (second minus first).
func RegisterDaysBetween() gojq.CompilerOption {
	return gojq.WithFunction("days_between", 1, 1, func(v any, args []any) any {
		a, err := parseTime(v)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("days_between: %v", err), nil)
		}
		b, err := parseTime(args[0])
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("days_between: %v", err), nil)
		}
		days := b.Sub(a).Hours() / 24
		return common.MakeUDFSuccessResult(int64(days), nil)
	})
}

// RegisterDayOfYear registers day_of_year, the day number within the year
// (1-366).
func RegisterDayOfYear() gojq.CompilerOption {
	return gojq.WithFunction("day_of_year", 0, 0, func(v any, args []any) any {
		t, err := parseTime(v)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("day_of_year: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(t.YearDay(), nil)
	})
}

// RegisterWeekOfYear registers week_of_year, the ISO 8601 week number (1-53).
func RegisterWeekOfYear() gojq.CompilerOption {
	return gojq.WithFunction("week_of_year", 0, 0, func(v any, args []any) any {
		t, err := parseTime(v)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("week_of_year: %v", err), nil)
		}
		_, week := t.ISOWeek()
		return common.MakeUDFSuccessResult(week, nil)
	})
}

// RegisterStartOfWeek registers start_of_week, a timestamp at the Monday
// midnight of its ISO week.
func RegisterStartOfWeek() gojq.CompilerOption {
	return gojq.WithFunction("start_of_week", 0, 0, func(v any, args []any) any {
		t, err := parseTime(v)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("start_of_week: %v", err), nil)
		}
		offset := (int(t.Weekday()) + 6) % 7 // days since Monday
		y, m, d := t.Date()
		start := time.Date(y, m, d, 0, 0, 0, 0, t.Location()).AddDate(0, 0, -offset)
		return common.MakeUDFSuccessResult(start.Unix(), nil)
	})
}

// RegisterAddMonths registers add_months, a timestamp plus n months.
func RegisterAddMonths() gojq.CompilerOption {
	return gojq.WithFunction("add_months", 1, 1, func(v any, args []any) any {
		n, ok := common.ToInt(args[0])
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("add_months: expected a number of months, got %v", args[0]), nil)
		}
		t, err := parseTime(v)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("add_months: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(t.AddDate(0, n, 0).Unix(), nil)
	})
}

// RegisterAddYears registers add_years, a timestamp plus n years.
func RegisterAddYears() gojq.CompilerOption {
	return gojq.WithFunction("add_years", 1, 1, func(v any, args []any) any {
		n, ok := common.ToInt(args[0])
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("add_years: expected a number of years, got %v", args[0]), nil)
		}
		t, err := parseTime(v)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("add_years: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(t.AddDate(n, 0, 0).Unix(), nil)
	})
}

// RegisterAgeInYears registers age_in_years, whole years between a birth date
// and now (or a given second timestamp for deterministic callers).
func RegisterAgeInYears() gojq.CompilerOption {
	return gojq.WithFunction("age_in_years", 0, 1, func(v any, args []any) any {
		birth, err := parseTime(v)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("age_in_years: %v", err), nil)
		}
		now := time.Now()
		if len(args) > 0 {
			if n, ok := common.ToFloat64(common.BindValue(args[0])); ok {
				now = time.Unix(int64(n), 0)
			}
		}
		age := now.Year() - birth.Year()
		if now.YearDay() < birth.YearDay() {
			age--
		}
		return common.MakeUDFSuccessResult(age, nil)
	})
}
