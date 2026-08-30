package mcpserver

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/queryrun"
	"github.com/xen0bit/pwrq/pkg/udf/discovery"
)

// notDefined matches the compile failure gojq raises for a call it cannot
// resolve. gojq reports the name and the arity it was called with and stops
// there, which is the whole problem: "C/0 is not defined" is true, and says
// nothing about the C/1 sitting three lines above it.
var notDefined = regexp.MustCompile(`function not defined: ([A-Za-z_][A-Za-z0-9_]*)/(\d+)`)

// explainCompileError adds to a "function not defined" failure the one fact
// that decides what the caller does next: whether the name exists at all.
//
// The failure this is written for cost a model, in one recorded session,
// roughly forty tool calls. It had written `def C(o): ...` and then called
// `$envelope | C`, which is C/0. gojq said "function not defined: C/0". The
// model tested its reading with `def C(o): ...; "hi" | C` - the same arity
// mistake a second time - got the same message, and concluded that the pwrq
// runtime rejects custom definitions outright. It then rewrote its entire
// query with every definition inlined, and reported the invented limitation to
// its user as a finding.
//
// Every step of that was reasonable given the message. The name was defined;
// only the arity was wrong; and nothing said so.
func explainCompileError(message, query string) string {
	match := notDefined.FindStringSubmatch(message)
	if match == nil {
		return message
	}
	name, arity := match[1], match[2]
	called, err := strconv.Atoi(arity)
	if err != nil {
		return message
	}

	if hint := arityHint(name, called, query); hint != "" {
		return message + " - " + hint
	}
	if hint := nameHint(name); hint != "" {
		return message + " - " + hint
	}
	return message
}

// arityHint explains a name that exists at some other arity, looking first at
// the definitions the caller wrote and then at the cmdlet catalogue.
func arityHint(name string, called int, query string) string {
	if defined := localArities(name, query); len(defined) > 0 {
		return fmt.Sprintf("you defined %s taking %s, but called it with %s: %s",
			name, arguments(defined), count(called), callForms(name, defined))
	}
	for _, c := range catalogue() {
		if c.Name != name && !slices.Contains(c.Aliases, name) {
			continue
		}
		if called >= c.MinArgs && called <= c.MaxArgs {
			// The catalogue says this arity is fine, so the failure is about
			// something else - a build without this cmdlet, most likely - and
			// guessing would only mislead.
			return ""
		}
		return fmt.Sprintf("%s takes %s, but was called with %s: %s",
			name, arityRange(c.MinArgs, c.MaxArgs), count(called), callForms(name, arities(c)))
	}
	// And then jq's own, on the same terms. `"a,b" | split` is a wrong arity,
	// not a wrong name, and without this it came back as "no cmdlet is named
	// split; did you mean split_path?" - which sends the caller to rewrite a
	// call that only needed its separator.
	for _, b := range jqBuiltins() {
		if b.Name != name {
			continue
		}
		if called >= b.MinArgs && called <= b.MaxArgs {
			return ""
		}
		return fmt.Sprintf("%s is a jq builtin taking %s, but was called with %s: %s",
			name, arityRange(b.MinArgs, b.MaxArgs), count(called),
			callForms(name, builtinArities(b)))
	}
	return ""
}

// builtinArities expands a builtin's arity range, as arities does for a
// catalogue entry.
func builtinArities(b builtin) []int {
	out := make([]int, 0, b.MaxArgs-b.MinArgs+1)
	for n := b.MinArgs; n <= b.MaxArgs; n++ {
		out = append(out, n)
	}
	return out
}

