package filesystem

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/typed"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// parseJoinPathArgs parses and validates arguments for join_path.
// Returns the list of path strings to join and an optional resolve flag.
func parseJoinPathArgs(args []any) ([]string, bool, error) {
	if len(args) == 0 {
		return nil, false, fmt.Errorf("join_path: requires at least one path argument")
	}

	// Check for -Resolve parameter (last argument if it's a bool true)
	resolve := false
	pathArgs := args

	// Handle named parameter: if last arg is a bool, treat as -Resolve flag
	if len(args) > 0 {
		if resolveFlag, ok := args[len(args)-1].(bool); ok {
			resolve = resolveFlag
			pathArgs = args[:len(args)-1]
		}
	}

	if len(pathArgs) == 0 {
		return nil, false, fmt.Errorf("join_path: requires at least one path argument")
	}

	paths := make([]string, 0, len(pathArgs))
	for i, arg := range pathArgs {
		// Extract value from object if wrapped
		val := common.BindValue(arg)

		// Validate and convert to string
		if val == nil {
			return nil, false, fmt.Errorf("join_path: argument %d is null", i+1)
		}

		pathStr, ok := val.(string)
		if !ok {
			return nil, false, fmt.Errorf("join_path: argument %d must be a string path, got %T", i+1, val)
		}

		if pathStr == "" {
			return nil, false, fmt.Errorf("join_path: argument %d is an empty string", i+1)
		}

		paths = append(paths, pathStr)
	}

	return paths, resolve, nil
}

// joinPath performs path joining with proper handling of edge cases.
// It normalizes separators, handles UNC paths, and optionally resolves to absolute paths.
func joinPath(paths []string, resolve bool) (string, error) {
	if len(paths) == 0 {
		return "", fmt.Errorf("join_path: no paths provided")
	}

	// Normalize path separators (convert / to \ on Windows-style paths)
	normalized := make([]string, len(paths))
	for i, p := range paths {
		// Normalize to OS-specific separator
		normalized[i] = filepath.FromSlash(p)
	}

	// Build the joined path
	result := normalized[0]
	for i := 1; i < len(normalized); i++ {
		// If the next path is absolute, it replaces the current result
		if filepath.IsAbs(normalized[i]) {
			result = normalized[i]
		} else {
			result = filepath.Join(result, normalized[i])
		}
	}

	// Clean the path to resolve . and .. components
	result = filepath.Clean(result)

	// Resolve to absolute path if requested
	if resolve {
		absPath, err := filepath.Abs(result)
		if err != nil {
			return "", fmt.Errorf("join_path: failed to resolve absolute path: %w", err)
		}
		result = absPath
	}

	return result, nil
}

// RegisterJoinPath registers the join_path function with gojq.
// Signature: join_path(path1, path2, ..., resolve?: bool)
// Returns a typed object with the joined path and metadata.
func RegisterJoinPath() gojq.CompilerOption {
	return common.WithFunctionOf("join_path", 1, 10, JoinedPath, func(v any, args []any) any {
		// Combine pipeline input with arguments
		allArgs := args
		if v != nil {
			// If there's pipeline input, prepend it
			inputVal := common.BindValue(v)
			if inputStr, ok := inputVal.(string); ok && inputStr != "" {
				allArgs = append([]any{inputStr}, args...)
			}
		}

		// Parse arguments
		paths, resolve, err := parseJoinPathArgs(allArgs)
		if err != nil {
			return common.MakeUDFErrorResult(err, map[string]any{
				"function": "join_path",
				"args":     args,
			})
		}

		// Perform the join
		joinedPath, err := joinPath(paths, resolve)
		if err != nil {
			return common.MakeUDFErrorResult(err, map[string]any{
				"function": "join_path",
				"paths":    paths,
				"resolve":  resolve,
			})
		}

		// Create object result with full metadata
		obj := typed.New(joinedPath)

		// Add useful path properties as NoteProperties
		obj.AddNoteProperty("Path", joinedPath)
		obj.AddNoteProperty("IsAbsolute", filepath.IsAbs(joinedPath))
		obj.AddNoteProperty("IsUnc", strings.HasPrefix(joinedPath, `\\`))

		// Add split components for convenience
		obj.AddNoteProperty("DirectoryName", filepath.Dir(joinedPath))
		obj.AddNoteProperty("FileName", filepath.Base(joinedPath))
		obj.AddNoteProperty("Extension", filepath.Ext(joinedPath))

		// Extract drive letter if present (Windows)
		drive := ""
		if len(joinedPath) >= 2 && joinedPath[1] == ':' {
			drive = joinedPath[:2]
		}
		obj.AddNoteProperty("Drive", drive)

		// Hand the object to MakeUDFSuccessResult rather than returning it
		// raw: it normalizes to the wire form. A raw *typed.Object is not
		// in gojq's value space, so any filter that touched it - and the
		// encoder in the end - would panic.
		return common.MakeUDFSuccessResult(JoinedPath.Build(obj.ToMap()), nil)
	})
}

// RegisterSplitPath registers the split_path function with gojq.
// Signature: split_path(path?: string)
// Returns a typed object with split path components.
func RegisterSplitPath() gojq.CompilerOption {
	return common.WithFunctionOf("split_path", 0, 2, SplitPath, func(v any, args []any) any {
		var path string

		// Get path from argument or pipeline input
		if len(args) > 0 {
			argVal := common.BindValue(args[0])
			if p, ok := argVal.(string); ok {
				path = p
			} else {
				return common.MakeUDFErrorResult(
					fmt.Errorf("split_path: argument must be a string path, got %T", argVal),
					map[string]any{"function": "split_path"},
				)
			}
		} else {
			inputVal := common.BindValue(v)
			if p, ok := inputVal.(string); ok {
				path = p
			} else {
				return common.MakeUDFErrorResult(
					fmt.Errorf("split_path: input must be a string path, got %T", inputVal),
					map[string]any{"function": "split_path"},
				)
			}
		}

		// Split the path
		dir := filepath.Dir(path)
		base := filepath.Base(path)
		ext := filepath.Ext(path)
		name := strings.TrimSuffix(base, ext)

		// Create object with split components
		obj := typed.New(path)
		obj.AddNoteProperty("Path", path)
		obj.AddNoteProperty("DirectoryName", dir)
		obj.AddNoteProperty("Name", name)
		obj.AddNoteProperty("Extension", ext)
		obj.AddNoteProperty("BaseName", base)
		obj.AddNoteProperty("IsAbsolute", filepath.IsAbs(path))

		// Add drive information (for Windows compatibility)
		drive := ""
		if len(path) >= 2 && path[1] == ':' {
			drive = path[:2]
		}
		obj.AddNoteProperty("Drive", drive)

		return common.MakeUDFSuccessResult(SplitPath.Build(obj.ToMap()), nil)
	})
}
