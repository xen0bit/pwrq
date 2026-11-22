package ideengine

import (
	_ "embed" // the recipe library
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

// Recipes are whole queries for common tasks (recipes.pwrq). When a comment
// says what a query should do and nothing is written yet, the editor offers
// the recipe the decision model reads as doing it, with its slots - the
// directory, the pattern, the thing a picture should show - filled from the
// comment's own words. A recipe also continues a query that starts like it.

//go:embed recipes.pwrq
var recipesText string

// slotsJSON is each kind's question, for the slots that do not ask their
// own: internal/tools/d1cal chooses them, phrasing by phrasing, by how often
// each finds the value a labelled comment states.
//
//go:embed slots.json
var slotsJSON []byte

// KindQuestions are the kinds' questions.
func KindQuestions() map[string]string {
	out := map[string]string{}
	if err := json.Unmarshal(slotsJSON, &out); err != nil {
		panic(fmt.Sprintf("ideengine: slots.json: %v", err))
	}
	return out
}

// Recipe is one task's query.
type Recipe struct {
	ID   string `json:"id"`
	Task string `json:"task"`
	// Also are more words a comment might use for it: they help find it,
	// and the model is not shown them.
	Also string `json:"also,omitempty"`
	// Source is the catalog example an example recipe was made from.
	Source string `json:"source,omitempty"`
	Query  string `json:"query"` // with {{slot}} placeholders
	Slots  []Slot `json:"slots,omitempty"`
}

// Slot is a value a recipe takes from the comment.
type Slot struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Question string `json:"question"`
	Default  string `json:"default"`
	// Optional is an input fed in as the first stage: with no value from
	// the comment the stage is dropped and the query reads the editor's
	// input instead of the example's.
	Optional bool `json:"optional,omitempty"`
}

var (
	recipesOnce sync.Once
	recipes     []Recipe
	recipesErr  error
)

// Recipes is the library, parsed once.
func Recipes() ([]Recipe, error) {
	recipesOnce.Do(func() { recipes, recipesErr = ParseRecipesWith(recipesText, KindQuestions()) })
	return recipes, recipesErr
}

var (
	slotLineRe = regexp.MustCompile(`^# slot ([a-z_]+) ([a-z]+)(?:: (.+?))? = (.*)$`)
	slotRefRe  = regexp.MustCompile(`\{\{([a-z_][a-z0-9_]*)\}\}`)
)

// slotKinds are the kinds a slot may have, each read from a comment its own
// way (normalizeSlot).
var slotKinds = map[string]bool{"path": true, "file": true, "text": true, "phrase": true, "pattern": true, "number": true,
	"size": true, "host": true, "url": true, "ext": true, "field": true, "ip": true, "days": true,
	"input": true, "value": true, "cidr": true}

// ParseRecipes reads recipes.pwrq's format, with the calibrated kind
// questions.
func ParseRecipes(src string) ([]Recipe, error) { return ParseRecipesWith(src, KindQuestions()) }

