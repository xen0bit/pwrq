package pwrgrep

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The rule cmdlets reach this package through UseVocabulary, which the
// registry calls and a test in this package cannot, so what is written here is
// plain jq. Whether a rule using scan_ast survives the round trip is a
// question for the package that has the vocabulary: see
// pkg/udf/pwrgrep.TestARuleWrittenOverMCPRunsInTheSameProcess.
const wellFormed = `# rules: scratch-panic
# languages: go

[{RuleId: "scratch-panic", Path: "."}]
`

func TestWriteSavesARuleTheCatalogueThenFinds(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvDirs, dir)
	resetCorpus()
	t.Cleanup(resetCorpus)

	rule, file, err := Write("mine/panic", wellFormed)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "mine", "panic.pwrq"); file != want {
		t.Fatalf("wrote %q, wanted %q", file, want)
	}
	if rule.Path != "mine/panic" || rule.Id() != "scratch-panic" {
		t.Fatalf("wrote %q reporting %q", rule.Path, rule.Id())
	}
	if got, err := Select([]string{"scratch-panic"}); err != nil || len(got) != 1 {
		t.Fatalf("selecting the rule just written: %v, %d rules", err, len(got))
	}

	// The name may carry the extension, because a caller copying a File back
	// in should not have to notice that Path does not.
	if _, again, err := Write("mine/panic.pwrq", wellFormed); err != nil || again != file {
		t.Fatalf("rewriting with the extension: %v, %q", err, again)
	}
}

// Nothing that would make the catalogue unreadable reaches the directory: one
// bad file there fails every later lookup, for every rule, not just this one.
func TestWriteRefusesWhatIsNotARule(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvDirs, dir)
	resetCorpus()
	t.Cleanup(resetCorpus)

	for _, c := range []struct{ name, source, says string }{
		{"mine/headerless", "[{RuleId: \"x\"}]\n", "# rules:"},
		{"mine/unparseable", "# rules: nope\n\n[{ | ]\n", "unexpected token"},
		{"../escape", wellFormed, "does not name a place"},
		{"/etc/evil", wellFormed, "absolute path"},
		{"mine/../../out", wellFormed, "does not name a place"},
		{"", wellFormed, "needs a name"},
		{wellFormed, "mine/swapped", "not its name"},
	} {
		if _, _, err := Write(c.name, c.source); err == nil {
			t.Errorf("%q was written", c.name)
		} else if !strings.Contains(err.Error(), c.says) {
			t.Errorf("%q refused with %q, wanted it to mention %q", c.name, err, c.says)
		}
	}

	var wrote []string
	_ = filepath.WalkDir(dir, func(name string, d os.DirEntry, _ error) error {
		if d != nil && !d.IsDir() {
			wrote = append(wrote, name)
		}
		return nil
	})
	if len(wrote) != 0 {
		t.Fatalf("a refused write still left %v behind", wrote)
	}
	if _, err := Rules(); err != nil {
		t.Fatalf("the catalogue is unreadable after the refusals: %v", err)
	}
}

// A rule goes where somebody can edit it, never into the directory the package
// manager owns and the next upgrade replaces.
func TestWritableDirIsNeverTheSystemCopy(t *testing.T) {
	t.Setenv(EnvDirs, "")
	if got := WritableDir(); got == SystemDir {
		t.Fatalf("rules would be written to %s", got)
	}
	t.Setenv(EnvDirs, strings.Join([]string{"/first", "/second"}, string(os.PathListSeparator)))
	if got := WritableDir(); got != "/first" {
		t.Fatalf("wrote to %q rather than the first entry of %s", got, EnvDirs)
	}
}
