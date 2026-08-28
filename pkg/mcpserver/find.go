package mcpserver

import (
	"sort"
	"strings"

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
	// matchedNone means neither tier found anything, and what came back is
	// suggestions rather than results.
	matchedNone = "none"
)

// findFunctions filters the cmdlet catalogue by a caller's search term.
//
// The search runs in tiers rather than in one pass, and the reason is a
// measurement. Matching descriptions alongside names looks strictly better
// until you count: "file" matches 15 cmdlets by name and 71 by description,
// and describeFunctions drops the examples above forty results. So folding
// descriptions into every search would make the searches that already work
// return more and explain less. Descriptions are searched when the name tier
// comes back empty, which is exactly when they help and never when they hurt.
//
// Matching is case-insensitive in both directions. It used to be neither: the
// filter went straight into strings.Contains against the name and the
// category, so "hash" found bcrypt_hash and geohash_encode but not one of the
// eight cmdlets in the category called Hash, and "stat" found nothing at all
// while Statistics held thirty-five.
func findFunctions(filter string) ([]functionInfo, string, []string) {
	// The registry publishes the catalogue as it is built, so make sure it has
	// been. Every other entry point has already done this, but a caller that
	// only ever lists functions would otherwise see an empty catalogue.
	catalog := discovery.Catalog()

	if filter == "" {
		out := make([]functionInfo, 0, len(catalog))
		for _, c := range catalog {
			out = append(out, describe(c))
		}
		return out, matchedName, nil
	}

	needle := strings.ToLower(filter)

	if named := collect(catalog, needle, byName); len(named) > 0 {
		return named, matchedName, nil
	}
	if described := collect(catalog, needle, byDescription); len(described) > 0 {
		return described, matchedDescription, nil
	}
	return nil, matchedNone, suggest(catalog, needle)
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
