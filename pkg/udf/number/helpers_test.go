package number

import (
	"strings"
	"testing"

	"github.com/itchyny/gojq"
)

func run(t *testing.T, query string, input any) any {
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

func runErr(t *testing.T, query string) error {
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
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if e, isErr := v.(error); isErr {
			return e
		}
	}
	t.Fatalf("expected %q to fail", query)
	return nil
}

var _ = strings.Contains
