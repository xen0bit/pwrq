package prompt

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/xen0bit/pwrq/pkg/d1/parsing"
)

// Calibration is a release's calibration.json: a temperature for option
// scores, and monotone tables for typed answers' probabilities.
type Calibration struct {
	Temperatures map[string]float64      `json:"temperatures"`
	Default      float64                 `json:"default"`
	Tables       map[string][][2]float64 `json:"tables"`
}

// ParseCalibration reads calibration.json.
func ParseCalibration(raw []byte) (*Calibration, error) {
	c := &Calibration{Default: 1}
	if err := json.Unmarshal(raw, c); err != nil {
		return nil, fmt.Errorf("calibration: %w", err)
	}
	if c.Default == 0 {
		c.Default = 1
	}
	return c, nil
}

func bucket(s Slot) string {
	switch s.kind {
	case slotNoul:
		return "noul"
	case slotScore:
		return "score:" + strconv.Itoa(s.levels)
	}
	k := s.cardinality()
	switch {
	case k <= 3:
		return "choice:small"
	case k <= 8:
		return "choice:mid"
	case k <= 26:
		return "choice:large"
	}
	return "choice:inventory"
}

func (c *Calibration) temperature(s Slot) float64 {
	if c == nil {
		return 1
	}
	if t, ok := c.Temperatures[bucket(s)]; ok {
		return t
	}
	return c.Default
}

// through maps value through increasing (raw, calibrated) knots, linearly
// between them and flat past the ends.
func through(value float64, knots [][2]float64) float64 {
	if len(knots) == 0 {
		return value
	}
	index := sort.Search(len(knots), func(i int) bool { return knots[i][0] >= value })
	if index == 0 {
		return knots[0][1]
	}
	if index == len(knots) {
		return knots[len(knots)-1][1]
	}
	a, b := knots[index-1], knots[index]
	return a[1] + (value-a[0])*(b[1]-a[1])/(b[0]-a[0])
}

func (c *Calibration) through(name string, value float64) float64 {
	if c == nil {
		return value
	}
	return through(value, c.Tables[name])
}

// level is the level whose observed share is share: the table read backwards.
func (c *Calibration) level(name string, share float64) float64 {
	if c == nil {
		return share
	}
	t := c.Tables[name]
	inv := make([][2]float64, len(t))
	for i, k := range t {
		inv[i] = [2]float64{k[1], k[0]}
	}
	return through(share, inv)
}

// readout is a slot's option probabilities from its row of logits: each
// option scores its best form, through the temperature when tempered, then
// a softmax across options.
func (b *Builder) readout(s Slot, logits []float32, cal *Calibration, tempered bool) ([]float64, error) {
	groups, err := b.readoutIDs(s)
	if err != nil {
		return nil, err
	}
	// log-softmax over the vocabulary is a shift, which the softmax across
	// options ignores; it is kept so a temperature divides the same values
	// the reference divides.
	m := math.Inf(-1)
	for _, x := range logits {
		m = math.Max(m, float64(x))
	}
	sum := 0.0
	for _, x := range logits {
		sum += math.Exp(float64(x) - m)
	}
	lse := m + math.Log(sum)
	scores := make([]float64, len(groups))
	for i, g := range groups {
		best := math.Inf(-1)
		for _, id := range g {
			if int(id) < len(logits) {
				best = math.Max(best, float64(logits[id])-lse)
			}
		}
		scores[i] = best
	}
	if tempered && cal != nil {
		if t := cal.temperature(s); t != 1 {
			for i := range scores {
				scores[i] /= t
			}
		}
	}
	return softmax(scores), nil
}

func softmax(scores []float64) []float64 {
	m := math.Inf(-1)
	for _, s := range scores {
		m = math.Max(m, s)
	}
	exps := make([]float64, len(scores))
	total := 0.0
	for i, s := range scores {
		exps[i] = math.Exp(s - m)
		total += exps[i]
	}
	for i := range exps {
		exps[i] /= total
	}
	return exps
}

// Score readouts: how a score's distribution becomes its answer.
const (
	ScoreMedian = "median" // the first level whose cumulative probability reaches one half (the default)
	ScoreMean   = "mean"   // the expected level, unrounded
	ScoreArgmax = "argmax" // the likeliest level
)

// AnswerOptions shape answers.
type AnswerOptions struct {
	ScoreReadout string   // ScoreMedian when empty
	AbstainBelow *float64 // pick a choice's "I don't know" option below this share
}

var idkTexts = map[string]bool{"i don't know": true, "i do not know": true, "i don't know the answer": true, "i do not know the answer": true}

func abstainOption(q Question) int {
	for i, o := range q.Options {
		text := o.Name
		if o.Desc != nil && *o.Desc != "" {
			text = *o.Desc
		}
		t := strings.ToLower(strings.TrimSpace(text))
		t = strings.ReplaceAll(t, "’", "'")
		t = strings.TrimRight(t, ".!")
		if idkTexts[t] {
			return i
		}
	}
	return -1
}

func argmax(ps []float64) int {
	best := 0
	for i, p := range ps {
		if p > ps[best] {
			best = i
		}
	}
	return best
}

