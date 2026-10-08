package llm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// fakeHbb writes a script that speaks `hbb serve --stdio`: a ready line, then one
// canned reply per request, echoing the id. A path ending .zzz is a skip, and
// one ending .boom an error, which are the three kinds of reply.
func fakeHbb(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "hbb")
	body := `#!/bin/bash
echo "$@" > "` + dir + `/args"
echo '{"ready":true,"version":"fake","model":"fake/enc@1","device":"cpu","labels":["cwe_79"],"window":{"lines":240},"questions":{"cwe_79":{"cwe":"CWE-79","title":"XSS","q":"?"}}}'
while IFS= read -r line; do
  id=$(sed -E 's/.*"id":([0-9]+).*/\1/' <<<"$line")
  case "$line" in
    *.zzz*) echo '{"id":'$id',"path":"a.zzz","skip":"no language for this extension"}' ;;
    *.boom*) echo '{"id":'$id',"error":"cannot read it"}' ;;
    *) echo '{"id":'$id',"path":"a.c","lang":"c","lines":300,"tokens":1234,"windows":[{"from":1,"to":240,"p":{"cwe_79":0.25}},{"from":241,"to":300,"p":{"cwe_79":0.75}}]}' ;;
  esac
done
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return script
}

func runHbbQuery(t *testing.T, q string, vars ...any) []any {
	t.Helper()
	query, err := gojq.Parse(q)
	if err != nil {
		t.Fatal(err)
	}
	code, err := gojq.Compile(query, RegisterInvokeHbb(), RegisterGetHbb(), RegisterGetUsage())
	if err != nil {
		t.Fatal(err)
	}
	var out []any
	it := code.Run(nil)
	for {
		v, ok := it.Next()
		if !ok {
			break
		}
		if err, isErr := v.(error); isErr {
			out = append(out, "ERR: "+err.Error())
			continue
		}
		out = append(out, common.BindValue(v))
	}
	return out
}

func TestInvokeHbbScoresAFile(t *testing.T) {
	resetUsage()
	bin := fakeHbb(t)
	out := runHbbQuery(t, `invoke_hbb({Path: "a.c", Text: "int x;"}; {Bin: "`+bin+`", Device: "cpu"}) | [.Lang, .Lines, .Tokens, (.Windows | length), .Windows[1].P.cwe_79]`)
	got, _ := out[0].([]any)
	if len(got) != 5 || got[0] != "c" || got[3] != 2 && got[3] != float64(2) {
		t.Fatalf("got %v", out)
	}
	args, _ := os.ReadFile(filepath.Join(filepath.Dir(bin), "args"))
	if !strings.Contains(string(args), "serve --stdio") || !strings.Contains(string(args), "--device cpu") {
		t.Fatalf("hbb was started with %q", args)
	}
	u := usageObject()
	if u["Calls"] != 1 || u["InputTokens"] != 1234 {
		t.Fatalf("usage %v", u)
	}
}

func TestInvokeHbbKeepsOneProcessAndReportsSkipsAndErrors(t *testing.T) {
	bin := fakeHbb(t)
	o := `{Bin: "` + bin + `"}`
	out := runHbbQuery(t, `(invoke_hbb({Path: "a.c", Text: "x"}; `+o+`) | .Path),
	  (invoke_hbb({Path: "a.zzz", Text: "x"}; `+o+`) | .Skip),
	  (try invoke_hbb({Path: "b.boom", Text: "x"}; `+o+`) catch .),
	  (invoke_hbb({Path: "a.c", Text: "x"}; `+o+`) | .Lines)`)
	if len(out) != 4 || out[0] != "a.c" || out[1] != "no language for this extension" ||
		!strings.Contains(out[2].(string), "cannot read it") {
		t.Fatalf("got %v", out)
	}
	n := 0
	for _, k := range hbbPoolKeys() {
		if strings.HasPrefix(k, bin+"\x00") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d processes for one set of options", n)
	}
}

func TestGetHbbReportsTheModel(t *testing.T) {
	bin := fakeHbb(t)
	out := runHbbQuery(t, `get_hbb({Bin: "`+bin+`"}) | [.Model, (.Questions | keys[0])]`)
	got, _ := out[0].([]any)
	if len(got) != 2 || got[0] != "fake/enc@1" || got[1] != "cwe_79" {
		t.Fatalf("got %v", out)
	}
}

func TestHbbRejectsWhatItDoesNotUnderstand(t *testing.T) {
	out := runHbbQuery(t, `invoke_hbb({Path: "a.c", Text: "x"}; {Temperature: 0})`)
	if s, _ := out[0].(string); !strings.Contains(s, `unknown option "Temperature"`) {
		t.Fatalf("got %v", out)
	}
	out = runHbbQuery(t, `invoke_hbb({Text: "x"}; {Bin: "/nonexistent/hbb"})`)
	if s, _ := out[0].(string); !strings.Contains(s, "no Path") {
		t.Fatalf("got %v", out)
	}
	out = runHbbQuery(t, `invoke_hbb({Path: "a.c", Text: "x"}; {Bin: "/nonexistent/hbb"})`)
	if s, _ := out[0].(string); !strings.Contains(s, EnvHbbBin) {
		t.Fatalf("got %v", out)
	}
}
