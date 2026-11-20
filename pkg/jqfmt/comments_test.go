package jqfmt

import (
	"fmt"
	"strings"
	"testing"

	"github.com/itchyny/gojq"
)

func TestFormatSourceKeepsComments(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			name: "no comments is Format",
			src:  ".a|.b",
			want: ".a\n| .b",
		},
		{
			name: "leading comment above a stage",
			src:  "# list the big files\nget_childitem(\"*.go\") | where_object(.Length > 1000)",
			want: "# list the big files\nget_childitem(\"*.go\")\n| where_object(.Length > 1000)",
		},
		{
			name: "trailing comments stay on their stage",
			src:  ".items[] # each item\n| select(.ok) # keep the good ones\n| .name",
			want: ".items[] # each item\n| select(.ok) # keep the good ones\n| .name",
		},
		{
			name: "a comment between stages goes above the next",
			src:  ".a |\n  # then b\n  .b",
			want: ".a\n# then b\n| .b",
		},
		{
			name: "a hash in a string is not a comment",
			src:  `"# not a comment" | . # but this is`,
			want: `"# not a comment"` + "\n| . # but this is",
		},
		{
			name: "a final comment stays at the end",
			src:  ".a | .b\n# the end",
			want: ".a\n| .b\n# the end\n",
		},
		{
			name: "definitions keep their comments",
			src:  "# square it\ndef sq: . * .;\n# then use it\n[1,2] | map(sq)",
		},
		{
			name: "continued comment (odd backslashes)",
			src:  "1 # a \\\n still the comment\n| . + 1",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := FormatSource(c.src)
			if err != nil {
				t.Fatal(err)
			}
			if c.want != "" && got != c.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, c.want)
			}
			assertSame(t, c.src, got)
		})
	}
}

// assertSame checks a formatting kept the program and every comment, in
// order, and that formatting again changes nothing.
func assertSame(t *testing.T, src, got string) {
	t.Helper()
	a, err := gojq.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	b, err := gojq.Parse(got)
	if err != nil {
		t.Fatalf("formatted text does not parse: %v\n%s", err, got)
	}
	if a.String() != b.String() {
		t.Fatalf("program changed:\n%s\n%s", a, b)
	}
	if want, have := commentsOf(src), commentsOf(got); strings.Join(want, "\x00") != strings.Join(have, "\x00") {
		t.Fatalf("comments %q, want %q\n%s", have, want, got)
	}
	again, err := FormatSource(got)
	if err != nil {
		t.Fatal(err)
	}
	if again != got {
		t.Errorf("not idempotent:\n%s\n---\n%s", got, again)
	}
}

func commentsOf(src string) []string {
	var out []string
	for _, tok := range lex(src) {
		if tok.comment {
			out = append(out, tok.text)
		}
	}
	return out
}

// TestCommentsSurviveEveryShape injects comments into queries of every
// shape the formatter lays out - after each top-level stage and on lines of
// their own - and checks nothing is lost or changed.
func TestCommentsSurviveEveryShape(t *testing.T) {
	queries := []string{
		`.[] | {name: .name, size: (.size / 1024 | floor), tags: [.tags[] | ascii_downcase]} | select(.size > 10)`,
		`reduce .[] as $x (0; . + $x) | if . > 10 then "big" elif . > 5 then "medium" else "small" end`,
		`def f($a): $a + 1; [range(5)] | map(f(.)) | try error("x") catch .`,
		`"name: \(.a | tostring)" | ascii_upcase | test("NAME"; "i")`,
		`.a as [$x, {b: $y}] | $x + $y | label $out | foreach .[] as $i (0; . + $i; if . > 3 then ., break $out else . end)`,
		`[paths(type == "number")] | length as $n | {count: $n, first: .[0]} | .count //= 0 | .first |= tostring`,
	}
	for i, q := range queries {
		stages := strings.Split(q, " | ")
		var b strings.Builder
		b.WriteString("# about query " + fmt.Sprint(i) + "\n")
		for k, s := range stages {
			if k > 0 {
				b.WriteString("\n| ")
			}
			b.WriteString(s)
			if k%2 == 0 {
				b.WriteString(" # stage " + fmt.Sprint(k))
			} else {
				b.WriteString("\n  # note before stage " + fmt.Sprint(k+1))
			}
		}
		src := b.String()
		if _, err := gojq.Parse(src); err != nil {
			t.Fatalf("test query %d does not parse: %v\n%s", i, err, src)
		}
		got, err := FormatSource(src)
		if err != nil {
			t.Fatalf("query %d: %v", i, err)
		}
		assertSame(t, src, got)
	}
}

func TestFormatSourceReturnsParseErrors(t *testing.T) {
	src := ".a | # unfinished\n"
	got, err := FormatSource(src)
	if err == nil || got != src {
		t.Fatalf("got %q, %v; want the source back with an error", got, err)
	}
}