func choiceIndex(probs []float64, abstain int, below *float64) int {
	best := argmax(probs)
	if abstain < 0 || below == nil {
		return best
	}
	var others []float64
	total := 0.0
	for i, p := range probs {
		if i != abstain {
			others = append(others, p)
			total += p
		}
	}
	mx := 0.0
	for _, p := range others {
		mx = math.Max(mx, p)
	}
	if total <= 0 || mx < *below*total {
		return abstain
	}
	return best
}

func scoreLevel(probs []float64, readout string) int {
	total := 0.0
	for _, p := range probs {
		total += p
	}
	if total == 0 {
		total = 1
	}
	switch readout {
	case ScoreMean:
		e := 0.0
		for i, p := range probs {
			e += float64(i) * p / total
		}
		return max(0, min(len(probs)-1, int(pyRound(e))))
	case ScoreArgmax:
		return argmax(probs)
	}
	mass := 0.0
	for i, p := range probs {
		mass += p / total
		if mass >= 0.5 {
			return i
		}
	}
	return len(probs) - 1
}

// readResult is a read's pointer outcome: what it points at and P(found).
type readResult struct {
	Span  Span
	Found float64
}

// answer builds one question's answer as d1's API gives it. probs holds one
// distribution per slot (a read uses read instead).
func (b *Builder) answer(q Question, probs [][]float64, read *readResult, context string, cal *Calibration, opts AnswerOptions) (map[string]any, error) {
	switch {
	case q.Type == Box:
		return boxAnswer(probs, cal, b.cfg.BoxEdges), nil
	case q.Type == Boxes:
		return boxesAnswer(probs, cal, b.cfg.BoxEdges), nil
	case q.Estimate != nil:
		var level func(float64) float64
		if cal != nil {
			level = func(share float64) float64 { return cal.level("estimate", share) }
		}
		r := q.Estimate.Read(probs[0], level)
		return map[string]any{"type": Number, "number": r.Median, "interval": []any{r.P10, r.P90}, "unit": strPtr(q.Unit)}, nil
	case q.IsRead():
		found := cal.through("found:"+q.Type, read.Found)
		present := found >= 0.5
		var text *string
		var span any
		if present {
			text = read.Span.Text
			if read.Span.Range != nil {
				span = []any{read.Span.Range[0], read.Span.Range[1]}
			}
		}
		var value any
		if text != nil {
			value = *text
		}
		var unit *string
		if q.Unit != nil {
			unit = q.Unit
		}
		if text != nil && q.Type == Number {
			if unit == nil {
				if u, ok := parsing.WrittenUnit(*text); ok {
					unit = &u
				}
			}
			given := ""
			if q.Unit != nil {
				given = *q.Unit
			}
			if v, ok := parsing.ParseNumber(*text, context, given); ok {
				value = v
			} else {
				value = nil
			}
		} else if text != nil && q.Type == Date {
			if v, _ := parsing.ParseDate(*text, context); v != "" {
				value = v
			} else {
				value = nil
			}
		}
		out := map[string]any{"type": q.Type, q.Type: value, "span": span, "found": found}
		if q.Type != Text {
			out["text"] = strPtr(text)
		}
		if present && read.Span.Path != "" {
			out["path"] = read.Span.Path
		}
		if q.Type == Number {
			out["unit"] = strPtr(unit)
		}
		return out, nil
	case q.Type == Noul:
		return map[string]any{"type": Noul, "noul": probs[0][0]}, nil
	case q.Type == Choice && q.Multiple:
		chosen := []any{}
		ps := map[string]any{}
		for i, o := range q.Options {
			p := cal.through("multiple", probs[i][0])
			ps[o.Name] = p
			if p >= 0.5 {
				chosen = append(chosen, o.Name)
			}
		}
		return map[string]any{"type": Choice, "choices": chosen, "probabilities": ps}, nil
	case q.Type == Choice:
		p := probs[0]
		idk := -1
		if opts.AbstainBelow != nil {
			idk = abstainOption(q)
		}
		pick := choiceIndex(p, idk, opts.AbstainBelow)
		ps := map[string]any{}
		for i, o := range q.Options {
			ps[o.Name] = p[i]
		}
		return map[string]any{"type": Choice, "choice": q.Options[pick].Name, "confidence": p[pick], "probabilities": ps}, nil
	}
	// score
	p := probs[0]
	readout := opts.ScoreReadout
	if readout == "" {
		readout = ScoreMedian
	}
	var level float64
	if readout == ScoreMean {
		for i, x := range p {
			level += float64(i) * x
		}
	} else {
		level = float64(scoreLevel(p, readout))
	}
	ps, legend := map[string]any{}, map[string]any{}
	for i, x := range p {
		ps[strconv.Itoa(i)] = x
		legend[strconv.Itoa(i)] = q.Levels[i]
	}
	return map[string]any{"type": Score, "score": level, "confidence": p[argmax(p)], "probabilities": ps, "legend": legend}, nil
}