// ParseRecipesWith reads recipes with the given kind questions: what
// calibration tries.
func ParseRecipesWith(src string, kinds map[string]string) ([]Recipe, error) {
	var out []Recipe
	var cur *Recipe
	var body []string
	seen := map[string]bool{}
	finish := func() error {
		if cur == nil {
			return nil
		}
		cur.Query = strings.TrimSpace(strings.Join(body, "\n"))
		if cur.Task == "" || cur.Query == "" {
			return fmt.Errorf("recipe %s: needs a task and a query", cur.ID)
		}
		declared := map[string]bool{}
		for _, s := range cur.Slots {
			declared[s.Name] = true
		}
		used := map[string]bool{}
		for _, m := range slotRefRe.FindAllStringSubmatch(cur.Query, -1) {
			if !declared[m[1]] {
				return fmt.Errorf("recipe %s: slot {{%s}} is not declared", cur.ID, m[1])
			}
			used[m[1]] = true
		}
		for name := range declared {
			if !used[name] {
				return fmt.Errorf("recipe %s: slot %s is declared and never used", cur.ID, name)
			}
		}
		out = append(out, *cur)
		cur, body = nil, nil
		return nil
	}
	for i, line := range strings.Split(src, "\n") {
		switch {
		case strings.HasPrefix(line, "## "):
			if err := finish(); err != nil {
				return nil, err
			}
			id := strings.TrimSpace(line[3:])
			if seen[id] {
				return nil, fmt.Errorf("line %d: recipe %s is defined twice", i+1, id)
			}
			seen[id] = true
			cur = &Recipe{ID: id}
		case cur == nil:
			// the file's preamble
		case strings.HasPrefix(line, "# task: "):
			cur.Task = strings.TrimSpace(line[len("# task: "):])
		case strings.HasPrefix(line, "# also: "):
			cur.Also = strings.TrimSpace(line[len("# also: "):])
		case strings.HasPrefix(line, "# slot "):
			m := slotLineRe.FindStringSubmatch(line)
			if m == nil || !slotKinds[m[2]] {
				return nil, fmt.Errorf("line %d: a slot is `# slot <name> <kind>[: <question>] = <default>`, with a known kind", i+1)
			}
			q := m[3]
			if q == "" {
				q = kinds[m[2]]
			}
			if q == "" {
				return nil, fmt.Errorf("line %d: a %s slot must ask its own question", i+1, m[2])
			}
			cur.Slots = append(cur.Slots, Slot{Name: m[1], Kind: m[2], Question: q, Default: m[4]})
		case strings.HasPrefix(line, "#"):
			return nil, fmt.Errorf("line %d: recipe %s: unknown header %q", i+1, cur.ID, line)
		default:
			body = append(body, line)
		}
	}
	if err := finish(); err != nil {
		return nil, err
	}
	return out, nil
}

// Fill writes values into a query's slots: the defaults where a value is
// missing or cannot be read as its kind. A slot inside a string literal is
// escaped for the string; any other is written as code.
func Fill(query string, slots []Slot, values map[string]string) string {
	byName := map[string]Slot{}
	for _, s := range slots {
		byName[s.Name] = s
		if _, ok := normalizeSlot(s.Kind, values[s.Name]); !ok && s.Optional {
			query = dropStage(query, "{{"+s.Name+"}}")
		}
	}
	var b strings.Builder
	inStr := false
	for i := 0; i < len(query); i++ {
		c := query[i]
		if strings.HasPrefix(query[i:], "{{") {
			if m := slotRefRe.FindStringSubmatchIndex(query[i:]); m != nil && m[0] == 0 {
				name := query[i+m[2] : i+m[3]]
				s := byName[name]
				v, ok := normalizeSlot(s.Kind, values[name])
				if !ok {
					v, _ = normalizeSlot(s.Kind, s.Default)
				}
				b.WriteString(writeSlot(s.Kind, v, inStr))
				i += m[1] - 1
				continue
			}
		}
		switch {
		case inStr && c == '\\' && i+1 < len(query):
			b.WriteByte(c)
			i++
			b.WriteByte(query[i])
			continue
		case c == '"':
			inStr = !inStr
		}
		b.WriteByte(c)
	}
	return b.String()
}

func writeSlot(kind, v string, inStr bool) string {
	if inStr {
		q, _ := json.Marshal(v)
		return string(q[1 : len(q)-1])
	}
	if kind == "field" {
		return fieldInsert(v)
	}
	return v
}

