package tui

import (
	"reflect"
	"testing"
)

// These are the cases pkg/web/src/js/highlight.test.js pins for the page's
// tokeniser. Keeping them identical is what keeps the two editors colouring a
// query alike.

var testVocab = Vocabulary{
	Cmdlets:  map[string]bool{"sha256": true, "get_childitem": true},
	Builtins: map[string]bool{"select": true, "map": true},
}

func kinds(src string) [][2]string {
	runes := []rune(src)
	var out [][2]string
	for _, token := range TokenizeQuery(runes, testVocab) {
		if token.Kind != tokSpace {
			out = append(out, [2]string{token.Kind, string(runes[token.Start:token.End])})
		}
	}
	return out
}

func TestQueryTellsCmdletsFromBuiltins(t *testing.T) {
	want := [][2]string{
		{"field", ".a"}, {"punct", "|"}, {"cmdlet", "sha256"}, {"punct", "|"},
		{"builtin", "select"}, {"punct", "("}, {"field", ".b"}, {"punct", ")"},
	}
	if got := kinds(".a | sha256 | select(.b)"); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestQueryKeepsStringsWhole(t *testing.T) {
	src := `"a \"quoted\" thing"`
	if got := kinds(src); !reflect.DeepEqual(got, [][2]string{{"string", src}}) {
		t.Errorf("got %v", got)
	}
}

func TestQueryTreatsInterpolationAsCode(t *testing.T) {
	found := false
	for _, kv := range kinds(`"total: \(.n + 1)"`) {
		found = found || kv[0] == tokInterp
	}
	if !found {
		t.Error("interpolation should be its own token")
	}
}

func TestQueryRecognisesVariablesFormatsCommentsNumbers(t *testing.T) {
	want := [][2]string{{"variable", "$x"}, {"format", "@base64"}, {"number", "1.5e3"}, {"comment", "# note"}}
	if got := kinds(`$x @base64 1.5e3 # note`); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

// TestTokensCoverEverything matters more here than in the page: the editor
// draws each line from its tokens, so a gap would drop text from the screen.
func TestTokensCoverEverything(t *testing.T) {
	for _, src := range []string{".a|sha256", `{"k": [1, 2]} # x`, `"s" as $v | $v`, "..", ".a?.b[]",
		`"unterminated \(`, `"trailing \`, `"🙂 \("x") é"`, `.a | "\("\\")"`} {
		assertCovers(t, src, TokenizeQuery([]rune(src), testVocab))
	}
	for _, src := range []string{`{"a":[1,true,null],"b":"x"} {"c":-2.5e10}`, `{"a": "unterminated`, `"\`} {
		assertCovers(t, src, TokenizeJSON([]rune(src)))
	}
}

func assertCovers(t *testing.T, src string, tokens []Token) {
	t.Helper()
	pos := 0
	for _, token := range tokens {
		if token.Start != pos || token.End <= token.Start {
			t.Errorf("%q: token %+v does not start where the last ended (%d)", src, token, pos)
			return
		}
		pos = token.End
	}
	if pos != len([]rune(src)) {
		t.Errorf("%q: tokens end at %d of %d", src, pos, len([]rune(src)))
	}
}

func TestJSONDistinguishesKeysFromValues(t *testing.T) {
	var got []string
	for _, token := range TokenizeJSON([]rune(`{"a": "b"}`)) {
		if token.Kind != tokSpace {
			got = append(got, token.Kind)
		}
	}
	if want := []string{"punct", "key", "punct", "string", "punct"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestWordAt(t *testing.T) {
	src := []rune(".a | sha2")
	if got := WordAt(src, 9); got != (Word{Text: "sha2", Start: 5, End: 9}) {
		t.Errorf("got %+v", got)
	}
	if got := WordAt([]rune("$va"), 3); got.Text != "$va" || got.Prefix != '$' {
		t.Errorf("a leading dollar belongs to the word, got %+v", got)
	}
}

func TestInLiteral(t *testing.T) {
	src := []rune(`"sha2" # sha2`)
	tokens := TokenizeQuery(src, testVocab)
	if !InLiteral(tokens, 5) {
		t.Error("inside the string")
	}
	if !InLiteral(tokens, 13) {
		t.Error("inside the comment")
	}
	if InLiteral(tokens, 7) {
		t.Error("between them")
	}
}
