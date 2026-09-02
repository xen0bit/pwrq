package pwrgrep_test

import "testing"

// TestEveryRuleCompiles is the gate the corpus is admitted through.
//
// Most of it was translated rather than written, and a translator that emits
// something which is not a query emits it hundreds of times. That the corpus
// now lives in its own repository does not move this check out of pwrq: what
// compiles is decided by the cmdlet vocabulary in this binary, so a rule that
// resolves against the corpus repository's pwrq and not against this one is a
// break only this side can see.
//
// The rules are compiled and not run. Running one means walking a tree and
// parsing every file in it, which is the wrong thing to spend a test on; a
// query still has to parse, resolve `include "pwrgrep"` and bind every
// function it names, which is where the mistakes are.
//
// Whether a rule finds the right lines is answered by the fixtures - see
// rules_test.go, and tools/validate.py in the corpus repository, which runs
// the whole of it against every fixture on every change there.
func TestEveryRuleCompiles(t *testing.T) {
	for _, rule := range corpus(t) {
		if _, err := rule.Compile(); err != nil {
			t.Errorf("%v", err)
		}
	}
}
