package rncd

import "github.com/xen0bit/pwrq/pkg/core/shape"

var (
	// SharedChunksShape is the decomposition of one byte string against
	// another. Coverage is the headline number; Chunks is the working.
	SharedChunksShape = shape.Fixed("Pwrq.SharedChunks",
		shape.Prop("Coverage", shape.Number, "fraction of the target the reference explains, 0 to 1"),
		shape.Prop("MatchedBytes", shape.Number, "how many bytes of the target were found in the reference"),
		shape.Prop("Spans", shape.Number, "how many separate matched runs there were"),
		shape.Prop("TargetLength", shape.Number, "length of the target in bytes"),
		shape.Prop("ReferenceLength", shape.Number, "length of the reference in bytes"),
		shape.Prop("MinMatch", shape.Number, "shortest run counted as a match, as the call configured it"),
		shape.Prop("Chunks", shape.Array, "the target split into runs, each a Pwrq.SharedChunk"),
	)

	// SharedChunkShape is one run within that decomposition. It is declared
	// even though no cmdlet returns it directly, because it is what Chunks
	// holds and a caller reading the catalogue is about to index into it.
	SharedChunkShape = shape.Fixed("Pwrq.SharedChunk",
		shape.Prop("Matched", shape.Boolean, "whether this run was found in the reference"),
		shape.Prop("Start", shape.Number, "offset of the run in the target, in bytes"),
		shape.Prop("End", shape.Number, "offset one past the run's last byte"),
		shape.Prop("Length", shape.Number, "length of the run in bytes"),
		shape.Prop("RefOffset", shape.Any, "where the run was found in the reference, or null when it was not matched"),
	)
)
