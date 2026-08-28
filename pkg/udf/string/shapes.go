package string

import "github.com/xen0bit/pwrq/pkg/core/shape"

var (
	// LineDiffShape is the difference between two blocks of text, by line.
	LineDiffShape = shape.Plain(
		shape.Prop("added", shape.Array, "lines in the second text that are not in the first"),
		shape.Prop("removed", shape.Array, "lines in the first text that are not in the second"),
	)

	// CharFrequencies counts characters, so its keys are the text's own
	// characters and cannot be listed.
	CharFrequencies = shape.Derived("one key per distinct character in the input, holding how many times it occurred")
)
