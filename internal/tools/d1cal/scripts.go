package main

// The scripts study calibrates the question that writes a whole query from
// a comment: which recipe the comment asks for, and what its slots are.
//
// The samples (studies/scripts/eval.jsonl) are comments written the way a
// person writes them - paraphrased, terse, misspelt - each labelled with
// the recipe that does it and the slot values it states, plus negatives
// that no recipe does. They are split in two: variants are compared,
// temperatures fitted and the ghost bar set on "dev"; "test" is scored once,
// with what dev chose, and is the number reported.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/xen0bit/pwrq/pkg/d1"
	"github.com/xen0bit/pwrq/pkg/d1/suggest"
	"github.com/xen0bit/pwrq/pkg/ideengine"
	"github.com/xen0bit/pwrq/pkg/udf/discovery"
)

// ScriptSample is one labelled comment.
type ScriptSample struct {
	ID      string            `json:"id"`
	Comment string            `json:"comment"`
	Recipe  string            `json:"recipe"` // "" when no recipe does it
	Slots   map[string]string `json:"slots"`
	Split   string            `json:"split"`
	// Source is "hand" (eval.jsonl, written against the hand recipes) or
	// "example" (intents.jsonl, one comment a catalog example at least).
	Source string `json:"source,omitempty"`
}

// ScriptResult is one sample under one variant.
type ScriptResult struct {
	ID      string            `json:"id"`
	Split   string            `json:"split"`
	Source  string            `json:"source"`
	Recipe  string            `json:"recipe"`
	LabelAt int               `json:"label_at"` // the label's index among the candidates, -1 when not offered
	Names   []string          `json:"names"`
	Probs   []float64         `json:"probs"` // raw (T=1)
	Slots   map[string]string `json:"slots"` // question -> words found
	Want    string            `json:"want"`  // the query the labels fill
	Got     string            `json:"got"`   // the query the top candidate fills
	Seconds float64           `json:"seconds"`
	Variant string            `json:"variant"`
	SlotsOK bool              `json:"slots_ok"`
	// Picked is the query chosen: the top candidate, or the member of it
	// the second pass picked. PickOK is whether it is the label.
	Picked string `json:"picked"`
	PickOK bool   `json:"pick_ok"`
	// Works is the functional match: the query chosen calls the cmdlets
	// the label does (perhaps more), and has every value the comment
	// states. A hand recipe wrapping the same cmdlet, or the cmdlet's other
	// calling form, does what was asked.
	Works    bool `json:"works"`
	Positive bool `json:"positive"`
}

// ghostPrecision is the floor on how often an unasked ghost does what the
// comment asks. Under it the bar maximises right ghosts less wrong ones: a
// wrong ghost costs a glance and a keystroke, a missing one the whole query.
const ghostPrecision = 0.8

type scriptVariant struct {
	Name     string
	Question string
	Style    string
	None     string
}

var scriptVariants = []scriptVariant{
	{"queries/task", "Which of these queries does what the comment asks?", "task", ""},
	{"queries/task+query", "Which of these queries does what the comment asks?", "task+query", ""},
	{"task/task", "Which of these tasks does the comment describe?", "task", ""},
	{"wants/task", "The comment says what the user wants done. Which of these does it?", "task", ""},
	{"task/task+none", "Which of these tasks does the comment describe?", "task", "None of these: the comment asks for something else"},
	{"wants/task+none", "The comment says what the user wants done. Which of these does it?", "task", "None of these does what the comment asks"},
}

// Intent is one line of intents.jsonl: a comment that asks for a catalog
// example.
type Intent struct {
	Example string `json:"example"`
	Comment string `json:"comment"`
}

func loadIntents(path string) ([]Intent, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []Intent
	for i, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var in Intent
		if err := json.Unmarshal([]byte(line), &in); err != nil {
			return nil, fmt.Errorf("intents.jsonl line %d: %w", i+1, err)
		}
		out = append(out, in)
	}
	return out, nil
}

