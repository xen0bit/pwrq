package csv

import (
	"fmt"
	"testing"

	"github.com/itchyny/gojq"
)

func runTSV(t *testing.T, query string, options ...gojq.CompilerOption) any {
	t.Helper()
	q, err := gojq.Parse(query)
	if err != nil {
		t.Fatalf("parse %q: %v", query, err)
	}
	code, err := gojq.Compile(q, options...)
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

func TestTSV(t *testing.T) {
	got := runTSV(t, `"a\tb\n1\t2" | tsv_parse`, RegisterTSVParse())
	if fmt.Sprint(got) != "[[a b] [1 2]]" {
		t.Errorf("tsv_parse = %v", got)
	}

	round := runTSV(t, `"a\tb\n1\t2" | tsv_parse | tsv_stringify`, RegisterTSVParse(), RegisterTSVStringify())
	if fmt.Sprint(round) != "a\tb\n1\t2\n" {
		t.Errorf("tsv round-trip = %q", round)
	}
}
