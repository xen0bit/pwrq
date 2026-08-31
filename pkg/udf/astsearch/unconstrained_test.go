package astsearch

import "testing"

// TestMatchesAnythingAgreesWithMatchesText is the check that matters for the
// short circuit: a pattern it calls unconstrained must be one MatchesText
// would have said yes to anyway, whatever it is asked about. If the two ever
// disagree the rules that lean on it start reporting different findings, which
// is the one failure this is not allowed to cause.
func TestMatchesAnythingAgreesWithMatchesText(t *testing.T) {
	sources := map[string][]string{
		"python": {
			"x", "1", "os.getenv(\"X\")", "args", "self.request.args.get('a')",
			"[1, 2, 3]", "{'a': 1}", "lambda x: x", "f(a, b)", "\"a string\"",
		},
		"go": {
			"x", "42", "os.Getenv(\"X\")", "md5.New()", "[]byte{1}",
			"func() {}", "a.b.c", "\"s\"",
		},
		"javascript": {
			"x", "1", "process.env.X", "eval(a)", "[1,2]", "({a:1})", "`t`",
		},
	}
	patterns := []string{
		// Unconstrained: a hole and nothing else.
		"$ARGS", "$X", "$_", "$$$X", "$$$_",
		// Constrained: these say something about the code.
		"os.getenv($X)", "f($A, $B)", "md5.New()", "x", "1", "$A.$B($$$C)",
	}

	for language, texts := range sources {
		for _, pattern := range patterns {
			free := MatchesAnything(pattern, language)
			if !free {
				continue
			}
			for _, source := range texts {
				ok, err := MatchesText(pattern, language, source)
				if err != nil {
					t.Fatalf("%s: pattern %q called unconstrained, but matching %q errored: %v",
						language, pattern, source, err)
				}
				if !ok {
					t.Fatalf("%s: pattern %q called unconstrained, but it does not match %q - "+
						"the short circuit would keep a match the long way drops",
						language, pattern, source)
				}
			}
		}
	}
}

// TestMatchesAnythingSaysNoToAPatternThatConstrains guards the other
// direction. Calling a real pattern unconstrained would keep everything, which
// is a rule reporting findings it did not find.
func TestMatchesAnythingSaysNoToAPatternThatConstrains(t *testing.T) {
	for _, c := range []struct {
		pattern  string
		language string
	}{
		{"os.getenv($X)", "python"},
		{"f($A, $B)", "python"},
		{"md5.New()", "go"},
		{"$A.$B($$$C)", "go"},
		{"eval($X)", "javascript"},
		// A bare word is a pattern: it matches that identifier, not any node.
		{"x", "python"},
		{"1", "python"},
	} {
		if MatchesAnything(c.pattern, c.language) {
			t.Errorf("%s: pattern %q constrains something, but was called unconstrained",
				c.language, c.pattern)
		}
	}
}

// TestMatchesAnythingSaysNoWithoutAGrammar keeps the short circuit from
// answering for a language this build cannot parse, where MatchesText reports
// no match rather than a match.
func TestMatchesAnythingSaysNoWithoutAGrammar(t *testing.T) {
	if MatchesAnything("$X", "no-such-language") {
		t.Fatal("a language with no grammar cannot have an unconstrained pattern")
	}
	if ok, _ := MatchesText("$X", "no-such-language", "x"); ok {
		t.Fatal("MatchesText should not match without a grammar either")
	}
}
