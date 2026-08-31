package pwrgrep_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xen0bit/pwrq/pkg/pwrgrep"
)

// Most of the corpus is generated rather than written - see gen/ - so what
// this file checks is what generation is allowed to produce, not what any one
// rule finds.
//
// Whether a rule finds the right lines is answered somewhere else, and has to
// be: the only statement of what a translated rule should find is the
// annotated fixture beside its original, and those fixtures are not in this
// repository. gen/validate.sh runs every rule against them in a checkout and
// writes VALIDATION.json, which is committed. What this file can insist on is
// that the two files and the corpus agree with each other, and that every rule
// in it compiles.

const genDir = "gen"

// translated is one line of MANIFEST.json: a rule in the set the corpus was
// translated from, and what became of it.
type translated struct {
	ID       string   `json:"id"`
	From     string   `json:"from"`
	Ported   bool     `json:"ported"`
	Why      string   `json:"why"`
	Grammars []string `json:"grammars"`
}

// score is one line of VALIDATION.json.
type score struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	Validated bool   `json:"validated"`
	Why       string `json:"why"`
	Exact     bool   `json:"exact"`
	Found     []int  `json:"found"`
	Missed    []int  `json:"missed"`
	Wrong     []int  `json:"wrong"`
	Extra     []int  `json:"extra"`
}

func readJSON[T any](t *testing.T, name string) []T {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(genDir, name))
	if err != nil {
		t.Fatalf("%s is missing; generate.sh and validate.sh write it: %v", name, err)
	}
	var out []T
	if err := json.Unmarshal(src, &out); err != nil {
		t.Fatalf("%s is not a list: %v", name, err)
	}
	if len(out) == 0 {
		t.Fatalf("%s is empty", name)
	}
	return out
}

// TestEveryRuleCompiles is the gate that matters for a generated corpus: a
// translator that emits something which is not a query emits it hundreds of
// times.
//
// The rules are compiled and not run. Running one means walking a tree and
// parsing every file in it, which is the wrong thing to spend a test on; a
// query still has to parse, resolve `include "pwrgrep"` and bind every
// function it names, which is where a generator's mistakes are.
func TestEveryRuleCompiles(t *testing.T) {
	for _, rule := range corpus(t) {
		if _, err := rule.Compile(); err != nil {
			t.Errorf("%v", err)
		}
	}
}

// TestTheManifestAccountsForEveryRuleItWasGiven is what makes the corpus a
// complete account of the set it was translated from rather than a selection
// from it. A rule is either translated, or listed with the reason it was not -
// and a reason that says nothing is worse than none, because it looks like an
// answer.
func TestTheManifestAccountsForEveryRuleItWasGiven(t *testing.T) {
	for _, r := range readJSON[translated](t, "MANIFEST.json") {
		switch {
		case r.ID == "" || r.From == "":
			t.Errorf("a manifest entry names neither a rule nor a file: %+v", r)
		case r.Ported && len(r.Grammars) == 0:
			t.Errorf("%s (%s) is translated but names no language to search", r.ID, r.From)
		case !r.Ported && strings.TrimSpace(r.Why) == "":
			t.Errorf("%s (%s) was not translated and does not say why", r.ID, r.From)
		}
	}
}

// TestTheManifestAndTheCorpusAgree keeps the two from drifting apart, in both
// directions: a rule the manifest says was translated has a file, that file
// says it came from that rule and reports under that id, and no generated file
// is there without a rule behind it.
func TestTheManifestAndTheCorpusAgree(t *testing.T) {
	byPath := map[string]*pwrgrep.Rule{}
	for _, rule := range corpus(t) {
		byPath[rule.Path] = rule
	}

	claimed := map[string]bool{}
	for _, r := range readJSON[translated](t, "MANIFEST.json") {
		if !r.Ported {
			continue
		}
		path := strings.TrimSuffix(strings.TrimSuffix(r.From, ".yaml"), ".yml")
		rule, ok := byPath[path]
		if !ok {
			t.Errorf("%s was translated from %s, but %s.pwrq is not in the corpus", r.ID, r.From, path)
			continue
		}
		claimed[path] = true
		if rule.From != r.From {
			t.Errorf("the manifest says %s came from %s; the rule says %s", path, r.From, rule.From)
		}
		if !contains(rule.Ids, r.ID) {
			t.Errorf("%s is listed as translating %s, but its header does not name that id", path, r.ID)
		}
	}
	for path, rule := range byPath {
		// The hand-written rules are the corpus directory generation does not
		// own, so the manifest has nothing to say about them.
		if strings.HasPrefix(path, "pwrq/") || claimed[path] || rule.Origin != pwrgrep.Builtin {
			continue
		}
		t.Errorf("%s is in the corpus but in no line of MANIFEST.json", path)
	}
}

// TestEveryTranslatedRuleWasMeasured says that the validation covers the
// corpus. A rule with no entry has not been shown to do anything at all, and a
// corpus where that is allowed to happen quietly grows a tail of them.
func TestEveryTranslatedRuleWasMeasured(t *testing.T) {
	scored := map[string]bool{}
	for _, s := range readJSON[score](t, "VALIDATION.json") {
		scored[s.From+"\x00"+s.ID] = true
		if !s.Validated && strings.TrimSpace(s.Why) == "" {
			t.Errorf("%s (%s) was not validated and does not say why", s.ID, s.From)
		}
	}
	for _, r := range readJSON[translated](t, "MANIFEST.json") {
		if r.Ported && !scored[r.From+"\x00"+r.ID] {
			t.Errorf("%s (%s) is translated but VALIDATION.json does not mention it", r.ID, r.From)
		}
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
