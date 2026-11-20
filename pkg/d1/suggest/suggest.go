// Package suggest ranks the IDE's query suggestions with the local d1-2B
// decision model: it implements ideengine.Ranker.
//
// The engine gathers the candidates - the cmdlets, builtins, fields or
// example arguments the cursor could take - and this asks d1 one packed set
// of questions about the query so far: which candidate comes next, each
// option described by its own help text (so the model reads the cmdlet's
// documentation as it chooses), and, after a finished stage, whether the
// query needs more at all. The question wording, the temperatures and the
// bar for showing a suggestion unasked are fitted by
// internal/tools/d1cal against pwrq's own published examples and recorded
// in harness.json; TestHarnessMatchesPrompt fails when the prompt changes
// without a recalibration.
package suggest

import (
	"context"
	"crypto/sha256"
	_ "embed" // the calibrated harness
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"

	"github.com/xen0bit/pwrq/pkg/d1"
	"github.com/xen0bit/pwrq/pkg/d1/ort"
	"github.com/xen0bit/pwrq/pkg/d1/prompt"
	"github.com/xen0bit/pwrq/pkg/ideengine"
)

//go:embed harness.json
var harnessJSON []byte

// Harness is how the questions are asked and read: calibration's output.
type Harness struct {
	Version         int     `json:"version"`
	Next            string  `json:"next"`
	More            string  `json:"more"`
	OptionStyle     string  `json:"option_style"` // "help" or "help+example"
	Temperature     float64 `json:"temperature"`
	MoreTemperature float64 `json:"more_temperature"`
	Threshold       float64 `json:"threshold"`
	PromptDigest    string  `json:"prompt_digest"`
}

// DefaultHarness is the embedded, calibrated harness.
func DefaultHarness() Harness {
	var h Harness
	if err := json.Unmarshal(harnessJSON, &h); err != nil {
		panic(fmt.Sprintf("suggest: harness.json: %v", err))
	}
	return h
}

// Digest identifies everything about a harness's prompt that calibration
// depends on: wording, option style, and the prompt layout's version.
func (h Harness) Digest() string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s\x00%s\x00%d", h.Version, h.Next, h.More, h.OptionStyle, layoutVersion)))
	return hex.EncodeToString(sum[:8])
}

// layoutVersion changes whenever State or Questions change what the model
// reads; bump it and recalibrate together.
const layoutVersion = 1

var positionText = map[string]string{
	ideengine.PosStage:    "starting a pipeline stage",
	ideengine.PosAfter:    "after a finished stage",
	ideengine.PosField:    "naming a field of the input",
	ideengine.PosArgument: "inside a call's arguments",
}

// State is what the model reads about the query.
func State(sc ideengine.SuggestContext) any {
	st := prompt.Object{
		{Key: "query so far", Value: sc.Before},
		{Key: "cursor", Value: positionText[sc.Position]},
	}
	if len(sc.Intent) > 0 {
		intent := make([]any, len(sc.Intent))
		for i, s := range sc.Intent {
			intent[i] = s
		}
		st = append(st, prompt.Member{Key: "comments", Value: intent})
	}
	if sc.Partial != "" {
		st = append(st, prompt.Member{Key: "typed so far", Value: sc.Partial})
	}
	if sc.Call != "" {
		st = append(st, prompt.Member{Key: "inside the arguments of", Value: sc.Call})
	}
	if sc.Input != "" {
		st = append(st, prompt.Member{Key: "input sample", Value: sc.Input})
	}
	return st
}

// Questions are what the model is asked: which candidate is next, and,
// after a finished stage, whether more is needed. Option names are the
// candidates' own, made unique; each is described by its help.
func Questions(h Harness, sc ideengine.SuggestContext) ([]prompt.Named, error) {
	opts := prompt.Object{}
	seen := map[string]bool{}
	for _, c := range sc.Candidates {
		name := c.Name
		for k := 2; seen[name]; k++ {
			name = fmt.Sprintf("%s (%d)", c.Name, k)
		}
		seen[name] = true
		desc := c.Help
		if h.OptionStyle == "help+example" && c.Example != "" {
			desc += " e.g. " + c.Example
		}
		if desc == "" {
			opts = append(opts, prompt.Member{Key: name, Value: nil})
		} else {
			opts = append(opts, prompt.Member{Key: name, Value: desc})
		}
	}
	raw := prompt.Object{{Key: "next", Value: prompt.Object{
		{Key: "type", Value: "choice"}, {Key: "instructions", Value: h.Next}, {Key: "criteria", Value: opts},
	}}}
	if sc.Position == ideengine.PosAfter {
		raw = append(raw, prompt.Member{Key: "more", Value: prompt.Object{
			{Key: "type", Value: "noul"}, {Key: "instructions", Value: h.More},
		}})
	}
	return prompt.ParseQuestions(raw)
}

// Asker answers a request: d1.Model, or a fake in tests.
type Asker interface {
	Ask(ctx context.Context, req d1.Request) (*d1.Response, error)
}

