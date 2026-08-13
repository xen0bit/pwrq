package common

import (
	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/psobject"
)

// This file is the boundary between cmdlet code and the query engine.
//
// gojq's value space is nil, bool, int, float64, *big.Int, json.Number, string,
// []any and map[string]any. gojq panics - it does not error - on anything else,
// and not only when encoding: `type`, arithmetic, comparison and most other
// builtins panic the moment they inspect such a value. A cmdlet that returns an
// int32 in a map therefore plants a crash that goes off whenever some later
// query happens to touch that field, which is a long way from where the mistake
// was made.
//
// Every cmdlet is registered through the wrappers below, so its results are
// normalized on the way out and the mistake stops being possible. Registering
// with gojq.WithFunction directly bypasses this; use these instead.

// WithFunction registers a cmdlet, normalizing whatever it returns into gojq's
// value space. It is a drop-in replacement for gojq.WithFunction.
func WithFunction(name string, minArity, maxArity int, f func(any, []any) any) gojq.CompilerOption {
	return gojq.WithFunction(name, minArity, maxArity, func(v any, args []any) any {
		return normalizeResult(f(v, args))
	})
}

// WithIterFunction registers a streaming cmdlet, normalizing each value it
// yields. It is a drop-in replacement for gojq.WithIterFunction.
func WithIterFunction(name string, minArity, maxArity int, f func(any, []any) gojq.Iter) gojq.CompilerOption {
	return gojq.WithIterFunction(name, minArity, maxArity, func(v any, args []any) gojq.Iter {
		inner := f(v, args)
		if inner == nil {
			return gojq.NewIter[any]()
		}
		return &normalizingIter{inner: inner}
	})
}

// normalizeResult puts one cmdlet result into gojq's value space.
func normalizeResult(v any) any {
	// An error is how a cmdlet reports failure: gojq routes it to jq's error
	// channel, where try/catch and the exit status can see it. NormalizeJSON
	// would turn it into its message string, so a failure would come back as
	// an ordinary successful value.
	if _, isErr := v.(error); isErr {
		return v
	}
	// A value too deep to normalize is passed through untouched rather than
	// dropped: the encoder refuses it with a query error, which is a better
	// answer than a silently mangled result.
	normalized, _ := psobject.EnsureJSON(v)
	return normalized
}

// normalizingIter normalizes each value a streaming cmdlet yields.
type normalizingIter struct {
	inner gojq.Iter
}

func (it *normalizingIter) Next() (any, bool) {
	v, ok := it.inner.Next()
	if !ok {
		return nil, false
	}
	return normalizeResult(v), true
}
