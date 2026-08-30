package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The corpus under testdata/rules is a port of a sample of opengrep's rules
// into pwrq queries, and this file is what keeps it honest.
//
// A structural rule fails in a way a unit test on the engine cannot catch. The
// patterns compile, the query runs, the pipeline produces an array - and the
// array is empty, or holds the wrong lines, because the pattern describes a
// construct the grammar spells differently. Nothing errors. So each rule is
// checked against a fixture that says, line by line, where it is supposed to
// fire and where it is not.
//
// The annotations are Semgrep's, because they are the convention the rules
// being ported are tested with: a comment reading `ruleid: <id>` says the next
// line must produce a finding, and `ok: <id>` says it must not. The check is
// set equality rather than containment, so a rule that fires somewhere nobody
// marked fails just as loudly as one that misses a line.

const rulesDir = "testdata/rules"

var (
	// annotation matches a `ruleid:`/`ok:` comment in a fixture, whatever the
	// language spells a comment as.
	annotation = regexp.MustCompile(`\b(ruleid|ok):\s*([A-Za-z0-9_-]+)`)
	// fixtureHeader and originHeader are the two things every rule must
	// declare: what to run it against, and where it came from.
	fixtureHeader = regexp.MustCompile(`(?m)^#\s*fixture:\s*(\S+)`)
	originHeader  = regexp.MustCompile(`(?m)^#\s*ported-from:\s*(\S.*?)\s*$`)
)

// finding is one result a rule reports. The fields the corpus is checked on
// are the ones a person reads: which rule, which file, which line.
type finding struct {
	RuleID     string `json:"RuleId"`
	Path       string `json:"Path"`
	LineNumber int    `json:"LineNumber"`
	Message    string `json:"Message"`
	Match      string `json:"Match"`
}

// rule is one ported rule and the fixture that proves it works.
type rule struct {
	id      string // the file's base name, which is also the RuleId it must emit
	path    string
	fixture string
	origin  string
}

// rules reads the corpus. It fails rather than skips when the corpus is empty:
// a test that silently checks nothing is worse than no test.
func rules(t *testing.T) []rule {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(rulesDir, "*.pwrq"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatalf("no rules found in %s", rulesDir)
	}
	sort.Strings(paths)

	out := make([]rule, 0, len(paths))
	for _, path := range paths {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		r := rule{id: strings.TrimSuffix(filepath.Base(path), ".pwrq"), path: path}
		if m := fixtureHeader.FindStringSubmatch(string(src)); m != nil {
			r.fixture = filepath.Join(rulesDir, filepath.FromSlash(m[1]))
		}
		if m := originHeader.FindStringSubmatch(string(src)); m != nil {
			r.origin = m[1]
		}
		out = append(out, r)
	}
	return out
}

// expectations reads a fixture and returns the lines a rule must report and
// the lines it must not.
//
// An annotation describes the next line of code, so a run of comments between
// the annotation and the statement it marks is skipped. Blank lines are not:
// an annotation separated from its subject by a blank line is a fixture
// nobody can read, and the line it lands on is the one the test will name.
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
			t.Errorf("%s:%d marks %q, but the rule beside it is %q",
				path, i+1, m[2], id)
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

// runRule runs one rule the way a person would: the query in a file, the path
// to search on standard input. Going through cli.run rather than compiling the
// query directly is deliberate - it is the only way the module resolution the
// rules depend on gets tested, since `include "opengrep"` is found by the
// query file sitting beside the library and by nothing else.
func runRule(t *testing.T, r rule) []finding {
	t.Helper()
	var out, errOut strings.Builder
	c := cli{
		inStream:  newStringReader(r.fixture + "\n"),
		outStream: &out,
		errStream: &errOut,
	}
	if code := c.run([]string{"--raw-input", "--from-file", r.path}); code != 0 {
		t.Fatalf("%s exited %d: %s", r.path, code, strings.TrimSpace(errOut.String()))
	}
	var findings []finding
	if err := json.Unmarshal([]byte(out.String()), &findings); err != nil {
		t.Fatalf("%s produced output that is not a list of findings: %v\n%s", r.path, err, out.String())
	}
	return findings
}

// TestEveryRuleFindsExactlyWhatItsFixtureMarks is the corpus test.
func TestEveryRuleFindsExactlyWhatItsFixtureMarks(t *testing.T) {
	for _, r := range rules(t) {
		t.Run(r.id, func(t *testing.T) {
			if r.fixture == "" {
				t.Fatalf("%s has no `# fixture:` header, so there is nothing to check it against", r.path)
			}
			want, permitted := expectations(t, r.fixture, r.id)
			if len(want) == 0 {
				t.Fatalf("%s marks no line with `ruleid: %s`, so it cannot show the rule fires",
					r.fixture, r.id)
			}
			if len(permitted) == 0 {
				t.Fatalf("%s marks no line with `ok: %s`, so it cannot show the rule is not "+
					"simply firing everywhere", r.fixture, r.id)
			}

			var got []int
			for _, f := range runRule(t, r) {
				if f.RuleID != r.id {
					t.Errorf("%s reported RuleId %q; a rule is named by its file", r.path, f.RuleID)
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
					r.fixture, got, want, permitted)
			}
		})
	}
}

// TestEveryRuleSaysWhereItCameFrom keeps the provenance attached to the rule
// rather than to a README that will drift away from it. These are ports of
// somebody else's work, and the id they were ported from is what lets a reader
// check the port against the original.
func TestEveryRuleSaysWhereItCameFrom(t *testing.T) {
	for _, r := range rules(t) {
		if r.origin == "" {
			t.Errorf("%s has no `# ported-from:` header naming the opengrep rule it came from", r.path)
		}
	}
}

// TestEveryFixtureBelongsToARule catches the fixture left behind when a rule is
// renamed or dropped - a file that looks like coverage and is not.
func TestEveryFixtureBelongsToARule(t *testing.T) {
	claimed := map[string]bool{}
	for _, r := range rules(t) {
		if r.fixture != "" {
			claimed[filepath.Clean(r.fixture)] = true
		}
	}
	err := filepath.WalkDir(filepath.Join(rulesDir, "fixtures"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !claimed[filepath.Clean(path)] {
			t.Errorf("%s is not the fixture of any rule", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
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
