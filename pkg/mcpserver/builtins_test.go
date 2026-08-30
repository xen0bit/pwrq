package mcpserver

import (
	"strings"
	"testing"
)

// TestEveryGlossIsABuiltin checks the one hand-written thing in builtins.go
// against gojq itself.
//
// The names and arities are computed, so they cannot go stale. The glosses are
// typed by hand, so they can: a gojq release that renames or drops a function
// would leave a description of something that is no longer callable, and a
// description of a function that does not exist is worse than no description
// at all - it is the exact failure this file was written to remove, pointed
// the other way.
func TestEveryGlossIsABuiltin(t *testing.T) {
	known := make(map[string]bool)
	for _, b := range jqBuiltins() {
		known[b.Name] = true
	}
	for name := range glosses {
		if !known[name] {
			t.Errorf("glosses describes %q, which is not a jq builtin pwrq exposes"+
				" - either gojq no longer has it, or a cmdlet now shadows it", name)
		}
	}
}

// TestGlossedBuiltinsAreCallable proves the stronger claim the gloss makes:
// not just that gojq lists the name, but that a caller writing it gets a
// program that compiles.
func TestGlossedBuiltinsAreCallable(t *testing.T) {
	for _, b := range jqBuiltins() {
		if b.Gloss == "" {
			continue
		}
		res := validateQuery(validateQueryArgs{Query: callAt(b.Name, b.MinArgs)})
		if !res.OK {
			t.Errorf("%s is offered to callers but does not compile: %s", b.Name, res.Error)
		}
	}
}

// callAt writes a call to a name at a given arity, with `.` for each argument
// so the program compiles whatever the parameter is used for.
func callAt(name string, args int) string {
	if args == 0 {
		return name
	}
	return name + "(" + strings.Repeat(".; ", args-1) + ".)"
}

// TestBuiltinsExcludeTheCatalogue checks the subtraction that makes this list
// mean what it says. A name pwrq documents as a cmdlet must not also be
// offered as an undocumented jq builtin: the catalogue's entry is the one that
// is true, and listing both would put two different answers for one name in
// front of the caller.
func TestBuiltinsExcludeTheCatalogue(t *testing.T) {
	documented := make(map[string]bool)
	for _, c := range catalogue() {
		documented[c.Name] = true
	}
	for _, b := range jqBuiltins() {
		if documented[b.Name] {
			t.Errorf("%s is listed both as a cmdlet and as a jq builtin", b.Name)
		}
	}
}

// TestTheVocabularyIsNotJustTheCatalogue is the regression this whole file is.
// list_functions used to answer "no functions match ascii_upcase, and nothing
// is named close to it" - a flat denial about a function that runs.
func TestTheVocabularyIsNotJustTheCatalogue(t *testing.T) {
	for _, name := range []string{"ascii_upcase", "ascii_downcase", "fromjson", "to_entries", "walk"} {
		t.Run(name, func(t *testing.T) {
			res := listFunctions(listFunctionsArgs{Filter: name})
			if len(res.Builtins) == 0 {
				t.Fatalf("list_functions %q found nothing, but %q runs", name, name)
			}
			text := describeFunctions(listFunctionsArgs{Filter: name}, res)
			if strings.Contains(text, "nothing is named close to it") {
				t.Errorf("list_functions %q denies a working function:\n%s", name, text)
			}
			if !strings.Contains(text, name) {
				t.Errorf("list_functions %q never names it:\n%s", name, text)
			}
		})
	}
}

// TestUnresolvedNamesReachJq pins the suggestions for the names a caller
// actually reaches for. Each of these was tried against a live MCP server and
// came back with either nothing or the wrong thing.
func TestUnresolvedNamesReachJq(t *testing.T) {
	for _, tc := range []struct{ typed, want string }{
		{"from_json", "fromjson"},
		{"parse_json", "fromjson"},
		{"to_upper", "ascii_upcase"},
		{"to_lower", "ascii_downcase"},
	} {
		t.Run(tc.typed, func(t *testing.T) {
			hint := nameHint(tc.typed)
			if !strings.Contains(hint, tc.want) {
				t.Errorf("%s hinted %q, which never mentions %s", tc.typed, hint, tc.want)
			}
		})
	}
}

// TestWrongArityOfABuiltinSaysSo covers the other way a jq name fails. `"a,b" |
// split` is a call missing its separator, and before the builtins were known
// it was reported as an unknown name and answered with "did you mean
// split_path?".
func TestWrongArityOfABuiltinSaysSo(t *testing.T) {
	hint := explainCompileError("function not defined: split/0", `"a,b" | split`)
	if !strings.Contains(hint, "1 to 2 arguments") {
		t.Errorf("split/0 explained as %q, which does not say what arity it takes", hint)
	}
	if strings.Contains(hint, "did you mean") {
		t.Errorf("split/0 explained as %q, which treats a known name as a typo", hint)
	}
}

// TestABroadSearchDoesNotFloodWithBuiltins keeps the jq section from doing to
// a listing what it was added to fix. A one-letter filter matches over a
// hundred builtins, and appending all of them would bury the cmdlets the
// caller actually searched for.
func TestABroadSearchDoesNotFloodWithBuiltins(t *testing.T) {
	res := listFunctions(listFunctionsArgs{Filter: "a"})
	if len(res.Builtins) <= builtinsUpTo {
		t.Skipf("%q matches only %d builtins, which is not a broad enough search to test the cap",
			"a", len(res.Builtins))
	}
	text := describeFunctions(listFunctionsArgs{Filter: "a"}, res)
	if lines := strings.Count(text, " [jq]"); lines > builtinsUpTo {
		t.Errorf("a one-letter filter printed %d jq builtins, above the cap of %d", lines, builtinsUpTo)
	}
	if !strings.Contains(text, "more - narrow the term") {
		t.Errorf("the listing was cut without saying so:\n%s", text[len(text)-200:])
	}
}
