package common

import (
	"sync"

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
	recordEmission(name, false)
	return gojq.WithFunction(name, minArity, maxArity, func(v any, args []any) any {
		return normalizeResult(f(v, args))
	})
}

// WithIterFunction registers a streaming cmdlet, normalizing each value it
// yields. It is a drop-in replacement for gojq.WithIterFunction.
func WithIterFunction(name string, minArity, maxArity int, f func(any, []any) gojq.Iter) gojq.CompilerOption {
	recordEmission(name, true)
	return gojq.WithIterFunction(name, minArity, maxArity, func(v any, args []any) gojq.Iter {
		inner := f(v, args)
		if inner == nil {
			return gojq.NewIter[any]()
		}
		return &normalizingIter{inner: inner}
	})
}

// Whether a cmdlet emits one value or a stream of them is the single fact
// callers most often get wrong: it decides whether a query needs to collect
// with [...] or must not. It is also the fact most likely to rot in a
// hand-written table, because nothing forces the table to agree with the code.
//
// So it is not written down anywhere. Choosing WithIterFunction over
// WithFunction *is* the declaration, and it is recorded here as the cmdlet is
// registered. get_help reports what these wrappers observed, which means the
// documentation cannot disagree with the behaviour.
var (
	emissionMu   sync.RWMutex
	streamingUDF = make(map[string]bool)
)

func recordEmission(name string, streaming bool) {
	emissionMu.Lock()
	defer emissionMu.Unlock()
	streamingUDF[name] = streaming
}

// IsStreaming reports whether the named cmdlet emits a stream of values rather
// than a single one, and whether it was registered through these wrappers at
// all.
func IsStreaming(name string) (streaming, known bool) {
	emissionMu.RLock()
	defer emissionMu.RUnlock()
	streaming, known = streamingUDF[name]
	return streaming, known
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
