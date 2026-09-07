//go:build viz && !ide_native

package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// TestWASMServerHasNoAPI is the isolation guarantee the safe deployment
// depends on: the WASM-only IDE mux serves the static page and answers 404
// under every API path, because it has no such route. A future change that
// mounts native behaviour on --ide fails here before it ships.
func TestWASMServerHasNoAPI(t *testing.T) {
	dist := fstest.MapFS{
		"index.html": {Data: []byte("<!DOCTYPE html><title>pwrq</title>")},
	}
	mux := newIDEMux(dist)

	for _, target := range []string{"/api/health", "/api/call", "/api/anything"} {
		req := httptest.NewRequest(http.MethodGet, routePrefix+target, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404: the WASM-only server must not answer API paths", target, rec.Code)
		}

		post := httptest.NewRequest(http.MethodPost, routePrefix+target,
			strings.NewReader(`{"method":"run","request":"{}"}`))
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, post)
		if rec.Code != http.StatusNotFound {
			t.Errorf("POST %s = %d, want 404: the WASM-only server must not answer API paths", target, rec.Code)
		}
	}

	// And the page itself still serves.
	req := httptest.NewRequest(http.MethodGet, routePrefix+"/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("GET / = %d, want the editor page", rec.Code)
	}
}

// TestNativeStubExplainsTheBuild keeps the error actionable: naming the flag
// without the tag must say which tag is missing, not fail to parse.
func TestNativeStubExplainsTheBuild(t *testing.T) {
	c := &cli{}
	err := c.launchNativeIDE()
	if err == nil {
		t.Fatal("expected an error from the stub")
	}
	if !strings.Contains(err.Error(), "ide_native") {
		t.Errorf("error = %q, want it to name the missing build tag", err)
	}
}
