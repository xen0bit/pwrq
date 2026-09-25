package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/itchyny/gojq"
)

func runCLI(args ...string) (string, int) {
	var out, errOut bytes.Buffer
	code := (&cli{inStream: strings.NewReader(""), outStream: &out, errStream: &errOut}).run(args)
	return out.String() + errOut.String(), code
}

func TestEmitNeedsTUI(t *testing.T) {
	out, code := runCLI("--emit=output", ".")
	if code == 0 || !strings.Contains(out, "--tui") {
		t.Errorf("--emit without --tui should say what it needs, got %d: %q", code, out)
	}
}

func TestTUIRejectsAnUnknownEmit(t *testing.T) {
	out, code := runCLI("--tui", "--emit=everything", ".")
	if code == 0 || !strings.Contains(out, "query or output") {
		t.Errorf("got %d: %q", code, out)
	}
}

// TestNamedArgsBecomeJSON: every way the command line binds a variable
// arrives in the Args pane as the JSON value jq would have bound.
func TestNamedArgsBecomeJSON(t *testing.T) {
	opts := &flagopts{
		Arg:     map[string]string{"s": `a "quoted" string`},
		ArgJSON: map[string]string{"j": `{"n": [1, 2]}`},
		RawFile: map[string]string{"r": "testdata/1.json"},
	}
	args, err := namedArgs(opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 3 {
		t.Fatalf("args = %+v", args)
	}
	got := map[string]string{}
	for _, arg := range args {
		got[arg.Name] = arg.Value
	}
	if got["s"] != `"a \"quoted\" string"` {
		t.Errorf("--arg = %s", got["s"])
	}
	if got["j"] != `{"n":[1,2]}` {
		t.Errorf("--argjson = %s", got["j"])
	}
	if !strings.HasPrefix(got["r"], `"`) {
		t.Errorf("--rawfile should bind the file as a string, got %s", got["r"])
	}
	if args[0].Name > args[1].Name || args[1].Name > args[2].Name {
		t.Errorf("args should be in a stable order: %+v", args)
	}

	if _, err := namedArgs(&flagopts{ArgJSON: map[string]string{"bad": "{"}}); err == nil {
		t.Error("an --argjson that is not JSON should be refused")
	}
}

// TestYAMLBecomesJSONLines: the TUI's engine reads JSON, so --yaml-input is
// read here and handed over as the values it decodes to.
func TestYAMLBecomesJSONLines(t *testing.T) {
	text, err := toJSONLines(newYAMLInputIter(strings.NewReader("a: 1\nb: [x, y]\n---\n- 3\n"), "test"))
	if err != nil {
		t.Fatal(err)
	}
	if text != "{\"a\":1,\"b\":[\"x\",\"y\"]}\n[3]\n" {
		t.Errorf("text = %q", text)
	}
}

func TestPositionalArgs(t *testing.T) {
	opts := &flagopts{Args: []any{"a", "b"}, JSONArgs: []any{nil, `{"x":1}`}}
	got, err := gojq.Marshal(positionalArgs(opts))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `["a",{"x":1}]` {
		t.Errorf("got %s", got)
	}
}
