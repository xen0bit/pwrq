package webapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/itchyny/gojq"
)

// Limits the page runs under. A browser tab has no Ctrl-C: a query like
// `repeat(1)` is an infinite stream, and without a bound the tab is lost with
// no way out but closing it.
const (
	defaultMaxResults = 10000
	maxMaxResults     = 1000000
	defaultTimeoutMs  = 5000
	maxTimeoutMs      = 120000
	// maxOutputBytes stops a query whose *values* are enormous even though
	// there are few of them - `[range(1e7)]` is one result.
	maxOutputBytes = 16 << 20
)

// RunRequest is a query, its input, and how to present what comes out.
type RunRequest struct {
	Query string `json:"query"`
	Input string `json:"input"`

	// Slurp reads the whole input as a single array, as jq -s does.
	Slurp bool `json:"slurp"`
	// NullInput ignores the input entirely, as jq -n does.
	NullInput bool `json:"nullInput"`
	// Raw prints string results without quotes, as jq -r does.
	Raw bool `json:"raw"`
	// Compact prints each result on one line, as jq -c does.
	Compact bool `json:"compact"`
	// Indent is how many spaces to indent by when not compact; Tab uses a tab.
	Indent int  `json:"indent"`
	Tab    bool `json:"tab"`

	// Limit caps the number of results; TimeoutMs caps how long the run may
	// take. Both have defaults, and both are clamped: the page may relax them,
	// but not to the point of hanging the tab.
	Limit     int `json:"limit"`
	TimeoutMs int `json:"timeoutMs"`

	// Args are values bound to named variables, the equivalent of jq's
	// --argjson. They are what makes a shared query reusable: the link carries
	// the program, the arguments carry the case.
	Args []Arg `json:"args"`
}

// Arg binds one named variable. Value is JSON text, so any value can be bound.
type Arg struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// RunResponse is what a run produced.
//
// Output and error are not exclusive: a query that emits ten values and then
// fails has told you something about all eleven, so both are reported.
type RunResponse struct {
	Values     []string `json:"values"`
	Count      int      `json:"count"`
	InputCount int      `json:"inputCount"`
	Truncated  bool     `json:"truncated"`
	Error      string   `json:"error,omitempty"`
	// Kind classifies the failure so the page can point at the right editor:
	// parse, compile, args, input, runtime, timeout or halt.
	Kind      string  `json:"kind,omitempty"`
	Halted    bool    `json:"halted,omitempty"`
	ElapsedMs float64 `json:"elapsedMs"`
}

// Run evaluates a query against the sample input.
func Run(request string) string {
	var req RunRequest
	if err := json.Unmarshal([]byte(request), &req); err != nil {
		return marshal(RunResponse{Error: "malformed request: " + err.Error(), Kind: "request"})
	}

	started := time.Now()
	resp := run(&req)
	resp.ElapsedMs = float64(time.Since(started).Microseconds()) / 1000
	return marshal(resp)
}

func run(req *RunRequest) RunResponse {
	var resp RunResponse

	if strings.TrimSpace(req.Query) == "" {
		resp.Error = "query is empty"
		resp.Kind = "parse"
		return resp
	}

	e := getEngine()

	query, err := e.prepare(req.Query)
	if err != nil {
		resp.Error = err.Error()
		resp.Kind = "parse"
		return resp
	}

	names, values, err := bindArgs(req.Args)
	if err != nil {
		resp.Error = err.Error()
		resp.Kind = "args"
		return resp
	}

	inputs, err := decodeInputs(req.Input)
	if err != nil {
		resp.Error = fmt.Sprintf("input is not JSON: %v", err)
		resp.Kind = "input"
		return resp
	}
	resp.InputCount = len(inputs)

	switch {
	case req.NullInput:
		// jq -n: the inputs are still readable through `input`/`inputs`, they
		// are just not fed to the program.
		inputs = nil
	case req.Slurp:
		all := make([]any, len(inputs))
		copy(all, inputs)
		inputs = []any{all}
	}

	options := append([]gojq.CompilerOption{}, e.options...)
	if len(names) > 0 {
		options = append(options, gojq.WithVariables(names))
	}
	// `input` and `inputs` read whatever the program was not handed, which is
	// how jq behaves and what makes -n useful on a multi-value input.
	remaining := &sliceIter{}
	options = append(options, gojq.WithInputIter(remaining))
	// A browser tab has no environment. Reporting the WASM runtime's is
	// meaningless, so `env` and `$ENV` are empty rather than misleading.
	options = append(options, gojq.WithEnvironLoader(func() []string { return nil }))

	code, err := gojq.Compile(query, options...)
	if err != nil {
		resp.Error = err.Error()
		resp.Kind = "compile"
		return resp
	}

	limit := req.Limit
	if limit <= 0 {
		limit = defaultMaxResults
	}
	if limit > maxMaxResults {
		limit = maxMaxResults
	}

	timeout := req.TimeoutMs
	if timeout <= 0 {
		timeout = defaultTimeoutMs
	}
	if timeout > maxTimeoutMs {
		timeout = maxTimeoutMs
	}
	ctx := newDeadline(time.Duration(timeout) * time.Millisecond)

	enc := newEncoder(req)
	program := inputs
	if req.NullInput {
		// The program runs once, on null; `inputs` still sees the file.
		program = []any{nil}
		remaining.values = decodedOrEmpty(req.Input)
	}

	bytesOut := 0
	for i, input := range program {
		// Whatever this run has not consumed yet is what `input` returns.
		if !req.NullInput {
			remaining.values = program[i+1:]
			remaining.pos = 0
		}

		iter := code.RunWithContext(ctx, input, values...)
		for {
			v, ok := iter.Next()
			if !ok {
				break
			}

			if err, isErr := v.(error); isErr {
				// `halt` and `halt_error` end the run deliberately, so what
				// they carry is the message, not a failure of the page. Match
				// the concrete type: the interface every gojq error satisfies
				// would swallow `error("boom")` as a clean stop.
				if halt, ok := err.(*gojq.HaltError); ok {
					resp.Halted = true
					resp.Kind = "halt"
					resp.Error = haltMessage(halt)
					return resp
				}
				if ctx.expired() {
					resp.Kind = "timeout"
					resp.Error = fmt.Sprintf("stopped after %dms; the query is still running", timeout)
					return resp
				}
				resp.Kind = "runtime"
				resp.Error = err.Error()
				return resp
			}

			text, err := enc.encode(v)
			if err != nil {
				resp.Kind = "runtime"
				resp.Error = err.Error()
				return resp
			}
			resp.Values = append(resp.Values, text)
			resp.Count++
			bytesOut += len(text)

			if resp.Count >= limit {
				resp.Truncated = true
				resp.Error = fmt.Sprintf("stopped after %d results; the query may produce an unbounded stream", limit)
				resp.Kind = "limit"
				return resp
			}
			if bytesOut >= maxOutputBytes {
				resp.Truncated = true
				resp.Error = fmt.Sprintf("stopped after %d MB of output", maxOutputBytes>>20)
				resp.Kind = "limit"
				return resp
			}
		}
	}

	return resp
}

