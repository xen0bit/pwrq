package astsearch

import "github.com/xen0bit/pwrq/pkg/core/shape"

// AstMatch is one structural match.
//
// It is deliberately close to Pwrq.Match, which select_string emits, because
// the two cmdlets answer the same question about different things: where in
// this tree does X occur. A caller who has written one pipeline should be able
// to write the other by changing the pattern.
//
// Captures is the part that has no analogue in a regex search. A pattern names
// its holes - `func $NAME($$$ARGS) error` - and the match reports what each
// hole caught, so the next stage can group by, sort on or filter against a
// piece of syntax rather than re-parsing the matched text.
var AstMatch = shape.Fixed("Pwrq.AstMatch",
	shape.Prop("Path", shape.String, "file the match was found in"),
	shape.Prop("Language", shape.String, "the language the file was parsed as"),
	shape.Prop("LineNumber", shape.Number, "one-based line the match starts on"),
	shape.Prop("Column", shape.Number, "one-based column the match starts at"),
	shape.Prop("EndLineNumber", shape.Number, "one-based line the match ends on"),
	shape.Prop("Text", shape.String, "the source text the pattern matched, spanning every line it covers"),
	shape.Prop("Captures", shape.Object, "one key per metavariable in the pattern, holding the text it caught"),
	shape.Prop("PwrqValue", shape.String, "the path, as the bindable value"),
)

// PatternInfo is what a pattern compiled to.
//
// The S-expression is the useful part and the reason this cmdlet exists: a
// pattern that does not parse as code compiles anyway, into a query full of
// ERROR nodes that matches nothing. Being able to read what a pattern became
// is the difference between "this code does not occur" and "I wrote the
// pattern wrong".
var PatternInfo = shape.Fixed("Pwrq.AstPattern",
	shape.Prop("Pattern", shape.String, "the pattern as it was written"),
	shape.Prop("Language", shape.String, "the language it was compiled against"),
	shape.Prop("Valid", shape.Boolean, "whether it parses as code in that language; a pattern that does not will match nothing, silently"),
	shape.Prop("MetaVariables", shape.Array, "the names the pattern captures, without their dollar signs"),
	shape.Prop("Query", shape.String, "the tree-sitter S-expression the pattern compiled to"),
	shape.Prop("PwrqValue", shape.String, "the pattern, as the bindable value"),
)

// LanguageInfo is one language this build can parse.
//
// Which languages are present is a property of the binary rather than of the
// project, because the grammars are chosen at build time. So it is reported by
// reading the registry the parser actually consults, and never from a list
// written down beside it.
var LanguageInfo = shape.Fixed("Pwrq.AstLanguage",
	shape.Prop("Name", shape.String, "the name to pass as the Language option"),
	shape.Prop("Extensions", shape.Array, "the file extensions that select this language automatically"),
	shape.Prop("PwrqValue", shape.String, "the name, as the bindable value"),
)
