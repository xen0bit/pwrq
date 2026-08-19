package llm

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/queryrun"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// step is one scripted turn of the agent protocol, encoded the way a provider
// would deliver it.
func step(thought, action, content string) string {
	encoded, _ := json.Marshal(map[string]any{"thought": thought, "action": action, "content": content})
	return openAIReply(string(encoded))
}

// installVocabulary gives the agent a small, known vocabulary.
//
// The real one comes from pkg/udf, which imports this package and so cannot be
// imported back — the same reason SetVocabulary is a hook. What matters here is
// the loop and the restriction; that the restriction is built from the real
// registry is pkg/udf's own test.
func installVocabulary(t *testing.T, names ...string) {
	t.Helper()
	available := map[string]gojq.CompilerOption{
		"test_rows": common.WithFunction("test_rows", 0, 0, func(any, []any) any {
			return []any{map[string]any{"Name": "a"}, map[string]any{"Name": "b"}}
		}),
		"test_shout": common.WithFunction("test_shout", 0, 0, func(v any, _ []any) any {
			s, _ := v.(string)
			return strings.ToUpper(s)
		}),
	}

	SetVocabulary(func(allow []string) (*Vocabulary, error) {
		var options []gojq.CompilerOption
		var commands []CommandDoc
		for _, name := range allow {
			option, ok := available[name]
			if !ok {
				return nil, fmt.Errorf("no such cmdlet: %s", name)
			}
			options = append(options, option)
			commands = append(commands, CommandDoc{Name: name, Description: "a test cmdlet"})
		}
		return &Vocabulary{Runner: &queryrun.Runner{Options: options}, Commands: commands, Options: options}, nil
	})
	t.Cleanup(func() { SetVocabulary(nil) })
}

func TestAgentAnswersAfterAQuery(t *testing.T) {
	newServer(t,
		step("count them", "query", "test_rows | length"),
		step("that is the answer", "answer", "2"),
	)
	installVocabulary(t, "test_rows")

	got, err := run(t, `invoke_agent("how many rows?"; {Allow: ["test_rows"]})`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "2" {
		t.Errorf("got %v, want the model's answer", got)
	}
}

// TestAgentTraceIsAuditable is the point of invoke_agent_request: an agent that
// answered from a query has made a claim, and .Steps is where it is checked.
func TestAgentTraceIsAuditable(t *testing.T) {
	newServer(t,
		step("count them", "query", "test_rows | length"),
		step("done", "answer", "2"),
	)
	installVocabulary(t, "test_rows")

	got, err := run(t, `invoke_agent_request("how many rows?"; {Allow: ["test_rows"]})`, nil)
	if err != nil {
		t.Fatal(err)
	}
	obj, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("got %T, want an object", got)
	}
	steps, _ := obj["Steps"].([]any)
	if len(steps) != 2 {
		t.Fatalf("got %d steps, want the query and the answer", len(steps))
	}
	first, _ := steps[0].(map[string]any)
	if first["Query"] != "test_rows | length" {
		t.Errorf("the trace does not carry the query: %v", first["Query"])
	}
	if first["Output"] != "2" {
		t.Errorf("the trace does not carry what the query returned: %v", first["Output"])
	}
	if obj["Content"] != "2" {
		t.Errorf("Content = %v", obj["Content"])
	}
	if obj["TotalTokens"] == nil {
		t.Error("the run reports no token usage")
	}
}

// TestAgentAllowlistIsStructural is the security property. A denied cmdlet is
// not blocked by a check the model can argue with; it does not exist to the
// compiler.
func TestAgentAllowlistIsStructural(t *testing.T) {
	newServer(t,
		step("shout it", "query", `"hi" | test_shout`),
		step("giving up", "answer", "cannot"),
	)
	installVocabulary(t, "test_rows", "test_shout")

	got, err := run(t, `invoke_agent_request("shout"; {Allow: ["test_rows"]})`, nil)
	if err != nil {
		t.Fatal(err)
	}
	obj, _ := got.(map[string]any)
	steps, _ := obj["Steps"].([]any)
	first, _ := steps[0].(map[string]any)
	failure, _ := first["Error"].(string)
	if failure == "" {
		t.Fatal("a cmdlet outside the allowlist ran")
	}
	if !strings.Contains(failure, "test_shout") {
		t.Errorf("the failure should name the cmdlet that is not available: %q", failure)
	}
}