// bindArgs turns named JSON arguments into the variable names and values gojq
// wants. Names may be written with or without the dollar.
func bindArgs(args []Arg) ([]string, []any, error) {
	if len(args) == 0 {
		return nil, nil, nil
	}
	names := make([]string, 0, len(args))
	values := make([]any, 0, len(args))
	seen := make(map[string]bool, len(args))

	for _, arg := range args {
		name := strings.TrimSpace(arg.Name)
		if name == "" {
			continue
		}
		if !strings.HasPrefix(name, "$") {
			name = "$" + name
		}
		if !validVariableName(name) {
			return nil, nil, fmt.Errorf("%q is not a valid variable name", arg.Name)
		}
		if seen[name] {
			return nil, nil, fmt.Errorf("%s is bound twice", name)
		}
		seen[name] = true

		text := strings.TrimSpace(arg.Value)
		if text == "" {
			text = "null"
		}
		dec := json.NewDecoder(strings.NewReader(text))
		dec.UseNumber()
		var value any
		if err := dec.Decode(&value); err != nil {
			return nil, nil, fmt.Errorf("%s is not JSON: %w", name, err)
		}
		names = append(names, name)
		values = append(values, value)
	}
	return names, values, nil
}

func validVariableName(name string) bool {
	if len(name) < 2 || name[0] != '$' {
		return false
	}
	for i := 1; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_':
		case c >= '0' && c <= '9' && i > 1:
		default:
			return false
		}
	}
	return true
}

// decodeInputs reads the sample input as a stream of JSON values, matching how
// the CLI reads a file: `{"a":1} {"a":2}` is two inputs, not a syntax error.
// Empty input is a single null, so a generator query like `[limit(3; repeat(1))]`
// still runs.
func decodeInputs(s string) ([]any, error) {
	if strings.TrimSpace(s) == "" {
		return []any{nil}, nil
	}

	dec := json.NewDecoder(bytes.NewReader([]byte(s)))
	// The CLI decodes with UseNumber() and so must this: json.Number is a
	// fmt.Stringer, and without it every number in the input is silently
	// turned into a string somewhere downstream.
	dec.UseNumber()

	var inputs []any
	for {
		var v any
		if err := dec.Decode(&v); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		inputs = append(inputs, v)
	}
	if len(inputs) == 0 {
		return []any{nil}, nil
	}
	return inputs, nil
}

// decodedOrEmpty is decodeInputs for the -n case, where a malformed input has
// already been reported and must not stop the program from running.
func decodedOrEmpty(s string) []any {
	inputs, err := decodeInputs(s)
	if err != nil {
		return nil
	}
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return inputs
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

// ---------------------------------------------------------------------------
// deadlines

// deadline is a context that expires without needing a timer.
//
// Under GOOS=js there is one thread and no preemption: a goroutine running a
// tight loop never yields, so the JavaScript event loop never runs, so a timer
// set by context.WithTimeout never fires. A deadline built on time.AfterFunc
// would therefore protect the page from exactly nothing.
//
// gojq calls ctx.Done() once per instruction it executes, so checking the
// clock there works where a timer cannot. The check is sampled rather than
// taken every time, because time.Now() is not free.
//
// It is used by one run at a time and is not safe for concurrent use.
type deadline struct {
	at        time.Time
	done      chan struct{}
	closed    bool
	countdown int
}

// clockCheckInterval is how many instructions pass between clock readings.
// Small enough that a runaway query is caught promptly, large enough that the
// check does not dominate the run.
const clockCheckInterval = 2000

func newDeadline(d time.Duration) *deadline {
	return &deadline{at: time.Now().Add(d), done: make(chan struct{}), countdown: clockCheckInterval}
}

func (d *deadline) Deadline() (time.Time, bool) { return d.at, true }

func (d *deadline) Done() <-chan struct{} {
	if d.closed {
		return d.done
	}
	d.countdown--
	if d.countdown > 0 {
		return d.done
	}
	d.countdown = clockCheckInterval
	if !time.Now().Before(d.at) {
		d.closed = true
		close(d.done)
	}
	return d.done
}

func (d *deadline) Err() error {
	if d.closed {
		return context.DeadlineExceeded
	}
	return nil
}

func (d *deadline) Value(any) any { return nil }

// expired reports whether the deadline is what stopped the run.
func (d *deadline) expired() bool { return d.closed }
