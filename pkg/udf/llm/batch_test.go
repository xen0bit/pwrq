package llm

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestBatchEmitsInInputOrder(t *testing.T) {
	s := newServer(t)
	// Every reply is the same shape, so the only thing that could order the
	// results is the code doing it.
	s.replies = []string{openAIReply("first"), openAIReply("second"), openAIReply("third")}
	// Hold every request until they are all in flight, so completion order is
	// the opposite of nothing in particular and ordering cannot pass by luck.
	s.release = make(chan struct{})
	close(s.release)

	got, err := runAll(t, `invoke_llm_batch(["a","b","c"]; {Parallel: 1}) | .Content`, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []any{"first", "second", "third"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestBatchIndexesResults pins the correlation a caller needs: which row did
// this answer come from.
func TestBatchIndexesResults(t *testing.T) {
	newServer(t, openAIReply("x"))

	got, err := runAll(t, `invoke_llm_batch(["a","b"]) | .Index`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(got) != fmt.Sprint([]any{0, 1}) {
		t.Errorf("got %v", got)
	}
}

// TestBatchRunsInParallel is the reason this cmdlet exists. A sequential
// implementation passes every other test here.
func TestBatchRunsInParallel(t *testing.T) {
	s := newServer(t, openAIReply("ok"))
	// Requests block until every worker has arrived, so a sequential
	// implementation would deadlock rather than merely be slow — which is the
	// only way to test concurrency without timing.
	s.release = make(chan struct{})
	go func() {
		for s.count() < 4 {
			time.Sleep(2 * time.Millisecond)
		}
		close(s.release)
	}()

	if _, err := runAll(t, `invoke_llm_batch(["a","b","c","d"]; {Parallel: 4}) | .Content`, nil); err != nil {
		t.Fatal(err)
	}
	if s.peak < 2 {
		t.Errorf("peak concurrency was %d; the batch ran sequentially", s.peak)
	}
}

func TestBatchStopsAtParallelLimit(t *testing.T) {
	s := newServer(t, openAIReply("ok"))

	if _, err := runAll(t, `invoke_llm_batch(["a","b","c","d"]; {Parallel: 2})`, nil); err != nil {
		t.Fatal(err)
	}
	if s.peak > 2 {
		t.Errorf("peak concurrency was %d, want no more than the 2 asked for", s.peak)
	}
}

// TestBatchFailsTheWholeCall pins the default: half a batch quietly becoming
// null is the outcome worth refusing.
func TestBatchFailsTheWholeCall(t *testing.T) {
	s := newServer(t, `{"error":{"message":"nope"}}`)
	s.statuses = []int{http.StatusBadRequest}

	if _, err := runAll(t, `invoke_llm_batch(["a","b"]; {Parallel: 1})`, nil); err == nil {
		t.Fatal("a failed prompt did not fail the call")
	}
}

func TestBatchContinueOnErrorReportsPerPrompt(t *testing.T) {
	s := newServer(t, `{"error":{"message":"nope"}}`, openAIReply("second"))
	s.statuses = []int{http.StatusBadRequest, 0}

	got, err := runAll(t, `invoke_llm_batch(["a","b"]; {Parallel: 1, ContinueOnError: true}) | {Index, Content, Error}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d results, want one per prompt", len(got))
	}
	first, _ := got[0].(map[string]any)
	if first["Error"] == nil {
		t.Error("the failed prompt reported no error")
	}
	if first["Content"] != nil {
		t.Error("the failed prompt reported content")
	}
	second, _ := got[1].(map[string]any)
	if second["Content"] != "second" || second["Error"] != nil {
		t.Errorf("the surviving prompt came back as %v", second)
	}
}

func TestBatchRequiresAnArray(t *testing.T) {
	newServer(t, openAIReply("ok"))

	_, err := runAll(t, `invoke_llm_batch("just one")`, nil)
	if err == nil || !strings.Contains(err.Error(), "array") {
		t.Fatalf("want an error asking for an array, got %v", err)
	}
}

func TestBatchTakesPromptsFromThePipeline(t *testing.T) {
	newServer(t, openAIReply("ok"))

	got, err := runAll(t, `["a","b"] | invoke_llm_batch({Parallel: 2}) | .Content`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("got %d results", len(got))
	}
}

func TestBudgetCeilingStopsARunawayMap(t *testing.T) {
	s := newServer(t, openAIReply("ok"))
	t.Setenv(EnvMaxCalls, "2")

	_, err := runAll(t, `invoke_llm_batch(["a","b","c","d"]; {Parallel: 1})`, nil)
	if err == nil {
		t.Fatal("the ceiling did not stop the run")
	}
	if !strings.Contains(err.Error(), EnvMaxCalls) {
		t.Errorf("the error should say how to raise the ceiling: %v", err)
	}
	if s.count() > 2 {
		t.Errorf("made %d requests past a ceiling of 2", s.count())
	}
}

func TestBudgetCeilingCanBeRemoved(t *testing.T) {
	newServer(t, openAIReply("ok"))
	t.Setenv(EnvMaxCalls, "0")

	if _, err := runAll(t, `invoke_llm_batch(["a","b","c"]; {Parallel: 1})`, nil); err != nil {
		t.Fatal(err)
	}
}

func TestUsageAccumulates(t *testing.T) {
	newServer(t, openAIReply("ok"))

	got, err := run(t, `[invoke_llm_batch(["a","b"])] | length as $n | get_llm_usage`, nil)
	if err != nil {
		t.Fatal(err)
	}
	usage, _ := got.(map[string]any)
	if usage["Calls"] != 2 {
		t.Errorf("Calls = %v, want 2", usage["Calls"])
	}
	if usage["InputTokens"] != 22 {
		t.Errorf("InputTokens = %v, want 22", usage["InputTokens"])
	}
	if usage["Cost"] != nil {
		t.Errorf("Cost = %v, want null with no prices supplied", usage["Cost"])
	}
}

func TestCacheAvoidsASecondRequest(t *testing.T) {
	s := newServer(t, openAIReply("cached answer"))
	dir := t.TempDir()
	t.Setenv(EnvCacheDir, dir)

	first, err := run(t, `invoke_llm_request("hi"; {Cache: true}) | {Content, Cached}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if obj, _ := first.(map[string]any); obj["Cached"] != false {
		t.Errorf("first call reported Cached = %v", obj["Cached"])
	}

	second, err := run(t, `invoke_llm_request("hi"; {Cache: true}) | {Content, Cached}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	obj, _ := second.(map[string]any)
	if obj["Cached"] != true {
		t.Errorf("second call reported Cached = %v", obj["Cached"])
	}
	if obj["Content"] != "cached answer" {
		t.Errorf("Content = %v", obj["Content"])
	}
	if s.count() != 1 {
		t.Errorf("made %d requests; the second should have been served from disk", s.count())
	}
}

// TestCacheKeyFollowsTheRequest pins that a changed parameter is a different
// entry. A cache that ignored temperature would serve the wrong answer.
func TestCacheKeyFollowsTheRequest(t *testing.T) {
	s := newServer(t, openAIReply("a"), openAIReply("b"))
	t.Setenv(EnvCacheDir, t.TempDir())

	if _, err := run(t, `invoke_llm("hi"; {Cache: true})`, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, `invoke_llm("hi"; {Cache: true, Temperature: 0.9})`, nil); err != nil {
		t.Fatal(err)
	}
	if s.count() != 2 {
		t.Errorf("made %d requests; a different temperature is a different request", s.count())
	}
}

// TestCacheNeverStoresTheKey is a leak check. Cache filenames and contents end
// up on disk and in backups.
func TestCacheNeverStoresTheKey(t *testing.T) {
	newServer(t, openAIReply("ok"))
	t.Setenv(EnvCacheDir, t.TempDir())

	o := defaults()
	o.Model = "openai/test-model"
	o.ApiKey = "sk-secret-value"
	key := (&cache{dir: "x"}).key(o, []message{{Role: "user", Content: "hi"}})
	if strings.Contains(key, "secret") {
		t.Fatal("the cache key was derived from something containing the API key")
	}
	// Two accounts asking the same question ask the same question.
	other := o
	other.ApiKey = "sk-different"
	if key != (&cache{dir: "x"}).key(other, []message{{Role: "user", Content: "hi"}}) {
		t.Error("the API key changed the cache key; it selects an account, not an answer")
	}
}

func TestContextNeverPrintsTheKey(t *testing.T) {
	newServer(t, openAIReply("ok"))
	t.Setenv(EnvOpenAIKey, "sk-very-secret")

	got, err := run(t, `get_llm_context`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fmt.Sprint(got), "sk-very-secret") {
		t.Fatal("get_llm_context printed the API key; query output ends up in logs")
	}
	obj, _ := got.(map[string]any)
	if obj["HasApiKey"] != true {
		t.Error("HasApiKey did not report a key that is set")
	}
	if obj["ApiKeySource"] != EnvOpenAIKey {
		t.Errorf("ApiKeySource = %v, want the variable it was read from", obj["ApiKeySource"])
	}
}

func TestContextExplainsAMissingModel(t *testing.T) {
	newServer(t, openAIReply("ok"))
	t.Setenv(EnvModel, "")

	got, err := run(t, `get_llm_context`, nil)
	if err != nil {
		t.Fatalf("get_llm_context must explain a misconfiguration rather than fail on it: %v", err)
	}
	obj, _ := got.(map[string]any)
	if obj["Problem"] == nil {
		t.Error("no Problem was reported for a missing model")
	}
}

// TestBatchStopsSpendingAfterAFailure is a cost test, not a correctness one.
// Running the pool to completion and reporting the error afterwards passes
// every other test here and bills the caller for every prompt whose answer is
// about to be discarded.
func TestBatchStopsSpendingAfterAFailure(t *testing.T) {
	s := newServer(t, `{"error":{"message":"nope"}}`)
	s.statuses = []int{http.StatusBadRequest}

	prompts := `["a","b","c","d","e","f","g","h"]`
	if _, err := runAll(t, `invoke_llm_batch(`+prompts+`; {Parallel: 1})`, nil); err == nil {
		t.Fatal("want a failure")
	}
	if s.count() > 2 {
		t.Errorf("sent %d prompts after the first failed; the rest should have been abandoned", s.count())
	}
}

// TestBatchContinueOnErrorKeepsGoing is the other side of it: asked to
// continue, every prompt is still sent.
func TestBatchContinueOnErrorKeepsGoing(t *testing.T) {
	s := newServer(t, `{"error":{"message":"nope"}}`, openAIReply("b"), openAIReply("c"))
	s.statuses = []int{http.StatusBadRequest, 0, 0}

	got, err := runAll(t, `invoke_llm_batch(["a","b","c"]; {Parallel: 1, ContinueOnError: true}) | .Index`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d results, want one per prompt", len(got))
	}
	if s.count() != 3 {
		t.Errorf("sent %d prompts, want all 3", s.count())
	}
}
