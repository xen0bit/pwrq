package ideengine

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf"
)

func TestRecipesParse(t *testing.T) {
	rs, err := Recipes()
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) < 80 {
		t.Errorf("only %d recipes", len(rs))
	}
	for _, r := range rs {
		for _, s := range r.Slots {
			if _, ok := normalizeSlot(s.Kind, s.Default); !ok {
				t.Errorf("recipe %s: slot %s's default %q is not a %s", r.ID, s.Name, s.Default, s.Kind)
			}
		}
	}
}

func compileRecipe(t *testing.T, reg *udf.Registry, query string) *gojq.Code {
	t.Helper()
	q, err := gojq.Parse(query)
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, query)
	}
	opts := append(reg.Options(), gojq.WithEnvironLoader(os.Environ))
	code, err := gojq.Compile(q, opts...)
	if err != nil {
		t.Fatalf("compile: %v\n%s", err, query)
	}
	return code
}

// TestRecipesCompile: every recipe, filled with its defaults, is a query the
// CLI compiles - a suggestion that does not compile is worse than none.
func TestRecipesCompile(t *testing.T) {
	rs, err := Recipes()
	if err != nil {
		t.Fatal(err)
	}
	reg := udf.DefaultRegistry()
	for _, r := range rs {
		t.Run(r.ID, func(t *testing.T) { compileRecipe(t, reg, Fill(r.Query, r.Slots, nil)) })
	}
}

// TestExampleRecipesCompile: every example made a recipe compiles both
// ways - with its own values, and with the optional input dropped for the
// editor's.
func TestExampleRecipesCompile(t *testing.T) {
	reg := udf.DefaultRegistry()
	rs := ExampleRecipes()
	if len(rs) < 600 {
		t.Fatalf("only %d example recipes", len(rs))
	}
	for _, r := range rs {
		t.Run(r.ID, func(t *testing.T) {
			all := map[string]string{}
			for _, s := range r.Slots {
				all[s.Name] = s.Default
			}
			compileRecipe(t, reg, Fill(r.Query, r.Slots, all))
			compileRecipe(t, reg, Fill(r.Query, r.Slots, nil))
		})
	}
}

func TestExampleRecipeSlots(t *testing.T) {
	_ = udf.DefaultRegistry()
	byID := map[string]Recipe{}
	for _, r := range ExampleRecipes() {
		byID[r.ID] = r
	}
	cases := map[string]struct{ withDefaults, without string }{
		"sha256#1":      {`"hello" | sha256`, `sha256`},
		"sha256#2":      {`"file.txt" | sha256(true)`, `"file.txt" | sha256(true)`},
		"head#2":        {`head("app.log"; 5)`, `head("app.log"; 5)`},
		"mean#1":        {`mean`, `mean`},
		"human_bytes#1": {`1048576 | human_bytes`, `human_bytes`},
	}
	for id, want := range cases {
		r, ok := byID[id]
		if !ok {
			t.Errorf("no recipe %s", id)
			continue
		}
		all := map[string]string{}
		for _, s := range r.Slots {
			all[s.Name] = s.Default
		}
		if got := Fill(r.Query, r.Slots, all); got != want.withDefaults {
			t.Errorf("%s with its values: %q, want %q", id, got, want.withDefaults)
		}
		if got := Fill(r.Query, r.Slots, nil); got != want.without {
			t.Errorf("%s without: %q, want %q", id, got, want.without)
		}
	}
}

