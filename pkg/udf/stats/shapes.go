package stats

import "github.com/xen0bit/pwrq/pkg/core/shape"

// SummaryShape is the six-number description of a numeric array.
//
// It is a Plain shape rather than a typed one: summary returns lowercase JSON
// the way jq's own vocabulary does, and stamping a PSTypeName on it to make it
// describable would add a key to a result that is deliberately clean.
var SummaryShape = shape.Plain(
	shape.Prop("count", shape.Number, "how many values were summarised"),
	shape.Prop("min", shape.Number, "smallest value"),
	shape.Prop("max", shape.Number, "largest value"),
	shape.Prop("mean", shape.Number, "arithmetic mean"),
	shape.Prop("median", shape.Number, "middle value, or the mean of the middle two"),
	shape.Prop("stdev", shape.Number, "population standard deviation"),
)
