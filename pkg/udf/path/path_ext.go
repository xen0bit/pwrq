package path

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterNormalizePath registers normalize_path, a path with redundant
// separators and ".." resolved lexically, without touching the filesystem.
func RegisterNormalizePath() gojq.CompilerOption {
	return gojq.WithFunction("normalize_path", 0, 1, func(v any, args []any) any {
		p, err := pathInput(v, args, "normalize_path")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		return common.MakeUDFSuccessResult(filepath.Clean(p), nil)
	})
}

// RegisterRelativePath registers relative_path, target expressed relative to
// base: relative_path(base; [target]), or target from the pipeline.
func RegisterRelativePath() gojq.CompilerOption {
	return gojq.WithFunction("relative_path", 1, 2, func(v any, args []any) any {
		base, ok := common.BindValue(args[0]).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("relative_path: base must be a string, got %T", args[0]), nil)
		}
		var target string
		if len(args) > 1 {
			target, ok = common.BindValue(args[1]).(string)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("relative_path: target must be a string, got %T", args[1]), nil)
			}
		} else {
			p, err := pathInput(v, nil, "relative_path")
			if err != nil {
				return common.MakeUDFErrorResult(err, nil)
			}
			target = p
		}
		rel, err := filepath.Rel(base, target)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("relative_path: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(rel, nil)
	})
}

// RegisterStem registers stem, the file name without its extension:
// "/tmp/a/b.tar.gz" -> "b.tar".
func RegisterStem() gojq.CompilerOption {
	return gojq.WithFunction("stem", 0, 1, func(v any, args []any) any {
		p, err := pathInput(v, args, "stem")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		name := filepath.Base(p)
		ext := filepath.Ext(name)
		return common.MakeUDFSuccessResult(strings.TrimSuffix(name, ext), nil)
	})
}

// RegisterWithExtension registers with_extension, a file name with its
// extension replaced: with_extension(".md"; [input]) or from the pipeline.
func RegisterWithExtension() gojq.CompilerOption {
	return gojq.WithFunction("with_extension", 1, 2, func(v any, args []any) any {
		ext, ok := common.BindValue(args[0]).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("with_extension: extension must be a string, got %T", args[0]), nil)
		}
		if ext != "" && !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		var name string
		if len(args) > 1 {
			name, ok = common.BindValue(args[1]).(string)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("with_extension: input must be a string, got %T", args[1]), nil)
			}
		} else {
			p, err := pathInput(v, nil, "with_extension")
			if err != nil {
				return common.MakeUDFErrorResult(err, nil)
			}
			name = p
		}
		base := filepath.Base(name)
		stem := strings.TrimSuffix(base, filepath.Ext(base))
		return common.MakeUDFSuccessResult(filepath.Join(filepath.Dir(name), stem+ext), nil)
	})
}

// RegisterHasExtension registers has_extension, whether a path's final
// component carries an extension.
func RegisterHasExtension() gojq.CompilerOption {
	return gojq.WithFunction("has_extension", 0, 1, func(v any, args []any) any {
		p, err := pathInput(v, args, "has_extension")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		return common.MakeUDFSuccessResult(filepath.Ext(filepath.Base(p)) != "", nil)
	})
}

// RegisterIsDirPath registers is_dir_path, whether a path reads as a directory
// (trailing separator).
func RegisterIsDirPath() gojq.CompilerOption {
	return gojq.WithFunction("is_dir_path", 0, 1, func(v any, args []any) any {
		p, err := pathInput(v, args, "is_dir_path")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		return common.MakeUDFSuccessResult(strings.HasSuffix(p, string(filepath.Separator)), nil)
	})
}

// RegisterPathSep registers path_sep, the OS path separator as a string.
func RegisterPathSep() gojq.CompilerOption {
	return gojq.WithFunction("path_sep", 0, 0, func(v any, args []any) any {
		return common.MakeUDFSuccessResult(string(filepath.Separator), nil)
	})
}

// RegisterExpandHome registers expand_home, "~" and "~/..." expanded to the
// current user's home directory. It needs the user database, so it is CLI-only.
func RegisterExpandHome() gojq.CompilerOption {
	return gojq.WithFunction("expand_home", 0, 1, func(v any, args []any) any {
		p, err := pathInput(v, args, "expand_home")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("expand_home: cannot determine home directory: %v", err), nil)
		}
		switch {
		case p == "~":
			return common.MakeUDFSuccessResult(home, nil)
		case strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`):
			return common.MakeUDFSuccessResult(filepath.Join(home, p[2:]), nil)
		default:
			return common.MakeUDFSuccessResult(p, nil)
		}
	})
}

// RegisterHomeDir registers home_dir, the current user's home directory.
// CLI-only, like expand_home.
func RegisterHomeDir() gojq.CompilerOption {
	return gojq.WithFunction("home_dir", 0, 0, func(v any, args []any) any {
		home, err := os.UserHomeDir()
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("home_dir: cannot determine home directory: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(home, nil)
	})
}
