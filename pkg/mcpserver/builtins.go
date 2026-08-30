package mcpserver

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/itchyny/gojq"
)

// A builtin is one of jq's own functions: the half of pwrq's vocabulary that
// list_functions used to leave out.
//
// pwrq is jq plus the cmdlets, and the catalogue documents only the cmdlets.
// Over MCP that made the tool lie. `list_functions` filtered by "ascii_upcase"
// answered "no functions match, and nothing is named close to it" about a
// function that runs; filtered by "split" it listed three cmdlets and never
// mentioned jq's own split/1. A model that asked the catalogue what pwrq could
// do was told, with no hedging, that half of it was missing.
//
// The same gap ran through the error path. `"x" | from_json` failed with "no
// cmdlet is named from_json; list_functions has the vocabulary" - pointing the
// caller at the catalogue that does not contain fromjson, which is the answer
// and is one character away.
type builtin struct {
	Name    string `json:"name" jsonschema:"the builtin's name, as it is called in a query"`
	MinArgs int    `json:"minArgs" jsonschema:"fewest arguments the builtin accepts"`
	MaxArgs int    `json:"maxArgs" jsonschema:"most arguments the builtin accepts"`
	// Gloss is a short description, for the builtins a caller is most likely
	// to be hunting for. Empty for the rest: the name is the documentation for
	// `sqrt`, and inventing a sentence for all hundred-odd would be a table
	// with nothing keeping it true.
	Gloss string `json:"gloss,omitempty" jsonschema:"one line on what the builtin does, for the builtins worth describing"`
}

// Arity renders how many arguments the builtin takes, in the same /0 or /1-2
// notation the cmdlet catalogue uses.
func (b builtin) Arity() string {
	if b.MinArgs == b.MaxArgs {
		return fmt.Sprintf("/%d", b.MinArgs)
	}
	return fmt.Sprintf("/%d-%d", b.MinArgs, b.MaxArgs)
}

// glosses describes the builtins worth describing: the ones a caller reaches
// for by meaning rather than by name, where knowing the name exists is not yet
// knowing it is the answer.
//
// Every entry must name a real builtin, and TestEveryGlossIsABuiltin enforces
// it against gojq itself rather than against anyone's memory.
var glosses = map[string]string{
	"ascii_downcase": "lowercase a string",
	"ascii_upcase":   "uppercase a string",
	"add":            "sum an array, or concatenate its strings or arrays",
	"any":            "whether any element is true",
	"all":            "whether every element is true",
	"capture":        "the named groups of a regex match, as an object",
	"contains":       "whether one value is contained in another",
	"endswith":       "whether a string ends with a suffix",
	"explode":        "a string as an array of codepoints",
	"flatten":        "collapse nested arrays into one",
	"from_entries":   "an array of {key, value} back into an object",
	"fromdate":       "an ISO 8601 string to a Unix timestamp",
	"fromjson":       "parse a JSON string into a value",
	"getpath":        "the value at a path given as an array",
	"gsub":           "replace every regex match",
	"group_by":       "bucket an array by a key expression",
	"implode":        "an array of codepoints back into a string",
	"index":          "where a substring or element first occurs",
	"indices":        "every position a substring or element occurs at",
	"join":           "an array into a string with a separator",
	"keys":           "an object's keys, sorted, or an array's indices",
	"limit":          "the first n outputs of an expression",
	"ltrimstr":       "a prefix removed if it is there",
	"map":            "apply an expression to every element",
	"map_values":     "apply an expression to every value, dropping empties",
	"match":          "regex matches as objects, with offsets and captures",
	"max_by":         "the element with the largest key expression",
	"min_by":         "the element with the smallest key expression",
	"now":            "the current Unix timestamp",
	"paths":          "every path in a value, as arrays",
	"range":          "a stream of numbers",
	"rtrimstr":       "a suffix removed if it is there",
	"scan":           "every regex match as a string or capture array",
	"select":         "keep the input only when an expression is true",
	"split":          "a string into an array on a separator or regex",
	"splits":         "a string into a stream on a separator or regex",
	"startswith":     "whether a string starts with a prefix",
	"strftime":       "a timestamp formatted by a strftime layout",
	"strptime":       "a date string parsed by a strftime layout",
	"sub":            "replace the first regex match",
	"test":           "whether a string matches a regex",
	"to_entries":     "an object as an array of {key, value}",
	"todate":         "a Unix timestamp as an ISO 8601 string",
	"tojson":         "a value as its JSON text",
	"tonumber":       "a string as a number",
	"tostring":       "a value as a string",
	"trim":           "whitespace removed from both ends",
	"unique_by":      "duplicates removed by a key expression",
	"walk":           "apply an expression to every value, bottom up",
	"with_entries":   "map over an object as {key, value} pairs",
}

