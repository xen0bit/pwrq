package udf

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCmdletsRegisterThroughCommon pins the invariant the whole value-space
// defence rests on: every cmdlet goes through common.WithFunction or
// common.WithIterFunction, which normalize results into gojq's value space.
//
// Registering straight with gojq.WithFunction skips that, and the damage does
// not show up where the mistake was made: a cmdlet that returns an int32 gets
// encoded perfectly well until some unrelated query calls `type` on the field
// and gojq panics, killing an MCP server that has no recover in its dispatch
// path. A grep is a blunt instrument, but this is a case where the compiler
// cannot help and the failure is remote from the cause.
func TestCmdletsRegisterThroughCommon(t *testing.T) {
	root := "."
	var offenders []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		// The wrappers themselves are the one place that may call gojq
		// directly - that is what they are for.
		if filepath.ToSlash(path) == "common/register.go" {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(src), "\n") {
			if strings.Contains(line, "gojq.WithFunction(") || strings.Contains(line, "gojq.WithIterFunction(") {
				offenders = append(offenders, fmt.Sprintf("pkg/udf/%s:%d: %s",
					filepath.ToSlash(path), i+1, strings.TrimSpace(line)))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	for _, o := range offenders {
		t.Errorf("cmdlet registered directly with gojq, bypassing result "+
			"normalization; use common.WithFunction / common.WithIterFunction:\n  %s", o)
	}
}
