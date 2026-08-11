package json

import (
	"fmt"
	"testing"
)

func TestPathExt(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{`{a: {b: [1, 2]}} | set_path("a.b[1]"; 9)`, "map[a:map[b:[1 9]]]"},
		{`{a: 1} | has_path("a")`, "true"},
		{`{a: 1} | has_path("b")`, "false"},
		{`{a: {b: 1, c: 2}} | del_path("a.b")`, "map[a:map[c:2]]"},
		{`[1, 2, 3] | del_path("[0]")`, "[2 3]"},
	}
	for _, tt := range tests {
		got := fmt.Sprint(run(t, tt.query))
		if got != tt.want {
			t.Errorf("%s = %s, want %s", tt.query, got, tt.want)
		}
	}
}

func TestSetPathRoundTrip(t *testing.T) {
	got := run(t, `{a: {b: 1}} | set_path("a.b"; 2) | get_path("a.b")`)
	if fmt.Sprint(got) != "2" {
		t.Errorf("set_path round-trip = %v", got)
	}
}
