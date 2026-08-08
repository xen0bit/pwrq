// Package integration exercises the pwrq binary end to end.
//
// These tests build the real binary and run it, because the behaviour they
// cover only exists once everything is assembled: aliases are compiled into the
// query by the CLI, and the object wire format is only observable in what gets
// printed. The previous version of this file defined a Run() that returned
// ("", "", 0) unconditionally and asserted against "", so every test passed
// without executing anything.
package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var (
	buildOnce  sync.Once
	binaryPath string
	buildErr   error
)

// pwrq builds the binary once per run and returns its path.
func pwrq(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "pwrq-integration-")
		if err != nil {
			buildErr = err
			return
		}
		binaryPath = filepath.Join(dir, "pwrq")
		cmd := exec.Command("go", "build", "-o", binaryPath, "github.com/xen0bit/pwrq/cmd/pwrq")
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = &buildFailure{err, string(out)}
		}
	})
	if buildErr != nil {
		t.Fatalf("building pwrq: %v", buildErr)
	}
	return binaryPath
}

type buildFailure struct {
	err    error
	output string
}

func (b *buildFailure) Error() string { return b.err.Error() + "\n" + b.output }

// run executes a query and returns stdout, stderr and the exit code.
func run(t *testing.T, input string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(pwrq(t), args...)
	cmd.Stdin = strings.NewReader(input)
	cmd.Env = append(os.Environ(), "NO_COLOR=1", "PWRQ_COLORS=", "GOJQ_COLORS=")

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	code := 0
	if err := cmd.Run(); err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("running pwrq %v: %v", args, err)
		}
		code = exitErr.ExitCode()
	}
	return stdout.String(), stderr.String(), code
}

// mustRun executes a query that is expected to succeed.
func mustRun(t *testing.T, input string, args ...string) string {
	t.Helper()
	stdout, stderr, code := run(t, input, args...)
	if code != 0 {
		t.Fatalf("pwrq %v exited %d: %s", args, code, stderr)
	}
	return stdout
}

// TestAliasesResolve covers what the previous stub only claimed to: aliases
// have to be compiled into the query, because gojq binds function names at
// compile time and never consults session state.
func TestAliasesResolve(t *testing.T) {
	cases := []struct{ alias, query string }{
		{"gci", `[gci(".")] | length > 0`},
		{"dir", `[dir(".")] | length > 0`},
		{"gps", `[gps] | length > 0`},
		{"gl", `gl | has("Path")`},
		{"gd", `gd | has("Year")`},
	}
	for _, tc := range cases {
		t.Run(tc.alias, func(t *testing.T) {
			out := mustRun(t, "null", "-c", tc.query)
			if strings.TrimSpace(out) != "true" {
				t.Errorf("%s: got %q, want true", tc.query, strings.TrimSpace(out))
			}
		})
	}
}

// TestAliasesDoNotShadowBuiltins is the property that makes aliases safe to
// ship: they are compiled as jq definitions, and a definition does take
// precedence over a builtin, so a badly chosen alias would silently change what
// existing jq programs mean.
func TestAliasesDoNotShadowBuiltins(t *testing.T) {
	cases := []struct{ query, input, want string }{
		{`split(",")`, `"a,b"`, `["a","b"]`},
		{`join("-")`, `["a","b"]`, `"a-b"`},
		{`sort`, `[3,1,2]`, `[1,2,3]`},
		{`map(select(. > 1))`, `[1,2,3]`, `[2,3]`},
		{`group_by(.)|length`, `[1,1,2]`, `2`},
		{`[limit(2; .[])]`, `[1,2,3]`, `[1,2]`},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			out := strings.TrimSpace(mustRun(t, tc.input, "-c", tc.query))
			if out != tc.want {
				t.Errorf("%s: got %s, want %s", tc.query, out, tc.want)
			}
		})
	}
}

// TestCmdletOutputIsQueryableJSON is the point of the object wire format: a
// cmdlet's output is ordinary JSON, so jq's own filters work on it.
func TestCmdletOutputIsQueryableJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "small.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "large.txt"), []byte(strings.Repeat("x", 5000)), 0644); err != nil {
		t.Fatal(err)
	}

	out := mustRun(t, "null", "-c",
		`[get_childitem("`+dir+`") | select(.Length > 1000) | .Name]`)
	if strings.TrimSpace(out) != `["large.txt"]` {
		t.Errorf("got %s, want [\"large.txt\"]", strings.TrimSpace(out))
	}

	// Properties keep their JSON types rather than being stringified.
	out = mustRun(t, "null", "-c",
		`[get_childitem("`+dir+`") | select(.Name == "small.txt") | .Length]`)
	if strings.TrimSpace(out) != "[2]" {
		t.Errorf("Length should be a number, got %s", strings.TrimSpace(out))
	}

	// The PowerShell type travels with the object.
	out = mustRun(t, "null", "-c",
		`[get_childitem("`+dir+`") | .PSTypeName] | unique`)
	if strings.TrimSpace(out) != `["System.IO.FileInfo"]` {
		t.Errorf("got %s, want [\"System.IO.FileInfo\"]", strings.TrimSpace(out))
	}
}

