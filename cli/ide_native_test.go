//go:build viz && ide_native

package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/xen0bit/pwrq/pkg/webnative"
)

// nativeTestDist is the shape make web.build-native produces: the native
// editor page at the root (as index.html, so the bare address lands on it)
// and its hashed bundle, without any WASM module.
func nativeTestDist() fstest.MapFS {
	return fstest.MapFS{
		"index.html":        {Data: []byte("<!DOCTYPE html><title>pwrq native</title>")},
		"index-native.html": {Data: []byte("<!DOCTYPE html><title>pwrq native</title>")},
		"index-abc123.js":   {Data: []byte("console.log(1)")},
	}
}

func nativeMux(t *testing.T) *http.ServeMux {
	t.Helper()
	t.Setenv("PWRQ_IDE_TOKEN", "")
	t.Setenv("PWRQ_MCP_TOKEN", "")
	return newNativeMux(nativeTestDist(), webnative.New())
}

func serveNative(mux *http.ServeMux, method, path string, body []byte, headers map[string]string) *http.Response {
	req := httptest.NewRequest(method, routePrefix+path, bytes.NewReader(body))
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	// httptest requests arrive as loopback remotes; auth tests override it.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Result()
}

func postCall(t *testing.T, mux *http.ServeMux, method, request string, headers map[string]string) *http.Response {
	t.Helper()
	envelope, err := json.Marshal(map[string]any{"method": method, "request": request})
	if err != nil {
		t.Fatal(err)
	}
	return serveNative(mux, http.MethodPost, "/api/call", envelope, headers)
}

func runRequest(t *testing.T, query string) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{"query": query, "compact": true})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func decodeNativeJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("response is not JSON: %v\n%s", err, body)
	}
	return decoded
}

