package system

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/itchyny/gojq"
)

func run(t *testing.T, query, input string) any {
	t.Helper()
	q, err := gojq.Parse(query)
	if err != nil {
		t.Fatalf("parse %q: %v", query, err)
	}
	code, err := gojq.Compile(q, RegisterAll()...)
	if err != nil {
		t.Fatalf("compile %q: %v", query, err)
	}
	iter := code.Run(input)
	v, ok := iter.Next()
	if !ok {
		t.Fatalf("%q produced no result", query)
	}
	if e, isErr := v.(error); isErr {
		t.Fatalf("%q: %v", query, e)
	}
	return v
}

func TestWhich(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skip("cannot locate test binary")
	}
	dir := filepath.Dir(exe)
	name := filepath.Base(exe)
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", dir)
	got := fmt.Sprint(run(t, fmt.Sprintf(`which("%s")`, name), ""))
	if got != exe {
		t.Errorf("which(%s) = %s, want %s", name, got, exe)
	}
	_ = os.Setenv("PATH", oldPath)
}

func TestResolveHost(t *testing.T) {
	got := run(t, `resolve_host("localhost")`, "")
	arr, ok := got.([]any)
	if !ok || len(arr) == 0 {
		t.Skipf("no addresses for localhost (got %v)", got)
	}
}
