package ideengine

import (
	"context"
	"encoding/json"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/shape"
	"github.com/xen0bit/pwrq/pkg/udf/discovery"
)

// Suggestions: what to write next at the cursor.
//
// The engine does the part that needs no model. It reads the query up to
// the cursor - where the cursor is (starting a stage, after a finished one,
// naming a field, inside a call's arguments), what is half typed, and what
// the author's comments say they want - and it gathers candidates from the
// catalog, prefiltered by how well each one's help matches that intent. A
// Ranker, when the host has one, weighs the candidates with a decision model
// that reads the same help; without one the lexical order stands. A host
// with no model (the WASM page) leaves Config.Ranker nil and the operation
// answers Unavailable.

// Ranker weighs candidates. pkg/d1/suggest implements it with d1-2B.
type Ranker interface {
	// Rank returns one probability a candidate, in the candidates' order,
	// and the probability the query needs more at the cursor at all.
	Rank(ctx context.Context, sc SuggestContext) (Ranking, error)
	// Status says what the model is doing, for a spinner; Warm starts
	// loading it without waiting.
	Status() SuggestStatus
	Warm()
}

// Speller is a Ranker that can correct a comment's misspellings: phrase
// slots, which are asked about again at run time, go through it.
type Speller interface {
	Correct(text string) string
}

// Ranking is a Ranker's answer.
type Ranking struct {
	Probabilities []float64
	// Continue is P(the query needs another step here); 1 when the ranker
	// does not ask.
	Continue float64
	// Threshold is the calibrated bar a suggestion clears to be shown as
	// ghost text without being asked for.
	Threshold float64
	// RecipeThreshold, when set, is the bar instead for the rest of a
	// recipe, offered after part of it is written.
	RecipeThreshold float64
	// Members holds, for a candidate with members (a cmdlet's examples),
	// one probability a member - asked for the top candidate only.
	Members map[int][]float64
	// Slots answers SuggestContext.SlotQuestions: the words of the comment
	// each question found, by question. A question the comment does not
	// answer is absent, and its slots keep their defaults.
	Slots map[string]string
}

// SuggestRequest asks what comes next at Cursor (in runes; < 0 or past the
// end means the end). Input is a sample of the query's input, if the editor
// has one. Explicit is a keystroke asking for suggestions, which shows them
// whatever their confidence.
type SuggestRequest struct {
	Query    string `json:"query"`
	Cursor   int    `json:"cursor"`
	Input    string `json:"input,omitempty"`
	Explicit bool   `json:"explicit,omitempty"`
}

// Suggestion is one thing to insert.
type Suggestion struct {
	Insert  string `json:"insert"` // the text that replaces the Replace runes before the cursor
	Replace int    `json:"replace"`
	// ReplaceAfter runes after the cursor go too: the bracket an editor
	// closed by itself, which the insert closes where it should.
	ReplaceAfter int     `json:"replaceAfter,omitempty"`
	Display      string  `json:"display"`
	Kind         string  `json:"kind"` // cmdlet, builtin, field, argument, pipe
	Probability  float64 `json:"probability"`
	Help         string  `json:"help,omitempty"`
}

// SuggestResponse is what to offer. Ghost, when set, is the rest of
// Items[0] after what is already typed, confident enough to show inline
// after the cursor; accepting it inserts exactly that.
type SuggestResponse struct {
	Items []Suggestion `json:"items"`
	Ghost string       `json:"ghost,omitempty"`
	// GhostAfter is the top item's ReplaceAfter: how many runes after the
	// cursor the ghost stands in for, and accepting it removes.
	GhostAfter  int           `json:"ghostAfter,omitempty"`
	Position    string        `json:"position"`
	Status      SuggestStatus `json:"status"`
	Unavailable string        `json:"unavailable,omitempty"`
	Error       string        `json:"error,omitempty"`
	Seconds     float64       `json:"seconds"`
	Ranked      bool          `json:"ranked"`
}

// SuggestStatus is the model's state, as a spinner shows it.
type SuggestStatus struct {
	State       string  `json:"state"` // idle, fetching, loading, ready, busy, error, unavailable
	Detail      string  `json:"detail,omitempty"`
	Fraction    float64 `json:"fraction"` // a fetch's progress, -1 when unknown
	Device      string  `json:"device,omitempty"`
	Unavailable string  `json:"unavailable,omitempty"`
}

// Positions the cursor can be at.
const (
	PosStage    = "stage"    // where a pipeline stage begins
	PosAfter    = "after"    // after a finished stage: a pipe and a next stage
	PosField    = "field"    // after a dot: a field name
	PosArgument = "argument" // inside a call's parentheses
	PosScript   = "script"   // nothing written but comments: a whole query
)

// Candidate is one thing the cursor could take, before ranking.
type Candidate struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"` // cmdlet, builtin, field, argument, recipe
	Insert  string `json:"insert"`
	Help    string `json:"help"`
	Example string `json:"example,omitempty"`
	// Display is how a list shows it, when not by Name: a recipe by its
	// task.
	Display string `json:"display,omitempty"`
	// Slots are values Insert takes from the comment (recipes).
	Slots []Slot `json:"slots,omitempty"`
	// Keywords are more words the prefilter matches, not shown to a model.
	Keywords string `json:"-"`
	// Members are the queries a candidate stands for when it is a cmdlet
	// with several examples: the model picks the cmdlet among the others,
	// then one of these. Best match first; nil for a single query.
	Members []Candidate `json:"-"`
	Lexical float64
}