// recipeInputs are what the recipes that read their input are run on.
var recipeInputs = map[string]string{
	"json-to-csv":         `[{"name":"ada","age":36},{"name":"bob","age":25}]`,
	"yaml-to-json":        `"name: ada\nrole: engineer\n"`,
	"json-to-yaml":        `{"name":"ada"}`,
	"xml-to-json":         `"<a><b>1</b></a>"`,
	"count-items":         `[1,2,3]`,
	"object-keys":         `[{"a":1},{"b":2}]`,
	"filter-by-field":     `[{"status":"active"},{"status":"gone"}]`,
	"filter-greater-than": `[{"age":20},{"age":40}]`,
	"sort-by-field":       `[{"name":"b"},{"name":"a"}]`,
	"top-by-field":        `[{"score":1},{"score":9}]`,
	"group-count":         `[{"status":"a"},{"status":"a"}]`,
	"group-by-field":      `[{"status":"a"},{"status":"b"}]`,
	"sum-field":           `[{"amount":1},{"amount":2}]`,
	"average-field":       `[{"amount":1},{"amount":2}]`,
	"stats-field":         `[{"amount":1},{"amount":2}]`,
	"unique-values":       `[{"status":"a"},{"status":"a"}]`,
	"pluck-field":         `[{"name":"a"}]`,
	"remove-duplicates":   `[1,1,2]`,
	"flatten-json":        `{"a":{"b":1}}`,
	"as-table":            `[{"a":1}]`,
	"extract-emails":      `"mail ada@example.com"`,
	"extract-urls":        `"see https://a.com/x"`,
	"extract-ips":         `"from 10.0.0.1"`,
	"base64-decode":       `"aGk="`,
	"base64-encode":       `"hi"`,
	"jwt-decode":          `"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2ln"`,
	"url-decode":          `"a%20b"`,
	"text-lines":          `"a\n\nb"`,
	"sha256-text":         `"hi"`,
	"timestamps-to-dates": `[0]`,
}

// recipesNotRun reach past a scratch directory: the network, the process
// table, the service manager, a model, or the shell.
var recipesNotRun = map[string]string{
	"images-containing": "needs the d1 model", "images-score": "needs the d1 model",
	"video-shows": "needs the d1 model", "video-when": "needs the d1 model", "videos-containing": "needs the d1 model", "video-frames-ffmpeg": "needs ffmpeg and the d1 model",
	"images-count-objects": "needs the d1 model", "images-classify": "needs the d1 model",
	"sentiment": "needs the d1 model", "text-yes-no": "needs the d1 model", "classify-lines": "needs the d1 model",
	"summarize-file": "needs a language model",
	"resolve-host":   "network", "reverse-dns": "network", "ping-host": "network", "port-open": "network",
	"http-status": "network", "http-headers": "network", "fetch-json": "network", "page-links": "network",
	"download-file": "network", "running-services": "the service manager",
	"run-command": "the shell", "which-command": "PATH", "code-rules": "scans the tree it is given",
	"top-memory-processes": "the process table", "top-cpu-processes": "the process table",
	"find-process": "the process table", "count-processes": "the process table",
	"move-images-containing": "needs the d1 model", "triage-errors": "needs the d1 model", "urgent-todos": "needs the d1 model",
	"ask-llm":    "needs a language model",
	"fetch-page": "network", "page-title": "network", "fetch-json-field": "network", "post-json": "network",
	"request-with-header": "network", "save-json-response": "network", "check-links": "network",
	"download-url-list": "network", "resolve-hosts-file": "network", "ping-hosts-file": "network",
	"stopped-services": "the service manager", "start-service": "the service manager", "stop-service": "the service manager",
	"kill-process": "the process table", "kill-by-name": "the process table",
	"disk-free": "the shell", "cpu-count": "the shell",
	"memory-info": "Linux's /proc", "load-average": "Linux's /proc", "uptime": "Linux's /proc",
	"findings-per-rule": "scans the tree it is given",
}

