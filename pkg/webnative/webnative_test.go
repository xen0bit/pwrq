package webnative

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xen0bit/pwrq/pkg/webapi"
)

// call is how the native page talks to this package: a method name and a JSON
// request, decoded into whatever the response type is.
func call[T any](t *testing.T, e *Engine, method string, request any) T {
	t.Helper()
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("encoding request: %v", err)
	}
	raw := e.Call(context.Background(), method, string(encoded))

	var resp T
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("response is not JSON: %v\n%s", err, raw)
	}
	return resp
}

func doRun(t *testing.T, e *Engine, req webapi.RunRequest) webapi.RunResponse {
	t.Helper()
	return call[webapi.RunResponse](t, e, "run", req)
}

func TestRunProducesResults(t *testing.T) {
	e := New()
	resp := doRun(t, e, webapi.RunRequest{Query: ".items[] | .Name", Input: `{"items":[{"Name":"a"},{"Name":"b"}]}`})

	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if got, want := resp.Values, []string{`"a"`, `"b"`}; !equal(got, want) {
		t.Errorf("values = %q, want %q", got, want)
	}
	if resp.Count != 2 {
		t.Errorf("count = %d, want 2", resp.Count)
	}
}

// TestRunReachesTheFilesystem is the capability the WASM page cannot have: a
// native cmdlet reading a real directory.
func TestRunReachesTheFilesystem(t *testing.T) {
	e := New()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}

	resp := doRun(t, e, webapi.RunRequest{Query: `[get_childitem($dir)] | map(.Name)`, Args: []webapi.Arg{{Name: "dir", Value: `"` + dir + `"`}}, Compact: true})
	if resp.Error != "" {
		t.Fatalf("unexpected error (%s): %s", resp.Kind, resp.Error)
	}
	if len(resp.Values) != 1 || !strings.Contains(resp.Values[0], "hello.txt") {
		t.Errorf("values = %q, want the directory listing to name hello.txt", resp.Values)
	}
}

// TestRunReachesSubprocesses is the other half of native: sh runs.
func TestRunReachesSubprocesses(t *testing.T) {
	e := New()
	resp := doRun(t, e, webapi.RunRequest{Query: `sh("echo native-hi")`, Raw: true})
	if resp.Error != "" {
		t.Fatalf("unexpected error (%s): %s", resp.Kind, resp.Error)
	}
	if len(resp.Values) != 1 || !strings.Contains(resp.Values[0], "native-hi") {
		t.Errorf("values = %q, want sh output", resp.Values)
	}
}

// TestRunSeesTheEnvironment keeps the page honest in the other direction from
// the WASM build: a native tab has an environment, and env reports it.
func TestRunSeesTheEnvironment(t *testing.T) {
	e := New()
	t.Setenv("PWRQ_NATIVE_IDE_TEST", "present")
	resp := doRun(t, e, webapi.RunRequest{Query: `env.PWRQ_NATIVE_IDE_TEST`, Raw: true})
	if resp.Error != "" {
		t.Fatalf("unexpected error (%s): %s", resp.Kind, resp.Error)
	}
	if got, want := resp.Values, []string{"present"}; !equal(got, want) {
		t.Errorf("values = %q, want %q", got, want)
	}
}

func TestRunResolvesAliases(t *testing.T) {
	e := New()
	resp := doRun(t, e, webapi.RunRequest{Query: `[{Name: "a", Size: 1}] | ft(.)`, Raw: true})
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if len(resp.Values) != 1 || !strings.Contains(resp.Values[0], "Name") {
		t.Errorf("ft did not resolve to format_table: %q", resp.Values)
	}
}

func TestRunStopsUnboundedStreams(t *testing.T) {
	e := New()
	resp := doRun(t, e, webapi.RunRequest{Query: "repeat(1)", Limit: 50})

	if !resp.Truncated {
		t.Error("an infinite stream should be reported as truncated")
	}
	if resp.Count != 50 {
		t.Errorf("count = %d, want the limit of 50", resp.Count)
	}
	if resp.Kind != "limit" {
		t.Errorf("kind = %q, want limit", resp.Kind)
	}
}

func TestRunTimesOut(t *testing.T) {
	e := New()
	resp := doRun(t, e, webapi.RunRequest{Query: `last(range(1e9) | select(. < 0))`, TimeoutMs: 100})

	if resp.Kind != "timeout" {
		t.Fatalf("kind = %q (error %q), want timeout", resp.Kind, resp.Error)
	}
}

