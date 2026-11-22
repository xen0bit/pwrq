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
	// LexicalWeight blends the step-by-step choice with the prefilter's
	// order: each candidate's probability is multiplied by (rank+1)^-w and
	// renormalized. On the gallery the prefilter alone beats the model at
	// picking one name, and the blend beats both a little.
	LexicalWeight float64 `json:"lexical_weight"`
	// Script is the question that picks a whole query (a recipe) for what
	// the comments ask, before anything is written; ScriptOptionStyle says
	// how its options are described: "task" or "task+query".
	Script            string  `json:"script"`
	ScriptOptionStyle string  `json:"script_option_style"`
	ScriptTemperature float64 `json:"script_temperature"`
	// ScriptNone, when set, describes one more option - that no candidate
	// does what the comment asks - whose probability is taken from the
	// candidates', so a comment no recipe answers shows nothing.
	ScriptNone      string  `json:"script_none,omitempty"`
	ScriptThreshold float64 `json:"script_threshold"`
	// ContinueThreshold is the bar for the rest of a recipe offered after
	// part of it is written: tested against the scripts study's comments
	// cut at every stage.
	ContinueThreshold float64 `json:"continue_threshold"`
	// Member is the question that picks one of a cmdlet's examples once
	// the cmdlet is chosen, each option shown as its query.
	Member       string `json:"member"`
	PromptDigest string `json:"prompt_digest"`
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
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s\x00%s\x00%s\x00%s\x00%d", h.Version, h.Next, h.More, h.OptionStyle,
		h.Script, h.ScriptOptionStyle+"\x00"+h.ScriptNone+"\x00"+h.Member, layoutVersion)))
	// LexicalWeight is fitted, not part of the prompt.
	return hex.EncodeToString(sum[:8])
}

// layoutVersion changes whenever State or Questions change what the model
// reads; bump it and recalibrate together.
const layoutVersion = 3

var positionText = map[string]string{
	ideengine.PosStage:    "starting a pipeline stage",
	ideengine.PosAfter:    "after a finished stage",
	ideengine.PosField:    "naming a field of the input",
	ideengine.PosArgument: "inside a call's arguments",
}

// State is what the model reads about the query. Before anything is
// written it is the comments alone: they are all there is, and the slot
// answers are spans of them.
func State(sc ideengine.SuggestContext) any {
	if sc.Position == ideengine.PosScript {
		return sc.Comment()
	}
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
// after a finished stage, whether more is needed; then each slot question,
// as a text question whose answer is a span of the comments. Option names
// are the candidates' own, made unique; each is described by its help.
func Questions(h Harness, sc ideengine.SuggestContext) ([]prompt.Named, error) {
	script := sc.Position == ideengine.PosScript
	opts := prompt.Object{}
	seen := map[string]bool{}
	for _, c := range sc.Candidates {
		name := c.Name
		for k := 2; seen[name]; k++ {
			name = fmt.Sprintf("%s (%d)", c.Name, k)
		}
		seen[name] = true
		desc := c.Help
		switch {
		case script && h.ScriptOptionStyle == "task+query" && c.Example != "":
			desc += ": " + c.Example
		case !script && h.OptionStyle == "help+example" && c.Example != "":
			desc += " e.g. " + c.Example
		}
		if desc == "" {
			opts = append(opts, prompt.Member{Key: name, Value: nil})
		} else {
			opts = append(opts, prompt.Member{Key: name, Value: desc})
		}
	}
	instructions := h.Next
	if script {
		instructions = h.Script
		if h.ScriptNone != "" {
			opts = append(opts, prompt.Member{Key: noneOption, Value: h.ScriptNone})
		}
	}
	raw := prompt.Object{{Key: "next", Value: prompt.Object{
		{Key: "type", Value: "choice"}, {Key: "instructions", Value: instructions}, {Key: "criteria", Value: opts},
	}}}
	if sc.Position == ideengine.PosAfter {
		raw = append(raw, prompt.Member{Key: "more", Value: prompt.Object{
			{Key: "type", Value: "noul"}, {Key: "instructions", Value: h.More},
		}})
	}
	if len(sc.Intent) > 0 {
		for i, q := range sc.SlotQuestions {
			if len(raw) >= 16 {
				break
			}
			raw = append(raw, prompt.Member{Key: fmt.Sprintf("slot%d", i), Value: prompt.Object{
				{Key: "type", Value: "text"}, {Key: "instructions", Value: q},
			}})
		}
	}
	return prompt.ParseQuestions(raw)
}

// noneOption is the option name of ScriptNone.
const noneOption = "none of these"

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
	a, err := r.Answer(ctx, sc)
	if err != nil {
		return ideengine.Ranking{}, err
	}
	threshold := r.harness.Threshold
	if sc.Position == ideengine.PosScript {
		threshold = r.harness.ScriptThreshold
	}
	return ideengine.Ranking{Probabilities: a.Probabilities, Continue: a.More, Threshold: threshold,
		RecipeThreshold: r.harness.ContinueThreshold, Slots: a.Slots, Members: a.Members}, nil
}

// Answers are what one pass of the model said about a suggestion context.
type Answers struct {
	Probabilities []float64         // one a candidate, tempered
	More          float64           // P(the query needs more), 1 when not asked
	Slots         map[string]string // slot question to the words it found
	Members       map[int][]float64 // the top candidate's members, when it has several
}

// Probabilities is Answer without the slots: calibration's view.
func (r *Ranker) Probabilities(ctx context.Context, sc ideengine.SuggestContext) ([]float64, float64, error) {
	a, err := r.Answer(ctx, sc)
	if err != nil {
		return nil, 0, err
	}
	return a.Probabilities, a.More, nil
}

