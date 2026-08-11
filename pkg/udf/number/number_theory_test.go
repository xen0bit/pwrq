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
		{`16 | is_perfect_square`, "true"},
		{`15 | is_perfect_square`, "false"},
		{`8 | is_coprime(15)`, "true"},
		{`8 | is_coprime(12)`, "false"},
		{`13 | next_prime`, "13"},
		{`14 | next_prime`, "17"},
		{`60 | prime_factors`, "[2 2 3 5]"},
		{`12 | proper_divisors`, "[1 2 3 4 6]"},
		{`28 | is_perfect_number`, "true"},
		{`12 | is_perfect_number`, "false"},
		{`10 | euler_totient`, "4"},
	}
	for _, tt := range tests {
		got := fmt.Sprint(run(t, tt.query, nil))
		if got != tt.want {
			t.Errorf("%s = %s, want %s", tt.query, got, tt.want)
		}
	}
}
