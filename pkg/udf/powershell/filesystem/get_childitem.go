package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/psobject"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// GetChildItemOptions represents options for Get-ChildItem
type GetChildItemOptions struct {
	Path      string
	Filter    string
	Recurse   bool
	Force     bool
	Name      string
	Include   []string
	Exclude   []string
	Depth     int
	Directory bool
	File      bool
}

// parseGetChildItemArgs parses arguments for get_childitem
func parseGetChildItemArgs(args []any) (GetChildItemOptions, error) {
	opts := GetChildItemOptions{
		Path:      ".",
		Filter:    "",
		Recurse:   false,
		Force:     false,
		Depth:     -1, // -1 means unlimited
		Include:   []string{},
		Exclude:   []string{},
	}

	if len(args) == 0 {
		return opts, nil
	}

	for i, arg := range args {
		argVal := common.ExtractUDFValue(arg)

		switch v := argVal.(type) {
		case string:
			if i == 0 {
				// First string argument is the path
				opts.Path = v
			} else if opts.Filter == "" {
				// Second string is filter
				opts.Filter = v
			}
		case bool:
			// Boolean flags - if true, enable recurse
			if v {
				opts.Recurse = true
			}
		case map[string]any:
			// Object with named options (PowerShell-style parameters)
			if path, ok := v["Path"].(string); ok {
				opts.Path = path
			}
			if filter, ok := v["Filter"].(string); ok {
				opts.Filter = filter
			}
			if recurse, ok := v["Recurse"].(bool); ok {
				opts.Recurse = recurse
			}
			if force, ok := v["Force"].(bool); ok {
				opts.Force = force
			}
			if name, ok := v["Name"].(string); ok {
				opts.Name = name
			}
			if depth, ok := v["Depth"].(float64); ok {
				opts.Depth = int(depth)
			}
			if directory, ok := v["Directory"].(bool); ok {
				opts.Directory = directory
			}
			if file, ok := v["File"].(bool); ok {
				opts.File = file
			}
			if include, ok := v["Include"].([]any); ok {
				for _, inc := range include {
					if s, ok := inc.(string); ok {
						opts.Include = append(opts.Include, s)
					}
				}
			}
			if exclude, ok := v["Exclude"].([]any); ok {
				for _, exc := range exclude {
					if s, ok := exc.(string); ok {
						opts.Exclude = append(opts.Exclude, s)
					}
				}
			}
		}
	}

	return opts, nil
}

