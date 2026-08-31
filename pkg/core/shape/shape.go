// Package shape describes what a cmdlet emits.
//
// A caller — increasingly a language model writing a query through the MCP
// server — has to know the shape of a cmdlet's output to write the next stage
// of the pipeline. pwrq already answers half of that question: the registration
// wrappers record whether a cmdlet streams, so get_help can say whether the
// caller must collect with [...]. This package answers the other half, which
// keys an object comes back with.
//
// # Why the type name alone is not enough
//
// pwrq already put a type name on the wire, and it looked like it did this job.
// It did not. It was applied at construction through three unrelated idioms — a
// map literal, a field assignment, a constructor — with no registry, so the type
// space could not be enumerated. Running every documented example showed what
// that cost: of the 43 cmdlets that emit an object, 34 carried no type at all,
// get_process and get_service among them. A catalogue built on the name alone
// would have been silently partial exactly where a caller most needs it.
//
// So the name is kept and demoted. It travels as PwrqType, where format_table
// reads it and a projection carries it, but it stops being the type system and
// becomes a foreign key into the catalogue this package holds. Names it can
// hold are pwrq's own — Pwrq.FileSystem.File, Pwrq.Process — because a caller
// resolving one against this catalogue is asking what pwrq emits, and a name
// borrowed from .NET promised a class that was never behind it.
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

	"github.com/xen0bit/pwrq/pkg/core/typed"
)

// JSONType is the JSON type of a property's value. It is deliberately the JSON
// vocabulary rather than Go's: it is what the caller sees.
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
	// name is the PwrqType a Fixed shape stamps, and is empty otherwise.
	name string
	// rule explains a Derived or Dynamic shape's keys in one line.
	rule string
	// note records a condition under which the cmdlet emits something other
	// than this shape at all.
	note string
	// inArray reports that the cmdlet returns an array of this shape rather
	// than one of it.
	inArray bool
	props   []Property
	// declared indexes props by name, for reconcile.
	//
	// It is built with the shape rather than in reconcile, because reconcile
	// runs on every object a cmdlet emits and a search emits one per match: a
	// map rebuilt there was the whole cost of Build. A shape's properties are
	// fixed at construction, and the two methods that return a modified shape
	// - Each and Named - change neither the properties nor what they are
	// called, so the index a copy inherits is still its own.
	declared map[string]Property
}

// declare builds a shape's name index. Every constructor ends in it.
func declare(s *Shape) *Shape {
	s.declared = make(map[string]Property, len(s.props))
	for _, p := range s.props {
		s.declared[p.Name] = p
	}
	return s
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

// Each marks a shape as describing one *element* of an array the cmdlet
// returns, rather than the value it returns.
//
// Streaming already distinguishes a stream of values from one value, and this
// is the third case it does not cover: read_archive and get_variable each
// return a single value that happens to be an array of objects. A caller told
// only "object [Pwrq.Archive.Entry]" would write `.Name`
// against an array and get null.
//
// It returns a copy, because a shape is a package-level variable shared by
// every cmdlet that emits it, and one of them returning a list is not a fact
// about the others.
func (s *Shape) Each() *Shape {
	copied := *s
	copied.inArray = true
	return &copied
}

// Named gives a Derived or Dynamic shape a type name.
//
// The kind says what decides the keys; the name says what the value is. Those
// are independent, and a SQL result row is the case that proves it: every row
// is a Pwrq.Sqlite.Row, and which keys it has is the SELECT's business. A
// caller still benefits from the name - it tells them these are rows - even
// though looking it up will not produce a property list.
func (s *Shape) Named(typeName string) *Shape {
	s.name = typeName
	return s
}

// Fixed declares a cmdlet that emits an object with a known set of keys, under
// a type name. The name is what Build stamps as PwrqType, so it is also the key
// a caller looks the shape up by.
//
// Because it is that key, two shapes must not share a name. A name that meant
// two different property sets would make the catalogue unusable for the thing
// it exists to do, which is to let a caller look a type up and learn what is
// in it.
func Fixed(typeName string, props ...Property) *Shape {
	return declare(&Shape{kind: KindFixed, name: typeName, props: props})
}

// Plain declares a cmdlet that emits a known set of keys under no type name.
//
// Much of pwrq's own vocabulary returns plain lowercase JSON - summary is
// {count, min, max, mean, median, stdev}, semver_parts is {major, minor,
// patch} - and stamping those with a PwrqType to make them describable would
// add a key to results that are deliberately clean. The shape is what the
// caller needs; the type name is only how a named object is looked up, and
// these are not that.
func Plain(props ...Property) *Shape {
	return declare(&Shape{kind: KindFixed, props: props})
}

// Derived declares a cmdlet whose output keys come from its input. The rule
// says how, in the one line get_help and the MCP catalogue will print:
// "one key per distinct value, holding its count".
//
// Properties are still accepted, for a shape that is partly its own and partly
// its input's: a decoded JWT is always {header, payload, signature}, but what
// is inside payload is the token's business.
func Derived(rule string, props ...Property) *Shape {
	return declare(&Shape{kind: KindDerived, rule: rule, props: props})
}

// Dynamic declares a cmdlet whose output keys come from somewhere neither the
// cmdlet nor the input controls — the columns of a SQL query, the headers of a
// CSV file, an HTTP response's header set.
func Dynamic(rule string, props ...Property) *Shape {
	return declare(&Shape{kind: KindDynamic, rule: rule, props: props})
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

// TypeName is the PwrqType a Fixed shape stamps, or "" for the other kinds.
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
	// The declaration wins over whatever is already there. A object built
	// from a scalar infers a type from that scalar - typed.New("a/path")
	// calls itself a System.String - so deferring to what is present would
	// stamp the value's type instead of the cmdlet's.
	if s.name != "" {
		m[typed.TypeKey] = s.name
	}
	s.reconcile(m)
	return m
}

// reconcile records the differences between what was built and what was
// declared. Derived and Dynamic shapes are only checked for their declared
// properties: the rest of their keys are the point.
func (s *Shape) reconcile(m map[string]any) {
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
		if k == typed.TypeKey {
			continue
		}
		if _, ok := s.declared[k]; !ok {
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
		return fmt.Sprintf("%s with %s", s.named("object"), countProps(len(s.props)))
	case KindDerived:
		return s.named("object") + ", keys from the input: " + s.rule
	case KindDynamic:
		return s.named("object") + ", keys from the data: " + s.rule
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

// named prefixes a description with the type name, when there is one.
func (s *Shape) named(noun string) string {
	if s.inArray {
		noun = "array of " + noun
	}
	if s.name == "" {
		return noun
	}
	return fmt.Sprintf("%s [%s]", noun, s.name)
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
