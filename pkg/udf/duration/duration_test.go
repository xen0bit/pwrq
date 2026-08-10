package duration

import (
	"fmt"
	"strings"
	"testing"

	"github.com/itchyny/gojq"
)

func run(t *testing.T, query string) any {
	t.Helper()
	q, err := gojq.Parse(query)
	if err != nil {
		t.Fatalf("parse %q: %v", query, err)
	}
	code, err := gojq.Compile(q, RegisterAll()...)
	if err != nil {
		t.Fatalf("compile %q: %v", query, err)
	}
	iter := code.Run(nil)
	v, ok := iter.Next()
	if !ok {
		t.Fatalf("%q produced no result", query)
	}
	if e, isErr := v.(error); isErr {
		t.Fatalf("%q: %v", query, e)
	}
	return v
}

func TestHumanDuration(t *testing.T) {
	tests := []struct {
		query, want string
	}{
		{`0 | human_duration`, "0s"},
		{`45 | human_duration`, "45s"},
		{`90 | human_duration`, "1m 30s"},
		{`3661 | human_duration`, "1h 1m 1s"},
		{`90061 | human_duration`, "1d 1h 1m 1s"},
	}
	for _, tt := range tests {
		if got := fmt.Sprint(run(t, tt.query)); got != tt.want {
			t.Errorf("%s = %s, want %q", tt.query, got, tt.want)
		}
	}
}

func TestParseDuration(t *testing.T) {
	if got := fmt.Sprint(run(t, `"2h30m" | parse_duration`)); got != "9000" {
		t.Errorf("parse_duration = %s, want 9000", got)
	}
	if got := fmt.Sprint(run(t, `"90s" | parse_duration`)); got != "90" {
		t.Errorf("parse_duration = %s, want 90", got)
	}
}

func TestWeekday(t *testing.T) {
	if got := fmt.Sprint(run(t, `"2026-08-10" | weekday`)); got != "Monday" {
		t.Errorf("weekday = %s, want Monday", got)
	}
	if got := fmt.Sprint(run(t, `"1970-01-01" | weekday`)); got != "Thursday" {
		t.Errorf("weekday epoch = %s, want Thursday", got)
	}
}

func TestIsWeekend(t *testing.T) {
	if got := run(t, `"2026-08-08" | is_weekend`); got != true {
		t.Errorf("2026-08-08 is Saturday, got %v", got)
	}
	if got := run(t, `"2026-08-10" | is_weekend`); got != false {
		t.Errorf("2026-08-10 is Monday, got %v", got)
	}
}

func TestTimeAgo(t *testing.T) {
	got := fmt.Sprint(run(t, `0 | time_ago`))
	if !strings.HasSuffix(got, "ago") {
		t.Errorf("time_ago(0) = %q, want it to end in 'ago'", got)
	}
}
