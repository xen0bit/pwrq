package llm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
)

// systemOneReply is a System One response carrying answers.
func systemOneReply(answers map[string]any) string {
	encoded, _ := json.Marshal(map[string]any{
		"model":   "served.gguf",
		"answers": answers,
		"usage":   map[string]any{"input_tokens": 40, "output_tokens": len(answers)},
	})
	return string(encoded)
}

func nouls(ids ...string) map[string]any {
	out := make(map[string]any, len(ids))
	for _, id := range ids {
		out[id] = map[string]any{"type": "noul", "noul": 0.75}
	}
	return out
}

func (s *server) path(i int) string {
	s.t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.paths[i]
}

func (s *server) header(i int, name string) string {
	s.t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reqHeaders[i].Get(name)
}

func TestSystemOneReturnsTheAnswers(t *testing.T) {
	s := newServer(t, systemOneReply(map[string]any{
		"urgent": map[string]any{"type": "noul", "noul": 0.9},
		"team": map[string]any{
			"type": "choice", "choice": "billing",
			"probabilities": map[string]any{"billing": 0.8, "tech": 0.2}, "confidence": 0.6,
		},
	}))

	got, err := run(t, `invoke_systemone({
		urgent: {type: "noul", instructions: "Is it urgent?"},
		team:   {type: "choice", criteria: {billing: "Payments", tech: null}}
	}) | [.urgent.noul, .team.choice]`, "my card was charged twice")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []any{0.9, "billing"}) {
		t.Errorf("got %v", got)
	}

	if p := s.path(0); p != "/v1/systemone" {
		t.Errorf("path = %q", p)
	}
	if h := s.header(0, "Authorization"); h != "Bearer test-key" {
		t.Errorf("Authorization = %q", h)
	}
	req := s.request(0)
	if req["state"] != "my card was charged twice" {
		t.Errorf("state = %v", req["state"])
	}
	// The provider prefix is pwrq's addressing, not the server's.
	if req["model"] != "test-model" {
		t.Errorf("model = %v, want the id without the provider", req["model"])
	}
	questions, _ := req["questions"].(map[string]any)
	team, _ := questions["team"].(map[string]any)
	if team["type"] != "choice" || len(questions) != 2 {
		t.Errorf("questions did not reach the wire intact: %v", questions)
	}
}

func TestSystemOneCallingFormsAgree(t *testing.T) {
	s := newServer(t, systemOneReply(nouls("q")))

	queries := []string{
		`invoke_systemone({q: {type: "noul"}})`,
		`invoke_systemone({q: {type: "noul"}}; {Retries: 0})`,
		`invoke_systemone({state: 1}; {q: {type: "noul"}}; {})`,
	}
	inputs := []any{map[string]any{"state": 1}, map[string]any{"state": 1}, nil}
	for i, q := range queries {
		if _, err := run(t, q, inputs[i]); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	for i := 1; i < len(queries); i++ {
		if !reflect.DeepEqual(s.request(0), s.request(i)) {
			t.Errorf("%s sent %v, want %v", queries[i], s.request(i), s.request(0))
		}
	}
}

func TestSystemOneRequestEnvelope(t *testing.T) {
	newServer(t, systemOneReply(nouls("a", "b")))

	got, err := run(t, `invoke_systemone_request({a: {type: "noul"}, b: {type: "noul"}}; {PriceInput: 1000000, PriceOutput: 1000000})`, "state")
	if err != nil {
		t.Fatal(err)
	}
	obj, _ := got.(map[string]any)
	if obj["Model"] != "systemone/test-model" || obj["ServedModel"] != "served.gguf" || obj["Provider"] != "systemone" {
		t.Errorf("addressing: %v", obj)
	}
	if obj["InputTokens"] != 40 || obj["OutputTokens"] != 2 || obj["TotalTokens"] != 42 {
		t.Errorf("tokens: %v", obj)
	}
	if obj["Cost"] != 42.0 {
		t.Errorf("Cost = %v, want 42", obj["Cost"])
	}
	if obj["Cached"] != false || obj["PwrqType"] != "Pwrq.LLM.SystemOne" {
		t.Errorf("envelope: %v", obj)
	}
	answers, _ := obj["Answers"].(map[string]any)
	if len(answers) != 2 {
		t.Errorf("Answers = %v", obj["Answers"])
	}

	got, err = run(t, `invoke_systemone_request({a: {type: "noul"}, b: {type: "noul"}}) | .Cost`, "state")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("Cost = %v, want null without prices", got)
	}
}

