package rncd

import (
	"fmt"

	"github.com/xen0bit/pwrq/pkg/core/typed"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// contentProperties are the properties a named value carries its bytes under,
// in binding order. This is PowerShell's ByPropertyName binding: a bare string
// binds by value, and an object binds by the property that holds the payload.
var contentProperties = []string{"Content", "Bytes", "Data"}

// nameProperties are the properties a named value is labelled by. Any of them
// identifies the value in the output, which is what lets a corpus assembled
// from files report paths rather than array offsets.
var nameProperties = []string{"Name", "PwrqValue", "FullName", "Path"}

// bindBytes casts a pipeline value to the bytes to measure.
//
// A string is its bytes. Go strings are byte strings, so a value read with cat
// keeps every byte a UTF-8 encoder would have replaced, and the measurements
// see the file rather than a sanitized version of it.
func bindBytes(v any) ([]byte, bool) {
	switch val := common.BindValue(v).(type) {
	case string:
		return []byte(val), true
	case []byte:
		return val, true
	case map[string]any:
		for _, name := range contentProperties {
			if got, ok := val[name]; ok {
				// One level only. The content property holds the payload; if
				// it holds another object, that is a mistake worth reporting
				// rather than a structure worth chasing.
				if b, ok := bindScalarBytes(got); ok {
					return b, true
				}
			}
		}
	default:
		return bindScalarBytes(val)
	}
	return nil, false
}

func bindScalarBytes(v any) ([]byte, bool) {
	switch val := common.BindValue(v).(type) {
	case string:
		return []byte(val), true
	case []byte:
		return val, true
	case fmt.Stringer:
		return []byte(val.String()), true
	}
	return nil, false
}

// bindName reads the label a value travels with, or "" if it has none.
func bindName(v any) string {
	switch val := v.(type) {
	case *typed.Object:
		for _, name := range nameProperties {
			if got, err := val.GetPropertyValue(name); err == nil {
				if s, ok := got.(string); ok && s != "" {
					return s
				}
			}
		}
	case map[string]any:
		for _, name := range nameProperties {
			if s, ok := val[name].(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// describe names an input's type in jq's own vocabulary, so the error points
// at the value the caller wrote rather than at a Go type they never mentioned.
func describe(v any) string {
	bound := common.BindValue(v)
	switch bound.(type) {
	case nil:
		return "null"
	case map[string]any:
		return fmt.Sprintf("an object with no %s property", contentProperties[0])
	case []any:
		return "an array"
	case bool:
		return "a boolean"
	}
	if _, ok := common.ToFloat64(bound); ok {
		return "a number"
	}
	return fmt.Sprintf("a %T", bound)
}
