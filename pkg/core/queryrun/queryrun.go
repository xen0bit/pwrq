// Package queryrun evaluates a pwrq query to completion and reports what it
// produced.
//
// It exists because pwrq has more than one place that is not a terminal. The
// browser IDE and the MCP server both have to take a query and an input as
// text, run them against a vocabulary, and hand back rendered values plus a
// reason the run stopped — and neither can rely on a user pressing Ctrl-C, so
// both need the same bounds. What differs between them is the vocabulary they
// compile against, the deadline they can enforce, and the limits they think
// are reasonable. All three are the caller's to supply; everything else is
// here, once.
//
// The contract is jq's. Inputs decode as a stream of values, `-n`, `-s`, `-r`
// and `-R` mean what they mean on the command line, values are rendered by
// gojq's own marshaller so numbers and escapes match what the CLI would print,
// and `halt` is a deliberate stop rather than a failure.
package queryrun

import (
	"context"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/itchyny/gojq"

	"github.com/xen0bit/pwrq/pkg/core/runctx"
	"github.com/xen0bit/pwrq/pkg/core/shape"
)

// Request is a query, its input, and how to present what comes out. The flags
// are jq's, and each is named after the option it stands for.
type Request struct {
	Query string
	Input string

	// RawInput reads Input as lines of text rather than JSON (jq -R).
	RawInput bool
	// Slurp reads the whole input as a single value: an array of the decoded
	// values, or the input verbatim under RawInput (jq -s).
	Slurp bool
	// NullInput runs the program once on null. The input is still readable
	// through `input` and `inputs` (jq -n).
	NullInput bool

	// Raw prints string results without quotes (jq -r).
	Raw bool
	// Compact prints each result on one line (jq -c).
	Compact bool
	// Indent is how many spaces to indent by when not compact; Tab uses a tab.
	Indent int
	Tab    bool

	// Args are values bound to named variables, the equivalent of jq's
	// --argjson.
	Args []Arg

	// MaxResults caps how many values the run may emit and MaxOutputBytes how
	// large they may be in total. A host with no user to interrupt it needs
	// both: `repeat(1)` is unbounded in count, and `[range(1e7)]` is a single
	// enormous value. Zero on either means unbounded, which is only sensible
	// when something else can stop the run. Clamping a caller's request into
	// something defensible is the host's job, not this package's.
	MaxResults     int
	MaxOutputBytes int

	// MaxValueBytes caps how large one rendered value may be. A value over it
	// is cut, with a marker saying how much was dropped.
	//
	// It sits between the two bounds above, which between them do not cover
	// the case that actually hurts a host answering a language model: a
	// handful of results, one of which is enormous. A single fetch of a
	// documentation page put nine kilobytes of Unicode samples into one such
	// caller's context, well inside a thousand-result limit and a 64MB byte
	// cap, and none of it was what the query was asking about.
	//
	// Zero means unbounded, which is right for a terminal: a user can see how
	// much came back and stop reading. A host whose caller cannot do that
	// should set it.
	MaxValueBytes int

	// ObserveShape asks the runner to describe the values the query produced:
	// how many there were, what kind each was, and which keys the objects
	// carried.
	//
	// It is off by default because a terminal user can see the output. A host
	// answering a language model cannot: the model gets the values as text and
	// has to infer their shape by reading all of them, which a truncated run
	// makes impossible. The observation is made from the raw value before
	// encoding, so it costs one pass over each result's top level.
	ObserveShape bool
}

// Result is what a run produced.
//
// Output and error are not exclusive: a query that emits ten values and then
// fails has told you something about all eleven, so both are reported.
type Result struct {
	// Values are the rendered results, one per query output.
	Values []string
	// Count is how many results were emitted, and InputCount how many values
	// the input decoded to.
	Count      int
	InputCount int
	// Truncated reports that a limit stopped the run before the query was
	// finished producing.
	Truncated bool
	// Elided is how many individual values MaxValueBytes cut. It is not the
	// same event as Truncated: the run produced every value it was going to,
	// and some of them are shown in part.
	Elided int
	// Halted reports that the query stopped itself with halt or halt_error.
	Halted bool
	// Error is why the run did not complete, and Kind classifies it: parse,
	// compile, args, input, runtime, timeout, limit or halt.
	Error string
	Kind  string

	// Shape describes the values that were produced, when ObserveShape asked
	// for it. It is what the query actually emitted rather than a claim about
	// what it should have, so it covers the cmdlets whose output keys come
	// from the data and could never be declared in advance.
	//
	// It is empty when the values speak for themselves - a few scalars, all
	// of them shown - and set when they do not: objects whose keys are worth
	// listing, or a run a limit cut short.
	Shape string
}

// Failure kinds. A caller that wants to point at the right editor, or decide
// whether a failure is the user's fault, matches on these rather than on the
// message.
const (
	KindParse   = "parse"
	KindCompile = "compile"
	KindArgs    = "args"
	KindInput   = "input"
	KindRuntime = "runtime"
	KindTimeout = "timeout"
	KindLimit   = "limit"
	KindHalt    = "halt"
)

