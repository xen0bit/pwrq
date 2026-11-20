package prompt

import (
	"fmt"
	"math"
)

// RowOutput is what the decoder returned for one row: logits at each of the
// row's slots, and, when the row has reads, the final-norm hidden state of
// every position.
type RowOutput struct {
	Logits [][]float32 // [slot][vocabulary]
	Hidden [][]float32 // [position][dimension], nil without reads
}

// NeedsHidden reports whether a row's reads need the hidden states.
func (r Row) NeedsHidden() bool { return r.Positions != nil }

// Answers turns a request's rows and their outputs into named answers, as
// d1's system_one returns them.
func (b *Builder) Answers(state any, questions []Named, rows []Row, outs []RowOutput, cal *Calibration, opts AnswerOptions) (map[string]any, error) {
	if len(rows) != len(outs) {
		return nil, fmt.Errorf("%d rows and %d outputs", len(rows), len(outs))
	}
	switch opts.ScoreReadout {
	case "", ScoreMedian, ScoreMean, ScoreArgmax:
	default:
		return nil, fmt.Errorf("score_readout %q: one of mean, median, argmax", opts.ScoreReadout)
	}
	if opts.AbstainBelow != nil && !(*opts.AbstainBelow > 0 && *opts.AbstainBelow <= 1) {
		return nil, fmt.Errorf("abstain_below %v: a share in (0, 1]", *opts.AbstainBelow)
	}
	probs := make([][][]float64, len(questions))
	reads := make([]*readResult, len(questions))
	for r, row := range rows {
		out := outs[r]
		if len(out.Logits) != len(row.Slots) {
			return nil, fmt.Errorf("row %d: %d slots and %d logit rows", r, len(row.Slots), len(out.Logits))
		}
		for j := 0; j < len(row.Slots); j++ {
			s := row.Slots[j]
			q := questions[s.Question].Question
			if s.Slot.kind == slotPosition {
				// A read's start then its end.
				if j+1 >= len(row.Slots) || row.Slots[j+1].Question != s.Question {
					return nil, fmt.Errorf("question %q: a read's end slot is missing", questions[s.Question].Name)
				}
				rr, err := pointerRead(row, out, s.Pos, row.Slots[j+1].Pos, SpanCaps[q.Type])
				if err != nil {
					return nil, fmt.Errorf("question %q: %w", questions[s.Question].Name, err)
				}
				reads[s.Question] = rr
				j++
				continue
			}
			p, err := b.readout(s.Slot, out.Logits[j], cal, !q.Typed())
			if err != nil {
				return nil, fmt.Errorf("question %q: %w", questions[s.Question].Name, err)
			}
			probs[s.Question] = append(probs[s.Question], p)
		}
	}
	context := ""
	answers := map[string]any{}
	for i, nq := range questions {
		if nq.Question.IsRead() && context == "" && state != nil {
			st, err := StateText(state)
			if err != nil {
				return nil, err
			}
			context = st
		}
		a, err := b.answer(nq.Question, probs[i], reads[i], context, cal, opts)
		if err != nil {
			return nil, fmt.Errorf("question %q: %w", nq.Name, err)
		}
		answers[nq.Name] = a
	}
	return answers, nil
}

// pointerRead scores the row's state positions against a read's start and
// end slots - the dot product of hidden states over the square root of
// their width - and decodes the likeliest span.
func pointerRead(row Row, out RowOutput, startPos, endPos, maxTokens int) (*readResult, error) {
	if row.Positions == nil || out.Hidden == nil {
		return nil, fmt.Errorf("a read needs the row's hidden states")
	}
	mask := row.Positions.PointerMask()
	if len(mask) > len(out.Hidden) {
		return nil, fmt.Errorf("the pointer mask is longer than the row")
	}
	scores := func(at int) []float64 {
		q := out.Hidden[at]
		scale := 1 / math.Sqrt(float64(len(q)))
		s := make([]float64, len(mask))
		for t, ok := range mask {
			if !ok {
				s[t] = math.Inf(-1)
				continue
			}
			k := out.Hidden[t]
			dot := 0.0
			for d := range q {
				dot += float64(q[d]) * float64(k[d])
			}
			s[t] = dot * scale
		}
		return s
	}
	first, last, found := pointerSpan(scores(startPos), scores(endPos), maxTokens)
	span, err := row.Positions.Read(first, last, maxTokens)
	if err != nil {
		return nil, err
	}
	return &readResult{Span: span, Found: found}, nil
}

// Tokens is how many tokens a request's rows read, its usage.
func Tokens(rows []Row) int {
	n := 0
	for _, r := range rows {
		n += len(r.IDs)
	}
	return n
}

// NeedsOneImage reports whether the questions read boxes, which read
// exactly one picture.
func NeedsOneImage(questions []Named) bool {
	for _, q := range questions {
		if q.Question.Type == Box || q.Question.Type == Boxes {
			return true
		}
	}
	return false
}
