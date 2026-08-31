package pwrgrep

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// A rule can ask a question about what a hole caught that is neither a shape
// nor a pattern: is this string random enough to be a secret, is this regex
// one an attacker can hang the process with. Both are properties of the text
// rather than of the syntax, and neither is a regex anyone could write, so
// each is an operator of its own.

// entropyFloor is the length below which randomness says nothing. Four random
// characters look exactly like a word, and the rules that ask this question
// are about keys and tokens, which are not four characters long.
const entropyFloor = 16

// entropyShare is how much of the randomness a string of that length could
// have that it must actually have.
//
// A string of n distinct characters can carry at most log2(n) bits per
// character, so the test is against what this string could have been rather
// than against a fixed number of bits: `0123456789abcdef0123456789abcdef` is
// random for a hex key and would fail a flat threshold set for base64.
const entropyShare = 0.6

// RegisterWhereCaptureEntropy registers where_capture_entropy: keep the
// matches whose named hole caught a string random enough to be a secret.
//
// This is what a rule means by "and the value is not a placeholder". Every
// hardcoded-credential rule in a corpus needs it, because the pattern - an
// assignment to something called `token` - matches the example in the README
// as readily as it matches the key that should not have been committed.
//
// The measure is Shannon entropy over the characters, as a share of the most
// the string could have carried, with a floor on the length. It is an
// approximation of the analyzer the rules were written against, and it is
// deliberately the recall-favouring direction: a rule that reports a
// suspicious constant is doing its job, and one that reports nothing is not.
//
//	$all | of($assignments) | where_capture_entropy("VALUE")
func RegisterWhereCaptureEntropy() gojq.CompilerOption {
	return analysisOp("where_capture_entropy", random)
}

// RegisterWhereCaptureRedos registers where_capture_redos: keep the matches
// whose named hole caught a regex an input can make run for ever.
//
//	$all | of($compiles) | where_capture_redos("PATTERN")
func RegisterWhereCaptureRedos() gojq.CompilerOption {
	return analysisOp("where_capture_redos", explosive)
}

// analysisOp is the shape both share: a hole, and a question about the text it
// caught that takes no argument.
func analysisOp(name string, decide func(string) bool) gojq.CompilerOption {
	common.DeclareInput(name, common.InputPipeline)
	return common.WithFunction(name, 1, 2, func(v any, args []any) any {
		in, rest := common.SplitInput(v, args, 1)
		list, err := matches(name, in)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		hole, err := common.BindString(rest[0], "hole name")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: %v", name, err), nil)
		}
		return keepIf(list, func(m map[string]any) bool {
			return decide(unquote(capture(m, hole)))
		})
	})
}

// unquote takes the quotes off what a hole caught, because a hole that caught
// a string literal caught the quotes with it and the question is about the
// string.
func unquote(s string) string {
	s = strings.TrimSpace(s)
	for _, q := range []string{`"`, "'", "`"} {
		if len(s) >= 2 && strings.HasPrefix(s, q) && strings.HasSuffix(s, q) {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// random reports whether a string carries enough of the randomness a string of
// its length could carry to be a key rather than a word.
func random(s string) bool {
	n := utf8.RuneCountInString(s)
	if n < entropyFloor {
		return false
	}
	counts := map[rune]int{}
	for _, r := range s {
		counts[r]++
	}
	bits := 0.0
	for _, c := range counts {
		p := float64(c) / float64(n)
		bits -= p * math.Log2(p)
	}
	// The ceiling is what a string drawn from this many distinct characters
	// could carry; a string with one character in it can carry nothing and is
	// not random however long it is.
	ceiling := math.Log2(float64(len(counts)))
	return ceiling > 0 && bits >= entropyShare*ceiling
}

// nestedQuantifier matches a group that repeats something that itself repeats,
// which is the shape that makes a regex engine try every way of splitting the
// input: `(a+)+`, `(a*)*`, `([a-z]+)*`.
var nestedQuantifier = regexp.MustCompile(`\((?:[^()\\]|\\.)*[*+]\)?(?:[^()\\]|\\.)*\)\s*(?:[*+]|\{\d+,\d*\})`)

// repeatedAlternation matches a repeated alternation whose branches can match
// the same text - `(a|a)*`, `(\w|\d)+` - which is the other way in.
var repeatedAlternation = regexp.MustCompile(`\((?:[^()\\]|\\.)*\|(?:[^()\\]|\\.)*\)\s*(?:[*+]|\{\d+,\d*\})`)

// explosive reports whether a regex has a shape a backtracking engine can be
// made to spend exponential time on.
//
// It is the shape that is looked for, not the behaviour: deciding the question
// properly means simulating the automaton, and what the rules asking it want
// to know is whether a person wrote one of the two well-known traps. pwrq's
// own matching is RE2 and cannot backtrack at all, which is exactly why a rule
// about someone else's regex has to be a search over the text of it.
func explosive(s string) bool {
	return nestedQuantifier.MatchString(s) || repeatedAlternation.MatchString(s)
}
