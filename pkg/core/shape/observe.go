package shape

import (
	"fmt"
	"sort"
	"strings"

	"github.com/xen0bit/pwrq/pkg/core/psobject"
)

// An Observer reports the shape of values a query actually produced.
//
// A declared catalogue can never be complete, and not because the declarations
// are unfinished: Derived and Dynamic shapes exist, so the keys of a great many
// results are simply not knowable until the query runs. Observation covers
// exactly that gap. It also cannot drift, because it describes the values in
// hand rather than a claim about them.
//
// It is fed the raw value before encoding, so it costs one pass over the top
// level of each result and never re-parses the rendered text.
type Observer struct {
	count int
	// kinds counts the JSON type of each result, so a stream that is not
	// homogeneous can say so instead of describing only its first value.
	kinds map[string]int
	// typeNames counts the PSTypeName carried by object results.
	typeNames map[string]int
	// props records, per key, how many results carried it and which JSON types
	// its values had.
	props map[string]*observedProp
	// overflowed reports that more distinct keys were seen than are tracked.
	// A Derived cmdlet over wild data can emit thousands, and an observer that
	// grew without bound would be a memory leak dressed as a diagnostic.
	overflowed bool
}

type observedProp struct {
	present int
	types   map[string]bool
}

// maxObservedProps caps how many distinct keys an Observer tracks. Beyond this
// the shape is better described as "many keys, from the data" than enumerated.
const maxObservedProps = 128

// NewObserver returns an Observer ready to be fed results.
func NewObserver() *Observer {
	return &Observer{
		kinds:     make(map[string]int),
		typeNames: make(map[string]int),
		props:     make(map[string]*observedProp),
	}
}

// Add records one result. Only the top level is inspected: a nested object's
// keys belong to a description of that object, not of this one, and walking the
// whole tree would make the observer cost scale with the data rather than with
// the number of results.
func (o *Observer) Add(v any) {
	if o == nil {
		return
	}
	o.count++
	o.kinds[jsonKind(v)]++

	m, ok := v.(map[string]any)
	if !ok {
		return
	}
	if name, ok := m[psobject.PSTypeNameKey].(string); ok {
		o.typeNames[name]++
	}
	for k, val := range m {
		if k == psobject.PSTypeNameKey {
			continue
		}
		p, seen := o.props[k]
		if !seen {
			if len(o.props) >= maxObservedProps {
				o.overflowed = true
				continue
			}
			p = &observedProp{types: make(map[string]bool)}
			o.props[k] = p
		}
		p.present++
		p.types[jsonKind(val)] = true
	}
}

// jsonKind names a value's JSON type. It is gojq's value space, so the cases
// are exhaustive; anything else is reported rather than silently called null.
func jsonKind(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case int, float64:
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return fmt.Sprintf("%T", v)
	}
}

// Notable reports whether the description is worth showing.
//
// A run that produced a handful of numbers has already shown the caller its
// shape: the values are right there, and a line saying "3 values, each a
// number" is noise. What cannot be read off the output is which keys an object
// carried when there are hundreds of them, and what the rest looked like when a
// limit cut the run short. Those are the two cases this returns true for.
func (o *Observer) Notable(truncated bool) bool {
	if o == nil || o.count == 0 {
		return false
	}
	return truncated || len(o.props) > 0 || o.overflowed
}

// Count is how many results were observed.
func (o *Observer) Count() int {
	if o == nil {
		return 0
	}
	return o.count
}

// Describe renders what was observed, in the form a caller can act on: how many
// results there were, what kind of value each was, and — when they were objects
// — which keys they carried and which types those held.
//
// A key that was not on every result is marked, because that is the difference
// between a field a caller can rely on and one it has to guard.
func (o *Observer) Describe() string {
	if o == nil || o.count == 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s, ", countResults(o.count))
	b.WriteString(o.describeKinds())

	if len(o.props) == 0 {
		if o.overflowed {
			b.WriteString("; many keys, from the data")
		}
		return b.String()
	}

	if names := o.describeTypeNames(); names != "" {
		b.WriteString(" ")
		b.WriteString(names)
	}

	b.WriteString("\nkeys: ")
	b.WriteString(o.describeProps())
	if o.overflowed {
		fmt.Fprintf(&b, "\n(more than %d distinct keys; the rest are not listed)", maxObservedProps)
	}
	return b.String()
}

// article picks a or an. The set of kinds is closed and small, so this is a
// lookup rather than a guess at English.
func article(kind string) string {
	switch kind {
	case "array", "object":
		return "an"
	default:
		return "a"
	}
}

func countResults(n int) string {
	if n == 1 {
		return "1 value"
	}
	return fmt.Sprintf("%d values", n)
}

// describeKinds says what the results were. A homogeneous stream is the common
// case and gets the short phrasing; a mixed one is named in full, since that is
// usually the surprise.
func (o *Observer) describeKinds() string {
	if len(o.kinds) == 1 {
		for k := range o.kinds {
			if o.count == 1 {
				return article(k) + " " + k
			}
			return "each " + article(k) + " " + k
		}
	}
	parts := make([]string, 0, len(o.kinds))
	for k, n := range o.kinds {
		parts = append(parts, fmt.Sprintf("%d %s", n, k))
	}
	sort.Strings(parts)
	return "mixed: " + strings.Join(parts, ", ")
}

// describeTypeNames names the PowerShell types the results carried, so a caller
// can look their declared shape up in the catalogue.
func (o *Observer) describeTypeNames() string {
	if len(o.typeNames) == 0 {
		return ""
	}
	names := make([]string, 0, len(o.typeNames))
	for n := range o.typeNames {
		names = append(names, n)
	}
	sort.Strings(names)
	return "[" + strings.Join(names, ", ") + "]"
}

// describeProps lists the observed keys in name order.
//
// Sorted rather than in the order they were seen, because there is no such
// order to preserve: Add walks a map, and Go randomises that, so anything else
// would differ from one run to the next and make this untestable.
func (o *Observer) describeProps() string {
	names := make([]string, 0, len(o.props))
	for name := range o.props {
		names = append(names, name)
	}
	sort.Strings(names)

	parts := make([]string, 0, len(names))
	for _, name := range names {
		p := o.props[name]
		types := make([]string, 0, len(p.types))
		for t := range p.types {
			types = append(types, t)
		}
		sort.Strings(types)

		entry := fmt.Sprintf("%s(%s)", name, strings.Join(types, "|"))
		if p.present < o.count {
			entry = fmt.Sprintf("%s(%s, on %d/%d)", name, strings.Join(types, "|"), p.present, o.count)
		}
		parts = append(parts, entry)
	}
	return strings.Join(parts, " ")
}
