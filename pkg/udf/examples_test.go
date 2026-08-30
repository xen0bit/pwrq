package udf

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// The catalogue publishes an example for every one of its cmdlets, and until
// this file existed nothing checked that any of them worked.
//
// Run against a live server, 100 of the 652 could not. `e.g. md5` was the
// whole example for md5, and running it says "argument must be a string, got
// <nil>". `e.g. aes_encrypt("data"; "key")` says "invalid key size 3 bytes".
// `e.g. base64_encode(true)` says "file argument requires string path". The
// table was inconsistent with itself about it, too: crc32's example was
// `"hello" | crc32` and md5's was `md5`, for two cmdlets of the same shape.
//
// An example is the shortest path from "what does this do" to a working query,
// and an agent reading the catalogue copies it verbatim. One that cannot run
// is worse than none: it costs a call, and it teaches a call form that does
// not work.

// fixtures are the files the examples name, so that an example demonstrating a
// cmdlet on a file can be run rather than exempted. Written into a temporary
// directory the test runs in, so nothing here touches the working tree.
//
// The mapping is deliberately literal: the key is the path the example writes.
// A new example naming a file nothing creates fails with "no such file", which
// is the right failure - it says exactly which line to fix and which fixture
// to add.
var fixtures = map[string]string{
	"a.txt":        "alpha\nbravo\ncharlie\n",
	"b.txt":        "alpha\ndelta\ncharlie\n",
	"c.txt":        "charlie\n",
	"file.txt":     "the quick brown fox\njumps over the lazy dog\n",
	"notes.txt":    "TODO: write the notes\n",
	"report.txt":   "a report\n",
	"app.log":      "level=info msg=started\nlevel=warn msg=slow\nlevel=error msg=failed\n",
	"out.log":      "an existing line\n",
	"run.log":      "an existing line\n",
	"events.jsonl": "{\"a\":1}\n{\"a\":2}\n",
	"a.bin":        "\x00\x01\x02\x03binary sample\n",
	"draft.txt":    "a draft\n",
	"b.bin":        "\x00\x01\x02\x04binary sample\n",
	"known.bin":    "a known sample\n",
	"suspect.bin":  "a suspect sample\n",
	"src/one.txt":  "one\n",
	// select_ast's examples search src, so it needs something with syntax in it.
	"src/app.go": "package app\n\nimport \"fmt\"\n\nfunc load(name string) error {\n\t" +
		"if name == \"\" {\n\t\treturn fmt.Errorf(\"empty\")\n\t}\n\treturn nil\n}\n",
	"src/two.txt":   "two\n",
	"samples/a.bin": "sample a\n",
	"samples/b.bin": "sample b\n",
}

// builtFixtures are the fixtures that are easier to make with pwrq than by
// hand: an archive, and a database with the table the examples select from.
var builtFixtures = []string{
	// ssdeep refuses anything under 4096 bytes, and the examples for it used
	// to be written against "hello".
	`[range(512) | "0123456789abcdef"] | join("") | out_file("firmware.bin")`,
	`[range(512) | "0123456789abcdef"] | join("") + "tail" | out_file("firmware2.bin")`,
	`compress_archive("src"; "release.zip")`,
	`compress_archive("src"; "backup.tar.gz")`,
	`[{id: 1, email: "a@b.c", name: "alice"}, {id: 2, email: "d@e.f", name: "bob"}] | out_sqlite("app.db"; "users")`,
}

