package find

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// FindOptions represents options for the find function
type FindOptions struct {
	Path     string
	Type     string // "file", "dir", or "" for both
	MaxDepth int    // -1 for unlimited
	MinDepth int    // minimum depth (default 0)
}

// findFiles performs the actual file finding
func findFiles(opts FindOptions) ([]any, error) {
	var results []any

	// Convert starting path to absolute
	startPath, err := filepath.Abs(opts.Path)
	if err != nil {
		return nil, fmt.Errorf("find: cannot resolve path %q: %v", opts.Path, err)
	}

	// Check if path exists
	_, err = os.Stat(startPath)
	if err != nil {
		return nil, fmt.Errorf("find: path %q does not exist: %v", startPath, err)
	}

	err = filepath.Walk(startPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Skip permission errors and continue
			if os.IsPermission(err) {
				return nil
			}
			return err
		}

		// Calculate depth relative to the starting path
		relPath, err := filepath.Rel(startPath, path)
		if err != nil {
			return nil
		}

		// Skip the root path itself if it's a file and we're looking for files
		if relPath == "." {
			if opts.Type == "file" && info.IsDir() {
				return nil
			}
			if opts.Type == "dir" && !info.IsDir() {
				return nil
			}
		}

		depth := 0
		if relPath != "." {
			depth = len(strings.Split(relPath, string(filepath.Separator)))
		}

		// Check min depth
		if depth < opts.MinDepth {
			if info.IsDir() {
				return nil
			}
			return nil
		}

		// Check max depth
		if opts.MaxDepth >= 0 && depth > opts.MaxDepth {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Check type filter
		if opts.Type == "file" && info.IsDir() {
			return nil
		}
		if opts.Type == "dir" && !info.IsDir() {
			return nil
		}
		
		// Path from Walk is already absolute (since startPath is absolute)
		// But ensure it's normalized
		absPath := filepath.Clean(path)
		
		// Determine type for metadata
		pathType := "file"
		if info.IsDir() {
			pathType = "dir"
		}
		
		// Return object with _val and _meta keys
		result := map[string]any{
			"_val": absPath,
			"_meta": map[string]any{
				"type": pathType,
			},
		}
		
		results = append(results, result)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return results, nil
}

// parseFindArgs parses arguments to the find function
func parseFindArgs(args []any) (FindOptions, error) {
	opts := FindOptions{
		Type:     "",
		MaxDepth: -1,
		MinDepth: 0,
	}

	if len(args) == 0 {
		return opts, fmt.Errorf("find: expected at least 1 argument (path)")
	}
	
	// Extract _val from UDF result objects (standard behavior for all UDFs)
	pathArg := common.ExtractUDFValue(args[0])
	
	// First argument is always the path
	path, ok := pathArg.(string)
	if !ok {
		return opts, fmt.Errorf("find: first argument must be a string (path)")
	}

	// Expand ~ to home directory
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return opts, fmt.Errorf("find: cannot determine home directory: %v", err)
		}
		path = home
	} else if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return opts, fmt.Errorf("find: cannot determine home directory: %v", err)
		}
		path = filepath.Join(home, path[2:])
	}

	opts.Path = path

	// Parse additional arguments
	for i := 1; i < len(args); i++ {
		// Extract _val from UDF result objects (standard behavior for all UDFs)
		arg := common.ExtractUDFValue(args[i])
		
		switch v := arg.(type) {
		case string:
			// String argument could be type specification
			if v == "file" || v == "files" {
				opts.Type = "file"
			} else if v == "dir" || v == "dirs" || v == "directory" || v == "directories" {
				opts.Type = "dir"
			} else {
				return opts, fmt.Errorf("find: unknown string option %q (expected 'file' or 'dir')", v)
			}
		case float64:
			// Numeric argument is maxdepth
			opts.MaxDepth = int(v)
		case int:
			opts.MaxDepth = v
		case map[string]any:
			// Object with options
			if t, ok := v["type"].(string); ok {
				if t == "file" || t == "files" {
					opts.Type = "file"
				} else if t == "dir" || t == "dirs" || t == "directory" || t == "directories" {
					opts.Type = "dir"
				}
			}
			if md, ok := v["maxdepth"].(float64); ok {
				opts.MaxDepth = int(md)
			} else if md, ok := v["maxdepth"].(int); ok {
				opts.MaxDepth = md
			}
			if md, ok := v["mindepth"].(float64); ok {
				opts.MinDepth = int(md)
			} else if md, ok := v["mindepth"].(int); ok {
				opts.MinDepth = md
			}
		default:
			return opts, fmt.Errorf("find: unsupported argument type %T", arg)
		}
	}

	return opts, nil
}

// RegisterFind registers the find function with gojq
func RegisterFind() gojq.CompilerOption {
	return gojq.WithIterFunction("find", 1, 4, func(v any, args []any) gojq.Iter {
		opts, err := parseFindArgs(args)
		if err != nil {
			return gojq.NewIter(err)
		}

		results, err := findFiles(opts)
		if err != nil {
			return gojq.NewIter(fmt.Errorf("find: %v", err))
		}

		// Convert []any to variadic arguments for NewIter
		if len(results) == 0 {
			return gojq.NewIter[string]()
		}

		// Create a slice iterator manually since NewIter needs type parameter
		iter := &stringSliceIter{values: results, index: 0}
		return iter
	})
}

// stringSliceIter is an iterator over a slice of strings
type stringSliceIter struct {
	values []any
	index  int
}

func (iter *stringSliceIter) Next() (any, bool) {
	if iter.index >= len(iter.values) {
		return nil, false
	}
	value := iter.values[iter.index]
	iter.index++
	return value, true
}
