package mcpserver

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/xen0bit/pwrq/pkg/udf"
)

// This file guards the run_query engine against the panics that used to kill
// the streamable HTTP transport.
//
// gojq's value space is nil, bool, int, float64, *big.Int, string, []any and
// map[string]any. A value outside it does not merely fail to encode: gojq
// panics on it, both in gojq.Marshal and inside any builtin that inspects it.
// A panic in a streamable HTTP handler goroutine terminates the whole server,
// because the SDK does not recover. So a cmdlet that puts, say, an int32 into
// its result map arms a crash for whichever query touches that field first.
//
// The tests below therefore run each query twice: once as written, and once
// wrapped in `.. | type`, which walks every nested value and asks gojq to
// classify it. The wrapped form is what catches a leaked type - the plain form
// is often encoded without ever inspecting the offending field, so it passes
// while the bug is still there.

// exercise runs a query through the exact engine path the server uses and fails
// the test if anything panics, naming the query that did it.
func exercise(t *testing.T, e *engine, name, query string) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("%s: query %q panicked: %v\n%s", name, query, r, debug.Stack())
		}
	}()
	e.execMu.Lock()
	defer e.execMu.Unlock()
	_ = e.execute(runQueryArgs{
		Query:     query,
		NullInput: true,
		Compact:   true,
		TimeoutMs: 300,
	})
}

// exerciseDeep runs the query with every value it produces passed through
// gojq's `type`, so a value outside gojq's value space is inspected rather than
// merely encoded. `..` recurses into arrays and objects, so a bad type nested
// inside a result map is reached too.
func exerciseDeep(t *testing.T, e *engine, name, query string) {
	t.Helper()
	exercise(t, e, name+" (deep)", fmt.Sprintf("[ (%s) | .. | type ] | length", query))
}

// sandbox points the examples that write files at a scratch directory and
// leaves the repository alone. Several documented examples create, copy and
// move files relative to the working directory.
func sandbox(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
}

// skipExample reports whether a documented example must not be run here.
//
// The examples are documentation, not a test corpus: some of them block, some
// reach the network, and some would act on the machine running the tests.
func skipExample(ex string) bool {
	switch {
	case strings.HasPrefix(ex, "http_serve("):
		// Blocks waiting for an HTTP client; would hang the run.
		return true
	case strings.Contains(ex, "example.com"), strings.Contains(ex, "://"):
		// Reaches the network: makes the suite slow, flaky and offline-hostile.
		return true
	case strings.Contains(ex, "stop_process"), strings.Contains(ex, "start_process"):
		// Would signal or spawn a real process on the machine running the
		// tests. stop_process is covered safely by TestKnownPanicQueries.
		return true
	}
	return false
}

// TestAllExamplesExercise runs every documented cmdlet example through the
// engine, so a newly added cmdlet cannot regress the server's crash-freedom
// without a test failing.
func TestAllExamplesExercise(t *testing.T) {
	sandbox(t)
	e := getEngine()
	count := 0
	for _, meta := range udf.GetFunctionMetadata() {
		for _, ex := range meta.Examples {
			if skipExample(ex) {
				continue
			}
			count++
			exercise(t, e, meta.Name, ex)
			exerciseDeep(t, e, meta.Name, ex)
		}
	}
	t.Logf("exercised %d documented examples without panic", count)
}

// TestKnownPanicQueries pins the exact queries that crashed the server before
// the fixes: each of these once produced a panic that took the process down.
func TestKnownPanicQueries(t *testing.T) {
	sandbox(t)
	e := getEngine()
	queries := []string{
		// ProcessInfo.Handles is int32 and WorkingSet/VirtualMemory are int64.
		// The raw values reached gojq, which panics on all three - the deep
		// form below is what exposes it.
		"get_process",
		"[get_process | .Handles, .WorkingSet, .VirtualMemory]",
		// stop_process read proc["Id"] as float64 when it is an int. Aimed at
		// this process, which stopProcesses refuses to signal, so the argument
		// parsing is exercised and nothing is killed.
		fmt.Sprintf("stop_process(%d)", os.Getpid()),
		// Negative n sliced the line slice out of range.
		`head("/etc/passwd"; -1)`,
		`tail("/etc/passwd"; -1)`,
		// Raw *psobject.PSObject leaked into the pipeline.
		`join_path("/tmp"; "x.txt")`,
		`split_path("/tmp/x.txt")`,
	}
	for _, q := range queries {
		exercise(t, e, "known panic", q)
		exerciseDeep(t, e, "known panic", q)
	}
}

// TestDeeplyNestedOutput pins the other way the server used to die. gojq.Marshal
// and NormalizeJSON both recurse over a value, and a deep enough one exhausts
// the goroutine stack: a fatal runtime error that no recover() can catch, so
// the recover in the run_query handler cannot help. The engine must refuse it
// as a query error instead.
func TestDeeplyNestedOutput(t *testing.T) {
	e := getEngine()
	e.execMu.Lock()
	res := e.execute(runQueryArgs{
		Query:     "reduce range(100000) as $i (null; [.])",
		NullInput: true,
		Compact:   true,
		TimeoutMs: 30000,
	})
	e.execMu.Unlock()

	if res.Error == "" {
		t.Fatal("deeply nested output was accepted; it must be refused")
	}
	if !strings.Contains(res.Error, "nests more than") {
		t.Errorf("error does not name the cause: %s", res.Error)
	}
}