// unrunnable names the cmdlets whose examples cannot be run here, with the
// reason each one cannot.
//
// The list is the honest boundary of the check rather than a place to put
// anything inconvenient: every entry is a cmdlet whose example needs something
// a test has no business having - a network, a credential, or the right to
// change the machine it runs on. An example that merely needed an input or a
// valid key was fixed instead, which is why this list is short and the fixtures
// above exist at all.
var unrunnable = map[string]string{
	// Reaches the network.
	"invoke_web_request": "makes an HTTP request",
	"invoke_rest_method": "makes an HTTP request",
	"http":               "makes an HTTP request",
	"http_serve":         "binds a port and serves until stopped",
	"test_connection":    "pings a host",
	"resolve_host":       "queries DNS",
	"reverse_dns":        "queries DNS",

	// Needs a credential this test does not have, and must not acquire.
	"invoke_llm":                      "calls a language model provider",
	"invoke_llm_request":              "calls a language model provider",
	"invoke_llm_batch":                "calls a language model provider",
	"invoke_agent":                    "calls a language model provider",
	"invoke_agent_request":            "calls a language model provider",
	"invoke_embedding":                "calls a language model provider",
	"get_llm_model":                   "calls a language model provider",
	"get_censys_context":              "calls the Censys API",
	"get_censys_host":                 "calls the Censys API",
	"get_censys_certificate":          "calls the Censys API",
	"get_censys_webproperty":          "calls the Censys API",
	"get_censys_enrichment":           "calls the Censys API",
	"get_censys_host_timeline":        "calls the Censys API",
	"get_censys_webproperty_timeline": "calls the Censys API",
	"get_censys_host_service":         "calls the Censys API",
	"search_censys":                   "calls the Censys API",
	"get_censys_aggregate":            "calls the Censys API",
	"get_censys_collection":           "calls the Censys API",
	"new_censys_collection":           "calls the Censys API",
	"set_censys_collection":           "calls the Censys API",
	"remove_censys_collection":        "calls the Censys API",
	"get_censys_collection_event":     "calls the Censys API",
	"new_censys_censeye_job":          "calls the Censys API",
	"get_censys_censeye_job":          "calls the Censys API",
	"get_censys_censeye_result":       "calls the Censys API",
	"get_censys_threat":               "calls the Censys API",
	"get_censys_tag":                  "calls the Censys API",
	"new_censys_tag":                  "calls the Censys API",
	"set_censys_tag":                  "calls the Censys API",
	"remove_censys_tag":               "calls the Censys API",
	"get_censys_tag_assignment":       "calls the Censys API",
	"add_censys_tag_assignment":       "calls the Censys API",
	"remove_censys_tag_assignment":    "calls the Censys API",
	"get_censys_organization":         "calls the Censys API",
	"get_censys_credits":              "calls the Censys API",

	// Changes the machine the test is running on.
	"sh":            "runs an arbitrary shell command",
	"start_process": "starts a process",
	"stop_process":  "kills a process",
	"start_service": "starts a system service",
	"stop_service":  "stops a system service",
	"set_date":      "sets the system clock",
	"rm":            "deletes a path outside the test's directory",
	"mkdir":         "creates a directory outside the test's directory",
	"new_item":      "creates a path outside the test's directory",
	"set_location":  "changes the process working directory the fixtures depend on",
	"push_location": "changes the process working directory the fixtures depend on",
	"pop_location":  "changes the process working directory the fixtures depend on",

	// Reads state a previous call was meant to leave behind, and the example
	// is written as the second half of that pair.
	"get_variable":    "reads a variable an earlier call sets",
	"remove_variable": "removes a variable an earlier call sets",
}

// placeholderExamples are the examples written against a $name the reader is
// expected to bind, which is a documented convention of the catalogue rather
// than a mistake: `get_censys_collection($uid)` says what the argument is
// better than any literal would.
func isPlaceholderExample(example string) bool {
	return strings.Contains(example, "$")
}

// TestEveryPublishedExampleRuns is the check the catalogue never had.
func TestEveryPublishedExampleRuns(t *testing.T) {
	options := DefaultRegistry().Options()
	inFixtureDir(t)

	for _, meta := range GetFunctionMetadata() {
		if unrunnable[meta.Name] != "" {
			continue
		}
		for _, example := range meta.Examples {
			if isPlaceholderExample(example) {
				continue
			}
			t.Run(meta.Name+": "+example, func(t *testing.T) {
				if err := runExample(options, example); err != nil {
					t.Errorf("the published example for %s does not run: %v\n    e.g. %s",
						meta.Name, err, example)
				}
			})
		}
	}
}

// runExample evaluates one example and drains it, reporting the first error.
// Draining matters: a streaming cmdlet reports a bad call on the value that
// fails, which may not be the first one.
func runExample(options []gojq.CompilerOption, example string) error {
	query, err := gojq.Parse(example)
	if err != nil {
		return err
	}
	code, err := gojq.Compile(query, options...)
	if err != nil {
		return err
	}
	iter := code.Run(nil)
	for {
		v, ok := iter.Next()
		if !ok {
			return nil
		}
		if err, isErr := v.(error); isErr {
			return err
		}
	}
}

// inFixtureDir moves the test into a directory holding the files the examples
// name, and leaves it there for the duration.
func inFixtureDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()

	paths := make([]string, 0, len(fixtures))
	for path := range fixtures {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("preparing fixture %s: %v", path, err)
		}
		if err := os.WriteFile(full, []byte(fixtures[path]), 0o644); err != nil {
			t.Fatalf("writing fixture %s: %v", path, err)
		}
	}

	t.Chdir(dir)

	// The cmdlets that read and write pwrq variables reach a session through a
	// package global, which nothing installs outside a real run.
	common.SetGlobalSessionState(sessionstate.NewSessionState())

	options := DefaultRegistry().Options()
	for _, build := range builtFixtures {
		if err := runExample(options, build); err != nil {
			t.Fatalf("building fixture %q: %v", build, err)
		}
	}
}

// TestEveryExemptionIsACmdlet keeps the exemption list from outliving what it
// exempts. A renamed cmdlet would otherwise leave its name here forever,
// quietly excusing nothing while its real examples went unchecked.
func TestEveryExemptionIsACmdlet(t *testing.T) {
	known := make(map[string]bool)
	for _, meta := range GetFunctionMetadata() {
		known[meta.Name] = true
	}
	for name := range unrunnable {
		if !known[name] {
			t.Errorf("unrunnable exempts %q, which is not a cmdlet", name)
		}
	}
}

// TestEveryCmdletHasAnExample pins what is already true, so that a cmdlet
// added without one is caught at the point it is added rather than by whoever
// searches for it later and finds a name and a sentence.
func TestEveryCmdletHasAnExample(t *testing.T) {
	for _, meta := range GetFunctionMetadata() {
		if len(meta.Examples) == 0 {
			t.Errorf("%s publishes no example", meta.Name)
		}
	}
}
