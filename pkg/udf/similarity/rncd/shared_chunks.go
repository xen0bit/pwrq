package rncd

import (
	"fmt"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/typed"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// SharedChunksOptions are shared_chunks' tuning knobs.
type SharedChunksOptions struct {
	MinMatch int `param:"MinMatch"`
}

// defaultMinMatch is the shortest run reported as shared. Any two values of
// any kind contain the same four bytes somewhere, so a short minimum fills the
// output with coincidences; 16 bytes is long enough that a match is evidence.
const defaultMinMatch = 16

// RegisterSharedChunks registers shared_chunks, which decomposes one byte
// string into the spans it shares with another.
//
// The input is the *target* — the value being explained — and the operand is
// the reference it is explained against, which is what makes the piped form
// read the way it is usually wanted: a stream of candidates measured against
// one fixed reference.
//
//	find("samples"; "file") | cat | shared_chunks($known)
//	shared_chunks(read_bytes("suspect.bin"); read_bytes("known.bin"); {MinMatch: 32})
//
// Where rncd_compare estimates, this counts. Coverage is the exact fraction of
// the target the reference accounts for, and every span it reports can be cut
// out of both values and compared byte for byte.
func RegisterSharedChunks() gojq.CompilerOption {
	common.DeclareInput("shared_chunks", common.InputPipeline)
	return common.WithFunctionOf("shared_chunks", 1, 3, SharedChunksShape, func(v any, args []any) any {
		in, rest := common.SplitInput(v, args, 1)
		result, err := sharedChunks(in, rest)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		return result
	})
}

func sharedChunks(in any, rest []any) (any, error) {
	targetData, ok := bindBytes(in)
	if !ok {
		return nil, fmt.Errorf("shared_chunks: the target does not cast to bytes, it is %s", describe(in))
	}
	refData, ok := bindBytes(rest[0])
	if !ok {
		return nil, fmt.Errorf("shared_chunks: the reference does not cast to bytes, it is %s", describe(rest[0]))
	}
	opts := SharedChunksOptions{MinMatch: defaultMinMatch}
	if len(rest) > 1 {
		if err := bindOptions("shared_chunks", rest[1], &opts); err != nil {
			return nil, err
		}
	}
	if opts.MinMatch < 1 {
		return nil, fmt.Errorf("shared_chunks: MinMatch must be at least 1, got %d", opts.MinMatch)
	}

	chunks := SharedChunks(refData, targetData, opts.MinMatch)
	matched, total, spans := Coverage(chunks)

	rendered := make([]any, len(chunks))
	for i, c := range chunks {
		chunk := typed.New(nil)
		chunk.AddNoteProperty("Matched", c.Matched)
		chunk.AddNoteProperty("Start", c.Start)
		chunk.AddNoteProperty("End", c.End)
		chunk.AddNoteProperty("Length", c.Length)
		// A literal run was found nowhere, so it has no offset to report. null
		// says that; -1 would be a number the caller has to know to exclude.
		if c.Matched {
			chunk.AddNoteProperty("RefOffset", c.RefOffset)
		} else {
			chunk.AddNoteProperty("RefOffset", nil)
		}
		rendered[i] = SharedChunkShape.Build(chunk.ToMap())
	}

	coverage := 0.0
	if total > 0 {
		coverage = float64(matched) / float64(total)
	}

	obj := typed.New(nil)
	obj.AddNoteProperty("MinMatch", opts.MinMatch)
	obj.AddNoteProperty("TargetLength", len(targetData))
	obj.AddNoteProperty("ReferenceLength", len(refData))
	obj.AddNoteProperty("MatchedBytes", matched)
	obj.AddNoteProperty("Spans", spans)
	// Coverage is the headline: the fraction of the target that the reference
	// explains. It is a similarity signal in its own right, and unlike the
	// compression distances it is exact.
	obj.AddNoteProperty("Coverage", round(coverage, 6))
	obj.AddNoteProperty("Chunks", rendered)
	return SharedChunksShape.Build(obj.ToMap()), nil
}
