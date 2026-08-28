package integration

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestVariablesPersistAcrossCmdletCalls covers the one place state survives
// between cmdlets in a single query. jq itself has no mutable state, so if
// session state were not shared, every cmdlet would see an empty store and
// get_variable would always fail.
func TestVariablesPersistAcrossCmdletCalls(t *testing.T) {
	got := strings.TrimSpace(mustRun(t, "null", "-c",
		`set_variable("x"; 42) | get_variable("x"; {ValueOnly: true})`))
	if got != "42" {
		t.Errorf("got %s, want 42", got)
	}

	// Setting again replaces rather than accumulating.
	got = strings.TrimSpace(mustRun(t, "null", "-c",
		`set_variable("x"; 1) | set_variable("x"; 2) | get_variable("x"; {ValueOnly: true})`))
	if got != "2" {
		t.Errorf("got %s, want 2", got)
	}

	// A value keeps its JSON type; it is not stringified on the way through.
	got = strings.TrimSpace(mustRun(t, "null", "-c",
		`set_variable("o"; {a: [1, 2]}) | get_variable("o"; {ValueOnly: true})`))
	if got != `{"a":[1,2]}` {
		t.Errorf("got %s, want {\"a\":[1,2]}", got)
	}
}

// TestGetVariableReportsMetadata checks the full PSVariable shape, which is
// what distinguishes get_variable from just reading a jq binding.
func TestGetVariableReportsMetadata(t *testing.T) {
	out := mustRun(t, "null", "-c", `set_variable("x"; 42) | get_variable("x")`)

	var v map[string]any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("get_variable did not return an object: %v (%q)", err, out)
	}
	for key, want := range map[string]any{
		"Name":     "x",
		"Value":    float64(42),
		"Scope":    "Global",
		"PwrqType": "Pwrq.Variable",
	} {
		if v[key] != want {
			t.Errorf("%s = %v, want %v", key, v[key], want)
		}
	}
}

// TestRemoveVariable checks that removal actually removes: the following read
// has to fail rather than return a stale value.
func TestRemoveVariable(t *testing.T) {
	got := strings.TrimSpace(mustRun(t, "null", "-c",
		`set_variable("x"; 42)
		 | remove_variable("x")
		 | try get_variable("x"; {ValueOnly: true}) catch "gone"`))
	if got != `"gone"` {
		t.Errorf("reading a removed variable gave %s, want \"gone\"", got)
	}
}

// TestLocationStack covers Push-Location/Pop-Location as a stack: the point of
// pushing is that the pop returns you exactly where you were, however deep.
func TestLocationStack(t *testing.T) {
	start, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	a, b := t.TempDir(), t.TempDir()

	out := mustRun(t, "null", "-c",
		`[ get_location.Path
		 , (push_location("`+a+`") | get_location.Path)
		 , (push_location("`+b+`") | get_location.Path)
		 , (pop_location | get_location.Path)
		 , (pop_location | get_location.Path)
		 ]`)

	var paths []string
	if err := json.Unmarshal([]byte(out), &paths); err != nil {
		t.Fatalf("%v (%q)", err, out)
	}
	want := []string{start, a, b, a, start}
	for i := range want {
		// Compare resolved paths: t.TempDir hands back /tmp/... which is a
		// symlink to /private/tmp on some systems.
		if !samePath(t, paths[i], want[i]) {
			t.Errorf("step %d: at %s, want %s", i, paths[i], want[i])
		}
	}
}

// TestSetLocationChangesWhereCmdletsLook is what makes the location stack more
// than bookkeeping: a relative path handed to another cmdlet resolves against
// it.
func TestSetLocationChangesWhereCmdletsLook(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "marker.txt", "found")

	got := strings.TrimSpace(mustRun(t, "null", "-r",
		`set_location("`+dir+`") | cat("marker.txt")`))
	if got != "found" {
		t.Errorf("got %q, want \"found\"", got)
	}
}

// samePath reports whether two paths name the same directory after resolving
// symlinks.
func samePath(t *testing.T, a, b string) bool {
	t.Helper()
	fa, err := os.Stat(a)
	if err != nil {
		return false
	}
	fb, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(fa, fb)
}
