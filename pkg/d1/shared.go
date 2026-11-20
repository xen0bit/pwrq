package d1

import (
	"context"
	"sync"
)

// The shared model: one per process, opened on first use and kept. Two
// gigabytes of weights take seconds to load and a pipeline asks per input
// value, so every caller - the cmdlets, the editors - shares this one.
// A failed load is not remembered: the next call tries again, so a
// download that failed for a moment does not disable the model for the
// life of the process.
var shared struct {
	mu    sync.Mutex
	model *Model
	key   sharedKey
}

// sharedKey is Options without its logger, which cannot be compared.
type sharedKey struct {
	device, modelDir, cacheDir, ortLib string
	offline, fetchGPU, verbose         bool
	threads, gpuID                     int
}

func keyOf(o Options) sharedKey {
	return sharedKey{o.Device, o.ModelDir, o.CacheDir, o.ORTLib, o.Offline, o.FetchGPU, o.Verbose, o.Threads, o.GPUID}
}

// Shared returns the process's model, opening it with o on first use (or
// when o differs from what it was opened with).
func Shared(ctx context.Context, o Options) (*Model, error) {
	shared.mu.Lock()
	defer shared.mu.Unlock()
	key := keyOf(o)
	if shared.model != nil && shared.key == key {
		return shared.model, nil
	}
	m, err := Open(ctx, o)
	if err != nil {
		setStatus(Status{State: StateError, Error: err.Error()})
		return nil, err
	}
	if shared.model != nil {
		shared.model.Close()
	}
	shared.model, shared.key = m, key
	return m, nil
}

// Loaded returns the shared model if one is open, without opening it.
func Loaded() (*Model, bool) {
	shared.mu.Lock()
	defer shared.mu.Unlock()
	return shared.model, shared.model != nil
}