// SuggestContext is what the engine read from the query, and the
// candidates it gathered: what a Ranker's prompt is made of.
type SuggestContext struct {
	Before     string      // the query up to the cursor
	Intent     []string    // the comments, nearest the cursor last
	Partial    string      // the half-typed word the cursor ends
	Position   string      // PosStage, PosAfter, PosField or PosArgument
	Call       string      // the call whose arguments the cursor is in
	Input      string      // a sample of the input, trimmed
	Candidates []Candidate // prefiltered, best first
	// Lead goes before whichever candidate is inserted: a line break when
	// the cursor ends a comment line, a space after a finished term.
	Lead string
	// Closers are what follows the cursor on its line when it is nothing
	// but closing brackets - typed by the editor, not the author.
	Closers string
	// SlotQuestions are the slot questions of the likeliest candidates,
	// once each, for a Ranker to ask about the comments.
	SlotQuestions []string
}

// Comment is the comments as one text: what slot questions read.
func (sc SuggestContext) Comment() string { return strings.Join(sc.Intent, "\n") }

const (
	maxCandidates = 16
	// maxScriptCandidates are the tasks a comment is matched against: more
	// than a step's names, since there are some eight hundred of them and
	// the prefilter's misses are the commonest miss.
	maxScriptCandidates = 24
	maxInputChars       = 1200
	maxItems            = 5
	// maxSlotQuestions fill a row with the choice: d1 reads 16 questions
	// together.
	maxSlotQuestions = 15
)

var unavailableText = "suggestions need a model; they run in the native IDE (pwrq --ide-native) and the TUI (pwrq --tui), not in the browser page"

// suggestState serialises suggestions: a newer request cancels the one in
// flight, whose answer nobody will read.
type suggestState struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	seq    uint64
}

// Suggest answers a suggestion request.
func (e *Engine) Suggest(parent context.Context, req SuggestRequest) SuggestResponse {
	start := time.Now()
	r := e.config.Ranker
	if r == nil {
		return SuggestResponse{Items: []Suggestion{}, Unavailable: unavailableText,
			Status: SuggestStatus{State: "unavailable", Fraction: -1, Unavailable: unavailableText}}
	}
	sc := e.suggestContext(req)
	resp := SuggestResponse{Items: []Suggestion{}, Position: sc.Position}
	if len(sc.Candidates) == 0 {
		resp.Status = r.Status()
		return resp
	}

	ctx, cancel := context.WithCancel(parent)
	e.suggest.mu.Lock()
	if e.suggest.cancel != nil {
		e.suggest.cancel()
	}
	e.suggest.seq++
	seq := e.suggest.seq
	e.suggest.cancel = cancel
	e.suggest.mu.Unlock()
	defer func() {
		e.suggest.mu.Lock()
		if e.suggest.seq == seq {
			e.suggest.cancel = nil
		}
		e.suggest.mu.Unlock()
		cancel()
	}()

	ranking, err := r.Rank(ctx, sc)
	resp.Status = r.Status()
	probs := make([]float64, len(sc.Candidates))
	threshold, cont, recipeThreshold := 1.0, 1.0, 0.0
	if err == nil && len(ranking.Probabilities) == len(sc.Candidates) {
		copy(probs, ranking.Probabilities)
		threshold, cont, recipeThreshold = ranking.Threshold, ranking.Continue, ranking.RecipeThreshold
		resp.Ranked = true
	} else {
		if err != nil {
			if ctx.Err() != nil {
				resp.Error = "superseded"
			} else {
				resp.Error = err.Error()
			}
		}
		// The lexical order stands, as a distribution over the candidates.
		total := 0.0
		for _, c := range sc.Candidates {
			total += c.Lexical
		}
		for i, c := range sc.Candidates {
			if total > 0 {
				probs[i] = c.Lexical / total
			}
		}
	}
	speller, _ := r.(Speller)
	order := make([]int, len(probs))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return probs[order[a]] > probs[order[b]] })
	if resp.Ranked && len(order) > 1 && sc.Candidates[order[0]].Kind != "recipe" {
		// The model's choice is a name a recipe goes on from: the recipe
		// says the same thing next and the rest after it, so it takes the
		// name's weight and its place.
		name := sc.Candidates[order[0]].Name
		for k, i := range order[1:] {
			c := sc.Candidates[i]
			if c.Kind != "recipe" {
				continue
			}
			first := strings.TrimLeft(strings.TrimPrefix(strings.TrimSpace(c.Insert), "| "), "[({")
			if identPrefix(first) == name {
				probs[i] += probs[order[0]]
				probs[order[0]] = 0
				copy(order[1:k+2], order[0:k+1])
				order[0] = i
				break
			}
		}
	}
	for _, i := range order[:min(maxItems, len(order))] {
		c := Choose(sc, ranking, i)
		display := c.Display
		if display == "" {
			display = c.Name
		}
		insert := sc.Lead + FillCandidateWith(c, ranking.Slots, sc.Comment(), speller)
		after := 0
		if sc.Closers != "" {
			first := insert
			if i := strings.IndexByte(insert, '\n'); i >= 0 {
				first = insert[:i]
			}
			if strings.HasSuffix(first, sc.Closers) {
				after = len([]rune(sc.Closers))
			}
		}
		resp.Items = append(resp.Items, Suggestion{
			Insert: insert, Replace: len([]rune(sc.Partial)), ReplaceAfter: after, Display: display, Kind: c.Kind,
			Probability: probs[i], Help: c.Help,
		})
	}
	if len(resp.Items) > 0 && resp.Ranked {
		top := resp.Items[0]
		if top.Kind == "recipe" && sc.Position != PosScript && recipeThreshold > 0 {
			threshold = recipeThreshold
		}
		// The ghost is what is left to type: the insert past the word
		// already half written.
		if (req.Explicit || top.Probability*cont >= threshold) && strings.HasPrefix(top.Insert, sc.Partial) {
			resp.Ghost = strings.TrimPrefix(top.Insert, sc.Partial)
			resp.GhostAfter = top.ReplaceAfter
		}
	}
	resp.Seconds = time.Since(start).Seconds()
	return resp
}

