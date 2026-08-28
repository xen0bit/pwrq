package mcpserver

import (
	"strings"
	"testing"

	"github.com/xen0bit/pwrq/pkg/udf"
)

// TestArityMistakeNamesTheDefinition is the regression this file is named for.
//
// A model wrote `def C(o): ...` and called `$envelope | C`. gojq answered
// "function not defined: C/0", which is true and useless: it says nothing
// about the C/1 three lines above. The model tested its reading with the same
// arity mistake a second time, got the same message, concluded that pwrq
// rejects custom definitions outright, rewrote its entire query with every
// definition inlined, and told its user about the limitation it had invented.
func TestArityMistakeNamesTheDefinition(t *testing.T) {
	udf.DefaultRegistry()

	query := `def C(o): o | json_stringify; "hi" | C`
	got := explainCompileError("function not defined: C/0", query)

	if !strings.Contains(got, "you defined C taking 1 argument") {
		t.Errorf("the message does not say C is defined at another arity:\n    %s", got)
	}
	if !strings.Contains(got, "C(...)") {
		t.Errorf("the message does not show how to write the call:\n    %s", got)
	}
}

// TestCatalogueArityMistakeNamesTheRange covers the same mistake against a
// cmdlet rather than a definition the caller wrote.
func TestCatalogueArityMistakeNamesTheRange(t *testing.T) {
	udf.DefaultRegistry()

	got := explainCompileError("function not defined: sha256/3", "sha256(1;2;3)")
	if !strings.Contains(got, "sha256 takes 0 to 2 arguments") {
		t.Errorf("the message does not give sha256's arity range:\n    %s", got)
	}
}

// TestUnknownNameSuggestsTheRealOne covers the other half: a name that is not
// defined anywhere, where what the caller needs is the name they meant.
func TestUnknownNameSuggestsTheRealOne(t *testing.T) {
	udf.DefaultRegistry()

	got := explainCompileError("function not defined: sha257/0", "sha257")
	if !strings.Contains(got, "sha256") {
		t.Errorf("the message does not suggest the name that exists:\n    %s", got)
	}
}

// TestUnrelatedCompileErrorsAreLeftAlone keeps the enrichment from inventing
// context for failures it knows nothing about.
func TestUnrelatedCompileErrorsAreLeftAlone(t *testing.T) {
	udf.DefaultRegistry()

	const message = "variable not defined: $foo"
	if got := explainCompileError(message, "$foo"); got != message {
		t.Errorf("rewrote an unrelated failure:\n    %s", got)
	}
}

// TestValidateCatchesWhatRunWouldReject is the divergence that made every
// green validate_query worth less than it looked.
//
// validate_query only parsed, and parsing accepts any name at any arity, so a
// query could validate cleanly and then fail to compile the moment it ran. In
// the recorded session that happened four times, and each green validate made
// the following failure read as though it were about something else.
func TestValidateCatchesWhatRunWouldReject(t *testing.T) {
	udf.DefaultRegistry()

	rejected := []struct {
		name  string
		query string
	}{
		{"wrong arity on a definition", `def C(o): o | json_stringify; "hi" | C`},
		{"wrong arity on a cmdlet", `sha256(1;2;3)`},
		{"a name that does not exist", `notacmdlet`},
	}
	for _, tc := range rejected {
		t.Run(tc.name, func(t *testing.T) {
			res := validateQuery(validateQueryArgs{Query: tc.query})
			if res.OK {
				t.Fatalf("validate accepted a query that cannot run: %s", tc.query)
			}
			if res.Stage != stageCompile {
				t.Errorf("stage = %q, want %q", res.Stage, stageCompile)
			}
			// A compile failure is about one call in an otherwise well-formed
			// program, and the caller is about to edit it.
			if res.Formatted == "" {
				t.Error("a query that parsed came back without its layout")
			}
		})
	}
}

// TestValidateStillAcceptsWorkingQueries checks the stricter check did not
// start rejecting programs that run, including the ones reaching cmdlets
// through an alias and through a named variable.
func TestValidateStillAcceptsWorkingQueries(t *testing.T) {
	udf.DefaultRegistry()

	accepted := []struct {
		name  string
		query string
		args  []namedArg
	}{
		{"plain jq", `.a.b | length`, nil},
		{"a cmdlet", `"hello" | sha256`, nil},
		{"a definition called correctly", `def C(o): o | json_stringify; C({a: 1})`, nil},
		{"a variable the caller bound", `$rows | length`, []namedArg{{Name: "rows", Value: "[]"}}},
		{"a variable bound with its dollar", `$rows | length`, []namedArg{{Name: "$rows", Value: "[]"}}},
	}
	for _, tc := range accepted {
		t.Run(tc.name, func(t *testing.T) {
			res := validateQuery(validateQueryArgs{Query: tc.query, Args: tc.args})
			if !res.OK {
				t.Errorf("validate rejected a working query %q: %s", tc.query, res.Error)
			}
		})
	}
}

// TestValidateSeparatesItsTwoFailures checks a caller can tell "this is not a
// program" from "this program calls something that is not there", which are
// different mistakes with different fixes.
func TestValidateSeparatesItsTwoFailures(t *testing.T) {
	udf.DefaultRegistry()

	res := validateQuery(validateQueryArgs{Query: `. | | .`})
	if res.OK {
		t.Fatal("validate accepted a query that does not parse")
	}
	if res.Stage != stageParse {
		t.Errorf("stage = %q, want %q", res.Stage, stageParse)
	}
}

// TestValidateExplainsArityInline checks the enrichment reaches validate_query
// too, which is the tool a caller iterates in.
func TestValidateExplainsArityInline(t *testing.T) {
	udf.DefaultRegistry()

	res := validateQuery(validateQueryArgs{Query: `def C(o): o; "hi" | C`})
	if !strings.Contains(res.Error, "you defined C taking 1 argument") {
		t.Errorf("validate reported the bare compiler message:\n    %s", res.Error)
	}
}