func TestAgentRefusesModelCmdletsInAllow(t *testing.T) {
	newServer(t, step("x", "answer", "y"))
	installVocabulary(t, "test_rows")

	for _, name := range []string{"invoke_llm", "invoke_agent", "get_llm_usage"} {
		_, err := run(t, fmt.Sprintf(`invoke_agent("x"; {Allow: [%q]})`, name), nil)
		if err == nil {
			t.Errorf("%s was allowed into an agent's vocabulary", name)
		}
	}
}

func TestAgentStepLimitIsReported(t *testing.T) {
	newServer(t, step("again", "query", "test_rows | length"))
	installVocabulary(t, "test_rows")

	_, err := run(t, `invoke_agent("loop"; {Allow: ["test_rows"], MaxSteps: 3})`, nil)
	if err == nil {
		t.Fatal("an agent that never answered was treated as success")
	}
	if !strings.Contains(err.Error(), "MaxSteps") {
		t.Errorf("the error should say how to give it more room: %v", err)
	}
}

// TestAgentSeesThePipelineInput pins what makes this a pipeline cmdlet rather
// than a chatbot: `$rows | invoke_agent("...")` puts $rows under `.`.
func TestAgentSeesThePipelineInput(t *testing.T) {
	newServer(t,
		step("look at the input", "query", "[.[] | .Name] | join(\",\")"),
		step("done", "answer", "x,y"),
	)
	installVocabulary(t, "test_rows")

	got, err := run(t, `invoke_agent_request("what names?"; {Allow: ["test_rows"]})`,
		[]any{map[string]any{"Name": "x"}, map[string]any{"Name": "y"}})
	if err != nil {
		t.Fatal(err)
	}
	obj, _ := got.(map[string]any)
	steps, _ := obj["Steps"].([]any)
	first, _ := steps[0].(map[string]any)
	if first["Output"] != `"x,y"` {
		t.Errorf("the query did not see the pipeline input: %v", first["Output"])
	}
}

func TestAgentWithoutInputRunsAgainstNull(t *testing.T) {
	newServer(t,
		step("check", "query", "."),
		step("done", "answer", "null"),
	)
	installVocabulary(t, "test_rows")

	got, err := run(t, `"what is the input?" | invoke_agent_request({Allow: ["test_rows"]})`, nil)
	if err != nil {
		t.Fatal(err)
	}
	obj, _ := got.(map[string]any)
	steps, _ := obj["Steps"].([]any)
	first, _ := steps[0].(map[string]any)
	if first["Output"] != "null" {
		t.Errorf("Output = %v, want null", first["Output"])
	}
}

// TestAgentSchemaTypesTheFinalAnswer covers the extra call finishAgent makes:
// a caller's Schema cannot ride on the step schema, because a step that is
// running a query has no answer to satisfy it with.
func TestAgentSchemaTypesTheFinalAnswer(t *testing.T) {
	s := newServer(t,
		step("count", "query", "test_rows | length"),
		step("done", "answer", "there are 2 rows"),
		openAIReply(`{"count":2}`),
	)
	installVocabulary(t, "test_rows")

	got, err := run(t, `invoke_agent("how many?"; {Allow: ["test_rows"], Schema: {type: "object", properties: {count: {type: "integer"}}, required: ["count"]}})`, nil)
	if err != nil {
		t.Fatal(err)
	}
	obj, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("got %T, want the decoded object", got)
	}
	if obj["count"] != float64(2) {
		t.Errorf("count = %v", obj["count"])
	}
	if s.count() != 3 {
		t.Errorf("made %d calls, want the two steps plus the typing call", s.count())
	}
}

