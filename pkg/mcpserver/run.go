package mcpserver

import (
	"context"
	"time"

	"github.com/itchyny/gojq"

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
	// maxTimeout is what a caller may ask for. An hour rather than the ten
	// minutes it was, because the thing that needs the time is real: a corpus
	// of rules over a repository is minutes of work even now that the files
	// are parsed once, and a ceiling below what the work costs turns "this
	// takes a while" into "this tool does not work". The ceiling still exists
	// because the caller has no Ctrl-C, and a run that reaches it now stops
	// when it is told to rather than when it happens to yield - see runctx.
	maxTimeout = time.Hour
	// maxOutputBytes stops a query whose *values* are enormous even though
	// there are few of them - `[range(1e7)]` is one result.
	maxOutputBytes = 64 << 20
	// defaultMaxValueBytes caps one rendered value, which neither bound above
	// does: a handful of results, one of them enormous, passes both.
	//
	// It exists because the caller here reads the values as text and pays for
	// all of them. One fetch of a documentation page in a recorded session put
	// nine kilobytes of Unicode samples into that caller's context - inside
	// both limits, and none of it what the query was about. Eight kilobytes is
	// roomy for a value someone means to read and small enough that an
	// accident stays an accident, and a caller who wants the whole thing says
	// so with maxBytes.
	defaultMaxValueBytes = 8 << 10
	maxMaxValueBytes     = 1 << 20
)

// execute runs a query to completion under the engine's vocabulary. It is
// called with the engine mutex held, and installs a fresh, private session
// state for the duration of the run: cmdlets like set_variable and set_location
// work within a single call and never leak into the next.
func (e *engine) execute(ctx context.Context, req runQueryArgs) runQueryResult {
	timeout := queryrun.Clamp(time.Duration(req.TimeoutMs)*time.Millisecond, defaultTimeout, maxTimeout)
	// Derived from the call's context rather than from Background, so that a
	// client which gives up on a call - or a transport that goes away - ends
	// the run rather than leaving it to finish into nothing while every later
	// call queues behind it.
	ctx, cancel := context.WithTimeout(ctx, timeout)
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
		MaxValueBytes:  queryrun.Clamp(req.MaxBytes, defaultMaxValueBytes, maxMaxValueBytes),
		// The caller is a language model reading the values as text. It cannot
		// see the output the way a terminal user can, and a run stopped by the
		// limit gives it no way to know what the rest looked like.
		ObserveShape: true,
	})

	return runQueryResult{
		Values:    res.Values,
		Count:     res.Count,
		Truncated: res.Truncated,
		Elided:    res.Elided,
		// Reported whether or not the run succeeded. A mismatch is usually
		// why a clean run is wrong, and occasionally why a failed one failed:
		// hex reaching base64_decode is both the warning and the error.
		Warnings:  warnEncodings(req.Query),
		Error:     res.Error,
		Kind:      res.Kind,
		Shape:     res.Shape,
		ElapsedMs: float64(time.Since(started).Microseconds()) / 1000,
	}
}

// warnEncodings re-parses a query to check its pipe stages against the encoding
// declarations.
//
// The second parse is deliberate. The runner owns its own parse and returning
// the AST from it would put a diagnostic concern into the shared evaluation
// path that the browser IDE also uses; parsing is microseconds against a run
// that reaches the network, and a query that does not parse has an error of its
// own and needs no warning on top of it.
func warnEncodings(query string) []encodingWarning {
	parsed, err := gojq.Parse(query)
	if err != nil {
		return nil
	}
	return checkEncodings(parsed)
}
