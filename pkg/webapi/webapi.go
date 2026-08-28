// Package webapi is the engine behind the browser IDE.
//
// Everything the page can ask for - validate this query, run it, draw it, tell
// me what I can call - is a function here that takes a JSON request and
// returns a JSON response. The WASM entry point in cmd/web does nothing but
// hand strings across the JavaScript boundary, which is what makes this
// testable: the IDE's behaviour is exercised by ordinary Go tests rather than
// by clicking around in a browser.
package webapi

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/queryrun"
	"github.com/xen0bit/pwrq/pkg/graph"
	"github.com/xen0bit/pwrq/pkg/jqfmt"
	"github.com/xen0bit/pwrq/pkg/jqinline"
	"github.com/xen0bit/pwrq/pkg/udf"
	"github.com/xen0bit/pwrq/pkg/udf/common"
	"github.com/xen0bit/pwrq/pkg/udf/discovery"
)

// Version is stamped at build time with the revision the page was built from,
// so a shared link's behaviour can be traced to a commit.
var Version = "dev"

// Call dispatches a named request. The page speaks one protocol - a method
// name and a JSON string - so adding a capability here needs no change to the
// worker or the WASM glue.
func Call(method, request string) string {
	switch method {
	case "validate":
		return Validate(request)
	case "run":
		return Run(request)
	case "diagram":
		return Diagram(request)
	case "format":
		return Format(request)
	case "minify":
		return Minify(request)
	case "inline":
		return Inline(request)
	case "catalog":
		return Catalog(request)
	default:
		return marshal(errorResponse{Error: fmt.Sprintf("unknown method %q", method)})
	}
}

// errorResponse is the shape every response degrades to when a request cannot
// even be read, so the page never has to handle a non-JSON reply.
type errorResponse struct {
	Error string `json:"error"`
}

// engine holds the vocabulary the page evaluates against. Building it is not
// free - signature discovery compiles a program - and the page calls in on
// every keystroke, so it is built once.
type engine struct {
	registry  *udf.Registry
	options   []gojq.CompilerOption
	aliases   []udf.Alias
	aliasDefs []*gojq.FuncDef
	cmdlets   map[string]bool
	names     []string

	// runner evaluates against the vocabulary above. It is the same runner the
	// MCP server uses, so the page and an agent get the same jq semantics out
	// of the same query.
	runner *queryrun.Runner
}

var (
	engineOnce sync.Once
	eng        *engine
)

func getEngine() *engine {
	engineOnce.Do(func() {
		reg := udf.WebRegistry()
		e := &engine{registry: reg, options: reg.Options()}

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

		// Script blocks handed to cmdlets compile against the same vocabulary
		// as the surrounding query.
		common.SetScriptBlockOptions(e.options)

		e.runner = &queryrun.Runner{Options: e.options, AliasDefs: e.aliasDefs}
		eng = e
	})
	return eng
}

// ---------------------------------------------------------------------------
// validate

// ValidateRequest asks whether a query parses.
type ValidateRequest struct {
	Query string `json:"query"`
}

// ValidateResponse reports the answer, and where it went wrong when it did.
// The position is what lets the editor underline the offending token instead
// of colouring the whole box red.
type ValidateResponse struct {
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
	Offset    int    `json:"offset,omitempty"`
	Line      int    `json:"line,omitempty"`
	Column    int    `json:"column,omitempty"`
	Token     string `json:"token,omitempty"`
	Start     int    `json:"start,omitempty"`
	End       int    `json:"end,omitempty"`
	Empty     bool   `json:"empty,omitempty"`
	Formatted string `json:"formatted,omitempty"`
}

// Validate parses a query and reports what it found.
func Validate(request string) string {
	var req ValidateRequest
	if err := json.Unmarshal([]byte(request), &req); err != nil {
		return marshal(ValidateResponse{Error: "malformed request: " + err.Error()})
	}

	if strings.TrimSpace(req.Query) == "" {
		return marshal(ValidateResponse{Empty: true})
	}

	query, err := gojq.Parse(req.Query)
	if err != nil {
		return marshal(parseFailure(req.Query, err))
	}
	return marshal(ValidateResponse{OK: true, Formatted: query.String()})
}