func TestRunReportsErrorsAfterOutput(t *testing.T) {
	e := New()
	resp := doRun(t, e, webapi.RunRequest{Query: `1, 2, error("boom")`, Compact: true})

	if resp.Count != 2 {
		t.Errorf("count = %d, want the two values that preceded the error", resp.Count)
	}
	if !strings.Contains(resp.Error, "boom") {
		t.Errorf("error = %q, want it to mention boom", resp.Error)
	}
	if resp.Kind != "runtime" {
		t.Errorf("kind = %q, want runtime", resp.Kind)
	}
}

// TestSessionsDoNotLeakBetweenRuns pins the per-call isolation: a variable
// set in one run is gone in the next.
func TestSessionsDoNotLeakBetweenRuns(t *testing.T) {
	e := New()
	first := doRun(t, e, webapi.RunRequest{Query: `set_variable("pwrq_native_probe"; 42)`, Compact: true})
	if first.Error != "" {
		t.Fatalf("setup run failed (%s): %s", first.Kind, first.Error)
	}
	second := doRun(t, e, webapi.RunRequest{Query: `get_variable("pwrq_native_probe")`, Compact: true})
	if second.Error == "" {
		t.Errorf("variable leaked between runs: %q", second.Values)
	}
}

func TestValidateAcceptsGoodQueries(t *testing.T) {
	e := New()
	ok := call[webapi.ValidateResponse](t, e, "validate", NativeValidateRequest{Query: `[get_childitem(".")] | length`})
	if !ok.OK {
		t.Errorf("a valid native query was rejected: %s", ok.Error)
	}

	empty := call[webapi.ValidateResponse](t, e, "validate", NativeValidateRequest{Query: "   "})
	if !empty.Empty || empty.OK {
		t.Errorf("an empty query should be reported as empty, got %+v", empty)
	}
}

// TestValidateCompilesAgainstTheNativeVocabulary is what the WASM page's
// parse-only check cannot do: unknown names and wrong arities fail here.
func TestValidateCompilesAgainstTheNativeVocabulary(t *testing.T) {
	e := New()
	for _, query := range []string{
		`nosuchcmdlet_zz9("x")`,
		`sha256("a"; "b"; "c"; "d"; "e"; "f")`,
		`get_childitem | nosuchcmdlet_zz9`,
	} {
		resp := call[webapi.ValidateResponse](t, e, "validate", NativeValidateRequest{Query: query})
		if resp.OK {
			t.Errorf("%q was accepted, want a compile error", query)
		}
		if resp.Error == "" {
			t.Errorf("%q produced no error text", query)
		}
	}
}

// TestValidateHonoursBoundVariables keeps the args editor honest: a query
// reading $name compiles when the name is supplied, and fails without it.
func TestValidateHonoursBoundVariables(t *testing.T) {
	e := New()
	withArgs := call[webapi.ValidateResponse](t, e, "validate",
		NativeValidateRequest{Query: `.[] | select(.n > $min)`, Args: []webapi.Arg{{Name: "min", Value: `3`}}})
	if !withArgs.OK {
		t.Errorf("a query with its variables bound was rejected: %s", withArgs.Error)
	}

	withoutArgs := call[webapi.ValidateResponse](t, e, "validate",
		NativeValidateRequest{Query: `.[] | select(.n > $min)`})
	if withoutArgs.OK {
		t.Error("a query reading an unbound variable was accepted")
	}
}

func TestValidateLocatesTheError(t *testing.T) {
	e := New()
	const query = ".a |\n. as |"
	resp := call[webapi.ValidateResponse](t, e, "validate", NativeValidateRequest{Query: query})

	if resp.OK {
		t.Fatal("a broken query was accepted")
	}
	if resp.Line != 2 {
		t.Errorf("line = %d, want 2 (the error is on the second line)", resp.Line)
	}
	if resp.Start >= resp.End {
		t.Errorf("span = [%d,%d), want a non-empty range to highlight", resp.Start, resp.End)
	}
	if resp.End > len(query) {
		t.Errorf("span end %d is past the end of the query", resp.End)
	}
}

func TestFormat(t *testing.T) {
	e := New()
	resp := call[webapi.FormatResponse](t, e, "format", webapi.FormatRequest{Query: ".a|select(.b>1)|{c:.d}"})
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if !strings.Contains(resp.Query, "\n| select(.b > 1)") {
		t.Errorf("formatted query is not spread across lines: %q", resp.Query)
	}
}

