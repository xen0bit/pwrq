// Package webnative is the engine behind the native browser IDE.
//
// It speaks the same method protocol as pkg/webapi (validate, run, diagram,
// format, minify, inline, catalog) and reuses its wire types, so the browser
// page works unchanged against either engine. What differs is the vocabulary
// and the host: webapi evaluates against udf.WebRegistry() inside a WASM
// worker with no environment, while this package evaluates against
// udf.DefaultRegistry() natively, with the process environment, per-call
// session state, and real cancellable deadlines — the same posture as
// pkg/mcpserver.
//
// This package is deliberately separate from webapi rather than a flag on it:
// webapi is compiled to GOOS=js/wasm, and importing the default registry there
// would link every native cmdlet into the browser module. Nothing here may be
// imported by the WASM build path.
package webnative

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/queryrun"
	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
	"github.com/xen0bit/pwrq/pkg/graph"
	"github.com/xen0bit/pwrq/pkg/jqfmt"
	"github.com/xen0bit/pwrq/pkg/jqinline"
	"github.com/xen0bit/pwrq/pkg/udf"
	"github.com/xen0bit/pwrq/pkg/udf/common"
	"github.com/xen0bit/pwrq/pkg/udf/discovery"
	"github.com/xen0bit/pwrq/pkg/webapi"
)

// Limits the native page runs under. The UX defaults match the WASM page so a
// shared link behaves the same in either tab; the ceilings are higher because
// the server can actually spend the time — a corpus-wide search is minutes of
// work, and a ceiling below that turns "this takes a while" into "this tool
// does not work". Every bound is clamped per call.
const (
	defaultMaxResults = 10000
	maxMaxResults     = 1000000
	defaultTimeoutMs  = 5000
	maxTimeoutMs      = 3600000
	// maxOutputBytes stops a query whose *values* are enormous even though
	// there are few of them - `[range(1e7)]` is one result.
	maxOutputBytes = 16 << 20
)

// Methods is the capability set this engine answers, in the same names the
// WASM engine answers, so one page drives either.
var Methods = []string{"validate", "run", "diagram", "format", "minify", "inline", "catalog"}

// Engine evaluates queries against the full native vocabulary.
//
// Building it is not free - alias resolution compiles a program - and a
// server may serve thousands of calls, so it is built once. The runner and
// everything it holds are immutable after construction; what is not shared is
// the session state, which cmdlets read through a package-level global, so
// each run installs a fresh private session under execMu, exactly as the MCP
// server does.
type Engine struct {
	registry  *udf.Registry
	options   []gojq.CompilerOption
	aliases   []udf.Alias
	aliasDefs []*gojq.FuncDef
	cmdlets   map[string]bool
	names     []string

	runner *queryrun.Runner
	execMu sync.Mutex
}

// New builds the native engine over the full cmdlet vocabulary.
func New() *Engine {
	reg := udf.DefaultRegistry()
	e := &Engine{registry: reg, options: reg.Options()}

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
	common.SetScriptBlockOptions(e.options)

	options := append([]gojq.CompilerOption{}, e.options...)
	options = append(options, gojq.WithEnvironLoader(os.Environ))
	// debug and stderr are registered here exactly as the CLI registers
	// them; gojq has no such builtins. stderr is the only safe sink: it
	// never corrupts the HTTP response the browser is reading.
	options = append(options,
		common.WithFunction("debug", 0, 0, writeToStderr),
		common.WithFunction("stderr", 0, 0, writeToStderr),
	)

	e.runner = &queryrun.Runner{Options: options, AliasDefs: e.aliasDefs}
	return e
}

func writeToStderr(v any, _ []any) any {
	if b, err := gojq.Marshal(v); err == nil {
		_, _ = os.Stderr.Write(append(b, '\n'))
	}
	return v
}

// errorResponse is the shape every response degrades to when a request cannot
// even be read, so the page never has to handle a non-JSON reply.
type errorResponse struct {
	Error string `json:"error"`
}

// Call dispatches a named request. The protocol mirrors webapi.Call - a
// method name and a JSON string - so the page needs no second code path.
func (e *Engine) Call(ctx context.Context, method, request string) string {
	switch method {
	case "validate":
		return e.Validate(request)
	case "run":
		return e.Run(ctx, request)
	case "diagram":
		return e.Diagram(request)
	case "format":
		return Format(request)
	case "minify":
		return Minify(request)
	case "inline":
		return Inline(request)
	case "catalog":
		return e.Catalog(request)
	default:
		return marshal(errorResponse{Error: fmt.Sprintf("unknown method %q", method)})
	}
}

