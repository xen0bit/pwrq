package common

import "fmt"

// SplitInput separates a cmdlet's explicit input from its operands.
//
// A cmdlet that takes `operands` operands after its input registers the arity
// range [operands, operands+1]. At the lower arity the input comes from the
// pipeline and every argument is an operand. At the upper arity — the explicit
// form — the first argument is the input and the operands follow it:
//
//	[1,2,3,4] | chunks(2)      // input from the pipeline
//	chunks([1,2,3,4]; 2)       // input as the leading argument
//
// The explicit form is always the maximum arity, which keeps it unambiguous for
// cmdlets whose last operand is itself optional: `summarize_by(key; column)`
// reads from the pipeline, and only `summarize_by(rows; key; column)` supplies
// an input.
//
// Binding is positional. The operands are never inspected to work out which one
// was "meant" to be the input, because guessing makes a genuine mistake — a key
// and an array passed the wrong way round — succeed quietly instead of failing.
func SplitInput(v any, args []any, operands int) (input any, rest []any) {
	if len(args) > operands {
		return args[0], args[1:]
	}
	return v, args
}

// BindArray resolves a value that must be an array, naming the cmdlet in any
// error so the message points at the call the user wrote.
func BindArray(v any, fn string) ([]any, error) {
	switch val := BindValue(v).(type) {
	case []any:
		return val, nil
	case nil:
		return nil, fmt.Errorf("%s: expected an array, got null", fn)
	default:
		return nil, fmt.Errorf("%s: expected an array, got %T", fn, val)
	}
}

// ArrayInput is SplitInput followed by BindArray, for the many collection
// cmdlets that operate on an array.
func ArrayInput(v any, args []any, operands int, fn string) ([]any, []any, error) {
	in, rest := SplitInput(v, args, operands)
	arr, err := BindArray(in, fn)
	if err != nil {
		return nil, nil, err
	}
	return arr, rest, nil
}
