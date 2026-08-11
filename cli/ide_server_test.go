//go:build viz

package cli

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

// testDist is the shape make web.build produces: a bundled page, the runtime
// files that are loaded by URL, and a pre-compressed copy of the module.
func testDist(t *testing.T) fstest.MapFS {
	t.Helper()

	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	if _, err := zw.Write([]byte("fake wasm module")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	return fstest.MapFS{
		"index.html":      {Data: []byte("<!DOCTYPE html><title>pwrq</title>")},
		"index-abc123.js": {Data: []byte("console.log(1)")},
		"worker.js":       {Data: []byte("// worker")},
		"web.wasm":        {Data: []byte("fake wasm module")},
		"web.wasm.gz":     {Data: compressed.Bytes()},
	}
}

func get(t *testing.T, dist fstest.MapFS, path string, headers map[string]string) *http.Response {
	t.Helper()
	handler := http.StripPrefix(routePrefix, assetHandler(dist, http.FileServer(http.FS(dist))))

	req := httptest.NewRequest(http.MethodGet, routePrefix+path, nil)
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Result()
}

// TestWasmIsServedCompressed is the difference between the page taking one
// second to become usable and taking four: the module is 27 MB uncompressed
// and a quarter of that gzipped.
func TestWasmIsServedCompressed(t *testing.T) {
	dist := testDist(t)
	resp := get(t, dist, "/web.wasm", map[string]string{"Accept-Encoding": "gzip, deflate"})
	defer func() { _ = resp.Body.Close() }()

	if got := resp.Header.Get("Content-Encoding"); got != "gzip" {
		t.Errorf("Content-Encoding = %q, want gzip", got)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/wasm" {
		t.Errorf("Content-Type = %q, want application/wasm; the browser refuses anything else", got)
	}
	if got := resp.Header.Get("Vary"); got != "Accept-Encoding" {
		t.Errorf("Vary = %q, want Accept-Encoding so a cache cannot mix the two forms up", got)
	}

	// What arrives has to be the module, not a description of it.
	reader, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatalf("the body is not gzip: %v", err)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "fake wasm module" {
		t.Errorf("body = %q", body)
	}
}

// TestWasmFallsBackWhenGzipIsNotAccepted covers the client that cannot take it,
// which must still get a working module.
func TestWasmFallsBackWhenGzipIsNotAccepted(t *testing.T) {
	dist := testDist(t)
	resp := get(t, dist, "/web.wasm", nil)
	defer func() { _ = resp.Body.Close() }()

	if got := resp.Header.Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q, want none", got)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "fake wasm module" {
		t.Errorf("body = %q", body)
	}
}

// TestWasmWithoutAPrecompressedCopy is the state of a tree built before the
// gzip step existed, or one where gzip was missing: the page must still work.
func TestWasmWithoutAPrecompressedCopy(t *testing.T) {
	dist := testDist(t)
	delete(dist, "web.wasm.gz")

	resp := get(t, dist, "/web.wasm", map[string]string{"Accept-Encoding": "gzip"})
	defer func() { _ = resp.Body.Close() }()

	if got := resp.Header.Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q, want none when there is nothing pre-compressed", got)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "fake wasm module" {
		t.Errorf("body = %q", body)
	}
}

// TestCaching keeps the two kinds of asset apart: the bundle carries a content
// hash and can be cached forever, everything else has to be revalidated or a
// rebuild would be invisible to a returning browser.
func TestCaching(t *testing.T) {
	dist := testDist(t)

	hashed := get(t, dist, "/index-abc123.js", nil)
	defer func() { _ = hashed.Body.Close() }()
	if got := hashed.Header.Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("hashed asset Cache-Control = %q", got)
	}

	for _, path := range []string{"/", "/index.html", "/worker.js", "/web.wasm"} {
		resp := get(t, dist, path, nil)
		_ = resp.Body.Close()
		if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
			t.Errorf("%s Cache-Control = %q, want no-cache", path, got)
		}
	}
}

func TestIndexIsServedAtTheRoot(t *testing.T) {
	dist := testDist(t)
	resp := get(t, dist, "/", nil)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(body, []byte("pwrq")) {
		t.Errorf("body = %q, want the editor page", body)
	}
}
