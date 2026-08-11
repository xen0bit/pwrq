package duration

import (
	"fmt"
	"strings"
	"testing"

	"github.com/itchyny/gojq"
)

func zrun(t *testing.T, query string) any {
	t.Helper()
	q, err := gojq.Parse(query)
	if err != nil {
		t.Fatalf("parse %q: %v", query, err)
	}
	code, err := gojq.Compile(q, RegisterAll()...)
	if err != nil {
		t.Fatalf("compile %q: %v", query, err)
	}
	v, ok := code.Run(nil).Next()
	if !ok {
		t.Fatalf("%q produced no result", query)
	}
	if e, isErr := v.(error); isErr {
		t.Fatalf("%q: %v", query, e)
	}
	return v
}

func TestToTimezone(t *testing.T) {
	got := zrun(t, `"2026-08-11T12:00:00Z" | to_timezone("Asia/Tokyo")`).(map[string]any)
	if got["DateTime"] != "2026-08-11T21:00:00+09:00" {
		t.Errorf("DateTime = %v", got["DateTime"])
	}
	if got["Offset"] != "+09:00" {
		t.Errorf("Offset = %v", got["Offset"])
	}
	if fmt.Sprint(got["OffsetSeconds"]) != "32400" {
		t.Errorf("OffsetSeconds = %v", got["OffsetSeconds"])
	}

	// The instant is unchanged by the zone: that is the property that makes
	// this a conversion rather than a relabelling.
	utc := zrun(t, `"2026-08-11T12:00:00Z" | to_timezone("UTC")`).(map[string]any)
	if fmt.Sprint(utc["Timestamp"]) != fmt.Sprint(got["Timestamp"]) {
		t.Errorf("timestamps differ across zones: %v vs %v", utc["Timestamp"], got["Timestamp"])
	}
}

func TestToTimezoneRejectsUnknownZone(t *testing.T) {
	q, _ := gojq.Parse(`"2026-08-11T12:00:00Z" | to_timezone("Mars/Olympus")`)
	code, _ := gojq.Compile(q, RegisterAll()...)
	v, _ := code.Run(nil).Next()
	err, isErr := v.(error)
	if !isErr || !strings.Contains(err.Error(), "unknown time zone") {
		t.Errorf("expected an unknown-zone error, got %v", v)
	}
}

func TestFormatDate(t *testing.T) {
	tests := []struct{ query, want string }{
		{`"2026-08-11T12:00:00Z" | format_date("date")`, "2026-08-11"},
		{`"2026-08-11T12:00:00Z" | format_date("time")`, "12:00:00"},
		{`"2026-08-11T12:00:00Z" | format_date("datetime")`, "2026-08-11 12:00:00"},
		{`"2026-08-11T12:00:00Z" | format_date("http")`, "Tue, 11 Aug 2026 12:00:00 GMT"},
		{`"2026-08-11T12:00:00Z" | format_date("2006/01/02")`, "2026/08/11"},
		{`"2026-08-11T12:00:00Z" | format_date("date"; "Asia/Tokyo")`, "2026-08-11"},
		{`"2026-08-11T23:00:00Z" | format_date("date"; "Asia/Tokyo")`, "2026-08-12"},
	}
	for _, tt := range tests {
		if got := fmt.Sprint(zrun(t, tt.query)); got != tt.want {
			t.Errorf("%s = %q, want %q", tt.query, got, tt.want)
		}
	}
}

func TestParseDate(t *testing.T) {
	tests := []struct{ query, want string }{
		{`"11/08/2026" | parse_date("02/01/2006")`, "2026-08-11T00:00:00Z"},
		{`"08/11/2026" | parse_date("01/02/2006")`, "2026-08-11T00:00:00Z"},
		{`"2026-08-11 09:30:00" | parse_date("datetime")`, "2026-08-11T09:30:00Z"},
	}
	for _, tt := range tests {
		if got := fmt.Sprint(zrun(t, tt.query)); got != tt.want {
			t.Errorf("%s = %q, want %q", tt.query, got, tt.want)
		}
	}
}

// TestParseDateIsTheInverseOfFormatDate is the reason both exist: jq can render
// an instant but cannot read a non-ISO one back.
func TestParseDateRoundTrip(t *testing.T) {
	got := zrun(t, `"2026-08-11T12:00:00Z" | format_date("02/01/2006") | parse_date("02/01/2006") | format_date("date")`)
	if fmt.Sprint(got) != "2026-08-11" {
		t.Errorf("round trip = %v", got)
	}
}

func TestParseDateZoneShiftsTheInstant(t *testing.T) {
	utc := fmt.Sprint(zrun(t, `"2026-08-11 09:30:00" | parse_date("datetime")`))
	berlin := fmt.Sprint(zrun(t, `"2026-08-11 09:30:00" | parse_date("datetime"; "Europe/Berlin")`))
	if utc == berlin {
		t.Errorf("zone ignored: both parsed to %s", utc)
	}
}

func TestListTimezones(t *testing.T) {
	got := zrun(t, `list_timezones("Europe/Lon")`).([]any)
	var names []string
	for _, n := range got {
		names = append(names, n.(string))
	}
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, "Europe/London") {
		t.Errorf("Europe/London missing from %v", names)
	}
	// The right/ and posix/ variants resolve but are noise in a listing.
	for _, n := range names {
		if strings.HasPrefix(n, "right/") || strings.HasPrefix(n, "posix/") {
			t.Errorf("tzdata variant %q should not be listed", n)
		}
	}
	// Every name listed must actually resolve.
	for _, n := range names {
		if _, err := zoneOf(n); err != nil {
			t.Errorf("listed zone %q does not resolve: %v", n, err)
		}
	}
}
