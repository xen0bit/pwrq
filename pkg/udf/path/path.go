// Package path provides filesystem-path utilities that need no filesystem:
// they are pure string operations, so they work in the browser too.
package path

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterAll registers every path cmdlet.
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterBasename(),
		RegisterDirname(),
		RegisterFileExtension(),
		RegisterIsAbsolute(),
	}
}

// pathInput resolves the path from the first argument or the pipeline.
func pathInput(v any, args []any, name string) (string, error) {
	if len(args) > 0 {
		if s, ok := common.BindValue(args[0]).(string); ok {
			return s, nil
		}
	}
	switch val := common.BindValue(v).(type) {
	case string:
		return val, nil
	case []byte:
		return string(val), nil
	default:
		return "", fmt.Errorf("%s: expected a path string, got %T", name, v)
	}
}

// RegisterBasename registers basename, the last path component.
func RegisterBasename() gojq.CompilerOption {
	return gojq.WithFunction("basename", 0, 1, func(v any, args []any) any {
		p, err := pathInput(v, args, "basename")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		return common.MakeUDFSuccessResult(filepath.Base(strings.TrimRight(p, "/")), nil)
	})
}

// RegisterDirname registers dirname, the path minus the last component.
func RegisterDirname() gojq.CompilerOption {
	return gojq.WithFunction("dirname", 0, 1, func(v any, args []any) any {
		p, err := pathInput(v, args, "dirname")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		return common.MakeUDFSuccessResult(filepath.Dir(strings.TrimRight(p, "/")), nil)
	})
}

// RegisterFileExtension registers file_extension, the suffix after the final
// dot (".txt"), or the empty string when there is none.
func RegisterFileExtension() gojq.CompilerOption {
	return gojq.WithFunction("file_extension", 0, 1, func(v any, args []any) any {
		p, err := pathInput(v, args, "file_extension")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		return common.MakeUDFSuccessResult(filepath.Ext(filepath.Base(p)), nil)
	})
}

// RegisterIsAbsolute registers is_absolute, whether a path is absolute.
func RegisterIsAbsolute() gojq.CompilerOption {
	return gojq.WithFunction("is_absolute", 0, 1, func(v any, args []any) any {
		p, err := pathInput(v, args, "is_absolute")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		return common.MakeUDFSuccessResult(filepath.IsAbs(p), nil)
	})
}
