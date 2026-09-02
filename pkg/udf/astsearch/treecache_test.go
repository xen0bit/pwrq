package astsearch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/xen0bit/pwrq/pkg/core/filewalk"
)

// A cache that changes what a search finds is worse than no cache, so the
// question these ask is not whether it is faster but whether it is invisible.

// corpus writes a few Go files worth searching and returns the directory.
func corpus(t *testing.T, files int) string {
	t.Helper()
	dir := t.TempDir()
	for i := range files {
		source := fmt.Sprintf(`package p

import "crypto/md5"

func digest%d(b []byte) []byte {
	h := md5.New()
	h.Write(b)
	return h.Sum(nil)
}

func other%d() int { return %d }
`, i, i, i)
		name := filepath.Join(dir, fmt.Sprintf("f%d.go", i))
		if err := os.WriteFile(name, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func mustWalk(t *testing.T, dir string) *filewalk.Walker {
	t.Helper()
	walk, err := filewalk.New(dir, "*.go")
	if err != nil {
		t.Fatal(err)
	}
	return walk
}

func search(t *testing.T, dir string, cache *TreeCache) []any {
	t.Helper()
	found, err := SearchTree(context.Background(), dir, []string{"md5.New()"}, "*.go", cache)
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// The same search, cached and not, reports the same thing - and reports it
// again on a second pass, which is the pass that reads what the first one
// parsed rather than parsing for itself.
func TestACachedSearchFindsWhatAnUncachedOneDoes(t *testing.T) {
	dir := corpus(t, 8)

	want := search(t, dir, nil)
	if len(want) != 8 {
		t.Fatalf("expected one match per file, got %d", len(want))
	}

	cache := NewTreeCache()
	defer cache.Release()

	first := search(t, dir, cache)
	second := search(t, dir, cache)

	for name, got := range map[string][]any{"first": first, "second": second} {
		if len(got) != len(want) {
			t.Fatalf("%s cached pass found %d matches, uncached found %d",
				name, len(got), len(want))
		}
		for i := range got {
			if fmt.Sprint(got[i]) != fmt.Sprint(want[i]) {
				t.Fatalf("%s cached pass differs at match %d:\n got %v\nwant %v",
					name, i, got[i], want[i])
			}
		}
	}
}

// A file rewritten between two searches is searched again rather than answered
// from the tree built for the text it used to hold. A long call - and a corpus
// run is minutes - is long enough for that to happen.
func TestAFileChangedUnderTheCacheIsReadAgain(t *testing.T) {
	dir := corpus(t, 1)
	cache := NewTreeCache()
	defer cache.Release()

	if got := len(search(t, dir, cache)); got != 1 {
		t.Fatalf("expected one match to begin with, got %d", got)
	}

	name := filepath.Join(dir, "f0.go")
	if err := os.WriteFile(name, []byte("package p\n\nfunc nothing() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The stamp is size and modification time, and a rewrite inside one
	// filesystem timestamp tick that kept the byte count would be invisible to
	// it. This rewrite changes the size, which is what the test is about.
	if got := len(search(t, dir, cache)); got != 0 {
		t.Fatalf("the rewritten file still reported %d matches, so the cache "+
			"answered from the text that is no longer there", got)
	}
}

// The budget is a limit rather than a suggestion: a cache too small for the
// tree keeps what fits and the search still returns everything.
func TestASearchOverflowingTheCacheStillFindsEverything(t *testing.T) {
	dir := corpus(t, 8)
	want := len(search(t, dir, nil))

	// One byte of budget takes nothing at all, which is the hardest case for
	// the caller: every file is parsed, and every file has to be released by
	// the search rather than by the cache.
	tiny := &TreeCache{budget: 1, files: map[string]*cachedTree{}}
	defer tiny.Release()

	if got := len(search(t, dir, tiny)); got != want {
		t.Fatalf("with a full cache the search found %d matches, want %d", got, want)
	}
	if len(tiny.files) != 0 {
		t.Fatalf("a one-byte cache took %d files", len(tiny.files))
	}
}

// A nil cache is a working cache. select_ast passes one, so this is the path
// every ordinary query takes.
func TestANilCacheSearchesNormally(t *testing.T) {
	dir := corpus(t, 3)
	var none *TreeCache
	if got := len(search(t, dir, none)); got != 3 {
		t.Fatalf("nil cache found %d matches, want 3", got)
	}
	none.Release()
}

// Setting the budget to zero turns the cache off, which is what a caller short
// of memory reaches for.
func TestAZeroBudgetTurnsTheCacheOff(t *testing.T) {
	t.Setenv(EnvTreeCacheBytes, "0")
	if cache := NewTreeCache(); cache != nil {
		t.Fatalf("a zero budget produced a cache: %#v", cache)
	}
}

// select_ast stops when its deadline passes even while it is finding nothing.
//
// The lazy walk yields per match, so gojq's own check between values covers a
// search that is finding things and covers nothing at all for one that is not.
// This is the case that had no other moment to notice.
func TestALazyWalkStopsWhenItsDeadlinePasses(t *testing.T) {
	dir := corpus(t, 4)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	it := &matchIter{
		ctx:      ctx,
		patterns: []string{"md5.New()"},
		opts:     selectAstOpts{include: "*.go"},
		walk:     mustWalk(t, dir),
	}
	v, ok := it.Next()
	if !ok {
		t.Fatal("a cancelled walk ended quietly, which reads as no matches")
	}
	if _, isErr := v.(error); !isErr {
		t.Fatalf("a cancelled walk produced %v rather than an error", v)
	}
}
