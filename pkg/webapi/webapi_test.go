package webapi

import (
	"encoding/json"
	"strings"
	"testing"
)

// call is how the page talks to this package: a method name and a JSON
// request, decoded into whatever the response type is.
func call[T any](t *testing.T, method string, request any) T {
	t.Helper()
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("encoding request: %v", err)
	}
	raw := Call(method, string(encoded))

	var resp T
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("response is not JSON: %v\n%s", err, raw)
	}
	return resp
}

func doRun(t *testing.T, req RunRequest) RunResponse {
	t.Helper()
	return call[RunResponse](t, "run", req)
}

func TestRunProducesResults(t *testing.T) {
	resp := doRun(t, RunRequest{Query: ".items[] | .Name", Input: `{"items":[{"Name":"a"},{"Name":"b"}]}`})

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

// TestRunUsesTheCmdletVocabulary is the point of the page: a query that is not
// jq has to work here exactly as it does at the command line.
func TestRunUsesTheCmdletVocabulary(t *testing.T) {
	resp := doRun(t, RunRequest{Query: `"hello" | sha256`, Compact: true})
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	const want = `"2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"`
	if len(resp.Values) != 1 || resp.Values[0] != want {
		t.Errorf("values = %q, want [%s]", resp.Values, want)
	}
}

// TestRunResolvesAliases guards the one thing that cannot work by accident:
// gojq binds names at compile time, so an alias only exists if the definition
// was prepended to the query before compiling.
func TestRunResolvesAliases(t *testing.T) {
	resp := doRun(t, RunRequest{
		Query: `[{Name: "a", Size: 1}] | ft(.)`,
		Raw:   true,
	})
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if len(resp.Values) != 1 || !strings.Contains(resp.Values[0], "Name") {
		t.Errorf("ft did not resolve to format_table: %q", resp.Values)
	}
}

// TestRunStopsUnboundedStreams is why the page can be handed a stranger's
// link: an infinite query has to end in a message, not a dead tab.
func TestRunStopsUnboundedStreams(t *testing.T) {
	resp := doRun(t, RunRequest{Query: "repeat(1)", Limit: 50})

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

// TestRunTimesOut covers the other kind of runaway: one that produces nothing
// at all while it spins. It is the case a result limit cannot catch, and the
// case a timer cannot catch either under GOOS=js, which is why the deadline
// reads the clock rather than waiting on one.
func TestRunTimesOut(t *testing.T) {
	resp := doRun(t, RunRequest{
		Query:     `last(range(1e9) | select(. < 0))`,
		TimeoutMs: 100,
	})

	if resp.Kind != "timeout" {
		t.Fatalf("kind = %q (error %q), want timeout", resp.Kind, resp.Error)
	}
	if resp.ElapsedMs > 5000 {
		t.Errorf("the run took %.0fms; the deadline did not stop it promptly", resp.ElapsedMs)
	}
}

func TestRunReportsErrorsAfterOutput(t *testing.T) {
	resp := doRun(t, RunRequest{Query: `1, 2, error("boom")`, Compact: true})

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

func TestRunHaltIsNotAFailure(t *testing.T) {
	resp := doRun(t, RunRequest{Query: `"done" | halt_error`})
	if !resp.Halted {
		t.Error("halt_error should be reported as a halt")
	}
	if resp.Error != "done" {
		t.Errorf("error = %q, want the halt's own message", resp.Error)
	}
}

func TestRunReadsAStreamOfInputs(t *testing.T) {
	resp := doRun(t, RunRequest{Query: ".a", Input: `{"a":1} {"a":2} {"a":3}`, Compact: true})

	if resp.InputCount != 3 {
		t.Errorf("inputCount = %d, want 3", resp.InputCount)
	}
	if got, want := resp.Values, []string{"1", "2", "3"}; !equal(got, want) {
		t.Errorf("values = %q, want %q", got, want)
	}
}

func TestRunSlurp(t *testing.T) {
	resp := doRun(t, RunRequest{Query: "length", Input: `1 2 3`, Slurp: true, Compact: true})
	if got, want := resp.Values, []string{"3"}; !equal(got, want) {
		t.Errorf("values = %q, want %q", got, want)
	}
}

func TestRunNullInputStillSeesInputs(t *testing.T) {
	resp := doRun(t, RunRequest{Query: "[inputs] | add", Input: `1 2 3`, NullInput: true, Compact: true})
	if got, want := resp.Values, []string{"6"}; !equal(got, want) {
		t.Errorf("values = %q, want %q; -n should not hide the input from `inputs`", got, want)
	}
}

func TestRunRawOutput(t *testing.T) {
	resp := doRun(t, RunRequest{Query: `"a b"`, Raw: true})
	if got, want := resp.Values, []string{"a b"}; !equal(got, want) {
		t.Errorf("values = %q, want %q", got, want)
	}

	// A non-string is still JSON: there is no other honest rendering of it.
	resp = doRun(t, RunRequest{Query: `{a: 1}`, Raw: true, Compact: true})
	if got, want := resp.Values, []string{`{"a":1}`}; !equal(got, want) {
		t.Errorf("values = %q, want %q", got, want)
	}
}

func TestRunPrettyPrints(t *testing.T) {
	resp := doRun(t, RunRequest{Query: `{a: [1, {}], b: "x"}`, Indent: 2})
	want := "{\n  \"a\": [\n    1,\n    {}\n  ],\n  \"b\": \"x\"\n}"
	if len(resp.Values) != 1 || resp.Values[0] != want {
		t.Errorf("value =\n%s\nwant\n%s", strings.Join(resp.Values, "\n"), want)
	}
}

// TestRunPreservesNumbers is the property the CLI decodes with UseNumber for:
// a big integer that round-trips through float64 comes out wrong.
func TestRunPreservesNumbers(t *testing.T) {
	const big = "10000000000000000000000000001"
	resp := doRun(t, RunRequest{Query: ".n", Input: `{"n":` + big + `}`, Compact: true})
	if len(resp.Values) != 1 || resp.Values[0] != big {
		t.Errorf("values = %q, want [%s]", resp.Values, big)
	}
}

func TestRunBindsArguments(t *testing.T) {
	resp := doRun(t, RunRequest{
		Query:   `.[] | select(.n > $min.value)`,
		Input:   `[{"n":1},{"n":5}]`,
		Args:    []Arg{{Name: "min", Value: `{"value": 3}`}},
		Compact: true,
	})
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if got, want := resp.Values, []string{`{"n":5}`}; !equal(got, want) {
		t.Errorf("values = %q, want %q", got, want)
	}
}

func TestRunRejectsBadArguments(t *testing.T) {
	resp := doRun(t, RunRequest{Query: ".", Args: []Arg{{Name: "x", Value: "{not json"}}})
	if resp.Kind != "args" {
		t.Errorf("kind = %q, want args (error %q)", resp.Kind, resp.Error)
	}
}

// TestRunEnvironmentIsEmpty keeps the page honest: a browser tab has no
// environment, and reporting the WASM runtime's would be an invention.
func TestRunEnvironmentIsEmpty(t *testing.T) {
	resp := doRun(t, RunRequest{Query: "env | length", Compact: true})
	if got, want := resp.Values, []string{"0"}; !equal(got, want) {
		t.Errorf("values = %q, want %q", got, want)
	}
}

func TestRunReportsBadInput(t *testing.T) {
	resp := doRun(t, RunRequest{Query: ".", Input: "{not json"})
	if resp.Kind != "input" {
		t.Errorf("kind = %q, want input (error %q)", resp.Kind, resp.Error)
	}
}

func TestValidate(t *testing.T) {
	ok := call[ValidateResponse](t, "validate", ValidateRequest{Query: ".a | select(.b)"})
	if !ok.OK {
		t.Errorf("a valid query was rejected: %s", ok.Error)
	}

	empty := call[ValidateResponse](t, "validate", ValidateRequest{Query: "   "})
	if !empty.Empty || empty.OK {
		t.Errorf("an empty query should be reported as empty, got %+v", empty)
	}
}

// TestValidateLocatesTheError is what lets the editor underline a token rather
// than colour the whole box red.
func TestValidateLocatesTheError(t *testing.T) {
	const query = ".a |\n. as |"
	resp := call[ValidateResponse](t, "validate", ValidateRequest{Query: query})

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
	resp := call[FormatResponse](t, "format", FormatRequest{Query: ".a|select(.b>1)|{c:.d}"})
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if !strings.Contains(resp.Query, " | ") {
		t.Errorf("formatted query is not spaced out: %q", resp.Query)
	}

	// Formatting has to be meaning-preserving, so the result must parse back
	// to the same text.
	again := call[FormatResponse](t, "format", FormatRequest{Query: resp.Query})
	if again.Query != resp.Query {
		t.Errorf("formatting is not idempotent:\n%q\n%q", resp.Query, again.Query)
	}
}

func TestFormatLeavesBrokenQueriesAlone(t *testing.T) {
	resp := call[FormatResponse](t, "format", FormatRequest{Query: ".a | ("})
	if resp.Error == "" {
		t.Error("a broken query should report why it was not formatted")
	}
	if resp.Query != ".a | (" {
		t.Errorf("query = %q, want the original text back", resp.Query)
	}
}

// TestCatalogDescribesOnlyWhatRuns is the honesty property the web registry
// exists for: every name offered as runnable has to be a name the page can
// evaluate, and every name that cannot run must be marked, not hidden.
func TestCatalogDescribesOnlyWhatRuns(t *testing.T) {
	catalog := call[CatalogResponse](t, "catalog", struct{}{})

	if len(catalog.Commands) == 0 || len(catalog.Cmdlets) == 0 {
		t.Fatal("the catalog is empty")
	}

	cmdlets := make(map[string]bool, len(catalog.Cmdlets))
	for _, name := range catalog.Cmdlets {
		cmdlets[name] = true
	}
	commands := make(map[string]Command, len(catalog.Commands))
	for _, cmd := range catalog.Commands {
		if _, dup := commands[cmd.Name]; dup {
			t.Errorf("the catalog lists %q twice", cmd.Name)
		}
		commands[cmd.Name] = cmd
		if cmd.Available != cmdlets[cmd.Name] {
			t.Errorf("the catalog marks %q available=%v but the registry has it=%v",
				cmd.Name, cmd.Available, cmdlets[cmd.Name])
		}
	}

	// The cmdlets that need a filesystem or a process table cannot work in a
	// tab, so the page must not run them - but they must still be documented,
	// so a reader can see the CLI's whole vocabulary.
	for _, name := range []string{"get_childitem", "get_process", "sh", "invoke_web_request"} {
		cmd, ok := commands[name]
		if !ok {
			t.Errorf("%q should be documented even though it cannot run in a browser", name)
			continue
		}
		if cmd.Available {
			t.Errorf("%q cannot work in a browser and must be marked unavailable", name)
		}
	}

	for _, present := range []string{"sha256", "base64_encode", "format_table", "select_object"} {
		if !cmdlets[present] {
			t.Errorf("%q is a pure transform and should be available", present)
		}
	}
}

func TestCatalogCarriesTheLegend(t *testing.T) {
	catalog := call[CatalogResponse](t, "catalog", struct{}{})

	if len(catalog.Classes) == 0 {
		t.Fatal("the diagram legend is empty")
	}
	for _, class := range catalog.Classes {
		if class.Label == "" || class.Description == "" {
			t.Errorf("class %q has nothing to show in a legend", class.Name)
		}
		if class.Dark.Fill == "" || class.Light.Fill == "" {
			t.Errorf("class %q is missing a colour in one of the themes", class.Name)
		}
	}

	if len(catalog.Builtins) == 0 {
		t.Error("jq's own builtins should be listed for completion")
	}
	if len(catalog.Aliases) == 0 {
		t.Error("the aliases the page resolves should be listed")
	}
}

func TestDiagramColoursByKind(t *testing.T) {
	resp := call[DiagramResponse](t, "diagram", DiagramRequest{
		Query: `.items[] | select(.n > 1) | {name: .Name, hash: (.Name | sha256)}`,
		D2:    true,
	})

	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if !strings.Contains(resp.SVG, "<svg") {
		t.Error("the response is not an SVG document")
	}

	// sha256 is a cmdlet and select is jq's own; the diagram has to tell them
	// apart, which is the whole point of colouring it.
	if !strings.Contains(resp.Script, "class: cmdlet") {
		t.Error("no node was coloured as a cmdlet")
	}
	if !strings.Contains(resp.Script, "class: builtin") {
		t.Error("no node was coloured as a builtin")
	}
	if !strings.Contains(resp.Script, "class: construct") {
		t.Error("the constructed object was not coloured as one")
	}
}

func TestDiagramThemes(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		resp := call[DiagramResponse](t, "diagram", DiagramRequest{Query: ".a | .b", Theme: theme, D2: true})
		if resp.Error != "" {
			t.Fatalf("%s: %s", theme, resp.Error)
		}
		if !strings.Contains(resp.Script, "classes: {") {
			t.Errorf("%s: the script declares no classes", theme)
		}
	}
}

func TestDiagramReportsBrokenQueries(t *testing.T) {
	resp := call[DiagramResponse](t, "diagram", DiagramRequest{Query: ".a | ("})
	if resp.Error == "" {
		t.Error("a broken query should not silently produce no diagram")
	}
}

func TestUnknownMethod(t *testing.T) {
	var resp errorResponse
	if err := json.Unmarshal([]byte(Call("nope", "{}")), &resp); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if !strings.Contains(resp.Error, "nope") {
		t.Errorf("error = %q, want it to name the unknown method", resp.Error)
	}
}

// TestMalformedRequestsStayJSON matters because the page has no other channel:
// a reply it cannot parse is indistinguishable from a crash.
func TestMalformedRequestsStayJSON(t *testing.T) {
	for _, method := range []string{"validate", "run", "diagram", "format", "catalog"} {
		var anything map[string]any
		raw := Call(method, "{not json")
		if err := json.Unmarshal([]byte(raw), &anything); err != nil {
			t.Errorf("%s returned something that is not JSON: %v\n%s", method, err, raw)
		}
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

// TestExamplesAllRun is what lets the gallery be trusted. Every example is
// evaluated against the same registry the page uses, so an entry cannot rot
// into a query that no longer parses, names a cmdlet the browser build does
// not have, or quietly produces nothing.
func TestExamplesAllRun(t *testing.T) {
	examples := Examples()
	if len(examples) == 0 {
		t.Fatal("there are no examples")
	}

	for _, example := range examples {
		t.Run(example.Title, func(t *testing.T) {
			if example.Title == "" || example.Description == "" || example.Category == "" {
				t.Error("an example needs a title, a description and a category to be shown")
			}

			resp := doRun(t, RunRequest{Query: example.Query, Input: example.Input, Args: example.Args})
			if resp.Error != "" {
				t.Fatalf("%s\n\nfailed (%s): %s", example.Query, resp.Kind, resp.Error)
			}
			if resp.Count == 0 {
				t.Errorf("%s\n\nproduced no output, which teaches nothing", example.Query)
			}
		})
	}
}

// TestExamplesDrawToo checks the other half of what an example demonstrates:
// the diagram beside it.
func TestExamplesDrawToo(t *testing.T) {
	for _, example := range Examples() {
		t.Run(example.Title, func(t *testing.T) {
			resp := call[DiagramResponse](t, "diagram", DiagramRequest{Query: example.Query})
			if resp.Error != "" {
				t.Errorf("%s\n\ndid not draw: %s", example.Query, resp.Error)
			}
		})
	}
}

// TestExamplesCoverTheGallery pins the gallery's size and sanity. The page's
// Examples tab is the visitor's first map of what pwrq can do, so there has to
// be enough of them to be useful, each with a distinct title, and they have to
// spread across more than one category rather than stack in one corner.
func TestExamplesCoverTheGallery(t *testing.T) {
	examples := Examples()
	if len(examples) < 100 {
		t.Errorf("the gallery has %d examples; it should hold at least 100", len(examples))
	}

	seen := make(map[string]bool, len(examples))
	categories := make(map[string]bool, len(examples))
	for _, example := range examples {
		if seen[example.Title] {
			t.Errorf("two examples share the title %q; the palette cannot tell them apart", example.Title)
		}
		seen[example.Title] = true
		categories[example.Category] = true
	}
	if len(categories) < 5 {
		t.Errorf("the gallery only spans %d categories; a visitor should see the vocabulary is broad", len(categories))
	}
}

// TestValidateUnderlinesUnclosedBrackets covers the commonest way a query is
// broken mid-edit. gojq reports an unexpected EOF, which names no token at
// all; the editor still needs somewhere to point.
func TestValidateUnderlinesUnclosedBrackets(t *testing.T) {
	const query = ".a | ("
	resp := call[ValidateResponse](t, "validate", ValidateRequest{Query: query})

	if resp.OK {
		t.Fatal("an unclosed bracket was accepted")
	}
	if resp.Start >= resp.End {
		t.Fatalf("span = [%d,%d), want something to underline", resp.Start, resp.End)
	}
	if got := query[resp.Start:resp.End]; got != "(" {
		t.Errorf("underlined %q, want the bracket that was never closed", got)
	}
}

// TestDiagramColoursReachTheSVG is the check the script-level assertion cannot
// make: D2 has to actually apply the class, or the diagram is monochrome no
// matter what the script says.
func TestDiagramColoursReachTheSVG(t *testing.T) {
	resp := call[DiagramResponse](t, "diagram", DiagramRequest{
		Query: `.a | sha256`,
		Theme: "light",
	})
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}

	cmdlet := graphPalette(t, "light")
	if !strings.Contains(strings.ToLower(resp.SVG), strings.ToLower(cmdlet)) {
		t.Errorf("the cmdlet colour %s never reaches the rendered SVG", cmdlet)
	}
}

func graphPalette(t *testing.T, theme string) string {
	t.Helper()
	catalog := call[CatalogResponse](t, "catalog", struct{}{})
	for _, class := range catalog.Classes {
		if class.Name == "cmdlet" {
			if theme == "light" {
				return class.Light.Fill
			}
			return class.Dark.Fill
		}
	}
	t.Fatal("the catalog has no cmdlet class")
	return ""
}
