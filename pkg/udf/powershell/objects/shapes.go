package objects

import "github.com/xen0bit/pwrq/pkg/core/shape"

// MeasureInfoShape is a measurement over a collection.
//
// Only Count is unconditional. The four statistics are present only when the
// call asked for them *and* named a property to measure, so a caller that
// writes `.Sum` after a bare measure_object gets null - which is precisely the
// kind of thing a declared shape exists to warn about.
var MeasureInfoShape = shape.Fixed("Microsoft.PowerShell.Commands.GenericMeasureInfo",
	shape.Prop("Count", shape.Number, "how many items were measured"),
	shape.OptProp("Sum", shape.Number, "total; only with a Property and the Sum option"),
	shape.OptProp("Average", shape.Number, "mean; only with a Property and the Average option"),
	shape.OptProp("Minimum", shape.Any, "smallest value; only with a Property and the Minimum option"),
	shape.OptProp("Maximum", shape.Any, "largest value; only with a Property and the Maximum option"),
)
