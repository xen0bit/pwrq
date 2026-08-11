package number

import (
	"fmt"
	"testing"
)

func TestNumberTheory(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{`-5 | sign`, "-1"},
		{`0 | sign`, "0"},
		{`5 | sign`, "1"},
		{`13 | next_prime`, "13"},
		{`14 | next_prime`, "17"},
		{`60 | prime_factors`, "[2 2 3 5]"},
	}
	for _, tt := range tests {
		got := fmt.Sprint(run(t, tt.query, nil))
		if got != tt.want {
			t.Errorf("%s = %s, want %s", tt.query, got, tt.want)
		}
	}
}
