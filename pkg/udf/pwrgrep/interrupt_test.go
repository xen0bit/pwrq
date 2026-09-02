package pwrgrep_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xen0bit/pwrq/pkg/core/queryrun"
	"github.com/xen0bit/pwrq/pkg/udf"
)

// What a run of the rule corpus does when it is not allowed to finish, and
// what it does when it is allowed to finish twice.
//
// Both are properties of invoke_pwrgrep rather than of any rule, and both were
// wrong in a way that only showed up on a repository large enough to be worth
// scanning: a deadline was ignored for as long as the scan took, and the files
// a rule parsed were parsed again by every rule after it.

// tree writes a few files a rule can be run over. Small, because none of these
// tests is about how much there is to find - only about whether the search
// stops when it is told to and agrees with itself when it does not.
func tree(t *testing.T, files int) string {
	t.Helper()
	dir := t.TempDir()
	for i := range files {
		name := filepath.Join(dir, "mod"+string(rune('a'+i%26))+string(rune('a'+i/26))+".py")
		source := strings.Join([]string{
			"import hashlib",
			"import subprocess",
			"",
			"def digest(data):",
			"    return hashlib.md5(data).hexdigest()",
			"",
			"def run(cmd):",
			"    return subprocess.call(cmd, shell=True)",
		}, "\n")
		if err := os.WriteFile(name, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// runIn evaluates a query under a context the caller controls, and returns the
// result rather than failing on it: these tests are about the failures.
func runIn(ctx context.Context, query string) queryrun.Result {
	runner := &queryrun.Runner{Options: udf.DefaultRegistry().Options()}
	return runner.Run(ctx, &queryrun.Request{
		Query: query, NullInput: true, Compact: true, MaxResults: 10000,
	})
}

// A deadline reaches inside the search rather than waiting at the edge of it.
//
// gojq checks the context between the values a program yields, and
// invoke_pwrgrep used to yield nothing until every rule had run - so a scan of
// a large tree ran for as long as it took whatever it had been told. Under the
// MCP server that was the whole failure: the client gave up, the run carried
// on holding the engine, and every later call queued behind a scan nobody was
// waiting for.
//
// One second against a corpus that takes far longer, so the test is not a race
// with the machine it runs on: what is being asserted is that the run ends
// somewhere near when it was told to, not exactly then.
func TestAScanStopsWhenItsDeadlinePasses(t *testing.T) {
	dir := tree(t, 40)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	started := time.Now()
	res := runIn(ctx, `invoke_pwrgrep("`+dir+`"; "python")`)
	took := time.Since(started)

	if res.Error == "" {
		t.Fatalf("the whole Python corpus finished inside a second, so this "+
			"proves nothing; it produced %d findings in %s", res.Count, took)
	}
	if res.Kind != queryrun.KindTimeout {
		t.Fatalf("stopped with %s (%q), want %s", res.Kind, res.Error, queryrun.KindTimeout)
	}
	// The bound is loose on purpose. It is not a claim about how promptly the
	// search notices - that is one file's parse - but about the difference
	// between stopping and not stopping, which before this was minutes.
	if took > 30*time.Second {
		t.Fatalf("took %s to honour a one-second deadline", took)
	}
}

// A cancelled run says so rather than reporting what it managed.
//
// This is the failure that matters most for a tool whose whole job is finding
// things: a search cut off partway has looked at some of the tree, and "some
// of the tree" and "the tree" produce the same empty result. Reporting the
// cancellation is what keeps a stopped scan from reading as a clean one.
func TestACancelledScanIsAFailureRatherThanACleanResult(t *testing.T) {
	dir := tree(t, 40)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res := runIn(ctx, `[invoke_pwrgrep("`+dir+`"; "python")] | length`)
	if res.Error == "" {
		t.Fatalf("a cancelled scan returned %v with no error", res.Values)
	}
}

// The findings do not depend on the cache.
//
// A cache that changes what a rule reports is worse than no cache at all, and
// the way it would go wrong is not subtle to state: a tree held from one rule
// and read by the next is the same tree or it is a bug. Running the same rules
// twice in one process is what exercises that - the second run reads what the
// first one parsed - and the two must agree.
//
// Named rules rather than the whole Python corpus, because this is the one
// test here that has to run to completion rather than stop at a deadline, and
// the corpus is 332 rules: under -race on a CI runner, running it twice is
// longer than `go test` allows a package. What the cache is asked to get wrong
// needs two rules over one tree, not every rule - these four all fire on the
// files tree writes, so each of them reads what the one before it parsed.
func TestTheSameCorpusTwiceInOneProcessAgreesWithItself(t *testing.T) {
	dir := tree(t, 12)
	rules := `["python-weak-hash", "python-subprocess-shell-true", "dangerous-subprocess-use-audit", "subprocess-injection"]`
	query := `[invoke_pwrgrep("` + dir + `"; ` + rules + `)] | map(.RuleId + " " + .Path + " " + (.LineNumber | tostring)) | sort | join(",")`

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	first := runIn(ctx, query)
	if first.Error != "" {
		t.Fatalf("first run: %s", first.Error)
	}
	second := runIn(ctx, query)
	if second.Error != "" {
		t.Fatalf("second run: %s", second.Error)
	}
	if len(first.Values) != 1 || len(second.Values) != 1 {
		t.Fatalf("expected one value from each run, got %d and %d",
			len(first.Values), len(second.Values))
	}
	if first.Values[0] != second.Values[0] {
		t.Fatalf("the same corpus reported different findings on the second run\n"+
			"first:  %s\nsecond: %s", first.Values[0], second.Values[0])
	}
	if first.Values[0] == `""` {
		t.Fatal("no findings at all, so the comparison proves nothing")
	}
}

// Findings arrive as they are found rather than at the end.
//
// `first(...)` reads one value and abandons the iterator, so a streaming
// search stops after the rule that produced it and a buffered one runs the
// whole corpus first. The difference is the whole point of streaming: a scan
// that is stopped partway still has something to show for the rules it
// finished.
//
// Measured against a deadline shorter than the full corpus takes. If the
// search were still buffered, the deadline would arrive before the first value
// did and this would fail with a timeout instead of a finding.
//
// Ninety seconds is not a claim that the first finding takes anything like
// that long - warm, it is a tenth of a second. It is the gap between the two
// answers this can give, and the gap is enormous: streaming puts a value in
// hand in well under a second, and buffering would have to run 332 rules over
// forty files first, which is minutes. What the number has to clear is the
// cost of being first, and the first search in a process compiles the corpus's
// tree-sitter patterns before it can match anything - four seconds here under
// -race, and more than twenty on a CI runner, which is what a deadline of
// twenty was failing on. A passing run still returns as soon as the finding
// does; the ninety is only ever spent proving the failure.
func TestTheFirstFindingArrivesBeforeTheLastRuleRuns(t *testing.T) {
	dir := tree(t, 40)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	res := runIn(ctx, `first(invoke_pwrgrep("`+dir+`"; "python")) | .RuleId`)
	if res.Error != "" {
		t.Fatalf("no finding arrived before the deadline, which is what a "+
			"buffered search would do: %s", res.Error)
	}
	if res.Count != 1 {
		t.Fatalf("expected one finding, got %d", res.Count)
	}
}