// FillCandidate is a candidate's insert with its slots filled: from the
// model's answers to their questions, else from what the comment plainly
// spells (a lone number, an address, a URL), else their defaults.
func FillCandidate(c Candidate, answers map[string]string, comment string) string {
	return FillCandidateWith(c, answers, comment, nil)
}

// FillCandidateWith is FillCandidate with a speller for phrase slots.
func FillCandidateWith(c Candidate, answers map[string]string, comment string, sp Speller) string {
	if len(c.Slots) == 0 {
		return c.Insert
	}
	values := map[string]string{}
	positional := positionalNumbers(c.Slots, comment)
	for _, s := range c.Slots {
		if v, ok := positional[s.Name]; ok {
			values[s.Name] = v
			continue
		}
		if v, ok := answers[TargetedQuestion(c, s)]; ok && v != "" {
			if _, readable := normalizeSlot(s.Kind, v); readable {
				if s.Kind == "phrase" && sp != nil {
					v = sp.Correct(v)
				}
				values[s.Name] = v
				continue
			}
		}
		if v, ok := answers[s.Question]; ok {
			if _, readable := normalizeSlot(s.Kind, v); readable {
				if s.Kind == "phrase" && sp != nil {
					v = sp.Correct(v)
				}
				values[s.Name] = v
				continue
			}
		}
		if v, ok := lexicalSlot(s.Kind, comment); ok {
			values[s.Name] = v
		}
	}
	return Fill(c.Insert, c.Slots, values)
}

// TargetedQuestion is a slot's question asked again for the one query
// chosen, naming what it is for: "What text or value should it work on?"
// reads nothing out of "rot13 hello" in a row of other questions, and
// "hello" when it names rot13.
func TargetedQuestion(c Candidate, s Slot) string {
	what := c.Name
	if cmdlet, _, ok := strings.Cut(c.Name, "#"); ok {
		what = cmdlet
	}
	return s.Question + " (for " + what + ")"
}

var standaloneNumberRe = regexp.MustCompile(`(?:^|[\s(,])(-?\d+(?:\.\d+)?)(?:$|[\s,.;:)!?])`)

// positionalNumbers fills a query's numeric slots from the numbers a
// comment states, in order, when there are exactly as many: "gcd of 12 and
// 18", "5 choose 2", "clamp 99 between 0 and 10".
func positionalNumbers(slots []Slot, comment string) map[string]string {
	var numeric []Slot
	for _, s := range slots {
		if s.Kind == "value" || s.Kind == "number" || s.Kind == "days" {
			numeric = append(numeric, s)
		}
	}
	if len(numeric) < 2 {
		return nil
	}
	var found []string
	for _, m := range standaloneNumberRe.FindAllStringSubmatch(" "+comment+" ", -1) {
		found = append(found, m[1])
	}
	if len(found) != len(numeric) {
		return nil
	}
	out := map[string]string{}
	for i, s := range numeric {
		out[s.Name] = found[i]
	}
	return out
}

var (
	loneIntRe = regexp.MustCompile(`(?i)(?:^|[\s(])(\d+)(\s*(?:kb|mb|gb|tb|kib|mib|gib|k|m|g|b|bytes|days?|weeks?|months?|%|x)\b)?`)
	daysRe    = regexp.MustCompile(`(?i)\b(\d+)\s*days?\b`)
	sizeLexRe = regexp.MustCompile(`(?i)\b\d+(?:\.\d+)?\s*(?:kb|mb|gb|tb|kib|mib|gib|bytes)\b`)
	ipv4Re    = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	urlLexRe  = regexp.MustCompile(`https?://[^\s"'<>]+`)
)