// TestRecipesRun runs every recipe that needs only files, in a scratch
// directory holding the files their defaults name.
func TestRecipesRun(t *testing.T) {
	rs, err := Recipes()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	files := map[string]string{
		"notes.txt":           "the quick brown fox\njumps over the lazy dog\nthe end\n",
		"app.log":             "INFO start\nERROR failed to open\nWARN slow\nERROR again\n",
		"data.csv":            "name,age\nada,36\nbob,25\n",
		"data.json":           `{"a":1}`,
		"config.yaml":         "name: ada\n",
		"src/main.go":         "package main\n// TODO: more\nfunc main() {}\n",
		"src/report-2026.txt": "",
		// The recipes below read or change files of their own: a recipe
		// that moves or deletes runs once, on a file nothing else reads.
		"events.jsonl": "{\"ts\":\"2026-08-11T09:15:00Z\",\"level\":\"error\",\"svc\":\"api\",\"dur_ms\":120}\n" +
			"{\"ts\":\"2026-08-11T09:40:00Z\",\"level\":\"info\",\"svc\":\"web\",\"dur_ms\":30}\n",
		"access.log": `10.0.0.1 - - [11/Aug/2026:09:15:01 +0000] "GET /index.html HTTP/1.1" 200 512` + "\n" +
			`10.0.0.2 - - [11/Aug/2026:09:15:40 +0000] "POST /login HTTP/1.1" 500 64` + "\n",
		"app.logfmt":     "ts=2026-08-11T09:15:00Z level=error msg=\"db down\"\n",
		"data.tsv":       "name\tage\nada\t36\n",
		"config.ini":     "[server]\nport = 8080\n",
		"app.properties": "db.url=postgres://db\npassword=hunter2\n",
		"users.json":     `[{"name":"ada","email":"ada@example.com"},{"name":"bob","email":"not-an-email"}]`,
		"bad.json":       "{not json",
		"draft.txt":      "draft",
		"old.txt":        "old",
		"scratch/x.txt":  "x",
		"docs/a.txt":     "a",
		"photos/p.txt":   "p",
	}
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// ssdeep needs 4096 bytes; the stale, big file is what cleanup finds.
	for name, size := range map[string]int{"sample1.bin": 8192, "sample2.bin": 8192, "stale.bin": 2 << 20} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(strings.Repeat("pwrq sample text ", size/17+1))[:size], 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().AddDate(0, 0, -400)
	if err := os.Chtimes(filepath.Join(dir, "stale.bin"), old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "locked.txt"), []byte("ro"), 0o444); err != nil {
		t.Fatal(err)
	}
	zf, err := os.Create(filepath.Join(dir, "archive.zip"))
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(zf)
	w, _ := zw.Create("inside.txt")
	_, _ = w.Write([]byte("hi"))
	_ = zw.Close()
	_ = zf.Close()
	if err := makeSQLite(filepath.Join(dir, "app.db")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	reg := udf.DefaultRegistry()
	for _, r := range rs {
		if why, skip := recipesNotRun[r.ID]; skip {
			_ = why
			continue
		}
		t.Run(r.ID, func(t *testing.T) {
			query := Fill(r.Query, r.Slots, nil)
			code := compileRecipe(t, reg, query)
			var input any
			if in, ok := recipeInputs[r.ID]; ok {
				v, err := gojq.Parse(in)
				if err != nil {
					t.Fatal(err)
				}
				c, err := gojq.Compile(v)
				if err != nil {
					t.Fatal(err)
				}
				input, _ = c.Run(nil).Next()
			}
			iter := code.RunWithContext(context.Background(), input)
			n := 0
			for {
				v, ok := iter.Next()
				if !ok {
					break
				}
				if err, isErr := v.(error); isErr {
					t.Fatalf("%v\n%s", err, query)
				}
				n++
			}
			if n == 0 && !mayBeEmpty[r.ID] {
				t.Errorf("no output\n%s", query)
			}
		})
	}
}

// mayBeEmpty find nothing in the scratch directory, rightly.
var mayBeEmpty = map[string]bool{"empty-files": true, "hidden-files": true, "duplicate-files": true,
	"file-entropy": true, "binary-files": true, "files-larger-than": true, "list-images": true}

