package ideengine

import (
	"context"
	"time"

	"github.com/xen0bit/pwrq/pkg/core/queryrun"
	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RunRequest is a query, its input, and how to present what comes out.
type RunRequest struct {
	Query string `json:"query"`
	Input string `json:"input"`

	// RawInput reads the input as lines of text rather than JSON, as jq -R
	// does.
	RawInput bool `json:"rawInput,omitempty"`
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
	// take. Both have defaults, and both are clamped to the engine's Limits.
	Limit     int `json:"limit"`
	TimeoutMs int `json:"timeoutMs"`

	// Args are values bound to named variables, the equivalent of jq's
	// --argjson. They are what makes a shared query reusable: the link carries
	// the program, the arguments carry the case.
	Args []Arg `json:"args"`

	// InputName is what input_filename reports, and Positional what
	// $ARGS.positional holds: jq's --args and --jsonargs. A host reading a
	// file has both; the browser has neither, so they are not on the wire.
	InputName  string `json:"-"`
	Positional []any  `json:"-"`
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
	// Kind classifies the failure so the editor can point at the right pane:
	// parse, compile, args, input, runtime, timeout or halt.
	Kind      string  `json:"kind,omitempty"`
	Halted    bool    `json:"halted,omitempty"`
	ElapsedMs float64 `json:"elapsedMs"`
}

// Run evaluates a query against its input.
//
// Cancelling ctx ends the run, as does the request's timeout; either way the
// values produced so far are reported. Runs are serialised, so a caller
// replacing one run with another should cancel the first before starting the
// second.
func (e *Engine) Run(ctx context.Context, req RunRequest) RunResponse {
	e.execMu.Lock()
	defer e.execMu.Unlock()

	if e.config.PrivateSession {
		common.SetGlobalSessionState(sessionstate.NewSessionState())
		defer common.SetGlobalSessionState(nil)
	}

	limits := e.config.Limits
	timeout := time.Duration(queryrun.Clamp(req.TimeoutMs, limits.DefaultTimeoutMs, limits.MaxTimeoutMs)) * time.Millisecond
	ctx, cancel := e.config.Deadline(ctx, timeout)
	defer cancel()

	args := make([]queryrun.Arg, len(req.Args))
	for i, arg := range req.Args {
		args[i] = queryrun.Arg{Name: arg.Name, Value: arg.Value}
	}

	started := time.Now()
	res := e.runner.Run(ctx, &queryrun.Request{
		Query:          req.Query,
		Input:          req.Input,
		RawInput:       req.RawInput,
		InputName:      req.InputName,
		Positional:     req.Positional,
		Slurp:          req.Slurp,
		NullInput:      req.NullInput,
		Raw:            req.Raw,
		Compact:        req.Compact,
		Indent:         req.Indent,
		Tab:            req.Tab,
		Args:           args,
		MaxResults:     queryrun.Clamp(req.Limit, limits.DefaultResults, limits.MaxResults),
		MaxOutputBytes: limits.MaxOutputBytes,
	})

	return RunResponse{
		Values:     res.Values,
		Count:      res.Count,
		InputCount: res.InputCount,
		Truncated:  res.Truncated,
		Halted:     res.Halted,
		Error:      res.Error,
		Kind:       res.Kind,
		ElapsedMs:  float64(time.Since(started).Microseconds()) / 1000,
	}
}
