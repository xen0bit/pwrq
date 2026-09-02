package pwrgrep_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	pwrgreprules "github.com/xen0bit/pwrgrep-rules"
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
// The annotations are the convention the rules being translated are tested
// with: a comment reading `ruleid: <id>` says the next line must produce a
// finding, and `ok: <id>` says it must not. The check is set equality rather
// than containment, so a rule that fires somewhere nobody marked fails just as
// loudly as one that misses a line.
//
// This is an external test package because a rule is a query, and a query
// compiles against the cmdlet registry - which imports the rules. Reaching
// both from outside is the only way to run one the way a person would.

// fixtureRoot is where the corpus's fixtures are unpacked for a run.
//
// They arrive embedded in the rules module rather than as files here, because
// a rule and the file proving it fires are one thing and travel together - a
// rule that moved without its fixture would arrive somewhere unverifiable. A
// rule is run against a path, though, not against an fs.FS, so they are
// written out once per test binary.
var fixtureRoot string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "pwrgrep-fixtures")
	if err != nil {
		fmt.Fprintf(os.Stderr, "unpacking fixtures: %v\n", err)
		os.Exit(1)
	}
	if err := unpack(pwrgreprules.Fixtures, fixtureDir, dir); err != nil {
		fmt.Fprintf(os.Stderr, "unpacking fixtures: %v\n", err)
		os.Exit(1)
	}
	fixtureRoot = dir
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// fixtureDir is where the fixtures sit inside the rules module. They are under
// testdata/ there so that the Go tool leaves them alone: they are Go, Java,
// Python and Ruby source by construction, and a Go fixture in an ordinary
// directory is a package that `go build` compiles and gofmt rewrites - which
// would quietly repair the constructs some rules exist to catch.
const fixtureDir = "testdata/fixtures"

// unpack writes an embedded tree rooted at from into dst, flattening the root
// away so that `# fixture: go/weak-hash.go` names dst/go/weak-hash.go.
func unpack(src fs.FS, from, dst string) error {
	return fs.WalkDir(src, from, func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(from, filepath.FromSlash(name))
		if err != nil {
			return err
		}
		out := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		body, err := fs.ReadFile(src, name)
		if err != nil {
			return err
		}
		return os.WriteFile(out, body, 0o644)
	})
}

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

// corpus installs the vocabulary rules compile against and returns them all.
func corpus(t *testing.T) []*pwrgrep.Rule {
	t.Helper()
	udf.DefaultRegistry()
	rules, err := pwrgrep.Rules()
	if err != nil {
		t.Fatalf("reading the corpus: %v", err)
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
func TestEveryRuleWithAFixtureFindsExactlyWhatItMarks(t *testing.T) {
	for _, rule := range withFixtures(t) {
		t.Run(rule.Id(), func(t *testing.T) {
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
	// Walked in the module rather than in the unpacked copy: an orphan is a
	// file shipped with the corpus that no rule names, and the shipped tree is
	// the one that can hold one.
	err := fs.WalkDir(pwrgreprules.Fixtures, fixtureDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(fixtureDir, filepath.FromSlash(path))
		if err != nil {
			return err
		}
		if !claimed[filepath.Clean(rel)] {
			t.Errorf("%s is not the fixture of any rule", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestEveryRuleSaysWhereItCameFrom keeps the provenance attached to the rule
// rather than to a README that will drift away from it. A translated rule is
// checkable against the one it was translated from only while it says which
// that was.
func TestEveryRuleSaysWhereItCameFrom(t *testing.T) {
	for _, rule := range corpus(t) {
		if rule.From == "" {
			t.Errorf("%s has no `# from:` header naming the rule it was translated from", rule.Path)
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
