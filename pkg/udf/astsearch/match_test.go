package astsearch

import (
	"context"
	"fmt"
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/grep"
	"github.com/xen0bit/pwrq/pkg/core/filewalk"
)

// Parsing a file once and running every query against that tree is only sound
// while it produces what running each query separately produced. grep exposes
// no way to reuse a tree, so the conversion from a tree-sitter match to a
// grep.Result is reproduced in match.go - and this is what keeps the copy
// honest. It runs both paths over the same source and compares them whole,
// captures and spans included, so a change to grep's conversion fails here
// rather than quietly changing what a rule reports.
func TestParsedMatchesGrep(t *testing.T) {
	cases := []struct {
		language string
		source   string
		patterns []string
	}{
		{"go", goSource, []string{
			"fmt.Errorf($$$A)", "func $N($$$A) error { $$$B }", "if $C { $$$B }",
			"return nil", "err != nil",
		}},
		{"python", pySource, []string{
			"def $N($$$A): $$$B", "return $E", "raise ValueError($M)", "class $C: $$$B",
		}},
		{"c", "#include <stdio.h>\nint main(void) { char b[8]; gets(b); return 0; }\n",
			[]string{"gets($B);", "return 0;"}},
	}

	for _, tc := range cases {
		entry := grammars.DetectLanguageByName(tc.language)
		if entry == nil {
			t.Skipf("this build has no %s grammar", tc.language)
		}
		source := []byte(tc.source)
		tree, err := parseOnce(entry, source)
		if err != nil {
			t.Fatalf("%s: %v", tc.language, err)
		}
		if tree == nil {
			t.Fatalf("%s: nothing parsed", tc.language)
		}

		compared := 0
		for _, pattern := range tc.patterns {
			c, err := compilePattern(pattern, tc.language)
			if err != nil {
				t.Fatalf("%s %q: %v", tc.language, pattern, err)
			}
			if !c.valid() {
				t.Fatalf("%s %q: %s", tc.language, pattern, c.problem)
			}
			for i, q := range c.queries {
				want, err := q.Match(source)
				if err != nil {
					t.Fatalf("%s %q reading %d: %v", tc.language, pattern, i, err)
				}
				got, err := tree.match(q)
				if err != nil {
					t.Fatalf("%s %q reading %d: %v", tc.language, pattern, i, err)
				}
				compareResults(t, tc.language+" "+pattern, want, got)
				compared += len(want)
			}
		}
		tree.release()
		if compared == 0 {
			t.Fatalf("%s: no pattern matched anything, so nothing was compared", tc.language)
		}
	}
}

func compareResults(t *testing.T, what string, want, got []grep.Result) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("%s: grep found %d matches, the shared tree found %d", what, len(want), len(got))
	}
	for i := range want {
		w, g := want[i], got[i]
		if w.StartByte != g.StartByte || w.EndByte != g.EndByte {
			t.Errorf("%s match %d: grep spans %d-%d, the shared tree spans %d-%d",
				what, i, w.StartByte, w.EndByte, g.StartByte, g.EndByte)
		}
		if len(w.Captures) != len(g.Captures) {
			t.Errorf("%s match %d: grep captured %d holes, the shared tree captured %d",
				what, i, len(w.Captures), len(g.Captures))
		}
		for name, wc := range w.Captures {
			gc, ok := g.Captures[name]
			if !ok {
				t.Errorf("%s match %d: the shared tree lost capture %s", what, i, name)
				continue
			}
			if string(wc.Text) != string(gc.Text) || wc.StartByte != gc.StartByte ||
				wc.EndByte != gc.EndByte || wc.Name != gc.Name {
				t.Errorf("%s match %d capture %s: grep has %q at %d-%d, the shared tree has %q at %d-%d",
					what, i, name, wc.Text, wc.StartByte, wc.EndByte, gc.Text, gc.StartByte, gc.EndByte)
			}
		}
	}
}

// SearchTree searches several files at once, so what it returns must not
// depend on which one finished first. This walks the same tree through
// select_ast, which is sequential and lazy, and compares the two whole.
func TestSearchTreeMatchesTheSequentialWalk(t *testing.T) {
	files := map[string]string{}
	// Enough files that every worker has work, and enough matches per file
	// that ordering within one is exercised too.
	for i := 0; i < 40; i++ {
		files[fmt.Sprintf("pkg%d/file%d.go", i%5, i)] = goSource
		files[fmt.Sprintf("pkg%d/mod%d.py", i%5, i)] = pySource
	}
	files["README.md"] = "# not code in any pattern here\n"
	dir := tree(t, files)

	patterns := []string{"fmt.Errorf($$$A)", "if $C { $$$B }", "return $E", "def $N($$$A): $$$B"}

	parallel, err := SearchTree(context.Background(), dir, patterns, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(parallel) == 0 {
		t.Fatal("nothing matched, so this compares nothing")
	}

	walk, err := filewalk.New(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	it := &matchIter{patterns: patterns, walk: walk}
	var sequential []any
	for {
		path, ok, err := walk.Next()
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		found, err := it.searchFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sequential = append(sequential, found...)
	}

	if len(parallel) != len(sequential) {
		t.Fatalf("searching in parallel found %d matches, one file at a time found %d",
			len(parallel), len(sequential))
	}
	for i := range parallel {
		p := parallel[i].(map[string]any)
		s := sequential[i].(map[string]any)
		for _, key := range []string{"Path", "Pattern", "Offset", "EndOffset", "LineNumber", "Column", "Text"} {
			if fmt.Sprint(p[key]) != fmt.Sprint(s[key]) {
				t.Fatalf("match %d differs on %s: parallel %v, sequential %v",
					i, key, p[key], s[key])
			}
		}
	}
}
