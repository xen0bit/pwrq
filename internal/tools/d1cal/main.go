// Command d1cal calibrates pwrq's own requests to d1-2B: the questions the
// IDE asks to suggest what to write.
//
// It follows ~/Projects/jev_calibration's method - a fixed harness (question
// wording, how options are described) searched against labelled samples and
// scored the same way for every sample - and adds what a local model allows
// and a remote one did not: real calibration of the probabilities. Three
// studies, each run in a fresh build because each reads the embedded files
// the one before wrote:
//
//  1. slots (studies/scripts/SLOTS.md): each slot kind's question - "How
//     many?", "Which directory or folder?" - chosen by how often it reads the
//     labelled value out of a dev comment; written to
//     pkg/ideengine/slots.json.
//
//  2. scripts (studies/scripts): the question that picks a whole recipe for
//     a comment, compared on dev comments, its temperature fitted by NLL and
//     its ghost bar set to show the most right queries for the fewest wrong
//     ones; then the same for the rest of a recipe after part of it is
//     written. Held-out test comments are scored once and reported.
//
//  3. next (studies/completion): the step-by-step question - which name comes
//     next - on pwrq's gallery, every example cut at each stage, as before.
//
//     go run ./internal/tools/d1cal [-study all|slots|scripts|next] [-limit N]
//
// harness.json gets the prompt's digest; TestHarnessMatchesPrompt fails when
// the prompt changes without a recalibration. It runs the real model
// (PWRQ_D1_* as everywhere; GPU when available).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/xen0bit/pwrq/pkg/d1"
	"github.com/xen0bit/pwrq/pkg/d1/suggest"
	"github.com/xen0bit/pwrq/pkg/ideengine"
	"github.com/xen0bit/pwrq/pkg/udf"
)

// Sample is one labelled cursor position.
type Sample struct {
	ID       string `json:"id"`
	Label    string `json:"label"` // the name that came next; "" for a "more" sample at the query's end
	More     bool   `json:"more"`  // whether the query continued
	Kind     string `json:"kind"`  // "next" (cursor starting a stage) or "more" (after a finished stage)
	Comment  bool   `json:"comment"`
	Query    string `json:"query"`
	Input    string `json:"input,omitempty"`
	Category string `json:"category"`
}

// Variant is one harness tried.
type Variant struct {
	Name    string
	Harness suggest.Harness
}

var variants = []Variant{
	{"intent+help+example", suggest.Harness{Version: 2, OptionStyle: "help+example",
		Next: "Which of these should be written next at the cursor, to do what the query's comments ask?",
		More: "Does the query need another step after what is written, to do what its comments ask?"}},
	{"intent+help", suggest.Harness{Version: 2, OptionStyle: "help",
		Next: "Which of these should be written next at the cursor, to do what the query's comments ask?",
		More: "Does the query need another step after what is written, to do what its comments ask?"}},
	{"plain+help+example", suggest.Harness{Version: 2, OptionStyle: "help+example",
		Next: "Which function comes next in this jq query?",
		More: "Is this jq query unfinished?"}},
}

// Result is one sample under one variant.
type Result struct {
	ID       string    `json:"id"`
	Covered  bool      `json:"covered"`  // the label was among the candidates
	LabelAt  int       `json:"label_at"` // its index, -1 when not covered
	Probs    []float64 `json:"probs,omitempty"`
	Lexical  int       `json:"lexical_rank"` // the label's rank in the engine's lexical order
	MoreP    float64   `json:"more_p"`
	More     bool      `json:"more"`
	Kind     string    `json:"kind"`
	Label    string    `json:"label"`
	Seconds  float64   `json:"seconds"`
	Tokens   int       `json:"tokens"`
	Variant  string    `json:"variant"`
	Category string    `json:"category"`
}

