package webapi

import (
	"context"
	"encoding/json"
	"time"

	"github.com/xen0bit/pwrq/pkg/core/queryrun"
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
	e := getEngine()

	args := make([]queryrun.Arg, len(req.Args))
	for i, arg := range req.Args {
		args[i] = queryrun.Arg{Name: arg.Name, Value: arg.Value}
	}

	ctx := newDeadline(time.Duration(queryrun.Clamp(req.TimeoutMs, defaultTimeoutMs, maxTimeoutMs)) * time.Millisecond)
	res := e.runner.Run(ctx, &queryrun.Request{
		Query:          req.Query,
		Input:          req.Input,
		Slurp:          req.Slurp,
		NullInput:      req.NullInput,
		Raw:            req.Raw,
		Compact:        req.Compact,
		Indent:         req.Indent,
		Tab:            req.Tab,
		Args:           args,
		MaxResults:     queryrun.Clamp(req.Limit, defaultMaxResults, maxMaxResults),
		MaxOutputBytes: maxOutputBytes,
	})

	return RunResponse{
		Values:     res.Values,
		Count:      res.Count,
		InputCount: res.InputCount,
		Truncated:  res.Truncated,
		Halted:     res.Halted,
		Error:      res.Error,
		Kind:       res.Kind,
	}
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
