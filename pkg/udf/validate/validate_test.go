package validate

import (
	"fmt"
	"reflect"
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

func TestValidators(t *testing.T) {
	tests := []struct {
		query string
		want  bool
	}{
		{`"ada@example.com" | is_email`, true},
		{`"not-an-email" | is_email`, false},
		{`"https://example.com/path" | is_url`, true},
		{`"example.com" | is_url`, false},
		{`"example.com" | is_domain`, true},
		{`"not a domain!" | is_domain`, false},
		{`"{\"a\":1}" | is_json`, true},
		{`"nope" | is_json`, false},
	}
	for _, tt := range tests {
		if got := run(t, tt.query); got != tt.want {
			t.Errorf("%s = %v, want %v", tt.query, got, tt.want)
		}
	}
}

func TestExtractors(t *testing.T) {
	emails := run(t, `"contact ada@example.com or bob@example.org" | extract_emails`)
	if got := asStrings(emails); !reflect.DeepEqual(got, []string{"ada@example.com", "bob@example.org"}) {
		t.Errorf("extract_emails = %v", got)
	}
	urls := run(t, `"see https://a.com/x and http://b.org/y" | extract_urls`)
	if got := asStrings(urls); !reflect.DeepEqual(got, []string{"https://a.com/x", "http://b.org/y"}) {
		t.Errorf("extract_urls = %v", got)
	}
	ips := run(t, `"from 10.0.0.1 and 192.168.1.1" | extract_ips`)
	if got := asStrings(ips); !reflect.DeepEqual(got, []string{"10.0.0.1", "192.168.1.1"}) {
		t.Errorf("extract_ips = %v", got)
	}
}

func TestStripTags(t *testing.T) {
	got := fmt.Sprint(run(t, `"<b>hello</b> <i>world</i>" | strip_tags`))
	if got != "hello world" {
		t.Errorf("strip_tags = %q", got)
	}
}

func asStrings(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, len(arr))
	for i, item := range arr {
		out[i] = fmt.Sprint(item)
	}
	return out
}