func TestMinify(t *testing.T) {
	e := New()
	resp := call[webapi.FormatResponse](t, e, "minify", webapi.FormatRequest{Query: ".a |\n select(.b)\n | {c: .d}"})
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if want := ".a | select(.b) | { c: .d }"; resp.Query != want {
		t.Errorf("minified query = %q, want %q", resp.Query, want)
	}
}

func TestInline(t *testing.T) {
	e := New()
	resp := call[webapi.InlineResponse](t, e, "inline", webapi.InlineRequest{Query: `def hot: select(.CPU > 50); [.[] | hot]`})
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if strings.Contains(resp.Query, "def ") {
		t.Errorf("the definition survived: %q", resp.Query)
	}
	if resp.Expanded != 1 {
		t.Errorf("expanded = %d, want 1 call site", resp.Expanded)
	}
}

// TestCatalogMarksEverythingAvailable is the native honesty property: the
// server can run the whole vocabulary, so nothing is marked unavailable.
func TestCatalogMarksEverythingAvailable(t *testing.T) {
	e := New()
	catalog := call[webapi.CatalogResponse](t, e, "catalog", struct{}{})

	if len(catalog.Commands) == 0 || len(catalog.Cmdlets) == 0 {
		t.Fatal("the catalog is empty")
	}

	cmdlets := make(map[string]bool, len(catalog.Cmdlets))
	for _, name := range catalog.Cmdlets {
		cmdlets[name] = true
	}
	for _, cmd := range catalog.Commands {
		if !cmd.Available {
			t.Errorf("the native catalog marks %q unavailable, but the server can run it", cmd.Name)
		}
		if !cmdlets[cmd.Name] {
			t.Errorf("the catalog documents %q without registering it", cmd.Name)
		}
	}

	// The cmdlets the browser build leaves out must be here and runnable.
	for _, name := range []string{"get_childitem", "get_process", "sh", "invoke_web_request"} {
		if !cmdlets[name] {
			t.Errorf("%q should be registered in the native engine", name)
		}
	}

	if len(catalog.Builtins) == 0 {
		t.Error("jq's own builtins should be listed for completion")
	}
	if len(catalog.Aliases) == 0 {
		t.Error("the aliases the page resolves should be listed")
	}
	if len(catalog.Classes) == 0 {
		t.Error("the diagram legend is empty")
	}
}

func TestDiagramColoursNativeCmdlets(t *testing.T) {
	e := New()
	resp := call[webapi.DiagramResponse](t, e, "diagram", webapi.DiagramRequest{
		Query: `get_childitem(".") | length`,
		D2:    true,
	})

	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if !strings.Contains(resp.SVG, "<svg") {
		t.Error("the response is not an SVG document")
	}
	// get_childitem is a cmdlet here, not an unknown name.
	if !strings.Contains(resp.Script, "class: cmdlet") {
		t.Error("no node was coloured as a cmdlet")
	}
}

func TestDiagramReportsBrokenQueries(t *testing.T) {
	e := New()
	resp := call[webapi.DiagramResponse](t, e, "diagram", webapi.DiagramRequest{Query: ".a | ("})
	if resp.Error == "" {
		t.Error("a broken query should not silently produce no diagram")
	}
}

func TestUnknownMethod(t *testing.T) {
	e := New()
	var resp errorResponse
	if err := json.Unmarshal([]byte(e.Call(context.Background(), "nope", "{}")), &resp); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if !strings.Contains(resp.Error, "nope") {
		t.Errorf("error = %q, want it to name the unknown method", resp.Error)
	}
}

// TestMalformedRequestsStayJSON matters because the page has no other channel:
// a reply it cannot parse is indistinguishable from a crash.
func TestMalformedRequestsStayJSON(t *testing.T) {
	e := New()
	for _, method := range Methods {
		var anything map[string]any
		raw := e.Call(context.Background(), method, "{not json")
		if err := json.Unmarshal([]byte(raw), &anything); err != nil {
			t.Errorf("%s returned something that is not JSON: %v\n%s", method, err, raw)
		}
	}
}

func TestHealth(t *testing.T) {
	e := New()
	var health HealthResponse
	if err := json.Unmarshal([]byte(e.Health()), &health); err != nil {
		t.Fatalf("health is not JSON: %v", err)
	}
	if health.Mode != "native" {
		t.Errorf("mode = %q, want native", health.Mode)
	}
	if health.Cmdlets == 0 {
		t.Error("the health report names no cmdlets")
	}
	if health.Cwd == "" {
		t.Error("the health report names no working directory")
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
