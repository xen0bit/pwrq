package suggest

import (
	"strings"
	"unicode"

	"github.com/xen0bit/pwrq/pkg/d1"
)

// Spelling. A slot's words are copied from the comment, typos and all, and
// some go on into a question the model is asked again at run time: "Does
// the image contain shimp?" is a question about nothing, and scores every
// picture alike, where "shrimp" separates them cleanly (0.99 against 0.01
// on the images it was measured on). So the words of a phrase slot are
// checked against the model's own vocabulary: a word the tokenizer knows as
// one whole token is a word; one it does not, within one edit of words it
// does, is taken for the most frequent of them - lower ids are merges the
// tokenizer learned earlier, from text where they were commoner.

// vocab answers whether a word is one token, and its id.
type vocab interface {
	ID(token string) (int32, bool)
}

// Correct implements ideengine.Speller with the loaded model's tokenizer,
// and leaves text alone while no model is loaded.
func (r *Ranker) Correct(text string) string {
	m, ok := d1.Loaded()
	if !ok {
		return text
	}
	return correct(m.Builder().Tokenizer(), text)
}

func correct(v vocab, text string) string {
	words := strings.Fields(text)
	for i, w := range words {
		core := strings.TrimFunc(w, func(r rune) bool { return !unicode.IsLetter(r) })
		if len(core) < 4 || core != strings.ToLower(core) || !isASCIIAlpha(core) {
			continue
		}
		if _, known := v.ID("Ġ" + core); known {
			continue
		}
		if fix := bestNeighbour(v, core); fix != "" {
			words[i] = strings.Replace(w, core, fix, 1)
		}
	}
	return strings.Join(words, " ")
}

func isASCIIAlpha(s string) bool {
	for _, r := range s {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}

// bestNeighbour is a whole-token word one edit from w. Edits are tried in
// the order typists make them - two letters swapped, a letter left out, a
// wrong letter, a letter doubled or stray - and the commonest word of the
// first kind that finds any wins: "shimp" is shrimp with the r left out,
// not ship with an m too many.
func bestNeighbour(v vocab, w string) string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	best, bestID := "", int32(1<<30)
	try := func(c string) {
		if id, ok := v.ID("Ġ" + c); ok && id < bestID {
			best, bestID = c, id
		}
	}
	tiers := []func(){
		func() { // transposition
			for i := 0; i+1 < len(w); i++ {
				try(w[:i] + string(w[i+1]) + string(w[i]) + w[i+2:])
			}
		},
		func() { // a letter left out
			for i := 0; i <= len(w); i++ {
				for _, c := range letters {
					try(w[:i] + string(c) + w[i:])
				}
			}
		},
		func() { // a wrong letter
			for i := 0; i < len(w); i++ {
				for _, c := range letters {
					try(w[:i] + string(c) + w[i+1:])
				}
			}
		},
		func() { // a letter too many
			for i := 0; i < len(w); i++ {
				try(w[:i] + w[i+1:])
			}
		},
	}
	for _, tier := range tiers {
		tier()
		if best != "" {
			return best
		}
	}
	return ""
}
