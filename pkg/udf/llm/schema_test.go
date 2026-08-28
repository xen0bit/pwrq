package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

const personSchema = `{Schema: {type: "object", properties: {name: {type: "string"}, age: {type: "integer"}}, required: ["name", "age"]}}`

func TestSchemaReturnsTheDecodedValue(t *testing.T) {
	newServer(t, openAIReply(`{"name":"Ada","age":36}`))

	got, err := run(t, `invoke_llm("extract"; `+personSchema+`)`, nil)
	if err != nil {
		t.Fatal(err)
	}
	obj, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("got %T, want an object; under a Schema the answer is the value, not the text it arrived as", got)
	}
	if obj["name"] != "Ada" {
		t.Errorf("name = %v", obj["name"])
	}
}

// TestSchemaReachesBothDialects pins how each provider is asked for typed
// output: OpenAI has response_format, the Messages API has no such thing and
// gets a forced tool call instead.
func TestSchemaReachesTheOpenAIRequest(t *testing.T) {
	s := newServer(t, openAIReply(`{"name":"Ada","age":36}`))

	if _, err := run(t, `invoke_llm("extract"; `+personSchema+`)`, nil); err != nil {
		t.Fatal(err)
	}
	format, _ := s.request(0)["response_format"].(map[string]any)
	if format["type"] != "json_schema" {
		t.Fatalf("response_format = %v", s.request(0)["response_format"])
	}
}

func TestSchemaBecomesAForcedToolOnAnthropic(t *testing.T) {
	s := newServer(t, anthropicToolReply(map[string]any{"name": "Ada", "age": 36}))
	t.Setenv(EnvModel, "anthropic/claude-test")

	got, err := run(t, `invoke_llm("extract"; `+personSchema+`)`, nil)
	if err != nil {
		t.Fatal(err)
	}
	obj, _ := got.(map[string]any)
	if obj["name"] != "Ada" {
		t.Fatalf("got %v, want the tool input decoded as the answer", got)
	}

	req := s.request(0)
	choice, _ := req["tool_choice"].(map[string]any)
	if choice["name"] != structuredToolName {
		t.Errorf("tool_choice = %v; the call must be forced through the tool or the model may answer in prose", req["tool_choice"])
	}
}

// TestSchemaRepairsOnce pins the bounded repair: the validation error goes back
// to the model, and a second answer that satisfies the schema is accepted.
func TestSchemaRepairsOnce(t *testing.T) {
	s := newServer(t, openAIReply(`{"name":"Ada"}`), openAIReply(`{"name":"Ada","age":36}`))

	got, err := run(t, `invoke_llm("extract"; `+personSchema+`)`, nil)
	if err != nil {
		t.Fatal(err)
	}
	obj, _ := got.(map[string]any)
	if obj["age"] != float64(36) {
		t.Errorf("got %v", got)
	}
	if s.count() != 2 {
		t.Errorf("made %d requests, want a repair round", s.count())
	}

	// The repair has to carry the reason, or the model is being asked to guess
	// what was wrong.
	messages, _ := s.request(1)["messages"].([]any)
	last, _ := messages[len(messages)-1].(map[string]any)
	if text, _ := last["content"].(string); !strings.Contains(text, "schema") {
		t.Errorf("the repair message does not say what failed: %q", text)
	}
}

func TestSchemaFailureIsAnErrorNotAGuess(t *testing.T) {
	s := newServer(t, openAIReply(`{"name":"Ada"}`))

	_, err := run(t, `invoke_llm("extract"; `+personSchema+`)`, nil)
	if err == nil {
		t.Fatal("an answer missing a required field was accepted; a pipeline grouping by that field would fail somewhere else entirely")
	}
	if s.count() != 2 {
		t.Errorf("made %d requests, want the one repair round the default allows", s.count())
	}
}