// lexicalSlot reads the values a comment spells so plainly that a pattern
// is as good as a model, and better when the model's read comes back
// empty: "top 20 largest files" has one number in it.
func lexicalSlot(kind, comment string) (string, bool) {
	switch kind {
	case "value":
		if ms := standaloneNumberRe.FindAllStringSubmatch(" "+comment+" ", -1); len(ms) == 1 {
			return ms[0][1], true
		}
	case "number":
		var found []string
		for _, m := range loneIntRe.FindAllStringSubmatch(comment, -1) {
			if m[2] == "" {
				found = append(found, m[1])
			}
		}
		if len(found) == 1 {
			return found[0], true
		}
	case "days":
		if m := daysRe.FindStringSubmatch(comment); m != nil {
			return m[1], true
		}
		low := strings.ToLower(comment)
		switch {
		case strings.Contains(low, "today"):
			return "1", true
		case strings.Contains(low, "week"):
			return "7", true
		case strings.Contains(low, "month"):
			return "30", true
		}
	case "size":
		if m := sizeLexRe.FindString(comment); m != "" {
			return m, true
		}
	case "ip":
		if m := ipv4Re.FindString(comment); m != "" {
			return m, true
		}
	case "url":
		if m := urlLexRe.FindString(comment); m != "" {
			return strings.TrimRight(m, ".,;:)"), true
		}
	}
	return "", false
}

// SuggestStatus reports the ranker's state, and starts it loading so the
// first suggestion does not wait for it.
func (e *Engine) SuggestStatus() SuggestStatus {
	if e.config.Ranker == nil {
		return SuggestStatus{State: "unavailable", Fraction: -1, Unavailable: unavailableText}
	}
	e.config.Ranker.Warm()
	return e.config.Ranker.Status()
}

var identRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*$`)

var identStartRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*`)

// identPrefix is the name s starts with.
func identPrefix(s string) string { return identStartRe.FindString(s) }

// SuggestContext reads the query up to the cursor and gathers its
// candidates: what Suggest hands a Ranker, exported for calibration.
func (e *Engine) SuggestContext(req SuggestRequest) SuggestContext { return e.suggestContext(req) }

func (e *Engine) suggestContext(req SuggestRequest) SuggestContext {
	runes := []rune(req.Query)
	cursor := req.Cursor
	if cursor < 0 || cursor > len(runes) {
		cursor = len(runes)
	}
	before := string(runes[:cursor])
	after := string(runes[cursor:])
	sc := SuggestContext{Before: before, Input: trimInput(req.Input)}
	code, comments := splitComments(before)
	sc.Intent = comments
	if inString(code) {
		return sc // nothing to suggest inside a string
	}
	restOfLine := after
	if i := strings.IndexByte(after, '\n'); i >= 0 {
		restOfLine = after[:i]
	}
	if t := strings.TrimRight(restOfLine, " \t"); t != "" && strings.Trim(t, ")]}") == "" {
		sc.Closers = t
		restOfLine = ""
	}
	blank := strings.TrimSpace(code) == ""
	// The cursor in a comment line is writing prose. At the end of one,
	// what comes next is the code the comment asks for, on the next line.
	if lastLine := before[strings.LastIndexByte(before, '\n')+1:]; strings.Contains(lastLine, "#") {
		if lc, cs := splitComments(lastLine); len(cs) > 0 || strings.TrimSpace(lc) != strings.TrimSpace(lastLine) {
			if strings.TrimSpace(restOfLine) != "" {
				return sc
			}
			sc.Lead = "\n"
			if blank {
				sc.Position = PosScript
			} else {
				sc.Position = PosAfter
			}
			sc.Candidates = e.candidates(sc, code)
			sc.SlotQuestions = slotQuestions(sc.Candidates)
			return sc
		}
	}
	if blank {
		// Nothing written but comments: a whole query, if they say what for.
		if len(comments) > 0 {
			sc.Position = PosScript
			if strings.TrimSpace(before[strings.LastIndexByte(before, '\n')+1:]) != "" {
				sc.Lead = "\n"
			}
			sc.Candidates = e.candidates(sc, code)
			sc.SlotQuestions = slotQuestions(sc.Candidates)
		}
		return sc
	}
	trimmed := strings.TrimRightFunc(code, unicode.IsSpace)
	endsInSpace := len(trimmed) < len(code)
	partial := ""
	if !endsInSpace {
		partial = identRe.FindString(trimmed)
	}
	sc.Partial = partial
	head := strings.TrimRightFunc(strings.TrimSuffix(trimmed, partial), unicode.IsSpace)
	last := ""
	if head != "" {
		last = string([]rune(head)[len([]rune(head))-1])
	}
	switch {
	case strings.HasSuffix(strings.TrimSuffix(trimmed, partial), "."):
		sc.Position = PosField
	case head == "" || strings.ContainsAny(last, "|(;,[{:=") || strings.HasSuffix(head, " as") || strings.HasSuffix(head, "then") || strings.HasSuffix(head, "else"):
		sc.Position = PosStage
		if call := openCall(head); call != "" && (last == "(" || last == ";") {
			sc.Position, sc.Call = PosArgument, call
		}
	case partial == "" && endsInSpace:
		sc.Position = PosAfter
	case partial == "" && strings.ContainsAny(last, ")]}\""):
		// right after a closing bracket: the stage is finished
		sc.Position, sc.Lead = PosAfter, " "
		if strings.TrimSpace(restOfLine) == "" {
			sc.Lead = "\n"
		}
	default:
		// a word right after a finished term: still naming the stage
		sc.Position = PosStage
	}
	sc.Candidates = e.candidates(sc, code)
	sc.SlotQuestions = slotQuestions(sc.Candidates)
	return sc
}