// nameHint covers the other half: a name nothing defines, where what the
// caller needs is the name they meant.
//
// It used to say "no cmdlet is named X", which was true and misleading in the
// same breath: pwrq is jq plus the cmdlets, so a name being no cmdlet says
// nothing about whether it exists. Both halves are searched now, and the
// wording no longer promises that the catalogue is the whole language.
func nameHint(name string) string {
	suggestions := suggest(catalogue(), strings.ToLower(name))
	if len(suggestions) == 0 {
		return fmt.Sprintf("nothing is named %s; list_functions has the vocabulary, cmdlets and jq's own", name)
	}
	// suggest also offers categories, which answer "where would this live"
	// rather than "what did I mean to type". Only the callable names help here.
	var callable []string
	for _, s := range suggestions {
		if !strings.HasPrefix(s, "category ") {
			callable = append(callable, s)
		}
	}
	if len(callable) == 0 {
		return fmt.Sprintf("nothing is named %s; list_functions has the vocabulary, cmdlets and jq's own", name)
	}
	return fmt.Sprintf("nothing is named %s; did you mean %s?", name, strings.Join(callable, ", "))
}

// localArities reports the arities the query itself defines a name at.
//
// The query is re-parsed rather than threaded down from the run, because this
// is the error path: it happens once, the program is already known to parse,
// and a parameter passed through four layers for the sake of a failure is a
// worse trade than parsing a string again.
func localArities(name, query string) []int {
	parsed, err := gojq.Parse(query)
	if err != nil {
		return nil
	}
	// Only the top-level definitions. A def nested inside a subexpression is
	// out of scope wherever the failing call sits, so reporting it would be
	// answering a question the caller did not ask.
	seen := map[int]bool{}
	for _, def := range parsed.FuncDefs {
		if def.Name == name {
			seen[len(def.Args)] = true
		}
	}

	out := make([]int, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

// arities expands a catalogue entry's arity range into the individual arities
// it accepts, so callForms can show what a working call looks like.
func arities(c discovery.Command) []int {
	out := make([]int, 0, c.MaxArgs-c.MinArgs+1)
	for n := c.MinArgs; n <= c.MaxArgs; n++ {
		out = append(out, n)
	}
	return out
}

// arguments renders a set of arities as English: "1 argument", "0 or 2
// arguments", "1 to 3 arguments".
func arguments(counts []int) string {
	switch len(counts) {
	case 0:
		return "no arguments"
	case 1:
		if counts[0] == 1 {
			return "1 argument"
		}
		return fmt.Sprintf("%d arguments", counts[0])
	}
	if counts[len(counts)-1]-counts[0] == len(counts)-1 {
		return arityRange(counts[0], counts[len(counts)-1])
	}
	rendered := make([]string, len(counts))
	for i, n := range counts {
		rendered[i] = strconv.Itoa(n)
	}
	return strings.Join(rendered[:len(rendered)-1], ", ") +
		" or " + rendered[len(rendered)-1] + " arguments"
}

// count renders how many arguments a call actually passed, reading naturally
// after "called with": "none", "1 argument", "3 arguments".
func count(n int) string {
	if n == 0 {
		return "none"
	}
	return arguments([]int{n})
}

func arityRange(low, high int) string {
	if low == high {
		return arguments([]int{low})
	}
	return fmt.Sprintf("%d to %d arguments", low, high)
}

// callForms shows how the name is actually written at each arity it accepts,
// because "takes 1 argument" still leaves a caller to work out that the
// argument goes in brackets rather than down the pipe.
func callForms(name string, counts []int) string {
	forms := make([]string, 0, len(counts))
	for _, n := range counts {
		if n == 0 {
			forms = append(forms, name)
			continue
		}
		placeholders := make([]string, n)
		for i := range placeholders {
			placeholders[i] = "..."
		}
		forms = append(forms, fmt.Sprintf("%s(%s)", name, strings.Join(placeholders, "; ")))
	}
	return "write " + strings.Join(forms, " or ")
}

// explainRunFailure enriches a run's failure message when there is more to say
// about it than the compiler said.
func explainRunFailure(res runQueryResult, query string) string {
	if res.Kind != queryrun.KindCompile {
		return res.Error
	}
	return explainCompileError(res.Error, query)
}
