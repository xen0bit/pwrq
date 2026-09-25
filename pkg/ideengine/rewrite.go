package ideengine

import (
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/jqfmt"
	"github.com/xen0bit/pwrq/pkg/jqinline"
)

// The three rewrites a query box wants. None of them evaluates anything, so
// none depends on the vocabulary, and none changes what a query means.

// FormatRequest asks for a query to be tidied.
type FormatRequest struct {
	Query string `json:"query"`
}

// FormatResponse carries the rewritten query. On a parse error the query
// comes back as it was, so an editor can apply the response unconditionally.
type FormatResponse struct {
	Query string `json:"query"`
	Error string `json:"error,omitempty"`
}

// Format pretty-prints a query onto multiple lines, breaking top-level
// pipelines and folding long objects, arrays and conditionals under
// indentation. The result parses back to the same program.
func Format(src string) FormatResponse {
	return rewrite(src, jqfmt.Format)
}

// Minify renders a query on a single line: the canonical form, spacing
// normalised and whitespace stripped. It is Format's inverse in spirit - one
// spreads the query for reading, the other compacts it for storing or
// sharing.
func Minify(src string) FormatResponse {
	return rewrite(src, jqfmt.Minify)
}

func rewrite(src string, render func(*gojq.Query) string) FormatResponse {
	if strings.TrimSpace(src) == "" {
		return FormatResponse{Query: src}
	}
	query, err := gojq.Parse(src)
	if err != nil {
		return FormatResponse{Query: src, Error: err.Error()}
	}
	return FormatResponse{Query: render(query)}
}

// InlineRequest asks for a query's definitions to be expanded where they are
// called.
type InlineRequest struct {
	Query string `json:"query"`
}

// InlineResponse carries the expanded query, how many calls it replaced, and
// what it had to leave alone. Kept names each definition that is still there
// and why, so the editor can say so rather than leave the user to wonder.
type InlineResponse struct {
	Query    string   `json:"query"`
	Expanded int      `json:"expanded"`
	Kept     []string `json:"kept,omitempty"`
	Error    string   `json:"error,omitempty"`
}

// Inline replaces every call to a definition the query makes with a copy of
// that definition's body, so a pipeline can be read without jumping back to
// the top, and then lays the result out the way Format would. A body is only
// moved to a call site where every name it reads still means the same thing,
// and a body that binds a name its arguments use is renamed apart first.
//
// A definition that calls itself cannot be unfolded into a finite query, so it
// stays a definition and Kept says so, as does one whose expansion would make
// the query too large to work in.
func Inline(src string) InlineResponse {
	if strings.TrimSpace(src) == "" {
		return InlineResponse{Query: src}
	}
	query, err := gojq.Parse(src)
	if err != nil {
		return InlineResponse{Query: src, Error: err.Error()}
	}
	result := jqinline.Inline(query)
	return InlineResponse{
		Query:    jqfmt.Format(result.Query),
		Expanded: result.Expanded,
		Kept:     result.Kept,
	}
}
