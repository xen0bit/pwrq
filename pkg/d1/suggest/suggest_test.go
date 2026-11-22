package suggest

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/xen0bit/pwrq/pkg/d1"
	"github.com/xen0bit/pwrq/pkg/d1/internal/testdata"
	"github.com/xen0bit/pwrq/pkg/d1/prompt"
	"github.com/xen0bit/pwrq/pkg/d1/tokenizer"
	"github.com/xen0bit/pwrq/pkg/ideengine"
)

// TestHarnessMatchesPrompt is the calibration guard: the embedded harness
// was fitted to one prompt, and a change to the wording or the layout
// without re-running `make d1.calibrate` would ship thresholds fitted to a
// prompt that no longer exists.
func TestHarnessMatchesPrompt(t *testing.T) {
	h := DefaultHarness()
	if h.PromptDigest != h.Digest() {
		t.Fatalf("harness.json was calibrated for prompt %q, the prompt is now %q: run `make d1.calibrate`", h.PromptDigest, h.Digest())
	}
	if h.Threshold <= 0 || h.Threshold >= 1 || h.Temperature <= 0 {
		t.Fatalf("implausible harness %+v", h)
	}
}

func sample() ideengine.SuggestContext {
	return ideengine.SuggestContext{
		Before: "# biggest files first\nget_childitem(\"*.go\") | ", Intent: []string{"biggest files first"},
		Position: ideengine.PosAfter, Input: `{"Name":"a.go","Length":10}`,
		Candidates: []ideengine.Candidate{
			{Name: "sort_object", Kind: "cmdlet", Insert: "| sort_object(", Help: "Sort objects by a property", Example: `sort_object(.Length)`},
			{Name: "select", Kind: "builtin", Insert: "| select(", Help: "Keep the input only when the condition is true"},
			{Name: "select", Kind: "field", Insert: "| select", Help: "a field"},
		},
	}
}

// TestPromptIsWellFormed builds the real rows for a suggestion prompt with
// d1-2B's tokenizer: the questions parse, duplicate names are made unique,
// and help text reaches the options.
func TestPromptIsWellFormed(t *testing.T) {
	sc := sample()
	qs, err := Questions(DefaultHarness(), sc)
	if err != nil {
		t.Fatal(err)
	}
	if len(qs) != 2 || qs[0].Name != "next" || qs[1].Name != "more" {
		t.Fatalf("questions %+v", qs)
	}
	opts := qs[0].Question.Options
	if len(opts) != 3 || opts[2].Name != "select (2)" || opts[0].Desc == nil || !strings.Contains(*opts[0].Desc, "e.g. sort_object") {
		t.Fatalf("options %+v", opts)
	}
	tok, err := tokenizer.Parse(testdata.TokenizerJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := prompt.NewBuilder(tok, prompt.Default).Rows(State(sc), qs, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || !strings.Contains(rows[0].Text, "biggest files first") || !strings.Contains(rows[0].Text, "Sort objects by a property") {
		t.Fatalf("rows %+v", rows)
	}
}

type fakeModel struct{ answers map[string]any }

func (f fakeModel) Ask(context.Context, d1.Request) (*d1.Response, error) {
	return &d1.Response{Answers: f.answers}, nil
}

func TestRankReadsAnswers(t *testing.T) {
	h := DefaultHarness()
	h.Temperature, h.MoreTemperature, h.LexicalWeight = 1, 1, 0
	r := NewWith(fakeModel{map[string]any{
		"next": map[string]any{"type": "choice", "choice": "sort_object", "probabilities": map[string]any{"sort_object": 0.7, "select": 0.2, "select (2)": 0.1}},
		"more": map[string]any{"type": "noul", "noul": 0.9},
	}}, h)
	got, err := r.Rank(context.Background(), sample())
	if err != nil {
		t.Fatal(err)
	}
	if got.Probabilities[0] != 0.7 || got.Probabilities[2] != 0.1 || got.Continue != 0.9 || got.Threshold != h.Threshold {
		t.Fatalf("got %+v", got)
	}
}

func TestTemper(t *testing.T) {
	p := []float64{0.6, 0.3, 0.1}
	if got := Temper(p, 1); got[0] != 0.6 {
		t.Fatal(got)
	}
	sharp, flat := Temper(p, 0.5), Temper(p, 2)
	if !(sharp[0] > 0.6 && flat[0] < 0.6) || math.Abs(sharp[0]+sharp[1]+sharp[2]-1) > 1e-12 {
		t.Fatalf("sharp %v flat %v", sharp, flat)
	}
}

type fakeVocab map[string]int32

func (f fakeVocab) ID(t string) (int32, bool) { id, ok := f[t]; return id, ok }

func TestCorrectSpelling(t *testing.T) {
	v := fakeVocab{"Ġship": 1000, "Ġshrimp": 44307, "Ġdog": 4448, "Ġago": 5526, "Ġego": 40531, "Ġbeautiful": 8153, "Ġsunset": 49422, "Ġcat": 5140}
	for in, want := range map[string]string{
		"shimp": "shrimp", "a shimp": "a shrimp", "beatiful sunset": "beautiful sunset",
		"a cat": "a cat", "kangaroo": "kangaroo", "Shimp": "Shimp", "the TODO": "the TODO",
	} {
		if got := correct(v, in); got != want {
			t.Errorf("correct(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBlend(t *testing.T) {
	got := Blend([]float64{0.2, 0.8}, 1) // 0.2*1, 0.8/2
	if math.Abs(got[0]-0.2/0.6) > 1e-12 || math.Abs(got[1]-0.4/0.6) > 1e-12 {
		t.Fatalf("Blend = %v", got)
	}
	if got := Blend([]float64{0.2, 0.8}, 0); got[1] != 0.8 {
		t.Fatalf("Blend at 0 = %v", got)
	}
}
