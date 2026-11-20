package prompt

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strings"
)

// Question types, as the API names them.
const (
	Noul   = "noul"
	Choice = "choice"
	Score  = "score"
	Text   = "text"
	Number = "number"
	Date   = "date"
	Box    = "box"
	Boxes  = "boxes"
)

// QuestionTypes lists every type a question may have.
var QuestionTypes = []string{Noul, Choice, Score, Text, Number, Date, Box, Boxes}

// Instances is how many blocks a boxes question reads without max, and the
// most it may ask: prompt version 1 trained 16 and says "16 at most".
const Instances = 16

// Option is one of a choice's options: its name and what it means (nil: the
// name, its underscores read as spaces).
type Option struct {
	Name string
	Desc *string
}

// Question is one typed question, parsed and checked.
type Question struct {
	Type         string
	Instructions string

	Options  []Option // choice
	Multiple bool     // choice: each option its own yes or no
	Levels   []string // score, lowest first
	NoulYes  *string  // noul criteria, when given
	NoulNo   *string
	HasNoul  bool
	Unit     *string // number: a read's or an estimate's unit
	Estimate *Grid   // number with a range
	Max      int     // boxes: blocks read (0: Instances)
}

// Named is a question with its caller's name.
type Named struct {
	Name     string
	Question Question
}

// Typed reports whether the answer is typed, read at its own slots and
// calibrated by its own table rather than by the temperature: a read, an
// estimate, a box or boxes, a multiple choice.
func (q Question) Typed() bool {
	switch q.Type {
	case Text, Number, Date, Box, Boxes:
		return true
	}
	return q.Type == Choice && q.Multiple
}

// IsRead reports whether the question reads a span of the state.
func (q Question) IsRead() bool {
	return (q.Type == Text || q.Type == Date || q.Type == Number) && q.Estimate == nil
}

func (q Question) blocks() int {
	if q.Max == 0 {
		return Instances
	}
	return q.Max
}

// ParseQuestions checks a request's questions, in the caller's order.
func ParseQuestions(raw any) ([]Named, error) {
	o, ok := members(raw)
	if !ok {
		return nil, fmt.Errorf(`questions are an object of name to question, {"refund": {"type": "noul", "instructions": "Is it a refund request?"}}, not %s`, kindOf(raw))
	}
	if len(o) == 0 {
		return nil, fmt.Errorf("questions is empty")
	}
	out := make([]Named, 0, len(o))
	for _, m := range o {
		q, err := ParseQuestion(m.Value)
		if err != nil {
			return nil, fmt.Errorf("question %q: %w", m.Key, err)
		}
		out = append(out, Named{m.Key, q})
	}
	return out, nil
}

var questionKeys = map[string]bool{
	"type": true, "instructions": true, "criteria": true, "multiple": true,
	"range": true, "step": true, "unit": true, "max": true,
}

