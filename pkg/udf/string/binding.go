// Input binding and the registration helpers the string cmdlets share.
package string

import (
	"fmt"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// strFromPipeline resolves a string from the pipeline value, naming the cmdlet
// in any error so the message points at the call the user wrote.
func strFromPipeline(v any, name string) (string, error) {
	input, err := stringInput(common.BindValue(v))
	if err != nil {
		return "", fmt.Errorf("%s: %v", name, err)
	}
	return input, nil
}

// stringInput coerces the bound pipeline value to a string, the way the string
// cmdlets' switch does everywhere else in this package.
func stringInput(inputVal any) (string, error) {
	switch val := inputVal.(type) {
	case string:
		return val, nil
	case []byte:
		return string(val), nil
	default:
		if str, ok := val.(fmt.Stringer); ok {
			return str.String(), nil
		}
		return "", fmt.Errorf("argument must be a string, got %T", val)
	}
}

// bindInput resolves the pipeline value, honouring the file flag the way
// ParseFileArgs does for the single-input cmdlets.
func bindInput(v any, isFile bool) (any, error) {
	if !isFile {
		return common.BindValue(v), nil
	}
	path, ok := common.BindPath(v)
	if !ok {
		return nil, fmt.Errorf("file argument requires a string path, got %T", v)
	}
	data, _, _, err := common.ReadFileFromPath(path)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

// registerTextFn registers a 0-2 arity string-in, value-out cmdlet.
func registerTextFn(name string, fn func(string) any) gojq.CompilerOption {
	return gojq.WithFunction(name, 0, 2, func(v any, args []any) any {
		inputVal, _, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: %v", name, err), nil)
		}
		input, err := stringInput(common.BindValue(inputVal))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: %v", name, err), nil)
		}
		return common.MakeUDFSuccessResult(fn(input), nil)
	})
}

// registerPredicate registers a 0-2 arity string-in, boolean-out cmdlet.
func registerPredicate(name string, fn func(string) bool) gojq.CompilerOption {
	return gojq.WithFunction(name, 0, 2, func(v any, args []any) any {
		inputVal, _, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: %v", name, err), nil)
		}
		input, err := stringInput(common.BindValue(inputVal))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: %v", name, err), nil)
		}
		return common.MakeUDFSuccessResult(fn(input), nil)
	})
}

// registerCaseConverter builds a 0-2 arity string-in, string-out cmdlet that
// applies transform to the pipeline input, mirroring upper/lower.
func registerCaseConverter(name string, transform func(string) string) gojq.CompilerOption {
	return gojq.WithFunction(name, 0, 2, func(v any, args []any) any {
		inputVal, _, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: %v", name, err), nil)
		}
		input, err := stringInput(common.BindValue(inputVal))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: %v", name, err), nil)
		}
		return common.MakeUDFSuccessResult(transform(input), nil)
	})
}
