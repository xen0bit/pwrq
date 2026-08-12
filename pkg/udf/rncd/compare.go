package rncd

import (
	"fmt"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/psobject"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// CompareOptions are rncd_compare's tuning knobs.
type CompareOptions struct {
	Alpha    float64 `param:"Alpha"`
	Beta     float64 `param:"Beta"`
	MaxPairs int     `param:"MaxPairs"`
	Workers  int     `param:"Workers"`
}

func defaultCompareOptions() CompareOptions {
	return CompareOptions{
		// alpha weights raw-byte similarity and beta weights class similarity,
		// leaving a quarter of the score to the two entropy terms. Those are
		// sim's weights, and they encode a judgement worth keeping: shared
		// bytes are the strongest evidence, but not the only evidence.
		Alpha: 0.5,
		Beta:  0.25,

		// MaxPairs refuses a corpus whose pair count would not fit in memory
		// as objects. Pairs grow as N^2, so this is a much lower ceiling on N
		// than it looks: 100,000 pairs is about 450 values.
		MaxPairs: 100_000,

		// Workers sets both the pool size and how many compressions run at
		// once, and the compressor allocates match tables per concurrent
		// compression — so raising this buys throughput with memory.
		Workers: defaultWorkers(),
	}
}

// RegisterCompare registers rncd_compare, which scores every unordered pair in
// a corpus of byte strings and emits one object per pair.
//
// The input is an array of values, each of which casts to bytes: a string, or
// an object carrying its bytes under Content and optionally a label under
// Name. Nothing is read from disk — a corpus of files is assembled in the
// query, which is what lets the same cmdlet compare HTTP bodies, decoded
// blobs or literals.
//
//	[cat("a.bin"), cat("b.bin")] | rncd_compare
//	rncd_compare([$a, $b]; {Alpha: 0.7})
//	[find("samples"; "file") | {Name: ., Content: cat(.)}] | rncd_compare
func RegisterCompare() gojq.CompilerOption {
	return gojq.WithIterFunction("rncd_compare", 0, 2, func(v any, args []any) gojq.Iter {
		results, err := compare(common.SplitInput(v, args, 0))
		if err != nil {
			return gojq.NewIter(err)
		}
		return gojq.NewIter(results...)
	})
}

func compare(in any, rest []any) ([]any, error) {
	items, err := common.BindArray(in, "rncd_compare")
	if err != nil {
		return nil, err
	}
	opts := defaultCompareOptions()
	if len(rest) > 0 {
		if err := bindOptions("rncd_compare", rest[0], &opts); err != nil {
			return nil, err
		}
	}
	if opts.Alpha < 0 || opts.Beta < 0 || opts.Alpha+opts.Beta > 1 {
		return nil, fmt.Errorf("rncd_compare: Alpha and Beta must be non-negative and sum to at most 1, got %g and %g",
			opts.Alpha, opts.Beta)
	}
	if opts.Workers < 1 {
		return nil, fmt.Errorf("rncd_compare: Workers must be at least 1, got %d", opts.Workers)
	}
	if opts.MaxPairs > 0 && numPairs(len(items)) > opts.MaxPairs {
		return nil, fmt.Errorf("rncd_compare: %d values make %d pairs, above MaxPairs (%d); "+
			"narrow the input or raise {MaxPairs: n} — 0 lifts the limit",
			len(items), numPairs(len(items)), opts.MaxPairs)
	}

	samples, err := bindCorpus(items)
	if err != nil {
		return nil, err
	}
	pairs, err := analyze(samples, opts.Alpha, opts.Beta, opts.Workers)
	if err != nil {
		return nil, fmt.Errorf("rncd_compare: %w", err)
	}

	out := make([]any, len(pairs))
	for i, p := range pairs {
		out[i] = pairObject(samples[p.a], samples[p.b], p)
	}
	return out, nil
}

// bindCorpus casts every element to the bytes and label it will be scored under.
//
// An element that cannot be measured is an error rather than a skip. The array
// was assembled by the caller, one element at a time, so every element is an
// instruction; dropping one silently would leave them waiting for pairs that
// never arrive.
func bindCorpus(items []any) ([]*sample, error) {
	samples := make([]*sample, len(items))
	for i, item := range items {
		data, ok := bindBytes(item)
		if !ok {
			return nil, fmt.Errorf("rncd_compare: element %d does not cast to bytes, it is %s", i, describe(item))
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("rncd_compare: element %d is empty, so there is nothing to compare", i)
		}
		samples[i] = &sample{index: i, name: bindName(item), data: data}
	}
	return samples, nil
}

// pairObject renders one scored pair.
//
// The distances are rounded because they are estimates, not measurements: NCD
// varies with the compressor, so publishing the sixteenth decimal of a number
// whose third is approximate invites comparisons that mean nothing.
func pairObject(a, b *sample, p pairScore) map[string]any {
	obj := psobject.NewPSObjectWithTypeName(nil, "Pwrq.RncdPair")
	// The index is always there and the name only sometimes, so both are
	// reported: the index says which element of the caller's own array this
	// is, which is the one label no corpus can be missing.
	obj.AddNoteProperty("IndexA", a.index)
	obj.AddNoteProperty("IndexB", b.index)
	obj.AddNoteProperty("NameA", nameOrNull(a.name))
	obj.AddNoteProperty("NameB", nameOrNull(b.name))
	obj.AddNoteProperty("LengthA", len(a.data))
	obj.AddNoteProperty("LengthB", len(b.data))
	obj.AddNoteProperty("EntropyA", round(a.entropy, 3))
	obj.AddNoteProperty("EntropyB", round(b.entropy, 3))
	obj.AddNoteProperty("Ncd", round(p.ncd, 6))
	obj.AddNoteProperty("NcdFingerprint", round(p.ncdFingerprint, 6))
	obj.AddNoteProperty("EntropyGlobal", round(p.entropyGlobal, 6))
	obj.AddNoteProperty("EntropyProfile", round(p.entropyProfile, 6))
	obj.AddNoteProperty("Hybrid", round(p.hybrid, 6))
	return obj.ToMap()
}

// nameOrNull keeps the object's shape stable across corpora. An unnamed value
// reports null rather than "" so that `select(.NameA)` means what it looks
// like it means.
func nameOrNull(name string) any {
	if name == "" {
		return nil
	}
	return name
}
