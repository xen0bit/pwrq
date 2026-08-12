package rncd

import (
	"fmt"
	"math"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/klauspost/compress/zstd"
)

const (
	// zstdLevel is the zstd level every compressed size is measured at. NCD is
	// a ratio of compressed sizes, so the level has to be the same everywhere
	// or the ratio is meaningless; it is a constant rather than an option for
	// that reason.
	//
	// Level 7 (SpeedBetterCompression) rather than the strongest setting is a
	// memory decision. An encoder pre-allocates its match tables, and at level
	// 10 that is ~37MB per concurrent compression against ~6MB at level 7 —
	// roughly a 6x memory cost for a difference in compressed size that moves
	// NCD in the third decimal.
	zstdLevel = 7

	// entropyBlockSize is the window the per-block entropy profile is measured
	// over. 256 bytes is small enough to show a header, a code section and a
	// packed resource as separate features, and large enough that the byte
	// histogram it is estimated from is not pure noise.
	entropyBlockSize = 256

	// minEntropyBlock drops a trailing block too short to estimate entropy
	// from. Eight samples over 256 possible values says nothing.
	minEntropyBlock = 8

	// maxEncoderWindow caps the compression window. The window has to span
	// both halves of a concatenation for NCD to see the second value repeat the
	// first, so it is sized from the corpus — but the encoder's buffers scale
	// with it, so a corpus of very large values is allowed to lose some
	// cross-boundary matching rather than allocate without limit.
	maxEncoderWindow = 1 << 24
)

// defaultWorkers matches sim's cpu_count-1, floored at 1.
func defaultWorkers() int {
	if n := runtime.NumCPU() - 1; n > 0 {
		return n
	}
	return 1
}

// ---------------------------------------------------------------------------
// Entropy

// shannonEntropy returns the entropy of data in bits per byte, so the result
// is in [0, 8] whatever the length.
func shannonEntropy(data []byte) float64 {
	if len(data) == 0 {
		return 0
	}
	var counts [256]int
	for _, b := range data {
		counts[b]++
	}
	n := float64(len(data))
	h := 0.0
	for _, c := range counts {
		if c == 0 {
			continue
		}
		p := float64(c) / n
		h -= p * math.Log2(p)
	}
	return h
}

// entropyProfile measures entropy block by block, which is what turns a single
// number into a shape: a value whose header is structured and whose payload is
// packed reads as a low value followed by high ones.
func entropyProfile(data []byte) []float64 {
	profile := make([]float64, 0, len(data)/entropyBlockSize+1)
	for i := 0; i < len(data); i += entropyBlockSize {
		block := data[i:min(i+entropyBlockSize, len(data))]
		if len(block) >= minEntropyBlock {
			profile = append(profile, shannonEntropy(block))
		}
	}
	if len(profile) == 0 {
		return []float64{shannonEntropy(data)}
	}
	return profile
}

// entropyFingerprint packs a profile into bytes so it can be fed to the
// compressor. Comparing fingerprints instead of raw bytes is what lets two
// values be recognized as the same *kind* of thing: two encrypted blobs share
// no bytes at all, but their fingerprints are both long runs of ~8.0.
//
// The values are quantized to one byte each rather than written as floats.
// A float32 fingerprint spends three of its four bytes on mantissa noise that
// differs between any two values, which is exactly the incompressible-looking
// detail the fingerprint exists to strip out.
func entropyFingerprint(profile []float64) []byte {
	fp := make([]byte, len(profile))
	for i, h := range profile {
		fp[i] = byte(math.Round(h / 8 * 255))
	}
	return fp
}

// profileDistance is the mean absolute difference between two profiles over
// their common length, divided by 8 to land in [0, 1].
func profileDistance(a, b []float64) float64 {
	n := min(len(a), len(b))
	if n == 0 {
		return 1
	}
	sum := 0.0
	for i := range n {
		sum += math.Abs(a[i] - b[i])
	}
	return sum / (float64(n) * 8)
}

// ---------------------------------------------------------------------------
// Compression

// windowFor returns the smallest legal power-of-two window that spans n.
func windowFor(n int) int {
	w := zstd.MinWindowSize
	for w < n && w < maxEncoderWindow {
		w <<= 1
	}
	return w
}

// newEncoder builds the one encoder every compressed size in a run is measured
// with. EncodeAll is safe for concurrent use up to the configured concurrency,
// so a single encoder serves the whole worker pool.
func newEncoder(window, workers int) (*zstd.Encoder, error) {
	return zstd.NewWriter(nil,
		zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(zstdLevel)),
		zstd.WithEncoderConcurrency(workers),
		zstd.WithWindowSize(window))
}

