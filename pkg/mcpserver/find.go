package mcpserver

import (
	"sort"
	"strings"

	"github.com/xen0bit/pwrq/pkg/udf"
	"github.com/xen0bit/pwrq/pkg/udf/discovery"
	"github.com/xen0bit/pwrq/pkg/udf/similarity"
)

// How a filter found what it found. The caller is told which tier answered,
// because "these are named that" and "these merely mention it" are different
// claims and a model that cannot tell them apart will trust the second as
// though it were the first.
const (
	// matchedName means the filter matched a name, an alias or a category:
	// the caller asked for a thing and this is that thing.
	matchedName = "name"
	// matchedDescription means nothing was named or categorised that way, and
	// these are the cmdlets whose description mentions it.
	matchedDescription = "description"
	// matchedBoth means the name tier answered and the description tier added
	// to it, which it may do only while the combined list stays small enough
	// to keep its examples.
	matchedBoth = "name-and-description"
	// matchedNone means neither tier found anything, and what came back is
	// suggestions rather than results.
	matchedNone = "none"
)

// findFunctions filters the cmdlet catalogue by a caller's search term.
//
// Names and descriptions are searched together, but a description-only match
// earns its place only when the combined list still fits the examples budget.
// The reason is a measurement in both directions. Searching descriptions always
// is too much: "file" matches 15 cmdlets by name and 63 more by description,
// "string" 70 and 42, and describeFunctions drops the examples above forty
// results - so the searches that already worked would return more and explain
// less. Searching them never is too little, and that was the first version's
// bug: "http" matches exactly two cmdlets by name, and invoke_web_request -
// the one with Headers, Body and AllowAutoRedirect on it - is named for
// neither http nor its category, so the single most obvious search term in the
// toolbox could not reach the cmdlet it most obviously meant.
//
// Merging when it fits settles both: "http" widens from 2 to 7 and finds it,
// "hash" from 12 to 20, while "file" and "string" stay as they were and say how
// many they held back.
//
// Matching is case-insensitive in both directions. It used to be neither: the
// filter went straight into strings.Contains against the name and the
// category, so "hash" found bcrypt_hash and geohash_encode but not one of the
// eight cmdlets in the category called Hash, and "stat" found nothing at all
// while Statistics held thirty-five.
func findFunctions(filter string) ([]functionInfo, string, []string) {
	catalog := catalogue()

	if filter == "" {
		out := make([]functionInfo, 0, len(catalog))
		for _, c := range catalog {
			out = append(out, describe(c))
		}
		return out, matchedName, nil
	}

	needle := strings.ToLower(filter)

	named := collect(catalog, needle, byName)
	// Description-only, so that a cmdlet matching both tiers is reported as
	// the stronger of the two rather than listed twice.
	described := collect(catalog, needle, func(c discovery.Command, n string) bool {
		return !byName(c, n) && byDescription(c, n)
	})

	switch {
	case len(named) == 0 && len(described) == 0:
		return nil, matchedNone, suggest(catalog, needle)
	case len(named) == 0:
		return described, matchedDescription, nil
	case len(described) == 0:
		return named, matchedName, nil
	case len(named)+len(described) <= examplesUpTo:
		return merge(catalog, named, described), matchedBoth, nil
	default:
		return named, matchedName, describedNames(described)
	}
}

// merge returns the two tiers as one list in catalogue order, so that a search
// answering from both does not read as two lists stapled together.
func merge(catalog []discovery.Command, named, described []functionInfo) []functionInfo {
	keep := make(map[string]bool, len(named)+len(described))
	for _, f := range named {
		keep[f.Name] = true
	}
	for _, f := range described {
		keep[f.Name] = true
	}
	out := make([]functionInfo, 0, len(keep))
	for _, c := range catalog {
		if keep[c.Name] {
			out = append(out, describe(c))
		}
	}
	return out
}

// describedNames lists the cmdlets held back by the budget, so that a caller
// whose answer is in that group is told it exists rather than told it does not.
func describedNames(described []functionInfo) []string {
	out := make([]string, len(described))
	for i, f := range described {
		out[i] = f.Name
	}
	return out
}

// collect gathers the catalogue entries a predicate accepts, preserving the
// catalogue's own order so two searches that overlap agree on it.
func collect(catalog []discovery.Command, needle string, match func(discovery.Command, string) bool) []functionInfo {
	var out []functionInfo
	for _, c := range catalog {
		if match(c, needle) {
			out = append(out, describe(c))
		}
	}
	return out
}

// byName matches the three things a cmdlet is called: its name, the aliases
// that reach it, and the group it belongs to.
func byName(c discovery.Command, needle string) bool {
	if contains(c.Name, needle) || contains(c.Category, needle) {
		return true
	}
	for _, alias := range c.Aliases {
		if contains(alias, needle) {
			return true
		}
	}
	return false
}

// byDescription matches the prose a cmdlet is documented by: its own sentence,
// and the options it takes. This is the tier that turns "header" from a dead
// end into jwt_decode, and "redirect" into invoke_web_request - which really
// does control redirects, through AllowAutoRedirect, and is named for none of
// that.
func byDescription(c discovery.Command, needle string) bool {
	if contains(c.Description, needle) {
		return true
	}
	for _, o := range c.Options {
		if contains(o.Name, needle) || contains(o.Description, needle) {
			return true
		}
	}
	return false
}

