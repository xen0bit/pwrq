package prompt

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/xen0bit/pwrq/pkg/d1/tokenizer"
)

// Config is what a checkpoint records about how it reads (config.json's d1_
// keys) and how its prompt is written. Default is d1-2B's.
type Config struct {
	Codes      int    // option code scheme: 1 (00..) or 2 (A..Z, AA..) past 26 options
	Numbers    bool   // grids read as the tokenizer's own number tokens
	BoxEdges   string // "median" or "interp"
	Version    int    // prompt version
	SetSize    int    // questions packed after one state
	Lead       string // what the template writes after the assistant header
	BOS        string
	System     string // a system turn's text, "" for none
	VisionMax  int    // pixels a picture is capped at
	ImageToken string
	VideoToken string
}

// Default is d1-2B's configuration.
var Default = Config{
	Codes:      2,
	Numbers:    true,
	BoxEdges:   "median",
	Version:    1,
	SetSize:    16,
	Lead:       "<think></think>",
	BOS:        "<|startoftext|>",
	VisionMax:  1638400,
	ImageToken: "<|image|>",
	VideoToken: "<|video|>",
}

const (
	imStart = "<|im_start|>"
	imEnd   = "<|im_end|>"
	// header opens each question after a state; set packing repeats it.
	header = "QUESTION:\n"
	// breakChar is a zero-width space put after the first character of a
	// special token's text in a caller's string: it reads the same and is no
	// longer the token. Without it a state holding <|im_end|> closed the user
	// turn.
	breakChar = "\u200b"
)

// Builder renders prompts and readouts for one tokenizer and configuration.
// It is safe for concurrent use.
type Builder struct {
	tok      *tokenizer.Tokenizer
	cfg      Config
	specials *regexp.Regexp

	mu       sync.Mutex
	single   map[string]int32 // text -> its id, when it is one token; -1 when not
	aliases  map[string][]alias
	numberID map[[4]float64][][]int32
	numberOK map[[4]float64]bool
}

type alias struct {
	code string
	id   int32
}

// NewBuilder prepares a Builder.
func NewBuilder(tok *tokenizer.Tokenizer, cfg Config) *Builder {
	b := &Builder{tok: tok, cfg: cfg, single: map[string]int32{}, aliases: map[string][]alias{},
		numberID: map[[4]float64][][]int32{}, numberOK: map[[4]float64]bool{}}
	var names []string
	for _, s := range tok.Specials() {
		if len([]rune(s)) > 1 && !strings.Contains(s, breakChar) {
			names = append(names, s)
		}
	}
	sort.SliceStable(names, func(i, j int) bool {
		if len([]rune(names[i])) != len([]rune(names[j])) {
			return len([]rune(names[i])) > len([]rune(names[j]))
		}
		return names[i] < names[j]
	})
	if len(names) > 0 {
		quoted := make([]string, len(names))
		for i, n := range names {
			quoted[i] = regexp.QuoteMeta(n)
		}
		b.specials = regexp.MustCompile(strings.Join(quoted, "|"))
	}
	return b
}

// Tokenizer is the Builder's tokenizer.
func (b *Builder) Tokenizer() *tokenizer.Tokenizer { return b.tok }

// Config is the Builder's configuration.
func (b *Builder) Config() Config { return b.cfg }

// one returns text's id when the tokenizer writes it as exactly one token.
func (b *Builder) one(text string) (int32, bool) {
	b.mu.Lock()
	id, ok := b.single[text]
	b.mu.Unlock()
	if !ok {
		ids := b.tok.Encode(text)
		id = -1
		if len(ids) == 1 {
			id = ids[0]
		}
		b.mu.Lock()
		b.single[text] = id
		b.mu.Unlock()
	}
	return id, id >= 0
}

// ids returns each text's id that is one token, each id once.
func (b *Builder) ids(texts ...string) []int32 {
	var out []int32
	seen := map[int32]bool{}
	for _, t := range texts {
		if id, ok := b.one(t); ok && !seen[id] {
			out = append(out, id)
			seen[id] = true
		}
	}
	return out
}

var letters = func() []string {
	out := make([]string, 26)
	for i := range out {
		out[i] = string(rune('A' + i))
	}
	return out
}()

