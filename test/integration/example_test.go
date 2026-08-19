package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestAgentTriageExampleRuns executes examples/agent-triage.sh against a fake
// provider.
//
// An example that needs an API key is an example nothing runs, and an example
// nothing runs is documentation that rots — this one already shipped two bugs
// past review (a jq scoping mistake in its rejoin stage, and a stage that
// printed zeros because usage is per process). The provider here is a stub, so
// the test asserts the *plumbing*: that four stages hand JSON to each other and
// that the agent loop reaches an answer.
func TestAgentTriageExampleRuns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the example is a shell script")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is not available")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, reply(string(body)))
	}))
	defer server.Close()

	script, err := filepath.Abs(filepath.Join("..", "..", "examples", "agent-triage.sh"))
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("bash", script)
	cmd.Env = append(os.Environ(),
		"PWRQ="+pwrq(t),
		"PWRQ_LLM_MODEL=openai-compatible/fake",
		"OPENAI_BASE_URL="+server.URL,
		"PWRQ_AGENT_MODEL=",
		"NO_COLOR=1",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the example failed: %v\n%s", err, out)
	}
	got := string(out)

	// Each stage has to reach the next one, so each stage's output is checked.
	for _, want := range []string{
		"6 error lines",           // stage 1 found the lines
		"worker.log",              // stage 2 joined classifications back to them
		"crash",                   // the schema's enum survived into the rows
		"== 3. summarised",        // stage 3 grouped them
		"worker.log has the most", // stage 4's agent reached an answer
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the run does not contain %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "error:") || strings.Contains(got, "pwrq:") {
		t.Errorf("the run reported an error:\n%s", got)
	}
}

// reply answers as whichever kind of call the request is: a classification, or
// a step in the agent loop. Both are structured calls, so both are decided by
// what the schema asked for.
func reply(request string) string {
	var decoded struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	_ = json.Unmarshal([]byte(request), &decoded)

	var prompt, system string
	for _, m := range decoded.Messages {
		if m.Role == "system" {
			system = m.Content
		} else {
			prompt = m.Content
		}
	}

	switch {
	case strings.Contains(system, "You answer questions by writing pwrq queries"):
		// The agent loop: one query, then the answer. Which one is due is
		// decided by whether a result has already come back.
		if strings.Contains(prompt, "Result:") || len(decoded.Messages) > 2 {
			return completion(`{"thought":"done","action":"answer","content":"worker.log has the most, and they are crashes."}`)
		}
		return completion(`{"thought":"look","action":"query","content":"[.[] | select(.severity == \"high\")] | group_by(.File) | map({File: .[0].File, count: length}) | sort_by(-.count) | .[0].File"}`)
	case strings.Contains(prompt, "panic"):
		return completion(`{"category":"crash","severity":"high"}`)
	case strings.Contains(prompt, "deadline exceeded"):
		return completion(`{"category":"timeout","severity":"medium"}`)
	case strings.Contains(prompt, "connection refused"):
		return completion(`{"category":"dependency","severity":"high"}`)
	case strings.Contains(prompt, "signature verification"):
		return completion(`{"category":"auth","severity":"high"}`)
	case strings.Contains(prompt, "unmarshal"):
		return completion(`{"category":"data","severity":"medium"}`)
	default:
		return completion(`{"category":"data","severity":"low"}`)
	}
}

func completion(content string) string {
	encoded, _ := json.Marshal(map[string]any{
		"choices": []any{map[string]any{
			"message":       map[string]any{"content": content},
			"finish_reason": "stop",
		}},
		"usage": map[string]any{"prompt_tokens": 20, "completion_tokens": 10},
	})
	return string(encoded)
}
