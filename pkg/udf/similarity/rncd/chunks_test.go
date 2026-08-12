package rncd

import (
	"bytes"
	"testing"
)

// noise returns deterministic bytes that do not match another seed's stream by
// accident — a linear ramp would match itself at every phase shift and make a
// "no shared content" fixture share content.
func noise(n, seed int) []byte {
	out := make([]byte, n)
	state := uint64(seed)*2862933555777941757 + 3037000493
	for i := range out {
		state = state*6364136223846793005 + 1442695040888963407
		out[i] = byte(state >> 33)
	}
	return out
}

// assertTiling checks the invariant every consumer relies on: the chunks cover
// the target exactly once, in order, with no gaps and no overlaps.
func assertTiling(t *testing.T, chunks []Chunk, total int) {
	t.Helper()
	pos := 0
	for i, c := range chunks {
		if c.Start != pos {
			t.Fatalf("chunk %d starts at %d, expected %d", i, c.Start, pos)
		}
		if c.End != c.Start+c.Length {
			t.Fatalf("chunk %d: End %d != Start+Length %d", i, c.End, c.Start+c.Length)
		}
		if c.Matched != (c.RefOffset >= 0) {
			t.Fatalf("chunk %d: Matched=%v with RefOffset=%d", i, c.Matched, c.RefOffset)
		}
		pos = c.End
	}
	if pos != total {
		t.Fatalf("chunks cover %d bytes of a %d-byte target", pos, total)
	}
}

func TestSharedSpanIsFoundAndLocated(t *testing.T) {
	shared := noise(1024, 0)
	ref := concat(noise(300, 11), shared, noise(200, 22))
	target := concat(noise(100, 33), shared, noise(500, 44))

	chunks := SharedChunks(ref, target, 16)
	assertTiling(t, chunks, len(target))

	var match *Chunk
	for i := range chunks {
		if chunks[i].Matched {
			if match != nil {
				t.Fatalf("expected one match, found another at %d", chunks[i].Start)
			}
			match = &chunks[i]
		}
	}
	if match == nil {
		t.Fatal("the shared span was not found")
	}
	if match.Start != 100 || match.Length != 1024 || match.RefOffset != 300 {
		t.Errorf("match = {Start:%d Length:%d RefOffset:%d}, want {100 1024 300}",
			match.Start, match.Length, match.RefOffset)
	}
	// The offset has to be usable, not merely plausible.
	if !bytes.Equal(ref[match.RefOffset:match.RefOffset+match.Length], target[match.Start:match.End]) {
		t.Error("the reported span does not equal the reference bytes at RefOffset")
	}

	matched, total, spans := Coverage(chunks)
	if matched != 1024 || total != len(target) || spans != 1 {
		t.Errorf("coverage = (%d, %d, %d), want (1024, %d, 1)", matched, total, spans, len(target))
	}
}

func TestIdenticalFilesAreFullyCovered(t *testing.T) {
	data := noise(20000, 7)
	chunks := SharedChunks(data, data, 16)
	assertTiling(t, chunks, len(data))
	matched, total, _ := Coverage(chunks)
	if matched != total {
		t.Errorf("a file against itself covered %d/%d bytes", matched, total)
	}
}

func TestDisjointFilesShareNothing(t *testing.T) {
	chunks := SharedChunks(bytes.Repeat([]byte{0x00}, 2000), bytes.Repeat([]byte{0xFF}, 2000), 16)
	assertTiling(t, chunks, 2000)
	matched, _, spans := Coverage(chunks)
	if matched != 0 || spans != 0 {
		t.Errorf("files with no byte in common reported %d bytes over %d spans", matched, spans)
	}
}

// TestMinMatchFiltersCoincidence is the reason MinMatch exists: at one byte
// almost everything matches something, so the decomposition stops being
// evidence of anything.
func TestMinMatchFiltersCoincidence(t *testing.T) {
	ref := noise(4000, 1)
	target := noise(4000, 2)

	loose, _, _ := Coverage(SharedChunks(ref, target, 2))
	strict, _, _ := Coverage(SharedChunks(ref, target, 16))
	if loose <= strict {
		t.Errorf("a 2-byte minimum matched %d bytes, no more than a 16-byte minimum's %d", loose, strict)
	}
	if strict != 0 {
		t.Errorf("unrelated files shared %d bytes in runs of 16+", strict)
	}
}

func TestMinMatchLongerThanTheFiles(t *testing.T) {
	chunks := SharedChunks(noise(10, 0), noise(10, 0), 16)
	assertTiling(t, chunks, 10)
	if len(chunks) != 1 || chunks[0].Matched {
		t.Errorf("expected a single literal chunk, got %+v", chunks)
	}
}

func TestEmptyTargetHasNoChunks(t *testing.T) {
	if chunks := SharedChunks(noise(100, 0), nil, 16); len(chunks) != 0 {
		t.Errorf("an empty target produced %d chunks", len(chunks))
	}
}

// TestMatchAtTheVeryEnd guards the tail: the last MinMatch-1 bytes cannot start
// a window, so a span that runs to the end of the target is the case a naive
// loop truncates.
func TestMatchAtTheVeryEnd(t *testing.T) {
	shared := noise(64, 5)
	ref := concat(noise(100, 6), shared)
	target := concat(noise(100, 7), shared)

	chunks := SharedChunks(ref, target, 16)
	assertTiling(t, chunks, len(target))
	last := chunks[len(chunks)-1]
	if !last.Matched || last.End != len(target) || last.Length != 64 {
		t.Errorf("trailing span = %+v, want a 64-byte match ending at %d", last, len(target))
	}
}

func TestRepetitiveContentStillTiles(t *testing.T) {
	// Every window hashes the same here, which is what exercises the chain
	// limit; the tiling invariant has to survive it.
	ref := bytes.Repeat([]byte("ab"), 5000)
	target := bytes.Repeat([]byte("ab"), 3000)
	chunks := SharedChunks(ref, target, 16)
	assertTiling(t, chunks, len(target))
	matched, total, _ := Coverage(chunks)
	if matched != total {
		t.Errorf("a repeat of the reference covered %d/%d bytes", matched, total)
	}
}

func concat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}
