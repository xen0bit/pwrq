package filesystem

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// CopyItemOptions represents options for Copy-Item
type CopyItemOptions struct {
	Path        string
	Destination string
	Recurse     bool
	Force       bool
	Filter      string
	Include     []string
	Exclude     []string
}

// parseCopyItemArgs parses arguments for copy_item
func parseCopyItemArgs(args []any) (CopyItemOptions, error) {
	opts := CopyItemOptions{
		Path:    ".",
		Recurse: false,
		Force:   false,
		Include: []string{},
		Exclude: []string{},
	}

	if len(args) == 0 {
		return opts, nil
	}

	stringArgCount := 0
	for _, arg := range args {
		argVal := common.BindValue(arg)

		switch v := argVal.(type) {
		case string:
			switch stringArgCount {
			case 0:
				opts.Path = v
			case 1:
				opts.Destination = v
			}
			stringArgCount++
		case bool:
			if v {
				opts.Recurse = true
			}
		case map[string]any:
			if path, ok := v["Path"].(string); ok {
				opts.Path = path
			}
			if dest, ok := v["Destination"].(string); ok {
				opts.Destination = dest
			}
			if recurse, ok := v["Recurse"].(bool); ok {
				opts.Recurse = recurse
			}
			if force, ok := v["Force"].(bool); ok {
				opts.Force = force
			}
			if filter, ok := v["Filter"].(string); ok {
				opts.Filter = filter
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

// copyItem copies a file or directory
func copyItem(opts CopyItemOptions) (any, error) {
	// Resolve tilde in paths
	srcPath := opts.Path
	if strings.HasPrefix(srcPath, "~") {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			srcPath = filepath.Join(homeDir, srcPath[1:])
		}
	}

	dstPath := opts.Destination
	if strings.HasPrefix(dstPath, "~") {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			dstPath = filepath.Join(homeDir, dstPath[1:])
		}
	}

	// Check if source exists
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		return nil, fmt.Errorf("source path not found: %s", srcPath)
	}

	// Handle single file copy (apply filters even for single files)
	if !srcInfo.IsDir() {
		// Apply Filter
		if opts.Filter != "" && !matchPattern(opts.Filter, filepath.Base(srcPath)) {
			return nil, fmt.Errorf("file does not match filter: %s", opts.Filter)
		}

		// Apply Include filters
		if len(opts.Include) > 0 {
			matched := false
			for _, pattern := range opts.Include {
				if matchPattern(pattern, filepath.Base(srcPath)) {
					matched = true
					break
				}
			}
			if !matched {
				return nil, fmt.Errorf("file does not match any include pattern")
			}
		}

		// Apply Exclude filters
		if len(opts.Exclude) > 0 {
			for _, pattern := range opts.Exclude {
				if matchPattern(pattern, filepath.Base(srcPath)) {
					return nil, fmt.Errorf("file matches exclude pattern: %s", pattern)
				}
			}
		}

		// Check if destination is a directory - if so, copy file into it
		if dstInfo, err := os.Stat(dstPath); err == nil && dstInfo.IsDir() {
			dstPath = filepath.Join(dstPath, filepath.Base(srcPath))
		}

		// Check if destination exists
		if dstInfo, err := os.Stat(dstPath); err == nil {
			// Destination exists
			if !opts.Force {
				return nil, fmt.Errorf("destination already exists and -Force not specified: %s", dstPath)
			}
			// Force: make destination writable if read-only
			if dstInfo.Mode()&0222 == 0 {
				if err := os.Chmod(dstPath, 0644); err != nil {
					return nil, fmt.Errorf("cannot make destination writable: %v", err)
				}
			}
		}

		// Copy the file
		if err := copyFile(srcPath, dstPath, opts); err != nil {
			return nil, err
		}

		// Return success object
		result := map[string]any{
			"Source":      srcPath,
			"Destination": dstPath,
			"Success":     true,
		}
		return result, nil
	}

	// Handle directory copy
	if !opts.Recurse {
		// Without -Recurse, only copy directory contents at top level
		return copyDirectoryContents(srcPath, dstPath, opts)
	}

	// With -Recurse, copy entire directory tree
	if err := copyDirectory(srcPath, dstPath, opts); err != nil {
		return nil, err
	}

	result := map[string]any{
		"Source":      srcPath,
		"Destination": dstPath,
		"Success":     true,
	}
	return result, nil
}

// copyFile copies a single file with timestamp preservation
func copyFile(src, dst string, opts CopyItemOptions) error {
	// Ensure parent directory exists
	parentDir := filepath.Dir(dst)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("cannot create parent directory: %v", err)
	}

	// Open source file
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("cannot open source file: %v", err)
	}
	defer func() { _ = srcFile.Close() }()

	// Get source file info for timestamp preservation
	srcInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("cannot stat source file: %v", err)
	}

	// Create destination file
	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("cannot create destination file: %v", err)
	}
	defer func() { _ = dstFile.Close() }()

	// Copy contents
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("cannot copy file contents: %v", err)
	}

	// Sync to ensure data is written
	if err := dstFile.Sync(); err != nil {
		return fmt.Errorf("cannot sync destination file: %v", err)
	}

	// Preserve timestamps (CreationTime, LastWriteTime, LastAccessTime)
	modTime := srcInfo.ModTime()
	accessTime := modTime // On Unix, we use modTime for both
	if err := os.Chtimes(dst, accessTime, modTime); err != nil {
		// Non-fatal, just log
		fmt.Fprintf(os.Stderr, "copy_item: warning - could not preserve timestamps: %v\n", err)
	}

	// Preserve file mode (permissions)
	if err := os.Chmod(dst, srcInfo.Mode()); err != nil {
		// Non-fatal for most cases
		fmt.Fprintf(os.Stderr, "copy_item: warning - could not preserve permissions: %v\n", err)
	}

	return nil
}