// ---------------------------------------------------------------------------
// health

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
	return marshal(HealthResponse{
		Mode:    "native",
		Version: webapi.Version,
		Cmdlets: len(e.names),
		User:    userName,
		Cwd:     cwd,
	})
}

// ---------------------------------------------------------------------------
// run

// Run evaluates a query against the caller's input under the full vocabulary.
//
// The deadline is ctx's, which the HTTP handler builds from the request's
// timeoutMs: unlike the WASM page's sampled clock, a native host can use a
// real context timeout, and client disconnects cancel the run.
func (e *Engine) Run(ctx context.Context, request string) string {
	var req webapi.RunRequest
	if err := json.Unmarshal([]byte(request), &req); err != nil {
		return marshal(webapi.RunResponse{Error: "malformed request: " + err.Error(), Kind: "request"})
	}

	e.execMu.Lock()
	defer e.execMu.Unlock()

	// A private session per run: the cmdlets that touch variables, aliases
	// and drives see one another within this query but nothing after it.
	common.SetGlobalSessionState(sessionstate.NewSessionState())
	defer common.SetGlobalSessionState(nil)

	timeout := time.Duration(queryrun.Clamp(req.TimeoutMs, defaultTimeoutMs, maxTimeoutMs)) * time.Millisecond
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := make([]queryrun.Arg, len(req.Args))
	for i, arg := range req.Args {
		args[i] = queryrun.Arg{Name: arg.Name, Value: arg.Value}
	}

	started := time.Now()
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

	return marshal(webapi.RunResponse{
		Values:     res.Values,
		Count:      res.Count,
		InputCount: res.InputCount,
		Truncated:  res.Truncated,
		Halted:     res.Halted,
		Error:      res.Error,
		Kind:       res.Kind,
		ElapsedMs:  float64(time.Since(started).Microseconds()) / 1000,
	})
}

// ---------------------------------------------------------------------------
// validate

// NativeValidateRequest asks whether a query parses and compiles. Args names
// the variables the query may read, so one that mentions $name compiles here
// as it would in a run; only the names are used.
type NativeValidateRequest struct {
	Query string       `json:"query"`
	Args  []webapi.Arg `json:"args"`
}

// Validate parses a query and compiles it against the native vocabulary.
//
// The WASM page's validate stops after parsing; this one also compiles, like
// the MCP server's validate_query, so an unknown cmdlet or a wrong arity is
// reported here rather than at run time. The response shape is
// webapi.ValidateResponse either way, so the page renders both identically.
func (e *Engine) Validate(request string) string {
	var req NativeValidateRequest
	if err := json.Unmarshal([]byte(request), &req); err != nil {
		return marshal(webapi.ValidateResponse{Error: "malformed request: " + err.Error()})
	}

	if strings.TrimSpace(req.Query) == "" {
		return marshal(webapi.ValidateResponse{Empty: true})
	}

	query, err := gojq.Parse(req.Query)
	if err != nil {
		return marshal(parseFailure(req.Query, err))
	}

	if err := e.compile(query, req.Args); err != nil {
		return marshal(webapi.ValidateResponse{Error: err.Error(), Formatted: query.String()})
	}
	return marshal(parseOK(query))
}

// parseOK reports a query that parses (and, for the native engine, compiles).
func parseOK(query *gojq.Query) webapi.ValidateResponse {
	return webapi.ValidateResponse{OK: true, Formatted: query.String()}
}

// compile compiles a parsed query against the engine's vocabulary exactly as
// a run would, so the two cannot disagree about what resolves.
func (e *Engine) compile(query *gojq.Query, args []webapi.Arg) error {
	if len(e.aliasDefs) > 0 {
		query.FuncDefs = append(append([]*gojq.FuncDef{}, e.aliasDefs...), query.FuncDefs...)
	}

	options := append([]gojq.CompilerOption{}, e.runner.Options...)
	if names := variableNames(args); len(names) > 0 {
		options = append(options, gojq.WithVariables(names))
	}

	_, err := gojq.Compile(query, options...)
	return err
}

