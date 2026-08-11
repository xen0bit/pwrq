package duration

import (
	"fmt"
	"testing"
)

func TestDurationExtras(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{`"2026-01-01" | days_between("2026-01-10")`, "9"},
		{`"2026-08-10" | day_of_year`, "222"},
		{`"2026-08-10" | week_of_year`, "33"},
		{`"2026-08-13T12:00:00Z" | start_of_week`, "1786320000"},
		{`"2026-01-15" | add_months(1)`, "1771113600"},
		{`"2026-01-15" | add_years(2)`, "1831507200"},
		{`"2000-06-15" | age_in_years(1771113600)`, "25"},
	}
	for _, tt := range tests {
		got := fmt.Sprint(run(t, tt.query))
		if got != tt.want {
			t.Errorf("%s = %s, want %s", tt.query, got, tt.want)
		}
	}
}
