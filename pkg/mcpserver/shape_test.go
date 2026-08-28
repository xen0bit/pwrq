package mcpserver

import (
	"strings"
	"testing"
)

// These tests pin what a model is told about a cmdlet's output.
//
// The gap they close was not a missing feature but a dropped one: pwrq knew
// whether get_childitem streams and what it emits, get_help printed both, and
// list_functions was built from the raw metadata table instead of the catalogue
// discovery assembles - so over MCP a model saw a name, an arity and a
// sentence, and had to run a probe query to learn the rest.

func findFunction(t *testing.T, res listFunctionsResult, name string) functionInfo {
	t.Helper()
	for _, fn := range res.Functions {
		if fn.Name == name {
			return fn
		}
	}
	t.Fatalf("%s is not in the catalogue", name)
	return functionInfo{}
}

func listFor(t *testing.T, filter string) (listFunctionsResult, string) {
	t.Helper()
	cs := newTestClient(t, NewServer("test"))
	res := callTool(t, cs, "list_functions", listFunctionsArgs{Filter: filter})
	if res.IsError {
		t.Fatalf("list_functions(%q): %s", filter, res.Content)
	}
	var out listFunctionsResult
	decodeStructured(t, res, &out)
	return out, contentText(res)
}

func TestCatalogueCarriesCardinalityAndShape(t *testing.T) {
	out, text := listFor(t, "get_childitem")
	fn := findFunction(t, out, "get_childitem")

	if !fn.Streaming {
		t.Error("get_childitem is not reported as streaming, so a model cannot know to collect it with [...]")
	}
	if fn.TypeName != "Pwrq.FileSystem.File" {
		t.Errorf("typeName = %q, want Pwrq.FileSystem.File", fn.TypeName)
	}
	for _, want := range []string{"Name", "Length", "FullName"} {
		if !strings.Contains(fn.Shape, want) {
			t.Errorf("shape does not name the %s property: %q", want, fn.Shape)
		}
	}
	if len(fn.Aliases) == 0 {
		t.Error("aliases are missing, so gci and dir are undiscoverable over MCP")
	}

	// A client is only obliged to show the model the content blocks, and
	// several drop structuredContent entirely, so the same facts have to
	// survive into the text.
	for _, want := range []string{"streams", "Pwrq.FileSystem.File", "Length"} {
		if !strings.Contains(text, want) {
			t.Errorf("the text block does not mention %q:\n%s", want, text)
		}
	}
}

// TestCatalogueDescribesDerivedShapesAsRules checks that a cmdlet whose keys
// come from the caller's data says so, rather than being given a property list
// that would be true only for the data in its example.
func TestCatalogueDescribesDerivedShapesAsRules(t *testing.T) {
	out, _ := listFor(t, "flatten_keys")
	fn := findFunction(t, out, "flatten_keys")

	if fn.TypeName != "" {
		t.Errorf("a Derived shape claimed the type name %q", fn.TypeName)
	}
	if !strings.Contains(fn.Shape, "keys from the input") {
		t.Errorf("shape does not say the keys come from the input: %q", fn.Shape)
	}
}

// TestCatalogueLeavesTransformsUndescribed pins the decision not to invent a
// shape for the ~300 cmdlets that return a string or a number. Silence is the
// correct answer for them, and 300 fabricated property lists would be the drift
// this whole mechanism exists to prevent.
func TestCatalogueLeavesTransformsUndescribed(t *testing.T) {
	out, _ := listFor(t, "sha256")
	fn := findFunction(t, out, "sha256")

	if fn.Shape != "" || fn.TypeName != "" {
		t.Errorf("sha256 returns a string but claims shape %q / type %q", fn.Shape, fn.TypeName)
	}
	if fn.Streaming {
		t.Error("sha256 is reported as streaming")
	}
}

func TestCatalogueReportsWhereInputComesFrom(t *testing.T) {
	out, _ := listFor(t, "to_timezone")
	fn := findFunction(t, out, "to_timezone")

	if fn.Input == "" {
		t.Fatal("to_timezone takes its input from the pipeline or as a leading argument, and says neither")
	}
	if !strings.Contains(fn.Input, "pipeline") {
		t.Errorf("input = %q, want it to mention the pipeline", fn.Input)
	}
}

// TestRunQueryReportsTheShapeItProduced covers what a declared catalogue cannot:
// the shape of whatever the query actually returned.
func TestRunQueryReportsTheShapeItProduced(t *testing.T) {
	cs := newTestClient(t, NewServer("test"))
	out := runQuery(t, cs, runQueryArgs{
		Query:     `[{"a":1,"b":"x"},{"a":2,"b":"y"}] | .[]`,
		NullInput: true,
		Compact:   true,
	})

	if out.Shape == "" {
		t.Fatal("run_query described no shape for a stream of objects")
	}
	if !strings.Contains(out.Shape, "2 values") {
		t.Errorf("shape does not report the count: %q", out.Shape)
	}
	if !strings.Contains(out.Shape, "a(number)") || !strings.Contains(out.Shape, "b(string)") {
		t.Errorf("shape does not report the keys and their types: %q", out.Shape)
	}
}

// TestRunQueryDescribesATruncatedRun is the case the observation exists for. The
// model is shown the first N values and cannot see the rest, so what all of them
// looked like is the only way it learns the shape.
func TestRunQueryDescribesATruncatedRun(t *testing.T) {
	cs := newTestClient(t, NewServer("test"))
	out := runQuery(t, cs, runQueryArgs{
		Query:     `range(100) | {n: .}`,
		NullInput: true,
		Compact:   true,
		Limit:     5,
	})

	if !out.Truncated {
		t.Fatal("the run was not truncated, so this test proves nothing")
	}
	if !strings.Contains(out.Shape, "n(number)") {
		t.Errorf("a truncated run did not describe its shape: %q", out.Shape)
	}
}

// TestRunQueryStaysQuietAboutPlainScalars keeps the description off output the
// caller can already read. Two numbers describe themselves.
func TestRunQueryStaysQuietAboutPlainScalars(t *testing.T) {
	cs := newTestClient(t, NewServer("test"))
	out := runQuery(t, cs, runQueryArgs{Query: `1, 2`, NullInput: true, Compact: true})

	if out.Shape != "" {
		t.Errorf("a run of two numbers was described as %q", out.Shape)
	}
}

// TestRunQueryShapeNamesTheType checks the foreign key works end to end: the
// observed values carry a PwrqType, and that name is what the catalogue lists
// the property list under.
func TestRunQueryShapeNamesTheType(t *testing.T) {
	cs := newTestClient(t, NewServer("test"))
	out := runQuery(t, cs, runQueryArgs{
		Query:     `get_date, get_date`,
		NullInput: true,
		Compact:   true,
	})

	if !strings.Contains(out.Shape, "Pwrq.DateTime") {
		t.Fatalf("the observed shape does not name the type: %q", out.Shape)
	}

	catalogue, _ := listFor(t, "get_date")
	if got := findFunction(t, catalogue, "get_date").TypeName; got != "Pwrq.DateTime" {
		t.Errorf("the catalogue lists get_date under %q, so the observed name does not resolve", got)
	}
}
