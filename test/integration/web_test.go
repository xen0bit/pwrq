package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// testServer starts a server the web cmdlets can be pointed at, so these tests
// stay hermetic: no -short guard, no reliance on a working internet connection,
// and the responses are ours to control.
func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("X-Test-Header", "present")
		_, _ = io.WriteString(w, "hello world")
	})
	mux.HandleFunc("/data.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[1,2,3],"ok":true}`)
	})
	mux.HandleFunc("/chunked", func(w http.ResponseWriter, r *http.Request) {
		// No Content-Length: this is the shape that used to report -1.
		w.Header().Set("Content-Type", "text/plain")
		flusher := w.(http.Flusher)
		_, _ = io.WriteString(w, "part one ")
		flusher.Flush()
		_, _ = io.WriteString(w, "part two")
	})
	mux.HandleFunc("/echo-method", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"method": r.Method,
			"body":   string(body),
		})
	})
	mux.HandleFunc("/missing", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not here", http.StatusNotFound)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestInvokeWebRequestShape checks the response object a user navigates after
// a request: it is plain JSON, it reports its PowerShell type like every other
// object producer, and its properties keep their JSON types.
func TestInvokeWebRequestShape(t *testing.T) {
	srv := testServer(t)

	out := mustRun(t, "null", "-c",
		`invoke_web_request("`+srv.URL+`/hello")`)

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("response is not JSON: %v (%q)", err, out)
	}

	if resp["StatusCode"] != float64(200) {
		t.Errorf("StatusCode = %v (%T), want the number 200", resp["StatusCode"], resp["StatusCode"])
	}
	if resp["Content"] != "hello world" {
		t.Errorf("Content = %v", resp["Content"])
	}
	if resp["ContentLength"] != float64(len("hello world")) {
		t.Errorf("ContentLength = %v, want %d", resp["ContentLength"], len("hello world"))
	}
	if resp["PwrqType"] != "Pwrq.Web.Response" {
		t.Errorf("PwrqType = %v", resp["PwrqType"])
	}

	headers, ok := resp["Headers"].(map[string]any)
	if !ok {
		t.Fatalf("Headers = %T, want an object", resp["Headers"])
	}
	if headers["X-Test-Header"] != "present" {
		t.Errorf("Headers[X-Test-Header] = %v, want present", headers["X-Test-Header"])
	}
}

// TestInvokeWebRequestContentIsQueryable is the payoff of returning plain JSON:
// a JSON response body parses with jq's own fromjson and needs no unwrapping.
func TestInvokeWebRequestContentIsQueryable(t *testing.T) {
	srv := testServer(t)

	got := strings.TrimSpace(mustRun(t, "null", "-c",
		`invoke_web_request("`+srv.URL+`/data.json") | .Content | fromjson | .items | add`))
	if got != "6" {
		t.Errorf("got %s, want 6", got)
	}
}

// TestInvokeWebRequestChunked guards the fix noted in Phase 5: a response with
// no Content-Length reported -1 rather than what it actually delivered.
func TestInvokeWebRequestChunked(t *testing.T) {
	srv := testServer(t)

	got := strings.TrimSpace(mustRun(t, "null", "-c",
		`invoke_web_request("`+srv.URL+`/chunked") | .ContentLength`))
	if got != "17" {
		t.Errorf("ContentLength = %s, want 17 (the length of the body actually read)", got)
	}
}

// TestInvokeWebRequestMethod checks that the method option reaches the wire
// rather than defaulting silently.
func TestInvokeWebRequestMethod(t *testing.T) {
	srv := testServer(t)

	got := strings.TrimSpace(mustRun(t, "null", "-c",
		`invoke_web_request("`+srv.URL+`/echo-method"; {Method: "POST", Body: "payload"})
		 | .Content | fromjson | {method, body}`))
	const want = `{"body":"payload","method":"POST"}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestInvokeWebRequestErrorStatus checks that an HTTP error status is data, not
// a jq error: the status code is exactly what a caller wants to branch on.
func TestInvokeWebRequestErrorStatus(t *testing.T) {
	srv := testServer(t)

	got := strings.TrimSpace(mustRun(t, "null", "-c",
		`invoke_web_request("`+srv.URL+`/missing") | .StatusCode`))
	if got != "404" {
		t.Errorf("got %s, want 404", got)
	}
}

// TestInvokeWebRequestUnreachableFails checks the other half: a request that
// never got a response has to use jq's error channel, so try/catch works.
func TestInvokeWebRequestUnreachableFails(t *testing.T) {
	// Port 1 on loopback refuses connections immediately.
	got := strings.TrimSpace(mustRun(t, "null", "-c",
		`try invoke_web_request("http://127.0.0.1:1/") catch "unreachable"`))
	if got != `"unreachable"` {
		t.Errorf("got %s, want \"unreachable\"", got)
	}
}
