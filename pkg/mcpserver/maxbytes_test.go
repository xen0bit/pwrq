package mcpserver

import (
	"strings"
	"testing"
)

// TestOneHugeValueIsCappedByDefault is the case neither existing bound
// covered. A fetched document is one result, so a thousand-result limit never
// sees it and a 64MB total cap never sees it, and in a recorded session one
// such fetch put nine kilobytes of Unicode samples into the caller's context
// in a single call.
func TestOneHugeValueIsCappedByDefault(t *testing.T) {
	res := runQuery(t, newTestClient(t, NewServer("test")), runQueryArgs{Query: `"x" * 40000`, NullInput: true})

	if res.Error != "" {
		t.Fatalf("the run failed: %s", res.Error)
	}
	if res.Elided != 1 {
		t.Fatalf("elided = %d, want 1: the default budget did not apply", res.Elided)
	}
	if len(res.Values[0]) > defaultMaxValueBytes+200 {
		t.Errorf("the value came back at %d bytes, past the %d-byte default",
			len(res.Values[0]), defaultMaxValueBytes)
	}
}

// TestMaxBytesRaisesTheBudget checks a caller that means to read the whole
// thing can say so, which is what makes the default defensible.
func TestMaxBytesRaisesTheBudget(t *testing.T) {
	res := runQuery(t, newTestClient(t, NewServer("test")), runQueryArgs{Query: `"x" * 40000`, NullInput: true, MaxBytes: 100000})

	if res.Elided != 0 {
		t.Errorf("elided = %d with maxBytes raised, want 0", res.Elided)
	}
	if len(res.Values[0]) < 40000 {
		t.Errorf("the value was cut anyway, at %d bytes", len(res.Values[0]))
	}
}

// TestOrdinaryResultsAreUnaffected keeps the cap out of the way of the
// overwhelming majority of runs.
func TestOrdinaryResultsAreUnaffected(t *testing.T) {
	res := runQuery(t, newTestClient(t, NewServer("test")), runQueryArgs{Query: `{a: 1} | .a`, NullInput: true})

	if res.Elided != 0 {
		t.Errorf("elided = %d for a one-byte result, want 0", res.Elided)
	}
	if res.Values[0] != "1" {
		t.Errorf("value = %q, want 1", res.Values[0])
	}
}

// TestSummarySeparatesCuttingFromStopping checks the text tells a caller which
// of the two happened. A cut value means the query finished and part of one
// result is hidden; a truncated run means the query did not finish. A caller
// that reads the first as the second re-runs work already done.
func TestSummarySeparatesCuttingFromStopping(t *testing.T) {
	res := runQuery(t, newTestClient(t, NewServer("test")), runQueryArgs{Query: `"x" * 40000`, NullInput: true})
	text := summarize(res)

	if !strings.Contains(text, "shown in part") {
		t.Errorf("the summary does not report the cut:\n%s", tailOf(text))
	}
	if strings.Contains(text, "stopped after") {
		t.Errorf("the summary reports a cut value as a stopped run:\n%s", tailOf(text))
	}
	if res.Truncated {
		t.Error("a cut value set the truncated flag")
	}
}

func tailOf(s string) string {
	if len(s) <= 200 {
		return s
	}
	return "..." + s[len(s)-200:]
}
