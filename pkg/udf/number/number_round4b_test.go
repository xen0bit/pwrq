package number

import (
	"fmt"
	"testing"
)

func TestNumberExtras(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{`3.14159 | to_fixed(2)`, "3.14"},
		{`2 | to_fixed(3)`, "2.000"},
		{`16 | is_power_of_two`, "true"},
		{`12 | is_power_of_two`, "false"},
	}
	for _, tt := range tests {
		got := fmt.Sprint(run(t, tt.query, nil))
		if got != tt.want {
			t.Errorf("%s = %s, want %s", tt.query, got, tt.want)
		}
	}
}
