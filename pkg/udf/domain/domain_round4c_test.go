package domain

import (
	"testing"
)

func TestNetPresentValue(t *testing.T) {
	got := run(t, `net_present_value([-100, 50, 60]; 0.1)`, nil, RegisterAll()...)
	if f, ok := toF64(got); !ok || !approx(-4.9587, f) {
		t.Errorf("npv = %v, want ~-4.9587", got)
	}
	got = run(t, `net_present_value([-100, 60, 60.5]; 0.1)`, nil, RegisterAll()...)
	if f, _ := toF64(got); f <= 0 {
		t.Errorf("npv positive = %v, want > 0", got)
	}
}
