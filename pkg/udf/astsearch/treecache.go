package astsearch

import (
	"os"
	"strconv"
	"sync"
)

// Keeping parsed files between one search and the next.
//
// A rule is a query, so each rule walks the tree itself and nothing outside a
// rule knows which files it will ask for. That is the right way round - a rule
// you can read and edit is worth more than one that is cheap to schedule - but
// it means a corpus run parses every file once per rule. Naming a language to
// invoke_pwrgrep selects a few hundred rules, and over a repository of a few
// hundred files that is a hundred thousand parses of the same text: the Python
// corpus over certbot took 7 minutes 20 seconds, and reading those files over
// again was a third of it.
//
// A parse tree does not depend on which rule is about to be run against it, so
// this keeps it. The second rule finds the tree the first one built, and every
// rule after that does no parsing at all.
//
// What stops it from being simply a map is that trees are large. A parsed file
// costs about two hundred times its source: certbot's 3MB of Python is 600MB
// of trees, and there are repositories where holding all of it would be worse
// than the problem. So the cache has a budget, and once it is full it stops
// taking new files rather than evicting one - a search that overflows still
// gets its answer, and the files it did keep are still hits for the next rule.
//
// Nothing is evicted while a search is running, which is what makes it safe
// without reference counting: a tree in the cache is never released while a
// worker might be reading it, because release only happens when the whole
// cache is dropped at the end of the call that made it.
//
// What it is worth, over the Python corpus and certbot, is 339 seconds down to
// 279 and a peak of 7.7GB up to 8.9GB. The peak is worth reading twice, because
// it is not what the cache costs: 7.7GB is what the same run costs with the
// cache turned off. Nearly all of it is garbage from running tree-sitter
// queries - 56% of everything a corpus run allocates - and from grep building
// a parser per pattern it compiles, neither of which is ours. The cache adds
// about 1.2GB on top of that and takes a fifth off the clock.

// treeCacheBudget is how many bytes of parse tree a cache will hold.
//
// 512MB, because a rule set asks for the same files over and over: a cache
// that holds most of a repository hits almost every time, and one that holds a
// fraction of it thrashes. The whole Python corpus over certbot is 339s with
// no cache, 308s with 128MB and 279s with 512MB; a fifty-rule search, where
// the fixed cost of compiling patterns is not there to dilute it, is 15.7s,
// 13.8s and 8.5s. Either way the last doubling is where the working set starts
// to fit, and 512MB is enough for a repository of a few thousand files.
//
// PWRQ_TREE_CACHE_BYTES overrides it. Zero turns the cache off, which is the
// setting for a machine short of memory: it costs about a fifth of the speed
// and saves the 1.2GB named above.
const treeCacheBudget = 512 << 20

// treeBytesPerSourceByte is how much memory a parsed file costs, as a multiple
// of the source it was parsed from.
//
// Measured rather than derived: holding every parsed Python file in certbot -
// 366 files, 2.9MB of source - costs 596MB of resident memory, and the ratio
// holds from fifty files upward. It is an estimate and does not need to be
// better than one; it decides when to stop filling a cache, and being wrong by
// a factor of two moves that point by a factor of two.
const treeBytesPerSourceByte = 200

// EnvTreeCacheBytes names the environment variable that overrides the budget.
const EnvTreeCacheBytes = "PWRQ_TREE_CACHE_BYTES"

// TreeCache holds files parsed by one call, so the next search in that call
// does not parse them again.
//
// The zero value is not usable; NewTreeCache makes one. A nil *TreeCache is,
// and means "do not cache" - which is what select_ast passes, because it
// promises a lazy walk that stops at the first match and must not be made to
// hold the tree it walked through to get there.
type TreeCache struct {
	mu     sync.Mutex
	budget int64
	held   int64
	files  map[string]*cachedTree
}

// cachedTree is one parsed file and what it was parsed from, so that a file
// changed on disk under a long call is parsed again rather than answered from
// the copy read first.
type cachedTree struct {
	tree    *parsed
	size    int64
	modTime int64
}

// NewTreeCache makes a cache with the budget this process runs under, or nil
// when the budget is zero. A nil cache is a working cache that never hits, so
// callers do not branch on it.
func NewTreeCache() *TreeCache {
	budget := treeCacheBudgetBytes()
	if budget <= 0 {
		return nil
	}
	return &TreeCache{budget: budget, files: map[string]*cachedTree{}}
}

// treeCacheBudgetBytes reads the budget, which is a constant unless the
// environment says otherwise. A value that is not a number is ignored rather
// than reported: this is a tuning knob, and failing a search over it would be
// out of proportion.
func treeCacheBudgetBytes() int64 {
	if v := os.Getenv(EnvTreeCacheBytes); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			return n
		}
	}
	return treeCacheBudget
}

// get returns the tree already parsed for this file, if it is there and still
// describes what is on disk.
//
// The caller must not release what comes back: the cache owns it.
func (c *TreeCache) get(path string, size, modTime int64) (*parsed, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.files[path]
	if !ok || entry.size != size || entry.modTime != modTime {
		return nil, false
	}
	return entry.tree, true
}

// put offers a parsed file to the cache and reports whether it was taken.
//
// Taken means the cache owns the tree and will release it; not taken means the
// caller still has to. There is no eviction: a full cache keeps what it has,
// because what it has is what the rest of this call is about to ask for again.
func (c *TreeCache) put(path string, size, modTime int64, tree *parsed) bool {
	if c == nil || tree == nil {
		return false
	}
	cost := size * treeBytesPerSourceByte
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.files[path]; ok {
		// The file changed under us. The copy we were holding describes text
		// nobody will ask about again.
		c.held -= existing.size * treeBytesPerSourceByte
		delete(c.files, path)
		existing.tree.release()
	}
	if c.held+cost > c.budget {
		return false
	}
	c.files[path] = &cachedTree{tree: tree, size: size, modTime: modTime}
	c.held += cost
	return true
}

// Release gives back every tree the cache is holding. It is what ends the call
// that made the cache, and using the cache afterwards is a bug rather than a
// slow path, so it empties the map as well.
func (c *TreeCache) Release() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, entry := range c.files {
		entry.tree.release()
	}
	c.files = map[string]*cachedTree{}
	c.held = 0
}

// The cache in force for the run in progress.
//
// scan_ast is what a rule calls, and invoke_pwrgrep is what runs the rules, so
// the one that knows a cache is worth having is not the one that would use it.
// gojq gives a cmdlet its input and its arguments and nothing else, so there is
// nowhere to pass it: it is ambient, installed for the length of the call that
// wants it, exactly as the run's context is. See runctx, which explains why
// that is sound.
var (
	ambientMu    sync.RWMutex
	ambientCache *TreeCache
)

// InstallTreeCache makes cache the one scan_ast will use, and returns the
// function that puts back what was there before. The caller defers the result;
// releasing the cache is the caller's too, because the caller knows when the
// last search that could hit it has finished.
func InstallTreeCache(cache *TreeCache) (restore func()) {
	ambientMu.Lock()
	previous := ambientCache
	ambientCache = cache
	ambientMu.Unlock()
	return func() {
		ambientMu.Lock()
		ambientCache = previous
		ambientMu.Unlock()
	}
}

// AmbientTreeCache is the cache searches should use, or nil when nothing has
// installed one - which is the ordinary case for a query typed at the prompt.
func AmbientTreeCache() *TreeCache {
	ambientMu.RLock()
	defer ambientMu.RUnlock()
	return ambientCache
}
