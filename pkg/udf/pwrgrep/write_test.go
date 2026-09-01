package pwrgrep_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/xen0bit/pwrq/pkg/core/queryrun"
	"github.com/xen0bit/pwrq/pkg/udf"
)

// A rule written now is a rule this process can run now.
//
// This is the whole of the authoring loop over MCP, where there is no shell to
// drop out to and no restart between writing a file and asking for it: one
// process answers for hours, and if the catalogue it read at startup is the
// only one it will ever have, an agent that writes a rule is told there is no
// such rule. So the test is the loop rather than the cmdlet - write, run,
// edit, run again - and it runs against the real vocabulary, because a rule
// that compiles without scan_ast is not a rule.
func TestARuleWrittenOverMCPRunsInTheSameProcess(t *testing.T) {
	rules := t.TempDir()
	t.Setenv("PWRQ_RULES", rules)
	src := write(t, "main.go", `package main

import "net/http"

func main() {
	c := &http.Client{}
	_, _ = c.Get("http://example.com")
	http.Get("http://example.com")
}
`)

	rule := func(patterns string) string {
		return `# rules: mine-no-timeout
# languages: go

scan_ast("*.go"; ` + patterns + `)
| finding("mine-no-timeout"; "this request can never give up")
| report
`
	}
	written := run(t, `write_pwrgrep_rule("mine/no-timeout"; `+quoted(rule(`["&http.Client{}"]`))+`) | {Path, File, Id}`)
	got, _ := written.(map[string]any)
	if got["Path"] != "mine/no-timeout" || got["Id"] != "mine-no-timeout" {
		t.Fatalf("wrote %v", written)
	}
	if file, _ := got["File"].(string); !strings.HasPrefix(file, rules) {
		t.Fatalf("the rule went to %q, outside %q", file, rules)
	}

	lines := run(t, `[invoke_pwrgrep(`+quoted(src)+`; "mine-no-timeout")] | map(.LineNumber)`)
	if want := []any{6.0}; !equal(lines, want) {
		t.Fatalf("the rule just written found %v, wanted %v", lines, want)
	}

	// And an edit, in the same process, is the version that runs: the rule was
	// compiled to run it the first time, and that copy has to go with it.
	edited := run(t, `write_pwrgrep_rule("mine/no-timeout"; `+quoted(rule(`["&http.Client{}", "http.Get($$$A)"]`))+`)
		| [invoke_pwrgrep(`+quoted(src)+`; .Path)] | map(.LineNumber)`)
	if want := []any{6.0, 8.0}; !equal(edited, want) {
		t.Fatalf("the edited rule found %v, wanted %v", edited, want)
	}
}

// A file that is not a rule fails the catalogue rather than failing to fire,
// so a write that would leave one behind is refused before it lands.
func TestARuleThatWouldBreakTheCatalogueIsNotWritten(t *testing.T) {
	rules := t.TempDir()
	t.Setenv("PWRQ_RULES", rules)

	for _, source := range []string{
		`scan_ast("*.go"; ["panic($$$A)"]) | report`,    // no header
		"# rules: nope\n\nscan_ast(\"*.go\" [\"x\"])\n", // scan_ast/1 is not a cmdlet
	} {
		if errorOf(t, `write_pwrgrep_rule("mine/bad"; `+quoted(source)+`)`) == "" {
			t.Errorf("this was written:\n%s", source)
		}
	}

	entries, err := os.ReadDir(rules)
	if err != nil || len(entries) != 0 {
		t.Fatalf("a refused write left %v behind (%v)", entries, err)
	}
	// The catalogue still reads, which is the thing being protected.
	run(t, `[get_pwrgrep_rule("go-weak-hash")] | length`)
}

// errorOf runs a query that is expected to fail and returns what it said.
func errorOf(t *testing.T, query string) string {
	t.Helper()
	runner := &queryrun.Runner{Options: udf.DefaultRegistry().Options()}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return runner.Run(ctx, &queryrun.Request{Query: query, NullInput: true, MaxResults: 1}).Error
}
