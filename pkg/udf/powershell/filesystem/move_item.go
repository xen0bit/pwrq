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

// MoveItemOptions represents options for Move-Item
type MoveItemOptions struct {
	Path        string
	Destination string
	Force       bool
}

// parseMoveItemArgs parses arguments for move_item
func parseMoveItemArgs(args []any) (MoveItemOptions, error) {
	opts := MoveItemOptions{
		Path:  ".",
		Force: false,
	}

	if len(args) == 0 {
		return opts, nil
	}

	stringArgCount := 0
	for _, arg := range args {
		argVal := common.ExtractUDFValue(arg)

		switch v := argVal.(type) {
		case string:
			if stringArgCount == 0 {
				opts.Path = v
			} else if stringArgCount == 1 {
				opts.Destination = v
			}
			stringArgCount++
		case map[string]any:
			if path, ok := v["Path"].(string); ok {
				opts.Path = path
			}
			if dest, ok := v["Destination"].(string); ok {
				opts.Destination = dest
			}
			if force, ok := v["Force"].(bool); ok {
				opts.Force = force
			}
		}
	}

	return opts, nil
}

// moveItem moves a file or directory
func moveItem(opts MoveItemOptions) (any, error) {
	srcPath := opts.Path
	dstPath := opts.Destination

	// Expand tilde to home directory
	if strings.HasPrefix(srcPath, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot determine home directory: %v", err)
		}
		if len(srcPath) == 1 {
			srcPath = homeDir
		} else if len(srcPath) > 1 && (srcPath[1] == '/' || srcPath[1] == '\\') {
			srcPath = filepath.Join(homeDir, srcPath[2:])
		}
	}

	if strings.HasPrefix(dstPath, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot determine home directory: %v", err)
		}
		if len(dstPath) == 1 {
			dstPath = homeDir
		} else if len(dstPath) > 1 && (dstPath[1] == '/' || dstPath[1] == '\\') {
			dstPath = filepath.Join(homeDir, dstPath[2:])
		}
	}

	// Check if source exists
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("source path not found: %s", srcPath)
		}
		return nil, fmt.Errorf("cannot access source: %v", err)
	}

	// Check if destination exists
	dstExists := true
	dstInfo, err := os.Stat(dstPath)
	if err != nil {
		if os.IsNotExist(err) {
			dstExists = false
		} else {
			return nil, fmt.Errorf("cannot check destination: %v", err)
		}
	}

	// If destination exists and is not a directory, and we're moving a directory, fail
	if dstExists && dstInfo.IsDir() == false && srcInfo.IsDir() {
		return nil, fmt.Errorf("cannot move directory to a file: %s", dstPath)
	}

	// If destination exists and is a directory, append source name
	if dstExists && dstInfo.IsDir() {
		dstPath = filepath.Join(dstPath, filepath.Base(srcPath))
	}

	// Check if destination already exists
	if _, err := os.Stat(dstPath); err == nil {
		if !opts.Force {
			return nil, fmt.Errorf("destination already exists: %s", dstPath)
		}
		// Force mode: remove existing destination
		if err := os.RemoveAll(dstPath); err != nil {
			return nil, fmt.Errorf("cannot remove existing destination: %v", err)
		}
	}

	// Ensure parent directory of destination exists
	parentDir := filepath.Dir(dstPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create parent directory: %v", err)
	}

	// Perform the move
	if err := os.Rename(srcPath, dstPath); err != nil {
		// If rename fails (e.g., cross-device), fall back to copy+delete
		if srcInfo.IsDir() {
			if err := copyDirectory(srcPath, dstPath, CopyItemOptions{Recurse: true}); err != nil {
				return nil, fmt.Errorf("cannot move directory: %v", err)
			}
			if err := os.RemoveAll(srcPath); err != nil {
				return nil, fmt.Errorf("cannot remove source after copy: %v", err)
			}
		} else {
			if err := copyFile(srcPath, dstPath, CopyItemOptions{}); err != nil {
				return nil, fmt.Errorf("cannot move file: %v", err)
			}
			if err := os.Remove(srcPath); err != nil {
				return nil, fmt.Errorf("cannot remove source after copy: %v", err)
			}
		}
	}

	// Get info about moved item
	movedInfo, err := os.Stat(dstPath)
	if err != nil {
		return nil, fmt.Errorf("cannot stat moved item: %v", err)
	}

	// Create PSObject for the moved item
	psobj := psobject.NewPSObject(dstPath)
	psobj.TypeName = "System.IO.FileInfo"
	if movedInfo.IsDir() {
		psobj.TypeName = "System.IO.DirectoryInfo"
	}
	psobj.AddNoteProperty("Name", movedInfo.Name())
	psobj.AddNoteProperty("FullName", dstPath)
	psobj.AddNoteProperty("Length", func() int64 {
		if movedInfo.IsDir() {
			return 0
		}
		return movedInfo.Size()
	}())
	psobj.AddNoteProperty("Mode", movedInfo.Mode().String())
	psobj.AddNoteProperty("LastWriteTime", movedInfo.ModTime())
	psobj.AddNoteProperty("Exists", true)
	psobj.AddNoteProperty("PSIsMove", true)

	return psobj.ToMap(), nil
}

// RegisterMoveItem registers the move_item function with gojq
func RegisterMoveItem() gojq.CompilerOption {
	return gojq.WithIterFunction("move_item", 1, 3, func(v any, args []any) gojq.Iter {
		opts, err := parseMoveItemArgs(args)
		if err != nil {
			return gojq.NewIter(err)
		}

		result, err := moveItem(opts)
		if err != nil {
			return gojq.NewIter(fmt.Errorf("move_item: %v", err))
		}

		return gojq.NewIter(result)
	})
}
