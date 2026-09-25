package tui

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/xen0bit/pwrq/pkg/ideengine"
)

var sampleShare = Shared{
	Query:   `.items[] | select(.n > $min) | "\(.Name) <&>"`,
	Input:   `{"items":[{"n":2,"Name":"🙂"}]}`,
	Args:    []ideengine.Arg{{Name: "min", Value: "1"}},
	Options: map[string]any{"output": "raw", "slurp": true},
}

// pageFragment is sampleShare as the browser page encodes it, made by
// pkg/web/src/js/share.js's encodeState. A link made in the page has to open
// here.
const pageFragment = "z=HctBCoJAFIDhqww_EQqDYMtHeIQuMG8WErMY0NF0rIW66RatumJHiNp_38odqS03hCrm0M_Om83MoQvXXFTJNObQx1SazSiqRXVp-1Ca87FRsESEVflHRdyqJEVOVvlBRZTP-_VUdr9jaRHn6GPCUuO9ZUBWhiWPS0aY2geWuVumEcnTEvb9Cw"

func TestShareOpensALinkFromThePage(t *testing.T) {
	got, err := DecodeShare("http://localhost:8080/tools/pwrq/#" + pageFragment)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, sampleShare) {
		t.Errorf("got %+v\nwant %+v", got, sampleShare)
	}
}

func TestShareRoundTrips(t *testing.T) {
	link, err := ShareLink(DefaultShareBase, sampleShare)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(link, DefaultShareBase+"#z=") {
		t.Errorf("link = %q", link)
	}
	got, err := DecodeShare(link)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, sampleShare) {
		t.Errorf("got %+v\nwant %+v", got, sampleShare)
	}
}

// TestShareLinkOpensInThePage runs the page's own decoder over a link made
// here. It needs bun, which the page's tests already do.
func TestShareLinkOpensInThePage(t *testing.T) {
	bun, err := exec.LookPath("bun")
	if err != nil {
		t.Skip("bun is not installed")
	}
	shareJS, err := filepath.Abs("../web/src/js/share.js")
	if err != nil {
		t.Fatal(err)
	}
	link, err := ShareLink(DefaultShareBase, sampleShare)
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(t.TempDir(), "decode.mjs")
	src := `import { decodeHash } from ` + strconvQuote(shareJS) + `;
const link = process.argv[2];
console.log(JSON.stringify(await decodeHash(link.slice(link.indexOf("#")))));`
	if err := os.WriteFile(script, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(bun, script, link).Output()
	if err != nil {
		t.Fatalf("bun: %v", err)
	}
	var back struct {
		Query   string          `json:"query"`
		Input   string          `json:"input"`
		Args    []ideengine.Arg `json:"args"`
		Options map[string]any  `json:"options"`
	}
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatalf("page decoded %q: %v", out, err)
	}
	if back.Query != sampleShare.Query || back.Input != sampleShare.Input ||
		!reflect.DeepEqual(back.Args, sampleShare.Args) || !reflect.DeepEqual(back.Options, sampleShare.Options) {
		t.Errorf("the page read %+v", back)
	}
}

func strconvQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestShareReadsTheHandWrittenForm(t *testing.T) {
	got, err := DecodeShare("#q=.a%20%7C%20.b&i=%7B%22a%22%3A1%7D")
	if err != nil {
		t.Fatal(err)
	}
	if got.Query != ".a | .b" || got.Input != `{"a":1}` {
		t.Errorf("got %+v", got)
	}
}

// TestShareDistrustsTheLink: a link comes from whoever sent it.
func TestShareDistrustsTheLink(t *testing.T) {
	link, _ := ShareLink(DefaultShareBase, Shared{Query: "."})
	// Rebuild the state by hand with hostile shapes.
	hostile := `{"v":1,"q":["not","a","string"],"i":7,"a":[["",""],"bad",["ok","1"]],"o":{"nested":{"x":1},"output":"raw"}}`
	got := validateShared(mustJSON(t, hostile))
	if got.Query != "" || got.Input != "" {
		t.Errorf("non-strings should become empty, got %+v", got)
	}
	if !reflect.DeepEqual(got.Args, []ideengine.Arg{{Name: "ok", Value: "1"}}) {
		t.Errorf("args = %+v", got.Args)
	}
	if _, ok := got.Options["nested"]; ok || got.Options["output"] != "raw" {
		t.Errorf("options = %+v", got.Options)
	}
	if _, err := DecodeShare(link[:len(link)-8] + "!!!!"); err == nil {
		t.Error("a damaged link should be reported, not opened")
	}
}

func mustJSON(t *testing.T, text string) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestLooksLikeShare(t *testing.T) {
	for text, want := range map[string]bool{
		"http://localhost:8080/tools/pwrq/#z=abc": true,
		"#q=.a":                  true,
		".a | .b":                false,
		`"#z=" | length`:         false,
		".a # comment with #z=x": false,
	} {
		if got := LooksLikeShare(text); got != want {
			t.Errorf("LooksLikeShare(%q) = %v, want %v", text, got, want)
		}
	}
}
