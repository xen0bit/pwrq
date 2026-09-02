package runctx

import (
	"context"
	"testing"
)

// Outside any run there is a context, and it is the one that says nothing.
// Cmdlets read it unconditionally, so the alternative to this is a nil
// dereference in whichever cmdlet is called from a test first.
func TestThereIsAlwaysAContext(t *testing.T) {
	if Current() == nil {
		t.Fatal("Current returned nil")
	}
	if err := Err(); err != nil {
		t.Fatalf("Err outside a run returned %v, want nil", err)
	}
}

// A run publishes its context and takes it back down again, so nothing after
// it inherits a deadline that has passed.
func TestARunPutsBackWhatItFound(t *testing.T) {
	before := Current()

	ctx, cancel := context.WithCancel(context.Background())
	restore := Install(ctx)
	if Current() != ctx {
		t.Fatal("Install did not publish the context")
	}
	cancel()
	if Err() == nil {
		t.Fatal("a cancelled run reported no error")
	}

	restore()
	if Current() != before {
		t.Fatal("restore did not put back the context that was there")
	}
	if err := Err(); err != nil {
		t.Fatalf("the cancellation outlived the run: %v", err)
	}
}

// A run nested inside a cmdlet - invoke_agent runs a query of its own - leaves
// the outer run's deadline in place when it finishes. Restoring rather than
// clearing is the whole reason Install hands back a function.
func TestANestedRunLeavesTheOuterOneInPlace(t *testing.T) {
	outer, cancelOuter := context.WithCancel(context.Background())
	defer cancelOuter()
	restoreOuter := Install(outer)
	defer restoreOuter()

	inner, cancelInner := context.WithCancel(outer)
	restoreInner := Install(inner)
	if Current() != inner {
		t.Fatal("the nested run did not become current")
	}
	cancelInner()
	restoreInner()

	if Current() != outer {
		t.Fatal("the outer run did not become current again")
	}
	if err := Err(); err != nil {
		t.Fatalf("cancelling the nested run stopped the outer one: %v", err)
	}
}

// A nil context is the caller's mistake and not worth a panic in a cmdlet
// three layers down, so it reads as the context that says nothing.
//
// Written through a variable because passing a nil context at a call site is
// exactly what a linter is right to object to everywhere except here, where it
// is the case under test.
func TestInstallingNothingIsBackground(t *testing.T) {
	var missing context.Context
	defer Install(missing)()
	if err := Err(); err != nil {
		t.Fatalf("a nil context reported %v", err)
	}
	if Current() == nil {
		t.Fatal("a nil context became a nil Current")
	}
}