// copyDirectory copies a directory recursively with filter support
func copyDirectory(src, dst string, opts CopyItemOptions) error {
	// Create destination directory
	if err := os.MkdirAll(dst, 0755); err != nil {
		return fmt.Errorf("cannot create destination directory: %v", err)
	}

	// Read source directory entries
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("cannot read source directory: %v", err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		// Apply Filter
		if opts.Filter != "" && !matchPattern(opts.Filter, entry.Name()) {
			continue
		}

		// Apply Include filters (if specified, must match at least one)
		if len(opts.Include) > 0 {
			matched := false
			for _, pattern := range opts.Include {
				if matchPattern(pattern, entry.Name()) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}

		// Apply Exclude filters (if specified, must not match any)
		if len(opts.Exclude) > 0 {
			excluded := false
			for _, pattern := range opts.Exclude {
				if matchPattern(pattern, entry.Name()) {
					excluded = true
					break
				}
			}
			if excluded {
				continue
			}
		}

		// Handle directories vs files
		if entry.IsDir() {
			if opts.Recurse {
				if err := copyDirectory(srcPath, dstPath, opts); err != nil {
					return err
				}
			}
		} else {
			// Handle Force: make destination writable if it exists and is read-only
			if opts.Force {
				if dstInfo, err := os.Stat(dstPath); err == nil {
					// Destination exists, check if read-only
					if dstInfo.Mode()&0222 == 0 {
						// Make writable
						if err := os.Chmod(dstPath, 0644); err != nil {
							return fmt.Errorf("cannot make destination writable: %v", err)
						}
					}
				}
			}

			if err := copyFile(srcPath, dstPath, opts); err != nil {
				return err
			}
		}
	}

	return nil
}

// copyDirectoryContents copies only the contents of a directory (not recursive)
func copyDirectoryContents(src, dst string, opts CopyItemOptions) (any, error) {
	// Create destination directory
	if err := os.MkdirAll(dst, 0755); err != nil {
		return nil, fmt.Errorf("cannot create destination directory: %v", err)
	}

	// Read source directory entries
	entries, err := os.ReadDir(src)
	if err != nil {
		return nil, fmt.Errorf("cannot read source directory: %v", err)
	}

	copied := 0
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		// Apply Filter
		if opts.Filter != "" && !matchPattern(opts.Filter, entry.Name()) {
			continue
		}

		// Apply Include filters
		if len(opts.Include) > 0 {
			matched := false
			for _, pattern := range opts.Include {
				if matchPattern(pattern, entry.Name()) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}

		// Apply Exclude filters
		if len(opts.Exclude) > 0 {
			excluded := false
			for _, pattern := range opts.Exclude {
				if matchPattern(pattern, entry.Name()) {
					excluded = true
					break
				}
			}
			if excluded {
				continue
			}
		}

		// Skip directories when not recursing
		if entry.IsDir() {
			continue
		}

		// Handle Force for files
		if opts.Force {
			if dstInfo, err := os.Stat(dstPath); err == nil {
				if dstInfo.Mode()&0222 == 0 {
					if err := os.Chmod(dstPath, 0644); err != nil {
						return nil, fmt.Errorf("cannot make destination writable: %v", err)
					}
				}
			}
		}

		if err := copyFile(srcPath, dstPath, opts); err != nil {
			return nil, err
		}
		copied++
	}

	result := map[string]any{
		"Source":      src,
		"Destination": dst,
		"FilesCopied": copied,
		"Success":     true,
	}
	return result, nil
}

// RegisterCopyItem registers the copy_item function with gojq
func RegisterCopyItem() gojq.CompilerOption {
	return common.WithIterFunction("copy_item", 1, 3, func(v any, args []any) gojq.Iter {
		opts, err := parseCopyItemArgs(args)
		if err != nil {
			return gojq.NewIter(err)
		}

		result, err := copyItem(opts)
		if err != nil {
			return gojq.NewIter(fmt.Errorf("copy_item: %v", err))
		}

		return gojq.NewIter(result)
	})
}
