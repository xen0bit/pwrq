package path

import (
	"testing"

	"github.com/itchyny/gojq"
)

func run(t *testing.T, query string) any {
	t.Helper()
	q, err := gojq.Parse(query)
	if err != nil {
		t.Fatalf("parse %q: %v", query, err)
	}
	code, err := gojq.Compile(q, RegisterAll()...)
	if err != nil {
		t.Fatalf("compile %q: %v", query, err)
	}
	iter := code.Run(nil)
	v, ok := iter.Next()
	if !ok {
		t.Fatalf("%q produced no result", query)
	}
	if e, isErr := v.(error); isErr {
		t.Fatalf("%q: %v", query, e)
	}
	return v
}

func TestPathCmdlets(t *testing.T) {
	tests := []struct {
		query, want string
	}{
		{`"/tmp/data/file.txt" | basename`, "file.txt"},
		{`"/tmp/data/file.txt" | dirname`, "/tmp/data"},
		{`"/tmp/data/file.txt" | file_extension`, ".txt"},
		{`"/tmp/noext" | file_extension`, ""},
		{`basename("/a/b/c")`, "c"},
		{`dirname("/a/b/c")`, "/a/b"},
	}
	for _, tt := range tests {
		got := run(t, tt.query)
		if got != tt.want {
			t.Errorf("%s = %v, want %q", tt.query, got, tt.want)
		}
	}
}

func TestIsAbsolute(t *testing.T) {
	if got := run(t, `"/tmp/x" | is_absolute`); got != true {
		t.Errorf("is_absolute absolute = %v", got)
	}
	if got := run(t, `"rel/x" | is_absolute`); got != false {
		t.Errorf("is_absolute relative = %v", got)
	}
}

