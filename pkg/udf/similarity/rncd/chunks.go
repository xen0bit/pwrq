package rncd

// Shared-chunk decomposition: which byte ranges of one value occur verbatim in
// another.
//
// This is the concrete form of the question a similarity score answers only in
// aggregate. A compressor already computes it internally — compressing target
// against a dictionary of reference emits back-references into that dictionary,
// and each one is a shared span — but the reference is discarded by the time
// the compressed bytes come back, and the parse it used depends on the
// compression level. So the parse is done here instead: a greedy LZ77 match
// over reference's raw bytes with a hash-chain match finder, which is exact,
// reproducible, and reports where in reference each span was found.

const (
	// hashPrime is the multiplier of the polynomial rolling hash over a
	// window. Any odd constant works; this is the FNV-64 prime.
	hashPrime uint64 = 1099511628211

	// mixPrime spreads a rolling hash across the head table's index bits. The
	// polynomial hash's low bits move slowly, so indexing by them directly
	// would pile unrelated windows into the same bucket.
	mixPrime uint64 = 0x9E3779B97F4A7C15

	// maxChain bounds how many candidate positions are examined per anchor.
	// Every candidate is a full byte comparison, so an unbounded walk makes a
	// repetitive input quadratic; 64 is the usual deflate/zstd compromise
	// between finding the longest match and finding one at all.
	maxChain = 64

	// maxHashBits caps the head table at 4M buckets (32MB), past which a
	// larger table costs memory without reducing collisions that the byte
	// comparison does not already catch.
	maxHashBits = 22
)

// Chunk is one contiguous segment of the target. A matched chunk occurs
// verbatim in the reference starting at RefOffset; a literal chunk is a run
// with no qualifying match, and its RefOffset is -1.
type Chunk struct {
	Start     int
	End       int
	Length    int
	RefOffset int
	Matched   bool
}

// index is a hash chain over every k-byte window of the reference: head[h] is
// the most recent window with hash h, and chain[p] is the window before p that
// hashed the same. -1 ends a chain.
type index struct {
	head  []int32
	chain []int32
	shift uint   // 64 - table index bits
	pow   uint64 // hashPrime^(k-1), the weight of the byte leaving the window
}

func hashWindow(data []byte, pos, k int) uint64 {
	var h uint64
	for i := range k {
		h = h*hashPrime + uint64(data[pos+i])
	}
	return h
}

func (ix *index) bucket(h uint64) uint64 {
	return (h * mixPrime) >> ix.shift
}

func buildIndex(ref []byte, k int) *index {
	bits := uint(10)
	for windows := len(ref) - k + 1; 1<<bits < windows && bits < maxHashBits; bits++ {
	}

	ix := &index{
		head:  make([]int32, 1<<bits),
		shift: 64 - bits,
		pow:   1,
	}
	for range k - 1 {
		ix.pow *= hashPrime
	}
	for i := range ix.head {
		ix.head[i] = -1
	}
	if len(ref) < k {
		return ix
	}

	ix.chain = make([]int32, len(ref)-k+1)
	h := hashWindow(ref, 0, k)
	for p := 0; ; p++ {
		slot := ix.bucket(h)
		ix.chain[p] = ix.head[slot]
		ix.head[slot] = int32(p)
		if p == len(ref)-k {
			break
		}
		h = (h-uint64(ref[p])*ix.pow)*hashPrime + uint64(ref[p+k])
	}
	return ix
}

// longestMatch returns the length and reference offset of the longest run
// starting at target[pos] that also occurs in ref, or (0, -1) if none reaches
// k bytes. Equal-length matches resolve to the earliest offset so the output
// does not depend on chain order.
func (ix *index) longestMatch(ref, target []byte, k, pos int, h uint64) (int, int) {
	bestLen, bestOff := 0, -1
	cand := ix.head[ix.bucket(h)]
	for steps := 0; cand >= 0 && steps < maxChain; steps++ {
		c := int(cand)
		// The hash only proposes a candidate; equality has to be checked.
		if equalAt(ref, target, c, pos, k) {
			l := k
			for pos+l < len(target) && c+l < len(ref) && ref[c+l] == target[pos+l] {
				l++
			}
			if l > bestLen || (l == bestLen && c < bestOff) {
				bestLen, bestOff = l, c
			}
		}
		cand = ix.chain[c]
	}
	if bestLen < k {
		return 0, -1
	}
	return bestLen, bestOff
}

func equalAt(ref, target []byte, refPos, targetPos, k int) bool {
	for i := range k {
		if ref[refPos+i] != target[targetPos+i] {
			return false
		}
	}
	return true
}

// SharedChunks decomposes target into an ordered cover of matched and literal
// segments against ref, where a matched segment is at least minMatch bytes
// long. The chunks tile [0, len(target)) with no gaps and no overlaps, so
// their lengths always sum to the target's size.
//
// minMatch is what separates structure from coincidence: any two values of any
// kind share four-byte runs by chance, so a low value reports noise.
func SharedChunks(ref, target []byte, minMatch int) []Chunk {
	k := max(minMatch, 1)
	n := len(target)

	var chunks []Chunk
	literalFrom := 0
	flushLiteral := func(end int) {
		if end > literalFrom {
			chunks = append(chunks, Chunk{
				Start: literalFrom, End: end, Length: end - literalFrom, RefOffset: -1,
			})
		}
	}

	if len(ref) < k || n < k {
		flushLiteral(n)
		return chunks
	}

	ix := buildIndex(ref, k)
	h := hashWindow(target, 0, k)
	pos := 0
	roll := func() {
		if pos+k < n {
			h = (h-uint64(target[pos])*ix.pow)*hashPrime + uint64(target[pos+k])
		}
		pos++
	}

	for pos+k <= n {
		length, offset := ix.longestMatch(ref, target, k, pos, h)
		if length == 0 {
			roll()
			continue
		}
		flushLiteral(pos)
		chunks = append(chunks, Chunk{
			Start: pos, End: pos + length, Length: length, RefOffset: offset, Matched: true,
		})
		for end := pos + length; pos < end; {
			roll()
		}
		literalFrom = pos
	}
	// The last k-1 bytes cannot start a window, so whatever is left is literal.
	flushLiteral(n)
	return chunks
}

// Coverage sums a decomposition: bytes explained by matches, bytes in total,
// and how many separate spans the matches came in.
func Coverage(chunks []Chunk) (matched, total, spans int) {
	for _, c := range chunks {
		total += c.Length
		if c.Matched {
			matched += c.Length
			spans++
		}
	}
	return matched, total, spans
}
