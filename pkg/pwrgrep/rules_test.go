package pwrgrep_test

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/xen0bit/pwrq/pkg/pwrgrep"
	"github.com/xen0bit/pwrq/pkg/udf"
)

// A structural rule fails in a way a unit test on the engine cannot catch. The
// patterns compile, the query runs, the pipeline produces an array - and the
// array is empty, or holds the wrong lines, because the pattern describes a
// construct the grammar spells differently. Nothing errors. So the rules that
// carry a fixture are checked against it line by line: where the rule is
// supposed to fire, and where it is not.
//
// The annotations are the convention semgrep's rules are tested with: a
// comment reading `ruleid: <id>` says the next line must produce a
// finding, and `ok: <id>` says it must not. The check is set equality rather
// than containment, so a rule that fires somewhere nobody marked fails just as
// loudly as one that misses a line.
//
// This is an external test package because a rule is a query, and a query
// compiles against the cmdlet registry - which imports the rules. Reaching
// both from outside is the only way to run one the way a person would.

// fixtureRoot is where the corpus's fixtures are. They are under testdata/ so
// that the Go tool leaves them alone: they are Go, Java, Python and C source by
// construction, and a Go fixture in an ordinary directory is a package that
// `go build` compiles and gofmt rewrites.
const fixtureRoot = "testdata/fixtures"

// annotation matches a `ruleid:`/`ok:` comment in a fixture, whatever the
// language spells a comment as.
var annotation = regexp.MustCompile(`\b(ruleid|ok):\s*([A-Za-z0-9_-]+)`)

// finding is one result a rule reports. The fields checked here are the ones a
// person reads: which rule, which file, which line.
type finding struct {
	RuleID     string `json:"RuleId"`
	Path       string `json:"Path"`
	LineNumber int    `json:"LineNumber"`
	Message    string `json:"Message"`
}

// corpus installs the vocabulary rules compile against and returns the rules
// built into the binary. Those found in a directory are left out: they are
// whatever this machine has installed or somebody is writing, and neither is
// what this repository ships.
func corpus(t *testing.T) []*pwrgrep.Rule {
	t.Helper()
	udf.DefaultRegistry()
	all, err := pwrgrep.Rules()
	if err != nil {
		t.Fatalf("reading the corpus: %v", err)
	}
	var rules []*pwrgrep.Rule
	for _, rule := range all {
		if rule.Origin == pwrgrep.Builtin {
			rules = append(rules, rule)
		}
	}
	if len(rules) == 0 {
		t.Fatal("the corpus is empty")
	}
	return rules
}

// withFixtures are the rules that name a fixture. It fails rather than skips
// when there are none: a test that silently checks nothing is worse than no
// test.
func withFixtures(t *testing.T) []*pwrgrep.Rule {
	t.Helper()
	var out []*pwrgrep.Rule
	for _, rule := range corpus(t) {
		if rule.Fixture != "" {
			out = append(out, rule)
		}
	}
	if len(out) == 0 {
		t.Fatal("no rule in the corpus names a fixture, so this test proves nothing")
	}
	return out
}

// expectations reads a fixture and returns the lines a rule must report and
// the lines it must not.
//
// An annotation describes the next line of code, so a run of comments between
// the annotation and the statement it marks is skipped. Blank lines are not:
// an annotation separated from its subject by a blank line is a fixture nobody
// can read, and the line it lands on is the one the test will name.
func expectations(t *testing.T, path, id string) (want []int, ok []int) {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(src), "\n")
	for i, line := range lines {
		m := annotation.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if m[2] != id {
			t.Errorf("%s:%d marks %q, but the rule beside it is %q", path, i+1, m[2], id)
			continue
		}
		subject := i + 1
		for subject < len(lines) && annotation.MatchString(lines[subject]) {
			subject++
		}
		if subject >= len(lines) {
			t.Errorf("%s:%d annotates nothing; it is the last line", path, i+1)
			continue
		}
		if m[1] == "ruleid" {
			want = append(want, subject+1)
		} else {
			ok = append(ok, subject+1)
		}
	}
	sort.Ints(want)
	sort.Ints(ok)
	return want, ok
}

// TestEveryRuleWithAFixtureFindsExactlyWhatItMarks is the corpus test.
//
// The subtests run in parallel because there is one per rule, and each reads
// one fixture file and shares nothing with the others.
func TestEveryRuleWithAFixtureFindsExactlyWhatItMarks(t *testing.T) {
	for _, rule := range withFixtures(t) {
		t.Run(rule.Id(), func(t *testing.T) {
			t.Parallel()
			id := rule.Id()
			fixture := filepath.Join(fixtureRoot, filepath.FromSlash(rule.Fixture))
			want, permitted := expectations(t, fixture, id)
			if len(want) == 0 {
				t.Fatalf("%s marks no line with `ruleid: %s`, so it cannot show the rule fires",
					fixture, id)
			}
			if len(permitted) == 0 {
				t.Fatalf("%s marks no line with `ok: %s`, so it cannot show the rule is not "+
					"simply firing everywhere", fixture, id)
			}

			var got []int
			for _, f := range run(t, rule, fixture) {
				if f.RuleID != id {
					t.Errorf("%s reported RuleId %q; a rule is named by its header", rule.Path, f.RuleID)
				}
				if f.Message == "" {
					t.Errorf("%s:%d reported no message, so a reader is told what was found "+
						"but not why it matters", f.Path, f.LineNumber)
				}
				got = append(got, f.LineNumber)
			}
			sort.Ints(got)

			if !sameLines(got, want) {
				t.Errorf("%s\n  fired on lines %v\n  fixture marks  %v\n  (lines marked ok: %v)",
					fixture, got, want, permitted)
			}
		})
	}
}

// run searches one path with one rule. The findings come back as the values a
// query produced, so they go through JSON to be read as findings - which is
// also what proves a rule emits the shape a caller is promised.
func run(t *testing.T, rule *pwrgrep.Rule, root string) []finding {
	t.Helper()
	values, err := rule.Run(context.Background(), root)
	if err != nil {
		t.Fatalf("running %s: %v", rule.Path, err)
	}
	body, err := json.Marshal(values)
	if err != nil {
		t.Fatalf("%s produced values that are not JSON: %v", rule.Path, err)
	}
	var findings []finding
	if err := json.Unmarshal(body, &findings); err != nil {
		t.Fatalf("%s produced output that is not a list of findings: %v\n%s", rule.Path, err, body)
	}
	return findings
}

// TestEveryFixtureBelongsToARule catches the fixture left behind when a rule
// is renamed or dropped - a file that looks like coverage and is not.
func TestEveryFixtureBelongsToARule(t *testing.T) {
	claimed := map[string]bool{}
	for _, rule := range withFixtures(t) {
		claimed[filepath.Clean(filepath.FromSlash(rule.Fixture))] = true
	}
	err := filepath.WalkDir(fixtureRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(fixtureRoot, path)
		if err != nil {
			return err
		}
		if !claimed[rel] {
			t.Errorf("%s is not the fixture of any rule", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestEveryShippedRuleHasAFixture holds the built-in corpus to the standard a
// reader is entitled to: a rule that ships says, in a file beside it, what it
// is supposed to find.
func TestEveryShippedRuleHasAFixture(t *testing.T) {
	for _, rule := range corpus(t) {
		if rule.Fixture == "" {
			t.Errorf("%s has no `# fixture:` header, so nothing shows what it finds", rule.Path)
		}
	}
}

// sameLines compares two sorted line lists.
func sameLines(a, b []int) bool {
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
