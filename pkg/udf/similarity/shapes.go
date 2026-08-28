package similarity

import "github.com/xen0bit/pwrq/pkg/core/shape"

// DeepDiffShape is the difference between two documents.
//
// All three properties are always present, empty arrays included. That is what
// lets a caller write `.changed | length` without guarding, and it is worth
// declaring precisely because the alternative - omitting an empty list - is the
// more common convention and the one a caller will otherwise assume.
var DeepDiffShape = shape.Plain(
	shape.Prop("added", shape.Array, "entries present in the second document only, as {path, after}"),
	shape.Prop("removed", shape.Array, "entries present in the first document only, as {path, before}"),
	shape.Prop("changed", shape.Array, "entries present in both with different values, as {path, before, after}"),
)