// TestSystemOneValidatesBeforeTheRequest pins that a malformed question costs
// an error message rather than a request.
func TestSystemOneValidatesBeforeTheRequest(t *testing.T) {
	cases := []struct {
		name, query string
		input       any
		want        string
	}{
		{"unknown type", `invoke_systemone({q: {type: "yesno"}})`, "s", "not one of noul, choice or score"},
		{"missing type", `invoke_systemone({q: {instructions: "x"}})`, "s", "type is missing"},
		{"choice without criteria", `invoke_systemone({q: {type: "choice"}})`, "s", "a choice needs criteria"},
		{"score with one level", `invoke_systemone({q: {type: "score", criteria: ["only"]}})`, "s", "at least 2"},
		{"null level", `invoke_systemone({q: {type: "score", criteria: ["a", null]}})`, "s", "level 1"},
		{"too many options", `invoke_systemone({q: {type: "choice", criteria: ([range(53)] | map({key: "o\(.)", value: null}) | from_entries)}})`, "s", "at most 52"},
		{"empty questions", `invoke_systemone({})`, "s", "questions is empty"},
		{"questions not an object", `invoke_systemone(["q"])`, "s", "questions must be an object"},
		{"question not an object", `invoke_systemone({q: "noul"})`, "s", "must be an object"},
		{"unknown key", `invoke_systemone({q: {type: "choice", criterion: {a: 1, b: 2}}})`, "s", `unknown key "criterion"`},
		{"bad noul criteria", `invoke_systemone({q: {type: "noul", criteria: {yes: "y"}}})`, "s", `not "yes"`},
		{"numeric instructions", `invoke_systemone({q: {type: "noul", instructions: 3}})`, "s", "instructions must be"},
		{"null state", `invoke_systemone({q: {type: "noul"}})`, nil, "state must be"},
		{"numeric state", `invoke_systemone({q: {type: "noul"}})`, 7, "state must be"},
		{"blank state", `invoke_systemone({q: {type: "noul"}})`, "  ", "state is empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newServer(t, systemOneReply(nouls("q")))
			_, err := run(t, tc.query, tc.input)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error containing %q, got %v", tc.want, err)
			}
			if s.count() != 0 {
				t.Errorf("made %d requests for a question that could only fail", s.count())
			}
		})
	}
}

func TestSystemOneRejectsChatOptions(t *testing.T) {
	for _, opt := range []string{"Temperature: 0.5", "Schema: {}", "MaxTokens: 10", `System: "x"`, "Parallel: 2", `Allow: ["x"]`} {
		s := newServer(t, systemOneReply(nouls("q")))
		_, err := run(t, fmt.Sprintf(`invoke_systemone({q: {type: "noul"}}; {%s})`, opt), "s")
		if err == nil || !strings.Contains(err.Error(), "unknown option") {
			t.Errorf("{%s} was accepted; an option that changes nothing is one the caller believes they set: %v", opt, err)
			continue
		}
		if strings.Contains(err.Error(), "Temperature,") || !strings.Contains(err.Error(), "BaseUrl") {
			t.Errorf("the suggestion should list only what applies: %v", err)
		}
		if s.count() != 0 {
			t.Errorf("{%s} made a request", opt)
		}
	}
}

