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
	if res.Matched == matchedNone {
		t.Errorf("filter %q matched by %q", "hash", res.Matched)
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

// TestSearchReachesCmdletsNamedForNeither pins the bug that the first version
// of the tiering shipped with, and that running the httpbin session against it
// found in the first call.
//
// invoke_web_request is the cmdlet with Headers, Body and AllowAutoRedirect on
// it - the one a caller wanting to do anything beyond a bare GET needs. Its
// name does not contain "http" and its category is PowerShell, so it lives
// only in the description tier. The tiering searched descriptions solely when
// names found nothing, names found http and http_serve, and so the most
// obvious search term in the toolbox could not reach the cmdlet it most
// obviously meant.
func TestSearchReachesCmdletsNamedForNeither(t *testing.T) {
	res := search(t, "http")
	found := names(res)
	for _, want := range []string{"http", "http_serve", "invoke_web_request"} {
		if !slices.Contains(found, want) {
			t.Errorf("filter %q did not find %s; got %v", "http", want, found)
		}
	}
}

// TestBroadSearchKeepsItsExamples guards the measurement in the other
// direction. "file" matches 15 cmdlets by name and 63 more by description, and
// describeFunctions drops every entry's examples above examplesUpTo results, so
// merging unconditionally would make the searches that already worked return
// more and explain less. The held-back names are still reported, because a
// caller whose answer is in that group needs to know it exists.
func TestBroadSearchKeepsItsExamples(t *testing.T) {
	res := search(t, "file")
	if res.Count > examplesUpTo {
		t.Errorf("filter %q returned %d functions, past the %d at which examples stop",
			"file", res.Count, examplesUpTo)
	}
	if len(res.Described) == 0 {
		t.Error("filter \"file\" held back its description matches without saying so")
	}
	if len(res.Suggestions) > 0 {
		t.Errorf("a search that matched reported %d suggestions; those are for searches that did not",
			len(res.Suggestions))
	}
}

// TestMergedSearchStaysInBudget checks the rule itself rather than a single
// term: whenever the two tiers are merged, the result still fits the budget
// that made merging safe.
func TestMergedSearchStaysInBudget(t *testing.T) {
	for _, term := range []string{"http", "hash", "web", "request", "time", "compress", "file", "string", "path"} {
		res := search(t, term)
		if res.Matched == matchedBoth && res.Count > examplesUpTo {
			t.Errorf("filter %q merged both tiers into %d functions, past the %d at which examples stop",
				term, res.Count, examplesUpTo)
		}
	}
}

// TestDescriptionOnlyMatchesAreNotDuplicated checks that a cmdlet matching both
// tiers is listed once.
func TestDescriptionOnlyMatchesAreNotDuplicated(t *testing.T) {
	for _, term := range []string{"http", "hash", "compress", "web"} {
		seen := map[string]bool{}
		for _, fn := range search(t, term).Functions {
			if seen[fn.Name] {
				t.Errorf("filter %q listed %s twice", term, fn.Name)
			}
			seen[fn.Name] = true
		}
	}
}
