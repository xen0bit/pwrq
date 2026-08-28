package objects

import "github.com/xen0bit/pwrq/pkg/core/shape"

// MeasureInfoShape is a measurement over a collection.
//
// Only Count is unconditional. The four statistics are present only when the
// call asked for them *and* named a property to measure, so a caller that
// writes `.Sum` after a bare measure_object gets null - which is precisely the
// kind of thing a declared shape exists to warn about.
var MeasureInfoShape = shape.Fixed("Pwrq.Measurement",
	shape.Prop("Count", shape.Number, "how many items were measured"),
	shape.OptProp("Sum", shape.Number, "total; only with a Property and the Sum option"),
	shape.OptProp("Average", shape.Number, "mean; only with a Property and the Average option"),
	shape.OptProp("Minimum", shape.Any, "smallest value; only with a Property and the Minimum option"),
	shape.OptProp("Maximum", shape.Any, "largest value; only with a Property and the Maximum option"),
)

// GroupInfoShape is one bucket from group_object.
//
// Name is a *rendering* of the grouping value rather than the value itself, so
// a caller grouping by an object gets a string like "map[a:1]". Group holds the
// rows, whose keys are the input's.
var GroupInfoShape = shape.Fixed("Pwrq.Group",
	shape.Prop("Name", shape.String, "the grouping value, rendered as a string"),
	shape.Prop("Count", shape.Number, "how many rows fell into this bucket"),
	shape.OptProp("Group", shape.Array, "the rows themselves, unchanged; absent when only the counts were asked for"),
).Note("the ashashtable option returns something else entirely: a single " +
	"Pwrq.GroupTable whose keys are the grouping values")
