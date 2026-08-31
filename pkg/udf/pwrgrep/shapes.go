package pwrgrep

import "github.com/xen0bit/pwrq/pkg/core/shape"

// Finding is one thing a rule found, at one place.
//
// It is what `finding` and `report` in rules/pwrgrep.jq build, which is where
// the keys come from - not from this cmdlet, which passes a rule's values
// through untouched. A rule of your own may emit whatever it likes; every rule
// that ships emits this, and this is what the catalogue promises a caller who
// pipes one into the next stage.
var Finding = shape.Fixed("Pwrq.PwrgrepFinding",
	shape.Prop("RuleId", shape.String, "the rule that reported this"),
	shape.Prop("Path", shape.String, "file the finding is in"),
	shape.Prop("LineNumber", shape.Number, "one-based line the finding starts on"),
	shape.Prop("Column", shape.Number, "one-based column the finding starts at"),
	shape.Prop("EndLineNumber", shape.Number, "one-based line the finding ends on"),
	shape.Prop("Message", shape.String, "what the rule has to say about what it found"),
	shape.Prop("Match", shape.String, "the source text the rule matched, spanning every line it covers"),
).Note("A finding is a value like any other, so filtering vendored code is the next stage of the pipeline rather than an option on the search: map(select(.Path | test(\"node_modules\") | not))")

// Rule is one rule in the catalogue.
//
// Query is published rather than summarised because a rule is a query and
// nothing else: reading it is how you decide whether it asks what you wanted,
// and editing a copy of it is how you write the rule nobody shipped. Origin
// says whether the copy you are looking at is one you can edit - a rule from a
// directory can be changed where it sits, and one built into the binary has to
// be copied out first.
var Rule = shape.Fixed("Pwrq.PwrgrepRule",
	shape.Prop("Id", shape.String, "the finding id this rule is usually called by"),
	shape.Prop("Ids", shape.Array, "every finding id the rule reports under; a rule file often holds several, so that rules about the same files share one walk of the tree"),
	shape.Prop("Path", shape.String, "where the rule sits in the catalogue, without the extension"),
	shape.Prop("From", shape.String, "the rule file it was translated from, or empty for one written here"),
	shape.Prop("Fixture", shape.String, "a file the rule is checked against, or empty"),
	shape.Prop("Description", shape.String, "the prose in the rule's header: what it does not cover, and why a pattern is written the way it is"),
	shape.Prop("Query", shape.String, "the rule itself, as a pwrq query"),
	shape.Prop("Origin", shape.String, "the directory the rule was read from, or \"<built in>\" for the copy inside the binary"),
	shape.Prop("PwrqValue", shape.String, "the id, as the bindable value"),
).Note("A rule is a file: copy one into $PWRQ_RULES or ~/.config/pwrq/rules, change it, and yours is the one that runs")
