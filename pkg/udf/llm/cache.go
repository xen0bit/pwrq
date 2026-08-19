package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// cache stores completed calls on disk, keyed by the exact request.
//
// jq's workflow is edit the query, run it, look, edit it again. Every one of
// those runs re-bills the same prompts, and the ones that changed are usually
// not the expensive ones. Caching is opt-in because a model call is not
// referentially transparent — the same prompt may be *meant* to give a
// different answer — but for building a pipeline it is close enough, and the
// alternative is paying for the same tokens twenty times.
//
// Entries never expire and nothing evicts them. That is a deliberate limit
// rather than an oversight: an expiry would have to guess how long an answer
// stays useful, which depends on the prompt and not on the clock. It is a
// build-time convenience, and a cache directory you can delete is the eviction
// policy.
//
// A nil *cache is a working no-op, so the call path does not branch.
type cache struct {
	dir string
}

// openCache resolves whether caching is on and where it writes.
func openCache(op string, o options) (*cache, error) {
	on := o.Cache
	if !on {
		switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvCache))) {
		case "1", "true", "yes", "on":
			on = true
		}
	}
	if !on {
		return nil, nil
	}

	dir := o.CacheDir
	if dir == "" {
		dir = os.Getenv(EnvCacheDir)
	}
	if dir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return nil, fmt.Errorf("%s: no cache directory; set %s", op, EnvCacheDir)
		}
		dir = filepath.Join(base, "pwrq", "llm")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("%s: creating cache directory: %w", op, err)
	}
	return &cache{dir: dir}, nil
}

// key identifies a request by everything that could change its answer, and
// nothing else. The API key is deliberately absent: it selects an account, not
// a reply, and hashing a credential into a filename on disk is a way to leak
// one.
func (c *cache) key(o options, messages []message) string {
	if c == nil {
		return ""
	}
	identity := struct {
		Model       string    `json:"model"`
		BaseURL     string    `json:"base_url"`
		System      string    `json:"system"`
		Temperature float64   `json:"temperature"`
		TopP        float64   `json:"top_p"`
		MaxTokens   int       `json:"max_tokens"`
		StopAt      []string  `json:"stop_at"`
		Schema      any       `json:"schema"`
		Messages    []message `json:"messages"`
	}{
		Model: o.Model, BaseURL: o.BaseUrl, System: o.System,
		Temperature: o.Temperature, TopP: o.TopP, MaxTokens: o.MaxTokens,
		StopAt: o.StopAt, Schema: o.Schema, Messages: messages,
	}
	encoded, err := json.Marshal(identity)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func (c *cache) path(key string) string { return filepath.Join(c.dir, key+".json") }

// get reads a stored answer. A cache that cannot be read is a cache miss: a
// corrupt entry should cost a request, not the run.
func (c *cache) get(key string) (*response, bool) {
	if c == nil || key == "" {
		return nil, false
	}
	payload, err := os.ReadFile(c.path(key))
	if err != nil {
		return nil, false
	}
	var stored response
	if err := json.Unmarshal(payload, &stored); err != nil {
		return nil, false
	}
	return &stored, true
}

// put stores an answer, ignoring a failure to write for the same reason: a
// full disk should not fail a query that already has its answer.
func (c *cache) put(key string, resp *response) {
	if c == nil || key == "" || resp == nil {
		return
	}
	payload, err := json.Marshal(resp)
	if err != nil {
		return
	}
	_ = os.WriteFile(c.path(key), payload, 0o600)
}