// variableNames renders the caller's named args the way gojq wants them, with
// the leading dollar it may or may not have been given.
func variableNames(args []webapi.Arg) []string {
	names := make([]string, 0, len(args))
	for _, arg := range args {
		name := strings.TrimSpace(arg.Name)
		if name == "" {
			continue
		}
		if !strings.HasPrefix(name, "$") {
			name = "$" + name
		}
		names = append(names, name)
	}
	return names
}

// parseFailure locates a parse error in the source. gojq reports a byte
// offset and the token it choked on; the editor wants a line, a column and a
// span it can highlight. It mirrors webapi's behaviour so both engines point
// at the same token.
func parseFailure(src string, err error) webapi.ValidateResponse {
	resp := webapi.ValidateResponse{Error: err.Error()}

	var perr *gojq.ParseError
	if !asParseError(err, &perr) {
		return resp
	}

	offset := perr.Offset
	if offset > len(src) {
		offset = len(src)
	}
	if offset < 0 {
		offset = 0
	}

	end := offset
	start := offset - len(perr.Token)
	if start < 0 {
		start = 0
	}
	if start == end {
		// An unexpected EOF names no token, so there is nothing to underline
		// at the offset itself. The last character before it is what the
		// reader has to look at - the bracket that was never closed.
		switch {
		case end < len(src):
			end = start + 1
		case start > 0:
			start--
		}
	}

	line, column := lineColumn(src, start)
	resp.Offset = offset
	resp.Token = perr.Token
	resp.Start = start
	resp.End = end
	resp.Line = line
	resp.Column = column
	return resp
}

func asParseError(err error, target **gojq.ParseError) bool {
	for err != nil {
		if perr, ok := err.(*gojq.ParseError); ok {
			*target = perr
			return true
		}
		unwrapper, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = unwrapper.Unwrap()
	}
	return false
}

// lineColumn converts a byte offset into a 1-based line and column.
func lineColumn(src string, offset int) (int, int) {
	if offset > len(src) {
		offset = len(src)
	}
	line, column := 1, 1
	for i := 0; i < offset; i++ {
		if src[i] == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}
	return line, column
}

// ---------------------------------------------------------------------------
// format / minify / inline
//
// These are registry-agnostic: they never evaluate, so the native page shares
// their behaviour with the WASM page exactly.

// Format pretty-prints a query onto multiple lines. The result parses back
// to the same program, so formatting can never change what a query does.
func Format(request string) string {
	var req webapi.FormatRequest
	if err := json.Unmarshal([]byte(request), &req); err != nil {
		return marshal(webapi.FormatResponse{Error: "malformed request: " + err.Error()})
	}
	if strings.TrimSpace(req.Query) == "" {
		return marshal(webapi.FormatResponse{Query: req.Query})
	}
	query, err := gojq.Parse(req.Query)
	if err != nil {
		return marshal(webapi.FormatResponse{Query: req.Query, Error: err.Error()})
	}
	return marshal(webapi.FormatResponse{Query: jqfmt.Format(query)})
}

// Minify renders a query on a single line: the canonical form, spacing
// normalised and whitespace stripped.
func Minify(request string) string {
	var req webapi.FormatRequest
	if err := json.Unmarshal([]byte(request), &req); err != nil {
		return marshal(webapi.FormatResponse{Error: "malformed request: " + err.Error()})
	}
	if strings.TrimSpace(req.Query) == "" {
		return marshal(webapi.FormatResponse{Query: req.Query})
	}
	query, err := gojq.Parse(req.Query)
	if err != nil {
		return marshal(webapi.FormatResponse{Query: req.Query, Error: err.Error()})
	}
	return marshal(webapi.FormatResponse{Query: jqfmt.Minify(query)})
}

// Inline replaces every call to a query-local definition with a copy of that
// definition's body, then lays the result out the way Format would.
func Inline(request string) string {
	var req webapi.InlineRequest
	if err := json.Unmarshal([]byte(request), &req); err != nil {
		return marshal(webapi.InlineResponse{Error: "malformed request: " + err.Error()})
	}
	if strings.TrimSpace(req.Query) == "" {
		return marshal(webapi.InlineResponse{Query: req.Query})
	}
	query, err := gojq.Parse(req.Query)
	if err != nil {
		return marshal(webapi.InlineResponse{Query: req.Query, Error: err.Error()})
	}
	result := jqinline.Inline(query)
	return marshal(webapi.InlineResponse{
		Query:    jqfmt.Format(result.Query),
		Expanded: result.Expanded,
		Kept:     result.Kept,
	})
}

