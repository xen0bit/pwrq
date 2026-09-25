package webapi

import (
	"context"
	"time"
)

// ---------------------------------------------------------------------------
// deadlines

// deadline is a context that expires without needing a timer.
//
// Under GOOS=js there is one thread and no preemption: a goroutine running a
// tight loop never yields, so the JavaScript event loop never runs, so a timer
// set by context.WithTimeout never fires. A deadline built on time.AfterFunc
// would therefore protect the page from exactly nothing.
//
// gojq calls ctx.Done() once per instruction it executes, so checking the
// clock there works where a timer cannot. The check is sampled rather than
// taken every time, because time.Now() is not free.
//
// It is used by one run at a time and is not safe for concurrent use.
type deadline struct {
	at        time.Time
	done      chan struct{}
	closed    bool
	countdown int
}

// clockCheckInterval is how many instructions pass between clock readings.
// Small enough that a runaway query is caught promptly, large enough that the
// check does not dominate the run.
const clockCheckInterval = 2000

// newDeadline is the page's ideengine.Config.Deadline. The parent is ignored:
// the worker has nothing that would cancel it, and the one way to stop a run
// early from outside is to terminate the worker.
func newDeadline(_ context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return &deadline{at: time.Now().Add(d), done: make(chan struct{}), countdown: clockCheckInterval}, func() {}
}

func (d *deadline) Deadline() (time.Time, bool) { return d.at, true }

func (d *deadline) Done() <-chan struct{} {
	if d.closed {
		return d.done
	}
	d.countdown--
	if d.countdown > 0 {
		return d.done
	}
	d.countdown = clockCheckInterval
	if !time.Now().Before(d.at) {
		d.closed = true
		close(d.done)
	}
	return d.done
}

func (d *deadline) Err() error {
	if d.closed {
		return context.DeadlineExceeded
	}
	return nil
}

func (d *deadline) Value(any) any { return nil }
