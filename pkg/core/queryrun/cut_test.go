package queryrun

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"
)

func runCapped(t *testing.T, query string, maxValueBytes int) Result {
	t.Helper()
	r := &Runner{}
	return r.Run(context.Background(), &Request{
		Query:         query,
		NullInput:     true,
		Compact:       true,
		MaxResults:    100,
		MaxValueBytes: maxValueBytes,
	})
}

// TestLargeValueIsCutAndSaysSo covers the gap between the two bounds that
// already existed. A single fetched document is one result, well inside a
// thousand-result limit and a 64MB total cap, and it is the shape that put
// nine kilobytes of Unicode samples into a model's context in one call.
func TestLargeValueIsCutAndSaysSo(t *testing.T) {
	res := runCapped(t, `"x" * 5000`, 256)

	if res.Error != "" {
		t.Fatalf("the run failed: %s", res.Error)
	}
	if res.Count != 1 {
		t.Fatalf("count = %d, want 1", res.Count)
	}
	if res.Elided != 1 {
		t.Errorf("elided = %d, want 1", res.Elided)
	}
	// Cutting a value is not stopping a run: the query produced everything it
	// was going to, and a caller that conflates the two re-runs work already
	// done.
	if res.Truncated {
		t.Error("a cut value was reported as a truncated run")
	}

	got := res.Values[0]
	if !strings.Contains(got, "cut ") || !strings.Contains(got, "5002 bytes") {
		t.Errorf("the cut value does not say how much was dropped: %q", tail(got))
	}
	if len(got) > 256+200 {
		t.Errorf("the cut value is %d bytes, well past the 256-byte budget", len(got))
	}
}

// TestValuesInsideTheBudgetAreUntouched keeps the cap from rewriting output
// that was never a problem.
func TestValuesInsideTheBudgetAreUntouched(t *testing.T) {
	res := runCapped(t, `{a: 1, b: "two"}`, 8192)

	if res.Elided != 0 {
		t.Errorf("elided = %d, want 0", res.Elided)
	}
	if got, want := res.Values[0], `{"a":1,"b":"two"}`; got != want {
		t.Errorf("value = %q, want %q", got, want)
	}
}

// TestZeroBudgetMeansUnbounded pins the default for every host that is a
// terminal, where a user can see how much came back and stop reading.
func TestZeroBudgetMeansUnbounded(t *testing.T) {
	res := runCapped(t, `"x" * 5000`, 0)

	if res.Elided != 0 {
		t.Errorf("elided = %d with no budget set, want 0", res.Elided)
	}
	if len(res.Values[0]) < 5000 {
		t.Errorf("an unbounded run cut its value anyway: %d bytes", len(res.Values[0]))
	}
}

// TestTheCutLandsOnARuneBoundary stops the marker being preceded by half a
// UTF-8 sequence, which renders as a replacement character and reads as
// corrupted data rather than as the truncation being announced.
func TestTheCutLandsOnARuneBoundary(t *testing.T) {
	// A budget chosen to fall inside a multi-byte rune.
	for budget := 40; budget < 60; budget++ {
		res := runCapped(t, `"→" * 200`, budget)
		if res.Elided != 1 {
			t.Fatalf("budget %d: elided = %d, want 1", budget, res.Elided)
		}
		if !utf8.ValidString(res.Values[0]) {
			t.Errorf("budget %d cut mid-rune: %q", budget, res.Values[0])
		}
	}
}

// TestEachOversizedValueIsCounted checks the count is per value rather than
// per run, since that is what tells a caller how much of the answer it is
// looking at.
func TestEachOversizedValueIsCounted(t *testing.T) {
	res := runCapped(t, `("x" * 500), "small", ("y" * 500)`, 100)

	if res.Count != 3 {
		t.Fatalf("count = %d, want 3", res.Count)
	}
	if res.Elided != 2 {
		t.Errorf("elided = %d, want 2", res.Elided)
	}
	if res.Values[1] != `"small"` {
		t.Errorf("the value inside the budget was rewritten: %q", res.Values[1])
	}
}

func tail(s string) string {
	if len(s) <= 120 {
		return s
	}
	return "..." + s[len(s)-120:]
}