// allSamples are the hand-labelled comments and the examples' intents, the
// latter labelled from the example itself: its recipe, and each slot's own
// value wherever the comment says it.
func allSamples(dir string) ([]ScriptSample, error) {
	samples, err := loadScriptSamples(filepath.Join(dir, "eval.jsonl"))
	if err != nil {
		return nil, err
	}
	for i := range samples {
		samples[i].Source = "hand"
	}
	intents, err := loadIntents(filepath.Join(dir, "intents.jsonl"))
	if err != nil {
		return nil, err
	}
	bySource := map[string]ideengine.Recipe{}
	for _, r := range ideengine.ExampleRecipes() {
		bySource[r.Source] = r
	}
	for i, in := range intents {
		r, ok := bySource[in.Example]
		if !ok {
			continue // TestEveryIntentNamesAnExample reports these
		}
		slots := map[string]string{}
		low := strings.ToLower(in.Comment)
		for _, sl := range r.Slots {
			if sl.Default != "" && strings.Contains(low, strings.ToLower(sl.Default)) {
				slots[sl.Name] = sl.Default
			}
		}
		split := "dev"
		if hashString(in.Example)%2 == 1 {
			split = "test"
		}
		samples = append(samples, ScriptSample{ID: fmt.Sprintf("i%03d", i), Comment: in.Comment, Recipe: r.ID, Slots: slots, Split: split, Source: "example"})
	}
	return samples, nil
}

func hashString(s string) int {
	h := 0
	for _, c := range s {
		h = (h*31 + int(c)) & 0x7fffffff
	}
	return h
}

