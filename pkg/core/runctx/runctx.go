// Package runctx carries the deadline of the query being evaluated down to the
// cmdlets evaluating under it.
//
// gojq hands a native function its input and its arguments and nothing else.
// There is no way to reach the context a query is running under from inside a
// cmdlet, so a cmdlet that does a long piece of work - walking a tree, running
// a corpus of rules - could not be stopped once it had started. The host's
// timeout was still enforced, but only between the values the program yielded,
// and a cmdlet that yields once at the end never reaches that check: a run of
// invoke_pwrgrep over a large repository ignored a ten-minute deadline for as
// long as it took, which under the MCP server meant every later call queued
// behind a run nobody was waiting for any more.
//
// So the context is ambient. Runner.Run installs the one it was given for the
// length of the run and puts back what was there before, and a cmdlet that can
// be interrupted reads it with Current.
//
// Ambient rather than threaded because threading it would mean a second
// signature for every cmdlet that might one day want it, and ambient rather
// than a field on some evaluation object because gojq gives us nowhere to hang
// one. It is the shape common.WithScriptBlockOptions already uses, and it is
// sound for the same reason: gojq evaluates a query on one goroutine, the MCP
// server serialises runs behind a mutex, and a run nested inside a cmdlet -
// invoke_agent is the one that does this - installs a context derived from the
// outer one and restores the outer on the way out.
package runctx

import (
	"context"
	"sync"
)

var (
	mu      sync.RWMutex
	current context.Context = context.Background()
)

// Install makes ctx the context cmdlets observe, and returns the function that
// puts back the one that was there before.
//
// The caller defers the result. Restoring rather than clearing is what makes a
// nested run work: the inner run's context is derived from the outer one, and
// the outer one has to be current again once the inner is done.
func Install(ctx context.Context) (restore func()) {
	if ctx == nil {
		ctx = context.Background()
	}
	mu.Lock()
	previous := current
	current = ctx
	mu.Unlock()
	return func() {
		mu.Lock()
		current = previous
		mu.Unlock()
	}
}

// Current is the context of the run in progress.
//
// It is never nil. A cmdlet reached outside any run - from a test, or from a
// host that does not use Runner - gets context.Background(), which is the
// honest answer: nothing has said when to stop.
func Current() context.Context {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// Err is why the run should stop, or nil while it should keep going.
//
// This is the cheap check a long loop makes per iteration. It is a read lock
// and a load, so a walk over a tree can afford it per file.
func Err() error {
	return Current().Err()
}