// splitComments separates code from comments, comments in order.
func splitComments(src string) (string, []string) {
	var code strings.Builder
	var comments []string
	inStr := false
	rs := []rune(src)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case inStr && c == '\\' && i+1 < len(rs):
			code.WriteRune(c)
			i++
			code.WriteRune(rs[i])
			continue
		case c == '"':
			inStr = !inStr
		case !inStr && c == '#':
			j := i
			for j < len(rs) && rs[j] != '\n' {
				j++
			}
			if text := strings.TrimSpace(strings.TrimLeft(string(rs[i:j]), "#")); text != "" {
				comments = append(comments, text)
			}
			i = j - 1
			continue
		}
		code.WriteRune(c)
	}
	return code.String(), comments
}

// inString reports whether src ends inside a string literal.
func inString(src string) bool {
	in := false
	rs := []rune(src)
	for i := 0; i < len(rs); i++ {
		switch {
		case in && rs[i] == '\\':
			i++
		case rs[i] == '"':
			in = !in
		}
	}
	return in
}

// openCall is the name of the innermost call whose parentheses are open at
// the end of src.
func openCall(src string) string {
	depth := 0
	rs := []rune(src)
	for i := len(rs) - 1; i >= 0; i-- {
		switch rs[i] {
		case ')', ']', '}':
			depth++
		case '[', '{':
			if depth == 0 {
				return ""
			}
			depth--
		case '(':
			if depth == 0 {
				return identRe.FindString(strings.TrimRightFunc(string(rs[:i]), unicode.IsSpace))
			}
			depth--
		}
	}
	return ""
}

func trimInput(s string) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > maxInputChars {
		return string(r[:maxInputChars]) + "…"
	}
	return s
}

var wordRe = regexp.MustCompile(`[a-z0-9]+`)

// words are a text's lowercase words, the stemming crude on purpose: files
// match file, sorting matches sort.
func words(s string) []string {
	var out []string
	for _, w := range wordRe.FindAllString(strings.ToLower(strings.NewReplacer("_", " ").Replace(s)), -1) {
		if len(w) < 2 || stopWords[w] {
			continue
		}
		// sha256sum, md5sum: the coreutils names for the hashes.
		if len(w) > 5 && strings.HasSuffix(w, "sum") && !strings.HasSuffix(w, "checksum") {
			out = append(out, strings.TrimSuffix(w, "sum"))
		}
		for _, suf := range []string{"ing", "es", "s", "ed"} {
			if len(w) > len(suf)+2 && strings.HasSuffix(w, suf) {
				w = strings.TrimSuffix(w, suf)
				break
			}
		}
		out = append(out, w)
	}
	return out
}

var stopWords = map[string]bool{"the": true, "a": true, "an": true, "of": true, "to": true, "and": true, "or": true,
	"in": true, "on": true, "for": true, "with": true, "is": true, "it": true, "its": true, "by": true, "as": true,
	"be": true, "that": true, "this": true, "from": true, "each": true, "all": true, "into": true, "at": true}

// candidates gathers what the cursor could take, scored lexically against
// the intent: BM25 over each candidate's name and help. code is the query
// before the cursor without its comments.
func (e *Engine) candidates(sc SuggestContext, code string) []Candidate {
	var pool []Candidate
	query := words(strings.Join(sc.Intent, " "))
	switch sc.Position {
	case PosScript:
		pool = recipeCandidates()
		scoreBM25(pool, query)
		sort.SliceStable(pool, func(a, b int) bool { return pool[a].Lexical > pool[b].Lexical })
		groups := groupByTask(pool)
		return groups[:min(maxScriptCandidates, len(groups))]
	case PosField:
		for _, k := range inputKeys(sc.Input) {
			pool = append(pool, Candidate{Name: k, Kind: "field", Insert: fieldInsert(k), Help: "a field of the input"})
		}
		if len(pool) == 0 {
			pool = shapeFields(code)
		}
	case PosArgument:
		pool = append(pool, e.argumentCandidates(sc.Call)...)
	default:
		pool = e.vocabulary()
		if sc.Position == PosAfter {
			for i := range pool {
				pool[i].Insert = "| " + pool[i].Insert
			}
		}
		pool = append(pool, recipeContinuations(code, sc.Position, sc.Partial)...)
	}
	if sc.Partial != "" {
		var keep []Candidate
		for _, c := range pool {
			if strings.HasPrefix(c.Name, sc.Partial) && c.Name != sc.Partial ||
				c.Kind == "recipe" && strings.HasPrefix(c.Insert, sc.Partial) {
				keep = append(keep, c)
			}
		}
		pool = keep
	}
	query = append(query, words(sc.Partial)...)
	scoreBM25(pool, query)
	for i := range pool {
		if pool[i].Kind == "recipe" {
			// It continues what is written, which a single name does not.
			pool[i].Lexical += recipeBonus
		}
	}
	sort.SliceStable(pool, func(a, b int) bool { return pool[a].Lexical > pool[b].Lexical })
	if len(pool) <= maxCandidates {
		return pool
	}
	// The staples - map, select, sort_by - are what most stages are, and a
	// comment rarely names them, so the comment's matches alone would crowd
	// them out. Some slots are kept for the likeliest staples, whatever the
	// comment says; the model, reading the comment, decides between them.
	out := append([]Candidate(nil), pool[:maxCandidates-reservedStaples]...)
	have := map[string]bool{}
	for _, c := range out {
		have[c.Name] = true
	}
	rest := append([]Candidate(nil), pool[maxCandidates-reservedStaples:]...)
	sort.SliceStable(rest, func(a, b int) bool { return prior[rest[a].Name] > prior[rest[b].Name] })
	for _, c := range rest {
		if len(out) == maxCandidates {
			break
		}
		if !have[c.Name] {
			out = append(out, c)
			have[c.Name] = true
		}
	}
	return out
}

