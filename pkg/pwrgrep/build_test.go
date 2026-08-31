package pwrgrep_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Which grammars a binary can parse is decided by build tags, and which ones
// the rules need is decided by the rules. Nothing connected the two, and the
// gap was not theoretical: 82 rules shipped in a release binary that could
// never fire, because their languages were not in the build.
//
// A rule that cannot fire is worse than a rule that is missing. It is in the
// catalogue, it runs, it reports nothing, and nothing anywhere says why.

var makefileGrammars = regexp.MustCompile(`(?ms)^GRAMMARS := (.*?)^GRAMMAR_TAGS`)

// TestEveryRuleLanguageIsInTheBuild is the check that keeps the two in step.
func TestEveryRuleLanguageIsInTheBuild(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "Makefile"))
	if err != nil {
		t.Fatalf("reading the Makefile: %v", err)
	}
	match := makefileGrammars.FindStringSubmatch(string(src))
	if match == nil {
		t.Fatal("the Makefile no longer defines GRAMMARS before GRAMMAR_TAGS")
	}
	built := map[string]bool{}
	for _, language := range strings.Fields(strings.ReplaceAll(match[1], "\\", " ")) {
		built[language] = true
	}
	if len(built) == 0 {
		t.Fatal("the Makefile's GRAMMARS list is empty")
	}

	missing := map[string][]string{}
	for _, rule := range corpus(t) {
		for _, language := range rule.Languages {
			if !built[language] {
				missing[language] = append(missing[language], rule.Path)
			}
		}
	}
	names := make([]string, 0, len(missing))
	for language := range missing {
		names = append(names, language)
	}
	sort.Strings(names)
	for _, language := range names {
		t.Errorf("%d rule(s) are written for %s, which the release build does not carry, "+
			"so they ship and can never fire - add it to GRAMMARS in the Makefile and to "+
			"both build stanzas in .goreleaser.yaml, or drop the rules (e.g. %s)",
			len(missing[language]), language, missing[language][0])
	}
}

// TestEveryRuleSaysWhatItIsWrittenFor keeps the header from being left off,
// which would make the check above pass by knowing nothing.
func TestEveryRuleSaysWhatItIsWrittenFor(t *testing.T) {
	structural, text := 0, 0
	for _, rule := range corpus(t) {
		if len(rule.Languages) > 0 {
			structural++
			continue
		}
		// A rule that searches text is written for no grammar, and says so by
		// naming none. It has to be searching text to get away with it.
		if !strings.Contains(rule.Query, "scan_regex") {
			t.Errorf("%s names no language and does not search text, so nothing says "+
				"which grammars it needs", rule.Path)
		}
		text++
	}
	if structural == 0 || text == 0 {
		t.Fatalf("the corpus has %d structural and %d text rules; this test proves nothing "+
			"unless it has both", structural, text)
	}
}