var (
	intRe     = regexp.MustCompile(`\d+(\.\d+)?`)
	sizeRe    = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*([kmgt]i?b?|bytes?|b)?\b`)
	fieldRe   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	spaceRe   = regexp.MustCompile(`\s+`)
	pathNoise = regexp.MustCompile(`(?i)^(the|in|inside|under|within|from|of|my)\s+|\s+(folder|directory|dir)$`)
)

// extensions maps what a comment calls a kind of file to its extension.
var extensions = map[string]string{
	"python": "py", "golang": "go", "go": "go", "javascript": "js", "typescript": "ts", "rust": "rs",
	"markdown": "md", "text": "txt", "java": "java", "ruby": "rb", "shell": "sh", "bash": "sh",
	"yaml": "yaml", "json": "json", "csv": "csv", "html": "html", "css": "css", "c": "c", "c++": "cpp",
	"cpp": "cpp", "c#": "cs", "csharp": "cs", "php": "php", "kotlin": "kt", "swift": "swift", "log": "log",
	"pdf": "pdf", "word": "docx", "excel": "xlsx", "sql": "sql", "xml": "xml", "toml": "toml",
	"jpeg": "jpg", "jpg": "jpg", "png": "png", "gif": "gif", "zip": "zip", "lua": "lua", "perl": "pl",
}

// NormalizeSlot reads a value of a kind out of a comment's words, as Fill
// does: calibration's view.
func NormalizeSlot(kind, raw string) (string, bool) { return normalizeSlot(kind, raw) }

// normalizeSlot reads a value of a kind out of the words a comment used,
// reporting false when they are not one: "10MB" is 10485760 bytes, "the src
// folder" is src, "Python files" are py.
func normalizeSlot(kind, raw string) (string, bool) {
	v := strings.TrimSpace(raw)
	v = strings.Trim(v, "\"'`")
	if t := strings.TrimRight(v, ".,;:!?"); t != "" {
		v = t // the sentence's full stop, not the value's
	}
	if v == "" {
		return "", false
	}
	switch kind {
	case "number", "days":
		m := intRe.FindString(v)
		if m == "" {
			if n, ok := numberWords[strings.ToLower(v)]; ok {
				return strconv.Itoa(n), true
			}
			return "", false
		}
		if strings.Contains(m, ".") {
			m = m[:strings.IndexByte(m, '.')]
		}
		return m, true
	case "size":
		m := sizeRe.FindStringSubmatch(v)
		if m == nil {
			return "", false
		}
		n, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			return "", false
		}
		mult := 1.0
		switch u := strings.ToLower(m[2]); {
		case u == "" && n < 10000:
			mult = 1 << 20 // a bare "10" means megabytes, the unit a size is talked about in
		case strings.HasPrefix(u, "k"):
			mult = 1 << 10
		case strings.HasPrefix(u, "m"):
			mult = 1 << 20
		case strings.HasPrefix(u, "g"):
			mult = 1 << 30
		case strings.HasPrefix(u, "t"):
			mult = 1 << 40
		}
		return strconv.FormatInt(int64(n*mult), 10), true
	case "path":
		low := strings.ToLower(v)
		switch low {
		case "current directory", "current folder", "this directory", "this folder", "here", "current dir",
			"the current directory", "the current folder", "cwd", "working directory", "the working directory":
			return ".", true
		case "home", "home directory", "my home directory", "home folder":
			return "~", true
		}
		for {
			s := pathNoise.ReplaceAllString(v, "")
			if s == v {
				break
			}
			v = strings.TrimSpace(s)
		}
		if v == "" || strings.ContainsAny(v, " \t") && !strings.ContainsAny(v, "/\\") {
			return "", false
		}
		return v, true
	case "file":
		v = pathNoise.ReplaceAllString(v, "")
		if strings.ContainsAny(v, " \t") || !strings.ContainsAny(v, "./\\") {
			return "", false
		}
		return v, true
	case "host":
		if i := strings.Index(v, "://"); i >= 0 {
			v = v[i+3:]
		}
		if i := strings.IndexAny(v, "/?#"); i >= 0 {
			v = v[:i]
		}
		if strings.ContainsAny(v, " \t") || !strings.Contains(v, ".") && v != "localhost" {
			return "", false
		}
		return strings.ToLower(v), true
	case "url":
		if strings.ContainsAny(v, " \t") || !strings.Contains(v, ".") {
			return "", false
		}
		if !strings.Contains(v, "://") {
			v = "https://" + v
		}
		return v, true
	case "ip":
		if net.ParseIP(v) == nil {
			return "", false
		}
		return v, true
	case "ext":
		low := strings.ToLower(v)
		low = strings.TrimSuffix(strings.TrimSuffix(low, " files"), " file")
		low = strings.TrimPrefix(strings.TrimPrefix(low, "*"), ".")
		if e, ok := extensions[low]; ok {
			return e, true
		}
		if fieldRe.MatchString(low) && len(low) <= 5 {
			return low, true
		}
		return "", false
	case "field":
		v = strings.TrimPrefix(v, ".")
		// "status is failed", "per category", "by the owner": the name is
		// the word the comment's own phrase hangs on.
		low := " " + strings.ToLower(v) + " "
		for _, sep := range []string{" is ", " equals ", " equal to ", " = ", " == ", " above ", " over ", " greater than ", " of "} {
			if i := strings.Index(low, sep); i > 0 {
				v = strings.TrimSpace(v[:i-1+1])
				low = " " + strings.ToLower(v) + " "
			}
		}
		for {
			s := regexp.MustCompile(`(?i)^(per|by|the|each|every|their|its)\s+`).ReplaceAllString(v, "")
			if s == v {
				break
			}
			v = s
		}
		if strings.ContainsAny(v, " \t") {
			// "last name" is the field last_name more often than not
			v = spaceRe.ReplaceAllString(v, "_")
		}
		if !fieldRe.MatchString(v) {
			return "", false
		}
		return v, true
	case "value":
		if m := regexp.MustCompile(`-?\d+(\.\d+)?`).FindString(v); m != "" {
			return m, true
		}
		return "", false
	case "cidr":
		if !cidrRe.MatchString(v) {
			return "", false
		}
		return v, true
	case "pattern", "text", "phrase", "input":
		return v, true
	}
	return v, true
}

