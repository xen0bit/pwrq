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

// NewItemOptions represents options for New-Item
type NewItemOptions struct {
	Path      string
	Name      string
	ItemType  string
	Value     any
	Force     bool
	ErrorCode int
}

// parseNewItemArgs parses arguments for new_item
func parseNewItemArgs(args []any) (NewItemOptions, error) {
	opts := NewItemOptions{
		Path:      ".",
		ItemType:  "file",
		Force:     false,
		ErrorCode: -1,
	}

	if len(args) == 0 {
		return opts, nil
	}

	for i, arg := range args {
		argVal := common.ExtractUDFValue(arg)

		switch v := argVal.(type) {
		case string:
			if i == 0 {
				opts.Path = v
			} else if opts.Name == "" {
				opts.Name = v
			}
		case float64:
			// Numeric error code for error file type
			opts.ErrorCode = int(v)
		case map[string]any:
			if path, ok := v["Path"].(string); ok {
				opts.Path = path
			}
			if name, ok := v["Name"].(string); ok {
				opts.Name = name
			}
			if itemType, ok := v["ItemType"].(string); ok {
				opts.ItemType = strings.ToLower(itemType)
			}
			if value, ok := v["Value"]; ok {
				opts.Value = value
			}
			if force, ok := v["Force"].(bool); ok {
				opts.Force = force
			}
			if errorCode, ok := v["ErrorCode"].(float64); ok {
				opts.ErrorCode = int(errorCode)
			}
		}
	}

	return opts, nil
}

// newItem creates a new file or directory
func newItem(opts NewItemOptions) (any, error) {
	path := opts.Path

	// If Name is specified separately, join it with Path
	if opts.Name != "" {
		path = filepath.Join(path, opts.Name)
	}

	// Expand tilde to home directory
	if strings.HasPrefix(path, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot determine home directory: %v", err)
		}
		if len(path) == 1 {
			path = homeDir
		} else if len(path) > 1 && (path[1] == '/' || path[1] == '\\') {
			path = filepath.Join(homeDir, path[2:])
		}
	}

	// Check if path already exists
	existing, err := os.Stat(path)
	if err == nil {
		// Path exists
		if !opts.Force {
			return nil, fmt.Errorf("path already exists: %s", path)
		}
		// Force mode: if it's a directory and we're creating a file, fail
		if existing.IsDir() && opts.ItemType == "file" {
			return nil, fmt.Errorf("cannot overwrite directory with file: %s", path)
		}
		if !existing.IsDir() && opts.ItemType == "directory" {
			return nil, fmt.Errorf("cannot overwrite file with directory: %s", path)
		}
	}

	// Create the item based on type
	var createdPath string
	switch opts.ItemType {
	case "file", "f":
		// Ensure parent directory exists
		parentDir := filepath.Dir(path)
		if err := os.MkdirAll(parentDir, 0755); err != nil {
			return nil, fmt.Errorf("cannot create parent directory: %v", err)
		}

		// Create the file
		file, err := os.Create(path)
		if err != nil {
			return nil, fmt.Errorf("cannot create file: %v", err)
		}
		defer file.Close()

		// Write initial value if provided
		if opts.Value != nil {
			var content string
			switch v := opts.Value.(type) {
			case string:
				content = v
			case []byte:
				content = string(v)
			default:
				// Convert to string representation
				content = fmt.Sprintf("%v", v)
			}
			if _, err := file.WriteString(content); err != nil {
				return nil, fmt.Errorf("cannot write to file: %v", err)
			}
		}

		createdPath = path

	case "directory", "dir", "d":
		// Create directory (and parents if needed)
		if err := os.MkdirAll(path, 0755); err != nil {
			return nil, fmt.Errorf("cannot create directory: %v", err)
		}
		createdPath = path

	default:
		return nil, fmt.Errorf("unsupported item type: %s (use 'file' or 'directory')", opts.ItemType)
	}

	// Get info about created item
	info, err := os.Stat(createdPath)
	if err != nil {
		return nil, fmt.Errorf("cannot stat created item: %v", err)
	}

	// Create PSObject for the created item
	psobj := psobject.NewPSObject(createdPath)
	psobj.TypeName = "System.IO.FileInfo"
	if info.IsDir() {
		psobj.TypeName = "System.IO.DirectoryInfo"
	}
	psobj.AddNoteProperty("Name", info.Name())
	psobj.AddNoteProperty("FullName", createdPath)
	psobj.AddNoteProperty("Length", func() int64 {
		if info.IsDir() {
			return 0
		}
		return info.Size()
	}())
	psobj.AddNoteProperty("Mode", info.Mode().String())
	psobj.AddNoteProperty("LastWriteTime", info.ModTime())
	psobj.AddNoteProperty("Exists", true)

	return psobj.ToMap(), nil
}

// RegisterNewItem registers the new_item function with gojq
func RegisterNewItem() gojq.CompilerOption {
	return gojq.WithIterFunction("new_item", 0, 3, func(v any, args []any) gojq.Iter {
		opts, err := parseNewItemArgs(args)
		if err != nil {
			return gojq.NewIter(err)
		}

		result, err := newItem(opts)
		if err != nil {
			return gojq.NewIter(fmt.Errorf("new_item: %v", err))
		}

		return gojq.NewIter(result)
	})
}
