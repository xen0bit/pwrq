package validate

import "github.com/xen0bit/pwrq/pkg/core/shape"

// SemverPartsShape is a semantic version split into its parts. prerelease and
// build are absent rather than empty when the version does not carry them,
// which is the distinction a caller has to guard.
var SemverPartsShape = shape.Plain(
	shape.Prop("major", shape.Number, "major version"),
	shape.Prop("minor", shape.Number, "minor version"),
	shape.Prop("patch", shape.Number, "patch version"),
	shape.OptProp("prerelease", shape.String, "text after the first hyphen; absent when there is none"),
	shape.OptProp("build", shape.String, "text after the plus sign; absent when there is none"),
)
