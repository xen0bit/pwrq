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
// the test asserts the *plumbing*: that five stages hand JSON to each other and
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
		if r.URL.Path == "/v1/systemone" {
			status, answer := systemOneReply(body)
			w.WriteHeader(status)
			_, _ = io.WriteString(w, answer)
			return
		}
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
		"PWRQ_SYSTEMONE_MODEL=systemone/fake",
		"TYPESAFE_BASE_URL="+server.URL,
		"TYPESAFE_API_KEY=",
		"NO_COLOR=1",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the example failed: %v\n%s", err, out)
	}
	got := string(out)

	// Each stage has to reach the next one, so each stage's output is checked.
	for _, want := range []string{
		"6 error lines",              // stage 1 found the lines
		"worker.log",                 // stage 2 joined classifications back to them
		"crash",                      // the schema's enum survived into the rows
		"== 3. scored by System One", // stage 3 asked its typed questions
		"backend",                    // and a choice's answer reached the table
		"security",                   //
		"== 4. summarised",           // stage 4 grouped them
		"worker.log has the most",    // stage 5's agent reached an answer
		`"Pwrq.LLM.Usage"`,           // and the cost stage ran on no input
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the run does not contain %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "error:") || strings.Contains(got, "pwrq:") || strings.Contains(got, "skipped") {
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

// systemOneReply answers stage 3's typed questions from keywords in the state,
// the way reply answers stage 2's prompts. A model id that still carries its
// provider prefix gets the 422 a real server would give an unroutable request.
func systemOneReply(body []byte) (int, string) {
	var req struct {
		State     map[string]any            `json:"state"`
		Model     string                    `json:"model"`
		Questions map[string]map[string]any `json:"questions"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Model != "fake" {
		return http.StatusUnprocessableEntity, `{"detail":[{"loc":["body","model"],"msg":"unexpected model","type":"value_error"}]}`
	}
	text, _ := req.State["Text"].(string)

	page, urgency, owner := 0.2, 1.0, "platform"
	switch {
	case strings.Contains(text, "panic"):
		page, urgency, owner = 0.9, 2.0, "backend"
	case strings.Contains(text, "unmarshal"):
		owner = "backend"
	case strings.Contains(text, "signature"):
		page, owner = 0.7, "security"
	}

	answers := map[string]any{}
	for id, q := range req.Questions {
		switch q["type"] {
		case "noul":
			answers[id] = map[string]any{"type": "noul", "noul": page}
		case "score":
			answers[id] = map[string]any{"type": "score", "score": urgency, "confidence": 0.5,
				"legend": map[string]any{}, "probabilities": map[string]any{}}
		case "choice":
			answers[id] = map[string]any{"type": "choice", "choice": owner, "confidence": 0.5,
				"probabilities": map[string]any{owner: 1.0}}
		}
	}
	encoded, _ := json.Marshal(map[string]any{
		"model":   "fake.gguf",
		"answers": answers,
		"usage":   map[string]any{"input_tokens": 30, "output_tokens": len(answers)},
	})
	return http.StatusOK, string(encoded)
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
