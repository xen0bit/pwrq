package rncd

import (
	"math"
	"strings"
	"testing"
)

func TestShannonEntropyExtremes(t *testing.T) {
	if h := shannonEntropy(nil); h != 0 {
		t.Errorf("entropy of nothing = %f, want 0", h)
	}
	if h := shannonEntropy([]byte("aaaaaaaa")); h != 0 {
		t.Errorf("entropy of one repeated symbol = %f, want 0", h)
	}
	uniform := make([]byte, 256)
	for i := range uniform {
		uniform[i] = byte(i)
	}
	if h := shannonEntropy(uniform); h < 7.99 || h > 8.0001 {
		t.Errorf("entropy of every byte once = %f, want ~8", h)
	}
}

func TestEntropyProfileAlwaysReported(t *testing.T) {
	// A file shorter than one block still has to produce a profile, or the
	// profile distance would divide by zero for every pair involving it.
	if got := entropyProfile([]byte("hi")); len(got) != 1 {
		t.Errorf("profile of a 2-byte file = %v, want one value", got)
	}
	if got := entropyProfile(make([]byte, 4*entropyBlockSize)); len(got) != 4 {
		t.Errorf("profile of 4 blocks has %d values, want 4", len(got))
	}
}

func TestProfileDistanceBounds(t *testing.T) {
	flat := []float64{8, 8, 8, 8}
	zero := []float64{0, 0, 0, 0}
	if d := profileDistance(flat, flat); d != 0 {
		t.Errorf("a profile against itself = %f, want 0", d)
	}
	if d := profileDistance(flat, zero); d != 1 {
		t.Errorf("the two extreme profiles = %f, want 1", d)
	}
	if d := profileDistance(nil, flat); d != 1 {
		t.Errorf("an absent profile = %f, want 1", d)
	}
}

// TestFingerprintStripsMantissaNoise is why the fingerprint quantizes each
// block's entropy to a byte instead of writing a float. Two random blobs have
// near-identical entropy but never the same low mantissa bits, and those bits
// are incompressible — encoding them makes two files of the same class look
// as different as two files of different classes, which is the one thing the
// fingerprint exists to prevent.
func TestFingerprintStripsMantissaNoise(t *testing.T) {
	a := entropyFingerprint([]float64{7.9991, 7.9987, 7.9993})
	b := entropyFingerprint([]float64{7.9986, 7.9994, 7.9989})
	if string(a) != string(b) {
		t.Errorf("two flat high-entropy profiles fingerprinted differently: %v vs %v", a, b)
	}
	low := entropyFingerprint([]float64{1.0, 1.2, 0.9})
	if string(low) == string(a) {
		t.Error("a low-entropy profile fingerprinted the same as a high-entropy one")
	}
}

func TestFingerprintSpansTheRange(t *testing.T) {
	fp := entropyFingerprint([]float64{0, 4, 8})
	want := []byte{0, 128, 255}
	for i := range want {
		if fp[i] != want[i] {
			t.Errorf("fingerprint of {0,4,8} = %v, want %v", fp, want)
			break
		}
	}
}

func TestWindowCoversInputAndIsCapped(t *testing.T) {
	for _, n := range []int{1, 1000, 5000, 1 << 20} {
		w := windowFor(n)
		if w < n {
			t.Errorf("window for %d bytes is %d, which cannot span the input", n, w)
		}
		if w&(w-1) != 0 {
			t.Errorf("window %d is not a power of two", w)
		}
	}
	if w := windowFor(1 << 30); w != maxEncoderWindow {
		t.Errorf("window for a huge input = %d, want the cap %d", w, maxEncoderWindow)
	}
}

// corpus is the fixture the score is judged on: two near-identical documents,
// an unrelated document, and two incompressible blobs.
//
// The files are 32KB rather than a few hundred bytes because the fingerprint
// holds one byte per 256-byte block, so a 4KB file fingerprints to 16 bytes —
// too short for a compressed size to say anything, and the distance over it is
// then noise. That is a real limit of the metric, not of the fixture: on inputs
// of a few kilobytes the class terms that carry weight are the two entropy
// distances, and the fingerprint only starts to contribute above ~32KB.
func corpus(t *testing.T) []*sample {
	t.Helper()
	blob := func(seed uint64, n int) []byte {
		out := make([]byte, n)
		for i := range out {
			seed = seed*6364136223846793005 + 1442695040888963407
			out[i] = byte(seed >> 33)
		}
		return out
	}
	return []*sample{
		{name: "doc_a", data: []byte(strings.Repeat("the cat sat on the mat. ", 1400))},
		{name: "doc_b", data: []byte(strings.Repeat("the cat lay on the rug. ", 1400))},
		{name: "doc_c", data: []byte(strings.Repeat("quantum entanglement abounds. ", 1100))},
		{name: "enc_a", data: blob(1, 32768)},
		{name: "enc_b", data: blob(2, 32768)},
	}
}

