package ideengine

import (
	"fmt"
	"net"
	"regexp"
	"strings"
	"sync"

	"github.com/xen0bit/pwrq/pkg/udf/discovery"
)

// Example recipes. Every published example in the catalog is a recipe too:
// its cmdlet's description is its task, its name and category are its
// keywords, and its literal values become slots. So every cmdlet can be
// written from a comment the day it is documented, and the calibration that
// scores the hand-written recipes scores these with it - each example has
// comments of its own in internal/tools/d1cal/studies/scripts/intents.jsonl,
// and a test there fails for an example that has none.
//
// What becomes a slot, and of which kind, is read off the literal:
//
//   - a value fed in as the first stage - "hello" | sha256, 1048576 |
//     human_bytes - is the input slot: the comment's value if it gives one,
//     and otherwise the stage is dropped, so the query works on the editor's
//     input. A constant array or object fed in is always dropped.
//   - a URL, an IP address, a host name, a file name, a path, a CIDR range,
//     each its own kind with its own calibrated question.
//   - a number written as an argument: a count.
//
// Anything else - a pattern, a format, an option - stays as written: a
// generic "what word?" question reads the wrong word out of a comment more
// often than the right one.

// AllRecipes are the hand-written recipes and then the examples'.
func AllRecipes() []Recipe {
	hand, _ := Recipes()
	return append(append([]Recipe(nil), hand...), ExampleRecipes()...)
}

// ExampleRecipeID names the recipe made from a cmdlet's i-th example
// (counting from 1).
func ExampleRecipeID(cmdlet string, i int) string { return fmt.Sprintf("%s#%d", cmdlet, i) }

var (
	exampleMu      sync.Mutex
	exampleRecipes []Recipe
)

// ExampleRecipes are the catalog's examples as recipes, in catalog order.
func ExampleRecipes() []Recipe {
	exampleMu.Lock()
	defer exampleMu.Unlock()
	if exampleRecipes != nil {
		return exampleRecipes
	}
	// Made once the catalog is published (a registry is built), and not
	// before: an empty catalog is not remembered as no examples.
	kinds := KindQuestions()
	var out []Recipe
	for _, c := range discovery.Catalog() {
		if !c.Available {
			continue
		}
		for i, ex := range c.Examples {
			if r, ok := exampleRecipe(c, i+1, ex, kinds); ok {
				out = append(out, r)
			}
		}
	}
	if len(out) > 0 {
		exampleRecipes = out
	}
	return out
}

// exampleRecipe makes one example a recipe. An example that reads a
// variable ($source, $a) is not one a comment could ask for whole: skipped.
func exampleRecipe(c discovery.Command, i int, ex string, kinds map[string]string) (Recipe, bool) {
	if code, _ := splitStrings(ex); strings.Contains(code, "$") {
		return Recipe{}, false
	}
	r := Recipe{
		ID:     ExampleRecipeID(c.Name, i),
		Source: ex,
		Task:   c.Description,
		Also:   strings.Join(append([]string{strings.ReplaceAll(c.Name, "_", " "), c.Category}, c.Aliases...), " "),
	}
	query := strings.TrimSpace(ex)
	stages := Stages(query)
	if len(stages) > 1 && isConstantInput(stages[0]) {
		if lit, ok := stringLiteral(stages[0]); ok {
			kind := classifyLiteral(lit)
			// A file fed to a cmdlet that reads files - cat, read_bytes,
			// sha256(true) - is the file to read, and stays. Any other
			// value is the input, the editor's when the comment gives none.
			reads := kind == "file" && (strings.Contains(stages[1], "(true)") || c.Category == "File Operations" || c.Category == "Archives")
			if kind == "" {
				kind = "input"
			}
			r.Slots = append(r.Slots, Slot{Name: "in", Kind: kind, Question: kinds[kind], Default: lit, Optional: !reads})
			stages[0] = `"{{in}}"`
		} else if numberRe.MatchString(stages[0]) {
			r.Slots = append(r.Slots, Slot{Name: "in", Kind: "value", Question: kinds["value"], Default: stages[0], Optional: true})
			stages[0] = "{{in}}"
		} else {
			stages = stages[1:] // a constant array or object: the editor's input stands in
		}
		query = strings.Join(stages, " | ")
	}
	query, slots := slotLiterals(query, kinds, len(r.Slots))
	r.Slots = append(r.Slots, slots...)
	r.Query = query
	return r, true
}

var (
	numberRe      = regexp.MustCompile(`^-?\d+(\.\d+)?$`)
	identRunRe    = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)
	hostRe        = regexp.MustCompile(`^(?i)[a-z0-9-]+(\.[a-z0-9-]+)*\.[a-z]{2,}(:\d+)?$`)
	fileRe        = regexp.MustCompile(`^[^\s/]*[^\s]*\.[A-Za-z0-9]{1,5}$`)
	cidrRe        = regexp.MustCompile(`^[0-9a-fA-F:.]+/\d{1,3}$`)
	argNumberRe   = regexp.MustCompile(`([(;]\s*)(\d+)\s*[;)]`)
	fileExtension = map[string]bool{"txt": true, "log": true, "json": true, "jsonl": true, "csv": true, "zip": true, "gz": true,
		"tar": true, "bin": true, "db": true, "go": true, "py": true, "md": true, "pdf": true, "jpg": true, "png": true,
		"html": true, "xml": true, "yaml": true, "yml": true, "sqlite": true, "toml": true,
		"mp4": true, "mov": true, "m4v": true, "webm": true, "mkv": true, "avi": true}
)

