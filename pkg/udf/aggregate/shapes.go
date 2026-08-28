package aggregate

import "github.com/xen0bit/pwrq/pkg/core/shape"

// Every cmdlet here emits an object whose keys came out of the caller's data,
// so none of them can be given a property list. Declaring one would be worse
// than declaring nothing: a caller reading `{dept: string}` off the catalogue
// would write a query against a key that only exists when the rows happen to
// have been grouped by a column called dept.
//
// What they can state exactly is the rule that produced the keys, which is the
// thing a caller actually needs in order to write the next stage.
var (
	// GroupedRows is one bucket per distinct value, holding the rows.
	GroupedRows = shape.Derived("one key per distinct value of the grouping property, holding an array of the matching rows")

	// CountedRows is one bucket per distinct value, holding how many.
	CountedRows = shape.Derived("one key per distinct value of the grouping property, holding the number of matching rows")

	// AggregatedRows is one bucket per distinct value, holding a number.
	AggregatedRows = shape.Derived("one key per distinct value of the grouping property, holding the aggregate of the named column")

	// IndexedRows is one bucket per distinct value, holding a whole row, so
	// the nested object's keys are the input rows' keys.
	IndexedRows = shape.Derived("one key per distinct value of the indexing property, holding the first matching row unchanged")

	// PivotTable is nested twice: rows, then columns.
	PivotTable = shape.Derived("one key per distinct row value, each holding an object with one key per distinct column value")

	// ValueCounts is a frequency table over whole values.
	ValueCounts = shape.Derived("one key per distinct value in the input array, holding how many times it occurred")
)