// ParseQuestion checks one question, refusing it with what to change: the
// reference's check_question and as_question, and stricter in one way - a
// key no question has is an error here, where the reference ignores it. A
// question with "criterion" instead of "criteria" would otherwise silently
// lose its options.
func ParseQuestion(raw any) (Question, error) {
	o, ok := members(raw)
	if !ok {
		return Question{}, fmt.Errorf(`a question is an object such as {"type": "noul", "instructions": "Is it urgent?"}, not %s`, kindOf(raw))
	}
	for _, m := range o {
		if !questionKeys[m.Key] {
			return Question{}, fmt.Errorf("unknown key %q; a question has type, instructions, criteria, and for some types multiple, range, step, unit or max", m.Key)
		}
	}
	get := func(k string) (any, bool) { return lookup(raw, k) }
	kindRaw, _ := get("type")
	kind, _ := kindRaw.(string)
	known := false
	for _, t := range QuestionTypes {
		known = known || t == kind
	}
	if !known {
		return Question{}, fmt.Errorf(`"type" is one of %s, not %s`, strings.Join(QuestionTypes, ", "), describe(kindRaw))
	}
	said, _ := get("instructions")
	text, ok := said.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return Question{}, fmt.Errorf(`"instructions" is the question as text`)
	}
	q := Question{Type: kind, Instructions: text}
	criteria, hasCriteria := get("criteria")
	if criteria == nil {
		hasCriteria = false
	}

	switch kind {
	case Choice:
		opts, ok := members(criteria)
		if !ok || len(opts) == 0 {
			hint := ""
			if _, isList := criteria.([]any); isList {
				hint = "; a list of levels is a score"
			}
			return Question{}, fmt.Errorf(`a choice's "criteria" names the options: {"billing": "Charges and refunds", "shipping": "Delivery"}%s`, hint)
		}
		for _, m := range opts {
			opt := Option{Name: m.Key}
			switch d := m.Value.(type) {
			case nil:
			case string:
				opt.Desc = &d
			default:
				return Question{}, fmt.Errorf(`a choice's "criteria" names the options: {"billing": "Charges and refunds", "shipping": "Delivery"}; option %q is described by %s, not text`, m.Key, kindOf(m.Value))
			}
			q.Options = append(q.Options, opt)
		}
	case Score:
		levels, ok := criteria.([]any)
		if !ok || len(levels) < 2 || len(levels) > 10 {
			return Question{}, fmt.Errorf(`a score's "criteria" lists 2 to 10 levels, lowest first: ["Can wait", "Today", "Now"]`)
		}
		for _, l := range levels {
			s, ok := l.(string)
			if !ok {
				return Question{}, fmt.Errorf(`a score's "criteria" lists 2 to 10 levels, lowest first: ["Can wait", "Today", "Now"]`)
			}
			q.Levels = append(q.Levels, s)
		}
	case Noul:
		if hasCriteria {
			c, ok := members(criteria)
			if !ok {
				return Question{}, fmt.Errorf(`a noul's "criteria", when given, is {"true": "what yes means", "false": "what no means"}`)
			}
			for _, m := range c {
				if m.Key != "true" && m.Key != "false" {
					return Question{}, fmt.Errorf(`a noul's "criteria" has only true and false, not %q`, m.Key)
				}
				s, ok := m.Value.(string)
				if !ok && m.Value != nil {
					return Question{}, fmt.Errorf(`a noul's criteria %q is text, not %s`, m.Key, kindOf(m.Value))
				}
				if m.Value != nil {
					if m.Key == "true" {
						q.NoulYes = &s
					} else {
						q.NoulNo = &s
					}
				}
			}
			q.HasNoul = len(c) > 0
		}
	default:
		if hasCriteria {
			return Question{}, fmt.Errorf(`a %s question has no "criteria"`, kind)
		}
	}

	if m, ok := get("multiple"); ok && m != nil {
		b, isBool := m.(bool)
		if !isBool {
			return Question{}, fmt.Errorf(`"multiple" is true or false`)
		}
		if b && kind != Choice {
			return Question{}, fmt.Errorf(`"multiple" belongs to a choice: {"type": "choice", "multiple": true, "criteria": {...}}`)
		}
		q.Multiple = b
	}
	if m, ok := get("max"); ok {
		n, isInt := wholeNumber(m)
		if kind != Boxes || !isInt || n < 1 || n > Instances {
			return Question{}, fmt.Errorf(`"max" belongs to boxes, a whole number from 1 to %d: {"type": "boxes", "instructions": "Every cup.", "max": 4}`, Instances)
		}
		q.Max = int(n)
	}
	if u, ok := get("unit"); ok && u != nil {
		s, isStr := u.(string)
		if !isStr || kind != Number {
			return Question{}, fmt.Errorf(`"unit" belongs to a number, as text: {"type": "number", "unit": "EUR"}`)
		}
		q.Unit = &s
	}
	step, hasStep := get("step")
	if hasStep && step == nil {
		hasStep = false
	}
	bounds, hasRange := get("range")
	if hasStep && !hasRange {
		return Question{}, fmt.Errorf(`"step" needs a "range": {"type": "number", "range": [0, 10], "step": 1}`)
	}
	if hasRange {
		pair, ok := bounds.([]any)
		var lo, hi float64
		okLo, okHi := false, false
		if ok && len(pair) == 2 {
			lo, okLo = toFloat(pair[0])
			hi, okHi = toFloat(pair[1])
		}
		if kind != Number || !okLo || !okHi {
			return Question{}, fmt.Errorf(`an estimate is {"type": "number", "range": [low, high]}, with an optional "step"`)
		}
		var st *float64
		if hasStep {
			s, ok := toFloat(step)
			if !ok {
				return Question{}, fmt.Errorf(`an estimate's "step" is a number above 0`)
			}
			st = &s
		}
		if !(lo < hi) || st != nil && !(*st > 0 && isClose((hi-lo) / *st, pyRound((hi-lo) / *st))) {
			stepText := "None"
			if st != nil {
				stepText = pyG(*st)
			}
			return Question{}, fmt.Errorf("an estimate's range runs low to high and its step, above 0, divides it: not [%s, %s] in steps of %s", pyG(lo), pyG(hi), stepText)
		}
		size := 128
		if st != nil {
			size = min(128, int(pyRound((hi-lo) / *st))+1)
		}
		g, err := NewGrid(lo, hi, size)
		if err != nil {
			return Question{}, err
		}
		q.Estimate = &g
	}
	return q, nil
}

