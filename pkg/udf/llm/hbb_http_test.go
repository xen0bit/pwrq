package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeHbbServer speaks `hbb serve --listen`. It records the last score request.
func fakeHbbServer(t *testing.T, token string, maxBody int, last *map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token != "" && r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"missing or wrong bearer token"}`))
			return
		}
		switch r.URL.Path {
		case "/v1/info":
			_, _ = w.Write([]byte(`{"ready":true,"protocol":1,"max_body":` + strconv.Itoa(maxBody) + `,"version":"fake","model":"remote/enc@1","device":"gpu","labels":["cwe_79"],"window":{"lines":240},"questions":{"cwe_79":{"cwe":"CWE-79","title":"XSS","q":"?"}}}`))
		case "/v1/score":
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			if last != nil {
				*last = req
			}
			if p, _ := req["path"].(string); strings.HasSuffix(p, ".boom") {
				_, _ = w.Write([]byte(`{"error":"cannot score it"}`))
				return
			}
			_, _ = w.Write([]byte(`{"path":"a.c","lang":"c","lines":1,"tokens":7,"windows":[{"from":1,"to":1,"p":{"cwe_79":0.5}}]}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestInvokeHbbOverHTTPSendsTheBytes(t *testing.T) {
	resetUsage()
	var last map[string]any
	srv := fakeHbbServer(t, "tok", 800, &last)
	f := filepath.Join(t.TempDir(), "a.c")
	if err := os.WriteFile(f, []byte("int x;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	o := `{Url: "` + srv.URL + `", Token: "tok"}`
	out := runHbbQuery(t, `invoke_hbb({Path: "src/a.c", Full: "`+f+`"}; `+o+`) | [.Lang, .Windows[0].P.cwe_79, .Tokens]`)
	got, _ := out[0].([]any)
	if len(got) != 3 || got[0] != "c" || got[1] != 0.5 {
		t.Fatalf("got %v", out)
	}
	if last["text"] != "int x;\n" || last["file"] != nil || last["path"] != "src/a.c" {
		t.Fatalf("server was sent %v", last)
	}
	if u := usageObject(); u["Calls"] != 1 || u["InputTokens"] != 7 {
		t.Fatalf("usage %v", u)
	}
	out = runHbbQuery(t, `get_hbb(`+o+`) | [.Model, .Device]`)
	if got, _ := out[0].([]any); len(got) != 2 || got[0] != "remote/enc@1" || got[1] != "gpu" {
		t.Fatalf("got %v", out)
	}
}

func TestInvokeHbbOverHTTPFailureClasses(t *testing.T) {
	srv := fakeHbbServer(t, "tok", 800, nil)
	o := `{Url: "` + srv.URL + `", Token: "tok"}`
	// a file the server could not score is that file's error and the query goes on
	out := runHbbQuery(t, `invoke_hbb({Path: "b.boom", Text: "x"}; `+o+`) | .Error`)
	if out[0] != "cannot score it" {
		t.Fatalf("got %v", out)
	}
	// a file over the server's limit is that file's error too, without being sent
	out = runHbbQuery(t, `invoke_hbb({Path: "big.c", Text: "`+strings.Repeat("x", 900)+`"}; `+o+`) | .Error`)
	if s, _ := out[0].(string); !strings.Contains(s, "per request") {
		t.Fatalf("got %v", out)
	}
	// a wrong token is hbb being unusable: raised
	out = runHbbQuery(t, `invoke_hbb({Path: "a.c", Text: "x"}; {Url: "`+srv.URL+`", Token: "nope"})`)
	if s, _ := out[0].(string); !strings.HasPrefix(s, "ERR: ") || !strings.Contains(s, "Token") {
		t.Fatalf("got %v", out)
	}
}

func TestHbbOverHTTPUnreachableIsRaisedAfterOneRetry(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	out := runHbbQuery(t, `get_hbb({Url: "`+url+`"})`)
	if s, _ := out[0].(string); !strings.HasPrefix(s, "ERR: ") || !strings.Contains(s, "what it is") {
		t.Fatalf("got %v", out)
	}
}

func TestHbbOverHTTPRetriesAServerThatWasBusy(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"protocol":1,"model":"m"}`))
	}))
	defer srv.Close()
	out := runHbbQuery(t, `get_hbb({Url: "`+srv.URL+`"}) | .Model`)
	if out[0] != "m" || n.Load() != 2 {
		t.Fatalf("got %v after %d requests", out, n.Load())
	}
}

func TestHbbOverHTTPRefusesAnOtherProtocol(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"protocol":2}`))
	}))
	defer srv.Close()
	out := runHbbQuery(t, `get_hbb({Url: "`+srv.URL+`"})`)
	if s, _ := out[0].(string); !strings.Contains(s, "protocol 2") {
		t.Fatalf("got %v", out)
	}
}

func TestHbbUrlExcludesLocalChildOptions(t *testing.T) {
	out := runHbbQuery(t, `get_hbb({Url: "http://x", Bin: "hbb", Device: "cpu"})`)
	if s, _ := out[0].(string); !strings.Contains(s, "Bin, Device") {
		t.Fatalf("got %v", out)
	}
	out = runHbbQuery(t, `get_hbb({Token: "t"})`)
	if s, _ := out[0].(string); !strings.Contains(s, "Token is for") {
		t.Fatalf("got %v", out)
	}
}
