package collection

import "github.com/xen0bit/pwrq/pkg/core/shape"

// These reshape an object the caller supplied, so their keys are the caller's
// keys. What varies between them is the relationship between the two sets, and
// that is what each rule states.
var (
	// MergedObject keeps both inputs' keys.
	MergedObject = shape.Derived("the keys of both inputs, merged recursively, with the second argument winning a conflict")

	// RenamedObject keeps every key, with some renamed.
	RenamedObject = shape.Derived("the input's keys, with those named in the mapping replaced by their new names")

	// PrunedObject keeps a subset of the input's keys.
	PrunedObject = shape.Derived("the input's keys, minus those whose value was empty at any depth")

	// FlattenedObject replaces nesting with dotted paths.
	FlattenedObject = shape.Derived("one key per leaf of the input, named by its dot-and-bracket path")

	// UnflattenedObject is the inverse, so the keys are path prefixes.
	UnflattenedObject = shape.Derived("the input's dotted keys expanded back into nested objects")

	// ComparisonShape is one difference between two collections.
	ComparisonShape = shape.Fixed("Microsoft.PowerShell.Commands.PSCompareObject",
		shape.Prop("InputObject", shape.Any, "the value that differed"),
		shape.Prop("SideIndicator", shape.String, "<= when only the reference had it, => when only the difference did, == when both"),
	)

	// MatchedRow is one of the input's own rows, returned unchanged.
	MatchedRow = shape.Derived("the matching input row, unchanged, so its keys are the row's keys")
)
