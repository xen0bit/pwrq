package tee

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterTee registers the tee function with gojq
func RegisterTee() gojq.CompilerOption {
	return gojq.WithFunction("tee", 0, 1, func(v any, args []any) any {
		inputVal := common.ExtractUDFValue(v)

		var filePath string
		writeToFile := false

		// Parse arguments: optional file path
		if len(args) > 0 {
			if path, ok := args[0].(string); ok {
				filePath = path
				writeToFile = true
			} else {
				return common.MakeUDFErrorResult(fmt.Errorf("tee: argument must be a string file path, got %T", args[0]), nil)
			}
		}

		// Marshal input to JSON
		jsonBytes, err := json.Marshal(inputVal)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("tee: failed to marshal JSON: %v", err), nil)
		}

		// Write to file or stderr
		if writeToFile {
			// Expand ~ to home directory and get absolute path
			absPath, err := filepath.Abs(filePath)
			if err != nil {
				// Try to expand ~ first
				if filePath == "~" {
					home, homeErr := os.UserHomeDir()
					if homeErr != nil {
						return common.MakeUDFErrorResult(fmt.Errorf("tee: cannot determine home directory: %v", homeErr), nil)
					}
					absPath = home
				} else if len(filePath) > 0 && filePath[0] == '~' && (len(filePath) == 1 || filePath[1] == '/') {
					home, homeErr := os.UserHomeDir()
					if homeErr != nil {
						return common.MakeUDFErrorResult(fmt.Errorf("tee: cannot determine home directory: %v", homeErr), nil)
					}
					if len(filePath) > 1 {
						absPath = filepath.Join(home, filePath[2:])
					} else {
						absPath = home
					}
					absPath, err = filepath.Abs(absPath)
					if err != nil {
						return common.MakeUDFErrorResult(fmt.Errorf("tee: cannot resolve path %q: %v", filePath, err), nil)
					}
				} else {
					return common.MakeUDFErrorResult(fmt.Errorf("tee: cannot resolve path %q: %v", filePath, err), nil)
				}
			}
			filePath = absPath

			// Write to file in append mode
			file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				meta := map[string]any{
					"operation": "tee",
					"file_path": filePath,
				}
				return common.MakeUDFErrorResult(fmt.Errorf("tee: failed to open file %q: %v", filePath, err), meta)
			}
			defer file.Close()

			_, err = file.Write(jsonBytes)
			if err != nil {
				meta := map[string]any{
					"operation": "tee",
					"file_path": filePath,
				}
				return common.MakeUDFErrorResult(fmt.Errorf("tee: failed to write to file %q: %v", filePath, err), meta)
			}

			// Write newline after JSON
			_, err = file.Write([]byte("\n"))
			if err != nil {
				meta := map[string]any{
					"operation": "tee",
					"file_path": filePath,
				}
				return common.MakeUDFErrorResult(fmt.Errorf("tee: failed to write newline to file %q: %v", filePath, err), meta)
			}
		} else {
			// Write to stderr
			os.Stderr.Write(jsonBytes)
			os.Stderr.Write([]byte("\n"))
		}

		// Return the input unchanged (pass through)
		// If input was a UDF result, return it as-is, otherwise wrap it
		if common.IsUDFResult(v) {
			return v
		}

		meta := map[string]any{
			"operation": "tee",
		}
		if writeToFile {
			meta["file_path"] = filePath
			meta["written"] = true
			meta["bytes_written"] = len(jsonBytes)
		} else {
			meta["written_to"] = "stderr"
			meta["bytes_written"] = len(jsonBytes)
		}

		// Return input with metadata
		return common.MakeUDFSuccessResult(inputVal, meta)
	})
}