// ncd is the Normalized Compression Distance of two byte strings, given their
// individually compressed sizes.
//
//	ncd = ( C(ab) - min(C(a), C(b)) ) / max(C(a), C(b))
//
// It is an approximation of a metric, not a metric: for very short or
// pathological inputs the compressor's own framing overhead can push it just
// outside [0, 1], so the caller clamps.
func ncd(enc *zstd.Encoder, a, b []byte, ca, cb int) float64 {
	denom := max(ca, cb)
	if denom == 0 {
		return 0
	}
	joined := make([]byte, 0, len(a)+len(b))
	joined = append(joined, a...)
	joined = append(joined, b...)
	cab := len(enc.EncodeAll(joined, nil))
	return float64(cab-min(ca, cb)) / float64(denom)
}

func clamp01(v float64) float64 {
	return min(max(v, 0), 1)
}

func round(v float64, places int) float64 {
	scale := math.Pow(10, float64(places))
	return math.Round(v*scale) / scale
}

// ---------------------------------------------------------------------------
// Corpus

// sample is one value in the corpus and everything the pair phase derives from
// it. The derived fields are filled in by analyze, once per value rather than
// once per pair: pairing is O(N^2), so anything computed per pair that depends
// on one side alone is computed N^2 times for no reason.
type sample struct {
	index   int    // position in the caller's array
	name    string // label the value travelled with, or ""
	data    []byte
	csize   int       // compressed size of data
	fp      []byte    // packed entropy fingerprint
	fpSize  int       // compressed size of fp
	entropy float64   // global entropy, bits/byte
	profile []float64 // per-block entropy
}

// pairScore is one unordered pair's five distances.
type pairScore struct {
	a, b           int
	hybrid         float64
	ncd            float64
	ncdFingerprint float64
	entropyGlobal  float64
	entropyProfile float64
}

// analyze derives metadata for every value and then scores every unordered
// pair. Both phases run across workers goroutines; the encoder is shared.
func analyze(metas []*sample, alpha, beta float64, workers int) ([]pairScore, error) {
	if workers < 1 {
		workers = 1
	}

	widest := 0
	for _, m := range metas {
		widest = max(widest, len(m.data))
	}
	// An encoder allocates its match tables per concurrent compression, up
	// front, so asking for more concurrency than there is work to do costs
	// megabytes for nothing — a three-value corpus has three pairs.
	concurrency := min(workers, max(numPairs(len(metas)), 1))

	// A concatenation is two values long, so the window is sized for the pair
	// rather than for one side — otherwise the second half could never
	// reference the first and NCD would see no similarity at all.
	enc, err := newEncoder(windowFor(2*widest), concurrency)
	if err != nil {
		return nil, fmt.Errorf("cannot create compressor: %w", err)
	}
	// Close releases the encoder's match tables, which are tens of megabytes.
	// Nothing was written to a stream, so it has no error to report.
	defer func() { _ = enc.Close() }()

	parallelFor(len(metas), workers, func(i int) {
		m := metas[i]
		m.profile = entropyProfile(m.data)
		m.entropy = shannonEntropy(m.data)
		m.fp = entropyFingerprint(m.profile)
		m.csize = len(enc.EncodeAll(m.data, nil))
		m.fpSize = len(enc.EncodeAll(m.fp, nil))
	})

	pairs := make([]pairScore, 0, numPairs(len(metas)))
	for i := range metas {
		for j := i + 1; j < len(metas); j++ {
			pairs = append(pairs, pairScore{a: i, b: j})
		}
	}

	gamma := (1 - alpha - beta) / 2
	parallelFor(len(pairs), workers, func(k int) {
		p := &pairs[k]
		a, b := metas[p.a], metas[p.b]

		p.ncd = clamp01(ncd(enc, a.data, b.data, a.csize, b.csize))
		p.ncdFingerprint = clamp01(ncd(enc, a.fp, b.fp, a.fpSize, b.fpSize))
		p.entropyGlobal = math.Abs(a.entropy-b.entropy) / 8
		p.entropyProfile = profileDistance(a.profile, b.profile)
		p.hybrid = alpha*p.ncd + beta*p.ncdFingerprint +
			gamma*p.entropyGlobal + gamma*p.entropyProfile
	})
	return pairs, nil
}

// numPairs is the number of unordered pairs among n items.
func numPairs(n int) int {
	if n < 2 {
		return 0
	}
	return n * (n - 1) / 2
}

// parallelFor runs fn over [0, n) across workers goroutines, handing out
// indices one at a time so an uneven cost per index still spreads evenly.
func parallelFor(n, workers int, fn func(i int)) {
	if n == 0 {
		return
	}
	workers = min(max(workers, 1), n)

	var next atomic.Int64
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for {
				i := int(next.Add(1)) - 1
				if i >= n {
					return
				}
				fn(i)
			}
		})
	}
	wg.Wait()
}
