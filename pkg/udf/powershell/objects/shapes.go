package objects

import "github.com/xen0bit/pwrq/pkg/core/shape"

// MeasureInfoShape is a measurement over a collection.
//
// Only Count is unconditional. The four statistics are present only when the
// call asked for them, so a caller that writes `.Sum` after a bare
// measure_object gets null - which is precisely the kind of thing a declared
// shape exists to warn about. Without a property they measure the values
// themselves, as `1,2,3 | Measure-Object -Sum` does.
var MeasureInfoShape = shape.Fixed("Pwrq.Measurement",
	shape.Prop("Count", shape.Number, "how many items were measured"),
	shape.OptProp("Sum", shape.Number, "total; only with the sum option"),
	shape.OptProp("Average", shape.Number, "mean; only with the average option"),
	shape.OptProp("Minimum", shape.Any, "smallest value; only with the minimum option"),
	shape.OptProp("Maximum", shape.Any, "largest value; only with the maximum option"),
)

// GroupInfoShape is one bucket from group_object.
//
// Name is a *rendering* of the grouping value rather than the value itself: a
// scalar prints as itself, and anything else as JSON, so grouping by a
// property that holds a list gives "[\"a\",\"b\"]". Group holds the rows,
// whose keys are the input's.
var GroupInfoShape = shape.Fixed("Pwrq.Group",
	shape.Prop("Name", shape.String, "the grouping value, rendered as a string"),
	shape.Prop("Count", shape.Number, "how many rows fell into this bucket"),
	shape.OptProp("Group", shape.Array, "the rows themselves, unchanged; absent when only the counts were asked for"),
).Note("the ashashtable option returns something else entirely: a single " +
	"Pwrq.GroupTable whose keys are the grouping values")

// SelectedProperties is select_object's output. The keys are the properties the
// caller asked for, so no fixed field list can describe it - only the rule -
// and the result is one object per input, or an array when several came in.
var SelectedProperties = shape.Derived("the selected properties, in the order given")
