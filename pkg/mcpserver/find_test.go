package mcpserver

import (
	"slices"
	"strings"
	"testing"

	"github.com/xen0bit/pwrq/pkg/udf"
)

// names pulls the cmdlet names out of a search, for tests that care which
// cmdlets came back rather than how many.
func names(res listFunctionsResult) []string {
	out := make([]string, len(res.Functions))
	for i, fn := range res.Functions {
		out[i] = fn.Name
	}
	return out
}

func search(t *testing.T, filter string) listFunctionsResult {
	t.Helper()
	udf.DefaultRegistry()
	return listFunctions(listFunctionsArgs{Filter: filter})
}

// TestFilterMatchesCategoryCaseInsensitively is the regression this whole file
// exists for.
//
// The filter used to go straight into strings.Contains against the name and
// the category, so a search for "hash" found bcrypt_hash and the two geohash
// cmdlets and not one of the eight in the category spelled "Hash". A model
// working through the MCP server therefore never saw sha256 in a listing: the
// only way to reach it was to already know it was there.
func TestFilterMatchesCategoryCaseInsensitively(t *testing.T) {
	res := search(t, "hash")
	found := names(res)

	for _, want := range []string{"md5", "sha256", "sha512", "bcrypt_hash"} {
		if !slices.Contains(found, want) {
			t.Errorf("filter %q did not find %s; got %v", "hash", want, found)
		}
	}
	if res.Matched != matchedName {
		t.Errorf("filter %q matched by %q, want %q", "hash", res.Matched, matchedName)
	}
}

// TestFilterFindsWholeCategories pins the other half of the same bug. The
// category is "Statistics"; the lower-case search that a model actually types
// found nothing, while thirty-five cmdlets sat behind the capital S.
func TestFilterFindsWholeCategories(t *testing.T) {
	res := search(t, "stat")
	if res.Count < 10 {
		t.Fatalf("filter %q found %d functions, want the Statistics category", "stat", res.Count)
	}
	for _, fn := range res.Functions {
		if strings.EqualFold(fn.Category, "Statistics") {
			return
		}
	}
	t.Errorf("filter %q found %d functions but none from Statistics", "stat", res.Count)
}

// TestFilterIsCaseInsensitiveInBothDirections checks that a caller who types
// the category as it is printed gets the same answer as one who types it
// lower-case, since list_functions prints it capitalised.
func TestFilterIsCaseInsensitiveInBothDirections(t *testing.T) {
	lower := names(search(t, "compression"))
	upper := names(search(t, "Compression"))
	if !slices.Equal(lower, upper) {
		t.Errorf("case changed the answer: %v vs %v", lower, upper)
	}
	if len(lower) == 0 {
		t.Error("filter \"compression\" found nothing")
	}
}

// TestNameTierWinsOverDescriptions guards the measurement the tiering is built
// on. "http" matches two cmdlets by name and five more by description; folding
// the two tiers together would push some searches past the forty-entry
// threshold at which describeFunctions stops printing examples, making the
// searches that already worked less useful.
func TestNameTierWinsOverDescriptions(t *testing.T) {
	res := search(t, "http")
	if res.Matched != matchedName {
		t.Fatalf("filter %q matched by %q, want %q", "http", res.Matched, matchedName)
	}
	for _, fn := range res.Functions {
		if !strings.Contains(strings.ToLower(fn.Name), "http") &&
			!strings.Contains(strings.ToLower(fn.Category), "http") {
			t.Errorf("filter %q returned %s, which is named for neither", "http", fn.Name)
		}
	}
}

// TestDescriptionTierAnswersWhenNamesDoNot covers the searches that used to
// come back empty. Nothing is named "header", but jwt_decode's description
// says what it does with them.
func TestDescriptionTierAnswersWhenNamesDoNot(t *testing.T) {
	res := search(t, "header")
	if res.Count == 0 {
		t.Fatal("filter \"header\" found nothing, not even by description")
	}
	if res.Matched != matchedDescription {
		t.Errorf("filter %q matched by %q, want %q", "header", res.Matched, matchedDescription)
	}

	// And the text has to say which tier answered, or the caller reads a
	// description match as though the cmdlet were named for what they asked.
	text := describeFunctions(listFunctionsArgs{Filter: "header"}, res)
	if !strings.Contains(text, "description") {
		t.Errorf("the rendered answer does not say it matched on descriptions:\n%s", text)
	}
}

// TestNoMatchSuggestsSomething checks that the one answer a caller cannot act
// on is never the answer. A search that finds nothing offers the nearest names
// instead, so the next call is a call rather than another guess.
func TestNoMatchSuggestsSomething(t *testing.T) {
	res := search(t, "sha257")
	if res.Count != 0 {
		t.Fatalf("filter %q unexpectedly matched %d functions", "sha257", res.Count)
	}
	if res.Matched != matchedNone {
		t.Errorf("filter %q matched by %q, want %q", "sha257", res.Matched, matchedNone)
	}
	if !slices.Contains(res.Suggestions, "sha256") {
		t.Errorf("filter %q suggested %v, want sha256 among them", "sha257", res.Suggestions)
	}

	text := describeFunctions(listFunctionsArgs{Filter: "sha257"}, res)
	if !strings.Contains(text, "sha256") {
		t.Errorf("the rendered answer drops the suggestions:\n%s", text)
	}
}

// TestSuggestionsAreBounded keeps the no-match answer shorter than a real one.
func TestSuggestionsAreBounded(t *testing.T) {
	res := search(t, "e")
	if len(res.Suggestions) > suggestionLimit {
		t.Errorf("got %d suggestions, want at most %d", len(res.Suggestions), suggestionLimit)
	}
}

// TestEmptyFilterListsEverything checks the unfiltered call still enumerates
// the whole catalogue, which is what the tool's description promises.
func TestEmptyFilterListsEverything(t *testing.T) {
	res := search(t, "")
	if res.Count < 400 {
		t.Errorf("unfiltered listing returned %d functions, want the whole catalogue", res.Count)
	}
	if res.Matched != matchedName {
		t.Errorf("unfiltered listing matched by %q, want %q", res.Matched, matchedName)
	}
}
