package duration

import (
	"fmt"
	"testing"
)

func TestIsoDuration(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{`0 | iso_duration`, "PT0S"},
		{`45 | iso_duration`, "PT45S"},
		{`90000 | iso_duration`, "P1DT1H"},
		{`93784 | iso_duration`, "P1DT2H3M4S"},
	}
	for _, tt := range tests {
		got := fmt.Sprint(run(t, tt.query))
		if got != tt.want {
			t.Errorf("%s = %s, want %s", tt.query, got, tt.want)
		}
	}
}
