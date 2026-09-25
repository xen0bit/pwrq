// Package ideengine is what an editor for pwrq queries runs on.
//
// Validate this query, run it, tidy it, draw it, tell me what I can call: each
// is a typed method here. Every editor is a front end on it - the browser page
// in WASM (pkg/webapi), the same page served natively (pkg/webnative), and the
// terminal UI - so they cannot disagree about what a query means, where its
// error is, or what the vocabulary holds.
//
// What differs between hosts is decided by Config, not by a copy of the code:
// which registry evaluates, how a deadline is kept, whether a run gets a
// session of its own, and whether validation compiles as well as parses.
//
// Nothing here renders an image. A diagram is its D2 script unless the host
// supplies a renderer, because the one that exists (pkg/graph/graphsvg) brings
// d2 and about 35MB with it, and a terminal has no use for an SVG.
package ideengine

import (
	"context"
	"sync"
	"time"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/queryrun"
	"github.com/xen0bit/pwrq/pkg/graph"
	"github.com/xen0bit/pwrq/pkg/udf"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// Config is what makes one host's engine differ from another's.
type Config struct {
	// Registry is the vocabulary queries evaluate against.
	Registry *udf.Registry

	// Version is reported by the catalog, so a shared link's behaviour can be
	// traced to the build that produced it.
	Version string

	// RunOptions are compiler options a run adds to the registry's own: the
	// environment loader, and where debug and stderr write. They belong to the
	// host because the right sink does - stderr is safe under an HTTP server
	// and ruinous under a terminal UI that owns the screen.
	RunOptions []gojq.CompilerOption

	// CompileOnValidate makes validation compile as well as parse, so an
	// unknown name or a wrong arity is reported while typing rather than at
	// run time.
	CompileOnValidate bool

	// PrivateSession gives each run a session state of its own: cmdlets that
	// touch variables, aliases and drives see one another within one query
	// but nothing after it.
	PrivateSession bool

	// Limits bound a run. The zero value is DefaultLimits.
	Limits Limits

	// Deadline turns a run's timeout into a context. It defaults to
	// context.WithTimeout; a host where timers cannot fire supplies its own.
	Deadline func(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc)

	// RenderSVG draws a diagram as an image. Without it a diagram is its D2
	// script alone.
	RenderSVG func(query *gojq.Query, opts graph.RenderOptions) (string, error)
}

// Limits bound what a run may cost. A request may relax the defaults, but not
// past the ceilings, and never to the point of hanging the host.
type Limits struct {
	DefaultResults   int
	MaxResults       int
	DefaultTimeoutMs int
	MaxTimeoutMs     int
	// MaxOutputBytes stops a query whose values are enormous even though
	// there are few of them - `[range(1e7)]` is one result.
	MaxOutputBytes int
}

// DefaultLimits are the browser page's: generous enough for real work, tight
// enough that a tab with no Ctrl-C always comes back.
var DefaultLimits = Limits{
	DefaultResults:   10000,
	MaxResults:       1000000,
	DefaultTimeoutMs: 5000,
	MaxTimeoutMs:     120000,
	MaxOutputBytes:   16 << 20,
}

// Engine evaluates queries against one vocabulary.
//
// Building it is not free - alias resolution compiles a program - and an
// editor calls in on every keystroke, so a host builds one and keeps it.
// Everything but the session state is immutable after New; runs are
// serialised because cmdlets read that state through a package-level global.
type Engine struct {
	config    Config
	aliases   []udf.Alias
	aliasDefs []*gojq.FuncDef
	cmdlets   map[string]bool
	names     []string

	runner *queryrun.Runner
	execMu sync.Mutex
}

// New builds an engine from a host's configuration.
func New(config Config) *Engine {
	if config.Limits == (Limits{}) {
		config.Limits = DefaultLimits
	}
	if config.Deadline == nil {
		config.Deadline = context.WithTimeout
	}

	reg := config.Registry
	e := &Engine{config: config}
	options := reg.Options()

	if known, err := reg.KnownAliases(udf.StandardAliases); err == nil {
		e.aliases = known
		if defs, err := reg.AliasFuncDefs(known); err == nil {
			e.aliasDefs = defs
		}
	}

	if names, err := reg.Names(); err == nil {
		e.names = names
		e.cmdlets = make(map[string]bool, len(names))
		for _, name := range names {
			e.cmdlets[name] = true
		}
	}

	// Script blocks handed to cmdlets compile against the same vocabulary as
	// the surrounding query.
	common.SetScriptBlockOptions(options)

	runOptions := append(append([]gojq.CompilerOption{}, options...), config.RunOptions...)
	e.runner = &queryrun.Runner{Options: runOptions, AliasDefs: e.aliasDefs}
	return e
}

// Cmdlets lists the names this engine's registry provides, sorted.
func (e *Engine) Cmdlets() []string { return e.names }
