package rncd

import "github.com/xen0bit/pwrq/pkg/core/shape"

// RncdPairShape is one pair of samples compared against each other.
//
// The A/B suffixes are the point: every property comes in two, one per side,
// and a caller reading a distance without knowing which sample each figure
// belongs to cannot act on it.
var RncdPairShape = shape.Fixed("Pwrq.RncdPair",
	shape.Prop("IndexA", shape.Number, "position of the first sample in the caller's array"),
	shape.Prop("IndexB", shape.Number, "position of the second sample"),
	shape.Prop("NameA", shape.Any, "name of the first sample, or null when the corpus carried none"),
	shape.Prop("NameB", shape.Any, "name of the second sample, or null"),
	shape.Prop("LengthA", shape.Number, "length of the first sample in bytes"),
	shape.Prop("LengthB", shape.Number, "length of the second sample in bytes"),
	shape.Prop("EntropyA", shape.Number, "Shannon entropy of the first sample, rounded to 3 places"),
	shape.Prop("EntropyB", shape.Number, "Shannon entropy of the second sample"),
	shape.Prop("Ncd", shape.Number, "normalised compression distance, 0 is identical"),
	shape.Prop("NcdFingerprint", shape.Number, "the same distance over fingerprints rather than raw bytes"),
	shape.Prop("EntropyGlobal", shape.Number, "distance between the samples' overall entropies"),
	shape.Prop("EntropyProfile", shape.Number, "distance between their entropy profiles"),
	shape.Prop("Hybrid", shape.Number, "the combined score the other four feed into"),
)
