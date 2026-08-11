package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/itchyny/gojq"
)

func arun(t *testing.T, query string, input any) any {
	t.Helper()
	q, err := gojq.Parse(query)
	if err != nil {
		t.Fatalf("parse %q: %v", query, err)
	}
	code, err := gojq.Compile(q, RegisterAll()...)
	if err != nil {
		t.Fatalf("compile %q: %v", query, err)
	}
	v, ok := code.Run(input).Next()
	if !ok {
		t.Fatalf("%q produced no result", query)
	}
	if e, isErr := v.(error); isErr {
		t.Fatalf("%q: %v", query, e)
	}
	return v
}

func TestAddContentAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.log")
	arun(t, fmt.Sprintf(`"first" | add_content(%q)`, path), nil)
	arun(t, fmt.Sprintf(`"second" | add_content(%q)`, path), nil)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := "first\nsecond\n"; string(got) != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

// TestAddContentDoesNotTruncate is the difference from set_content, and the
// reason the cmdlet exists.
func TestAddContentDoesNotTruncate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.log")
	arun(t, fmt.Sprintf(`set_content(%q; "kept")`, path), nil)
	arun(t, fmt.Sprintf(`"added" | add_content(%q)`, path), nil)
	got, _ := os.ReadFile(path)
	if want := "kept" + "\nadded\n"; string(got) != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

func TestAddContentExplicitValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.log")
	arun(t, fmt.Sprintf(`add_content(%q; "explicit")`, path), nil)
	got, _ := os.ReadFile(path)
	if want := "explicit\n"; string(got) != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

func TestAddContentArrayWritesOneLinePerElement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.log")
	arun(t, fmt.Sprintf(`["a","b"] | add_content(%q)`, path), nil)
	got, _ := os.ReadFile(path)
	if want := "a\nb\n"; string(got) != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

func TestAddContentForceCreatesParents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deep", "out.log")
	arun(t, fmt.Sprintf(`"x" | add_content(%q; {Force: true})`, path), nil)
	if _, err := os.Stat(path); err != nil {
		t.Errorf("Force did not create the parent directories: %v", err)
	}
}

// TestOutFilePassesTheValueThrough is what separates out_file from
// add_content: the pipeline carries on with the value it wrote.
func TestOutFilePassesTheValueThrough(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.txt")
	got := arun(t, fmt.Sprintf(`"payload" | out_file(%q)`, path), nil)
	if got != "payload" {
		t.Errorf("out_file returned %v, want the input value", got)
	}
	body, _ := os.ReadFile(path)
	if string(body) != "payload" {
		t.Errorf("file = %q, want %q", body, "payload")
	}
}

func TestOutFileTruncatesUnlessAppending(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.log")
	arun(t, fmt.Sprintf(`"one" | out_file(%q)`, path), nil)
	arun(t, fmt.Sprintf(`"two" | out_file(%q)`, path), nil)
	got, _ := os.ReadFile(path)
	if string(got) != "two" {
		t.Errorf("out_file should truncate: file = %q", got)
	}

	path2 := filepath.Join(t.TempDir(), "append.log")
	arun(t, fmt.Sprintf(`"one" | out_file(%q; {Append: true})`, path2), nil)
	arun(t, fmt.Sprintf(`"two" | out_file(%q; {Append: true})`, path2), nil)
	got2, _ := os.ReadFile(path2)
	if want := "one\ntwo\n"; string(got2) != want {
		t.Errorf("appending out_file = %q, want %q", got2, want)
	}
}

func TestAddContentRejectsUnknownOption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.log")
	q, _ := gojq.Parse(fmt.Sprintf(`"x" | add_content(%q; {Nonsense: true})`, path))
	code, _ := gojq.Compile(q, RegisterAll()...)
	v, _ := code.Run(nil).Next()
	if _, isErr := v.(error); !isErr {
		t.Errorf("unknown option should be an error, got %v", v)
	}
}
