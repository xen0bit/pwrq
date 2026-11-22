package ideengine

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xen0bit/pwrq/pkg/udf"
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
	slots     map[string]string // slot question -> the words "found"
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
	return f.withSlots(sc, Ranking{Probabilities: probs, Continue: f.cont, Threshold: f.threshold}), nil
}

// slots answers every slot question the way a model reading the comment
// would, for the tests that check filling.
func (f *fakeRanker) withSlots(sc SuggestContext, r Ranking) Ranking {
	r.Slots = map[string]string{}
	for _, q := range sc.SlotQuestions {
		if v, ok := f.slots[q]; ok {
			r.Slots[q] = v
		}
	}
	return r
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
		{"", "", "", "", ""}, // nothing written and nothing asked for
		{".items | ", "", PosStage, "", ""},
		{".items | sel", "", PosStage, "sel", ""},
		{".items ", "", PosAfter, "", ""},
		{".", `{"name": 1, "size": 2}`, PosField, "", ""},
		{".na", `{"name": 1, "size": 2}`, PosField, "na", ""},
		{"map(", "", PosArgument, "", "map"},
		{"select(.a) | sort_by(", "", PosArgument, "", "sort_by"},
		{"# list the big files", "", PosScript, "", ""},
		{"# list the big files\n", "", PosScript, "", ""},
		{"# a comment\n.items ", "", PosAfter, "", ""},
		{"get_childitem(\".\")", "", PosAfter, "", ""},
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

// TestCommentWritesTheQuery: a comment and nothing else asks for a whole
// query - the recipe the ranker picks, its slots filled from the comment,
// on the line after the comment, ready for Tab.
func TestCommentWritesTheQuery(t *testing.T) {
	r := &fakeRanker{favourite: "images-containing", prob: 0.95, cont: 1, threshold: 0.5,
		slots: map[string]string{"What should the images contain or show?": "a shrimp", "Which directory or folder?": "current directory"}}
	e := New(Config{Registry: webEngine(Config{}).config.Registry, Ranker: r})
	query := "# Use a decision model to list images in the current directory that contain a shrimp"
	sc := e.SuggestContext(SuggestRequest{Query: query, Cursor: -1})
	if sc.Position != PosScript || sc.Lead != "\n" || len(sc.SlotQuestions) == 0 {
		t.Fatalf("context %+v", sc)
	}
	found := false
	for _, c := range sc.Candidates {
		found = found || c.Name == "images-containing"
	}
	if !found {
		t.Fatalf("the prefilter lost the images recipe: %+v", sc.Candidates)
	}
	resp := e.Suggest(context.Background(), SuggestRequest{Query: query, Cursor: -1})
	want := "\nget_childitem(\".\")\n| select("
	if !strings.HasPrefix(resp.Ghost, want) || !strings.Contains(resp.Ghost, `Does the image contain a shrimp?`) {
		t.Fatalf("ghost %q", resp.Ghost)
	}
	if resp.Items[0].Kind != "recipe" || !strings.Contains(resp.Items[0].Display, "images") {
		t.Fatalf("items %+v", resp.Items)
	}
	// The cursor in the middle of the comment is still writing it.
	if sc := e.SuggestContext(SuggestRequest{Query: query, Cursor: 5}); len(sc.Candidates) != 0 {
		t.Fatalf("suggested mid-comment: %+v", sc.Position)
	}
}

// TestRecipeContinues: written code that starts a recipe is offered the
// recipe's rest.
func TestRecipeContinues(t *testing.T) {
	e := webEngine(Config{})
	sc := e.SuggestContext(SuggestRequest{Query: "# images that contain a cat\nget_childitem(\".\")\n", Cursor: -1})
	if sc.Position != PosAfter {
		t.Fatalf("position %q", sc.Position)
	}
	ok := false
	for _, c := range sc.Candidates {
		if c.Kind == "recipe" && c.Name == "images-containing" && strings.HasPrefix(c.Insert, "| select(.Extension") {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("no continuation among %+v", sc.Candidates)
	}
}

// TestFieldsComeFromTheCmdletsShape: with no input sample, the fields after
// a dot are what the stage before declares it emits.
func TestFieldsComeFromTheCmdletsShape(t *testing.T) {
	e := New(Config{Registry: webEngine(Config{}).config.Registry})
	for _, q := range []string{`get_childitem(".") | select(.`, `[get_childitem(".") | select(.Length > 0)] | sort_by(.`, `get_childitem(".") | .`} {
		sc := e.SuggestContext(SuggestRequest{Query: q, Cursor: -1})
		var names []string
		for _, c := range sc.Candidates {
			names = append(names, c.Name)
		}
		if !strings.Contains(strings.Join(names, " "), "Length") {
			t.Errorf("%s: fields %v", q, names)
		}
	}
}

func TestLexicalSlots(t *testing.T) {
	cases := []struct{ kind, comment, want string }{
		{"number", "top 20 largest files", "20"},
		{"number", "files over 10MB in the last 3 days", ""},
		{"days", "changed in the past 3 days", "3"},
		{"days", "files modified this week", "7"},
		{"size", "anything over 1.5 GB", "1.5 GB"},
		{"ip", "who is 10.0.0.1", "10.0.0.1"},
		{"url", "fetch https://example.com/a.json.", "https://example.com/a.json"},
	}
	for _, c := range cases {
		got, _ := lexicalSlot(c.kind, c.comment)
		if got != c.want {
			t.Errorf("lexicalSlot(%s, %q) = %q, want %q", c.kind, c.comment, got, c.want)
		}
	}
}

// TestRecipeTakesItsFirstNamesPlace: the model picking get_process, with a
// recipe that writes get_process and goes on, offers the recipe.
func TestRecipeTakesItsFirstNamesPlace(t *testing.T) {
	r := &fakeRanker{favourite: "get_process", prob: 0.6, cont: 1, threshold: 0.5}
	e := New(Config{Registry: udf.DefaultRegistry(), Ranker: r})
	resp := e.Suggest(context.Background(), SuggestRequest{Query: "# the processes using the most memory\n[get_pr]", Cursor: len([]rune("# the processes using the most memory\n[get_pr"))})
	if len(resp.Items) == 0 || resp.Items[0].Kind != "recipe" || !strings.HasPrefix(resp.Items[0].Insert, "get_process]\n| sort_by(") {
		t.Fatalf("items %+v", resp.Items)
	}
	if resp.GhostAfter != 1 || !strings.HasPrefix(resp.Ghost, "ocess]\n") {
		t.Fatalf("ghost %q after %d; top %+v", resp.Ghost, resp.GhostAfter, resp.Items[0])
	}
}
