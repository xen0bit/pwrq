package prompt

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// SpanCaps are a read's longest span, in tokens.
var SpanCaps = map[string]int{Text: 512, Number: 32, Date: 32}

// StatePositions maps a row's tokens to the caller's rendered state, for a
// read's pointer: which tokens may be pointed at, and what text they cover.
type StatePositions struct {
	Text    []rune
	Offsets []*[2]int // per token: its span in Text, nil for no text position
	Paths   []stringPath
}

type stringPath struct {
	Start, End int
	Path       string
}

// Valid reports which tokens a pointer may land on: position 0 (the
// template's first token, "nothing") and every state text token.
func (sp *StatePositions) Valid() []bool {
	out := make([]bool, len(sp.Offsets))
	for i, o := range sp.Offsets {
		out[i] = i == 0 || o != nil
	}
	return out
}

// PointerMask is Valid up to its last valid position: the keys a read's
// pointer scores.
func (sp *StatePositions) PointerMask() []bool {
	v := sp.Valid()
	last := 0
	for i, ok := range v {
		if ok {
			last = i
		}
	}
	return v[:last+1]
}

// Span is what a read points at.
type Span struct {
	Text  *string
	Range *[2]int
	Path  string
}

// Read turns a pointer's endpoints into the text it covers: boundary space
// trimmed, and clipped to the one JSON string it starts or ends in, whose
// characters are returned unescaped.
func (sp *StatePositions) Read(first, last, maxTokens int) (Span, error) {
	if first == 0 && last == 0 {
		return Span{}, nil
	}
	if !(0 < first && first <= last && last < len(sp.Offsets)) || last-first >= maxTokens {
		return Span{}, fmt.Errorf("invalid pointer endpoints")
	}
	a, b := sp.Offsets[first], sp.Offsets[last]
	if a == nil || b == nil {
		return Span{}, fmt.Errorf("pointer leaves the state text")
	}
	span := [2]int{a[0], b[1]}
	for span[0] < span[1] && unicode.IsSpace(sp.Text[span[0]]) {
		span[0]++
	}
	for span[1] > span[0] && unicode.IsSpace(sp.Text[span[1]-1]) {
		span[1]--
	}
	var held []stringPath
	for _, p := range sp.Paths {
		if p.Start < span[1] && span[0] < p.End {
			held = append(held, p)
		}
	}
	if len(held) == 1 {
		p := held[0]
		span = [2]int{max(span[0], p.Start), min(span[1], p.End)}
		text := jsonText(string(sp.Text[span[0]:span[1]]))
		return Span{Text: &text, Range: &span, Path: p.Path}, nil
	}
	text := string(sp.Text[span[0]:span[1]])
	return Span{Text: &text, Range: &span}, nil
}

// jsonText is a slice of a rendered JSON string as the characters it
// encodes, unchanged when it cuts an escape.
func jsonText(escaped string) string {
	var out string
	if err := json.Unmarshal([]byte(`"`+escaped+`"`), &out); err != nil {
		return escaped
	}
	return out
}

// statePositions aligns one rendered prompt's tokens to its state text;
// template, picture and question tokens stay invalid.
func (b *Builder) statePositions(state any, prefix, full string, offs []Offset) (*StatePositions, error) {
	if !strings.HasPrefix(full, prefix) {
		return nil, fmt.Errorf("the tokenized prompt must start with the state prefix")
	}
	fullR := []rune(full)
	prefixLen := len([]rune(prefix))
	var text []rune
	var boundaries []int
	start := prefixLen
	if state != nil {
		body, err := StateBlock(state)
		if err != nil {
			return nil, err
		}
		escaped, bounds := b.plain(body, true)
		tail := escaped + "\n" + questionHeader(state)
		if !strings.HasSuffix(prefix, tail) {
			return nil, fmt.Errorf("state rendering differs from the prompt prefix")
		}
		start = prefixLen - len([]rune(tail))
		boundaries = bounds
		st, _ := StateText(state)
		text = []rune(st)
	} else {
		boundaries = []int{0}
	}
	// stop = start + bisect_right(boundaries, len(text)) - 1
	stop := start + sort.Search(len(boundaries), func(i int) bool { return boundaries[i] > len(text) }) - 1
	offsets := make([]*[2]int, len(offs))
	for i, o := range offs {
		lo, hi := max(o.Start, start), min(o.End, stop)
		if lo < hi && isBlank(string(fullR[o.Start:lo])) && isBlank(string(fullR[hi:o.End])) {
			left, right := boundaries[lo-start], boundaries[hi-start]
			if left < right && right <= len(text) && !isBlank(string(text[left:right])) {
				offsets[i] = &[2]int{left, right}
			}
		}
	}
	if len(offsets) == 0 || offsets[0] != nil {
		return nil, fmt.Errorf("position 0 must be a template token for the nothing readout")
	}
	sp := &StatePositions{Text: text, Offsets: offsets}
	if state != nil {
		if _, isText := state.(string); !isText {
			paths, err := stringPaths(text)
			if err != nil {
				return nil, err
			}
			sp.Paths = paths
		}
	}
	return sp, nil
}

