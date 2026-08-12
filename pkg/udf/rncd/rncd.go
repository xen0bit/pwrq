// Package rncd scores how similar byte strings are — by their bytes, by their
// information density, and by which ranges they literally share.
//
// Everything here works on values, not on paths. Anything that casts to bytes
// is a valid input: a string from stdin, a decoded blob, a captured response
// body, or a file read into the pipeline with cat. That is deliberate. The
// measurements are properties of the bytes, so making the filesystem a
// precondition would have narrowed the cmdlets to one of the places bytes come
// from — and it is why these cmdlets are offered in the browser IDE too.
//
// Two different questions, and so two cmdlets:
//
//   - rncd_compare scores every pair in a corpus, so a collection of samples
//     can be sorted by "what looks like what".
//   - shared_chunks decomposes one value against another, so the score for a
//     single interesting pair can be explained as concrete byte ranges.
//
// # The hybrid score
//
// rncd_compare blends four distances, every one normalized to [0, 1] with
// lower meaning more similar:
//
//	hybrid = alpha*ncd + beta*ncd_fingerprint + gamma*(entropy_global +
//	         entropy_profile),   gamma = (1 - alpha - beta) / 2
//
// ncd is the Normalized Compression Distance: if two values share structure,
// compressing them together costs less than compressing them apart. That is
// the workhorse, but on its own it is blind to *class*. Two unrelated
// encrypted blobs are both incompressible, so they compress together as badly
// as an encrypted blob and a text file, and NCD calls all three pairs equally
// unrelated. The other three terms recover the class: each value's per-block
// entropy is packed into a fingerprint and NCD'd, its global entropy is
// compared, and its entropy profile is compared point by point. Two encrypted
// values have the same flat ~8 bits/byte shape and score close on all three.
//
// # Deviations from the reference implementation
//
// This is a re-implementation of github.com/xen0bit/sim, and it differs from
// it in two places where sim is wrong. Both are pinned by tests.
//
// Ncd is measured by concatenation, C(ab), not with a trained zstd dictionary.
// sim documents the dictionary form but has never run it: it calls
// zstd.BuildDict with Contents but no History, and BuildDict builds entropy
// tables around a caller-supplied history rather than mining one out of the
// samples, so it fails with "dictionary of size 0 < 8" on every call and every
// pair silently falls back to concatenation. Implementing the dictionary form
// properly is possible — WithEncoderDictRaw gives a real C(b|a) — but it is a
// worse distance. sim's min(C(b|a), C(a|b)) / max(C(a), C(b)) collapses on
// size-mismatched pairs, scoring a 4KB text file against a 130KB binary at
// 0.001; the max form that makes it the actual Normalized Information Distance
// rates *identical* inputs at 0.5, because C(a|a) still pays frame overhead.
// Concatenation NCD has neither problem and needs one shared encoder rather
// than one per sample.
//
// The entropy fingerprint quantizes each block to one byte instead of writing
// it as a little-endian float32. Three of a float32's four bytes are mantissa
// noise that differs between any two inputs — exactly the incompressible
// detail the fingerprint exists to strip — and the term ends up inverted:
// measured on two random blobs and one document, float32 scores blob-vs-blob
// 0.831 against blob-vs-document 0.792, rating same-class inputs as *less*
// alike than different-class ones. Quantized, the same corpus scores 0.379
// against 0.437, in the right order.
package rncd

import "github.com/itchyny/gojq"

// RegisterAll returns every similarity cmdlet in this package.
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterCompare(),
		RegisterSharedChunks(),
	}
}