// parseFailure locates a parse error in the source. gojq reports a byte
// offset and the token it choked on; the editor wants a line, a column and a
// span it can highlight.
func parseFailure(src string, err error) ValidateResponse {
	resp := ValidateResponse{Error: err.Error()}

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
// format

// FormatRequest asks for a query to be tidied.
type FormatRequest struct {
	Query string `json:"query"`
}

// FormatResponse carries the normalised query.
type FormatResponse struct {
	Query string `json:"query"`
	Error string `json:"error,omitempty"`
}

// Format pretty-prints a query onto multiple lines, breaking top-level
// pipelines and folding long objects, arrays and conditionals under
// indentation. The result parses back to the same program, so formatting can
// never change what a query does.
func Format(request string) string {
	var req FormatRequest
	if err := json.Unmarshal([]byte(request), &req); err != nil {
		return marshal(FormatResponse{Error: "malformed request: " + err.Error()})
	}
	if strings.TrimSpace(req.Query) == "" {
		return marshal(FormatResponse{Query: req.Query})
	}
	query, err := gojq.Parse(req.Query)
	if err != nil {
		return marshal(FormatResponse{Query: req.Query, Error: err.Error()})
	}
	return marshal(FormatResponse{Query: jqfmt.Format(query)})
}

// Minify renders a query on a single line: the canonical form, spacing
// normalised and whitespace stripped. It is Format's inverse in spirit - one
// spreads the query for reading, the other compacts it for storing or
// sharing - and like Format it never changes what the query means.
func Minify(request string) string {
	var req FormatRequest
	if err := json.Unmarshal([]byte(request), &req); err != nil {
		return marshal(FormatResponse{Error: "malformed request: " + err.Error()})
	}
	if strings.TrimSpace(req.Query) == "" {
		return marshal(FormatResponse{Query: req.Query})
	}
	query, err := gojq.Parse(req.Query)
	if err != nil {
		return marshal(FormatResponse{Query: req.Query, Error: err.Error()})
	}
	return marshal(FormatResponse{Query: jqfmt.Minify(query)})
}

// ---------------------------------------------------------------------------
// inline

// InlineRequest asks for a query's definitions to be expanded where they are
// called.
type InlineRequest struct {
	Query string `json:"query"`
}

// InlineResponse carries the expanded query, how many calls it replaced, and
// what it had to leave alone. Kept names each definition that is still there
// and why, so the page can say so rather than leave the user to wonder.
type InlineResponse struct {
	Query    string   `json:"query"`
	Expanded int      `json:"expanded"`
	Kept     []string `json:"kept,omitempty"`
	Error    string   `json:"error,omitempty"`
}

// Inline replaces every call to a definition the query makes with a copy of
// that definition's body, so a pipeline can be read without jumping back to
// the top, and then lays the result out the way Format would. It is the third
// thing a query box wants beside Format and Minify, and like them it does not
// change what the query means: a body is only moved to a call site where every
// name it reads still means the same thing, and a body that binds a name its
// arguments use is renamed apart first.
//
// A definition that calls itself cannot be unfolded into a finite query, so it
// stays a definition and Kept says so, as does one whose expansion would make
// the query too large to work in.
func Inline(request string) string {
	var req InlineRequest
	if err := json.Unmarshal([]byte(request), &req); err != nil {
		return marshal(InlineResponse{Error: "malformed request: " + err.Error()})
	}
	if strings.TrimSpace(req.Query) == "" {
		return marshal(InlineResponse{Query: req.Query})
	}
	query, err := gojq.Parse(req.Query)
	if err != nil {
		return marshal(InlineResponse{Query: req.Query, Error: err.Error()})
	}
	result := jqinline.Inline(query)
	return marshal(InlineResponse{
		Query:    jqfmt.Format(result.Query),
		Expanded: result.Expanded,
		Kept:     result.Kept,
	})
}

// ---------------------------------------------------------------------------
// diagram

// DiagramRequest asks for a query's flow diagram.
type DiagramRequest struct {
	Query     string `json:"query"`
	Theme     string `json:"theme"`
	Layout    string `json:"layout"`
	Direction string `json:"direction"`
	Sketch    bool   `json:"sketch"`
	// D2 asks for the script alongside the picture, which is what makes the
	// diagram editable elsewhere rather than a dead end.
	D2 bool `json:"d2"`
}

// DiagramResponse carries the rendered diagram.
type DiagramResponse struct {
	SVG    string `json:"svg,omitempty"`
	Script string `json:"script,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Diagram renders a query's structure.
func Diagram(request string) string {
	var req DiagramRequest
	if err := json.Unmarshal([]byte(request), &req); err != nil {
		return marshal(DiagramResponse{Error: "malformed request: " + err.Error()})
	}
	if strings.TrimSpace(req.Query) == "" {
		return marshal(DiagramResponse{Error: "query is empty"})
	}

	query, err := gojq.Parse(req.Query)
	if err != nil {
		return marshal(DiagramResponse{Error: err.Error()})
	}

	// The user's own query is drawn, not the alias-expanded one: the diagram
	// should show what was written.
	opts := graph.RenderOptions{
		Cmdlets:   getEngine().cmdlets,
		Theme:     req.Theme,
		Layout:    req.Layout,
		Direction: req.Direction,
		Sketch:    req.Sketch,
	}

	resp := DiagramResponse{}
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

// Command is one callable name, as the page's help and completion show it.
type Command struct {
	Name        string   `json:"name"`
	Aliases     []string `json:"aliases,omitempty"`
	MinArgs     int      `json:"minArgs"`
	MaxArgs     int      `json:"maxArgs"`
	Category    string   `json:"category"`
	Description string   `json:"description"`
	Examples    []string `json:"examples,omitempty"`
	// Available reports whether this registry can actually evaluate the name.
	// The catalog lists the CLI's full vocabulary, so the cmdlets that need a
	// filesystem, process table or service manager are present but marked
	// unavailable rather than hidden.
	Available bool `json:"available"`
}

// AliasInfo is a short name and what it stands for.
type AliasInfo struct {
	Name   string `json:"name"`
	Target string `json:"target"`
}

// ClassStyle is one node class in the diagram legend, in both themes.
type ClassStyle struct {
	Name        string           `json:"name"`
	Label       string           `json:"label"`
	Description string           `json:"description"`
	Dark        graph.ClassStyle `json:"dark"`
	Light       graph.ClassStyle `json:"light"`
}

// CatalogResponse is everything the page needs to describe its own vocabulary:
// completion, help, syntax highlighting and the diagram legend all read it.
type CatalogResponse struct {
	Version  string       `json:"version"`
	Commands []Command    `json:"commands"`
	Aliases  []AliasInfo  `json:"aliases"`
	Cmdlets  []string     `json:"cmdlets"`
	Builtins []string     `json:"builtins"`
	Classes  []ClassStyle `json:"classes"`
	Examples []Example    `json:"examples"`
}

// Catalog reports what can be called here.
//
// It is derived from the registry the page actually evaluates against, not
// from a hand-kept list. Commands the browser cannot run - those that need a
// filesystem, process table or service manager - are still listed, because the
// Catalog tab should show the CLI's whole vocabulary, but each one is marked
// unavailable so completion never offers a function the page would fail to run.
func Catalog(string) string {
	e := getEngine()

	resp := CatalogResponse{
		Version:  Version,
		Cmdlets:  e.names,
		Builtins: jqBuiltins(),
		Examples: Examples(),
	}

	for _, cmd := range discovery.Catalog() {
		resp.Commands = append(resp.Commands, Command{
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
		resp.Aliases = append(resp.Aliases, AliasInfo{Name: alias.Name, Target: alias.Target})
	}

	dark, light := graph.PaletteFor("dark"), graph.PaletteFor("light")
	for _, class := range graph.Classes() {
		resp.Classes = append(resp.Classes, ClassStyle{
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
// list that would drift. The names carry no arity: completion offers a name,
// and jq dispatches on arity itself.
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