func TestAgentNudgesOnEmptyContent(t *testing.T) {
	newServer(t,
		step("thinking", "query", ""),
		step("now", "answer", "done"),
	)
	installVocabulary(t, "test_rows")

	got, err := run(t, `invoke_agent("x"; {Allow: ["test_rows"], MaxSteps: 3})`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "done" {
		t.Errorf("got %v", got)
	}
}

func TestAgentNeedsAVocabulary(t *testing.T) {
	newServer(t, step("x", "answer", "y"))
	SetVocabulary(nil)

	if _, err := run(t, `invoke_agent("x")`, nil); err == nil {
		t.Fatal("an agent ran with no vocabulary installed")
	}
}

// TestStripQueryFence covers what small models actually emit around a query.
func TestStripQueryFence(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`[get_childitem(".")] | length`, `[get_childitem(".")] | length`},
		{"```jq\n[get_childitem(\".\")] | length\n```", `[get_childitem(".")] | length`},
		{"[get_childitem(\".\")] | length}```\u200b {", `[get_childitem(".")] | length`},
		{"[get_childitem(\".\")] | length\u200b", `[get_childitem(".")] | length`},
	} {
		if got := stripQueryFence(c.in); got != c.want {
			t.Errorf("stripQueryFence(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTrimUnmatchedClosers(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`test_rows | length`, `test_rows | length`},
		{`test_rows | length}`, `test_rows | length`},
		{`test_rows | length)]}`, `test_rows | length`},
		{`{a: 1}`, `{a: 1}`},
		{`"a}"`, `"a}"`},
		// An unmatched closer that is not trailing is a mistake this cannot
		// repair without guessing, so it is left alone for the parser to
		// report.
		{`[test_rows]} | length`, `[test_rows]} | length`},
	} {
		if got := trimUnmatchedClosers(c.in); got != c.want {
			t.Errorf("trimUnmatchedClosers(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestDescribeInputSummarisesRatherThanReproduces pins the fix for what a
// small model does with data it has been handed: gemma-4-e2b inlined a
// six-element array into its query as a literal, every step, rather than
// writing `.`. The prompt now says what shape the data is, not what it says.
func TestDescribeInputSummarisesRatherThanReproduces(t *testing.T) {
	rows := `[{"File":"a.log","Line":2,"Text":"a very distinctive string"},{"File":"b.log","Line":9,"Text":"another"}]`

	got := describeInput(rows)
	if !strings.Contains(got, "2 element") {
		t.Errorf("the description does not say how many elements there are: %q", got)
	}
	for _, field := range []string{"File", "Line", "Text"} {
		if !strings.Contains(got, field) {
			t.Errorf("the description does not name the %s field: %q", field, got)
		}
	}
	// One sample is useful; the whole corpus is what gets pasted into a query.
	if strings.Contains(got, "another") {
		t.Errorf("the description reproduced every element rather than a sample: %q", got)
	}
}

func TestDescribeInputHandlesEveryShape(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`{"a":1,"b":2}`, "fields: a, b"},
		{`[1,2,3]`, "3 element"},
		{`"just text"`, "single value"},
		{`not json at all`, "not json"},
		{`[]`, "0 element"},
	} {
		if got := describeInput(c.in); !strings.Contains(got, c.want) {
			t.Errorf("describeInput(%q) = %q, want it to mention %q", c.in, got, c.want)
		}
	}
}

// TestAgentPromptAvoidsPlaceholderSyntax covers a mistake that cost a whole
// run: the prompt said "collect with [...]", and the model wrote `[...]` into
// its query as if it were syntax. A placeholder that looks like code is one a
// model will copy.
func TestAgentPromptAvoidsPlaceholderSyntax(t *testing.T) {
	prompt := agentSystemPrompt(
		[]CommandDoc{{Name: "get_childitem", Description: "list a directory", Streaming: true}},
		"", "")
	if strings.Contains(prompt, "[...]") {
		t.Error("the agent prompt contains [...], which a model will write into a query verbatim")
	}
	if !strings.Contains(prompt, "square brackets") {
		t.Error("the prompt no longer explains how to collect a stream")
	}
}
