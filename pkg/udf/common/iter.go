package common

import "github.com/itchyny/gojq"

// SliceIter yields the values of a slice, in order.
//
// gojq.NewIter is variadic over a single element type, so a []any of mixed
// PSObjects cannot be handed to it without a spread that loses the type. Every
// streaming cmdlet that had already computed its results therefore carried its
// own three-line iterator; this is that iterator, written once.
//
// A cmdlet that can produce its results lazily should not use this: buffering
// the whole answer first is exactly what streaming is meant to avoid. Use it
// where the work is already done, such as reading a process table.
func SliceIter(values []any) gojq.Iter {
	return &sliceIter{values: values}
}

type sliceIter struct {
	values []any
	index  int
}

func (it *sliceIter) Next() (any, bool) {
	if it.index >= len(it.values) {
		return nil, false
	}
	v := it.values[it.index]
	it.index++
	return v, true
}