// optionCodes are native letters when the labels already are letters, else
// A..Z, else the scheme's codes past 26: 00.. (1), or A..Z then AA, AB.. (2).
func optionCodes(labels []string, scheme int) []string {
	labs := make([]string, len(labels))
	native := len(labels) > 0
	for i, l := range labels {
		labs[i] = strings.TrimSpace(l)
		r := []rune(labs[i])
		native = native && len(r) == 1 && unicode.IsLetter(r[0])
	}
	if native {
		return labs
	}
	if len(labs) <= 26 {
		return append([]string(nil), letters[:len(labs)]...)
	}
	if scheme == 2 {
		all := append([]string(nil), letters...)
		for _, a := range letters {
			for _, c := range letters {
				all = append(all, a+c)
			}
		}
		if len(labs) > len(all) {
			return nil
		}
		return all[:len(labs)]
	}
	out := make([]string, len(labs))
	for i := range out {
		out[i] = fmt.Sprintf("%02d", i)
	}
	return out
}

// fallbackPool is where a code that is not one token, or repeats one, takes
// its alias from.
var fallbackPool = func() []string {
	var out []string
	for c := 'A'; c <= 'Z'; c++ {
		out = append(out, string(c))
	}
	for i := 0; i < 100; i++ {
		out = append(out, fmt.Sprintf("%02d", i))
	}
	for c := 'a'; c <= 'z'; c++ {
		out = append(out, string(c))
	}
	for i := 0; i < 200; i++ {
		out = append(out, "#"+strconv.Itoa(i))
	}
	for a := 'A'; a <= 'Z'; a++ {
		for c := 'A'; c <= 'Z'; c++ {
			out = append(out, string(a)+string(c))
		}
	}
	return out
}()