// TestJqSupersetGuarantee spot-checks the promise the whole design rests on:
// a valid jq program behaves as jq, including for objects whose keys happen to
// collide with what pwrq once used for its own metadata.
func TestJqSupersetGuarantee(t *testing.T) {
	cases := []struct{ input, query, want string }{
		{`{"_val":1,"_meta":{"a":2}}`, ".", `{"_meta":{"a":2},"_val":1}`},
		{`{"_val":"x"}`, ".", `{"_val":"x"}`},
		{`[{"_val":1,"_meta":null}]`, ".", `[{"_meta":null,"_val":1}]`},
		{`{"a":{"b":[1,2]}}`, ".a.b[1]", `2`},
		{`null`, `[limit(3; repeat(1))]`, `[1,1,1]`},
	}
	for _, tc := range cases {
		t.Run(tc.query+" on "+tc.input, func(t *testing.T) {
			out := strings.TrimSpace(mustRun(t, tc.input, "-c", tc.query))
			if out != tc.want {
				t.Errorf("got %s, want %s", out, tc.want)
			}
		})
	}
}

// TestCmdletsChain covers pipeline binding: a cmdlet emitting paths and a
// cmdlet emitting objects both have to feed a path-consuming cmdlet.
func TestCmdletsChain(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "content.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	// find yields strings
	out := mustRun(t, "null", "-c", `[find("`+dir+`"; "file") | cat]`)
	if strings.TrimSpace(out) != `["hello"]` {
		t.Errorf("find | cat: got %s", strings.TrimSpace(out))
	}

	// get_childitem yields objects, bound by property name
	out = mustRun(t, "null", "-c", `[get_childitem("`+dir+`") | cat]`)
	if strings.TrimSpace(out) != `["hello"]` {
		t.Errorf("get_childitem | cat: got %s", strings.TrimSpace(out))
	}

	// transforms compose without any unwrapping
	out = mustRun(t, "null", "-r", `"`+path+`" | cat | sha256`)
	const wantHash = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if strings.TrimSpace(out) != wantHash {
		t.Errorf("cat | sha256: got %s, want %s", strings.TrimSpace(out), wantHash)
	}
}

// TestFailuresUseTheErrorChannel checks that a failing cmdlet errors rather
// than returning a truthy object the pipeline would carry on with.
func TestFailuresUseTheErrorChannel(t *testing.T) {
	_, stderr, code := run(t, "null", "-c", `cat("/nonexistent/path/file.txt")`)
	if code == 0 {
		t.Error("reading a missing file should fail")
	}
	if !strings.Contains(stderr, "nonexistent") {
		t.Errorf("the error should name the path, got %q", stderr)
	}

	// and it is catchable like any other jq error
	out := mustRun(t, "null", "-c", `try cat("/nonexistent/path/file.txt") catch "caught"`)
	if strings.TrimSpace(out) != `"caught"` {
		t.Errorf("got %s, want \"caught\"", strings.TrimSpace(out))
	}
}

// TestScriptBlocksAreJq covers expressions that the previous hand-rolled
// script-block parser could not express.
func TestScriptBlocksAreJq(t *testing.T) {
	const data = `[{"Name":"Alice","Age":30},{"Name":"Bob","Age":25},{"Name":"Carol","Age":35}]`
	cases := []struct{ script, want string }{
		{`.Age > 26 and (.Name | startswith("A"))`, `["Alice"]`},
		{`.Name | test("^[AB]")`, `["Alice","Bob"]`},
		{`(.Age / 5 | floor) == 6`, `["Alice"]`},
	}
	for _, tc := range cases {
		t.Run(tc.script, func(t *testing.T) {
			query := `where_object(.; {script: ` + quoteForJq(tc.script) + `}) | map(.Name)`
			out := strings.TrimSpace(mustRun(t, data, "-c", query))
			if out != tc.want {
				t.Errorf("got %s, want %s", out, tc.want)
			}
		})
	}
}

// quoteForJq renders a Go string as a jq string literal.
func quoteForJq(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// TestUDFListMatchesReality checks the discovery surface a PowerShell user
// reaches for first actually lists what the binary provides.
func TestUDFListMatchesReality(t *testing.T) {
	listing := mustRun(t, "", "--udf-list")

	for _, name := range []string{"get_childitem", "get_process", "get_date", "invoke_web_request"} {
		if !strings.Contains(listing, name) {
			t.Errorf("--udf-list omits %s", name)
		}
	}
	if !strings.Contains(listing, "Aliases:") {
		t.Error("--udf-list should list aliases; they are as callable as anything else")
	}
	// split, join and trim were listed but shadowed by jq builtins, so they
	// never ran. They are gone; jq's own are what you get.
	for _, gone := range []string{"\n  split ", "\n  join_string ", "\n  trim "} {
		if strings.Contains(listing, gone) {
			t.Errorf("--udf-list still advertises %q, which jq provides", strings.TrimSpace(gone))
		}
	}

	// Everything listed must actually run.
	if out := mustRun(t, `"msg"`, "-r", `hmac_sha256("key")`); len(strings.TrimSpace(out)) != 64 {
		t.Errorf("hmac_sha256 is listed but did not produce a hash: %q", out)
	}
}