// stringPaths finds the string values' spans in rendered JSON, quotes
// excluded, with RFC 6901 paths.
func stringPaths(text []rune) ([]stringPath, error) {
	var out []stringPath
	skip := func(at int) int {
		for at < len(text) && unicode.IsSpace(text[at]) {
			at++
		}
		return at
	}
	// str reads a string starting at its opening quote: its decoded value and
	// the index after its closing quote.
	str := func(at int) (string, int, error) {
		if at >= len(text) || text[at] != '"' {
			return "", 0, fmt.Errorf("invalid JSON string")
		}
		i := at + 1
		for i < len(text) && text[i] != '"' {
			if text[i] == '\\' {
				i++
			}
			i++
		}
		if i >= len(text) {
			return "", 0, fmt.Errorf("unterminated JSON string")
		}
		var s string
		if err := json.Unmarshal([]byte(string(text[at:i+1])), &s); err != nil {
			return "", 0, err
		}
		return s, i + 1, nil
	}
	var value func(at int, path string) (int, error)
	value = func(at int, path string) (int, error) {
		at = skip(at)
		if at >= len(text) {
			return 0, fmt.Errorf("truncated JSON")
		}
		switch text[at] {
		case '{':
			at = skip(at + 1)
			for at < len(text) && text[at] != '}' {
				key, end, err := str(at)
				if err != nil {
					return 0, err
				}
				end = skip(end)
				if end >= len(text) || text[end] != ':' {
					return 0, fmt.Errorf("invalid JSON object")
				}
				name := strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
				next, err := value(end+1, path+"/"+name)
				if err != nil {
					return 0, err
				}
				at = skip(next)
				if at < len(text) && text[at] == ',' {
					at = skip(at + 1)
				} else if at >= len(text) || text[at] != '}' {
					return 0, fmt.Errorf("invalid JSON object separator")
				}
			}
			return at + 1, nil
		case '[':
			at = skip(at + 1)
			index := 0
			for at < len(text) && text[at] != ']' {
				next, err := value(at, path+"/"+strconv.Itoa(index))
				if err != nil {
					return 0, err
				}
				index++
				at = skip(next)
				if at < len(text) && text[at] == ',' {
					at = skip(at + 1)
				} else if at >= len(text) || text[at] != ']' {
					return 0, fmt.Errorf("invalid JSON array separator")
				}
			}
			return at + 1, nil
		case '"':
			_, end, err := str(at)
			if err != nil {
				return 0, err
			}
			out = append(out, stringPath{at + 1, end - 1, path})
			return end, nil
		}
		// a number or a literal: up to the next separator
		end := at
		for end < len(text) && !strings.ContainsRune(",]} \t\r\n", text[end]) {
			end++
		}
		return end, nil
	}
	end, err := value(0, "")
	if err != nil {
		return nil, err
	}
	if skip(end) != len(text) {
		return nil, fmt.Errorf("trailing JSON text")
	}
	return out, nil
}

// pointerSpan finds a read's span from its start and end scores over the
// row's positions (-Inf where masked): found when P(found) >= 0.5, then the
// likeliest span within maxTokens; (0, 0) for nothing.
func pointerSpan(start, end []float64, maxTokens int) (first, last int, found float64) {
	ls, le := logSoftmax(start), logSoftmax(end)
	found = -math.Expm1(ls[0])
	t := len(start)
	if t == 1 {
		return 0, 0, found
	}
	capN := min(maxTokens, t-1)
	best, bestScore := -1, math.Inf(-1)
	bestLast := 0
	for i := 1; i < t; i++ {
		// the window of ends i .. i+cap-1 (padded past T with -Inf)
		w, wAt := math.Inf(-1), -1
		for j := i; j < min(t, i+capN); j++ {
			if wAt < 0 || le[j] > w {
				w, wAt = le[j], j
			}
		}
		score := ls[i] + w
		if best < 0 || score > bestScore {
			best, bestScore, bestLast = i, score, wAt
		}
	}
	if found < 0.5 || best < 0 || bestLast < 0 {
		return 0, 0, found
	}
	return best, bestLast, found
}

func logSoftmax(xs []float64) []float64 {
	m := math.Inf(-1)
	for _, x := range xs {
		m = math.Max(m, x)
	}
	sum := 0.0
	for _, x := range xs {
		sum += math.Exp(x - m)
	}
	lse := m + math.Log(sum)
	out := make([]float64, len(xs))
	for i, x := range xs {
		out[i] = x - lse
	}
	return out
}
