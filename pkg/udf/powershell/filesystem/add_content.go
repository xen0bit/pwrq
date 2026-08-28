package filesystem

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/typed"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterAddContent registers add_content, PowerShell's Add-Content: the same
// write set_content performs, but appended rather than replacing the file.
//
// set_content truncates, which makes it useless for the thing people most want
// a write cmdlet for — accumulating lines into a log or a report across a run.
//
//	"a line" | add_content("out.log")
//	add_content("out.log"; "a line")
func RegisterAddContent() gojq.CompilerOption {
	return common.WithFunctionOf("add_content", 1, 3, WrittenFile, func(v any, args []any) any {
		opts, err := parseAppendArgs(v, args, "add_content")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		written, err := appendContent(opts)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("add_content: %v", err), map[string]any{"path": opts.Path})
		}
		return fileResult(written, "Add-Content")
	})
}

// RegisterOutFile registers out_file, PowerShell's Out-File: write a value to a
// file and pass it on, so a pipeline can record an intermediate result without
// being interrupted.
//
// tee already does this for the whole value in JSON. out_file writes text the
// way set_content does, and takes {Append: true}, which is the combination
// reporting pipelines actually need.
func RegisterOutFile() gojq.CompilerOption {
	return common.WithFunctionOf("out_file", 1, 3, WrittenFile, func(v any, args []any) any {
		opts, err := parseAppendArgs(v, args, "out_file")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		if opts.Append {
			if _, err := appendContent(opts); err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("out_file: %v", err), map[string]any{"path": opts.Path})
			}
		} else if _, err := setContent(opts.SetContentOptions); err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("out_file: %v", err), map[string]any{"path": opts.Path})
		}
		// Out-File is a pass-through: the value carries on down the pipeline.
		return common.MakeUDFSuccessResult(common.BindValue(v), nil)
	})
}

// appendOptions is SetContentOptions plus the append switch.
type appendOptions struct {
	SetContentOptions
	Append bool
}

// parseAppendArgs binds the path and the value. The value comes from the
// pipeline unless an explicit one is supplied, which is what lets
// `"line" | add_content("f")` and `add_content("f"; "line")` both read
// naturally.
func parseAppendArgs(v any, args []any, fn string) (appendOptions, error) {
	o := appendOptions{SetContentOptions: SetContentOptions{Encoding: "utf8"}}
	path, err := common.BindString(args[0], "path")
	if err != nil {
		return o, fmt.Errorf("%s: %v", fn, err)
	}
	o.Path = path

	rest := args[1:]
	o.Value = common.BindValue(v)
	if len(rest) > 0 {
		if m, ok := common.BindValue(rest[0]).(map[string]any); ok {
			if err := applyAppendOptions(&o, m, fn); err != nil {
				return o, err
			}
		} else {
			o.Value = common.BindValue(rest[0])
			rest = rest[1:]
			if len(rest) > 0 {
				m, ok := common.BindValue(rest[0]).(map[string]any)
				if !ok {
					return o, fmt.Errorf("%s: options must be an object, got %T", fn, common.BindValue(rest[0]))
				}
				if err := applyAppendOptions(&o, m, fn); err != nil {
					return o, err
				}
			}
		}
	}
	if o.Value == nil {
		return o, fmt.Errorf("%s: nothing to write; pipe a value in or pass one as an argument", fn)
	}
	return o, nil
}

func applyAppendOptions(o *appendOptions, m map[string]any, fn string) error {
	for k, val := range m {
		switch lowerASCII(k) {
		case "encoding":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("%s: Encoding must be a string", fn)
			}
			o.Encoding = s
		case "force":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("%s: Force must be a boolean", fn)
			}
			o.Force = b
		case "append":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("%s: Append must be a boolean", fn)
			}
			o.Append = b
		case "value":
			o.Value = val
		default:
			return fmt.Errorf("%s: unknown option %q", fn, k)
		}
	}
	return nil
}

// appendContent writes to the end of the file, creating it if absent. It goes
// through the same encoding and line-ending conversion set_content uses, so a
// file built by appending matches one written in a single call.
func appendContent(o appendOptions) (string, error) {
	path, err := filepath.Abs(o.Path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	if err := validatePath(path); err != nil {
		return "", err
	}
	content := renderContent(o.Value)
	enc, err := getEncoding(o.Encoding)
	if err != nil {
		return "", err
	}
	newline := getNewLine()
	// A newline terminates each append, matching Add-Content: successive calls
	// add lines rather than running into one another.
	//
	// The leading one is for the file we are appending to. set_content writes
	// no trailing newline, so appending to a file it produced would otherwise
	// splice the two values together into one line.
	prefix := ""
	if needsSeparator(path) {
		prefix = newline
	}
	encoded, err := convertToEncoding(prefix+content+newline, enc)
	if err != nil {
		return "", err
	}
	if o.Force {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", fmt.Errorf("failed to create parent directory: %w", err)
		}
	}
	if _, err := os.Stat(path); err == nil && isReadOnly(path) {
		if !o.Force {
			return "", fmt.Errorf("access denied: %s is read-only (use Force to override)", o.Path)
		}
		if err := os.Chmod(path, 0o644); err != nil {
			return "", fmt.Errorf("failed to remove read-only attribute: %w", err)
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(encoded); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}
	return path, nil
}

// needsSeparator reports whether the file has content that does not end in a
// newline, and so needs one before more is appended.
func needsSeparator(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || fi.Size() == 0 {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	last := make([]byte, 1)
	if _, err := f.ReadAt(last, fi.Size()-1); err != nil {
		return false
	}
	return last[0] != '\n'
}

func fileResult(path, operation string) any {
	var size int64
	if fi, err := os.Stat(path); err == nil {
		size = fi.Size()
	}
	obj := typed.New(path)
	obj.AddNoteProperty("Path", path)
	obj.AddNoteProperty("Length", size)
	obj.AddNoteProperty("Exists", true)
	obj.AddNoteProperty("Operation", operation)
	return WrittenFile.Build(obj.ToMap())
}

func lowerASCII(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}
