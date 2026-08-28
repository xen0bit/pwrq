// Package shape describes what a cmdlet emits.
//
// A caller — increasingly a language model writing a query through the MCP
// server — has to know the shape of a cmdlet's output to write the next stage
// of the pipeline. pwrq already answers half of that question: the registration
// wrappers record whether a cmdlet streams, so get_help can say whether the
// caller must collect with [...]. This package answers the other half, which
// keys an object comes back with.
//
// # Why not PSTypeName
//
// PSTypeName looks like it already does this, and it does not. It is applied at
// construction through three unrelated idioms — a map literal, a field
// assignment, a constructor — with no registry, so the type space cannot be
// enumerated. Running every documented example shows what that costs: of the 43
// cmdlets that emit an object, 34 carry no type at all, get_process and
// get_service among them. A catalogue built on PSTypeName would be silently
// partial exactly where a model most needs it.
//
// So the name is kept and demoted. PSTypeName stays on the wire, where
// format_table reads it and a projection carries it, but it stops being the
// type system and becomes a foreign key into the catalogue this package holds.
//
// # The three kinds
//
// Not every object producer has a field list, and pretending otherwise would
// misdescribe a third of them:
//
//   - Fixed: the cmdlet decides the keys. get_process is always
//     {CPU, Handles, Id, Name, …}.
//   - Derived: the input decides them. flatten_keys returns {"a.b": …} because
//     the input had a nested a.b. What it can honestly declare is the rule.
//   - Dynamic: an external source decides them — SQLite columns, CSV headers.
//
// A catalogue that only understood Fixed would either omit the other two or
// state something untrue about them. Both are worse than saying nothing,
// because a caller trusts a catalogue.
package shape

import (
	"fmt"
	"sort"
	"strings"

	"github.com/xen0bit/pwrq/pkg/core/psobject"
)

// JSONType is the JSON type of a property's value. It is deliberately the JSON
// vocabulary rather than Go's or PowerShell's: it is what the caller sees.
type JSONType string

// The JSON types a property can hold. Any is for a property whose type depends
// on the data — a decoded JWT payload, a SQLite column — where naming one type
// would be a guess.
const (
	String  JSONType = "string"
	Number  JSONType = "number"
	Boolean JSONType = "boolean"
	Object  JSONType = "object"
	Array   JSONType = "array"
	Any     JSONType = "any"
)

// Kind distinguishes what decides a value's keys.
type Kind int

const (
	// KindUnspecified is a cmdlet that does not emit an object, or one whose
	// shape has not been declared. It renders as nothing rather than as a
	// guess.
	KindUnspecified Kind = iota
	// KindFixed is a cmdlet whose keys it chooses itself.
	KindFixed
	// KindDerived is a cmdlet whose keys come from its input.
	KindDerived
	// KindDynamic is a cmdlet whose keys come from an external source.
	KindDynamic
)

// Property is one key of an emitted object.
type Property struct {
	Name string
	Type JSONType
	Doc  string
	// Optional marks a property that is present only sometimes — a process
	// whose executable path could not be read, a file object built from a
	// value that was not a path.
	Optional bool
}

// Prop declares a property that is always present.
func Prop(name string, t JSONType, doc string) Property {
	return Property{Name: name, Type: t, Doc: doc}
}

// OptProp declares a property that is present only sometimes.
func OptProp(name string, t JSONType, doc string) Property {
	return Property{Name: name, Type: t, Doc: doc, Optional: true}
}

// Shape is what one cmdlet emits.
//
// The zero value is a valid unspecified shape, so a cmdlet registered without
// one behaves as though it said nothing rather than panicking.
type Shape struct {
	kind Kind
	// name is the PSTypeName a Fixed shape stamps, and is empty otherwise.
	name string
	// rule explains a Derived or Dynamic shape's keys in one line.
	rule string
	// note records a condition under which the cmdlet emits something other
	// than this shape at all.
	note  string
	props []Property
}

