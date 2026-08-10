package number

import (
	"fmt"
	"testing"
)

func TestRescale(t *testing.T) {
	if got := fmt.Sprint(run(t, `5 | rescale(0; 10; 0; 100)`, nil)); got != "50" {
		t.Errorf("rescale = %s", got)
	}
	if got := fmt.Sprint(run(t, `0 | rescale(0; 10; 100; 200)`, nil)); got != "100" {
		t.Errorf("rescale zero = %s", got)
	}
}

func TestPctChange(t *testing.T) {
	if got := fmt.Sprint(run(t, `100 | pct_change(120)`, nil)); got != "20" {
		t.Errorf("pct_change = %s, want 20", got)
	}
	if got := fmt.Sprint(run(t, `100 | pct_change(80)`, nil)); got != "-20" {
		t.Errorf("pct_change down = %s, want -20", got)
	}
}

func TestDigitSum(t *testing.T) {
	if got := fmt.Sprint(run(t, `1234 | digit_sum`, nil)); got != "10" {
		t.Errorf("digit_sum = %s", got)
	}
	if got := fmt.Sprint(run(t, `0 | digit_sum`, nil)); got != "0" {
		t.Errorf("digit_sum zero = %s", got)
	}
}

func TestHammingWeight(t *testing.T) {
	if got := fmt.Sprint(run(t, `0 | hamming_weight`, nil)); got != "0" {
		t.Errorf("hamming_weight 0 = %s", got)
	}
	if got := fmt.Sprint(run(t, `7 | hamming_weight`, nil)); got != "3" {
		t.Errorf("hamming_weight 7 = %s", got)
	}
	if got := fmt.Sprint(run(t, `255 | hamming_weight`, nil)); got != "8" {
		t.Errorf("hamming_weight 255 = %s", got)
	}
}
