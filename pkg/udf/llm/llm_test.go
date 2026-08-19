package llm

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/similarity"
)

// Every test here runs against a fake provider. A test that reached a real API
// would be slow, billed, and would fail for reasons that have nothing to do
// with the code — which is the same reason pkg/udf/censys points its SDK at an
// httptest server.

// server is a fake provider. It records what it was asked for, so a test can
// assert on the request as well as the reply: the options a caller passes are
// only meaningful if they reach the wire.
type server struct {
	t        *testing.T
	url      string
	replies  []string
	statuses []int
	headers  []map[string]string

	mu       sync.Mutex
	requests []map[string]any
	// inFlight and peak record concurrency, which is what invoke_llm_batch's
	// Parallel option is for.
	inFlight int32
	peak     int32
	release  chan struct{}
}

// newServer starts a fake provider that answers with replies in order, then
// repeats the last one.
func newServer(t *testing.T, replies ...string) *server {
	t.Helper()
	s := &server{t: t, replies: replies}

	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var decoded map[string]any
		// A listing is a GET with no body, so only a body that exists has to
		// be JSON.
		if len(body) > 0 {
			if err := json.Unmarshal(body, &decoded); err != nil {
				t.Errorf("request body is not JSON: %v", err)
			}
		}

		now := atomic.AddInt32(&s.inFlight, 1)
		for {
			peak := atomic.LoadInt32(&s.peak)
			if now <= peak || atomic.CompareAndSwapInt32(&s.peak, peak, now) {
				break
			}
		}
		defer atomic.AddInt32(&s.inFlight, -1)

		// Recorded before the gate below, so a test waiting for every worker
		// to arrive can see them arrive.
		s.mu.Lock()
		n := len(s.requests)
		s.requests = append(s.requests, decoded)
		s.mu.Unlock()

		// release holds every request until the test lets it go, which is how
		// the concurrency tests observe overlap without timing anything. The
		// deadline matters: a sequential implementation should fail the
		// assertion, not hang until the test binary is killed.
		if s.release != nil {
			select {
			case <-s.release:
			case <-time.After(2 * time.Second):
			}
		}

		if n < len(s.headers) {
			for k, v := range s.headers[n] {
				w.Header().Set(k, v)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if n < len(s.statuses) && s.statuses[n] != 0 {
			w.WriteHeader(s.statuses[n])
		}
		reply := ""
		if len(s.replies) > 0 {
			reply = s.replies[min(n, len(s.replies)-1)]
		}
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(httpSrv.Close)
	s.url = httpSrv.URL

	// Every test resolves its model and credentials from the environment, so
	// pointing the environment at the fake server is also the test for that
	// resolution.
	t.Setenv(EnvModel, "openai/test-model")
	t.Setenv(EnvOpenAIKey, "test-key")
	t.Setenv(EnvOpenAIBase, httpSrv.URL)
	t.Setenv(EnvAnthropicKey, "test-key")
	t.Setenv(EnvAnthropicBase, httpSrv.URL)
	t.Setenv(EnvCache, "")
	t.Setenv(EnvMaxCalls, "0")
	resetUsage()
	return s
}

func (s *server) request(i int) map[string]any {
	s.t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if i >= len(s.requests) {
		s.t.Fatalf("wanted request %d, only %d were made", i, len(s.requests))
	}
	return s.requests[i]
}

func (s *server) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}

// openAIReply is a chat completion carrying content.
func openAIReply(content string) string {
	encoded, _ := json.Marshal(map[string]any{
		"choices": []any{map[string]any{
			"message":       map[string]any{"content": content},
			"finish_reason": "stop",
		}},
		"usage": map[string]any{"prompt_tokens": 11, "completion_tokens": 7},
	})
	return string(encoded)
}

// anthropicReply is a Messages response carrying a text block.
func anthropicReply(content string) string {
	encoded, _ := json.Marshal(map[string]any{
		"content":     []any{map[string]any{"type": "text", "text": content}},
		"stop_reason": "end_turn",
		"usage":       map[string]any{"input_tokens": 11, "output_tokens": 7},
	})
	return string(encoded)
}

// anthropicToolReply is what a schema call comes back as: the answer is the
// input of a forced tool call, not text.
func anthropicToolReply(value any) string {
	encoded, _ := json.Marshal(map[string]any{
		"content": []any{map[string]any{
			"type": "tool_use", "name": structuredToolName, "input": value,
		}},
		"stop_reason": "tool_use",
		"usage":       map[string]any{"input_tokens": 11, "output_tokens": 7},
	})
	return string(encoded)
}

// testOptions is the vocabulary the tests compile against: the LLM cmdlets,
// plus the similarity ones, because ranking embeddings by meaning is a
// pipeline that spans both packages and is worth testing as one.
func testOptions() []gojq.CompilerOption {
	return append(RegisterAll(), similarity.RegisterAll()...)
}

// run evaluates a query against the LLM cmdlets and returns its first result.
func run(t *testing.T, query string, input any) (any, error) {
	t.Helper()
	parsed, err := gojq.Parse(query)
	if err != nil {
		t.Fatalf("parsing %q: %v", query, err)
	}
	code, err := gojq.Compile(parsed, testOptions()...)
	if err != nil {
		t.Fatalf("compiling %q: %v", query, err)
	}
	iter := code.Run(input)
	v, ok := iter.Next()
	if !ok {
		return nil, nil
	}
	if err, isErr := v.(error); isErr {
		return nil, err
	}
	return v, nil
}

// runAll collects every result, which the streaming cmdlets need.
func runAll(t *testing.T, query string, input any) ([]any, error) {
	t.Helper()
	parsed, err := gojq.Parse(query)
	if err != nil {
		t.Fatalf("parsing %q: %v", query, err)
	}
	code, err := gojq.Compile(parsed, testOptions()...)
	if err != nil {
		t.Fatalf("compiling %q: %v", query, err)
	}
	var out []any
	iter := code.Run(input)
	for {
		v, ok := iter.Next()
		if !ok {
			return out, nil
		}
		if err, isErr := v.(error); isErr {
			return out, err
		}
		out = append(out, v)
	}
}

func TestInvokeLLMReturnsTheCompletion(t *testing.T) {
	s := newServer(t, openAIReply("a bee"))

	got, err := run(t, `invoke_llm("what makes honey?")`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "a bee" {
		t.Errorf("got %v, want %q", got, "a bee")
	}

	req := s.request(0)
	messages, _ := req["messages"].([]any)
	if len(messages) != 1 {
		t.Fatalf("sent %d messages, want 1", len(messages))
	}
	first, _ := messages[0].(map[string]any)
	if first["content"] != "what makes honey?" {
		t.Errorf("prompt reached the API as %v", first["content"])
	}
	if req["model"] != "test-model" {
		t.Errorf("model reached the API as %v, want the provider prefix stripped", req["model"])
	}
}

// TestCallingFormsAgree pins the rule in parseCall: the prompt may come from
// the pipeline or from the first argument, and options may follow either way.
func TestCallingFormsAgree(t *testing.T) {
	newServer(t, openAIReply("same"))

	forms := []string{
		`invoke_llm("hello")`,
		`"hello" | invoke_llm`,
		`"hello" | invoke_llm({Temperature: 0.5})`,
		`invoke_llm("hello"; {Temperature: 0.5})`,
	}
	for _, form := range forms {
		got, err := run(t, form, nil)
		if err != nil {
			t.Fatalf("%s: %v", form, err)
		}
		if got != "same" {
			t.Errorf("%s: got %v", form, got)
		}
	}
}

func TestOptionsReachTheRequest(t *testing.T) {
	s := newServer(t, openAIReply("ok"))

	if _, err := run(t, `invoke_llm("hi"; {System: "be terse", Temperature: 0.25, MaxTokens: 64, TopP: 0.9, StopAt: ["END"]})`, nil); err != nil {
		t.Fatal(err)
	}
	req := s.request(0)
	if req["max_tokens"] != float64(64) {
		t.Errorf("max_tokens = %v", req["max_tokens"])
	}
	if req["temperature"] != 0.25 {
		t.Errorf("temperature = %v", req["temperature"])
	}
	if req["top_p"] != 0.9 {
		t.Errorf("top_p = %v", req["top_p"])
	}
	messages, _ := req["messages"].([]any)
	first, _ := messages[0].(map[string]any)
	if first["role"] != "system" || first["content"] != "be terse" {
		t.Errorf("System did not become a system message: %v", messages[0])
	}
}

// TestTemperatureDefaultsToZero pins the choice that a pipeline should be
// re-runnable. A provider default of 0.7 would make two runs of the same query
// disagree for no reason the caller chose.
func TestTemperatureDefaultsToZero(t *testing.T) {
	s := newServer(t, openAIReply("ok"))
	if _, err := run(t, `invoke_llm("hi")`, nil); err != nil {
		t.Fatal(err)
	}
	if got := s.request(0)["temperature"]; got != float64(0) {
		t.Errorf("temperature = %v, want 0", got)
	}
}

func TestAnthropicDialect(t *testing.T) {
	s := newServer(t, anthropicReply("hello there"))
	t.Setenv(EnvModel, "anthropic/claude-test")

	got, err := run(t, `invoke_llm("hi"; {System: "be terse"})`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello there" {
		t.Errorf("got %v", got)
	}
	req := s.request(0)
	// The Messages API takes the system prompt as its own field rather than a
	// message, which is the difference that makes this a second dialect and
	// not a base URL.
	if req["system"] != "be terse" {
		t.Errorf("system = %v", req["system"])
	}
	if _, hasMax := req["max_tokens"]; !hasMax {
		t.Error("max_tokens is required by the Messages API and was not sent")
	}
}

func TestUnknownOptionIsRejected(t *testing.T) {
	newServer(t, openAIReply("ok"))

	_, err := run(t, `invoke_llm("hi"; {MaxTokns: 10})`, nil)
	if err == nil {
		t.Fatal("a misspelled option was accepted; the call would have used the default and the caller would not know")
	}
	if !strings.Contains(err.Error(), "MaxTokns") || !strings.Contains(err.Error(), "MaxTokens") {
		t.Errorf("the error should name the bad option and the good one: %v", err)
	}
}

// TestGroupedOptionsAreRejectedElsewhere checks the group tags: an option that
// belongs to another cmdlet is a mistake, not a silent no-op.
func TestGroupedOptionsAreRejectedElsewhere(t *testing.T) {
	newServer(t, openAIReply("ok"))

	for _, query := range []string{
		`invoke_llm("hi"; {Allow: ["cat"]})`,
		`invoke_llm("hi"; {Parallel: 4})`,
	} {
		if _, err := run(t, query, nil); err == nil {
			t.Errorf("%s: accepted an option this cmdlet does not have", query)
		}
	}
}

func TestMissingModelIsReportedBeforeTheRequest(t *testing.T) {
	s := newServer(t, openAIReply("ok"))
	t.Setenv(EnvModel, "")

	_, err := run(t, `invoke_llm("hi")`, nil)
	if err == nil || !strings.Contains(err.Error(), EnvModel) {
		t.Fatalf("want an error naming %s, got %v", EnvModel, err)
	}
	if s.count() != 0 {
		t.Error("a request was made without a model")
	}
}

func TestUnknownProviderIsReported(t *testing.T) {
	newServer(t, openAIReply("ok"))
	_, err := run(t, `invoke_llm("hi"; {Model: "acme/model"})`, nil)
	if err == nil || !strings.Contains(err.Error(), "acme") {
		t.Fatalf("want an error naming the provider, got %v", err)
	}
}

func TestModelWithoutProviderIsReported(t *testing.T) {
	newServer(t, openAIReply("ok"))
	_, err := run(t, `invoke_llm("hi"; {Model: "gpt-4o"})`, nil)
	if err == nil || !strings.Contains(err.Error(), "provider/model") {
		t.Fatalf("want an error explaining the spelling, got %v", err)
	}
}

func TestMissingKeyIsReportedBeforeTheRequest(t *testing.T) {
	s := newServer(t, openAIReply("ok"))
	t.Setenv(EnvOpenAIKey, "")

	_, err := run(t, `invoke_llm("hi")`, nil)
	if err == nil || !strings.Contains(err.Error(), EnvOpenAIKey) {
		t.Fatalf("want an error naming %s rather than a 401, got %v", EnvOpenAIKey, err)
	}
	if s.count() != 0 {
		t.Error("a request was made with no credential")
	}
}

// TestLocalProviderNeedsNoKey pins that a local server is reachable without
// inventing a credential for it.
func TestLocalProviderNeedsNoKey(t *testing.T) {
	s := newServer(t, openAIReply("local"))
	t.Setenv(EnvOpenAIKey, "")

	got, err := run(t, fmt.Sprintf(`invoke_llm("hi"; {Model: "ollama/llama3", BaseUrl: %q})`, s.url), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "local" {
		t.Errorf("got %v", got)
	}
}

func TestRetriesRateLimitThenSucceeds(t *testing.T) {
	s := newServer(t, `{"error":{"message":"slow down"}}`, openAIReply("second try"))
	s.statuses = []int{http.StatusTooManyRequests, 0}
	s.headers = []map[string]string{{"Retry-After": "0"}}

	got, err := run(t, `invoke_llm("hi")`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "second try" {
		t.Errorf("got %v", got)
	}
	if s.count() != 2 {
		t.Errorf("made %d requests, want 2", s.count())
	}
}

func TestClientErrorIsNotRetried(t *testing.T) {
	s := newServer(t, `{"error":{"message":"bad request"}}`)
	s.statuses = []int{http.StatusBadRequest}

	_, err := run(t, `invoke_llm("hi")`, nil)
	if err == nil {
		t.Fatal("a 400 was not reported as a failure")
	}
	if !strings.Contains(err.Error(), "bad request") {
		t.Errorf("the API's own message should survive: %v", err)
	}
	if s.count() != 1 {
		t.Errorf("made %d requests; a 400 will not become a 200 by asking again", s.count())
	}
}

func TestFailureIsAJqError(t *testing.T) {
	s := newServer(t, `{"error":{"message":"nope"}}`)
	s.statuses = []int{http.StatusBadRequest}

	got, err := run(t, `try invoke_llm("hi") catch "caught"`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "caught" {
		t.Errorf("got %v; a cmdlet failure must be catchable like any jq error", got)
	}
}

// TestTruncatedReplyNamesTheCap covers what a reasoning model does when the
// budget runs out mid-thought: it returns nothing, and "no content" is a true
// but useless thing to tell the caller.
func TestTruncatedReplyNamesTheCap(t *testing.T) {
	encoded, _ := json.Marshal(map[string]any{
		"choices": []any{map[string]any{
			"message": map[string]any{"content": ""}, "finish_reason": "length",
		}},
		"usage": map[string]any{"prompt_tokens": 9, "completion_tokens": 64},
	})
	newServer(t, string(encoded))

	_, err := run(t, `invoke_llm("think hard"; {MaxTokens: 64})`, nil)
	if err == nil {
		t.Fatal("an empty reply was accepted")
	}
	if !strings.Contains(err.Error(), "MaxTokens") || !strings.Contains(err.Error(), "64") {
		t.Errorf("the error should name the cap and its value: %v", err)
	}
}

func TestTruncatedReplyOnAnthropic(t *testing.T) {
	encoded, _ := json.Marshal(map[string]any{
		"content": []any{}, "stop_reason": "max_tokens",
		"usage": map[string]any{"input_tokens": 9, "output_tokens": 64},
	})
	newServer(t, string(encoded))
	t.Setenv(EnvModel, "anthropic/claude-test")

	_, err := run(t, `invoke_llm("think hard"; {MaxTokens: 64})`, nil)
	if err == nil || !strings.Contains(err.Error(), "MaxTokens") {
		t.Fatalf("want an error naming the cap, got %v", err)
	}
}
