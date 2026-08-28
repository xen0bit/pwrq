package shape

import (
	"strings"
	"testing"

	"github.com/xen0bit/pwrq/pkg/core/psobject"
)

func TestObserverDescribesObjectKeys(t *testing.T) {
	o := NewObserver()
	o.Add(map[string]any{"Name": "a", "Length": 1})
	o.Add(map[string]any{"Name": "b", "Length": 2})

	got := o.Describe()
	if !strings.Contains(got, "2 values, each an object") {
		t.Errorf("Describe = %q", got)
	}
	if !strings.Contains(got, "Length(number)") || !strings.Contains(got, "Name(string)") {
		t.Errorf("Describe did not report the keys and their types: %q", got)
	}
}

// TestObserverMarksPartialKeys is the distinction a caller has to act on: a key
// on every result can be relied on, and one on some of them has to be guarded.
func TestObserverMarksPartialKeys(t *testing.T) {
	o := NewObserver()
	o.Add(map[string]any{"Always": 1, "Sometimes": 2})
	o.Add(map[string]any{"Always": 1})

	got := o.Describe()
	if !strings.Contains(got, "Sometimes(number, on 1/2)") {
		t.Errorf("Describe did not mark the partial key: %q", got)
	}
	if strings.Contains(got, "Always(number, on") {
		t.Errorf("Describe marked a key that was on every result: %q", got)
	}
}

func TestObserverReportsTheTypeName(t *testing.T) {
	o := NewObserver()
	o.Add(map[string]any{"Name": "a", psobject.PSTypeNameKey: "System.IO.FileInfo"})

	got := o.Describe()
	if !strings.Contains(got, "[System.IO.FileInfo]") {
		t.Errorf("Describe did not name the type: %q", got)
	}
	// The type name is metadata about the object, not one of its keys.
	if strings.Contains(got, "PSTypeName(") {
		t.Errorf("Describe listed PSTypeName as a key: %q", got)
	}
}

func TestObserverNamesAMixedStream(t *testing.T) {
	o := NewObserver()
	o.Add("a")
	o.Add(1)
	o.Add(map[string]any{"k": 1})

	got := o.Describe()
	if !strings.Contains(got, "mixed:") {
		t.Errorf("Describe did not report the stream as mixed: %q", got)
	}
}

// TestObserverIsDeterministic guards the reason describeProps sorts: Add walks
// a map, and Go randomises map iteration, so an order derived from it would
// differ between runs.
func TestObserverIsDeterministic(t *testing.T) {
	describe := func() string {
		o := NewObserver()
		for i := 0; i < 3; i++ {
			o.Add(map[string]any{"z": 1, "a": 2, "m": 3, "b": 4, "y": 5})
		}
		return o.Describe()
	}

	first := describe()
	for i := 0; i < 20; i++ {
		if got := describe(); got != first {
			t.Fatalf("Describe is not stable across runs:\n %q\n %q", first, got)
		}
	}
	if !strings.Contains(first, "keys: a(number) b(number) m(number) y(number) z(number)") {
		t.Errorf("keys are not in name order: %q", first)
	}
}

// TestObserverNotableSkipsPlainScalars keeps the shape line off output the
// caller can already read. A handful of numbers describe themselves.
func TestObserverNotableSkipsPlainScalars(t *testing.T) {
	scalars := NewObserver()
	scalars.Add(1)
	scalars.Add(2)
	if scalars.Notable(false) {
		t.Error("a run of plain scalars was reported as worth describing")
	}
	// Unless the run was cut short, in which case what the rest looked like is
	// exactly what the caller cannot see.
	if !scalars.Notable(true) {
		t.Error("a truncated run was not reported as worth describing")
	}

	objects := NewObserver()
	objects.Add(map[string]any{"k": 1})
	if !objects.Notable(false) {
		t.Error("a run of objects was not reported as worth describing")
	}

	empty := NewObserver()
	if empty.Notable(true) {
		t.Error("a run that produced nothing was reported as worth describing")
	}
}

// TestObserverCapsTheKeysItTracks pins the bound. A Derived cmdlet over wild
// data can emit thousands of distinct keys, and an observer that grew with them
// would be a memory leak wearing a diagnostic's clothes.
func TestObserverCapsTheKeysItTracks(t *testing.T) {
	o := NewObserver()
	wide := make(map[string]any, maxObservedProps*2)
	for i := 0; i < maxObservedProps*2; i++ {
		wide[strings.Repeat("k", i+1)] = i
	}
	o.Add(wide)

	if len(o.props) > maxObservedProps {
		t.Errorf("tracked %d keys, want at most %d", len(o.props), maxObservedProps)
	}
	if !strings.Contains(o.Describe(), "not listed") {
		t.Errorf("Describe did not say the list was cut short: %q", o.Describe())
	}
}

func TestNilObserverIsInert(t *testing.T) {
	var o *Observer
	o.Add(map[string]any{"k": 1})
	if o.Describe() != "" || o.Count() != 0 || o.Notable(true) {
		t.Error("a nil observer was not inert")
	}
}