func TestSystemOneSurfaces422Detail(t *testing.T) {
	s := newServer(t, `{"detail":[{"loc":["body","questions","q","criteria"],"msg":"Field required","type":"missing"}]}`)
	s.statuses = []int{http.StatusUnprocessableEntity}

	_, err := run(t, `invoke_systemone({q: {type: "noul"}})`, "s")
	if err == nil || !strings.Contains(err.Error(), "questions.q.criteria: Field required") {
		t.Fatalf("want the field and message, got %v", err)
	}
	if s.count() != 1 {
		t.Errorf("made %d requests; a validation error is not worth retrying", s.count())
	}
}

func TestSystemOne501IsNotRetried(t *testing.T) {
	s := newServer(t, `{"error":{"code":501,"message":"This server does not support readout","type":"not_supported_error"}}`)
	s.statuses = []int{http.StatusNotImplemented}

	_, err := run(t, `invoke_systemone({q: {type: "noul"}})`, "s")
	if err == nil || !strings.Contains(err.Error(), "does not support readout") {
		t.Fatalf("want the server's message, got %v", err)
	}
	if s.count() != 1 {
		t.Errorf("made %d requests; not implemented stays not implemented", s.count())
	}
}

func TestSystemOneRetriesAServerFault(t *testing.T) {
	s := newServer(t, `{"error":{"message":"loading"}}`, systemOneReply(nouls("q")))
	s.statuses = []int{http.StatusServiceUnavailable, 0}
	s.headers = []map[string]string{{"Retry-After": "0"}}

	if _, err := run(t, `invoke_systemone({q: {type: "noul"}})`, "s"); err != nil {
		t.Fatal(err)
	}
	if s.count() != 2 {
		t.Errorf("made %d requests, want 2", s.count())
	}
}

func TestSystemOneIsOneCallPerRequest(t *testing.T) {
	newServer(t, systemOneReply(nouls("a", "b", "c")))

	got, err := run(t, `invoke_systemone({a: {type: "noul"}, b: {type: "noul"}, c: {type: "noul"}}) | get_llm_usage`, "s")
	if err != nil {
		t.Fatal(err)
	}
	usage, _ := got.(map[string]any)
	if usage["Calls"] != 1 || usage["InputTokens"] != 40 || usage["OutputTokens"] != 3 {
		t.Errorf("usage = %v, want one call of 40 in and 3 out", usage)
	}

	t.Setenv(EnvMaxCalls, "1")
	if _, err := run(t, `invoke_systemone({a: {type: "noul"}, b: {type: "noul"}, c: {type: "noul"}})`, "t"); err == nil {
		t.Fatal("a second request passed a ceiling of one call")
	}
}

func TestSystemOneCache(t *testing.T) {
	s := newServer(t, systemOneReply(nouls("q", "r")))
	dir := t.TempDir()
	t.Setenv(EnvCacheDir, dir)

	query := `invoke_systemone_request({q: {type: "noul"}}; {Cache: true}) | .Cached`
	for i, want := range []bool{false, true} {
		got, err := run(t, query, "s")
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("call %d: Cached = %v, want %v", i, got, want)
		}
	}
	if s.count() != 1 {
		t.Errorf("made %d requests; the second should have come from disk", s.count())
	}

	if _, err := run(t, `invoke_systemone({r: {type: "noul"}}; {Cache: true})`, "s"); err != nil {
		t.Fatal(err)
	}
	if s.count() != 2 {
		t.Error("different questions were served another question's answers")
	}

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		payload, _ := os.ReadFile(dir + "/" + e.Name())
		if strings.Contains(string(payload), "test-key") {
			t.Error("the API key reached the cache")
		}
	}
}

// TestSystemOneCacheIsNotAChatCache pins that the two key spaces never meet.
func TestSystemOneCacheIsNotAChatCache(t *testing.T) {
	o := defaults()
	o.Model = "systemone/m"
	c := &cache{dir: "x"}
	if c.systemOneKey(o, "hi", nil) == c.key(o, []message{{Role: "user", Content: "hi"}}) {
		t.Error("a System One request and a prompt share a cache key")
	}
}

