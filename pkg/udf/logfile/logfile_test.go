package logfile

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/itchyny/gojq"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sample.log")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

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

func TestHeadTail(t *testing.T) {
	path := writeTemp(t, "one\ntwo\nthree\nfour\nfive\n")
	head := run(t, fmt.Sprintf(`head("%s"; 2)`, path), "")
	arr := head.([]any)
	if len(arr) != 2 || arr[0] != "one" || arr[1] != "two" {
		t.Errorf("head = %v", arr)
	}
	tail := run(t, fmt.Sprintf(`tail("%s"; 2)`, path), "")
	arr = tail.([]any)
	if len(arr) != 2 || arr[0] != "four" || arr[1] != "five" {
		t.Errorf("tail = %v", arr)
	}
}

func TestGrepLines(t *testing.T) {
	path := writeTemp(t, "ok\nfail: timeout\nerror: disk full\nok\n")
	got := run(t, fmt.Sprintf(`grep_lines("%s"; "error|fail")`, path), "")
	arr := got.([]any)
	if len(arr) != 2 {
		t.Fatalf("grep_lines = %v, want 2 matches", arr)
	}
	if arr[0] != "fail: timeout" || arr[1] != "error: disk full" {
		t.Errorf("grep_lines = %v", arr)
	}
}

func TestWcLines(t *testing.T) {
	path := writeTemp(t, "a\nb\nc\n")
	if got := fmt.Sprint(run(t, fmt.Sprintf(`wc_lines("%s")`, path), "")); got != "3" {
		t.Errorf("wc_lines = %s, want 3", got)
	}
}
