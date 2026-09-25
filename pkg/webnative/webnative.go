// Package webnative is the engine behind the native browser IDE.
//
// It speaks the same method protocol as pkg/webapi (validate, run, diagram,
// format, minify, inline, catalog), because both are pkg/ideengine answering
// through the same Call, so the browser page works unchanged against either.
// What differs is the configuration: webapi evaluates against
// udf.WebRegistry() inside a WASM worker with no environment, while this
// package evaluates against udf.DefaultRegistry() natively, with the process
// environment, per-call session state, real cancellable deadlines, and
// validation that compiles - the same posture as pkg/mcpserver.
//
// This package is deliberately separate from webapi rather than a flag on it:
// webapi is compiled to GOOS=js/wasm, and importing the default registry there
// would link every native cmdlet into the browser module. Nothing here may be
// imported by the WASM build path.
package webnative

import (
	"context"
	"os"
	"os/user"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/graph/graphsvg"
	"github.com/xen0bit/pwrq/pkg/ideengine"
	"github.com/xen0bit/pwrq/pkg/udf"
	"github.com/xen0bit/pwrq/pkg/udf/common"
	"github.com/xen0bit/pwrq/pkg/webapi"
)

// Limits the native page runs under. The UX defaults match the WASM page so a
// shared link behaves the same in either tab; the ceilings are higher because
// the server can actually spend the time — a corpus-wide search is minutes of
// work, and a ceiling below that turns "this takes a while" into "this tool
// does not work". Every bound is clamped per call.
var limits = ideengine.Limits{
	DefaultResults:   10000,
	MaxResults:       1000000,
	DefaultTimeoutMs: 5000,
	MaxTimeoutMs:     3600000,
	MaxOutputBytes:   16 << 20,
}

// Methods is the capability set this engine answers, in the same names the
// WASM engine answers, so one page drives either.
var Methods = []string{"validate", "run", "diagram", "format", "minify", "inline", "catalog"}

// NativeValidateRequest is the validate request the native page sends: a
// query and the argument names it may read.
type NativeValidateRequest = ideengine.ValidateRequest

// Engine evaluates queries against the full native vocabulary.
type Engine struct {
	core *ideengine.Engine
}

// New builds the native engine over the full cmdlet vocabulary.
func New() *Engine {
	return &Engine{core: ideengine.New(ideengine.Config{
		Registry: udf.DefaultRegistry(),
		Version:  webapi.Version,
		RunOptions: []gojq.CompilerOption{
			gojq.WithEnvironLoader(os.Environ),
			// debug and stderr are registered here exactly as the CLI
			// registers them; gojq has no such builtins. stderr is the only
			// safe sink: it never corrupts the HTTP response the browser is
			// reading.
			common.WithFunction("debug", 0, 0, writeToStderr),
			common.WithFunction("stderr", 0, 0, writeToStderr),
		},
		CompileOnValidate: true,
		PrivateSession:    true,
		Limits:            limits,
		RenderSVG:         graphsvg.GenerateSVGOpts,
	})}
}

func writeToStderr(v any, _ []any) any {
	if b, err := gojq.Marshal(v); err == nil {
		_, _ = os.Stderr.Write(append(b, '\n'))
	}
	return v
}

// Call dispatches a named request. The deadline of a run is ctx's as well as
// the request's timeout, so a client that disconnects cancels its run.
func (e *Engine) Call(ctx context.Context, method, request string) string {
	return e.core.Call(ctx, method, request)
}

// HealthResponse describes the native server behind the page. It powers the
// banner that tells the user where their queries actually run.
type HealthResponse struct {
	Mode    string `json:"mode"`
	Version string `json:"version"`
	Cmdlets int    `json:"cmdlets"`
	User    string `json:"user"`
	Cwd     string `json:"cwd"`
}

// Health reports what this engine is.
func (e *Engine) Health() string {
	userName := ""
	if u, err := user.Current(); err == nil {
		userName = u.Username
	}
	cwd, _ := os.Getwd()
	return ideengine.Marshal(HealthResponse{
		Mode:    "native",
		Version: webapi.Version,
		Cmdlets: len(e.core.Cmdlets()),
		User:    userName,
		Cwd:     cwd,
	})
}