var (
	builtinsOnce sync.Once
	builtinList  []builtin
)

// jqBuiltins returns the jq functions the cmdlet catalogue does not document.
//
// gojq answers this itself: its `builtins` function reports every callable
// name and arity. Compiled with no cmdlets registered it reports jq's own, and
// subtracting the catalogue leaves exactly what list_functions was missing.
// Nothing here is written down twice, so nothing here can drift: a gojq upgrade
// that adds or removes a builtin changes this list on the next run.
//
// Subtracting the catalogue rather than listing jq's set outright is what
// handles the names that are both. pwrq registers cmdlets over some of jq's
// names, and for those the catalogue's entry is the one that is true; only the
// names it says nothing about need saying here.
func jqBuiltins() []builtin {
	builtinsOnce.Do(func() {
		documented := make(map[string]bool)
		for _, c := range catalogue() {
			documented[c.Name] = true
		}

		arities := map[string][]int{}
		for _, entry := range rawBuiltins() {
			name, arity, ok := splitArity(entry)
			if !ok || documented[name] {
				continue
			}
			// Internal plumbing, never written by hand.
			if strings.HasPrefix(name, "_") {
				continue
			}
			arities[name] = append(arities[name], arity)
		}

		builtinList = make([]builtin, 0, len(arities))
		for name, counts := range arities {
			sort.Ints(counts)
			builtinList = append(builtinList, builtin{
				Name:    name,
				MinArgs: counts[0],
				MaxArgs: counts[len(counts)-1],
				Gloss:   glosses[name],
			})
		}
		sort.Slice(builtinList, func(i, j int) bool {
			return builtinList[i].Name < builtinList[j].Name
		})
	})
	return builtinList
}

// rawBuiltins asks gojq for its vocabulary, as "name/arity" strings.
//
// Compiled with no options, so the answer is jq's own set rather than jq's set
// plus whatever pwrq happens to have registered into this process.
func rawBuiltins() []string {
	query, err := gojq.Parse("builtins")
	if err != nil {
		return nil
	}
	code, err := gojq.Compile(query)
	if err != nil {
		return nil
	}
	v, ok := code.Run(nil).Next()
	if !ok {
		return nil
	}
	entries, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// splitArity reads gojq's "name/arity" notation.
func splitArity(entry string) (string, int, bool) {
	slash := strings.LastIndex(entry, "/")
	if slash < 1 {
		return "", 0, false
	}
	arity, err := strconv.Atoi(entry[slash+1:])
	if err != nil {
		return "", 0, false
	}
	return entry[:slash], arity, true
}

// findBuiltins returns the jq builtins matching a search term, by name or by
// what their gloss says they do.
func findBuiltins(needle string) []builtin {
	var out []builtin
	for _, b := range jqBuiltins() {
		if contains(b.Name, needle) || (b.Gloss != "" && contains(b.Gloss, needle)) {
			out = append(out, b)
		}
	}
	return out
}

// builtinNames lists the jq builtins by name, for the suggester.
func builtinNames() []string {
	all := jqBuiltins()
	out := make([]string, len(all))
	for i, b := range all {
		out[i] = b.Name
	}
	return out
}

// renderBuiltins prints the jq half of a search's answer, kept separate from
// the cmdlets and labelled, because "pwrq registered this" and "jq has always
// had this" are different claims and only one of them is documented in the
// catalogue the caller is reading.
func renderBuiltins(found []builtin, afterCmdlets bool) string {
	var sb strings.Builder
	if afterCmdlets {
		fmt.Fprintf(&sb, "%s in jq's own vocabulary, which the catalogue above does not cover:\n", jqCount(len(found)))
	} else {
		fmt.Fprintf(&sb, "%s in jq's own vocabulary, which pwrq is a superset of:\n", jqCount(len(found)))
	}
	shown := found
	if len(shown) > builtinsUpTo {
		shown = shown[:builtinsUpTo]
	}
	for _, b := range shown {
		if b.Gloss == "" {
			fmt.Fprintf(&sb, "%s%s [jq]\n", b.Name, b.Arity())
			continue
		}
		fmt.Fprintf(&sb, "%s%s [jq] %s\n", b.Name, b.Arity(), b.Gloss)
	}
	if len(found) > len(shown) {
		fmt.Fprintf(&sb, "and %d more - narrow the term to see them\n", len(found)-len(shown))
	}
	return sb.String()
}

// builtinsUpTo caps how much of jq's vocabulary one search may print. A single
// letter matches a hundred names, and a hundred names appended to a cmdlet
// listing buries the answer the caller asked for under the one they did not.
const builtinsUpTo = 20

func jqCount(n int) string {
	if n == 1 {
		return "1 function"
	}
	return fmt.Sprintf("%d functions", n)
}
