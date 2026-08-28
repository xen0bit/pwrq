package config

import "github.com/xen0bit/pwrq/pkg/core/shape"

// A parser's output keys are the document's keys, so none of these can name a
// property. The rule is still worth stating, because the three differ in
// exactly the way a caller trips over: one nests, two do not.
var (
	// ParsedINI nests one level: sections, then their settings.
	ParsedINI = shape.Derived("one key per section, each holding an object of that section's settings; settings before any section header sit at the top level")

	// ParsedProperties is flat, and every value is a string.
	ParsedProperties = shape.Derived("one key per property in the document, holding its value as a string")

	// ParsedLogfmt is flat, and values are typed.
	ParsedLogfmt = shape.Derived("one key per field on the line, holding a number or boolean where the value looks like one and a string otherwise")
)
