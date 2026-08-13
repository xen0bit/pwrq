// Package yaml converts between YAML and JSON-shaped values, for config files
// and the documents the page meets in the wild.
package yaml

import (
	"fmt"

	"github.com/itchyny/go-yaml"
	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterAll registers every YAML cmdlet.
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterYAMLParse(),
		RegisterYAMLStringify(),
	}
}

// RegisterYAMLParse registers yaml_parse, a YAML document to a JSON value.
func RegisterYAMLParse() gojq.CompilerOption {
	return common.WithFunction("yaml_parse", 0, 2, func(v any, args []any) any {
		inputVal, _, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("yaml_parse: %v", err), nil)
		}
		var input []byte
		switch val := common.BindValue(inputVal).(type) {
		case string:
			input = []byte(val)
		case []byte:
			input = val
		default:
			return common.MakeUDFErrorResult(fmt.Errorf("yaml_parse: expected a string, got %T", inputVal), nil)
		}
		var out any
		if err := yaml.Unmarshal(input, &out); err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("yaml_parse: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterYAMLStringify registers yaml_stringify, a value to a YAML document.
func RegisterYAMLStringify() gojq.CompilerOption {
	return common.WithFunction("yaml_stringify", 0, 1, func(v any, args []any) any {
		input := common.BindValue(v)
		if len(args) > 0 {
			input = common.BindValue(args[0])
		}
		out, err := yaml.Marshal(input)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("yaml_stringify: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(string(out), nil)
	})
}