// Runner evaluates queries against one fixed vocabulary.
//
// Building the vocabulary is not free — alias resolution compiles a program —
// and a runner may serve thousands of queries, so a caller builds one and
// keeps it. Nothing here is mutated by a run, so a Runner may be shared by
// concurrent callers as long as the cmdlets themselves are safe to share.
type Runner struct {
	// Options are the compiler options the query evaluates under: the
	// registry's cmdlets, and whatever else the host adds — its environment
	// loader, its debug and stderr sinks.
	Options []gojq.CompilerOption
	// AliasDefs are prepended to every query, which is the only point at
	// which a short name can come to mean a cmdlet: gojq binds function names
	// at compile time and never consults session state.
	AliasDefs []*gojq.FuncDef
}

// Clamp resolves a caller-supplied bound into one the host is willing to
// honour: zero or less asks for the default, and anything above the ceiling is
// capped there. Every host that lets a caller relax a limit needs this, for
// both result counts and timeouts, which is why it is generic over the two.
func Clamp[T ~int | ~int64](v, fallback, ceiling T) T {
	if v <= 0 {
		return fallback
	}
	return min(v, ceiling)
}

// Run evaluates a request and returns everything it produced.
//
// The deadline is ctx's. That is the one bound the runner cannot supply for
// itself: a native host wants context.WithTimeout, and the browser cannot use
// one at all — under GOOS=js a tight loop never yields, so a timer never
// fires and a sampled deadline is the only kind that works.
func (r *Runner) Run(ctx context.Context, req *Request) (result Result) {
	started := time.Now()
	resp := Result{Values: []string{}}

	// gojq checks the deadline between the values a program yields, which is
	// no check at all for a cmdlet that does all its work before yielding
	// anything. Publishing the context is what lets such a cmdlet stop by
	// itself; see runctx. Restored on the way out, so a run nested inside a
	// cmdlet leaves the outer run's deadline in place.
	defer runctx.Install(ctx)()

	// The observer is attached on the way out rather than at each return: a
	// run that failed halfway still produced values, and describing them is
	// most useful precisely then.
	var observer *shape.Observer
	if req.ObserveShape {
		observer = shape.NewObserver()
		defer func() {
			if observer.Notable(result.Truncated) {
				result.Shape = observer.Describe()
			}
		}()
	}

	query, res := r.parse(req)
	if query == nil {
		return res
	}

	inputs, err := decodeInputs(req)
	if err != nil {
		resp.Error = fmt.Sprintf("input is not JSON: %v", err)
		resp.Kind = KindInput
		return resp
	}
	resp.InputCount = len(inputs)

	names, values, err := bindArgs(req.Args)
	if err != nil {
		resp.Error = err.Error()
		resp.Kind = KindArgs
		return resp
	}

	// remaining is the input cursor. `input` and `inputs` read from it, and so
	// does the program itself once it is filled in below.
	remaining := &sliceIter{}
	options := append([]gojq.CompilerOption{}, r.Options...)
	options = append(options, gojq.WithInputIter(remaining))
	if len(names) > 0 {
		options = append(options, gojq.WithVariables(names))
	}

	code, err := gojq.Compile(query, options...)
	if err != nil {
		resp.Error = err.Error()
		resp.Kind = KindCompile
		return resp
	}

	// Under -n the program runs once on null, and every input stays readable
	// through `input` and `inputs`.
	//
	// Otherwise the program is run per input, drawing from the same cursor
	// those builtins read. Sharing the cursor is what makes them behave as
	// they do on the command line: `[., input]` over two values is one result
	// rather than two, because the second value was consumed by the first run
	// and there is nothing left to start a second.
	remaining.values = inputs
	nextInput := remaining.Next
	if req.NullInput {
		nextInput = onceOn(nil)
	}

	enc := newEncoder(req)
	bytesOut := 0

	for {
		input, ok := nextInput()
		if !ok {
			break
		}

		iter := code.RunWithContext(ctx, input, values...)
		for {
			v, ok := iter.Next()
			if !ok {
				break
			}

			if err, isErr := v.(error); isErr {
				// `halt` and `halt_error` end the run deliberately, so what
				// they carry is the message, not a failure of the host. Match
				// the concrete type: the interface every gojq error satisfies
				// would swallow `error("boom")` as a clean stop.
				if halt, ok := err.(*gojq.HaltError); ok {
					resp.Halted = true
					resp.Kind = KindHalt
					resp.Error = haltMessage(halt)
					return resp
				}
				if ctx.Err() == context.DeadlineExceeded {
					resp.Kind = KindTimeout
					resp.Error = timeoutMessage(ctx, started)
					return resp
				}
				resp.Kind = KindRuntime
				resp.Error = err.Error()
				return resp
			}

			// Observed before encoding: the raw value is the one with a
			// type, and re-parsing the rendered text would be both slower and
			// wrong under Raw, which does not emit JSON.
			observer.Add(v)

			text, err := enc.encode(v)
			if err != nil {
				resp.Kind = KindRuntime
				resp.Error = err.Error()
				return resp
			}
			if cut, elided := cutValue(text, req.MaxValueBytes); elided {
				text = cut
				resp.Elided++
			}
			resp.Values = append(resp.Values, text)
			resp.Count++
			bytesOut += len(text)

			if req.MaxResults > 0 && resp.Count >= req.MaxResults {
				resp.Truncated = true
				resp.Kind = KindLimit
				resp.Error = fmt.Sprintf("stopped after %d results; the query may produce an unbounded stream", req.MaxResults)
				return resp
			}
			if req.MaxOutputBytes > 0 && bytesOut >= req.MaxOutputBytes {
				resp.Truncated = true
				resp.Kind = KindLimit
				resp.Error = fmt.Sprintf("stopped after %d MB of output", req.MaxOutputBytes>>20)
				return resp
			}
		}
	}

	return resp
}