// recipeBonus lifts a recipe that continues the query over single names in
// the prefilter.
const recipeBonus = 1.5

// recipeCandidates are the recipes as whole-query candidates.
func recipeCandidates() []Candidate {
	rs := AllRecipes()
	out := make([]Candidate, 0, len(rs))
	for _, r := range rs {
		out = append(out, Candidate{Name: r.ID, Kind: "recipe", Insert: r.Query, Help: r.Task, Display: r.Task,
			Example: Fill(r.Query, r.Slots, nil), Slots: r.Slots, Keywords: r.Also})
	}
	return out
}

// recipeContinuations are the rests of the recipes that start the way the
// query does: same stages, by what each starts with, up to the cursor's.
func recipeContinuations(code, pos, partial string) []Candidate {
	trimmed := strings.TrimSpace(code)
	if trimmed == "" {
		return nil
	}
	typed := Stages(trimmed)
	complete, current, opened := typed, "", ""
	if pos == PosStage {
		last := strings.TrimSpace(typed[len(typed)-1])
		complete, current = typed[:len(typed)-1], last
		opened = strings.TrimSuffix(last, partial)
		if strings.Trim(opened, "[({ ") != "" {
			return nil // the stage holds more than brackets and a name: not a recipe's
		}
	}
	var out []Candidate
	seen := map[string]bool{}
	for _, r := range AllRecipes() {
		stages := Stages(r.Query)
		k := len(complete)
		if k >= len(stages) {
			continue
		}
		ok := true
		for i := 0; i < k && ok; i++ {
			ok = stageHead(complete[i]) == stageHead(stages[i])
		}
		if !ok || !strings.HasPrefix(stages[k], current) {
			continue
		}
		insert := strings.Join(stages[k:], "\n| ")
		if pos != PosStage {
			insert = "| " + insert
		} else {
			insert = insert[len(opened):] // the brackets are written already
		}
		if seen[insert] {
			continue
		}
		seen[insert] = true
		next := Fill(stages[k], r.Slots, nil)
		if len(next) > 60 {
			next = next[:57] + "..."
		}
		out = append(out, Candidate{Name: r.ID, Kind: "recipe", Insert: insert, Help: r.Task, Display: next,
			Example: Fill(r.Query, r.Slots, nil), Slots: r.Slots, Keywords: r.Also})
	}
	return out
}

// shapePreserving stages pass their input's objects through unchanged, so
// the fields after them are the fields before.
var shapePreserving = map[string]bool{"select": true, "sort_by": true, "sort_object": true, "where_object": true,
	"unique_by": true, "first": true, "last": true, "limit": true, "reverse": true, "sort": true, "unique": true}

// shapeFields are the properties of what the stage before the cursor's
// emits, as its cmdlet declared them: get_childitem's Name, Length, ... .
func shapeFields(code string) []Candidate {
	stages := Stages(strings.TrimSpace(code))
	shapes := commandShapes()
	for i := len(stages) - 1; i >= 0; i-- {
		st := stages[i]
		if i == len(stages)-1 {
			// The cursor's own stage: a call that opens it, as in
			// select(.  or  [get_childitem(".") | select(.
			if inner := Stages(strings.TrimLeft(st, "[({ ")); len(inner) > 1 {
				st = inner[len(inner)-2]
			} else {
				continue
			}
		}
		head := strings.TrimLeft(stageHead(st), "[({")
		if props, ok := shapes[head]; ok {
			out := make([]Candidate, 0, len(props))
			for _, p := range props {
				out = append(out, Candidate{Name: p.Name, Kind: "field", Insert: fieldInsert(p.Name), Help: p.Doc})
			}
			return out
		}
		if !shapePreserving[head] {
			return nil
		}
	}
	return nil
}

var (
	shapesOnce sync.Once
	shapesBy   map[string][]shape.Property
)

func commandShapes() map[string][]shape.Property {
	shapesOnce.Do(func() {
		shapesBy = map[string][]shape.Property{}
		for _, c := range discovery.Catalog() {
			if c.Shape != nil && len(c.Shape.Properties()) > 0 {
				shapesBy[c.Name] = c.Shape.Properties()
			}
		}
	})
	return shapesBy
}

// groupByTask makes one candidate of each cmdlet's examples, in the order of
// its best: they share a description, so as separate options the model
// could not tell them apart, and they would crowd out the other tasks. The
// group's members are its examples, best first; the model chooses among
// them once the cmdlet is chosen.
func groupByTask(pool []Candidate) []Candidate {
	var out []Candidate
	at := map[string]int{}
	for _, c := range pool {
		cmdlet, _, isExample := strings.Cut(c.Name, "#")
		if !isExample {
			out = append(out, c)
			continue
		}
		if k, ok := at[cmdlet]; ok {
			out[k].Members = append(out[k].Members, c)
			continue
		}
		at[cmdlet] = len(out)
		group := c
		group.Name = cmdlet
		group.Members = []Candidate{c}
		out = append(out, group)
	}
	return out
}