// optionAliases gives every label a distinct single-token code.
func (b *Builder) optionAliases(labels []string) ([]alias, error) {
	codes := optionCodes(labels, b.cfg.Codes)
	if codes == nil {
		return nil, fmt.Errorf("%d options are more than the codes can name", len(labels))
	}
	key := strings.Join(codes, "\x00")
	b.mu.Lock()
	cached, ok := b.aliases[key]
	b.mu.Unlock()
	if ok {
		return cached, nil
	}
	used := map[int32]bool{}
	var out []alias
	take := func(raw string) bool {
		id, ok := b.one(raw)
		if !ok || used[id] {
			return false
		}
		out = append(out, alias{raw, id})
		used[id] = true
		return true
	}
	for _, code := range codes {
		if take(code) {
			continue
		}
		found := false
		for _, raw := range fallbackPool {
			if take(raw) {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("no single-token alias left for %d options", len(codes))
		}
	}
	b.mu.Lock()
	b.aliases[key] = out
	b.mu.Unlock()
	return out, nil
}

// slotKind is how one answer position is read.
type slotKind int

const (
	slotNoul slotKind = iota
	slotChoice
	slotScore
	slotPosition
	slotNumber
)

// Slot is one answer position's readout: a question has one, a read two, a
// box five.
type Slot struct {
	kind   slotKind
	labels []string // choice: option names
	levels int      // score
	grid   Grid     // number
	scale  int      // number
	name   string   // position
}

// cardinality is how many options the slot reads.
func (s Slot) cardinality() int {
	switch s.kind {
	case slotNoul:
		return 2
	case slotChoice:
		return len(s.labels)
	case slotScore:
		return s.levels
	case slotNumber:
		return s.grid.Size
	}
	return 0
}

var edges = []string{"top", "bottom", "left", "right"}

var (
	codeGrid    = Grid{0, 1, 128}      // an edge's values as codes, 128 of them
	thousandths = Grid{0, 0.999, 1000} // an edge's values as numbers: 0-999 ("1000" takes two tokens)
)

// edgeGrid is a box edge's grid and the factor its values are written at.
func (b *Builder) edgeGrid() (Grid, int) {
	if b.cfg.Numbers && b.numberIDs(thousandths, 1000) != nil {
		return thousandths, 1000
	}
	return codeGrid, 1
}

// numberIDs is each grid value times scale as the tokenizer writes it, one
// token; nil when a value is no integer or takes more than one token.
func (b *Builder) numberIDs(g Grid, scale int) [][]int32 {
	key := [4]float64{g.Low, g.High, float64(g.Size), float64(scale)}
	b.mu.Lock()
	groups, done := b.numberID[key], b.numberOK[key]
	b.mu.Unlock()
	if done {
		return groups
	}
	groups = nil
	for _, v := range g.Values() {
		n := pyRound(v * float64(scale))
		if math.Abs(n-v*float64(scale)) > 1e-6 {
			groups = nil
			break
		}
		s := strconv.FormatInt(int64(n), 10)
		if _, ok := b.one(s); !ok {
			groups = nil
			break
		}
		groups = append(groups, b.ids(s, " "+s))
	}
	b.mu.Lock()
	b.numberID[key], b.numberOK[key] = groups, true
	b.mu.Unlock()
	return groups
}

// gridSlot is a grid's slot: the tokenizer's own numbers where the
// checkpoint reads them so and every value is one token, else the grid's
// codes.
func (b *Builder) gridSlot(g Grid, scale int) Slot {
	if b.cfg.Numbers && b.numberIDs(g, scale) != nil {
		return Slot{kind: slotNumber, grid: g, scale: scale}
	}
	return gridQuestion(g)
}

// gridQuestion is a grid's ordered bins on the digit or option-code readout.
func gridQuestion(g Grid) Slot {
	if g.Size <= 10 {
		return Slot{kind: slotScore, levels: g.Size}
	}
	labels := make([]string, g.Size)
	for i := range labels {
		labels[i] = strconv.Itoa(i)
	}
	return Slot{kind: slotChoice, labels: labels}
}

// Slots are the question's answer positions in order.
func (b *Builder) Slots(q Question) []Slot {
	switch {
	case q.IsRead():
		return []Slot{{kind: slotPosition, name: "start"}, {kind: slotPosition, name: "end"}}
	case q.Estimate != nil:
		return []Slot{b.gridSlot(*q.Estimate, 1)}
	case q.Type == Box:
		g, scale := b.edgeGrid()
		out := []Slot{{kind: slotNoul}}
		for range edges {
			out = append(out, b.gridSlot(g, scale))
		}
		return out
	case q.Type == Boxes:
		one := b.Slots(Question{Type: Box})
		var out []Slot
		for range q.blocks() {
			out = append(out, one...)
		}
		return out
	case q.Type == Choice && q.Multiple:
		out := make([]Slot, len(q.Options))
		for i := range out {
			out[i] = Slot{kind: slotNoul}
		}
		return out
	case q.Type == Noul:
		return []Slot{{kind: slotNoul}}
	case q.Type == Score:
		return []Slot{{kind: slotScore, levels: len(q.Levels)}}
	}
	labels := make([]string, len(q.Options))
	for i, o := range q.Options {
		labels[i] = o.Name
	}
	return []Slot{{kind: slotChoice, labels: labels}}
}

// readoutIDs are the token ids scored for a slot, one group per option,
// max-pooled: the answer is a softmax over these and nothing else.
func (b *Builder) readoutIDs(s Slot) ([][]int32, error) {
	switch s.kind {
	case slotNumber:
		return b.numberIDs(s.grid, s.scale), nil
	case slotPosition:
		return nil, fmt.Errorf("a pointer reads state positions, not vocabulary codes")
	case slotNoul:
		yes, no := b.ids("yes", "Yes", "YES"), b.ids("no", "No", "NO")
		if len(yes) == 0 || len(no) == 0 {
			return nil, fmt.Errorf("tokenizer has no single-token yes/no")
		}
		return [][]int32{yes, no}, nil
	case slotScore:
		groups := make([][]int32, s.levels)
		for i := range groups {
			groups[i] = b.ids(strconv.Itoa(i))
			if len(groups[i]) == 0 {
				return nil, fmt.Errorf("score with %d levels needs single-token digits; the primitive is defined for 2 to 10", s.levels)
			}
		}
		return groups, nil
	}
	as, err := b.optionAliases(s.labels)
	if err != nil {
		return nil, err
	}
	groups := make([][]int32, 0, len(as))
	for _, a := range as {
		g := []int32{a.id}
		for _, id := range b.ids(" " + a.code) {
			if id != a.id {
				g = append(g, id)
			}
		}
		groups = append(groups, g)
	}
	if len(groups) == 0 {
		return nil, fmt.Errorf("choice with no options")
	}
	return groups, nil
}