func score(t *testing.T) (map[string]pairScore, []*sample) {
	t.Helper()
	metas := corpus(t)
	pairs, err := analyze(metas, 0.5, 0.25, 4)
	if err != nil {
		t.Fatal(err)
	}
	byName := make(map[string]pairScore, len(pairs))
	for _, p := range pairs {
		byName[metas[p.a].name+"|"+metas[p.b].name] = p
	}
	return byName, metas
}

func TestEveryComponentIsBounded(t *testing.T) {
	pairs, _ := score(t)
	if len(pairs) != numPairs(5) {
		t.Fatalf("scored %d pairs, want %d", len(pairs), numPairs(5))
	}
	for name, p := range pairs {
		for label, v := range map[string]float64{
			"hybrid": p.hybrid, "ncd": p.ncd, "ncd_fingerprint": p.ncdFingerprint,
			"entropy_global": p.entropyGlobal, "entropy_profile": p.entropyProfile,
		} {
			if v < 0 || v > 1 || math.IsNaN(v) {
				t.Errorf("%s: %s = %f, outside [0,1]", name, label, v)
			}
		}
	}
}

func TestMostSimilarPairIsTheTwoLikeDocuments(t *testing.T) {
	pairs, _ := score(t)
	best, bestVal := "", 2.0
	for name, p := range pairs {
		if p.hybrid < bestVal {
			best, bestVal = name, p.hybrid
		}
	}
	if best != "doc_a|doc_b" {
		t.Errorf("closest pair is %s (%f), want doc_a|doc_b", best, bestVal)
	}
}

// TestClassSimilarityRescuesIncompressiblePairs is the whole reason the score
// is a blend. Two random blobs share no bytes, so compression alone calls them
// as unrelated as a blob and a document; the entropy terms have to pull them
// back together.
func TestClassSimilarityRescuesIncompressiblePairs(t *testing.T) {
	pairs, _ := score(t)
	same := pairs["enc_a|enc_b"]
	mixed := pairs["doc_a|enc_a"]

	if same.ncd < 0.9 {
		t.Errorf("two random blobs should not compress together, ncd = %f", same.ncd)
	}
	if same.entropyGlobal > 0.05 {
		t.Errorf("two random blobs should have the same global entropy, got %f", same.entropyGlobal)
	}
	if same.ncdFingerprint >= mixed.ncdFingerprint {
		t.Errorf("fingerprint distance failed to separate the classes: blob/blob %f >= blob/doc %f",
			same.ncdFingerprint, mixed.ncdFingerprint)
	}
	if same.hybrid >= mixed.hybrid {
		t.Errorf("blob/blob (%f) should score closer than blob/doc (%f)", same.hybrid, mixed.hybrid)
	}
}

func TestWeightsShiftTheScore(t *testing.T) {
	metas := corpus(t)
	// All weight on raw bytes: the two random blobs stop looking alike.
	pairs, err := analyze(metas, 1, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pairs {
		if metas[p.a].name == "enc_a" && metas[p.b].name == "enc_b" {
			if p.hybrid != p.ncd {
				t.Errorf("with Alpha=1 the score should be the compression distance, got %f vs %f", p.hybrid, p.ncd)
			}
			if p.hybrid < 0.9 {
				t.Errorf("byte-wise, two random blobs should be far apart, got %f", p.hybrid)
			}
		}
	}
}

func TestNumPairs(t *testing.T) {
	for n, want := range map[int]int{0: 0, 1: 0, 2: 1, 5: 10, 100: 4950} {
		if got := numPairs(n); got != want {
			t.Errorf("numPairs(%d) = %d, want %d", n, got, want)
		}
	}
}

func TestParallelForVisitsEveryIndexOnce(t *testing.T) {
	const n = 1000
	seen := make([]int32, n)
	parallelFor(n, 8, func(i int) { seen[i]++ })
	for i, c := range seen {
		if c != 1 {
			t.Fatalf("index %d visited %d times, want once", i, c)
		}
	}
	parallelFor(0, 8, func(int) { t.Fatal("called for an empty range") })
}