// Note records that the cmdlet sometimes emits something other than this shape,
// and under what condition: `get_date` returns an object, but a formatted
// string when it is given a Format option.
//
// A shape that stayed silent about that would be wrong for those calls, and a
// catalogue is trusted, so a caveat it cannot express is a caveat that becomes
// a lie. Note is the smallest thing that keeps the declaration honest without
// modelling union types for the two cmdlets that need one.
func (s *Shape) Note(note string) *Shape {
	s.note = note
	return s
}

// Fixed declares a cmdlet that emits an object with a known set of keys, under
// a PowerShell type name. The name is what Build stamps as PSTypeName, so it is
// also the key a caller looks the shape up by.
//
// Because it is that key, two shapes must not share a name. A name that meant
// two different property sets would make the catalogue unusable for the thing
// it exists to do, which is to let a caller look a type up and learn what is
// in it.
func Fixed(typeName string, props ...Property) *Shape {
	return &Shape{kind: KindFixed, name: typeName, props: props}
}

// Plain declares a cmdlet that emits a known set of keys under no type name.
//
// Much of pwrq's own vocabulary returns plain lowercase JSON - summary is
// {count, min, max, mean, median, stdev}, semver_parts is {major, minor,
// patch} - rather than PowerShell-style objects, and stamping those with a
// PSTypeName to make them describable would add a key to results that are
// deliberately clean. The shape is what the caller needs; the type name is
// only how a PowerShell-style object is looked up, and these are not that.
func Plain(props ...Property) *Shape {
	return &Shape{kind: KindFixed, props: props}
}

// Derived declares a cmdlet whose output keys come from its input. The rule
// says how, in the one line get_help and the MCP catalogue will print:
// "one key per distinct value, holding its count".
//
// Properties are still accepted, for a shape that is partly its own and partly
// its input's: a decoded JWT is always {header, payload, signature}, but what
// is inside payload is the token's business.
func Derived(rule string, props ...Property) *Shape {
	return &Shape{kind: KindDerived, rule: rule, props: props}
}

// Dynamic declares a cmdlet whose output keys come from somewhere neither the
// cmdlet nor the input controls — the columns of a SQL query, the headers of a
// CSV file, an HTTP response's header set.
func Dynamic(rule string, props ...Property) *Shape {
	return &Shape{kind: KindDynamic, rule: rule, props: props}
}

// Scalar is the shape of a cmdlet that does not emit an object. It is the same
// as declaring nothing, and exists so a registration can say so on purpose.
func Scalar() *Shape { return &Shape{} }

// Kind reports what decides this shape's keys.
func (s *Shape) Kind() Kind {
	if s == nil {
		return KindUnspecified
	}
	return s.kind
}

// TypeName is the PSTypeName a Fixed shape stamps, or "" for the other kinds.
func (s *Shape) TypeName() string {
	if s == nil {
		return ""
	}
	return s.name
}

// Rule explains a Derived or Dynamic shape's keys, and is empty for a Fixed one
// whose field list says it already.
func (s *Shape) Rule() string {
	if s == nil {
		return ""
	}
	return s.rule
}

// Properties are the declared keys, in declaration order.
func (s *Shape) Properties() []Property {
	if s == nil {
		return nil
	}
	return append([]Property(nil), s.props...)
}

// Specified reports whether this shape says anything at all.
func (s *Shape) Specified() bool {
	return s.Kind() != KindUnspecified
}

// Build stamps a map with this shape's type name and reconciles what was passed
// against what was declared.
//
// It is the constructor for a Fixed shape: routing every construction site
// through it is what stops the declaration and the code from drifting, because
// there is no longer a separate string literal to forget to update.
//
// A mismatch is never an error to the caller. A key that was not declared, or a
// declared key that is missing, is recorded as a discrepancy and the object is
// returned unchanged — a documentation bug must not break a user's query. A
// test asserts the discrepancy table is empty once the suite has exercised the
// cmdlets, so the reconciliation happens against real output rather than
// against a second hand-written list.
func (s *Shape) Build(m map[string]any) map[string]any {
	if s == nil || m == nil {
		return m
	}
	// The declaration wins over whatever is already there. A PSObject built
	// from a scalar infers a type from that scalar - NewPSObject("a/path")
	// calls itself a System.String - so deferring to what is present would
	// stamp the value's type instead of the cmdlet's.
	if s.name != "" {
		m[psobject.PSTypeNameKey] = s.name
	}
	s.reconcile(m)
	return m
}

