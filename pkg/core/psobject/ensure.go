package psobject

import (
	"encoding/json"
	"errors"
	"math/big"
	"reflect"
)

// MaxJSONDepth bounds how deeply a value may nest before this package refuses
// to walk it.
//
// NormalizeJSON recurses, and so does gojq's own marshaller. A value nested
// just under four million levels exhausts the 1GB goroutine stack, and a stack
// overflow is a fatal runtime error that no recover() can catch: it takes the
// whole process down, which for a server shared by many callers is the worst
// possible failure. encoding/json draws the same line at 10000, comfortably
// inside every recursion budget here.
const MaxJSONDepth = 10000

// ErrTooDeep reports a value nested past MaxJSONDepth. EnsureJSON returns it
// alongside the value untouched: normalizing such a value would recurse deeply
// enough to overflow the stack, which is the very failure being defended
// against, so the caller has to decide - a cmdlet passes it along, the encoder
// refuses it as a query error.
var ErrTooDeep = errors.New("value nests too deeply to encode")

// EnsureJSON returns v in gojq's value space, converting only what has to be
// converted.
//
// gojq operates on nil, bool, int, float64, *big.Int, json.Number, string,
// []any and map[string]any, and it *panics* on anything else - not only when
// marshalling, but inside any builtin that inspects the value. A cmdlet that
// hands back an int32, a time.Duration or a *PSObject therefore arms a crash
// for whichever query touches that value first. Passing every cmdlet result
// through here disarms the whole class.
//
// The common case is a result that is already clean, so EnsureJSON checks
// before it converts and returns the original value untouched when it can:
// four times faster and three orders of magnitude fewer allocations than
// NormalizeJSON, which deep-copies every container it walks.
func EnsureJSON(v any) (any, error) {
	clean, tooDeep := inspectJSON(v)
	switch {
	case tooDeep:
		return v, ErrTooDeep
	case clean:
		return v, nil
	default:
		return NormalizeJSON(v), nil
	}
}

// isJSONScalar reports whether v is a leaf gojq already understands.
func isJSONScalar(v any) bool {
	switch v.(type) {
	case nil, bool, int, float64, string, *big.Int, json.Number:
		return true
	}
	return false
}

// inspectJSON reports whether v is already entirely within gojq's value space,
// and whether it nests past MaxJSONDepth.
//
// It walks with an explicit stack rather than recursing, so the check itself
// cannot overflow on the very input it exists to catch. Scalars are tested in
// place instead of being pushed, so a flat array of a million numbers costs no
// stack growth at all - only containers and values needing conversion are.
//
// Finding a value that needs conversion does not end the walk. The depth answer
// has to be complete before NormalizeJSON is allowed to run on it: a value that
// is both dirty near the top and very deep somewhere else would otherwise be
// reported as merely dirty, and normalizing it would overflow. The descent
// mirrors NormalizeJSON's, reflection fallback included, so the depth measured
// here is the depth NormalizeJSON would actually recurse to.
func inspectJSON(v any) (clean, tooDeep bool) {
	if isJSONScalar(v) {
		return true, false
	}

	type item struct {
		v     any
		depth int
	}
	stack := make([]item, 0, 16)
	stack = append(stack, item{v: v, depth: 1})
	clean = true

	push := func(e any, depth int) {
		if isJSONScalar(e) {
			return
		}
		stack = append(stack, item{v: e, depth: depth})
	}

	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if cur.depth > MaxJSONDepth {
			return false, true
		}

		switch x := cur.v.(type) {
		case []any:
			for _, e := range x {
				push(e, cur.depth+1)
			}
		case map[string]any:
			for _, e := range x {
				push(e, cur.depth+1)
			}
		case *PSObject:
			clean = false
			push(x.Value, cur.depth+1)
			for _, m := range x.Members {
				push(m.Value, cur.depth+1)
			}
		default:
			clean = false
			rv := reflect.ValueOf(cur.v)
			if !rv.IsValid() {
				continue
			}
			switch rv.Kind() {
			case reflect.Pointer, reflect.Interface:
				if !rv.IsNil() {
					push(rv.Elem().Interface(), cur.depth+1)
				}
			case reflect.Slice, reflect.Array:
				for i := range rv.Len() {
					push(rv.Index(i).Interface(), cur.depth+1)
				}
			case reflect.Map:
				for iter := rv.MapRange(); iter.Next(); {
					push(iter.Value().Interface(), cur.depth+1)
				}
			}
		}
	}
	return clean, false
}
