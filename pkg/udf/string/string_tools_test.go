package string

import (
	"fmt"
	"testing"
)

func TestStripANSI(t *testing.T) {
	if got := fmt.Sprint(run(t, `"\u001b[31mred\u001b[0m" | strip_ansi`, nil, RegisterAll()...)); got != "red" {
		t.Errorf("strip_ansi = %q", got)
	}
	if got := fmt.Sprint(run(t, `"\u001b[1;32mgreen\u001b[m plain" | strip_ansi`, nil, RegisterAll()...)); got != "green plain" {
		t.Errorf("strip_ansi = %q", got)
	}
}

func TestTemplate(t *testing.T) {
	if got := fmt.Sprint(run(t, `"hello {{name}} ({{count}})" | template({name: "ada", count: 3})`, nil, RegisterAll()...)); got != "hello ada (3)" {
		t.Errorf("template = %q", got)
	}
	if got := fmt.Sprint(run(t, `"{{ user }}@{{host}}" | template({user: "root", host: "db-01"})`, nil, RegisterAll()...)); got != "root@db-01" {
		t.Errorf("template spaced = %q", got)
	}
}

func TestWrapText(t *testing.T) {
	got := run(t, `"the quick brown fox jumps" | wrap_text(10)`, nil, RegisterAll()...)
	arr, ok := got.([]any)
	if !ok || len(arr) != 3 {
		t.Fatalf("wrap_text = %v", got)
	}
	if arr[0] != "the quick" || arr[1] != "brown fox" || arr[2] != "jumps" {
		t.Errorf("wrap_text = %v", arr)
	}
}

func TestIndent(t *testing.T) {
	if got := fmt.Sprint(run(t, `"a\nb" | indent(2)`, nil, RegisterAll()...)); got != "  a\n  b" {
		t.Errorf("indent = %q", got)
	}
}
