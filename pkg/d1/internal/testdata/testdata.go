// Package testdata loads the fixtures pkg/d1's packages share: d1-2B's real
// tokenizer (gzipped, 2.5 MB) and the reference outputs written from the
// Python d1 code by internal/tools/d1fixtures. With the real tokenizer in
// the repository, every prompt the Go port builds is checked id for id in CI
// without the two-gigabyte model.
package testdata

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// Dir is pkg/d1/testdata.
func Dir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata")
}

var (
	tokOnce sync.Once
	tokRaw  []byte
	tokErr  error
)

// TokenizerJSON is d1-2B's tokenizer.json.
func TokenizerJSON(t testing.TB) []byte {
	t.Helper()
	tokOnce.Do(func() {
		var raw []byte
		raw, tokErr = os.ReadFile(filepath.Join(Dir(), "tokenizer.json.gz"))
		if tokErr != nil {
			return
		}
		var zr *gzip.Reader
		zr, tokErr = gzip.NewReader(bytes.NewReader(raw))
		if tokErr != nil {
			return
		}
		tokRaw, tokErr = io.ReadAll(zr)
	})
	if tokErr != nil {
		t.Fatal(tokErr)
	}
	return tokRaw
}

// Load decodes a JSON fixture from pkg/d1/testdata.
func Load(t testing.TB, name string, into any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(Dir(), name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}
