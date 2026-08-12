package mcpserver

import (
	"context"
	"time"

	"github.com/xen0bit/pwrq/pkg/core/queryrun"
	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// Limits the server runs under. An MCP client has no Ctrl-C: a query like
// `repeat(1)` is an infinite stream, and without a bound the server would be
// stuck for ever. The defaults are generous for a local tool, and a call can
// relax them up to the caps.
const (
	defaultMaxResults = 1000
	maxMaxResults     = 100000
	defaultTimeout    = 30 * time.Second
	maxTimeout        = 10 * time.Minute
	// maxOutputBytes stops a query whose *values* are enormous even though
	// there are few of them - `[range(1e7)]` is one result.
	maxOutputBytes = 64 << 20
)

// execute runs a query to completion under the engine's vocabulary. It is
// called with the engine mutex held, and installs a fresh, private session
// state for the duration of the run: cmdlets like set_variable and set_location
// work within a single call and never leak into the next.
func (e *engine) execute(req runQueryArgs) runQueryResult {
	timeout := queryrun.Clamp(time.Duration(req.TimeoutMs)*time.Millisecond, defaultTimeout, maxTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := make([]queryrun.Arg, len(req.Args))
	for i, arg := range req.Args {
		args[i] = queryrun.Arg{Name: arg.Name, Value: arg.Value}
	}

	// A private session per run: the cmdlets that touch variables, aliases and
	// drives see one another within this query but nothing after it.
	common.SetGlobalSessionState(sessionstate.NewSessionState())
	defer common.SetGlobalSessionState(nil)

	started := time.Now()
	res := e.runner.Run(ctx, &queryrun.Request{
		Query:          req.Query,
		Input:          req.Input,
		RawInput:       req.RawInput,
		Slurp:          req.Slurp,
		NullInput:      req.NullInput,
		Raw:            req.Raw,
		Compact:        req.Compact,
		Indent:         req.Indent,
		Args:           args,
		MaxResults:     queryrun.Clamp(req.Limit, defaultMaxResults, maxMaxResults),
		MaxOutputBytes: maxOutputBytes,
	})

	return runQueryResult{
		Values:    res.Values,
		Count:     res.Count,
		Truncated: res.Truncated,
		Error:     res.Error,
		Kind:      res.Kind,
		ElapsedMs: float64(time.Since(started).Microseconds()) / 1000,
	}
}
