package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xen0bit/pwrq/pkg/ideengine"
)

func TestHistoryRecordsRunsNewestFirstWithoutDuplicates(t *testing.T) {
	s := OpenStore(t.TempDir())
	s.Record(Entry{Query: ".a"})
	s.Record(Entry{Query: ".b"})
	s.Record(Entry{Query: ".a"})
	s.Record(Entry{Query: "  "})

	var got []string
	for _, entry := range s.History() {
		got = append(got, entry.Query)
	}
	if strings.Join(got, ",") != ".a,.b" {
		t.Errorf("history = %v", got)
	}
}

func TestHistoryDropsLargeInputs(t *testing.T) {
	s := OpenStore(t.TempDir())
	s.Record(Entry{Query: ".", Input: strings.Repeat("x", storedInputLimit+1)})
	if h := s.History(); len(h) != 1 || h[0].Input != "" {
		t.Errorf("a large input should not be stored, got %d bytes", len(h[0].Input))
	}
}

func TestHistoryIsBounded(t *testing.T) {
	s := OpenStore(t.TempDir())
	for i := 0; i < historyLimit+10; i++ {
		s.Record(Entry{Query: ".a" + strings.Repeat(" ", i)})
	}
	if n := len(s.History()); n != historyLimit {
		t.Errorf("history holds %d, want %d", n, historyLimit)
	}
}

// TestSnippetsAreInThePageFormat is what lets the two editors share them.
func TestSnippetsAreInThePageFormat(t *testing.T) {
	dir := t.TempDir()
	s := OpenStore(dir)
	if _, err := s.SaveSnippet(Entry{Name: "big files", Query: `select(.Length > $n)`, Args: []ideengine.Arg{{Name: "n", Value: "1000"}}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "snippets.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"tool": "pwrq"`, `"snippets"`, `"name": "big files"`, `"args"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("snippets.json lacks %s:\n%s", want, data)
		}
	}
}

func TestSnippetsImportFromThePage(t *testing.T) {
	s := OpenStore(t.TempDir())
	file := filepath.Join(t.TempDir(), "export.json")
	export := `{"tool":"pwrq","snippets":[
		{"name":"first","query":".a","input":"{}","args":[{"name":"x","value":"1"}],"at":1},
		{"name":"second","query":".b","input":"","args":[]},
		{"name":7,"query":".broken"},
		"not an object"]}`
	if err := os.WriteFile(file, []byte(export), 0o600); err != nil {
		t.Fatal(err)
	}
	added, err := s.ImportSnippets(file)
	if err != nil || added != 2 {
		t.Fatalf("added %d, err %v", added, err)
	}
	snippets := s.Snippets()
	if snippets[0].Name != "first" || snippets[0].Args[0].Value != "1" {
		t.Errorf("the file's order should survive, got %+v", snippets)
	}

	out := filepath.Join(t.TempDir(), "out.json")
	if n, err := s.ExportSnippets(out); err != nil || n != 2 {
		t.Fatalf("exported %d, err %v", n, err)
	}
	again := OpenStore(t.TempDir())
	if added, err := again.ImportSnippets(out); err != nil || added != 2 {
		t.Errorf("an export should import: added %d, err %v", added, err)
	}
}

func TestSnippetsSaveAndDelete(t *testing.T) {
	s := OpenStore(t.TempDir())
	if _, err := s.SaveSnippet(Entry{Name: " ", Query: "."}); err == nil {
		t.Error("a snippet without a name should be refused")
	}
	_, _ = s.SaveSnippet(Entry{Name: "a", Query: ".a"})
	_, _ = s.SaveSnippet(Entry{Name: "a", Query: ".a2"})
	if snippets := s.Snippets(); len(snippets) != 1 || snippets[0].Query != ".a2" {
		t.Errorf("saving under a name replaces it, got %+v", snippets)
	}
	if left := s.DeleteSnippet("a"); len(left) != 0 {
		t.Errorf("left = %+v", left)
	}
}

func TestSessionRoundTrips(t *testing.T) {
	s := OpenStore(t.TempDir())
	if _, ok := s.LoadSession(); ok {
		t.Error("a new store has no session")
	}
	s.SaveSession(Session{Query: ".a", Input: "{}"})
	if got, ok := s.LoadSession(); !ok || got.Query != ".a" || got.Input != "{}" {
		t.Errorf("got %+v", got)
	}
}

func TestAStoreWithoutADirectoryForgets(t *testing.T) {
	s := OpenStore("")
	s.Record(Entry{Query: ".a"})
	if len(s.History()) != 0 {
		t.Error("nothing should be remembered")
	}
}

func TestACorruptStoreIsEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "history.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if len(OpenStore(dir).History()) != 0 {
		t.Error("a corrupt file should read as empty")
	}
}