func TestSystemOneBaseURLForms(t *testing.T) {
	s := newServer(t, systemOneReply(nouls("q")))

	for _, base := range []string{s.url, s.url + "/", s.url + "/v1", s.url + "/v1/"} {
		query := fmt.Sprintf(`invoke_systemone({q: {type: "noul"}}; {BaseUrl: %q})`, base)
		if _, err := run(t, query, "s"); err != nil {
			t.Fatalf("%s: %v", base, err)
		}
	}
	for i := range 4 {
		if p := s.path(i); p != "/v1/systemone" {
			t.Errorf("request %d went to %q", i, p)
		}
	}
}

func TestSystemOneLocalNeedsNoKey(t *testing.T) {
	s := newServer(t, systemOneReply(nouls("q")))
	t.Setenv(EnvTypeSafeKey, "")

	if _, err := run(t, `invoke_systemone({q: {type: "noul"}})`, "s"); err != nil {
		t.Fatal(err)
	}
	if h := s.header(0, "Authorization"); h != "" {
		t.Errorf("sent Authorization %q with no key configured", h)
	}
}

// TestSystemOneHostedNeedsKey is also a network guard: with nothing set, the
// base is TypeSafe's own, and the call must fail before it gets there.
func TestSystemOneHostedNeedsKey(t *testing.T) {
	s := newServer(t, systemOneReply(nouls("q")))
	t.Setenv(EnvTypeSafeKey, "")
	t.Setenv(EnvTypeSafeBase, "")

	_, err := run(t, `invoke_systemone({q: {type: "noul"}})`, "s")
	if err == nil || !strings.Contains(err.Error(), EnvTypeSafeKey) {
		t.Fatalf("want an error naming %s, got %v", EnvTypeSafeKey, err)
	}
	if s.count() != 0 {
		t.Error("a request was made")
	}

	got, err := run(t, `get_llm_context({Model: "systemone/jev-latest"}) | [.ApiKeyRequired, .Endpoint]`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []any{true, "https://api.typesafe.ai/v1/systemone"}) {
		t.Errorf("context = %v", got)
	}
}

