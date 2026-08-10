package number

import (
	"fmt"
	"testing"
)

func TestFactorial(t *testing.T) {
	if got := fmt.Sprint(run(t, `5 | factorial`, nil)); got != "120" {
		t.Errorf("factorial(5) = %s", got)
	}
	if got := fmt.Sprint(run(t, `0 | factorial`, nil)); got != "1" {
		t.Errorf("factorial(0) = %s", got)
	}
	if got := fmt.Sprint(run(t, `25 | factorial`, nil)); got != "15511210043330985984000000" {
		t.Errorf("factorial(25) = %s", got)
	}
}

func TestIsPrime(t *testing.T) {
	for n, want := range map[int]bool{2: true, 3: true, 4: false, 13: true, 15: false, 97: true, 100: false, 1: false} {
		if got := run(t, fmt.Sprintf(`%d | is_prime`, n), nil); got != want {
			t.Errorf("is_prime(%d) = %v, want %v", n, got, want)
		}
	}
}

func TestFibonacci(t *testing.T) {
	if got := fmt.Sprint(run(t, `0 | fibonacci`, nil)); got != "0" {
		t.Errorf("fib(0) = %s", got)
	}
	if got := fmt.Sprint(run(t, `1 | fibonacci`, nil)); got != "1" {
		t.Errorf("fib(1) = %s", got)
	}
	if got := fmt.Sprint(run(t, `10 | fibonacci`, nil)); got != "55" {
		t.Errorf("fib(10) = %s", got)
	}
	if got := fmt.Sprint(run(t, `100 | fibonacci`, nil)); got != "354224848179261915075" {
		t.Errorf("fib(100) = %s", got)
	}
}

func TestCombinations(t *testing.T) {
	if got := fmt.Sprint(run(t, `5 | combinations_count(2)`, nil)); got != "10" {
		t.Errorf("C(5,2) = %s", got)
	}
	if got := fmt.Sprint(run(t, `5 | permutations_count(2)`, nil)); got != "20" {
		t.Errorf("P(5,2) = %s", got)
	}
	if got := fmt.Sprint(run(t, `52 | combinations_count(5)`, nil)); got != "2598960" {
		t.Errorf("C(52,5) = %s", got)
	}
}

func TestOrdinal(t *testing.T) {
	for in, want := range map[string]string{
		`1 | ordinal`: "1st", `2 | ordinal`: "2nd", `3 | ordinal`: "3rd",
		`4 | ordinal`: "4th", `11 | ordinal`: "11th", `12 | ordinal`: "12th",
		`21 | ordinal`: "21st", `22 | ordinal`: "22nd", `111 | ordinal`: "111th",
	} {
		if got := fmt.Sprint(run(t, in, nil)); got != want {
			t.Errorf("%s = %s, want %s", in, got, want)
		}
	}
}

func TestLerp(t *testing.T) {
	if got := fmt.Sprint(run(t, `0 | lerp(10; 0.5)`, nil)); got != "5" {
		t.Errorf("lerp(0;10;0.5) = %s", got)
	}
	if got := fmt.Sprint(run(t, `0 | lerp(10; 1)`, nil)); got != "10" {
		t.Errorf("lerp(0;10;1) = %s", got)
	}
}

func TestHumanNumber(t *testing.T) {
	tests := []struct {
		query, want string
	}{
		{`999 | human_number`, "999"},
		{`1500 | human_number`, "1.5k"},
		{`1234567 | human_number`, "1.2M"},
		{`3200000000 | human_number`, "3.2B"},
	}
	for _, tt := range tests {
		if got := fmt.Sprint(run(t, tt.query, nil)); got != tt.want {
			t.Errorf("%s = %s, want %q", tt.query, got, tt.want)
		}
	}
}

func TestEvenOdd(t *testing.T) {
	if got := run(t, `4 | is_even`, nil); got != true {
		t.Error("4 is_even = false")
	}
	if got := run(t, `3 | is_odd`, nil); got != true {
		t.Error("3 is_odd = false")
	}
}