func contains(haystack, lowerNeedle string) bool {
	return strings.Contains(strings.ToLower(haystack), lowerNeedle)
}

// shortestMeaningfulWord is the length below which a word out of a cmdlet name
// says nothing about what the caller wanted. It is set where it is so that
// "to", "of", "is" and "from" fall out and "json", "date", "upper" and "hash"
// stay in: a name is mostly its nouns, and the glue words match everything.
const shortestMeaningfulWord = 4

// meaningfulWords splits a name a caller typed into the words worth searching
// for, so that to_upper can be matched on "upper".
func meaningfulWords(name string) []string {
	var out []string
	for _, word := range strings.FieldsFunc(name, func(r rune) bool { return r == '_' || r == '-' }) {
		if len(word) >= shortestMeaningfulWord {
			out = append(out, strings.ToLower(word))
		}
	}
	return out
}

// suggestionLimit is how many near misses are worth offering. Enough to cover
// a plausible typo and a plausible synonym, few enough that the answer to "no
// match" stays shorter than a real result would be.
const suggestionLimit = 6

// maxSuggestionDistance is how far a name may be from the search term and
// still be offered. Three edits covers a transposition, a dropped letter and a
// wrong one; beyond that the suggestions stop being about what was typed.
const maxSuggestionDistance = 3

// suggest offers the names and categories closest to a search term that
// matched nothing, so a caller whose guess was wrong is pointed somewhere
// rather than told to guess again.
//
// A term that is a prefix of a name, or contains one, counts as closest of
// all: someone searching for "redirects" should be shown "redirect_*" ahead of
// anything three edits away, and edit distance alone would not do that because
// the lengths differ too much.
//
// jq's own names are offered alongside the cmdlets, because a caller who
// guessed wrong guessed at the whole language rather than at half of it. The
// case that made this necessary: `from_json` used to come back as "no cmdlet
// is named from_json; list_functions has the vocabulary", sending the caller
// to a catalogue that does not contain fromjson - one edit away, and the
// answer.
func suggest(catalog []discovery.Command, needle string) []string {
	type candidate struct {
		text     string
		distance int
	}

	seen := make(map[string]bool)
	var found []candidate

	consider := func(text, label string) {
		lower := strings.ToLower(text)
		if lower == "" || seen[label] {
			return
		}
		distance := similarity.Levenshtein(lower, needle)
		if strings.HasPrefix(lower, needle) || strings.HasPrefix(needle, lower) {
			distance = 0
		}
		if distance > maxSuggestionDistance {
			return
		}
		seen[label] = true
		found = append(found, candidate{text: label, distance: distance})
	}

	for _, c := range catalog {
		consider(c.Name, c.Name)
		for _, alias := range c.Aliases {
			consider(alias, alias)
		}
		consider(c.Category, "category "+c.Category)
	}
	for _, name := range builtinNames() {
		consider(name, name)
	}
	// And by meaning, not only by spelling. A caller who wants to uppercase a
	// string writes to_upper, which is nine edits from ascii_upcase and was
	// coming back as "did you mean tonumber?". Reading the words out of the
	// name they typed and matching those against what the builtins do turns
	// to_upper into ascii_upcase and parse_json into fromjson.
	//
	// Offered at the far end of the range, so a genuine typo still outranks a
	// guess at what the caller meant.
	for _, word := range meaningfulWords(needle) {
		for _, b := range jqBuiltins() {
			if b.Gloss == "" || seen[b.Name] || !contains(b.Gloss, word) {
				continue
			}
			seen[b.Name] = true
			found = append(found, candidate{text: b.Name, distance: maxSuggestionDistance})
		}
	}

	sort.SliceStable(found, func(i, j int) bool {
		if found[i].distance != found[j].distance {
			return found[i].distance < found[j].distance
		}
		return found[i].text < found[j].text
	})

	out := make([]string, 0, suggestionLimit)
	for _, c := range found {
		if len(out) == suggestionLimit {
			break
		}
		out = append(out, c.text)
	}
	return out
}

// catalogue reads the published cmdlet catalogue, building the registry first
// if nothing has yet.
//
// An empty catalogue does not fail here; it answers, and answers wrongly.
// "no cmdlet is named sha256" is a confident lie of exactly the kind this file
// was written to stop, so the one line that prevents it goes where the
// catalogue is read rather than at each of the callers.
func catalogue() []discovery.Command {
	udf.DefaultRegistry()
	return discovery.Catalog()
}

// describe converts a catalogue entry into the tool's own result type.
func describe(c discovery.Command) functionInfo {
	return functionInfo{
		Name:        c.Name,
		MinArgs:     c.MinArgs,
		MaxArgs:     c.MaxArgs,
		Category:    c.Category,
		Description: c.Description,
		Examples:    c.Examples,
		Aliases:     c.Aliases,
		Streaming:   c.Streaming,
		Output:      c.EmitsDescription(),
		Input:       c.Input,
		Accepts:     c.Accepts,
		Returns:     c.Returns,
		Options:     options(c),
		Shape:       c.ShapeDescription(),
		TypeName:    c.Shape.TypeName(),
	}
}

// options converts a cmdlet's documented option keys into the tool's result
// type.
func options(c discovery.Command) []optionInfo {
	if len(c.Options) == 0 {
		return nil
	}
	out := make([]optionInfo, len(c.Options))
	for i, o := range c.Options {
		out[i] = optionInfo{Name: o.Name, Type: o.Type, Description: o.Description}
	}
	return out
}
