package common

import (
	"fmt"
	"os"
	"path/filepath"
)

// ParseFileArgs parses function arguments to extract input value and file flag.
// It handles the pattern where the first argument can be either:
// - A boolean (file flag) - in which case inputVal comes from v
// - A value - in which case file flag can be the second argument
// Returns: inputVal, isFile, error
func ParseFileArgs(v any, args []any) (any, bool, error) {
	var inputVal any
	var isFile bool

	if len(args) > 0 {
		// Check if first arg is boolean (file flag) or value
		if fileFlag, ok := args[0].(bool); ok {
			isFile = fileFlag
			inputVal = v
		} else {
			inputVal = args[0]
			// Check for file flag as second arg
			if len(args) > 1 {
				if fileFlag, ok := args[1].(bool); ok {
					isFile = fileFlag
				}
			}
		}
	} else {
		inputVal = v
	}

	return inputVal, isFile, nil
}

// ReadFileFromPath reads a file from a path string, handling ~ expansion and absolute path resolution.
// Returns: fileData, absPath, fileSize, error
func ReadFileFromPath(filePath string) ([]byte, string, int64, error) {
	// Expand ~ to home directory
	if filePath == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, "", 0, fmt.Errorf("cannot determine home directory: %v", err)
		}
		filePath = home
	} else if len(filePath) > 0 && filePath[0] == '~' && (len(filePath) == 1 || filePath[1] == '/') {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, "", 0, fmt.Errorf("cannot determine home directory: %v", err)
		}
		if len(filePath) > 1 {
			filePath = filepath.Join(home, filePath[2:])
		} else {
			filePath = home
		}
	}

	// Convert to absolute path
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, "", 0, fmt.Errorf("cannot resolve path %q: %v", filePath, err)
	}

	// Read file contents
	fileData, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", 0, fmt.Errorf("file does not exist: %q", absPath)
		}
		if os.IsPermission(err) {
			return nil, "", 0, fmt.Errorf("permission denied reading file: %q", absPath)
		}
		return nil, "", 0, fmt.Errorf("failed to read file %q: %v", absPath, err)
	}

	// Get file info for metadata
	fileInfo, err := os.Stat(absPath)
	var fileSize int64
	if err == nil {
		fileSize = fileInfo.Size()
	}

	return fileData, absPath, fileSize, nil
}