func loadScriptSamples(path string) ([]ScriptSample, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []ScriptSample
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var s ScriptSample
		if err := json.Unmarshal(sc.Bytes(), &s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, sc.Err()
}

func runScripts(ctx context.Context, eng *ideengine.Engine, model *d1.Model, out, harnessOut, only string) error {
	samples, err := allSamples(out)
	if err != nil {
		return err
	}
	rs := ideengine.AllRecipes()
	byID := map[string]ideengine.Recipe{}
	for _, r := range rs {
		byID[r.ID] = r
	}
	base := suggest.DefaultHarness()
	results := map[string][]ScriptResult{}
	for _, v := range scriptVariants {
		if only != "" && !strings.Contains(","+only+",", ","+v.Name+",") {
			continue
		}
		h := base
		h.Script, h.ScriptOptionStyle, h.ScriptNone, h.ScriptTemperature = v.Question, v.Style, v.None, 1
		ranker := suggest.NewWith(model, h)
		var rows []ScriptResult
		for _, s := range samples {
			sc := eng.SuggestContext(ideengine.SuggestRequest{Query: "# " + s.Comment, Cursor: -1})
			r := ScriptResult{ID: s.ID, Split: s.Split, Source: s.Source, Recipe: s.Recipe, LabelAt: -1, Variant: v.Name, Positive: s.Recipe != ""}
			for k, c := range sc.Candidates {
				r.Names = append(r.Names, c.Name)
				if c.Name == s.Recipe {
					r.LabelAt = k
				}
				for _, m := range c.Members {
					if m.Name == s.Recipe {
						r.LabelAt = k
					}
				}
			}
			if rec, ok := byID[s.Recipe]; ok {
				r.Want = ideengine.Fill(rec.Query, rec.Slots, s.Slots)
			}
			t0 := time.Now()
			a, err := ranker.Answer(ctx, sc)
			if err != nil {
				return fmt.Errorf("%s: %w", s.ID, err)
			}
			r.Seconds = time.Since(t0).Seconds()
			r.Probs, r.Slots = a.Probabilities, a.Slots
			top := argsort(a.Probabilities)[0]
			ranking := ideengine.Ranking{Probabilities: a.Probabilities, Members: a.Members}
			chosen := ideengine.Choose(sc, ranking, top)
			r.Picked = chosen.Name
			r.PickOK = chosen.Name == s.Recipe
			r.Got = ideengine.FillCandidate(chosen, a.Slots, sc.Comment())
			r.Works = r.Positive && works(r.Want, r.Got, s.Slots)
			if r.LabelAt >= 0 {
				label := sc.Candidates[r.LabelAt]
				for _, m := range label.Members {
					if m.Name == s.Recipe {
						label = m
					}
				}
				r.SlotsOK = ideengine.FillCandidate(label, a.Slots, sc.Comment()) == r.Want
			}
			rows = append(rows, r)
		}
		results[v.Name] = rows
		_ = os.MkdirAll(filepath.Join(out, "runs"), 0o755)
		if err := writeJSONL(filepath.Join(out, "runs", strings.ReplaceAll(v.Name, "/", "_")+".jsonl"), rows); err != nil {
			return err
		}
		m := scoreScripts(rows, "dev", 1)
		fmt.Fprintf(os.Stderr, "%-20s dev: coverage %.2f top-1 %.2f slots %.2f exact %.2f\n", v.Name, m.Coverage, m.Top1, m.Slots, m.Exact)
	}
	return reportScripts(eng, model, results, model.Info(), samples, out, harnessOut)
}

type scriptMetrics struct {
	N, Positives, Negatives        int
	Coverage, Top1, Top3, Slots    float64 // over positives; Slots over the right picks
	Exact                          float64 // the whole query right, over positives
	Works                          float64 // a query that does it: the functional match, over positives
	NLL, ECE                       float64
	Threshold, Precision, ShownPos float64 // the ghost bar and what it yields
	ShownNeg                       float64 // negatives shown anyway
	P50, P95                       float64
}

// inSplit matches "dev", "test", "test:example", "dev:hand", or "" for all.
func inSplit(r ScriptResult, split string) bool {
	sp, src, _ := strings.Cut(split, ":")
	return (sp == "" || r.Split == sp) && (src == "" || r.Source == src)
}

func scoreScripts(rows []ScriptResult, split string, t float64) scriptMetrics {
	var m scriptMetrics
	var conf, correct, secs []float64
	right, slotsRight, top3, exact, covered, worksN := 0, 0, 0, 0, 0, 0
	nll := 0.0
	for _, r := range rows {
		if !inSplit(r, split) {
			continue
		}
		m.N++
		secs = append(secs, r.Seconds)
		p := suggest.Temper(r.Probs, t)
		o := argsort(p)
		if !r.Positive {
			m.Negatives++
			continue
		}
		m.Positives++
		if r.Works {
			worksN++
		}
		if r.LabelAt < 0 {
			conf, correct = append(conf, p[o[0]]), append(correct, 0)
			continue
		}
		covered++
		nll -= math.Log(math.Max(p[r.LabelAt], 1e-12))
		ok := o[0] == r.LabelAt && r.PickOK
		for _, i := range o[:min(3, len(o))] {
			if i == r.LabelAt {
				top3++
			}
		}
		if ok {
			right++
			if r.SlotsOK {
				slotsRight++
			}
			if r.Got == r.Want {
				exact++
			}
		}
		conf = append(conf, p[o[0]])
		correct = append(correct, map[bool]float64{true: 1, false: 0}[ok])
	}
	pos := math.Max(1, float64(m.Positives))
	m.Coverage = float64(covered) / pos
	m.Top1, m.Top3 = float64(right)/pos, float64(top3)/pos
	m.Slots = float64(slotsRight) / math.Max(1, float64(right))
	m.Exact = float64(exact) / pos
	m.Works = float64(worksN) / pos
	m.NLL = nll / math.Max(1, float64(covered))
	m.ECE = ece(conf, correct, 10)
	sort.Float64s(secs)
	if len(secs) > 0 {
		m.P50, m.P95 = secs[len(secs)/2], secs[min(len(secs)-1, len(secs)*95/100)]
	}
	return m
}

// ghostBar is the lowest top-1 confidence at which what is shown - the
// recipe picked, on positives and negatives alike - is the right recipe at
// least `precision` of the time.
func ghostBar(rows []ScriptResult, split string, t, precision float64) (th, prec, shownPos, shownNeg float64) {
	type pt struct {
		c       float64
		ok, neg bool
	}
	var pts []pt
	pos, neg := 0, 0
	for _, r := range rows {
		if !inSplit(r, split) {
			continue
		}
		p := suggest.Temper(r.Probs, t)
		o := argsort(p)
		pts = append(pts, pt{p[o[0]], r.Works, !r.Positive})
		if r.Positive {
			pos++
		} else {
			neg++
		}
	}
	measure := func(th float64) (float64, float64, float64) { // precision, positives shown, negatives shown
		shown, ok, sp, sn := 0, 0, 0, 0
		for _, x := range pts {
			if x.c >= th {
				shown++
				if x.ok {
					ok++
				}
				if x.neg {
					sn++
				} else {
					sp++
				}
			}
		}
		if shown == 0 {
			return 0, 0, 0
		}
		return float64(ok) / float64(shown), float64(sp) / math.Max(1, float64(pos)), float64(sn) / math.Max(1, float64(neg))
	}
	// The bar that shows the most right queries for the fewest wrong ones:
	// each right one shown +1, each wrong one -1 (a recipe for a comment no
	// recipe answers is wrong), with precision kept at the target.
	bestU := math.Inf(-1)
	th = 0.99
	for t := 0.05; t < 1; t += 0.01 {
		shown, ok := 0, 0
		for _, x := range pts {
			if x.c >= t {
				shown++
				if x.ok {
					ok++
				}
			}
		}
		u := float64(ok - (shown - ok))
		if p, _, _ := measure(t); p >= precision && u > bestU {
			bestU, th = u, math.Round(t*100)/100
		}
	}
	prec, shownPos, shownNeg = measure(th)
	return th, prec, shownPos, shownNeg
}

// applyBar scores a split under a bar fixed elsewhere.
func applyBar(rows []ScriptResult, split string, t, th float64) (prec, shownPos, shownNeg float64) {
	shown, ok, sp, sn, pos, neg := 0, 0, 0, 0, 0, 0
	for _, r := range rows {
		if !inSplit(r, split) {
			continue
		}
		if r.Positive {
			pos++
		} else {
			neg++
		}
		p := suggest.Temper(r.Probs, t)
		o := argsort(p)
		if p[o[0]] < th {
			continue
		}
		shown++
		if r.Positive {
			sp++
			if r.Works {
				ok++
			}
		} else {
			sn++
		}
	}
	return float64(ok) / math.Max(1, float64(shown)), float64(sp) / math.Max(1, float64(pos)), float64(sn) / math.Max(1, float64(neg))
}

func byIDRecipes() map[string]ideengine.Recipe {
	rs := ideengine.AllRecipes()
	out := map[string]ideengine.Recipe{}
	for _, r := range rs {
		out[r.ID] = r
	}
	return out
}

type contResult struct {
	conf  float64
	ok    bool
	split string
}

// runContinuations scores the rest of a recipe offered after part of it:
// each labelled comment's query, cut after each of its stages, asked what
// comes next on the following line. Right is the whole rest exactly; the
// bar is fitted on dev as the scripts' is, and test is reported.
func runContinuations(eng *ideengine.Engine, model *d1.Model, h suggest.Harness, samples []ScriptSample, byID map[string]ideengine.Recipe) (float64, string, error) {
	ranker := suggest.NewWith(model, h)
	var rows []contResult
	for _, s := range samples {
		rec, ok := byID[s.Recipe]
		if !ok {
			continue
		}
		want := ideengine.Fill(rec.Query, rec.Slots, s.Slots)
		stages := ideengine.Stages(want)
		for k := 1; k < len(stages); k++ {
			query := "# " + s.Comment + "\n" + strings.Join(stages[:k], "\n| ") + "\n"
			rest := "| " + strings.Join(stages[k:], "\n| ")
			req := ideengine.SuggestRequest{Query: query, Cursor: -1}
			sc := eng.SuggestContext(req)
			if len(sc.Candidates) == 0 {
				continue
			}
			a, err := ranker.Answer(context.Background(), sc)
			if err != nil {
				return 0, "", err
			}
			top := argsort(a.Probabilities)[0]
			got := sc.Lead + ideengine.FillCandidate(ideengine.Choose(sc, ideengine.Ranking{Probabilities: a.Probabilities, Members: a.Members}, top), a.Slots, sc.Comment())
			rows = append(rows, contResult{a.Probabilities[top] * a.More, got == rest, s.Split})
		}
	}
	type acc struct{ shown, ok, n int }
	measure := func(split string, th float64) acc {
		var a acc
		for _, r := range rows {
			if r.split != split {
				continue
			}
			a.n++
			if r.conf >= th {
				a.shown++
				if r.ok {
					a.ok++
				}
			}
		}
		return a
	}
	best, bestU := 0.99, math.Inf(-1)
	for t := 0.05; t < 1; t += 0.01 {
		a := measure("dev", t)
		if a.shown == 0 || float64(a.ok)/float64(a.shown) < ghostPrecision {
			continue
		}
		if u := float64(a.ok - (a.shown - a.ok)); u > bestU {
			best, bestU = math.Round(t*100)/100, u
		}
	}
	right := func(split string) (int, int) {
		ok, n := 0, 0
		for _, r := range rows {
			if r.split == split {
				n++
				if r.ok {
					ok++
				}
			}
		}
		return ok, n
	}
	dok, dn := right("dev")
	tok, tn := right("test")
	d, t := measure("dev", best), measure("test", best)
	var j strings.Builder
	fmt.Fprintf(&j, "\n## Continuing a recipe\n\nEach labelled query cut after each stage, the cursor on the next line: the top suggestion is the exact rest on %d of %d dev cuts and %d of %d test cuts. ", dok, dn, tok, tn)
	fmt.Fprintf(&j, "Ghost bar %.2f (dev: shown %d of %d, right %d; test: shown %d of %d, right %d).\n", best, d.shown, d.n, d.ok, t.shown, t.n, t.ok)
	return best, j.String(), nil
}

func reportScripts(eng *ideengine.Engine, model *d1.Model, results map[string][]ScriptResult, info d1.Info, samples []ScriptSample, out, harnessOut string) error {
	var names []string
	for n := range results {
		names = append(names, n)
	}
	sort.Strings(names)
	best, bestScore := "", -1.0
	var j strings.Builder
	fmt.Fprintf(&j, "# Journal: the whole-query question on d1-2B\n\nRun %s on %s (%s), %d comments (dev %d, test %d).\n\n",
		time.Now().UTC().Format("2006-01-02"), info.Device, info.Variant, len(samples), countSplit(samples, "dev"), countSplit(samples, "test"))
	fmt.Fprintf(&j, "Dev split, raw (T=1):\n\n| variant | coverage | top-1 | top-3 | slots | exact | works | NLL | ECE | negatives shown | p50 s |\n|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, n := range names {
		m := scoreScripts(results[n], "dev", 1)
		_, _, _, sn := ghostBar(results[n], "dev", 1, ghostPrecision)
		fmt.Fprintf(&j, "| %s | %.3f | %.3f | %.3f | %.3f | %.3f | %.3f | %.3f | %.3f | %.0f%% | %.3f |\n", n, m.Coverage, m.Top1, m.Top3, m.Slots, m.Exact, m.Works, m.NLL, m.ECE, 100*sn, m.P50)
		// Exact queries first; a ghost over a comment no recipe answers is
		// the cost that decides between near ties.
		if score := m.Works - 0.05*sn + 0.01*m.Exact; score > bestScore {
			best, bestScore = n, score
		}
	}
	rows := results[best]
	// temperature by NLL on dev
	bt, bnll := 1.0, math.Inf(1)
	for _, t := range temperatures {
		if m := scoreScripts(rows, "dev", t); m.NLL < bnll {
			bt, bnll = t, m.NLL
		}
	}
	th, prec, sp, sn := ghostBar(rows, "dev", bt, ghostPrecision)
	test := scoreScripts(rows, "test", bt)
	tprec, tsp, tsn := applyBar(rows, "test", bt, th)
	raw := scoreScripts(rows, "test", 1)
	fmt.Fprintf(&j, "\nChosen: **%s** (most working queries on dev). Temperature %.3f by dev NLL; ghost bar %.2f, the most right for the fewest wrong at precision %.2f or more on dev (precision %.3f, shown on %.0f%% of positives and %.0f%% of negatives).\n\n",
		best, bt, th, ghostPrecision, prec, 100*sp, 100*sn)
	fmt.Fprintf(&j, "Test split, scored once:\n\n| | coverage | top-1 | top-3 | slots | exact | works | NLL | ECE |\n|---|---|---|---|---|---|---|---|---|\n")
	fmt.Fprintf(&j, "| raw (T=1) | %.3f | %.3f | %.3f | %.3f | %.3f | %.3f | %.3f | %.3f |\n", raw.Coverage, raw.Top1, raw.Top3, raw.Slots, raw.Exact, raw.Works, raw.NLL, raw.ECE)
	fmt.Fprintf(&j, "| tempered | %.3f | %.3f | %.3f | %.3f | %.3f | %.3f | %.3f | %.3f |\n", test.Coverage, test.Top1, test.Top3, test.Slots, test.Exact, test.Works, test.NLL, test.ECE)
	for _, src := range []string{"hand", "example"} {
		m := scoreScripts(rows, "test:"+src, bt)
		fmt.Fprintf(&j, "| tempered, %s comments (%d) | %.3f | %.3f | %.3f | %.3f | %.3f | %.3f | %.3f | %.3f |\n", src, m.N, m.Coverage, m.Top1, m.Top3, m.Slots, m.Exact, m.Works, m.NLL, m.ECE)
	}
	j.WriteString("\n")
	fmt.Fprintf(&j, "Ghost text on test at the dev bar: precision %.3f, shown on %.0f%% of positives and %.0f%% of negatives.\n\n", tprec, 100*tsp, 100*tsn)
	fmt.Fprintf(&j, "Misses on test (chosen variant):\n\n| comment | wanted | got |\n|---|---|---|\n")
	byID := map[string]ScriptSample{}
	for _, s := range samples {
		byID[s.ID] = s
	}
	for _, r := range rows {
		if r.Split != "test" || !r.Positive || r.Source != "hand" {
			continue
		}
		top := argsort(r.Probs)[0]
		got := r.Picked
		_ = top
		if r.Works {
			continue
		}
		why := got
		if got == r.Recipe {
			why = "slots: `" + strings.ReplaceAll(firstDiff(r.Want, r.Got), "|", "\\|") + "`"
		} else if r.LabelAt < 0 {
			why += " (not a candidate)"
		}
		fmt.Fprintf(&j, "| %s | %s | %s |\n", strings.ReplaceAll(byID[r.ID].Comment, "|", "\\|"), r.Recipe, why)
	}
	if err := os.WriteFile(filepath.Join(out, "JOURNAL.md"), []byte(j.String()), 0o644); err != nil {
		return err
	}
	if err := writeExampleMisses(rows, byID, filepath.Join(out, "EXAMPLES.md")); err != nil {
		return err
	}

	var f strings.Builder
	fmt.Fprintf(&f, "# Findings: writing a whole query from a comment with d1-2B\n\n")
	fmt.Fprintf(&f, "- Question: `%s`.\n", best)
	fmt.Fprintf(&f, "- On held-out comments (test, %d with a recipe, %d without): the right recipe is a candidate %.0f%% of the time, picked first %.0f%% (top-3 %.0f%%); when it is, the slots are all right %.0f%%; the whole query comes out exactly right for %.0f%% of comments.\n",
		test.Positives, test.Negatives, 100*test.Coverage, 100*test.Top1, 100*test.Top3, 100*test.Slots, 100*test.Exact)
	for _, src := range []string{"hand", "example"} {
		m := scoreScripts(rows, "test:"+src, bt)
		what := map[string]string{"hand": "hand-written comments for the hand recipes", "example": "the catalog examples' intents (one comment an example, every example)"}[src]
		fmt.Fprintf(&f, "  - %s (%d): a query that does it %.0f%% (the labelled recipe first %.0f%%, its exact query %.0f%%).\n", what, m.N, 100*m.Works, 100*m.Top1, 100*m.Exact)
	}
	fmt.Fprintf(&f, "- Calibration: temperature %.2f takes test NLL from %.3f to %.3f and ECE from %.3f to %.3f.\n", bt, raw.NLL, test.NLL, raw.ECE, test.ECE)
	fmt.Fprintf(&f, "- Ghost text appears unasked at confidence %.2f: on test it is the right recipe %.0f%% of the time, shows for %.0f%% of comments a recipe does and %.0f%% of those none does.\n", th, 100*tprec, 100*tsp, 100*tsn)
	fmt.Fprintf(&f, "- Latency (%s): p50 %.3f s, p95 %.3f s, choice and slot questions in one pass.\n", info.Device, test.P50, test.P95)
	fmt.Fprintf(&f, "\nCaveats. The eval set is small (%d comments) and was written by the same hand as the recipes; the recipes' retrieval keywords (`# also:`) were last revised after reading a test run's misses, so the test numbers are somewhat optimistic. Comments for tasks no recipe does are the weak spot: a few still get a confident wrong recipe.\n", len(samples))
	fmt.Fprintf(&f, "\nRe-run with `make d1.calibrate` after changing recipes.pwrq, the eval set or the question.\n")
	if err := os.WriteFile(filepath.Join(out, "FINDINGS.md"), []byte(f.String()), 0o644); err != nil {
		return err
	}
	h := suggest.DefaultHarness()
	if raw, err := os.ReadFile(harnessOut); err == nil {
		_ = json.Unmarshal(raw, &h)
	}
	for _, v := range scriptVariants {
		if v.Name == best {
			h.Script, h.ScriptOptionStyle, h.ScriptNone = v.Question, v.Style, v.None
		}
	}
	h.ScriptTemperature, h.ScriptThreshold = bt, th
	h.PromptDigest = h.Digest()
	cth, cj, err := runContinuations(eng, model, h, samples, byIDRecipes())
	if err != nil {
		return err
	}
	h.ContinueThreshold = cth
	j.WriteString(cj)
	if err := os.WriteFile(filepath.Join(out, "JOURNAL.md"), []byte(j.String()), 0o644); err != nil {
		return err
	}
	rawH, _ := json.MarshalIndent(h, "", "  ")
	if err := os.WriteFile(harnessOut, append(rawH, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Fprint(os.Stderr, j.String())
	return nil
}

var cmdletNames = func() map[string]bool {
	out := map[string]bool{}
	for _, c := range discovery.Catalog() {
		out[c.Name] = true
	}
	return out
}

var identOnlyRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

// calls are the cmdlets a query names.
func calls(query string, names map[string]bool) map[string]bool {
	out := map[string]bool{}
	for _, w := range identOnlyRe.FindAllString(query, -1) {
		if names[w] {
			out[w] = true
		}
	}
	return out
}

// works is the functional match: got calls every cmdlet want does, and
// holds every value the comment states.
func works(want, got string, stated map[string]string) bool {
	if want == "" {
		return false
	}
	names := cmdletNames()
	have := calls(got, names)
	for c := range calls(want, names) {
		if !have[c] {
			return false
		}
	}
	for _, v := range stated {
		if !strings.Contains(got, v) {
			if n, ok := ideengine.NormalizeSlot("size", v); !ok || !strings.Contains(got, n) {
				return false
			}
		}
	}
	return true
}

func countSplit(s []ScriptSample, split string) int {
	n := 0
	for _, x := range s {
		if x.Split == split {
			n++
		}
	}
	return n
}

// firstDiff shows where two queries part, for the journal.
func firstDiff(a, b string) string {
	i := 0
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	from := max(0, i-20)
	end := func(s string) string { return s[from:min(len(s), i+30)] }
	return end(a) + " → " + end(b)
}

// writeExampleMisses lists every example whose intent did not come back as
// its own query - dev and test alike - with what came instead: where an
// example, its description or its intent needs work.
func writeExampleMisses(rows []ScriptResult, byID map[string]ScriptSample, path string) error {
	var b strings.Builder
	b.WriteString("# Examples the scripts study misses\n\nEvery catalog example has a comment in intents.jsonl. These are the ones whose comment did not come back as a query that does it - the example's cmdlets with the values the comment states - with what came instead. Fix the example, the cmdlet's description, or the comment if it is not what someone would write - then `make d1.calibrate`.\n\n| example | comment | got |\n|---|---|---|\n")
	n, missed := 0, 0
	for _, r := range rows {
		if r.Source != "example" {
			continue
		}
		n++
		top := argsort(r.Probs)[0]
		got := r.Picked
		_ = top
		if r.Works {
			continue
		}
		missed++
		why := got
		switch {
		case got == r.Recipe:
			why = "slots: `" + strings.ReplaceAll(firstDiff(r.Want, r.Got), "|", "\\|") + "`"
		case r.LabelAt < 0:
			why += " (not a candidate)"
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", r.Recipe, strings.ReplaceAll(byID[r.ID].Comment, "|", "\\|"), why)
	}
	fmt.Fprintf(&b, "\n%d of %d examples missed.\n", missed, n)
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
