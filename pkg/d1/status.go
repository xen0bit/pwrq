package d1

import (
	"sync"

	"github.com/xen0bit/pwrq/pkg/d1/assets"
)

// States the shared model passes through, for a spinner.
const (
	StateIdle     = "idle"     // not loaded yet
	StateFetching = "fetching" // downloading model or runtime files
	StateLoading  = "loading"  // opening sessions
	StateReady    = "ready"
	StateBusy     = "busy" // a forward pass is running
	StateError    = "error"
)

// Status is what the model is doing now.
type Status struct {
	State  string `json:"state"`
	Device string `json:"device,omitempty"`
	File   string `json:"file,omitempty"`
	Done   int64  `json:"done,omitempty"`
	Total  int64  `json:"total,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Fraction is a fetch's progress, -1 when unknown.
func (s Status) Fraction() float64 {
	if s.Total <= 0 {
		return -1
	}
	return float64(s.Done) / float64(s.Total)
}

var (
	statusMu sync.Mutex
	status   = Status{State: StateIdle}
	watchers []func(Status)
)

func setStatus(s Status) {
	statusMu.Lock()
	status = s
	ws := append([]func(Status){}, watchers...)
	statusMu.Unlock()
	for _, w := range ws {
		w(s)
	}
}

// CurrentStatus is what the shared model is doing now.
func CurrentStatus() Status {
	statusMu.Lock()
	defer statusMu.Unlock()
	return status
}

// Watch calls f on every status change until the returned function is
// called.
func Watch(f func(Status)) func() {
	statusMu.Lock()
	defer statusMu.Unlock()
	watchers = append(watchers, f)
	i := len(watchers) - 1
	return func() {
		statusMu.Lock()
		defer statusMu.Unlock()
		if i < len(watchers) {
			watchers[i] = func(Status) {}
		}
	}
}

func reportAsset(e assets.Event) {
	setStatus(Status{State: StateFetching, File: e.Name, Done: e.Done, Total: e.Total})
}