// getChildItems performs the actual file system enumeration
func getChildItems(opts GetChildItemOptions) ([]any, error) {
	var results []any
	var errors []error

	// Resolve path (tilde expansion)
	path := opts.Path
	if strings.HasPrefix(path, "~") {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(homeDir, path[1:])
		}
	}

	// Check if path exists
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("path not found: %s", path)
	}

	// Walk the directory tree
	walkErr := filepath.Walk(path, func(walkPath string, info os.FileInfo, err error) error {
		if err != nil {
			// Collect errors for $ERROR variable (PowerShell behavior)
			errors = append(errors, err)
			return nil // Continue walking despite errors
		}

		// Skip the root path itself - we only want children
		if walkPath == path {
			return nil
		}

		// Calculate depth from root
		relPath, err := filepath.Rel(path, walkPath)
		if err != nil {
			return nil
		}
		currentDepth := strings.Count(relPath, string(filepath.Separator)) + 1

		// Check depth limit (Depth > 0 means limit, Depth <= 0 means unlimited)
		if opts.Depth > 0 && currentDepth > opts.Depth {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Apply Force filter (hidden files) - skip hidden dirs entirely
		if !opts.Force && isHiddenFile(info) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Apply type filters
		if opts.Directory && !info.IsDir() {
			return nil
		}
		if opts.File && info.IsDir() {
			// If we're looking for files only and hit a directory, skip descending
			return filepath.SkipDir
		}

		// Apply Filter (PowerShell-style wildcard matching)
		if opts.Filter != "" && !matchPattern(opts.Filter, info.Name()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Apply Include filters (if specified, file must match at least one)
		if len(opts.Include) > 0 {
			matched := false
			for _, pattern := range opts.Include {
				if matchPattern(pattern, info.Name()) {
					matched = true
					break
				}
			}
			if !matched {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		// Apply Exclude filters (if specified, file must not match any)
		if len(opts.Exclude) > 0 {
			for _, pattern := range opts.Exclude {
				if matchPattern(pattern, info.Name()) {
					if info.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
			}
		}

		// Add to results
		psobj, err := createPSObjectFromFileInfo(walkPath, info)
		if err != nil {
			errors = append(errors, err)
			return nil
		}
		results = append(results, psobj)

		// If not recursing and this is a directory, skip descending into it
		if !opts.Recurse && info.IsDir() {
			return filepath.SkipDir
		}

		return nil
	})

	if walkErr != nil {
		return nil, walkErr
	}

	// Store errors in $ERROR variable if any occurred
	if len(errors) > 0 {
		// In a full implementation, this would write to session state's $ERROR
		// For now, we just log them
		for _, err := range errors {
			fmt.Fprintf(os.Stderr, "get_childitem: %v\n", err)
		}
	}

	return results, nil
}

// countPathDepth counts the number of path segments
func countPathDepth(path string) int {
	path = filepath.Clean(path)
	if path == "." || path == "/" {
		return 0
	}
	return strings.Count(path, string(filepath.Separator)) + 1
}

// matchPattern performs PowerShell-style wildcard matching
// Supports * (any chars), ? (single char), and character ranges [abc]
func matchPattern(pattern, name string) bool {
	// Convert PowerShell wildcard pattern to regex
	regexPattern := "^"
	for _, r := range pattern {
		switch r {
		case '*':
			regexPattern += ".*"
		case '?':
			regexPattern += "."
		case '[':
			regexPattern += "["
		case ']':
			regexPattern += "]"
		case '.':
			regexPattern += "\\."
		default:
			regexPattern += string(r)
		}
	}
	regexPattern += "$"

	matched, _ := filepath.Match(pattern, name)
	return matched
}

// isHiddenFile detects hidden files cross-platform
func isHiddenFile(info os.FileInfo) bool {
	name := info.Name()

	// Unix: files starting with '.'
	if strings.HasPrefix(name, ".") {
		return true
	}

	// Windows: check file attributes (would require syscall on Windows)
	// For now, Unix-style detection is sufficient

	return false
}

// createPSObjectFromFileInfo creates a PSObject from file info
func createPSObjectFromFileInfo(path string, info os.FileInfo) (map[string]any, error) {
	// Create PSObject with file properties
	psobj := psobject.NewPSObject(path)
	psobj.TypeName = "System.IO.FileInfo"
	if info.IsDir() {
		psobj.TypeName = "System.IO.DirectoryInfo"
	}

	// Add NoteProperties (PowerShell-style properties)
	psobj.AddNoteProperty("Name", info.Name())
	psobj.AddNoteProperty("FullName", path)
	psobj.AddNoteProperty("Length", func() int64 {
		if info.IsDir() {
			return 0
		}
		return info.Size()
	}())
	psobj.AddNoteProperty("Mode", func() string {
		mode := info.Mode().String()
		if info.IsDir() {
			mode = "d" + mode[1:]
		}
		return mode
	}())
	psobj.AddNoteProperty("LastWriteTime", info.ModTime())
	psobj.AddNoteProperty("CreationTime", func() any {
		// On Unix, creation time is not always available
		return info.ModTime()
	}())
	psobj.AddNoteProperty("LastAccessTime", func() any {
		// Last access time
		return info.ModTime()
	}())
	psobj.AddNoteProperty("IsReadOnly", func() bool {
		return info.Mode()&0222 == 0
	}())
	psobj.AddNoteProperty("IsHidden", strings.HasPrefix(info.Name(), "."))
	psobj.AddNoteProperty("Extension", filepath.Ext(info.Name()))

	return psobj.ToMap(), nil
}

// RegisterGetChildItem registers the get_childitem function with gojq
func RegisterGetChildItem() gojq.CompilerOption {
	return gojq.WithIterFunction("get_childitem", 0, 5, func(v any, args []any) gojq.Iter {
		opts, err := parseGetChildItemArgs(args)
		if err != nil {
			return gojq.NewIter(err)
		}

		results, err := getChildItems(opts)
		if err != nil {
			return gojq.NewIter(fmt.Errorf("get_childitem: %v", err))
		}

		if len(results) == 0 {
			return gojq.NewIter[string]()
		}

		iter := &anySliceIter{values: results, index: 0}
		return iter
	})
}

// anySliceIter is an iterator over a slice of any
type anySliceIter struct {
	values []any
	index  int
}

func (iter *anySliceIter) Next() (any, bool) {
	if iter.index >= len(iter.values) {
		return nil, false
	}
	value := iter.values[iter.index]
	iter.index++
	return value, true
}
