// Calendar arithmetic: days, weeks, months and years.
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
	return common.WithFunction("days_between", 1, 1, func(v any, args []any) any {
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
	return common.WithFunction("day_of_year", 0, 0, func(v any, args []any) any {
		t, err := parseTime(v)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("day_of_year: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(t.YearDay(), nil)
	})
}

// RegisterWeekOfYear registers week_of_year, the ISO 8601 week number (1-53).
func RegisterWeekOfYear() gojq.CompilerOption {
	return common.WithFunction("week_of_year", 0, 0, func(v any, args []any) any {
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
	return common.WithFunction("start_of_week", 0, 0, func(v any, args []any) any {
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

// RegisterStartOfDay registers start_of_day, a timestamp at local midnight.
func RegisterStartOfDay() gojq.CompilerOption {
	return common.WithFunction("start_of_day", 0, 0, func(v any, args []any) any {
		t, err := parseTime(v)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("start_of_day: %v", err), nil)
		}
		y, m, d := t.Date()
		return common.MakeUDFSuccessResult(time.Date(y, m, d, 0, 0, 0, 0, t.Location()).Unix(), nil)
	})
}

// RegisterEndOfDay registers end_of_day, a timestamp at the last second of its
// local day.
func RegisterEndOfDay() gojq.CompilerOption {
	return common.WithFunction("end_of_day", 0, 0, func(v any, args []any) any {
		t, err := parseTime(v)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("end_of_day: %v", err), nil)
		}
		y, m, d := t.Date()
		return common.MakeUDFSuccessResult(time.Date(y, m, d, 23, 59, 59, 0, t.Location()).Unix(), nil)
	})
}

// RegisterAddSeconds registers add_seconds, a timestamp plus n seconds.
func RegisterAddSeconds() gojq.CompilerOption {
	return common.WithFunction("add_seconds", 1, 1, func(v any, args []any) any {
		n, ok := common.ToFloat64(common.BindValue(args[0]))
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("add_seconds: expected a number of seconds, got %v", args[0]), nil)
		}
		t, err := parseTime(v)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("add_seconds: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(t.Add(time.Duration(n*float64(time.Second))).Unix(), nil)
	})
}

// RegisterAddDays registers add_days, a timestamp plus n days.
func RegisterAddDays() gojq.CompilerOption {
	return common.WithFunction("add_days", 1, 1, func(v any, args []any) any {
		n, ok := common.ToInt(args[0])
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("add_days: expected a number of days, got %v", args[0]), nil)
		}
		t, err := parseTime(v)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("add_days: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(t.AddDate(0, 0, n).Unix(), nil)
	})
}

// RegisterAddMonths registers add_months, a timestamp plus n months.
func RegisterAddMonths() gojq.CompilerOption {
	return common.WithFunction("add_months", 1, 1, func(v any, args []any) any {
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
	return common.WithFunction("add_years", 1, 1, func(v any, args []any) any {
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
	return common.WithFunction("age_in_years", 0, 1, func(v any, args []any) any {
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

// RegisterIsLeapYear registers is_leap_year, whether a year has 366 days.
func RegisterIsLeapYear() gojq.CompilerOption {
	return common.WithFunction("is_leap_year", 0, 0, func(v any, args []any) any {
		y, ok := common.ToInt(common.BindValue(v))
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("is_leap_year: expected a year number, got %T", v), nil)
		}
		return common.MakeUDFSuccessResult(isLeap(y), nil)
	})
}

func isLeap(y int) bool {
	return y%4 == 0 && (y%100 != 0 || y%400 == 0)
}

// RegisterDaysInMonth registers days_in_month, the number of days in a month
// of a year: year from the pipeline, month (1-12) as the argument.
func RegisterDaysInMonth() gojq.CompilerOption {
	return common.WithFunction("days_in_month", 1, 1, func(v any, args []any) any {
		y, yOK := common.ToInt(common.BindValue(v))
		m, mOK := common.ToInt(common.BindValue(args[0]))
		if !yOK || !mOK || m < 1 || m > 12 {
			return common.MakeUDFErrorResult(fmt.Errorf("days_in_month: expected a year and a month 1-12"), nil)
		}
		return common.MakeUDFSuccessResult(time.Date(y, time.Month(m), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, -1).Day(), nil)
	})
}

// RegisterMonthName registers month_name, the name of a month (1-12).
func RegisterMonthName() gojq.CompilerOption {
	return common.WithFunction("month_name", 0, 0, func(v any, args []any) any {
		m, ok := common.ToInt(common.BindValue(v))
		if !ok || m < 1 || m > 12 {
			return common.MakeUDFErrorResult(fmt.Errorf("month_name: expected a month 1-12, got %v", v), nil)
		}
		return common.MakeUDFSuccessResult(time.Month(m).String(), nil)
	})
}

// RegisterWeekday registers weekday, the name of the day for a timestamp or
// date string.
func RegisterWeekday() gojq.CompilerOption {
	return common.WithFunction("weekday", 0, 0, func(v any, args []any) any {
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
	return common.WithFunction("is_weekend", 0, 0, func(v any, args []any) any {
		t, err := parseTime(v)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("is_weekend: %v", err), nil)
		}
		d := t.Weekday()
		return common.MakeUDFSuccessResult(d == time.Saturday || d == time.Sunday, nil)
	})
}