// isClose is math.isclose with its default relative tolerance.
func isClose(a, b float64) bool {
	return math.Abs(a-b) <= math.Max(1e-9*math.Max(math.Abs(a), math.Abs(b)), 0)
}

// pyRound is Python's round(): half to even.
func pyRound(x float64) float64 { return math.RoundToEven(x) }

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case bool:
		return 0, false
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case *big.Int:
		f, _ := new(big.Float).SetInt(n).Float64()
		return f, true
	}
	return 0, false
}

func wholeNumber(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	case float64:
		if n == math.Trunc(n) {
			return int64(n), true
		}
	}
	return 0, false
}

func kindOf(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case string:
		return "text"
	case bool:
		return "a boolean"
	case []any:
		return "an array"
	case map[string]any, Object:
		return "an object"
	}
	return "a number"
}

func describe(v any) string {
	if s, ok := v.(string); ok {
		return fmt.Sprintf("%q", s)
	}
	return kindOf(v)
}

// Grid is an ordered set of equally spaced values a distribution is read
// over: an estimate's, or a box edge's.
type Grid struct {
	Low, High float64
	Size      int
}

// NewGrid checks a grid of two values or more, low to high.
func NewGrid(low, high float64, size int) (Grid, error) {
	if size < 2 || !(low < high) {
		return Grid{}, fmt.Errorf("a grid needs two values or more, low to high")
	}
	return Grid{low, high, size}, nil
}

// Step is the distance between neighbouring values.
func (g Grid) Step() float64 { return (g.High - g.Low) / float64(g.Size-1) }

// Values are the grid's values: low + i*step, and high exactly at the end.
func (g Grid) Values() []float64 {
	out := make([]float64, g.Size)
	step := g.Step()
	for i := 0; i < g.Size-1; i++ {
		out[i] = g.Low + float64(i)*step
	}
	out[g.Size-1] = g.High
	return out
}

// GridReading is a distribution read on a grid.
type GridReading struct {
	Mean, P10, Median, P90 float64
}

// Read returns the mean, median and central 80 percent interval on the
// grid's values; level maps a quantile to the level it is read at (a
// calibration's, increasing), nil for none.
func (g Grid) Read(probs []float64, level func(float64) float64) GridReading {
	total := fsum(probs)
	values := g.Values()
	terms := make([]float64, len(probs))
	for i, p := range probs {
		terms[i] = values[i] * (p / total)
	}
	out := GridReading{Mean: fsum(terms)}
	type q struct {
		at   float64
		into *float64
		set  bool
	}
	at := func(x float64) float64 {
		if level != nil {
			return level(x)
		}
		return x
	}
	qs := []*q{{at: at(0.1), into: &out.P10}, {at: at(0.5), into: &out.Median}, {at: at(0.9), into: &out.P90}}
	mass := 0.0
	for i, p := range probs {
		mass += p / total
		for _, k := range qs {
			if !k.set && mass >= k.at {
				*k.into, k.set = values[i], true
			}
		}
	}
	for _, k := range qs { // a level near 1 the summed mass falls short of by rounding: the last value
		if !k.set {
			*k.into = values[len(values)-1]
		}
	}
	return out
}

// fsum is an exactly rounded sum, as Python's math.fsum (Shewchuk's
// partials).
func fsum(xs []float64) float64 {
	var partials []float64
	for _, x := range xs {
		i := 0
		for _, y := range partials {
			if math.Abs(x) < math.Abs(y) {
				x, y = y, x
			}
			hi := x + y
			lo := y - (hi - x)
			if lo != 0 {
				partials[i] = lo
				i++
			}
			x = hi
		}
		partials = append(partials[:i], x)
	}
	sum := 0.0
	for _, p := range partials {
		sum += p
	}
	return sum
}