// Choose is the query a ranking settles on for candidate i: the candidate,
// or the member of it the ranking picked.
func Choose(sc SuggestContext, ranking Ranking, i int) Candidate {
	c := sc.Candidates[i]
	if len(c.Members) == 0 {
		return c
	}
	if len(c.Members) == 1 {
		return c.Members[0]
	}
	pick := 0
	if p, ok := ranking.Members[i]; ok && len(p) == len(c.Members) {
		for k := range p {
			if p[k] > p[pick] {
				pick = k
			}
		}
	}
	return c.Members[pick]
}

// slotQuestions are the slot questions of the candidates, likeliest first,
// once each and no more than fit beside the choice.
func slotQuestions(cs []Candidate) []string {
	var out []string
	seen := map[string]bool{}
	for _, c := range cs {
		for _, m := range append([]Candidate{c}, c.Members...) {
			for _, s := range m.Slots {
				if seen[s.Question] || len(out) == maxSlotQuestions {
					continue
				}
				seen[s.Question] = true
				out = append(out, s.Question)
			}
		}
	}
	return out
}

// reservedStaples is how many of the candidates are the likeliest staples.
const reservedStaples = 6

func fieldInsert(k string) string {
	if regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(k) {
		return k
	}
	b, _ := json.Marshal(k)
	return string(b)
}

// scoreBM25 scores each candidate's name and help against the query words,
// plus a prior: commonly needed names first when the query says nothing.
func scoreBM25(pool []Candidate, query []string) {
	const k1, b = 1.2, 0.75
	docs := make([][]string, len(pool))
	df := map[string]int{}
	total := 0
	for i, c := range pool {
		text := c.Name + " " + c.Name + " " + c.Help + " " + c.Example
		if c.Kind == "recipe" {
			// A recipe's query is mostly get_childitem and select: its task
			// and keywords say what it is for.
			text = strings.ReplaceAll(c.Name, "-", " ") + " " + c.Help + " " + c.Keywords
		}
		docs[i] = words(text)
		total += len(docs[i])
		seen := map[string]bool{}
		for _, w := range docs[i] {
			if !seen[w] {
				df[w]++
				seen[w] = true
			}
		}
	}
	avg := 1.0
	if len(pool) > 0 && total > 0 {
		avg = float64(total) / float64(len(pool))
	}
	n := float64(len(pool))
	for i := range pool {
		tf := map[string]int{}
		for _, w := range docs[i] {
			tf[w]++
		}
		score := 0.0
		for _, q := range query {
			f := float64(tf[q])
			if f == 0 {
				continue
			}
			idf := math.Log(1 + (n-float64(df[q])+0.5)/(float64(df[q])+0.5))
			score += idf * f * (k1 + 1) / (f + k1*(1-b+b*float64(len(docs[i]))/avg))
		}
		pool[i].Lexical = score + prior[pool[i].Name] + 0.01
	}
}

// prior is a small preference for what queries most often need next, which
// decides the order when there is no intent to match.
var prior = map[string]float64{
	"map": 0.9, "select": 0.6, "sort_by": 0.55, "to_entries": 0.5, "length": 0.45, "group_by": 0.45,
	"keys": 0.4, "map_values": 0.4, "split": 0.4, "join": 0.35, "add": 0.35, "unique": 0.3, "first": 0.3,
	"reverse": 0.3, "from_entries": 0.3, "with_entries": 0.3, "tostring": 0.25, "flatten": 0.25,
	"where_object": 0.4, "select_object": 0.35, "sort_object": 0.35, "group_by_key": 0.3, "value_counts": 0.3,
	"count_by": 0.3, "summarize_by": 0.25, "format_table": 0.3, "get_childitem": 0.3, "top_by": 0.25,
}

// builtinHelp describes jq builtins, which the catalog lists by name only,
// so a model ranking them has something to read.
var builtinHelp = map[string]string{
	"select": "Keep the input only when the condition is true", "map": "Apply a filter to every element of an array",
	"length": "Count elements, keys or characters", "keys": "List an object's keys, sorted",
	"sort": "Sort an array", "sort_by": "Sort an array by a computed key", "group_by": "Group array elements sharing a key",
	"unique": "Remove duplicate elements", "unique_by": "Keep one element per computed key",
	"min_by": "The element with the smallest key", "max_by": "The element with the largest key",
	"add": "Sum numbers, concatenate strings or arrays, merge objects", "first": "The first output or element",
	"last": "The last output or element", "to_entries": "Turn an object into key/value pairs",
	"from_entries": "Build an object from key/value pairs", "with_entries": "Rewrite an object's key/value pairs",
	"tostring": "Convert to a string", "tonumber": "Parse a number", "ascii_downcase": "Lower-case text",
	"ascii_upcase": "Upper-case text", "split": "Split text on a separator", "join": "Join strings with a separator",
	"test": "Whether text matches a regular expression", "capture": "Named captures of a regex match",
	"contains": "Whether the input contains a value", "has": "Whether an object has a key",
	"del": "Delete a path", "to_entries[]": "Each key/value pair", "flatten": "Flatten nested arrays",
	"range": "A range of numbers", "reverse": "Reverse an array or string", "limit": "The first n outputs",
	"empty": "Produce nothing", "error": "Raise an error", "type": "The JSON type of the input",
	"paths": "Every path in the input", "leaf_paths": "Paths to every scalar", "any": "Whether any element is true",
	"all": "Whether every element is true", "env": "Environment variables", "input": "The next input",
	"inputs": "All remaining inputs", "debug": "Print the input to stderr and pass it on",
	"splits": "Split on a regex", "sub": "Replace the first regex match", "gsub": "Replace every regex match",
	"ltrimstr": "Remove a prefix", "rtrimstr": "Remove a suffix", "startswith": "Whether text starts with a prefix",
	"endswith": "Whether text ends with a suffix", "tojson": "Encode as JSON text", "fromjson": "Decode JSON text",
	"todate": "Format seconds as a date", "fromdate": "Parse a date to seconds", "now": "The current time in seconds",
	"floor": "Round down", "ceil": "Round up", "round": "Round to the nearest integer", "values": "Drop nulls",
	"recurse": "Every value, recursively", "walk": "Rewrite every value bottom-up", "indices": "Positions of a value",
	"index": "Position of the first occurrence", "transpose": "Transpose an array of arrays",
	"input_filename": "The current input's file name", "getpath": "The value at a path", "setpath": "Set a path",
	"min": "The smallest element", "max": "The largest element", "not": "Logical negation",
	"objects": "Keep only objects", "arrays": "Keep only arrays", "strings": "Keep only strings",
	"numbers": "Keep only numbers", "nulls": "Keep only nulls", "tostream": "Stream paths and leaves",
}