func TestRepairCanBeTurnedOff(t *testing.T) {
	s := newServer(t, openAIReply(`{"name":"Ada"}`))

	if _, err := run(t, `invoke_llm("extract"; {Repair: 0, Schema: {type: "object", required: ["age"]}})`, nil); err == nil {
		t.Fatal("want a failure")
	}
	if s.count() != 1 {
		t.Errorf("made %d requests, want 1 with Repair: 0", s.count())
	}
}

func TestNonJSONUnderSchemaIsAnError(t *testing.T) {
	newServer(t, openAIReply("I am afraid I cannot do that"))

	_, err := run(t, `invoke_llm("extract"; {Repair: 0, Schema: {type: "object"}})`, nil)
	if err == nil || !strings.Contains(err.Error(), "did not return JSON") {
		t.Fatalf("want a clear error, got %v", err)
	}
}

func TestInvalidSchemaIsReportedBeforeTheCall(t *testing.T) {
	s := newServer(t, openAIReply("ok"))

	if _, err := run(t, `invoke_llm("hi"; {Schema: {type: 42}})`, nil); err == nil {
		t.Fatal("an unusable schema was accepted")
	}
	if s.count() != 0 {
		t.Error("a request was made with a schema that could never validate anything")
	}
}

// TestFenceIsStripped covers the practical case: models wrap JSON in markdown
// however firmly they are told not to, and a fence is never valid JSON, so
// unwrapping cannot be ambiguous.
func TestFenceIsStripped(t *testing.T) {
	newServer(t, openAIReply("```json\n{\"name\":\"Ada\",\"age\":36}\n```"))

	got, err := run(t, `invoke_llm("extract"; `+personSchema+`)`, nil)
	if err != nil {
		t.Fatal(err)
	}
	obj, _ := got.(map[string]any)
	if obj["name"] != "Ada" {
		t.Errorf("got %v", got)
	}
}

func TestStripFence(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"{\"a\":1}", `{"a":1}`},
		{"```json\n{\"a\":1}\n```", `{"a":1}`},
		{"```\n{\"a\":1}\n```", `{"a":1}`},
		{"  {\"a\":1}  ", `{"a":1}`},
	} {
		if got := stripFence(c.in); got != c.want {
			t.Errorf("stripFence(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestRequestObjectCarriesTheCall checks the object producer: the same call
// reported with everything the transform drops.
func TestRequestObjectCarriesTheCall(t *testing.T) {
	newServer(t, openAIReply("hello"))

	got, err := run(t, `invoke_llm_request("hi"; {PriceInput: 3, PriceOutput: 15})`, nil)
	if err != nil {
		t.Fatal(err)
	}
	obj, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("got %T, want an object", got)
	}
	for _, key := range []string{"Content", "Model", "Provider", "StopReason", "InputTokens", "OutputTokens", "TotalTokens", "Cost", "Cached", "PwrqType"} {
		if _, has := obj[key]; !has {
			t.Errorf("the response object has no %s", key)
		}
	}
	if obj["Content"] != "hello" {
		t.Errorf("Content = %v", obj["Content"])
	}
	// 11 input tokens at $3/M plus 7 output at $15/M.
	want := 11*3.0/1e6 + 7*15.0/1e6
	if cost, _ := toFloat(obj["Cost"]); cost != want {
		t.Errorf("Cost = %v, want %v", obj["Cost"], want)
	}
}

// TestCostIsNullWithoutPrices pins the decision not to compile a price table
// into the binary. A confident wrong number is worse than an honest null.
func TestCostIsNullWithoutPrices(t *testing.T) {
	newServer(t, openAIReply("hello"))

	got, err := run(t, `invoke_llm_request("hi")`, nil)
	if err != nil {
		t.Fatal(err)
	}
	obj, _ := got.(map[string]any)
	if obj["Cost"] != nil {
		t.Errorf("Cost = %v, want null when no prices were supplied", obj["Cost"])
	}
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}
