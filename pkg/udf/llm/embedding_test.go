package llm

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// embeddingReply is what an embeddings endpoint answers with. The indexes are
// deliberately out of order: the API documents index rather than position, and
// a vector matched to the wrong text is a failure nothing downstream detects.
func embeddingReply(vectors ...[]float64) string {
	data := make([]any, 0, len(vectors))
	for i := len(vectors) - 1; i >= 0; i-- {
		data = append(data, map[string]any{"index": i, "embedding": vectors[i]})
	}
	encoded, _ := json.Marshal(map[string]any{
		"data":  data,
		"usage": map[string]any{"prompt_tokens": 5},
	})
	return string(encoded)
}

func TestEmbeddingReturnsTheVector(t *testing.T) {
	s := newServer(t, embeddingReply([]float64{0.1, 0.2, 0.3}))

	got, err := run(t, `invoke_embedding("a sentence")`, nil)
	if err != nil {
		t.Fatal(err)
	}
	vec, ok := got.([]any)
	if !ok {
		t.Fatalf("got %T, want the vector itself", got)
	}
	if len(vec) != 3 || vec[0] != 0.1 {
		t.Errorf("got %v", vec)
	}

	req := s.request(0)
	input, _ := req["input"].([]any)
	if len(input) != 1 || input[0] != "a sentence" {
		t.Errorf("input reached the API as %v", req["input"])
	}
}

func TestEmbeddingArrayKeepsInputOrder(t *testing.T) {
	newServer(t, embeddingReply([]float64{1, 0}, []float64{0, 1}))

	got, err := run(t, `invoke_embedding(["first","second"])`, nil)
	if err != nil {
		t.Fatal(err)
	}
	vectors, _ := got.([]any)
	if len(vectors) != 2 {
		t.Fatalf("got %d vectors", len(vectors))
	}
	first, _ := vectors[0].([]any)
	if first[0] != float64(1) {
		t.Errorf("vectors came back in the server's order rather than the input's: %v", got)
	}
}

func TestEmbeddingRejectsAnthropic(t *testing.T) {
	newServer(t, embeddingReply([]float64{1}))
	t.Setenv(EnvModel, "anthropic/claude-test")

	_, err := run(t, `invoke_embedding("hi")`, nil)
	if err == nil || !strings.Contains(err.Error(), "embeddings") {
		t.Fatalf("want an error explaining the provider has no embeddings API, got %v", err)
	}
}

func TestEmbeddingRejectsNonText(t *testing.T) {
	newServer(t, embeddingReply([]float64{1}))

	if _, err := run(t, `invoke_embedding({text: "hi"})`, nil); err == nil {
		t.Fatal("an object was accepted; guessing which field held the text is a guess that fails silently")
	}
}

func TestEmbeddingCountsAgainstTheBudget(t *testing.T) {
	newServer(t, embeddingReply([]float64{1}))
	t.Setenv(EnvMaxCalls, "1")

	if _, err := run(t, `invoke_embedding("one")`, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, `invoke_embedding("two")`, nil); err == nil {
		t.Fatal("embeddings are billed like completions and must count against the ceiling")
	}
}