// vocabulary is every name a stage can start with, with its help.
func (e *Engine) vocabulary() []Candidate {
	e.vocabOnce.Do(func() {
		cat := e.Catalog()
		seen := map[string]bool{}
		for _, c := range cat.Commands {
			if !c.Available || seen[c.Name] {
				continue
			}
			seen[c.Name] = true
			insert := c.Name
			if c.MinArgs > 0 {
				insert += "("
			}
			ex := ""
			if len(c.Examples) > 0 {
				ex = c.Examples[0]
			}
			e.vocab = append(e.vocab, Candidate{Name: c.Name, Kind: "cmdlet", Insert: insert, Help: c.Description, Example: ex})
		}
		minArity := builtinArities()
		for _, b := range cat.Builtins {
			name, _, _ := strings.Cut(b, "/")
			if seen[name] || strings.HasPrefix(name, "_") {
				continue
			}
			seen[name] = true
			help, ok := builtinHelp[name]
			if !ok {
				help = "jq builtin " + name
			}
			insert := name
			if minArity[name] > 0 {
				insert += "("
			}
			e.vocab = append(e.vocab, Candidate{Name: name, Kind: "builtin", Insert: insert, Help: help})
		}
	})
	out := make([]Candidate, len(e.vocab))
	copy(out, e.vocab)
	return out
}

// argumentCandidates are a cmdlet's example arguments: what its help shows
// it being called with.
func (e *Engine) argumentCandidates(call string) []Candidate {
	if call == "" {
		return nil
	}
	var out []Candidate
	seen := map[string]bool{}
	for _, c := range e.Catalog().Commands {
		if c.Name != call {
			continue
		}
		for _, ex := range c.Examples {
			i := strings.Index(ex, call+"(")
			if i < 0 {
				continue
			}
			rest := ex[i+len(call):]
			// the balanced argument list
			depth, end := 0, -1
			for k, r := range rest {
				if r == '(' {
					depth++
				} else if r == ')' {
					depth--
					if depth == 0 {
						end = k
						break
					}
				}
			}
			if end <= 1 {
				continue
			}
			args := rest[1:end]
			if seen[args] {
				continue
			}
			seen[args] = true
			out = append(out, Candidate{Name: args, Kind: "argument", Insert: args + ")", Help: c.Description, Example: ex})
		}
	}
	return out
}

// inputKeys are the field names a sample of the input offers: an object's
// keys, or the keys of an array's objects.
func inputKeys(input string) []string {
	if input == "" {
		return nil
	}
	dec := json.NewDecoder(strings.NewReader(input))
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil
	}
	set := map[string]bool{}
	var walk func(v any, depth int)
	walk = func(v any, depth int) {
		switch x := v.(type) {
		case map[string]any:
			for k, val := range x {
				set[k] = true
				if depth < 1 {
					walk(val, depth+1)
				}
			}
		case []any:
			for i, el := range x {
				if i >= 20 {
					break
				}
				walk(el, depth)
			}
		}
	}
	walk(v, 0)
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// builtinArities is each jq builtin's smallest arity, from gojq's own list
// (the catalog keeps names only): a name that is never called bare is
// suggested with its opening parenthesis.
func builtinArities() map[string]int {
	out := map[string]int{}
	q, err := gojq.Parse("builtins")
	if err != nil {
		return out
	}
	code, err := gojq.Compile(q)
	if err != nil {
		return out
	}
	v, _ := code.Run(nil).Next()
	list, _ := v.([]any)
	for _, e := range list {
		s, _ := e.(string)
		name, arity, ok := strings.Cut(s, "/")
		if !ok {
			continue
		}
		n := 0
		for _, r := range arity {
			n = n*10 + int(r-'0')
		}
		if old, seen := out[name]; !seen || n < old {
			out[name] = n
		}
	}
	return out
}
