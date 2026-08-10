package yaml

import (
	"fmt"
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

func TestYAMLParse(t *testing.T) {
	got := run(t, `"name: ada\nrole: engineer\n" | yaml_parse`)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("yaml_parse = %T, want an object", got)
	}
	if fmt.Sprint(m["name"]) != "ada" || fmt.Sprint(m["role"]) != "engineer" {
		t.Errorf("yaml_parse = %v", m)
	}
}

func TestYAMLStringify(t *testing.T) {
	got := fmt.Sprint(run(t, `{name: "ada", role: "engineer"} | yaml_stringify`))
	if got != "name: ada\nrole: engineer\n" {
		t.Errorf("yaml_stringify = %q", got)
	}
}

func TestYAMLRoundTrip(t *testing.T) {
	got := run(t, `"port: 8080\nhost: db-01\n" | yaml_parse`)
	m := got.(map[string]any)
	if fmt.Sprint(m["port"]) != "8080" || fmt.Sprint(m["host"]) != "db-01" {
		t.Errorf("yaml round trip = %v", m)
	}
}
