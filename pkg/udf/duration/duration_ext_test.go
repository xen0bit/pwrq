package duration

import (
	"fmt"
	"testing"
)

func TestDurationBetween(t *testing.T) {
	if got := fmt.Sprint(run(t, `"2026-01-01" | duration_between("2026-01-03")`)); got != "172800" {
		t.Errorf("duration_between = %s, want 172800", got)
	}
}

func TestAdd(t *testing.T) {
	// 0 | add_seconds(3600) = 3600
	if got := fmt.Sprint(run(t, `0 | add_seconds(3600)`)); got != "3600" {
		t.Errorf("add_seconds = %s", got)
	}
	if got := fmt.Sprint(run(t, `0 | add_days(1)`)); got != "86400" {
		t.Errorf("add_days = %s", got)
	}
}

func TestStartEndOfDay(t *testing.T) {
	// Using a date string is timezone-dependent for the local midnight; instead
	// anchor to Unix 0 which is a fixed instant. The offset is local.
	start := toFloat(t, `0 | start_of_day`)
	end := toFloat(t, `0 | end_of_day`)
	if end-start < 86390 || end-start > 86410 {
		t.Errorf("start_of_day/end_of_day span = %v, want ~86400", end-start)
	}
}

func TestCalendar(t *testing.T) {
	if got := fmt.Sprint(run(t, `2024 | is_leap_year`)); got != "true" {
		t.Errorf("2024 leap = %s", got)
	}
	if got := fmt.Sprint(run(t, `2023 | is_leap_year`)); got != "false" {
		t.Errorf("2023 not leap = %s", got)
	}
	if got := fmt.Sprint(run(t, `2024 | days_in_month(2)`)); got != "29" {
		t.Errorf("Feb 2024 = %s", got)
	}
	if got := fmt.Sprint(run(t, `2023 | days_in_month(2)`)); got != "28" {
		t.Errorf("Feb 2023 = %s", got)
	}
	if got := fmt.Sprint(run(t, `2 | month_name`)); got != "February" {
		t.Errorf("month_name(2) = %s", got)
	}
}

func toFloat(t *testing.T, query string) float64 {
	t.Helper()
	v := run(t, query)
	switch n := v.(type) {
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case float64:
		return n
	default:
		t.Fatalf("%s returned %T", query, v)
		return 0
	}
}