func main() {
	limit := flag.Int("limit", 0, "use at most this many samples of each kind (0: all)")
	out := flag.String("out", filepath.Join("internal", "tools", "d1cal", "studies", "completion"), "the study folder")
	harnessOut := flag.String("harness", filepath.Join("pkg", "d1", "suggest", "harness.json"), "where the calibrated harness goes")
	only := flag.String("variants", "", "comma-separated variant names to run (default all)")
	study := flag.String("study", "all", "which study: slots (each kind's question), scripts (whole queries from comments), next (the step-by-step choice), or all")
	flag.Parse()
	if err := run(*limit, *out, *harnessOut, *only, *study); err != nil {
		fmt.Fprintln(os.Stderr, "d1cal:", err)
		os.Exit(1)
	}
}

func run(limit int, out, harnessOut, only, study string) error {
	_ = udf.DefaultRegistry() // publishes the catalog the example recipes are made from
	scriptsDir := filepath.Join(filepath.Dir(out), "scripts")
	switch study {
	case "all":
		// Each study reads what the one before wrote into embedded files
		// (slots.json, harness.json), so each runs in a fresh build.
		for _, st := range []string{"slots", "scripts", "next"} {
			if err := reexec(st, limit, out, harnessOut, only); err != nil {
				return err
			}
		}
		return nil
	case "slots", "scripts":
		model, err := d1.Shared(context.Background(), d1.OptionsFromEnv())
		if err != nil {
			return err
		}
		if study == "slots" {
			return runSlots(context.Background(), model, scriptsDir)
		}
		eng := ideengine.New(ideengine.Config{Registry: udf.DefaultRegistry()})
		return runScripts(context.Background(), eng, model, scriptsDir, harnessOut, only)
	case "next":
	default:
		return fmt.Errorf("unknown study %q", study)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	samples := buildSamples(limit)
	if err := writeJSONL(filepath.Join(out, "samples.jsonl"), samples); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "%d samples\n", len(samples))

	opts := d1.OptionsFromEnv()
	opts.Log = func(f string, a ...any) { fmt.Fprintf(os.Stderr, f+"\n", a...) }
	ctx := context.Background()
	model, err := d1.Shared(ctx, opts)
	if err != nil {
		return err
	}
	info := model.Info()
	fmt.Fprintf(os.Stderr, "d1-2B on %s (%s)\n", info.Device, info.Variant)
	eng := ideengine.New(ideengine.Config{Registry: udf.DefaultRegistry()})

	byVariant := map[string][]Result{}
	for _, v := range variants {
		if only != "" && !strings.Contains(","+only+",", ","+v.Name+",") {
			continue
		}
		cache := filepath.Join(out, "runs", v.Name+".jsonl")
		results, err := runVariant(ctx, eng, model, v, samples, cache)
		if err != nil {
			return err
		}
		byVariant[v.Name] = results
	}
	return report(byVariant, info, samples, out, harnessOut)
}

// reexec runs one study in a fresh `go run`, so it is built with the
// embedded files the studies before it wrote.
func reexec(study string, limit int, out, harnessOut, only string) error {
	args := []string{"run", "./internal/tools/d1cal", "-study", study, "-out", out, "-harness", harnessOut, "-limit", fmt.Sprint(limit)}
	if only != "" {
		args = append(args, "-variants", only)
	}
	cmd := exec.Command("go", args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

// stageSplit cuts a query at its top-level pipes.
func stageSplit(q string) []string {
	var stages []string
	depth, inStr, start := 0, false, 0
	rs := []rune(q)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case inStr:
			switch c {
			case '\\':
				i++
			case '"':
				inStr = false
			}
		case c == '"':
			inStr = true
		case c == '#':
			for i < len(rs) && rs[i] != '\n' {
				i++
			}
		case c == '(' || c == '[' || c == '{':
			depth++
		case c == ')' || c == ']' || c == '}':
			depth--
		case c == '|' && depth == 0 && (i+1 >= len(rs) || rs[i+1] != '=') && (i == 0 || rs[i-1] != '|'):
			stages = append(stages, strings.TrimSpace(string(rs[start:i])))
			start = i + 1
		}
	}
	return append(stages, strings.TrimSpace(string(rs[start:])))
}