// TestAdversarialQueries throws hostile inputs at the engine: deep nesting,
// pathological numbers, malformed everything. None may panic.
func TestAdversarialQueries(t *testing.T) {
	sandbox(t)
	e := getEngine()
	queries := []string{
		"[range(1000000)]",
		". | recurse",
		"def f: f; f",
		"def f: f | f; f",
		"repeat(1)",
		"empty | empty",
		".a.b.c.d",
		".. | numbers",
		"[limit(100000; repeat({a: 1}))]",
		"reduce range(100000) as $i (null; [.])",
		"1e1000000000",
		"-1e1000000000",
		"infinite - infinite",
		"nan | tostring",
		"1 / 0",
		"0 / 0",
		"[1/0, 0/0, nan, infinite]",
		"get_childitem(\"/\")",
		"sh(\"echo done\")",
		"crc64",
		"ssdeep",
		"file_type",
		"random_string(100000)",
		"random_int(-2147483648; 2147483647)",
		"jwt_decode(\"a.b.c\")",
		"base64_decode(\"!!!\")",
		"hex_decode(\"zz\")",
		"xml_parse(\"<a\")",
		"csv_parse(\",\"; \"a,b\\nc\")",
		"yaml_parse(\"a: [1, 2\")",
		"json_parse(\"{\")",
		"gzip_decompress(\"!!!!\")",
		"zlib_decompress(base64_decode(\"eJxL\"))",
		"aes_decrypt(\"x\"; \"key\")",
		"triple_des_encrypt(\"data\"; \"k\")",
		"chacha20(\"key\")",
		"rc4(\"key\")",
		"xor(\"key\")",
		"bcrypt_hash",
		"bcrypt_verify(\"$2a$10$abcdefghijklmnopqrstuv\")",
		"argon2id_hash(\"salt\"; 4; 64)",
		"template({name: 1})",
		"deep_merge({a: 1}; {a: 2})",
		"unflatten_keys({\"a.b\": 1})",
		"json_pointer(\"/\")",
		"json_pointer(\"\")",
		"get_path(\"a.b[1]\")",
		"geohash_encode(42.6; -5.6; 0)",
		"convert_unit(\"C\"; \"F\")",
		"parse_size(\"1.5 XX\")",
		"duration_between(\"garbage\"; \"also\")",
		"list_timezones(\"\")",
		"hamming_distance(\"a\"; \"abc\")",
		"shared_chunks(\"a\"; \"\")",
		"rncd_compare",
		"where_object(.; {script: \".\"})",
		"where_object(.; {property: \"x\", operator: \"like\", value: \"*\"})",
		"group_object(\"x\")",
		"sort_object(\"x\")",
		"measure_object",
		"format_table",
		"format_list",
		"set_location(\"/nonexistent\")",
		"get_variable(\"*\")",
		"set_variable(\"x\"; null)",
		"new_timespan({\"Hours\": -3})",
		"grep_lines(\"/etc/passwd\"; \"[\")",
		"wc_lines(\"/nonexistent\")",
		"read_archive(\"nope.zip\")",
		"expand_archive(\"nope.zip\"; \"/tmp\")",
		"nanoid(0)",
		"nanoid(-1)",
		"to_base(255; 36)",
		"from_base(\"zzz\"; 36)",
		"factorial(1000)",
		"fibonacci(100000)",
		"is_prime(99999999999999999999)",
		"clamp(1; 10; 20)",
		"ip_add(\"255.255.255.255\"; 1)",
		"in_cidr(\"10.0.0.0/8\"; \"10.0.0.0/7\")",
		"int_to_ip(-1)",
		"punycode_encode(\"xn--\")",
		"wrap_text(\"\"; 0)",
		"indent(\"a\"; -2)",
		"pad_center(\"hi\"; -5)",
		"truncate(\"hello\"; -1)",
		"rot(1; 1e9)",
		"ordinal(-3)",
		"percentage(0; 0)",
		"cagr(0; 100; 2)",
		"monthly_payment(100; 0; 0)",
		"geomean([-1])",
		"harmonic_mean([0])",
		"correlation([1,2]; [1])",
		"weighted_mean([1,2]; [0,0])",
		"trimmed_mean([1,2,3]; 0.5)",
		"percentile([1,2,3]; 200)",
		"top_n([1,2]; 100000)",
		"rotate([1,2]; 100000000)",
		"moving_average([1,2,3]; 100)",
		"ema([1,2,3]; 0)",
		"lag([1,2]; -5)",
		"chunks([1,2,3]; 0)",
		"sample([1,2]; 100)",
		"combinations_count(100; 50)",
		"permutations_count(1000; 999)",
		"zlib_compress | gzip_decompress",
		"base64_encode | base64_decode",
		"http(\"GET\"; \"http://127.0.0.1:1/x\")",
		"invoke_web_request(\"http://127.0.0.1:1/\")",
		"which(\"\")",
		"[get_process | .Name]",
		"[get_service | .Name]",
		"sh(\"echo hi\")",
		"[get_command | .Name] | length",
		"get_help(\"gci\")",
	}

	for _, q := range queries {
		exercise(t, e, "adversarial", q)
		exerciseDeep(t, e, "adversarial", q)
	}
	t.Logf("exercised %d adversarial queries without panic", len(queries))
}
