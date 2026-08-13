package queryrun

import (
	"strings"
	"testing"

	"github.com/xen0bit/pwrq/pkg/core/psobject"
)

// The encoder is the last thing between a query result and the caller, and it
// is the one place that can still be handed a value gojq cannot represent. Both
// ways that used to end the process are pinned here: gojq.Marshal panics on a
// type it does not know, and it recurses, so a deep enough value overflows the
// goroutine stack - a fatal error no recover() can catch. A host that embeds
// this package (the MCP server serves many callers from one process) must get
// an error back instead.

// nest builds a value nested depth levels deep: [[[...]]].
func nest(depth int) any {
	var v any
	for range depth {
		v = []any{v}
	}
	return v
}

func TestEncodeRejectsDeepValues(t *testing.T) {
	e := newEncoder(&Request{Compact: true})

	if _, err := e.encode(nest(psobject.MaxJSONDepth - 1)); err != nil {
		t.Fatalf("value just inside the limit should encode, got %v", err)
	}

	_, err := e.encode(nest(psobject.MaxJSONDepth + 1))
	if err == nil {
		t.Fatal("value past the limit encoded; it should be refused")
	}
	if !strings.Contains(err.Error(), "nests too deeply") {
		t.Errorf("error does not name the cause: %v", err)
	}
}

// TestEncodeRejectsDeepValuesWithoutOverflow uses a depth that does overflow
// the goroutine stack when the guard is absent - measured at just under four
// million levels, where NormalizeJSON exhausts the 1GB stack limit and the
// runtime aborts with "fatal error: stack overflow". Without this the guard
// could be removed and the cheaper tests above would still pass. The check is
// iterative and stops at the limit, so the test costs the allocation and
// nothing more.
func TestEncodeRejectsDeepValuesWithoutOverflow(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates several million values")
	}
	e := newEncoder(&Request{Compact: true})
	if _, err := e.encode(nest(5_000_000)); err == nil {
		t.Fatal("deeply nested value encoded; it should be refused")
	}
}

// TestEncodeNormalizesCmdletTypes pins that values a cmdlet may leak - a
// PSObject, a typed integer - are normalized rather than reaching gojq.Marshal,
// which would panic on them.
func TestEncodeNormalizesCmdletTypes(t *testing.T) {
	obj := psobject.NewPSObject("/tmp/x.txt")
	obj.AddNoteProperty("Handles", int32(7))

	e := newEncoder(&Request{Compact: true})
	got, err := e.encode(obj)
	if err != nil {
		t.Fatalf("encode PSObject: %v", err)
	}
	if !strings.Contains(got, `"Handles":7`) {
		t.Errorf("int32 note property not normalized: %s", got)
	}
}

// unencodable is a type NormalizeJSON turns into a string rather than a value
// gojq knows; the encoder must never panic on whatever it is handed.
type unencodable struct{ ch chan int }

func TestEncodeDoesNotPanicOnUnknownTypes(t *testing.T) {
	e := newEncoder(&Request{Compact: true})
	for _, v := range []any{
		unencodable{ch: make(chan int)},
		make(chan int),
		func() {},
		[]any{make(chan int)},
		map[string]any{"f": func() {}},
	} {
		// A result is fine and an error is fine; a panic is not.
		if _, err := e.encode(v); err != nil {
			t.Logf("%T -> %v", v, err)
		}
	}
}
