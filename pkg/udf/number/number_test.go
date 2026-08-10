package number

import (
	"fmt"
	"strings"
	"testing"
)

func TestToBaseFromBase(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{`42 | to_base(16)`, "2a"},
		{`255 | to_base(2)`, "11111111"},
		{`"2a" | from_base(16)`, "42"},
		{`"11111111" | from_base(2)`, "255"},
		{`255 | to_hex_number`, "ff"},
		{`"ff" | from_hex_number`, "255"},
		{`"0xFF" | from_hex_number`, "255"},
	}
	for _, tt := range tests {
		got := fmt.Sprint(run(t, tt.query, nil))
		if got != tt.want {
			t.Errorf("%s = %s, want %q", tt.query, got, tt.want)
		}
	}
}

func TestClamp(t *testing.T) {
	tests := []struct {
		query, want string
	}{
		{`5 | clamp(0; 10)`, "5"},
		{`-3 | clamp(0; 10)`, "0"},
		{`99 | clamp(0; 10)`, "10"},
		{`7 | clamp(5; 20)`, "7"},
	}
	for _, tt := range tests {
		if got := fmt.Sprint(run(t, tt.query, nil)); got != tt.want {
			t.Errorf("%s = %s, want %q", tt.query, got, tt.want)
		}
	}
}

func TestGcdLcm(t *testing.T) {
	tests := []struct {
		query, want string
	}{
		{`12 | gcd(18)`, "6"},
		{`35 | gcd(10)`, "5"},
		{`4 | lcm(6)`, "12"},
		{`0 | lcm(5)`, "0"},
		{`21 | lcm(6)`, "42"},
	}
	for _, tt := range tests {
		if got := fmt.Sprint(run(t, tt.query, nil)); got != tt.want {
			t.Errorf("%s = %s, want %q", tt.query, got, tt.want)
		}
	}
}

func TestRoundTo(t *testing.T) {
	if got := fmt.Sprint(run(t, `3.14159 | round_to(2)`, nil)); got != "3.14" {
		t.Errorf("round_to(2) = %s, want 3.14", got)
	}
	if got := fmt.Sprint(run(t, `1234 | round_to(-2)`, nil)); got != "1200" {
		t.Errorf("round_to(-2) = %s, want 1200", got)
	}
}

func TestHumanBytes(t *testing.T) {
	tests := []struct {
		query, want string
	}{
		{`0 | human_bytes`, "0 B"},
		{`1023 | human_bytes`, "1023 B"},
		{`1024 | human_bytes`, "1.0 KiB"},
		{`1048576 | human_bytes`, "1.0 MiB"},
		{`3221225472 | human_bytes`, "3.0 GiB"},
	}
	for _, tt := range tests {
		if got := fmt.Sprint(run(t, tt.query, nil)); got != tt.want {
			t.Errorf("%s = %s, want %q", tt.query, got, tt.want)
		}
	}
}

func TestPercentage(t *testing.T) {
	if got := fmt.Sprint(run(t, `40 | percentage(200)`, nil)); got != "20" {
		t.Errorf("percentage = %s, want 20", got)
	}
}

func TestFromBaseErrors(t *testing.T) {
	err := runErr(t, `"zz" | from_base(16)`)
	if err == nil || !strings.Contains(err.Error(), "cannot parse") {
		t.Errorf("expected a parse error, got %v", err)
	}
}