// Ranker is ideengine.Ranker over d1-2B.
type Ranker struct {
	opts    d1.Options
	harness Harness
	open    func(ctx context.Context) (Asker, error)

	mu      sync.Mutex
	warming bool
}

// New ranks with the shared model, opened with opts.
func New(opts d1.Options) *Ranker {
	r := &Ranker{opts: opts, harness: DefaultHarness()}
	r.open = func(ctx context.Context) (Asker, error) { return d1.Shared(ctx, r.opts) }
	return r
}

// NewWith ranks with a given model and harness: calibration's runs, tests.
func NewWith(a Asker, h Harness) *Ranker {
	return &Ranker{harness: h, open: func(context.Context) (Asker, error) { return a, nil }}
}

// Available reports whether this platform can run the model at all.
func Available() bool { return ort.Supported }

// Rank implements ideengine.Ranker.
func (r *Ranker) Rank(ctx context.Context, sc ideengine.SuggestContext) (ideengine.Ranking, error) {
	probs, more, err := r.Probabilities(ctx, sc)
	if err != nil {
		return ideengine.Ranking{}, err
	}
	return ideengine.Ranking{Probabilities: probs, Continue: more, Threshold: r.harness.Threshold}, nil
}

// Probabilities asks the model and reads its answers through the harness's
// temperatures: one probability a candidate, and P(more needed).
func (r *Ranker) Probabilities(ctx context.Context, sc ideengine.SuggestContext) ([]float64, float64, error) {
	questions, err := Questions(r.harness, sc)
	if err != nil {
		return nil, 0, err
	}
	model, err := r.open(ctx)
	if err != nil {
		return nil, 0, err
	}
	resp, err := model.Ask(ctx, d1.Request{State: State(sc), Questions: questions, Uncalibrated: true})
	if err != nil {
		return nil, 0, err
	}
	next, _ := resp.Answers["next"].(map[string]any)
	byName, _ := next["probabilities"].(map[string]any)
	opts := questions[0].Question.Options
	raw := make([]float64, len(opts))
	for i, o := range opts {
		raw[i], _ = byName[o.Name].(float64)
	}
	probs := Temper(raw, r.harness.Temperature)
	more := 1.0
	if m, ok := resp.Answers["more"].(map[string]any); ok {
		p, _ := m["noul"].(float64)
		more = Temper([]float64{p, 1 - p}, r.harness.MoreTemperature)[0]
	}
	return probs, more, nil
}

// Temper rescales a distribution by a temperature: softmax(log p / t), which
// is the softmax of the logits over t.
func Temper(p []float64, t float64) []float64 {
	if t <= 0 || t == 1 {
		return p
	}
	out := make([]float64, len(p))
	m := math.Inf(-1)
	for i, x := range p {
		out[i] = math.Log(math.Max(x, 1e-300)) / t
		m = math.Max(m, out[i])
	}
	total := 0.0
	for i := range out {
		out[i] = math.Exp(out[i] - m)
		total += out[i]
	}
	for i := range out {
		out[i] /= total
	}
	return out
}

// Status implements ideengine.Ranker.
func (r *Ranker) Status() ideengine.SuggestStatus {
	if !Available() {
		return ideengine.SuggestStatus{State: "unavailable", Fraction: -1, Unavailable: "ONNX Runtime has no build for this platform"}
	}
	s := d1.CurrentStatus()
	st := ideengine.SuggestStatus{State: s.State, Fraction: s.Fraction(), Device: s.Device}
	switch s.State {
	case d1.StateFetching:
		st.Detail = "downloading " + s.File
	case d1.StateLoading:
		st.Detail = "loading d1-2B"
	case d1.StateBusy:
		st.Detail = "thinking"
	case d1.StateError:
		st.Detail = firstLine(s.Error)
	}
	if m, ok := d1.Loaded(); ok && st.Device == "" {
		st.Device = m.Info().Device
	}
	return st
}

// Warm implements ideengine.Ranker: it starts loading the model in the
// background, once, so the first suggestion does not wait two seconds - or
// a download - for it.
func (r *Ranker) Warm() {
	if !Available() {
		return
	}
	if _, ok := d1.Loaded(); ok {
		return
	}
	r.mu.Lock()
	if r.warming {
		r.mu.Unlock()
		return
	}
	r.warming = true
	r.mu.Unlock()
	go func() {
		_, _ = r.open(context.Background())
		r.mu.Lock()
		r.warming = false
		r.mu.Unlock()
	}()
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// Host is the ranker an editor host configures: d1-2B with the
// environment's options, or nil (no ranker, suggestions unavailable) where
// ONNX Runtime cannot run. PWRQ_SUGGEST=off turns suggestions off.
func Host() ideengine.Ranker {
	if !Available() || os.Getenv("PWRQ_SUGGEST") == "off" {
		return nil
	}
	return New(d1.OptionsFromEnv())
}
