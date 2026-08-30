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

// TestDurationRoundTripsThroughEveryWriter is the guard the Duration category
// did not have. It writes durations three ways and reads all three back, so a
// change to any one writer that its reader cannot follow fails here rather
// than in a caller's pipeline.
//
// The bug it pins: iso_duration and human_duration both produced strings
// parse_duration rejected, with Go's own parser supplying the message. A model
// that ran `93784 | iso_duration | parse_duration` was told "invalid duration
// P1DT2H3M4S" about a string pwrq had written one stage earlier.
func TestDurationRoundTripsThroughEveryWriter(t *testing.T) {
	for _, seconds := range []int{0, 1, 59, 60, 3661, 86400, 93784, 604800} {
		for _, writer := range []string{"iso_duration", "human_duration"} {
			query := fmt.Sprintf("%d | %s | parse_duration", seconds, writer)
			t.Run(query, func(t *testing.T) {
				got := toFloat(t, query)
				if int(got) != seconds {
					t.Errorf("%s = %v, want %d", query, got, seconds)
				}
			})
		}
	}
}

// TestParseDurationReadsEveryGrammar covers the forms a caller arrives with
// that no pwrq cmdlet writes.
func TestParseDurationReadsEveryGrammar(t *testing.T) {
	for _, tc := range []struct {
		text string
		want float64
	}{
		{"2h30m", 9000},
		{"90s", 90},
		{"1.5h", 5400},
		{"PT1H30M", 5400},
		{"pt1h30m", 5400},
		{"P1W", 604800},
		{"P1DT2H3M4S", 93784},
		{"1d 2h 3m 4s", 93784},
		{"-PT1H", -3600},
	} {
		t.Run(tc.text, func(t *testing.T) {
			query := fmt.Sprintf("%q | parse_duration", tc.text)
			if got := toFloat(t, query); got != tc.want {
				t.Errorf("%s = %v, want %v", query, got, tc.want)
			}
		})
	}
}

// TestParseDurationRefusesACalendarMonth checks that the one ambiguous case
// stays an error. "P1M" is a month, and a month is not a fixed number of
// seconds; answering it would mean silently choosing a length.
func TestParseDurationRefusesACalendarMonth(t *testing.T) {
	for _, text := range []string{"P1Y", "P1M", "P3Y6M"} {
		if _, err := runErr(`"` + text + `" | parse_duration`); err == nil {
			t.Errorf("%q | parse_duration succeeded; a calendar month has no fixed length", text)
		}
	}
}

// runErr evaluates a query and returns its error rather than failing the test,
// for the cases where the error is the thing being checked.
func runErr(query string) (any, error) {
	q, err := gojq.Parse(query)
	if err != nil {
		return nil, err
	}
	code, err := gojq.Compile(q, RegisterAll()...)
	if err != nil {
		return nil, err
	}
	v, ok := code.Run(nil).Next()
	if !ok {
		return nil, fmt.Errorf("%q produced no result", query)
	}
	if e, isErr := v.(error); isErr {
		return nil, e
	}
	return v, nil
}
