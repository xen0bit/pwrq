package astsearch

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Which grammars a binary can parse is decided by build tags, and two things
// build pwrq: the Makefile, for a checkout, and .goreleaser.yaml, for
// everything anyone installs. Neither can read the other, so the list is
// written twice and this test is what keeps the two honest.
//
// The failure it exists to prevent is silent and expensive. goreleaser never
// consults the Makefile, so a grammar list added only there leaves every
// released binary embedding all 206 of gotreesitter's grammars - correct,
// 14MB larger, and on eight architectures. The other direction is worse: a
// language dropped from one list and not the other means get_ast_language
// answers differently depending on where the binary came from.

var (
	makefileGrammars = regexp.MustCompile(`(?ms)^GRAMMARS := (.*?)^GRAMMAR_TAGS`)
	subsetTag        = regexp.MustCompile(`grammar_subset_([a-z0-9_]+)`)
	// buildID opens a goreleaser build stanza. Each is checked on its own:
	// scanning the whole file would let one build's tag list stand in for the
	// other's, and the first version of this test did exactly that - dropping
	// a language from pwrq passed, because pwrq-viz still named it.
	buildID = regexp.MustCompile(`(?m)^  - id: (\S+)`)
)

// repoFile reads a file from the repository root, which is three levels above
// this package.
func repoFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", name))
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return string(data)
}

// makefileLanguages reads the list a checkout builds.
func makefileLanguages(t *testing.T) []string {
	t.Helper()
	match := makefileGrammars.FindStringSubmatch(repoFile(t, "Makefile"))
	if match == nil {
		t.Fatal("the Makefile no longer defines GRAMMARS before GRAMMAR_TAGS; " +
			"this test can no longer tell what a checkout builds")
	}
	langs := strings.Fields(strings.ReplaceAll(match[1], "\\", " "))
	if len(langs) == 0 {
		t.Fatal("the Makefile's GRAMMARS list is empty, so select_ast can parse nothing")
	}
	return langs
}

// releaseBuilds splits .goreleaser.yaml into its build stanzas, keyed by id.
//
// An id repeats: the same two names head a stanza under builds, under archives
// and under nfpms. Only the ones that name a main package are builds, and they
// are the only ones that carry tags - keying on the id alone kept the last
// stanza of each name, which is a packaging block with no tags in it, and the
// test then reported that neither binary passed any.
func releaseBuilds(t *testing.T) map[string]string {
	t.Helper()
	release := repoFile(t, ".goreleaser.yaml")
	starts := buildID.FindAllStringSubmatchIndex(release, -1)
	out := map[string]string{}
	for i, start := range starts {
		end := len(release)
		if i+1 < len(starts) {
			end = starts[i+1][0]
		}
		stanza := release[start[0]:end]
		if !strings.Contains(stanza, "main: ./cmd/") {
			continue
		}
		out[release[start[2]:start[3]]] = stanza
	}
	return out
}

func TestGrammarTagsMatchTheReleaseBuild(t *testing.T) {
	fromMake := makefileLanguages(t)
	builds := releaseBuilds(t)

	for _, id := range []string{"pwrq", "pwrq-viz"} {
		t.Run(id, func(t *testing.T) {
			stanza, ok := builds[id]
			if !ok {
				t.Fatalf(".goreleaser.yaml has no build called %q; this test cannot "+
					"tell what that binary would ship with", id)
			}
			if !strings.Contains(stanza, "- grammar_subset\n") {
				t.Fatalf("the %s build does not pass the grammar_subset tag, so it embeds "+
					"all of gotreesitter's grammars rather than the chosen ones", id)
			}

			named := map[string]bool{}
			for _, m := range subsetTag.FindAllStringSubmatch(stanza, -1) {
				named[m[1]] = true
			}

			var missing []string
			for _, lang := range fromMake {
				if !named[lang] {
					missing = append(missing, lang)
				}
				delete(named, lang)
			}
			var extra []string
			for lang := range named {
				extra = append(extra, lang)
			}
			sort.Strings(missing)
			sort.Strings(extra)

			if len(missing) > 0 {
				t.Errorf("the Makefile builds %s and the %s release does not, so a released "+
					"binary would answer get_ast_language differently from a checkout",
					strings.Join(missing, ", "), id)
			}
			if len(extra) > 0 {
				t.Errorf("the %s release builds %s and the Makefile does not, so a checkout "+
					"cannot reproduce what was shipped", id, strings.Join(extra, ", "))
			}
		})
	}
}
