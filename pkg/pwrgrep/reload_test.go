package pwrgrep

import (
	"os"
	"path/filepath"
	"testing"
)

// A rule written while the process is running is a rule the process can run.
// The MCP server is one process for hours, and an agent that writes a rule and
// then asks for it is the whole of the authoring loop.
func TestARuleWrittenNowIsVisibleNow(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvDirs, dir)
	resetCorpus()
	t.Cleanup(resetCorpus)

	before, err := Rules()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Select([]string{"scratch-hello"}); err == nil {
		t.Fatal("selected a rule that has not been written yet")
	}

	source := "# rules: scratch-hello\n\nscan_ast(\"*.go\"; [\"panic($$$A)\"])\n| finding(\"scratch-hello\"; \"a panic\")\n| report\n"
	write(t, filepath.Join(dir, "scratch.pwrq"), source)

	found, err := Select([]string{"scratch-hello"})
	if err != nil {
		t.Fatalf("a rule on disk was not found: %v", err)
	}
	if len(found) != 1 || found[0].Origin != dir {
		t.Fatalf("got %d rules, origin %q", len(found), found[0].Origin)
	}
	if after, _ := Rules(); len(after) != len(before)+1 {
		t.Fatalf("catalogue went from %d to %d", len(before), len(after))
	}

	// And an edit to that file is the version that runs, not the one read
	// first: the compiled copy is keyed by path and has to go with the reload.
	write(t, filepath.Join(dir, "scratch.pwrq"), source+"\n# edited\n")
	edited, err := Select([]string{"scratch-hello"})
	if err != nil {
		t.Fatal(err)
	}
	if edited[0].Query == found[0].Query {
		t.Fatal("the edited rule still reads as the version loaded first")
	}
	if _, ok := compiled.Load(edited[0].Path); ok {
		t.Fatal("the compiled copy of a reloaded rule was kept")
	}

	if err := os.Remove(filepath.Join(dir, "scratch.pwrq")); err != nil {
		t.Fatal(err)
	}
	if _, err := Select([]string{"scratch-hello"}); err == nil {
		t.Fatal("a deleted rule is still in the catalogue")
	}
}

// The check that decides whether to reload runs on every lookup, so it has to
// be cheap enough to sit in front of one.
func BenchmarkStamp(b *testing.B) {
	dir := b.TempDir()
	for i := range 200 {
		write(b, filepath.Join(dir, "r"+string(rune('a'+i%26))+string(rune('a'+i/26))+".pwrq"), "# rules: x\n\n.\n")
	}
	b.Setenv(EnvDirs, dir)
	for b.Loop() {
		stamp()
	}
}

func write(t testing.TB, name, body string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// resetCorpus forgets what was read, so a test that changes the search path
// starts from the path it set rather than from another test's.
func resetCorpus() {
	corpusMu.Lock()
	defer corpusMu.Unlock()
	corpusRead, corpusStamp, corpusRules, corpusErr = false, "", nil, nil
	forgetCompiled()
}
