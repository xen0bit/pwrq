package ideengine

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/graph"
	"github.com/xen0bit/pwrq/pkg/udf"
)

// The browser hosts are tested through their JSON protocol in pkg/webapi and
// pkg/webnative. What is tested here is what Config changes, since that is
// what a new host - the terminal UI - chooses.

func webEngine(config Config) *Engine {
	config.Registry = udf.WebRegistry()
	return New(config)
}

func TestValidateCompilesOnlyWhenAsked(t *testing.T) {
	const query = `no_such_cmdlet(1)`

	if resp := webEngine(Config{}).Validate(ValidateRequest{Query: query}); !resp.OK {
		t.Errorf("a parse-only engine should accept %q, got %q", query, resp.Error)
	}

	resp := webEngine(Config{CompileOnValidate: true}).Validate(ValidateRequest{Query: query})
	if resp.OK || !strings.Contains(resp.Error, "no_such_cmdlet") {
		t.Errorf("a compiling engine should reject %q by name, got ok=%v error=%q", query, resp.OK, resp.Error)
	}
}

// TestValidateFormatsTheUsersQuery: compiling prepends the aliases, which
// must not leak into what the editor is shown.
func TestValidateFormatsTheUsersQuery(t *testing.T) {
	for _, config := range []Config{{}, {CompileOnValidate: true}} {
		for _, query := range []string{`.a | .b`, `$unbound`} {
			resp := New(Config{Registry: udf.DefaultRegistry(), CompileOnValidate: config.CompileOnValidate}).Validate(ValidateRequest{Query: query})
			if strings.Contains(resp.Formatted, "def ") {
				t.Errorf("compile=%v %q: formatted = %q", config.CompileOnValidate, query, resp.Formatted)
			}
		}
	}
}

// TestValidateCompilesWithTheArgs keeps validation from disagreeing with a
// run: a query reading $n compiles exactly when the run would bind $n.
func TestValidateCompilesWithTheArgs(t *testing.T) {
	e := webEngine(Config{CompileOnValidate: true})

	if resp := e.Validate(ValidateRequest{Query: `$n + 1`}); resp.OK {
		t.Error("$n is unbound, so the query should not compile")
	}
	if resp := e.Validate(ValidateRequest{Query: `$n + 1`, Args: []Arg{{Name: "n", Value: "1"}}}); !resp.OK {
		t.Errorf("$n is bound, so the query should compile, got %q", resp.Error)
	}
}

func TestValidateLocatesTheParseError(t *testing.T) {
	resp := webEngine(Config{}).Validate(ValidateRequest{Query: ".a |\n  .b ]"})
	if resp.OK {
		t.Fatal("the query does not parse")
	}
	if resp.Line != 2 || resp.Column != 6 || resp.Token != "]" {
		t.Errorf("error at %d:%d on %q, want 2:6 on %q", resp.Line, resp.Column, resp.Token, "]")
	}
}

// TestDiagramWithoutARendererIsTheScript is what lets a host without d2 draw
// a query at all: the script is the diagram, asked for or not.
func TestDiagramWithoutARendererIsTheScript(t *testing.T) {
	resp := webEngine(Config{}).Diagram(DiagramRequest{Query: `.a | sha256`})
	if resp.Error != "" {
		t.Fatalf("diagram failed: %s", resp.Error)
	}
	if resp.SVG != "" {
		t.Error("an engine with no renderer should not produce an image")
	}
	if !strings.Contains(resp.Script, "sha256") {
		t.Errorf("the script should draw the query, got:\n%s", resp.Script)
	}
}

func TestDiagramWithARendererSendsTheScriptOnRequest(t *testing.T) {
	var drew graph.RenderOptions
	e := webEngine(Config{RenderSVG: func(_ *gojq.Query, opts graph.RenderOptions) (string, error) {
		drew = opts
		return "<svg/>", nil
	}})

	resp := e.Diagram(DiagramRequest{Query: `.a | sha256`, Theme: "light"})
	if resp.SVG != "<svg/>" || resp.Script != "" {
		t.Errorf("got svg=%q script=%q, want the image alone", resp.SVG, resp.Script)
	}
	if drew.Theme != "light" || !drew.Cmdlets["sha256"] {
		t.Errorf("the renderer should get the request's theme and the engine's cmdlets, got %+v", drew)
	}

	if resp := e.Diagram(DiagramRequest{Query: `.a`, D2: true}); resp.Script == "" {
		t.Error("asking for the script should return it beside the image")
	}
}

func TestRunIsClampedToTheLimits(t *testing.T) {
	limits := DefaultLimits
	limits.DefaultResults, limits.MaxResults = 3, 5
	e := webEngine(Config{Limits: limits})

	resp := e.Run(context.Background(), RunRequest{Query: `range(10)`, NullInput: true})
	if resp.Count != 3 || !resp.Truncated {
		t.Errorf("default limit: count=%d truncated=%v, want 3 and truncated", resp.Count, resp.Truncated)
	}

	resp = e.Run(context.Background(), RunRequest{Query: `range(10)`, NullInput: true, Limit: 1000})
	if resp.Count != 5 {
		t.Errorf("a request cannot raise the limit past the ceiling: count=%d, want 5", resp.Count)
	}
}

// TestRunStopsWhenCancelled is what an interactive host relies on to replace
// a run that is no longer wanted: cancelling the context ends it now, not
// when its timeout would have.
func TestRunStopsWhenCancelled(t *testing.T) {
	e := webEngine(Config{})
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)

	started := time.Now()
	resp := e.Run(ctx, RunRequest{
		Query:     `repeat(1) | select(. == 0)`,
		NullInput: true,
		TimeoutMs: 60000,
	})
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("the run took %v after being cancelled at 50ms", elapsed)
	}
	if resp.Kind != "cancelled" {
		t.Errorf("a cancelled run should say so, got kind=%q error=%q", resp.Kind, resp.Error)
	}
}

// TestTheCoreDoesNotLinkD2 keeps the engine small enough for the everyday
// binary. Rendering an image is a host's choice, made by importing
// pkg/graph/graphsvg; the engine must not make it for them.
func TestTheCoreDoesNotLinkD2(t *testing.T) {
	if testing.Short() {
		t.Skip("lists the build graph")
	}
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Skipf("go list unavailable: %v", err)
	}
	for _, dep := range strings.Fields(string(out)) {
		if strings.HasPrefix(dep, "oss.terrastruct.com/d2") {
			t.Fatalf("ideengine depends on %s", dep)
		}
	}
}
