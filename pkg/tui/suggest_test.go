package tui

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/xen0bit/pwrq/pkg/ideengine"
)

// The TUI's tests never load the real model: suggestions are off unless a
// test hands the TUI a ranker of its own.
func TestMain(m *testing.M) {
	_ = os.Setenv("PWRQ_SUGGEST", "off")
	os.Exit(m.Run())
}

// sureRanker is confident in sort_by after a pipe.
type sureRanker struct{}

func (sureRanker) Rank(_ context.Context, sc ideengine.SuggestContext) (ideengine.Ranking, error) {
	p := make([]float64, len(sc.Candidates))
	for i, c := range sc.Candidates {
		if c.Name == "sort_by" {
			p[i] = 0.9
		}
	}
	return ideengine.Ranking{Probabilities: p, Continue: 1, Threshold: 0.5}, nil
}
func (sureRanker) Status() ideengine.SuggestStatus {
	return ideengine.SuggestStatus{State: "ready", Fraction: -1, Device: "cpu"}
}
func (sureRanker) Warm() {}

func TestGhostSuggestionAndTab(t *testing.T) {
	h := newHarness(t, Options{Query: " ", Ranker: sureRanker{}})
	h.press("ctrl+a", "ctrl+k")
	h.typeText("# order by size\n")
	h.typeText(".files | ")
	h.until("a ghost suggestion", func() bool { return h.m.query.Ghost != "" })
	if h.m.query.Ghost != "sort_by(" {
		t.Fatalf("ghost %q", h.m.query.Ghost)
	}
	if f := h.frame(); !strings.Contains(f, "sort_by(") || !strings.Contains(f, "Tab ⇥ sort_by 90%") {
		t.Fatalf("the ghost is not drawn:\n%s", f)
	}
	h.press("tab")
	if got := h.m.query.Value(); !strings.HasSuffix(got, ".files | sort_by(") || h.m.focus != paneQuery {
		t.Fatalf("Tab did not accept the ghost: %q (focus %v)", got, h.m.focus)
	}
	// With no ghost, Tab moves between panes as it always did.
	h.m.query.Ghost = ""
	h.press("tab")
	if h.m.focus == paneQuery {
		t.Fatal("Tab without a ghost did not move focus")
	}
}

func TestTypingDropsTheGhost(t *testing.T) {
	h := newHarness(t, Options{Query: " ", Ranker: sureRanker{}})
	h.press("ctrl+a", "ctrl+k")
	h.typeText(".files | ")
	h.until("a ghost suggestion", func() bool { return h.m.query.Ghost != "" })
	h.typeText("m")
	if h.m.query.Ghost != "" {
		t.Fatalf("an edit kept a ghost for the old text: %q", h.m.query.Ghost)
	}
	h.press("esc")
	if h.m.query.Ghost != "" {
		t.Fatal("Esc did not dismiss the ghost")
	}
}

func TestSuggestionsOffShowNothing(t *testing.T) {
	h := newHarness(t, Options{Query: ".a | "})
	h.settle()
	if h.m.sugg.enabled || h.m.query.Ghost != "" || strings.Contains(h.frame(), "d1") {
		t.Fatalf("suggestions are off but showed:\n%s", h.frame())
	}
}