func TestModelListing(t *testing.T) {
	newServer(t, `{"data":[{"id":"zeta","owned_by":"me"},{"id":"alpha","owned_by":"me"}]}`)

	got, err := runAll(t, `get_llm_model | {Model, Id}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d models", len(got))
	}
	first, _ := got[0].(map[string]any)
	if first["Id"] != "alpha" {
		t.Errorf("models are not sorted: %v", got)
	}
	// The Model field is the spelling every other cmdlet takes, so a listing
	// feeds straight back into a call.
	if first["Model"] != "openai/alpha" {
		t.Errorf("Model = %v, want the provider-qualified name", first["Model"])
	}
}

func TestModelListingReportsAFailure(t *testing.T) {
	s := newServer(t, `{"error":{"message":"no such endpoint"}}`)
	s.statuses = []int{404}

	_, err := runAll(t, `get_llm_model`, nil)
	if err == nil || !strings.Contains(err.Error(), "no such endpoint") {
		t.Fatalf("want the server's own message, got %v", err)
	}
}

// TestReasoningIsReportedSeparately covers what local runtimes send: a thinking
// model's chain of thought arrives beside the answer, and folding it into
// .Content would put the reasoning into every downstream filter.
func TestReasoningIsReportedSeparately(t *testing.T) {
	encoded, _ := json.Marshal(map[string]any{
		"choices": []any{map[string]any{
			"message":       map[string]any{"content": "42", "reasoning_content": "let me count"},
			"finish_reason": "stop",
		}},
		"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 1},
	})
	newServer(t, string(encoded))

	got, err := run(t, `invoke_llm_request("how many?")`, nil)
	if err != nil {
		t.Fatal(err)
	}
	obj, _ := got.(map[string]any)
	if obj["Content"] != "42" {
		t.Errorf("Content = %v, want just the answer", obj["Content"])
	}
	if obj["Reasoning"] != "let me count" {
		t.Errorf("Reasoning = %v", obj["Reasoning"])
	}
}

func TestReasoningIsNullWhenAbsent(t *testing.T) {
	newServer(t, openAIReply("plain"))

	got, err := run(t, `invoke_llm_request("hi")`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if obj, _ := got.(map[string]any); obj["Reasoning"] != nil {
		t.Errorf("Reasoning = %v, want null", obj["Reasoning"])
	}
}

// TestSemanticSearchPipeline is the shape invoke_embedding exists for, end to
// end: embed a corpus, embed a question, rank by meaning.
func TestSemanticSearchPipeline(t *testing.T) {
	s := newServer(t)
	s.replies = []string{
		embeddingReply([]float64{1, 0}, []float64{0, 1}),
		embeddingReply([]float64{0.9, 0.1}),
	}

	got, err := run(t, `
		["about cats", "about bridges"] as $docs
		| invoke_embedding($docs) as $vectors
		| invoke_embedding("feline pets") as $q
		| [range(0; 2) | {text: $docs[.], score: cosine_similarity($vectors[.]; $q)}]
		| sort_by(-.score) | .[0].text`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "about cats" {
		t.Errorf("got %v, want the semantically closest document", got)
	}
	if s.count() != 2 {
		t.Errorf("made %d requests, want one for the corpus and one for the question", s.count())
	}
}

func TestCosineSimilarityRejectsMismatchedVectors(t *testing.T) {
	newServer(t, openAIReply("ok"))

	_, err := run(t, `cosine_similarity([1,2]; [1,2,3])`, nil)
	if err == nil {
		t.Fatal("vectors of different lengths were compared")
	}
	if !strings.Contains(fmt.Sprint(err), "different lengths") {
		t.Errorf("the error should say why: %v", err)
	}
}

// TestModelListingTakesAProviderAlone pins the natural way to ask: listing
// needs the endpoint, not a model, so naming a model to find out which models
// exist would be a chicken and egg.
func TestModelListingTakesAProviderAlone(t *testing.T) {
	newServer(t, `{"data":[{"id":"one"}]}`)
	t.Setenv(EnvModel, "")

	for _, spec := range []string{"openai", "openai/", "openai/some-model"} {
		got, err := runAll(t, fmt.Sprintf(`get_llm_model({Model: %q}) | .Id`, spec), nil)
		if err != nil {
			t.Errorf("{Model: %q}: %v", spec, err)
			continue
		}
		if len(got) != 1 || got[0] != "one" {
			t.Errorf("{Model: %q}: got %v", spec, got)
		}
	}
}

func TestModelListingNeedsAProvider(t *testing.T) {
	newServer(t, `{"data":[]}`)
	t.Setenv(EnvModel, "")

	if _, err := runAll(t, `get_llm_model`, nil); err == nil {
		t.Fatal("a listing with no provider was accepted")
	}
}