func makeSQLite(path string) error {
	reg := udf.DefaultRegistry()
	q, err := gojq.Parse(`invoke_sqlite_command($p; "create table users (id integer primary key, name text)"), invoke_sqlite_command($p; "insert into users (name) values ('ada')")`)
	if err != nil {
		return err
	}
	code, err := gojq.Compile(q, append(reg.Options(), gojq.WithVariables([]string{"$p"}))...)
	if err != nil {
		return err
	}
	iter := code.Run(nil, path)
	for {
		v, ok := iter.Next()
		if !ok {
			return nil
		}
		if err, isErr := v.(error); isErr {
			return err
		}
	}
}

func TestFillEscapesAndReads(t *testing.T) {
	slots := []Slot{{Name: "dir", Kind: "path", Default: "."}, {Name: "size", Kind: "size", Default: "10MB"},
		{Name: "subject", Kind: "text", Default: "a cat"}, {Name: "f", Kind: "field", Default: "name"}}
	q := `get_childitem("{{dir}}") | select(.Length > {{size}}) | "{{subject}}" | .{{f}}`
	cases := []struct {
		values map[string]string
		want   string
	}{
		{nil, `get_childitem(".") | select(.Length > 10485760) | "a cat" | .name`},
		{map[string]string{"dir": "the src folder", "size": "2 GB", "subject": `jumbo "tiger" prawns`, "f": "last name"},
			`get_childitem("src") | select(.Length > 2147483648) | "jumbo \"tiger\" prawns" | .last_name`},
		{map[string]string{"dir": "current directory", "size": "lots", "f": "!!"},
			`get_childitem(".") | select(.Length > lots) | "a cat" | .name`},
	}
	cases[2].want = `get_childitem(".") | select(.Length > 10485760) | "a cat" | .name`
	for _, c := range cases {
		if got := Fill(q, slots, c.values); got != c.want {
			t.Errorf("Fill(%v)\n got %s\nwant %s", c.values, got, c.want)
		}
	}
}

func TestNormalizeSlot(t *testing.T) {
	cases := []struct{ kind, in, want string }{
		{"ext", "Python files", "py"}, {"ext", ".go", "go"}, {"ext", "*.md", "md"},
		{"host", "https://github.com/x", "github.com"}, {"url", "example.com", "https://example.com"},
		{"number", "top 10", "10"}, {"number", "five", "5"}, {"days", "3 days", "3"},
		{"size", "500KB", "512000"}, {"path", "~/Downloads", "~/Downloads"}, {"path", "here", "."},
		{"file", "README.md", "README.md"},
		{"file", "a.txt and b.txt", "a.txt"}, {"file", "old.json, new.json", "old.json"},
	}
	for _, c := range cases {
		if got, ok := normalizeSlot(c.kind, c.in); !ok || got != c.want {
			t.Errorf("normalizeSlot(%s, %q) = %q, %v; want %q", c.kind, c.in, got, ok, c.want)
		}
	}
	for _, c := range []struct{ kind, in string }{{"file", "the file"}, {"ip", "home"}, {"host", "my server"}, {"number", "some"}} {
		if got, ok := normalizeSlot(c.kind, c.in); ok {
			t.Errorf("normalizeSlot(%s, %q) = %q, want no value", c.kind, c.in, got)
		}
	}
}

func TestStages(t *testing.T) {
	got := Stages(`[get_childitem(".") | select(.a | not)] | sort_by(.x) | "a|b" | .y |= 1`)
	want := []string{`[get_childitem(".") | select(.a | not)]`, `sort_by(.x)`, `"a|b"`, `.y |= 1`}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("Stages = %q", got)
	}
	for in, want := range map[string]string{`[get_childitem(".")`: "[get_childitem", `.Name`: ".Name", `select(.x)`: "select", `"x"`: `"`} {
		if got := stageHead(in); got != want {
			t.Errorf("stageHead(%q) = %q, want %q", in, got, want)
		}
	}
}