var headRe = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)`)

// buildSamples cuts the gallery's examples into labelled cursor positions:
// with the example's title as a comment and without, at every stage start
// ("next") and after every stage ("more", true while the query goes on).
func buildSamples(limit int) []Sample {
	var next, more []Sample
	for i, ex := range ideengine.Examples() {
		if strings.Contains(ex.Query, "def ") {
			continue // definitions reshape the vocabulary under the cursor
		}
		stages := stageSplit(ex.Query)
		input := ex.Input
		if len(input) > 1200 {
			input = input[:1200]
		}
		for _, withComment := range []bool{true, false} {
			head := ""
			if withComment {
				head = "# " + ex.Title + "\n"
			}
			for k := 1; k <= len(stages); k++ {
				prefix := head + strings.Join(stages[:k], "\n| ")
				id := fmt.Sprintf("ex%03d.s%d.c%v", i, k, withComment)
				cont := k < len(stages)
				more = append(more, Sample{ID: id + ".more", More: cont, Kind: "more", Comment: withComment,
					Query: prefix + " ", Input: input, Category: ex.Category})
				if !cont {
					continue
				}
				m := headRe.FindStringSubmatch(stages[k])
				if m == nil || m[1] == "if" || m[1] == "reduce" || m[1] == "foreach" || m[1] == "try" {
					continue
				}
				next = append(next, Sample{ID: id + ".next", Label: m[1], More: true, Kind: "next", Comment: withComment,
					Query: prefix + "\n| ", Input: input, Category: ex.Category})
			}
		}
	}
	thin := func(s []Sample) []Sample {
		if limit <= 0 || len(s) <= limit {
			return s
		}
		// every k-th, so each part of the gallery is represented
		out := make([]Sample, 0, limit)
		step := float64(len(s)) / float64(limit)
		for i := 0; i < limit; i++ {
			out = append(out, s[int(float64(i)*step)])
		}
		return out
	}
	return append(thin(next), thin(more)...)
}

func runVariant(ctx context.Context, eng *ideengine.Engine, model *d1.Model, v Variant, samples []Sample, cache string) ([]Result, error) {
	done := map[string]Result{}
	if raw, err := os.ReadFile(cache); err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			var r Result
			if json.Unmarshal([]byte(line), &r) == nil && r.ID != "" {
				done[r.ID] = r
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(cache), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(cache, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	h := v.Harness
	h.Temperature, h.MoreTemperature = 1, 1
	ranker := suggest.NewWith(model, h)
	var results []Result
	start := time.Now()
	for i, s := range samples {
		if r, ok := done[s.ID]; ok {
			results = append(results, r)
			continue
		}
		sc := eng.SuggestContext(ideengine.SuggestRequest{Query: s.Query, Cursor: -1, Input: s.Input})
		r := Result{ID: s.ID, LabelAt: -1, Kind: s.Kind, Label: s.Label, More: s.More, Variant: v.Name, Category: s.Category, Lexical: -1}
		for k, c := range sc.Candidates {
			if s.Kind == "next" && !r.Covered && candidateIs(c, s.Label) {
				r.LabelAt, r.Covered, r.Lexical = k, true, k
			}
		}
		if s.Kind == "next" && !r.Covered {
			// Not among the candidates: the ranker cannot pick it, and is
			// not asked. Coverage is the engine's prefilter's to improve.
			results = append(results, r)
			writeLine(f, r)
			continue
		}
		t0 := time.Now()
		probs, moreP, err := ranker.Probabilities(ctx, sc)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", s.ID, err)
		}
		r.Probs, r.MoreP, r.Seconds = probs, moreP, time.Since(t0).Seconds()
		r.Tokens = len([]rune(s.Query))
		results = append(results, r)
		writeLine(f, r)
		if i%25 == 0 {
			fmt.Fprintf(os.Stderr, "%s: %d/%d (%.1f s elapsed)\n", v.Name, i, len(samples), time.Since(start).Seconds())
		}
	}
	return results, nil
}

// candidateIs reports whether a candidate writes the labelled name next: a
// name by itself, or a recipe whose continuation starts with it.
func candidateIs(c ideengine.Candidate, label string) bool {
	if c.Kind != "recipe" {
		return c.Name == label
	}
	first := ideengine.Stages(strings.TrimPrefix(strings.TrimSpace(c.Insert), "| "))[0]
	return strings.TrimLeft(identHead(first), "[({") == label
}

var identHeadRe = regexp.MustCompile(`^[\[({]*[A-Za-z_][A-Za-z0-9_]*`)

func identHead(s string) string { return identHeadRe.FindString(strings.TrimSpace(s)) }

func writeLine(f *os.File, v any) {
	raw, _ := json.Marshal(v)
	_, _ = f.Write(append(raw, '\n'))
}

func writeJSONL[T any](path string, rows []T) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	for _, r := range rows {
		writeLine(f, r)
	}
	return nil
}

// --- metrics ---

type metrics struct {
	N, Covered                  int
	Top1, Top3, LexicalTop1     float64
	NLL, ECE, Brier             float64
	MoreN                       int
	MoreAcc, MoreNLL, MoreBrier float64
	P50, P95                    float64
}

func temperPick(r Result, t float64) []float64 { return suggest.Temper(r.Probs, t) }

func evaluate(rs []Result, t, tm float64) metrics {
	var m metrics
	var conf, correct []float64
	var secs []float64
	for _, r := range rs {
		if r.Seconds > 0 {
			secs = append(secs, r.Seconds)
		}
		switch r.Kind {
		case "next":
			m.N++
			if !r.Covered {
				continue
			}
			m.Covered++
			p := temperPick(r, t)
			order := argsort(p)
			if order[0] == r.LabelAt {
				m.Top1++
			}
			for _, k := range order[:min(3, len(order))] {
				if k == r.LabelAt {
					m.Top3++
				}
			}
			if r.Lexical == 0 {
				m.LexicalTop1++
			}
			m.NLL += -math.Log(math.Max(p[r.LabelAt], 1e-12))
			for k, q := range p {
				y := 0.0
				if k == r.LabelAt {
					y = 1
				}
				m.Brier += (q - y) * (q - y)
			}
			conf = append(conf, p[order[0]])
			if order[0] == r.LabelAt {
				correct = append(correct, 1)
			} else {
				correct = append(correct, 0)
			}
		case "more":
			m.MoreN++
			p := suggest.Temper([]float64{r.MoreP, 1 - r.MoreP}, tm)[0]
			y := 0.0
			if r.More {
				y = 1
			}
			if (p >= 0.5) == r.More {
				m.MoreAcc++
			}
			m.MoreNLL += -(y*math.Log(math.Max(p, 1e-12)) + (1-y)*math.Log(math.Max(1-p, 1e-12)))
			m.MoreBrier += (p - y) * (p - y)
		}
	}
	if m.Covered > 0 {
		c := float64(m.Covered)
		m.Top1 /= c
		m.Top3 /= c
		m.LexicalTop1 /= c
		m.NLL /= c
		m.Brier /= c
		m.ECE = ece(conf, correct, 10)
	}
	if m.MoreN > 0 {
		n := float64(m.MoreN)
		m.MoreAcc /= n
		m.MoreNLL /= n
		m.MoreBrier /= n
	}
	sort.Float64s(secs)
	if len(secs) > 0 {
		m.P50 = secs[len(secs)/2]
		m.P95 = secs[min(len(secs)-1, len(secs)*95/100)]
	}
	return m
}

func argsort(p []float64) []int {
	idx := make([]int, len(p))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return p[idx[a]] > p[idx[b]] })
	return idx
}

// ece is the expected calibration error of top-1 confidences, equal-width
// bins.
func ece(conf, correct []float64, bins int) float64 {
	if len(conf) == 0 {
		return 0
	}
	sumC := make([]float64, bins)
	sumA := make([]float64, bins)
	n := make([]float64, bins)
	for i, c := range conf {
		b := min(bins-1, int(c*float64(bins)))
		sumC[b] += c
		sumA[b] += correct[i]
		n[b]++
	}
	e := 0.0
	for b := range n {
		if n[b] > 0 {
			e += n[b] / float64(len(conf)) * math.Abs(sumC[b]/n[b]-sumA[b]/n[b])
		}
	}
	return e
}

var temperatures = func() []float64 {
	var out []float64
	for t := 0.25; t <= 12.0001; t *= 1.1 {
		out = append(out, math.Round(t*1000)/1000)
	}
	return out
}()

// fold assigns a result to one of five folds by its label (or, for "more"
// samples, by its example), so a name is never scored on a fold that fitted
// its parameters.
func fold(r Result) int {
	key := r.Label
	if key == "" {
		key, _, _ = strings.Cut(r.ID, ".")
	}
	h := 0
	for _, c := range key {
		h = h*31 + int(c)
	}
	return ((h % 5) + 5) % 5
}

// fitCV picks temperatures by NLL on four folds and reports the fifth.
func fitCV(rs []Result) (t, tm float64, cv metrics) {
	best := func(train []Result) (float64, float64) {
		bt, bnll := 1.0, math.Inf(1)
		btm, bmnll := 1.0, math.Inf(1)
		for _, cand := range temperatures {
			m := evaluate(train, cand, cand)
			if m.Covered > 0 && m.NLL < bnll {
				bt, bnll = cand, m.NLL
			}
			if m.MoreN > 0 && m.MoreNLL < bmnll {
				btm, bmnll = cand, m.MoreNLL
			}
		}
		return bt, btm
	}
	var held []Result
	var heldT []float64
	for k := 0; k < 5; k++ {
		var train, test []Result
		for _, r := range rs {
			if fold(r) == k {
				test = append(test, r)
			} else {
				train = append(train, r)
			}
		}
		ft, fm := best(train)
		for _, r := range test {
			held = append(held, r)
			heldT = append(heldT, ft, fm)
		}
	}
	// Cross-validated metrics: each held-out result under its own fold's
	// temperatures.
	var tuned []Result
	for i, r := range held {
		r2 := r
		if r.Probs != nil {
			r2.Probs = suggest.Temper(r.Probs, heldT[2*i])
		}
		if r.Kind == "more" {
			r2.MoreP = suggest.Temper([]float64{r.MoreP, 1 - r.MoreP}, heldT[2*i+1])[0]
		}
		tuned = append(tuned, r2)
	}
	cv = evaluate(tuned, 1, 1)
	t, tm = best(rs)
	return t, tm, cv
}

// threshold is the lowest bar on top-1 confidence at which the shown
// suggestions are right at least `precision` of the time, across folds.
func threshold(rs []Result, t float64, precision float64) (float64, float64, float64) {
	type pt struct {
		c  float64
		ok bool
	}
	var pts []pt
	for _, r := range rs {
		if r.Kind != "next" {
			continue
		}
		if !r.Covered {
			pts = append(pts, pt{0, false}) // never confident about a miss
			continue
		}
		p := temperPick(r, t)
		o := argsort(p)
		pts = append(pts, pt{p[o[0]], o[0] == r.LabelAt})
	}
	for th := 0.05; th < 1; th += 0.01 {
		shown, right := 0, 0
		for _, x := range pts {
			if x.c >= th {
				shown++
				if x.ok {
					right++
				}
			}
		}
		if shown > 0 && float64(right)/float64(shown) >= precision {
			return math.Round(th*100) / 100, float64(right) / float64(shown), float64(shown) / float64(len(pts))
		}
	}
	return 0.99, 0, 0
}

func report(by map[string][]Result, info d1.Info, samples []Sample, out, harnessOut string) error {
	var names []string
	for n := range by {
		names = append(names, n)
	}
	sort.Strings(names)
	var bestName string
	bestTop1 := -1.0
	var j strings.Builder
	fmt.Fprintf(&j, "# Journal: calibrating the IDE's suggestion questions to d1-2B\n\n")
	fmt.Fprintf(&j, "Run %s on %s (%s, ONNX Runtime %s), %d samples.\n\n", time.Now().UTC().Format("2006-01-02"), info.Device, info.Variant, info.ORTVersion, len(samples))
	fmt.Fprintf(&j, "| variant | covered | top-1 | top-3 | lexical top-1 | NLL | ECE | Brier | more acc | p50 s | p95 s |\n|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, n := range names {
		m := evaluate(by[n], 1, 1)
		fmt.Fprintf(&j, "| %s | %d/%d | %.3f | %.3f | %.3f | %.3f | %.3f | %.3f | %.3f | %.2f | %.2f |\n",
			n, m.Covered, m.N, m.Top1, m.Top3, m.LexicalTop1, m.NLL, m.ECE, m.Brier, m.MoreAcc, m.P50, m.P95)
		if m.Top1 > bestTop1 {
			bestTop1, bestName = m.Top1, n
		}
	}
	// The blend with the prefilter's order: the weight with the best top-1,
	// applied before temperatures are fitted.
	bestW, bestW1 := 0.0, -1.0
	for _, w := range []float64{0, 0.5, 1, 1.5, 2, 2.5, 3, 4, 5} {
		var blended []Result
		for _, r := range by[bestName] {
			if r.Probs != nil {
				r.Probs = suggest.Blend(r.Probs, w)
			}
			blended = append(blended, r)
		}
		if m := evaluate(blended, 1, 1); m.Top1 > bestW1+1e-9 {
			bestW, bestW1 = w, m.Top1
		}
	}
	modelOnly := evaluate(by[bestName], 1, 1)
	var rs []Result
	for _, r := range by[bestName] {
		if r.Probs != nil {
			r.Probs = suggest.Blend(r.Probs, bestW)
		}
		rs = append(rs, r)
	}
	raw := evaluate(rs, 1, 1)
	t, tm, cv := fitCV(rs)
	th, prec, cover := threshold(rs, t, 0.8)
	h := suggest.DefaultHarness()
	if raw, err := os.ReadFile(harnessOut); err == nil {
		_ = json.Unmarshal(raw, &h) // keep the scripts study's fields
	}
	for _, v := range variants {
		if v.Name == bestName {
			h.Version, h.Next, h.More, h.OptionStyle = v.Harness.Version, v.Harness.Next, v.Harness.More, v.Harness.OptionStyle
		}
	}
	h.Temperature, h.MoreTemperature, h.Threshold, h.LexicalWeight = t, tm, th, bestW
	h.PromptDigest = h.Digest()

	// by category
	cats := map[string][]Result{}
	for _, r := range rs {
		cats[r.Category] = append(cats[r.Category], r)
	}
	var catNames []string
	for c := range cats {
		catNames = append(catNames, c)
	}
	sort.Strings(catNames)
	fmt.Fprintf(&j, "\nChosen: **%s** (best top-1 among covered samples). Blended with the prefilter's order at weight %.1f: top-1 %.3f, against %.3f for the model alone and %.3f for the order alone.\n\n", bestName, bestW, raw.Top1, modelOnly.Top1, modelOnly.LexicalTop1)
	fmt.Fprintf(&j, "Temperatures fitted by NLL: choice %.3f, more %.3f. Cross-validated (5 folds by label):\n\n", t, tm)
	fmt.Fprintf(&j, "| | top-1 | NLL | ECE | Brier | more acc | more NLL | more Brier |\n|---|---|---|---|---|---|---|---|\n")
	fmt.Fprintf(&j, "| raw (T=1) | %.3f | %.3f | %.3f | %.3f | %.3f | %.3f | %.3f |\n", raw.Top1, raw.NLL, raw.ECE, raw.Brier, raw.MoreAcc, raw.MoreNLL, raw.MoreBrier)
	fmt.Fprintf(&j, "| tempered (CV) | %.3f | %.3f | %.3f | %.3f | %.3f | %.3f | %.3f |\n\n", cv.Top1, cv.NLL, cv.ECE, cv.Brier, cv.MoreAcc, cv.MoreNLL, cv.MoreBrier)
	fmt.Fprintf(&j, "Ghost-text bar: %.2f (precision %.3f, shown on %.1f%% of stage starts).\n\n", th, prec, 100*cover)
	fmt.Fprintf(&j, "By category (chosen variant, raw):\n\n| category | n | top-1 | top-3 |\n|---|---|---|---|\n")
	for _, c := range catNames {
		m := evaluate(cats[c], 1, 1)
		if m.Covered == 0 {
			continue
		}
		fmt.Fprintf(&j, "| %s | %d | %.2f | %.2f |\n", c, m.Covered, m.Top1, m.Top3)
	}
	if err := os.WriteFile(filepath.Join(out, "JOURNAL.md"), []byte(j.String()), 0o644); err != nil {
		return err
	}

	var f strings.Builder
	fmt.Fprintf(&f, "# Findings: the IDE's suggestion questions on d1-2B\n\n")
	fmt.Fprintf(&f, "- Harness: `%s`. The choice's options are the engine's prefiltered candidates, each described by its help text%s.\n", bestName, map[bool]string{true: " and first example", false: ""}[h.OptionStyle == "help+example"])
	fmt.Fprintf(&f, "- Coverage: the label is among the %d candidates on %d of %d stage starts (%.0f%%); the rest are the prefilter's misses, which no ranking can recover.\n", 16, raw.Covered, raw.N, 100*float64(raw.Covered)/math.Max(1, float64(raw.N)))
	fmt.Fprintf(&f, "- Ranking: the model alone is right first %.1f%% of the time and the prefilter's order alone %.1f%%; blended (prior weight %.1f) top-1 is %.1f%% and top-3 %.1f%% among covered samples.\n", 100*modelOnly.Top1, 100*modelOnly.LexicalTop1, bestW, 100*raw.Top1, 100*raw.Top3)
	fmt.Fprintf(&f, "- Calibration: temperature %.2f takes the choice's ECE from %.3f to %.3f and NLL from %.3f to %.3f (cross-validated); the gate's temperature is %.2f.\n", t, raw.ECE, cv.ECE, raw.NLL, cv.NLL, tm)
	fmt.Fprintf(&f, "- Ghost text appears unasked at confidence %.2f or more, where it is right %.0f%% of the time, on %.0f%% of stage starts.\n", th, 100*prec, 100*cover)
	fmt.Fprintf(&f, "- Latency (%s): p50 %.2f s, p95 %.2f s a suggestion.\n", info.Device, raw.P50, raw.P95)
	fmt.Fprintf(&f, "\nRe-run with `make d1.calibrate` whenever the prompt (pkg/d1/suggest) or the candidate prefilter (pkg/ideengine/suggest.go) changes; `TestHarnessMatchesPrompt` fails until it is.\n")
	if err := os.WriteFile(filepath.Join(out, "FINDINGS.md"), []byte(f.String()), 0o644); err != nil {
		return err
	}
	raw2, _ := json.MarshalIndent(h, "", "  ")
	if err := os.WriteFile(harnessOut, append(raw2, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Fprint(os.Stderr, j.String())
	return nil
}
