package udf

import (
	"context"
	"strings"
	"testing"

	"github.com/xen0bit/pwrq/pkg/core/queryrun"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// evaluate runs a query against a restricted vocabulary, the way invoke_agent
// does.
func evaluate(t *testing.T, allow []string, query string) queryrun.Result {
	t.Helper()
	vocab, err := VocabularyFor(allow)
	if err != nil {
		t.Fatalf("building the vocabulary: %v", err)
	}
	return vocab.Runner.Run(context.Background(), &queryrun.Request{
		Query: query, NullInput: true, Compact: true, MaxResults: 10, MaxOutputBytes: 4096,
	})
}

// TestVocabularyGrantsWhatWasAsked checks the allowed half: a cmdlet on the
// list is callable.
func TestVocabularyGrantsWhatWasAsked(t *testing.T) {
	res := evaluate(t, []string{"basename"}, `basename("/a/b/c.txt")`)
	if res.Error != "" {
		t.Fatalf("an allowed cmdlet did not run: %s", res.Error)
	}
	if len(res.Values) != 1 || res.Values[0] != `"c.txt"` {
		t.Errorf("got %v", res.Values)
	}
}

// TestVocabularyDeniesStructurally is the security property invoke_agent rests
// on: a cmdlet that was not allowed is not a rule to be checked at call time,
// it is absent from the compiler.
//
// The distinction matters. A runtime check runs after the arguments have been
// evaluated and has to be right everywhere it is consulted; a name that does
// not compile cannot be reached at all, whatever the model writes.
func TestVocabularyDeniesStructurally(t *testing.T) {
	for _, denied := range []string{
		`sh("id")`,
		`rm("/tmp/x"; "file")`,
		`out_file("/tmp/x")`,
		`http("GET"; "https://example.com")`,
		`invoke_llm("hi")`,
		`get_process`,
	} {
		res := evaluate(t, []string{"basename", "cat"}, denied)
		if res.Error == "" {
			t.Errorf("%s ran under a vocabulary that did not include it", denied)
			continue
		}
		if res.Kind != queryrun.KindCompile {
			t.Errorf("%s failed as %q; it should not compile at all", denied, res.Kind)
		}
	}
}

// TestVocabularyRejectsUnknownNames keeps a typo in Allow from silently
// producing a smaller vocabulary than the caller asked for.
func TestVocabularyRejectsUnknownNames(t *testing.T) {
	_, err := VocabularyFor([]string{"cat", "not_a_cmdlet"})
	if err == nil {
		t.Fatal("an unknown cmdlet name was accepted")
	}
	if !strings.Contains(err.Error(), "not_a_cmdlet") {
		t.Errorf("the error should name what was not found: %v", err)
	}
}

// TestVocabularyCarriesDocumentation checks that the agent is told what its
// cmdlets do, from the same metadata --udf-list reads.
func TestVocabularyCarriesDocumentation(t *testing.T) {
	vocab, err := VocabularyFor([]string{"get_childitem"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vocab.Commands) != 1 {
		t.Fatalf("got %d commands", len(vocab.Commands))
	}
	c := vocab.Commands[0]
	if c.Description == "" || len(c.Examples) == 0 {
		t.Errorf("get_childitem reached the agent with no description or example: %+v", c)
	}
	if !c.Streaming {
		t.Error("get_childitem streams, and an agent that does not know will forget the brackets")
	}
}

// TestVocabularyKeepsJqItself pins that narrowing the cmdlets does not narrow
// the language. An agent that could not call length or map would be unable to
// use the cmdlets it does have.
func TestVocabularyKeepsJqItself(t *testing.T) {
	res := evaluate(t, []string{"basename"}, `[1,2,3] | map(. * 2) | add`)
	if res.Error != "" {
		t.Fatalf("jq's own builtins are not available: %s", res.Error)
	}
	if res.Values[0] != "12" {
		t.Errorf("got %v", res.Values)
	}
}

// TestVocabularyExcludesTheEnvironment: the agent's queries run without an
// environment loader, so `env` cannot hand a model the API keys of the process
// running it.
func TestVocabularyExcludesTheEnvironment(t *testing.T) {
	res := evaluate(t, []string{"basename"}, `env | length`)
	if res.Error == "" && len(res.Values) > 0 && res.Values[0] != "0" {
		t.Errorf("env returned %v to an agent query", res.Values)
	}
}

// TestScriptBlocksAreNarrowedToo covers the hole an allowlist would otherwise
// have. A script block is a whole query compiled separately, against whatever
// vocabulary the host installed at startup — so an agent allowed to call
// where_object could reach sh from inside `{script: "..."}` while every
// cmdlet in its own query was restricted.
//
// invoke_agent runs its queries inside common.WithScriptBlockOptions for this
// reason. The second half of this test is the part that justifies the first:
// with the full vocabulary installed, the same script block does reach sh.
func TestScriptBlocksAreNarrowedToo(t *testing.T) {
	vocab, err := VocabularyFor([]string{"where_object"})
	if err != nil {
		t.Fatal(err)
	}
	const escape = `where_object([{a: 1}]; {script: "sh(\"echo escaped\") | true"})`

	common.SetScriptBlockOptions(DefaultRegistry().Options())
	t.Cleanup(func() { common.SetScriptBlockOptions(nil) })

	unguarded := vocab.Runner.Run(context.Background(), &queryrun.Request{
		Query: escape, NullInput: true, Compact: true, MaxResults: 5, MaxOutputBytes: 1024,
	})
	if unguarded.Error != "" {
		t.Skipf("the escape does not work even unguarded, so the guard proves nothing here: %s", unguarded.Error)
	}

	var guarded queryrun.Result
	common.WithScriptBlockOptions(vocab.Options, func() {
		guarded = vocab.Runner.Run(context.Background(), &queryrun.Request{
			Query: escape, NullInput: true, Compact: true, MaxResults: 5, MaxOutputBytes: 1024,
		})
	})
	if guarded.Error == "" {
		t.Fatal("a script block reached sh from inside a restricted vocabulary")
	}
}

// TestScriptBlockOptionsAreRestored checks the swap puts back what it found.
// A host that lost its vocabulary after one agent run would break every later
// query in the process.
func TestScriptBlockOptionsAreRestored(t *testing.T) {
	full := DefaultRegistry().Options()
	common.SetScriptBlockOptions(full)
	t.Cleanup(func() { common.SetScriptBlockOptions(nil) })

	common.WithScriptBlockOptions(nil, func() {
		if _, err := common.CompileScriptBlock(`sh("true")`); err == nil {
			t.Error("the narrowed vocabulary was not installed")
		}
	})

	if _, err := common.CompileScriptBlock(`sh("true")`); err != nil {
		t.Errorf("the previous vocabulary was not restored: %v", err)
	}
}
