package validate

import (
	"fmt"
	"testing"
)

func TestSemver(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{`semver_compare("1.2.3"; "1.2.3")`, "0"},
		{`semver_compare("1.2.3"; "2.0.0")`, "-1"},
		{`semver_compare("2.0.0"; "1.9.9")`, "1"},
		{`semver_compare("1.0.0-rc.1"; "1.0.0")`, "-1"},
		{`semver_compare("1.0.0-alpha"; "1.0.0-beta")`, "-1"},
		{`semver_compare("1.0.0-alpha.2"; "1.0.0-alpha.10")`, "-1"},
	}
	for _, tt := range tests {
		got := fmt.Sprint(run(t, tt.query))
		if got != tt.want {
			t.Errorf("%s = %s, want %s", tt.query, got, tt.want)
		}
	}

	parts := run(t, `"2.1.3-rc.1+build5" | semver_parts`)
	m, ok := parts.(map[string]any)
	if !ok {
		t.Fatalf("semver_parts = %T", parts)
	}
	if fmt.Sprint(m["major"]) != "2" || fmt.Sprint(m["minor"]) != "1" || fmt.Sprint(m["patch"]) != "3" ||
		m["prerelease"] != "rc.1" || m["build"] != "build5" {
		t.Errorf("semver_parts = %v", m)
	}
}

func TestNewValidators(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{`"ff00" | is_hex`, "true"},
		{`"0x1F" | is_hex`, "true"},
		{`"hello" | is_hex`, "false"},
		{`"10.0.0.0/8" | is_cidr`, "true"},
		{`"10.0.0.0" | is_cidr`, "false"},
		{`443 | is_port`, "true"},
		{`0 | is_port`, "false"},
		{`"65536" | is_port`, "false"},
		{`"2026-08-10" | is_date`, "true"},
		{`"2026-13-40" | is_date`, "false"},
		{`"2026-08-10T12:34:56Z" | is_iso8601`, "true"},
		{`"2026-08-10" | is_iso8601`, "true"},
		{`"hello" | is_iso8601`, "false"},
		{`"my-cool-slug-2" | is_slug`, "true"},
		{`"Hello World" | is_slug`, "false"},
		{`"born 2024-01-02, died 2025-03-04" | extract_dates`, "[2024-01-02 2025-03-04]"},
	}
	for _, tt := range tests {
		got := fmt.Sprint(run(t, tt.query))
		if got != tt.want {
			t.Errorf("%s = %s, want %s", tt.query, got, tt.want)
		}
	}
}
