package ideengine

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeRanker prefers the candidate named by favourite, and records what it
// was asked.
type fakeRanker struct {
	mu        sync.Mutex
	favourite string
	prob      float64
	cont      float64
	threshold float64
	err       error
	delay     time.Duration
	seen      []SuggestContext
	warmed    int
}

func (f *fakeRanker) Rank(ctx context.Context, sc SuggestContext) (Ranking, error) {
	f.mu.Lock()
	f.seen = append(f.seen, sc)
	delay := f.delay
	f.mu.Unlock()
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return Ranking{}, ctx.Err()
		}
	}
	if f.err != nil {
		return Ranking{}, f.err
	}
	probs := make([]float64, len(sc.Candidates))
	for i, c := range sc.Candidates {
		if c.Name == f.favourite {
			probs[i] = f.prob
		} else {
			probs[i] = (1 - f.prob) / float64(len(sc.Candidates))
		}
	}
	return Ranking{Probabilities: probs, Continue: f.cont, Threshold: f.threshold}, nil
}

func (f *fakeRanker) Status() SuggestStatus {
	return SuggestStatus{State: "ready", Fraction: -1, Device: "cpu"}
}
func (f *fakeRanker) Warm() { f.mu.Lock(); f.warmed++; f.mu.Unlock() }

func TestSuggestUnavailableWithoutARanker(t *testing.T) {
	resp := webEngine(Config{}).Suggest(context.Background(), SuggestRequest{Query: ".a | "})
	if resp.Unavailable == "" || resp.Status.State != "unavailable" || resp.Items == nil {
		t.Fatalf("got %+v", resp)
	}
	var decoded SuggestResponse
	if err := json.Unmarshal([]byte(webEngine(Config{}).Call(context.Background(), "suggest", `{"query":"."}`)), &decoded); err != nil || decoded.Unavailable == "" {
		t.Fatalf("Call: %+v, %v", decoded, err)
	}
}

func TestSuggestPositions(t *testing.T) {
	for _, c := range []struct {
		query, input, position, partial, call string
	}{
		{"", "", PosStage, "", ""},
		{".items | ", "", PosStage, "", ""},
		{".items | sel", "", PosStage, "sel", ""},
		{".items ", "", PosAfter, "", ""},
		{".", `{"name": 1, "size": 2}`, PosField, "", ""},
		{".na", `{"name": 1, "size": 2}`, PosField, "na", ""},
		{"map(", "", PosArgument, "", "map"},
		{"select(.a) | sort_by(", "", PosArgument, "", "sort_by"},
		{"# just prose here", "", "", "", ""},
		{`"inside a string `, "", "", "", ""},
	} {
		sc := webEngine(Config{}).suggestContext(SuggestRequest{Query: c.query, Cursor: -1, Input: c.input})
		if sc.Position != c.position || sc.Partial != c.partial || sc.Call != c.call {
			t.Errorf("%q: position %q partial %q call %q; want %q %q %q", c.query, sc.Position, sc.Partial, sc.Call, c.position, c.partial, c.call)
		}
	}
}

// TestCommentsSteerTheCandidates: the comment above the cursor is the
// author's intent, and the candidate whose help matches it comes first.
func TestCommentsSteerTheCandidates(t *testing.T) {
	e := webEngine(Config{})
	sc := e.suggestContext(SuggestRequest{Query: "# sort the rows by a computed key\n.rows | ", Cursor: -1})
	if len(sc.Intent) != 1 || !strings.Contains(sc.Intent[0], "sort the rows") {
		t.Fatalf("intent %q", sc.Intent)
	}
	if len(sc.Candidates) == 0 || len(sc.Candidates) > maxCandidates {
		t.Fatalf("%d candidates", len(sc.Candidates))
	}
	top := []string{}
	for _, c := range sc.Candidates[:3] {
		top = append(top, c.Name)
	}
	if !strings.Contains(strings.Join(top, " "), "sort") {
		t.Errorf("top candidates %v for a sorting comment", top)
	}
	for _, c := range sc.Candidates {
		if c.Help == "" {
			t.Errorf("%s has no help for the model to read", c.Name)
		}
	}
}