// TestNativeHealthNamesTheServer is what the page's banner reads: the mode,
// the vocabulary size and where queries run.
func TestNativeHealthNamesTheServer(t *testing.T) {
	resp := serveNative(nativeMux(t), http.MethodGet, "/api/health", nil, nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	health := decodeNativeJSON(t, resp)
	if health["mode"] != "native" {
		t.Errorf("mode = %v, want native", health["mode"])
	}
	if n, ok := health["cmdlets"].(float64); !ok || n == 0 {
		t.Errorf("cmdlets = %v, want the vocabulary size", health["cmdlets"])
	}
	if cwd, ok := health["cwd"].(string); !ok || cwd == "" {
		t.Errorf("cwd = %v, want the server working directory", health["cwd"])
	}
}

// TestNativeRunEvaluatesNativeCmdlets is the capability under test for the
// whole target: the filesystem, from the page protocol.
func TestNativeRunEvaluatesNativeCmdlets(t *testing.T) {
	mux := nativeMux(t)
	dir := t.TempDir()

	dirJSON, err := json.Marshal(dir)
	if err != nil {
		t.Fatal(err)
	}
	request, err := json.Marshal(map[string]any{
		"query":   `[get_childitem($dir)] | map(.Name)`,
		"args":    []any{map[string]any{"name": "dir", "value": string(dirJSON)}},
		"compact": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	resp := postCall(t, mux, "run", string(request), nil)
	decoded := decodeNativeJSON(t, resp)
	if errText, _ := decoded["error"].(string); errText != "" {
		t.Fatalf("run failed: %s", errText)
	}
	// An empty temp dir lists nothing, but the run itself must succeed: the
	// cmdlet resolved and read the directory.
	if _, ok := decoded["values"]; !ok {
		t.Errorf("response has no values: %v", decoded)
	}
}

// TestNativeValidateCompiles guards the native-only improvement over the WASM
// page: unknown names fail validation, not just the run.
func TestNativeValidateCompiles(t *testing.T) {
	mux := nativeMux(t)
	resp := postCall(t, mux, "run", runRequest(t, "nosuchcmdlet_zz9"), nil)
	decoded := decodeNativeJSON(t, resp)
	if kind, _ := decoded["kind"].(string); kind != "compile" {
		t.Errorf("kind = %v, want compile", decoded)
	}
}

// TestNativeCatalogIsFullyAvailable pins the honesty property: everything
// listed runs here.
func TestNativeCatalogIsFullyAvailable(t *testing.T) {
	mux := nativeMux(t)
	resp := postCall(t, mux, "catalog", `{}`, nil)
	decoded := decodeNativeJSON(t, resp)
	commands, ok := decoded["commands"].([]any)
	if !ok || len(commands) == 0 {
		t.Fatalf("catalog is empty: %v", decoded)
	}
	for _, entry := range commands {
		cmd, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if available, _ := cmd["available"].(bool); !available {
			t.Errorf("the native catalog marks %v unavailable", cmd["name"])
			break
		}
	}
}

// TestNativeCallRejectsUnknownMethods keeps the protocol honest: a typo is a
// JSON error, not a silent 200 with no body.
func TestNativeCallRejectsUnknownMethods(t *testing.T) {
	mux := nativeMux(t)
	resp := postCall(t, mux, "nope", `{}`, nil)
	decoded := decodeNativeJSON(t, resp)
	if errText, _ := decoded["error"].(string); !strings.Contains(errText, "nope") {
		t.Errorf("error = %v, want it to name the unknown method", decoded)
	}
}

// TestNativeCallNeedsJSON: a body that is not even the envelope stays JSON.
func TestNativeCallNeedsJSON(t *testing.T) {
	mux := nativeMux(t)
	resp := serveNative(mux, http.MethodPost, "/api/call", []byte("{not json"), nil)
	_ = decodeNativeJSON(t, resp)
}

// TestNativePageHasNoWasm: the native page is not the WASM page served twice.
func TestNativePageHasNoWasm(t *testing.T) {
	mux := nativeMux(t)
	req := httptest.NewRequest(http.MethodGet, routePrefix+"/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	resp := rec.Result()
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(body, []byte("pwrq native")) {
		t.Errorf("body = %q, want the native editor page", body)
	}
}

// TestNativeAPICaching: API answers are never cached; hashed bundles are.
func TestNativeAPICaching(t *testing.T) {
	mux := nativeMux(t)
	for _, path := range []string{"/api/health", "/api/call"} {
		resp := serveNative(mux, http.MethodGet, path, nil, nil)
		_ = resp.Body.Close()
		if got := resp.Header.Get("Cache-Control"); got == "public, max-age=31536000, immutable" {
			t.Errorf("%s is cached as immutable, want revalidation", path)
		}
	}
}

// TestNativeBearerGatesTheAPI: with a token set, the API answers only to it.
// Health is gated too: it names the user and working directory.
func TestNativeBearerGatesTheAPI(t *testing.T) {
	t.Setenv("PWRQ_IDE_TOKEN", "test-secret")
	mux := newNativeMux(nativeTestDist(), webnative.New())

	for _, path := range []string{"/api/health", "/api/call"} {
		method := http.MethodGet
		var body []byte
		if strings.HasSuffix(path, "/call") {
			method = http.MethodPost
			body, _ = json.Marshal(map[string]any{"method": "catalog", "request": `{}`})
		}

		denied := serveNative(mux, method, path, body, nil)
		_ = denied.Body.Close()
		if denied.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s without a token: status = %d, want 401", path, denied.StatusCode)
		}

		wrong := serveNative(mux, method, path, body, map[string]string{"Authorization": "Bearer wrong"})
		_ = wrong.Body.Close()
		if wrong.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s with a wrong token: status = %d, want 401", path, wrong.StatusCode)
		}

		allowed := serveNative(mux, method, path, body, map[string]string{"Authorization": "Bearer test-secret"})
		_ = allowed.Body.Close()
		if allowed.StatusCode != http.StatusOK {
			t.Errorf("%s with the token: status = %d, want 200", path, allowed.StatusCode)
		}
	}
}

// TestNativeBearerFallsBackToMCPToken: one secret can gate both servers.
func TestNativeBearerFallsBackToMCPToken(t *testing.T) {
	t.Setenv("PWRQ_IDE_TOKEN", "")
	t.Setenv("PWRQ_MCP_TOKEN", "shared-secret")
	mux := newNativeMux(nativeTestDist(), webnative.New())

	denied := serveNative(mux, http.MethodGet, "/api/health", nil, nil)
	_ = denied.Body.Close()
	if denied.StatusCode != http.StatusUnauthorized {
		t.Errorf("without a token: status = %d, want 401", denied.StatusCode)
	}

	allowed := serveNative(mux, http.MethodGet, "/api/health", nil,
		map[string]string{"Authorization": "Bearer shared-secret"})
	_ = allowed.Body.Close()
	if allowed.StatusCode != http.StatusOK {
		t.Errorf("with the MCP token: status = %d, want 200", allowed.StatusCode)
	}
}