func TestSystemOneContextAtALocalServer(t *testing.T) {
	s := newServer(t)
	t.Setenv(EnvTypeSafeKey, "")

	got, err := run(t, `get_llm_context({Model: "systemone/gemma"}) | {ApiKeyRequired, Endpoint, MaxTokens, Problem}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"ApiKeyRequired": false, "Endpoint": s.url + "/v1/systemone", "MaxTokens": nil, "Problem": nil}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestSystemOneContextFromTheEnvironmentAlone covers the shell that has only
// System One set up. get_llm_context exists to explain a misconfiguration, and
// answering "set PWRQ_LLM_MODEL" to someone whose calls work would be one it
// invented.
func TestSystemOneContextFromTheEnvironmentAlone(t *testing.T) {
	s := newServer(t)
	t.Setenv(EnvModel, "")

	got, err := run(t, `get_llm_context | {Model, ModelSource, Provider, Endpoint, Problem}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"Model": "systemone/test-model", "ModelSource": EnvSystemOneModel,
		"Provider": "systemone", "Endpoint": s.url + "/v1/systemone", "Problem": nil,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	// A chat model configured as well is the one reported, since that is what
	// a bare invoke_llm would use; the other is a {Model: ...} away.
	t.Setenv(EnvModel, "openai/test-model")
	got, err = run(t, `get_llm_context | .Provider`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "openai" {
		t.Errorf("Provider = %v, want openai", got)
	}
}

func TestSystemOneMissingAnswerIsAnError(t *testing.T) {
	newServer(t, systemOneReply(nouls("a")))

	_, err := run(t, `invoke_systemone({a: {type: "noul"}, b: {type: "noul"}})`, "s")
	if err == nil || !strings.Contains(err.Error(), `question "b"`) {
		t.Fatalf("want an error naming the unanswered question, got %v", err)
	}
}

func TestSystemOneModelPrecedence(t *testing.T) {
	s := newServer(t, systemOneReply(nouls("q")))

	t.Setenv(EnvModel, "systemone/from-llm-model")
	if _, err := run(t, `invoke_systemone({q: {type: "noul"}})`, "s"); err != nil {
		t.Fatal(err)
	}
	if m := s.request(0)["model"]; m != "test-model" {
		t.Errorf("model = %v; %s should win", m, EnvSystemOneModel)
	}

	t.Setenv(EnvSystemOneModel, "")
	if _, err := run(t, `invoke_systemone({q: {type: "noul"}})`, "s"); err != nil {
		t.Fatal(err)
	}
	if m := s.request(1)["model"]; m != "from-llm-model" {
		t.Errorf("model = %v; a System One %s is the fallback", m, EnvModel)
	}

	t.Setenv(EnvModel, "openai/gpt")
	_, err := run(t, `invoke_systemone({q: {type: "noul"}})`, "s")
	if err == nil || !strings.Contains(err.Error(), EnvSystemOneModel) {
		t.Fatalf("a chat model in %s was used, or the error did not name %s: %v", EnvModel, EnvSystemOneModel, err)
	}
}

func TestSystemOneRejectsChatModel(t *testing.T) {
	s := newServer(t, systemOneReply(nouls("q")))

	_, err := run(t, `invoke_systemone({q: {type: "noul"}}; {Model: "openai/gpt-4o"})`, "s")
	if err == nil || !strings.Contains(err.Error(), "needs a System One model") {
		t.Fatalf("got %v", err)
	}
	if s.count() != 0 {
		t.Error("a request was made")
	}
}

func TestPromptCmdletsRejectSystemOneModel(t *testing.T) {
	s := newServer(t, openAIReply("x"))
	t.Setenv(EnvModel, "systemone/test-model")

	for _, query := range []string{
		`invoke_llm("hi")`,
		`invoke_llm_request("hi")`,
		`[invoke_llm_batch(["a", "b"])]`,
		`invoke_embedding("hi")`,
	} {
		_, err := run(t, query, nil)
		if err == nil {
			t.Errorf("%s accepted a System One model", query)
		}
	}
	if _, err := run(t, `invoke_llm("hi")`, nil); err == nil || !strings.Contains(err.Error(), "use invoke_systemone") {
		t.Errorf("the error should point at invoke_systemone: %v", err)
	}
	got, _ := run(t, `get_llm_usage | .Calls`, nil)
	if s.count() != 0 || got != 0 {
		t.Errorf("made %d requests and charged %v calls for calls that could only fail", s.count(), got)
	}
}

func TestModelListingOnSystemOne(t *testing.T) {
	s := newServer(t, `{"models":[{"name":"jev-latest","description":"flagship","release_date":"2026-01-01"}]}`)

	got, err := runAll(t, `get_llm_model({Model: "systemone"}) | {Model, DisplayName, Created}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []any{map[string]any{"Model": "systemone/jev-latest", "DisplayName": "flagship", "Created": "2026-01-01"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
	if p := s.path(0); p != "/v1/models" {
		t.Errorf("path = %q", p)
	}
}

func TestModelListingPrefersDataOverModels(t *testing.T) {
	// llama.cpp sends both lists; the ids are the names that route.
	newServer(t, `{"data":[{"id":"gemma.gguf"}],"models":[{"name":"gemma.gguf"}]}`)

	got, err := runAll(t, `get_llm_model({Model: "systemone"}) | .Id`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []any{"gemma.gguf"}) {
		t.Errorf("got %v", got)
	}
}

func TestDetailMessage(t *testing.T) {
	cases := map[string]string{
		`"Not authenticated"`: "Not authenticated",
		`[{"loc":["body","questions","q","type"],"msg":"bad tag"},{"loc":["body","state"],"msg":"Field required"}]`: "questions.q.type: bad tag; state: Field required",
		`[{"loc":[],"msg":"Invalid JSON"}]`: "Invalid JSON",
		`{}`:                                "",
	}
	for raw, want := range cases {
		if got := detailMessage(json.RawMessage(raw)); got != want {
			t.Errorf("detailMessage(%s) = %q, want %q", raw, got, want)
		}
	}
}
