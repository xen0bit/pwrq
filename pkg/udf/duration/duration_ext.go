package duration

import (
	"fmt"
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

// RegisterAddSeconds registers add_seconds, a timestamp plus n seconds.
func RegisterAddSeconds() gojq.CompilerOption {
	return gojq.WithFunction("add_seconds", 1, 1, func(v any, args []any) any {
		n, ok := common.ToFloat64(common.BindValue(args[0]))
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("add_seconds: expected a number of seconds, got %v", args[0]), nil)
		}
		t, err := parseTime(v)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("add_seconds: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(t.Add(time.Duration(n * float64(time.Second))).Unix(), nil)
	})
}

// RegisterAddDays registers add_days, a timestamp plus n days.
func RegisterAddDays() gojq.CompilerOption {
	return gojq.WithFunction("add_days", 1, 1, func(v any, args []any) any {
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

// RegisterStartOfDay registers start_of_day, a timestamp at local midnight.
func RegisterStartOfDay() gojq.CompilerOption {
	return gojq.WithFunction("start_of_day", 0, 0, func(v any, args []any) any {
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
	return gojq.WithFunction("end_of_day", 0, 0, func(v any, args []any) any {
		t, err := parseTime(v)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("end_of_day: %v", err), nil)
		}
		y, m, d := t.Date()
		return common.MakeUDFSuccessResult(time.Date(y, m, d, 23, 59, 59, 0, t.Location()).Unix(), nil)
	})
}

// RegisterIsLeapYear registers is_leap_year, whether a year has 366 days.
func RegisterIsLeapYear() gojq.CompilerOption {
	return gojq.WithFunction("is_leap_year", 0, 0, func(v any, args []any) any {
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
	return gojq.WithFunction("days_in_month", 1, 1, func(v any, args []any) any {
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
	return gojq.WithFunction("month_name", 0, 0, func(v any, args []any) any {
		m, ok := common.ToInt(common.BindValue(v))
		if !ok || m < 1 || m > 12 {
			return common.MakeUDFErrorResult(fmt.Errorf("month_name: expected a month 1-12, got %v", v), nil)
		}
		return common.MakeUDFSuccessResult(time.Month(m).String(), nil)
	})
}