var numberWords = map[string]int{"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7,
	"eight": 8, "nine": 9, "ten": 10, "twenty": 20, "fifty": 50, "hundred": 100, "a dozen": 12, "dozen": 12}

// Stages splits a query at its top-level pipes: the parts a recipe and a
// query are compared by.
func Stages(code string) []string {
	var out []string
	depth := 0
	inStr := false
	start := 0
	rs := []rune(code)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case inStr && c == '\\':
			i++
		case c == '"':
			inStr = !inStr
		case inStr:
		case strings.ContainsRune("([{", c):
			depth++
		case strings.ContainsRune(")]}", c):
			depth--
		case c == '|' && depth == 0 && (i+1 >= len(rs) || rs[i+1] != '='):
			out = append(out, strings.TrimSpace(string(rs[start:i])))
			start = i + 1
		}
	}
	out = append(out, strings.TrimSpace(string(rs[start:])))
	return out
}

// stageHead is what a stage starts with, which is what two stages must share
// to be the same step: its leading brackets and its first name or token.
func stageHead(stage string) string {
	s := strings.TrimSpace(stage)
	i := 0
	for i < len(s) && strings.ContainsRune("[({", rune(s[i])) {
		i++
	}
	j := i
	for j < len(s) && (s[j] == '_' || s[j] == '$' || s[j] == '.' && j == i || unicode.IsLetter(rune(s[j])) || unicode.IsDigit(rune(s[j]))) {
		j++
	}
	if j == i && j < len(s) {
		j++
	}
	return s[:j]
}

// dropStage removes the stage that is nothing but the given slot (quoted or
// not), and the pipe after it.
func dropStage(query, ref string) string {
	stages := Stages(query)
	for i, st := range stages {
		if st == ref || st == `"`+ref+`"` {
			stages = append(stages[:i], stages[i+1:]...)
			sep := " | "
			if strings.Contains(query, "\n| ") {
				sep = "\n| "
			}
			return strings.Join(stages, sep)
		}
	}
	return query
}