func TestSuggestFieldsComeFromTheInput(t *testing.T) {
	sc := webEngine(Config{}).suggestContext(SuggestRequest{Query: ".", Cursor: -1, Input: `[{"Name": "a", "Length": 3, "odd key": 1}]`})
	var names []string
	for _, c := range sc.Candidates {
		names = append(names, c.Insert)
	}
	got := strings.Join(names, ",")
	for _, want := range []string{"Name", "Length", `"odd key"`} {
		if !strings.Contains(got, want) {
			t.Errorf("fields %s lack %s", got, want)
		}
	}
}

func TestSuggestArgumentsComeFromExamples(t *testing.T) {
	sc := webEngine(Config{}).suggestContext(SuggestRequest{Query: "chunks(", Cursor: -1})
	if len(sc.Candidates) == 0 || sc.Candidates[0].Kind != "argument" || !strings.HasSuffix(sc.Candidates[0].Insert, ")") {
		t.Fatalf("candidates %+v", sc.Candidates)
	}
}

func TestGhostNeedsConfidenceUnlessAsked(t *testing.T) {
	r := &fakeRanker{favourite: "select", prob: 0.9, cont: 0.9, threshold: 0.7}
	e := webEngine(Config{Ranker: r})
	resp := e.Suggest(context.Background(), SuggestRequest{Query: ".items | sel", Cursor: -1})
	if resp.Ghost != "ect(" || resp.Items[0].Display != "select" || resp.Items[0].Replace != 3 || resp.Items[0].Insert != "select(" {
		t.Fatalf("got %+v", resp)
	}
	r.prob = 0.5 // 0.5 * 0.9 is under the bar
	resp = e.Suggest(context.Background(), SuggestRequest{Query: ".items | sel", Cursor: -1})
	if resp.Ghost != "" || len(resp.Items) == 0 {
		t.Fatalf("an unsure suggestion became ghost text: %+v", resp)
	}
	resp = e.Suggest(context.Background(), SuggestRequest{Query: ".items | sel", Cursor: -1, Explicit: true})
	if resp.Ghost != "ect(" {
		t.Fatalf("asking for a suggestion did not show one: %+v", resp)
	}
	after := e.Suggest(context.Background(), SuggestRequest{Query: ".items ", Cursor: -1, Explicit: true})
	if !strings.HasPrefix(after.Ghost, "| ") {
		t.Fatalf("after a finished stage the suggestion should start a new one: %+v", after)
	}
}

func TestSuggestFallsBackToLexicalOrder(t *testing.T) {
	r := &fakeRanker{err: errors.New("model failed")}
	resp := webEngine(Config{Ranker: r}).Suggest(context.Background(), SuggestRequest{Query: "# count them\n.[] | ", Cursor: -1})
	if resp.Ranked || resp.Ghost != "" || len(resp.Items) == 0 || resp.Error == "" {
		t.Fatalf("got %+v", resp)
	}
}

// TestNewerSuggestionSupersedesOlder: typing on cancels the suggestion for
// the text that is no longer there.
func TestNewerSuggestionSupersedesOlder(t *testing.T) {
	r := &fakeRanker{favourite: "select", prob: 0.9, cont: 1, threshold: 0.5, delay: 2 * time.Second}
	e := webEngine(Config{Ranker: r})
	done := make(chan SuggestResponse, 1)
	go func() { done <- e.Suggest(context.Background(), SuggestRequest{Query: ".a | s", Cursor: -1}) }()
	time.Sleep(100 * time.Millisecond)
	r.mu.Lock()
	r.delay = 0
	r.mu.Unlock()
	second := e.Suggest(context.Background(), SuggestRequest{Query: ".a | se", Cursor: -1})
	select {
	case first := <-done:
		if first.Error != "superseded" {
			t.Errorf("the older suggestion finished: %+v", first)
		}
	case <-time.After(time.Second):
		t.Fatal("the older suggestion was not cancelled")
	}
	if second.Ghost == "" {
		t.Errorf("the newer suggestion has no ghost: %+v", second)
	}
}

func TestSuggestStatusWarmsTheRanker(t *testing.T) {
	r := &fakeRanker{}
	st := webEngine(Config{Ranker: r}).SuggestStatus()
	if st.State != "ready" || r.warmed != 1 {
		t.Fatalf("status %+v, warmed %d", st, r.warmed)
	}
}