// reconcile records the differences between what was built and what was
// declared. Derived and Dynamic shapes are only checked for their declared
// properties: the rest of their keys are the point.
func (s *Shape) reconcile(m map[string]any) {
	declared := make(map[string]Property, len(s.props))
	for _, p := range s.props {
		declared[p.Name] = p
	}

	for _, p := range s.props {
		if p.Optional {
			continue
		}
		if _, ok := m[p.Name]; !ok {
			record(Discrepancy{Shape: s.label(), Property: p.Name, Reason: ReasonMissing})
		}
	}

	if s.kind != KindFixed {
		return
	}
	for k := range m {
		if k == psobject.PSTypeNameKey {
			continue
		}
		if _, ok := declared[k]; !ok {
			record(Discrepancy{Shape: s.label(), Property: k, Reason: ReasonUndeclared})
		}
	}
}

// label names a shape in a discrepancy. A Fixed shape has a type name; the
// others are identified by their rule, which is the only thing that
// distinguishes them.
func (s *Shape) label() string {
	if s.name != "" {
		return s.name
	}
	if s.rule != "" {
		return s.rule
	}
	return "(unnamed shape)"
}

// Summary is the one-line description of a shape, for a catalogue listing that
// cannot afford a field table.
func (s *Shape) Summary() string {
	switch s.Kind() {
	case KindFixed:
		if s.name == "" {
			return fmt.Sprintf("object with %s", countProps(len(s.props)))
		}
		return fmt.Sprintf("object [%s] with %s", s.name, countProps(len(s.props)))
	case KindDerived:
		return "object, keys from the input: " + s.rule
	case KindDynamic:
		return "object, keys from the data: " + s.rule
	default:
		return ""
	}
}

// summaryWithNote is Summary plus the caveat, when there is one.
func (s *Shape) summaryWithNote() string {
	summary := s.Summary()
	if summary == "" || s.note == "" {
		return summary
	}
	return summary + " (" + s.note + ")"
}

func countProps(n int) string {
	if n == 1 {
		return "1 property"
	}
	return fmt.Sprintf("%d properties", n)
}

// Describe renders the shape over several lines: the summary, then one line per
// declared property. It is what get_help prints and what the MCP catalogue
// sends when it has room.
func (s *Shape) Describe() string {
	if !s.Specified() {
		return ""
	}
	var b strings.Builder
	b.WriteString(s.summaryWithNote())
	for _, p := range s.props {
		b.WriteString("\n    ")
		b.WriteString(p.Name)
		b.WriteString(" (")
		b.WriteString(string(p.Type))
		if p.Optional {
			b.WriteString(", optional")
		}
		b.WriteString(")")
		if p.Doc != "" {
			b.WriteString(" — ")
			b.WriteString(p.Doc)
		}
	}
	return b.String()
}

// Compact renders the shape on one line with its property names, for a listing
// that wants more than Summary but cannot spend a line per property.
func (s *Shape) Compact() string {
	if !s.Specified() {
		return ""
	}
	if len(s.props) == 0 {
		return s.summaryWithNote()
	}
	names := make([]string, len(s.props))
	for i, p := range s.props {
		names[i] = p.Name
		if p.Optional {
			names[i] += "?"
		}
	}
	return fmt.Sprintf("%s: %s", s.summaryWithNote(), strings.Join(names, " "))
}

// PropertyNames are the declared keys, sorted, for a caller that wants to
// compare them against an observed object.
func (s *Shape) PropertyNames() []string {
	names := make([]string, 0, len(s.Properties()))
	for _, p := range s.Properties() {
		names = append(names, p.Name)
	}
	sort.Strings(names)
	return names
}