// ---------------------------------------------------------------------------
// diagram

// Diagram renders a query's structure, coloured by the native vocabulary, so
// a cmdlet the server can run is drawn as one.
func (e *Engine) Diagram(request string) string {
	var req webapi.DiagramRequest
	if err := json.Unmarshal([]byte(request), &req); err != nil {
		return marshal(webapi.DiagramResponse{Error: "malformed request: " + err.Error()})
	}
	if strings.TrimSpace(req.Query) == "" {
		return marshal(webapi.DiagramResponse{Error: "query is empty"})
	}

	query, err := gojq.Parse(req.Query)
	if err != nil {
		return marshal(webapi.DiagramResponse{Error: err.Error()})
	}

	// The user's own query is drawn, not the alias-expanded one: the diagram
	// should show what was written.
	opts := graph.RenderOptions{
		Cmdlets:   e.cmdlets,
		Theme:     req.Theme,
		Layout:    req.Layout,
		Direction: req.Direction,
		Sketch:    req.Sketch,
	}

	resp := webapi.DiagramResponse{}
	if req.D2 {
		resp.Script = graph.RenderD2Opts(query, opts)
	}
	svg, err := graph.GenerateSVGOpts(query, opts)
	if err != nil {
		resp.Error = err.Error()
		return marshal(resp)
	}
	resp.SVG = svg
	return marshal(resp)
}

// ---------------------------------------------------------------------------
// catalog

// Catalog reports the full native vocabulary. Every command is runnable
// here, so Available is true throughout — read from the same discovery
// catalog get_command serves, which the native registry publishes.
func (e *Engine) Catalog(string) string {
	resp := webapi.CatalogResponse{
		Version:  webapi.Version,
		Cmdlets:  e.names,
		Builtins: jqBuiltins(),
		Examples: webapi.Examples(),
	}

	for _, cmd := range discovery.Catalog() {
		resp.Commands = append(resp.Commands, webapi.Command{
			Name:        cmd.Name,
			Aliases:     cmd.Aliases,
			MinArgs:     cmd.MinArgs,
			MaxArgs:     cmd.MaxArgs,
			Category:    cmd.Category,
			Description: cmd.Description,
			Examples:    cmd.Examples,
			Available:   cmd.Available,
		})
	}
	sort.Slice(resp.Commands, func(i, j int) bool { return resp.Commands[i].Name < resp.Commands[j].Name })

	for _, alias := range e.aliases {
		resp.Aliases = append(resp.Aliases, webapi.AliasInfo{Name: alias.Name, Target: alias.Target})
	}

	dark, light := graph.PaletteFor("dark"), graph.PaletteFor("light")
	for _, class := range graph.Classes() {
		resp.Classes = append(resp.Classes, webapi.ClassStyle{
			Name:        class.Name,
			Label:       class.Label,
			Description: class.Description,
			Dark:        dark[class.Name],
			Light:       light[class.Name],
		})
	}

	return marshal(resp)
}

// jqBuiltins lists jq's own functions, asked of gojq rather than kept in a
// list that would drift. It mirrors webapi's helper so both catalogs agree.
func jqBuiltins() []string {
	query, err := gojq.Parse("builtins")
	if err != nil {
		return nil
	}
	code, err := gojq.Compile(query)
	if err != nil {
		return nil
	}
	iter := code.Run(nil)
	v, ok := iter.Next()
	if !ok {
		return nil
	}
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	seen := make(map[string]bool, len(list))
	names := make([]string, 0, len(list))
	for _, entry := range list {
		s, ok := entry.(string)
		if !ok {
			continue
		}
		name, _, _ := strings.Cut(s, "/")
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// marshal renders a response, falling back to a hand-built error object so the
// page always receives JSON.
func marshal(v any) string {
	encoded, err := json.Marshal(v)
	if err != nil {
		return `{"error":"failed to encode response: ` + jsonEscape(err.Error()) + `"}`
	}
	return string(encoded)
}

func jsonEscape(s string) string {
	encoded, err := json.Marshal(s)
	if err != nil {
		return "encoding error"
	}
	return string(encoded[1 : len(encoded)-1])
}
