package assets

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLockPinsEverything(t *testing.T) {
	l, err := Locked()
	if err != nil {
		t.Fatal(err)
	}
	if l.Model.Repo != "LiquidAI/d1-2B-ONNX" || len(l.Model.Revision) != 40 {
		t.Fatalf("model %s@%s", l.Model.Repo, l.Model.Revision)
	}
	for _, name := range append(append([]string{}, l.Model.CPU...), l.Model.GPU...) {
		f, ok := l.Model.Files[name]
		if !ok || len(f.SHA256) != 64 || f.Size <= 0 {
			t.Errorf("%s is not pinned: %+v", name, f)
		}
	}
	for _, plat := range []string{"linux/amd64", "linux/arm64", "darwin/arm64", "windows/amd64", "windows/arm64"} {
		w, ok := l.ONNXRuntime.CPU[plat]
		if !ok || len(w.SHA256) != 64 || len(w.Libs) != 1 || !strings.HasPrefix(w.URL, "https://files.pythonhosted.org/") {
			t.Errorf("no ONNX Runtime pinned for %s: %+v", plat, w)
		}
	}
	for plat := range l.ONNXRuntime.CUDA {
		if len(l.CUDA[plat]) == 0 {
			t.Errorf("a CUDA ONNX Runtime for %s but no CUDA libraries", plat)
		}
	}
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// TestFetchResumesAndVerifies serves a file that fails halfway the first
// time: the second request must ask for the rest, and the result must pass
// the checksum.
func TestFetchResumesAndVerifies(t *testing.T) {
	body := bytes.Repeat([]byte("d1-weights "), 10000)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if rng := r.Header.Get("Range"); rng != "" {
			var start int
			_, _ = fmt.Sscanf(rng, "bytes=%d-", &start)
			w.Header().Set("Content-Range", "bytes")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(body[start:])
			return
		}
		if calls == 1 {
			w.Header().Set("Content-Length", "110000")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body[:50000])
			return // the client sees an unexpected EOF
		}
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	dir := t.TempDir()
	dst := filepath.Join(dir, "w.bin")
	o := &Options{CacheDir: dir}
	if err := fetch(context.Background(), o, srv.URL, dst, sum(body), int64(len(body)), nil); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dst)
	if !bytes.Equal(got, body) {
		t.Fatal("downloaded bytes differ")
	}
	if ok, _ := verified(dst, sum(body), int64(len(body))); !ok {
		t.Fatal("not verified after the download")
	}
	if calls < 2 {
		t.Errorf("%d requests; the interrupted download was not resumed", calls)
	}
}

func TestFetchRejectsWrongChecksum(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("tampered")) }))
	defer srv.Close()
	dir := t.TempDir()
	dst := filepath.Join(dir, "w.bin")
	err := fetch(context.Background(), &Options{CacheDir: dir}, srv.URL, dst, sum([]byte("original")), 8, nil)
	if err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("got %v, want a checksum error", err)
	}
	if _, err := os.Stat(dst); err == nil {
		t.Fatal("a file that failed its checksum was kept")
	}
}

func TestOfflineRefuses(t *testing.T) {
	dir := t.TempDir()
	err := fetch(context.Background(), &Options{CacheDir: dir, Offline: true}, "http://unused", filepath.Join(dir, "x"), "", 0, nil)
	if !errors.Is(err, ErrOffline) {
		t.Fatalf("got %v, want ErrOffline", err)
	}
}

// TestUnzipKeepsOnlyWhatIsAsked mirrors taking ONNX Runtime out of its
// wheel: the library and its providers, flat, nothing else.
func TestUnzipKeepsOnlyWhatIsAsked(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range []string{"onnxruntime/capi/libonnxruntime.so.1.30.0", "onnxruntime/capi/libonnxruntime_providers_shared.so", "onnxruntime/__init__.py"} {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte(name))
	}
	_ = zw.Close()
	dir := t.TempDir()
	archive := filepath.Join(dir, "x.whl")
	_ = os.WriteFile(archive, buf.Bytes(), 0o644)
	out := filepath.Join(dir, "out")
	keep := func(name string) bool {
		return name == "onnxruntime/capi/libonnxruntime.so.1.30.0" || companions.MatchString(filepath.Base(name))
	}
	if err := unzip(archive, out, keep); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(out)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "libonnxruntime.so.1.30.0,libonnxruntime_providers_shared.so" {
		t.Fatalf("unpacked %v", names)
	}
}

func TestModelNotFetchedIsAnError(t *testing.T) {
	_, err := ResolveModel(context.Background(), ModelOptions{Options: Options{CacheDir: t.TempDir()}})
	if !errors.Is(err, ErrNotFetched) {
		t.Fatalf("got %v, want ErrNotFetched", err)
	}
}