// cutValue trims one rendered value to a byte budget, marking what it dropped.
//
// The marked text is no longer valid JSON, and that is the honest cost: a
// caller that re-parses the values has to notice. The alternative - dropping
// the value entirely, or letting it through whole - is worse in both
// directions. What comes back tells a reader what the value looked like and
// how much of it there was, which is enough to decide whether to ask for a
// narrower slice of it or a larger budget.
//
// The cut lands on a rune boundary. Half a UTF-8 sequence renders as a
// replacement character, which reads as corrupted data rather than as the
// truncation the marker is about to announce.
func cutValue(text string, limit int) (string, bool) {
	if limit <= 0 || len(text) <= limit {
		return text, false
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return fmt.Sprintf("%s... (cut %d of %d bytes; slice the value or raise the byte budget to see the rest)",
		text[:cut], len(text)-cut, len(text)), true
}

// parse reads a query and prepends the alias definitions. It returns nil and
// a filled-in failure when the query cannot be parsed, so Run can bail before
// touching the input.
func (r *Runner) parse(req *Request) (*gojq.Query, Result) {
	if isBlank(req.Query) {
		return nil, Result{Values: []string{}, Error: "query is empty", Kind: KindParse}
	}
	query, err := gojq.Parse(req.Query)
	if err != nil {
		return nil, Result{Values: []string{}, Error: err.Error(), Kind: KindParse}
	}
	if len(r.AliasDefs) > 0 {
		query.FuncDefs = append(append([]*gojq.FuncDef{}, r.AliasDefs...), query.FuncDefs...)
	}
	return query, Result{}
}

// timeoutMessage describes the deadline the run hit.
//
// The figure is the budget that remained when the run started, which is a
// little less than the timeout the caller configured - short by however long
// it took to get from context.WithTimeout to here. A context exposes only an
// absolute deadline, never the duration it was built from, so that gap cannot
// be recovered; on a loaded machine it is easily a millisecond, which is why
// a 50ms deadline can read back as 49ms. Close enough to tell a user which
// limit they hit, and not something to assert an exact string against.
//
// It says what to do rather than what happened, because what to do is the part
// the caller cannot work out: the values above the message are real findings
// and reading them as the whole answer is the mistake this is here to prevent.
// It used to say the query was still running, which was true of a cmdlet that
// ignored the deadline and is no longer the ordinary case - see runctx.
func timeoutMessage(ctx context.Context, started time.Time) string {
	const advice = "any values above are what it produced before stopping, not the whole answer; raise the timeout to let it finish"
	at, ok := ctx.Deadline()
	if !ok {
		return "stopped early: " + advice
	}
	return fmt.Sprintf("stopped after %s: %s",
		at.Sub(started).Round(time.Millisecond), advice)
}

// haltMessage renders what a halt carried, mirroring how the CLI writes it to
// stderr: a string goes out as text, anything else as JSON, and a bare `halt`
// carries nothing at all.
func haltMessage(halt *gojq.HaltError) string {
	v := halt.Value()
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	encoded, err := gojq.Marshal(v)
	if err != nil {
		return halt.Error()
	}
	return string(encoded)
}

// onceOn yields a single value and then nothing, which is how -n feeds the
// program while leaving the real input untouched for `input` and `inputs`.
func onceOn(v any) func() (any, bool) {
	done := false
	return func() (any, bool) {
		if done {
			return nil, false
		}
		done = true
		return v, true
	}
}

// sliceIter feeds `input` and `inputs` from a slice.
type sliceIter struct {
	values []any
	pos    int
}

func (s *sliceIter) Next() (any, bool) {
	if s.pos >= len(s.values) {
		return nil, false
	}
	v := s.values[s.pos]
	s.pos++
	return v, true
}