// isConstantInput reports whether a stage is a literal value: strings,
// numbers, arrays and objects of them, with no call and no input.
func isConstantInput(stage string) bool {
	code, _ := splitStrings(stage)
	for _, loc := range identRunRe.FindAllStringIndex(code, -1) {
		word := code[loc[0]:loc[1]]
		if word == "true" || word == "false" || word == "null" || word == "S" {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(code[loc[1]:]), ":") {
			continue // an object key
		}
		return false
	}
	return !strings.ContainsAny(code, "|.$") || numberRe.MatchString(strings.TrimSpace(stage))
}

// splitStrings replaces each string literal with S, returning the literals.
func splitStrings(src string) (string, []string) {
	var b strings.Builder
	var lits []string
	for i := 0; i < len(src); i++ {
		if src[i] != '"' {
			b.WriteByte(src[i])
			continue
		}
		j := i + 1
		for j < len(src) && src[j] != '"' {
			if src[j] == '\\' {
				j++
			}
			j++
		}
		if j >= len(src) {
			j = len(src) - 1
		}
		lits = append(lits, src[i:j+1])
		b.WriteByte('S')
		i = j
	}
	return b.String(), lits
}

// stringLiteral is the text of a stage that is one plain string literal:
// no escapes, so it is the same text in a comment and in a query.
func stringLiteral(stage string) (string, bool) {
	s := strings.TrimSpace(stage)
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' || strings.ContainsAny(s[1:len(s)-1], `"\`) {
		return "", false
	}
	return s[1 : len(s)-1], true
}

// classifyLiteral is the slot kind a string literal is, or "" for none.
func classifyLiteral(s string) string {
	switch {
	case strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://"):
		return "url"
	case net.ParseIP(s) != nil:
		return "ip"
	case cidrRe.MatchString(s):
		return "cidr"
	case fileRe.MatchString(s) && fileExtension[strings.ToLower(s[strings.LastIndexByte(s, '.')+1:])]:
		return "file"
	case hostRe.MatchString(s):
		return "host"
	case s == "." || s == "~" || strings.HasPrefix(s, "/") || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "~/"):
		if strings.ContainsAny(s, " *?") {
			return ""
		}
		return "path"
	}
	return ""
}

// slotLiterals turns the classifiable string literals of a query, and the
// numbers it passes as arguments, into slots named s1, s2, ... - one slot a
// distinct value, so a key used twice is asked for once.
func slotLiterals(query string, kinds map[string]string, have int) (string, []Slot) {
	var slots []Slot
	byValue := map[string]string{}
	name := func(kind, value string) string {
		if n, ok := byValue[kind+"\x00"+value]; ok {
			return n
		}
		n := fmt.Sprintf("s%d", have+len(slots)+1)
		byValue[kind+"\x00"+value] = n
		slots = append(slots, Slot{Name: n, Kind: kind, Question: kinds[kind], Default: value})
		return n
	}
	var b strings.Builder
	for i := 0; i < len(query); i++ {
		if query[i] != '"' {
			b.WriteByte(query[i])
			continue
		}
		j := i + 1
		for j < len(query) && query[j] != '"' {
			if query[j] == '\\' {
				j++
			}
			j++
		}
		if j >= len(query) {
			b.WriteString(query[i:])
			break
		}
		lit := query[i : j+1]
		if text, ok := stringLiteral(lit); ok && !strings.Contains(text, "{{") {
			if kind := classifyLiteral(text); kind != "" {
				lit = `"{{` + name(kind, text) + `}}"`
			}
		}
		b.WriteString(lit)
		i = j
	}
	out := b.String()
	// Numbers passed as arguments - outside strings, so the strings are
	// masked while they are found.
	masked := maskStrings(out)
	{
		var nb strings.Builder
		last := 0
		// One at a time from just past each number, so "(0; 10)" finds both:
		// the separator after one is the opener of the next.
		for at := 0; at < len(masked); {
			m := argNumberRe.FindStringSubmatchIndex(masked[at:])
			if m == nil {
				break
			}
			start, end := at+m[4], at+m[5]
			nb.WriteString(out[last:start])
			nb.WriteString("{{" + name("number", out[start:end]) + "}}")
			last, at = end, end
		}
		nb.WriteString(out[last:])
		out = nb.String()
	}
	return out, slots
}

// maskStrings blanks out string literals byte for byte, so offsets in the
// result are offsets in src.
func maskStrings(src string) string {
	b := []byte(src)
	in := false
	for i := 0; i < len(b); i++ {
		switch {
		case in && b[i] == '\\' && i+1 < len(b):
			b[i], b[i+1] = 'S', 'S'
			i++
		case b[i] == '"':
			in = !in
			b[i] = 'S'
		case in:
			b[i] = 'S'
		}
	}
	return string(b)
}
