package number

import (
	"fmt"
	"testing"
)

func TestNumberWordsAndFormat(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{`1234 | to_words`, "one thousand two hundred thirty-four"},
		{`0 | to_words`, "zero"},
		{`2026 | roman_numeral`, "MMXXVI"},
		{`4 | roman_numeral`, "IV"},
		{`1234567 | group_digits`, "1,234,567"},
		{`-1234567 | group_digits`, "-1,234,567"},
		{`1234.5 | format_currency`, "$1,234.50"},
		{`1234.5 | format_currency("€")`, "€1,234.50"},
		{`6 | collatz_steps`, "8"},
	}
	for _, tt := range tests {
		got := fmt.Sprint(run(t, tt.query, nil))
		if got != tt.want {
			t.Errorf("%s = %s, want %s", tt.query, got, tt.want)
		}
	}
}