func strPtr(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

// --- boxes (box.py) ---

const draws = 512

// halton is DRAWS points of a Halton sequence in four dimensions: the same
// draws every call, so a box's confidence is deterministic.
var halton = func() [][4]float64 {
	radical := func(i, base int) float64 {
		v, scale := 0.0, 1.0
		for i > 0 {
			digit := i % base
			i /= base
			scale /= float64(base)
			v += float64(digit) * scale
		}
		return v
	}
	out := make([][4]float64, draws)
	for i := range out {
		for k, base := range []int{2, 3, 5, 7} {
			out[i][k] = radical(i+1, base)
		}
	}
	return out
}()

func readAs(dist []float64) Grid {
	if len(dist) == thousandths.Size {
		return thousandths
	}
	return codeGrid
}

func ordered(a, b float64, g Grid) (float64, float64) {
	if a < b {
		return a, b
	}
	half := g.Step() / 2
	middle := math.Min(1-half, math.Max(half, (a+b)/2))
	return middle - half, middle + half
}

type rect struct{ top, bottom, left, right float64 }

func rectangle(e [4]float64, g Grid) rect {
	t, bt := ordered(e[0], e[1], g)
	l, r := ordered(e[2], e[3], g)
	return rect{t, bt, l, r}
}

func (r rect) area() float64 { return (r.bottom - r.top) * (r.right - r.left) }

func iou(a, b rect) float64 {
	overlap := math.Max(0, math.Min(a.bottom, b.bottom)-math.Max(a.top, b.top)) *
		math.Max(0, math.Min(a.right, b.right)-math.Max(a.left, b.left))
	return overlap / (a.area() + b.area() - overlap)
}

// boxConfidence is the share of boxes drawn from the edge distributions
// (through each one's CDF) with IoU >= 0.5 against box.
func boxConfidence(edgeDists [][]float64, box rect) float64 {
	g := readAs(edgeDists[0])
	values := g.Values()
	cdfs := make([][]float64, 4)
	for k, row := range edgeDists {
		total := fsum(row)
		acc := 0.0
		cdfs[k] = make([]float64, len(row))
		for i, p := range row {
			acc += p / total
			cdfs[k][i] = acc
		}
	}
	hits := 0
	for _, point := range halton {
		var e [4]float64
		for k := range 4 {
			idx := sort.SearchFloat64s(cdfs[k], point[k])
			e[k] = values[min(g.Size-1, idx)]
		}
		if iou(rectangle(e, g), box) >= 0.5 {
			hits++
		}
	}
	return float64(hits) / draws
}

// crossing is where a distribution's CDF crosses one half, the mass spread
// evenly over each bin.
func crossing(probs []float64, g Grid) float64 {
	total, mass := fsum(probs), 0.0
	values := g.Values()
	for i, p := range probs {
		share := p / total
		if share > 0 && mass+share >= 0.5 {
			return math.Min(g.High, math.Max(g.Low, values[i]-g.Step()/2+(0.5-mass)/share*g.Step()))
		}
		mass += share
	}
	return g.High
}

func medians(edgeDists [][]float64, read string) rect {
	g := readAs(edgeDists[0])
	var e [4]float64
	for k, row := range edgeDists {
		if read == "interp" {
			e[k] = crossing(row, g)
		} else {
			e[k] = g.Read(row, nil).Median
		}
	}
	return rectangle(e, g)
}

func rectObject(r rect) map[string]any {
	return map[string]any{"top": r.top, "bottom": r.bottom, "left": r.left, "right": r.right}
}

func boxAnswer(probs [][]float64, cal *Calibration, read string) map[string]any {
	found := cal.through("found:box", probs[0][0])
	if found < 0.5 {
		return map[string]any{"type": Box, "box": nil, "found": found, "confidence": 0.0}
	}
	box := medians(probs[1:5], read)
	return map[string]any{"type": Box, "box": rectObject(box), "found": found,
		"confidence": cal.through("box", boxConfidence(probs[1:5], box))}
}

func boxesAnswer(probs [][]float64, cal *Calibration, read string) map[string]any {
	var presence []float64
	below := 1.0
	for k := 0; k < len(probs); k += 5 {
		below = math.Min(below, cal.through("found:boxes", probs[k][0]))
		presence = append(presence, below)
	}
	type found struct {
		box        rect
		found, cnf float64
	}
	var out []found
	for k, f := range presence {
		if f < 0.5 {
			break
		}
		block := probs[5*k+1 : 5*k+5]
		box := medians(block, read)
		out = append(out, found{box, f, cal.through("boxes", boxConfidence(block, box))})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].box.left != out[j].box.left {
			return out[i].box.left < out[j].box.left
		}
		return out[i].box.top < out[j].box.top
	})
	boxes := make([]any, len(out))
	for i, f := range out {
		boxes[i] = map[string]any{"box": rectObject(f.box), "found": f.found, "confidence": f.cnf}
	}
	pres := make([]any, len(presence))
	for i, p := range presence {
		pres[i] = p
	}
	return map[string]any{"type": Boxes, "boxes": boxes, "count": len(out),
		"at_least": len(out) > 0 && len(out) == len(presence), "presence": pres}
}