// Answer asks the model and reads its answers through the harness's
// temperatures: one probability a candidate, P(more needed), and the slot
// values the comments give.
func (r *Ranker) Answer(ctx context.Context, sc ideengine.SuggestContext) (Answers, error) {
	questions, err := Questions(r.harness, sc)
	if err != nil {
		return Answers{}, err
	}
	model, err := r.open(ctx)
	if err != nil {
		return Answers{}, err
	}
	// Calibrated: the release's tables decide when a span is found, and
	// the choice's temperature is refitted on top of its own.
	resp, err := model.Ask(ctx, d1.Request{State: State(sc), Questions: questions})
	if err != nil {
		return Answers{}, err
	}
	next, _ := resp.Answers["next"].(map[string]any)
	byName, _ := next["probabilities"].(map[string]any)
	opts := questions[0].Question.Options
	raw := make([]float64, len(opts))
	for i, o := range opts {
		raw[i], _ = byName[o.Name].(float64)
	}
	t := r.harness.Temperature
	if sc.Position == ideengine.PosScript {
		t = r.harness.ScriptTemperature
	}
	// With a "none" option the candidates keep what it leaves them: their
	// probabilities sum to 1 - P(none).
	// Blend, then temper: the order calibration fitted them in. A "none"
	// option, last, keeps rank 17 and its share of what is left.
	if sc.Position != ideengine.PosScript {
		raw = Blend(raw, r.harness.LexicalWeight)
	}
	probs := Temper(raw, t)[:len(sc.Candidates)]
	a := Answers{Probabilities: probs, More: 1, Slots: map[string]string{}}
	if m, ok := resp.Answers["more"].(map[string]any); ok {
		p, _ := m["noul"].(float64)
		a.More = Temper([]float64{p, 1 - p}, r.harness.MoreTemperature)[0]
	}
	for _, q := range questions {
		if !strings.HasPrefix(q.Name, "slot") {
			continue
		}
		ans, _ := resp.Answers[q.Name].(map[string]any)
		if text, ok := ans["text"].(string); ok && text != "" {
			a.Slots[q.Question.Instructions] = text
		}
	}
	if err := r.followUp(ctx, model, sc, &a); err != nil {
		return Answers{}, err
	}
	return a, nil
}

// followUp is a second, small pass about the likeliest candidate only, when
// there is anything left to ask: which of a cmdlet's examples the comment
// wants (its options the queries themselves - md5 of the text, or of a
// file), and each of the chosen query's slots the first pass left empty,
// asked again naming the cmdlet. Only in script mode, where a whole query
// is at stake.
func (r *Ranker) followUp(ctx context.Context, model Asker, sc ideengine.SuggestContext, a *Answers) error {
	if len(a.Probabilities) == 0 || sc.Position != ideengine.PosScript || len(sc.Intent) == 0 {
		return nil
	}
	top := 0
	for i, p := range a.Probabilities {
		if p > a.Probabilities[top] {
			top = i
		}
	}
	raw := prompt.Object{}
	members := sc.Candidates[top].Members
	if len(members) > 16 {
		members = members[:16]
	}
	if len(members) > 1 {
		opts := prompt.Object{}
		for _, m := range members {
			opts = append(opts, prompt.Member{Key: m.Name, Value: m.Example})
		}
		question := r.harness.Member
		if question == "" {
			question = "Which of these queries does what the comment asks?"
		}
		raw = append(raw, prompt.Member{Key: "member", Value: prompt.Object{
			{Key: "type", Value: "choice"}, {Key: "instructions", Value: question}, {Key: "criteria", Value: opts},
		}})
	}
	// The slots every member could need that the first pass found nothing
	// for: asked for whichever member wins, so the follow-up needs no third
	// pass.
	targets := map[string]bool{}
	cands := members
	if len(cands) == 0 {
		cands = []ideengine.Candidate{sc.Candidates[top]}
	}
	for _, c := range cands {
		for _, s := range c.Slots {
			if _, ok := a.Slots[s.Question]; ok {
				continue
			}
			q := ideengine.TargetedQuestion(c, s)
			if targets[q] || len(raw) >= 16 {
				continue
			}
			targets[q] = true
			raw = append(raw, prompt.Member{Key: fmt.Sprintf("t%d", len(raw)), Value: prompt.Object{
				{Key: "type", Value: "text"}, {Key: "instructions", Value: q},
			}})
		}
	}
	if len(raw) == 0 {
		return nil
	}
	qs, err := prompt.ParseQuestions(raw)
	if err != nil {
		return err
	}
	resp, err := model.Ask(ctx, d1.Request{State: State(sc), Questions: qs})
	if err != nil {
		return err
	}
	for _, q := range qs {
		switch q.Name {
		case "member":
			ans, _ := resp.Answers["member"].(map[string]any)
			byName, _ := ans["probabilities"].(map[string]any)
			p := make([]float64, len(sc.Candidates[top].Members))
			for i, m := range members {
				p[i], _ = byName[m.Name].(float64)
			}
			a.Members = map[int][]float64{top: p}
		default:
			ans, _ := resp.Answers[q.Name].(map[string]any)
			if text, ok := ans["text"].(string); ok && text != "" {
				a.Slots[q.Question.Instructions] = text
			}
		}
	}
	return nil
}

// Blend weighs probabilities, given in the prefilter's order, by a prior
// that falls with rank: p_k (k+1)^-w, renormalized.
func Blend(p []float64, w float64) []float64 {
	if w == 0 {
		return p
	}
	out := make([]float64, len(p))
	total := 0.0
	for k, x := range p {
		out[k] = x * math.Pow(float64(k+1), -w)
		total += out[k]
	}
	if total == 0 {
		return p
	}
	for k := range out {
		out[k] /= total
	}
	return out
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
