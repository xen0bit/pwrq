package string

import (
	"fmt"
	"testing"

	"github.com/itchyny/gojq"
)

func run(t *testing.T, query string, input any, options ...gojq.CompilerOption) any {
	t.Helper()
	q, err := gojq.Parse(query)
	if err != nil {
		t.Fatalf("parse %q: %v", query, err)
	}
	code, err := gojq.Compile(q, options...)
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

func TestCaseConverters(t *testing.T) {
	tests := []struct {
		name, query, want string
	}{
		{"slugify basic", `"Hello, World!" | slugify`, "hello-world"},
		{"slugify camel", `"fooBar Baz" | slugify`, "foo-bar-baz"},
		{"snake_case", `"FooBar baz" | snake_case`, "foo_bar_baz"},
		{"kebab_case", `"FooBar baz" | kebab_case`, "foo-bar-baz"},
		{"camel_case", `"hello world foo" | camel_case`, "helloWorldFoo"},
		{"pascal_case", `"hello world foo" | pascal_case`, "HelloWorldFoo"},
		{"title_case", `"the quick brown fox" | title_case`, "The Quick Brown Fox"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, tt.query, nil, RegisterAll()...)
			if got != tt.want {
				t.Errorf("%s = %q, want %q", tt.query, got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		query string
		input any
		want  string
	}{
		{`"hello world" | truncate(5)`, nil, "hello…"},
		{`"hello" | truncate(10)`, nil, "hello"},
		{`"hello world" | truncate(5; "…")`, nil, "hello…"},
		{`"hello world" | truncate(5; "[...]")`, nil, "hello[...]"},
	}
	for _, tt := range tests {
		got := run(t, tt.query, tt.input, RegisterAll()...)
		if got != tt.want {
			t.Errorf("%s = %q, want %q", tt.query, got, tt.want)
		}
	}
}

func TestPad(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{`"5" | pad_left(3; "0")`, "005"},
		{`"5" | pad_right(3; "0")`, "500"},
		{`"hi" | pad_left(4)`, "  hi"},
		{`"longer" | pad_left(3)`, "longer"},
	}
	for _, tt := range tests {
		got := run(t, tt.query, nil, RegisterAll()...)
		if got != tt.want {
			t.Errorf("%s = %q, want %q", tt.query, got, tt.want)
		}
	}
}

func TestMask(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{`"hunter2" | mask`, "*******"},
		{`"hunter2" | mask(2)`, "hu***r2"},
		{`"ab" | mask(2)`, "ab"},
		{`"1234" | mask(1)`, "1**4"},
	}
	for _, tt := range tests {
		got := run(t, tt.query, nil, RegisterAll()...)
		if got != tt.want {
			t.Errorf("%s = %q, want %q", tt.query, got, tt.want)
		}
	}
}

func TestCountOccurrences(t *testing.T) {
	got := run(t, `"banana" | count_occurrences("an")`, nil, RegisterAll()...)
	if fmt.Sprint(got) != "2" {
		t.Errorf("count_occurrences = %v, want 2", got)
	}
}

func TestSlugifyThroughPipeline(t *testing.T) {
	// The registered function must be reachable through an actual pipeline,
	// not just as a re-implementation.
	got := run(t, `"Hello, World!" | slugify`, nil, RegisterSlugify())
	if got != "hello-world" {
		t.Errorf("slugify = %q, want %q", got, "hello-world")
	}
}
