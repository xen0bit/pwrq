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

// ObjectInput is SplitInput followed by NormalizeToSlice over BindObjectInput,
// for the object cmdlets that accept one object or an array of them and must
// not collapse a cmdlet's scalar-valued object to that scalar.
//
// One allowance beyond SplitInput: a single trailing argument that is the data
// rather than an operand - a non-nil array or object, but never a string (that
// is a property name) or a map (that is options) - becomes the input when the
// pipeline supplied none. That is what keeps the explicit one-argument form
// (`measure_object(ROWS)`) working now that the cmdlets also read the pipeline.
func ObjectInput(v any, args []any, operands int) ([]any, []any) {
	in, rest := SplitInput(v, args, operands)
	if len(rest) == 1 && rest[0] != nil {
		objects := NormalizeToSlice(BindObjectInput(in))
		switch rest[0].(type) {
		case map[string]any, string:
			// options, or a property name: not the data
		default:
			if len(objects) == 0 {
				in, rest = rest[0], nil
			}
		}
	}
	return NormalizeToSlice(BindObjectInput(in)), rest
}
