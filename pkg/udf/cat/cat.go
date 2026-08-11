package cat

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// CatOptions holds options for the cat function
type CatOptions struct {
	TailLines  int
	TotalCount int
	Encoding   string
}

// getEncoding returns the appropriate decoder for the given encoding name
func getEncoding(encodingName string) (encoding.Encoding, error) {
	switch strings.ToLower(encodingName) {
	case "utf8", "utf-8", "ascii", "default", "":
		// ASCII is a subset of UTF-8, so UTF-8 decoder handles ASCII correctly
		return unicode.UTF8, nil
	case "utf16le", "utf-16le":
		return unicode.UTF16(unicode.LittleEndian, unicode.UseBOM), nil
	case "utf16be", "utf-16be":
		return unicode.UTF16(unicode.BigEndian, unicode.UseBOM), nil
	default:
		return nil, fmt.Errorf("unsupported encoding: %s", encodingName)
	}
}

// RegisterCat registers the cat function with gojq
// Supports PowerShell-style parameters: -Tail, -TotalCount, -Encoding
// Usage: cat(path) or cat(path; {tail: n, totalcount: n, encoding: "utf8"})
func RegisterCat() gojq.CompilerOption {
	return gojq.WithFunction("cat", 0, 2, func(v any, args []any) any {
		var filePath string
		var tailLines int = -1  // -1 means no tail limit
		var totalCount int = -1 // -1 means no total count limit
		var encoding string = "utf8"

		// Parse arguments: file path can come from pipe or as argument
		if len(args) > 0 {
			// File path provided as argument
			if path, ok := args[0].(string); ok {
				filePath = path
			} else {
				// Try to extract from UDF result
				pathVal := common.BindValue(args[0])
				if pathStr, ok := pathVal.(string); ok {
					filePath = pathStr
				} else {
					return common.MakeUDFErrorResult(fmt.Errorf("cat: argument must be a string file path, got %T", args[0]), nil)
				}
			}

			// Parse options from second argument if provided
			if len(args) > 1 {
				optsVal := common.BindValue(args[1])
				if opts, ok := optsVal.(map[string]any); ok {
					if tailVal, exists := opts["tail"]; exists {
						if tailNum, ok := tailVal.(float64); ok {
							tailLines = int(tailNum)
						} else if tailStr, ok := tailVal.(string); ok {
							if n, err := strconv.Atoi(tailStr); err == nil {
								tailLines = n
							}
						}
					}
					if totalVal, exists := opts["totalcount"]; exists {
						if totalNum, ok := totalVal.(float64); ok {
							totalCount = int(totalNum)
						} else if totalStr, ok := totalVal.(string); ok {
							if n, err := strconv.Atoi(totalStr); err == nil {
								totalCount = n
							}
						}
					}
					if encVal, exists := opts["encoding"]; exists {
						if encStr, ok := encVal.(string); ok {
							encoding = strings.ToLower(encStr)
						}
					}
				}
			}
		} else {
			// File path from pipe
			inputVal := common.BindValue(v)
			if pathStr, ok := inputVal.(string); ok {
				filePath = pathStr
			} else {
				return common.MakeUDFErrorResult(fmt.Errorf("cat: input must be a string file path, got %T", inputVal), nil)
			}
		}

		// Validate encoding parameter
		validEncodings := map[string]bool{
			"utf8": true, "utf-8": true, "ascii": true, "utf16le": true, "utf-16le": true,
			"utf16be": true, "utf-16be": true, "default": true,
		}
		if !validEncodings[encoding] {
			return common.MakeUDFErrorResult(fmt.Errorf("cat: unsupported encoding %q (supported: utf8, ascii, utf16le, utf16be)", encoding), nil)
		}

		// Expand ~ to home directory and get absolute path
		absPath, err := filepath.Abs(filePath)
		if err != nil {
			if filePath == "~" {
				home, homeErr := os.UserHomeDir()
				if homeErr != nil {
					return common.MakeUDFErrorResult(fmt.Errorf("cat: cannot determine home directory: %v", homeErr), nil)
				}
				absPath = home
			} else if len(filePath) > 0 && filePath[0] == '~' && (len(filePath) == 1 || filePath[1] == '/') {
				home, homeErr := os.UserHomeDir()
				if homeErr != nil {
					return common.MakeUDFErrorResult(fmt.Errorf("cat: cannot determine home directory: %v", homeErr), nil)
				}
				if len(filePath) > 1 {
					absPath = filepath.Join(home, filePath[2:])
				} else {
					absPath = home
				}
				absPath, err = filepath.Abs(absPath)
				if err != nil {
					return common.MakeUDFErrorResult(fmt.Errorf("cat: cannot resolve path %q: %v", filePath, err), nil)
				}
			} else {
				return common.MakeUDFErrorResult(fmt.Errorf("cat: cannot resolve path %q: %v", filePath, err), nil)
			}
		}
		filePath = absPath

		// Check if file exists and is not a directory
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			meta := map[string]any{
				"operation": "cat",
				"file_path": filePath,
			}
			if os.IsNotExist(err) {
				return common.MakeUDFErrorResult(fmt.Errorf("cat: file does not exist: %q", filePath), meta)
			}
			if os.IsPermission(err) {
				return common.MakeUDFErrorResult(fmt.Errorf("cat: permission denied: %q", filePath), meta)
			}
			return common.MakeUDFErrorResult(fmt.Errorf("cat: cannot access file %q: %v", filePath, err), meta)
		}

		if fileInfo.IsDir() {
			meta := map[string]any{
				"operation": "cat",
				"file_path": filePath,
			}
			return common.MakeUDFErrorResult(fmt.Errorf("cat: %q is a directory, not a file", filePath), meta)
		}

		// Open file for reading
		file, err := os.Open(filePath)
		if err != nil {
			meta := map[string]any{
				"operation": "cat",
				"file_path": filePath,
			}
			return common.MakeUDFErrorResult(fmt.Errorf("cat: cannot open file %q: %v", filePath, err), meta)
		}
		defer file.Close()

		// Get the encoding decoder
		enc, err := getEncoding(encoding)
		if err != nil {
			meta := map[string]any{
				"operation": "cat",
				"file_path": filePath,
			}
			return common.MakeUDFErrorResult(fmt.Errorf("cat: %v", err), meta)
		}

		// Create a transformed reader that decodes the file content
		decodedReader := transform.NewReader(file, enc.NewDecoder())

		var content string

		// Handle -TotalCount and -Tail options
		if totalCount > 0 || tailLines > 0 {
			var lines []string
			scanner := bufio.NewScanner(decodedReader)

			for scanner.Scan() {
				line := scanner.Text()

				// Apply TotalCount limit
				if totalCount > 0 && len(lines) >= totalCount {
					break
				}

				lines = append(lines, line)
			}

			if err := scanner.Err(); err != nil {
				meta := map[string]any{
					"operation": "cat",
					"file_path": filePath,
				}
				return common.MakeUDFErrorResult(fmt.Errorf("cat: error reading file %q: %v", filePath, err), meta)
			}

			// Apply Tail limit (get last N lines)
			if tailLines > 0 && len(lines) > tailLines {
				lines = lines[len(lines)-tailLines:]
			}

			content = strings.Join(lines, "\n")
			if len(lines) > 0 {
				content += "\n"
			}
		} else {
			// Read entire file with encoding
			fileData, err := io.ReadAll(decodedReader)
			if err != nil {
				meta := map[string]any{
					"operation": "cat",
					"file_path": filePath,
				}
				return common.MakeUDFErrorResult(fmt.Errorf("cat: failed to read file %q: %v", filePath, err), meta)
			}
			content = string(fileData)
		}

		meta := map[string]any{
			"operation": "cat",
			"file_path": filePath,
			"file_size": int(fileInfo.Size()),
			"encoding":  encoding,
		}
		if tailLines > 0 {
			meta["tail"] = tailLines
		}
		if totalCount > 0 {
			meta["totalcount"] = totalCount
		}

		return common.MakeUDFSuccessResult(content, meta)
	})
}

// cat is the internal implementation for testing
func cat(inputPath string, args []any) any {
	var filePath string
	var tailLines int = -1
	var totalCount int = -1
	var encoding string = "utf8"

	if len(args) > 0 {
		// Check if args[0] is a path string or options map
		if p, ok := args[0].(string); ok {
			filePath = p
			// Options in args[1]
			if len(args) > 1 {
				if opts, ok := args[1].(map[string]any); ok {
					if tailVal, exists := opts["tail"]; exists {
						if tailNum, ok := tailVal.(float64); ok {
							tailLines = int(tailNum)
						} else if tailStr, ok := tailVal.(string); ok {
							if n, err := strconv.Atoi(tailStr); err == nil {
								tailLines = n
							}
						}
					}
					if totalVal, exists := opts["totalcount"]; exists {
						if totalNum, ok := totalVal.(float64); ok {
							totalCount = int(totalNum)
						} else if totalStr, ok := totalVal.(string); ok {
							if n, err := strconv.Atoi(totalStr); err == nil {
								totalCount = n
							}
						}
					}
					if encVal, exists := opts["encoding"]; exists {
						if encStr, ok := encVal.(string); ok {
							encoding = strings.ToLower(encStr)
						}
					}
				}
			}
		} else if opts, ok := args[0].(map[string]any); ok {
			// args[0] is options map, use inputPath
			filePath = inputPath
			if tailVal, exists := opts["tail"]; exists {
				if tailNum, ok := tailVal.(float64); ok {
					tailLines = int(tailNum)
				} else if tailStr, ok := tailVal.(string); ok {
					if n, err := strconv.Atoi(tailStr); err == nil {
						tailLines = n
					}
				}
			}
			if totalVal, exists := opts["totalcount"]; exists {
				if totalNum, ok := totalVal.(float64); ok {
					totalCount = int(totalNum)
				} else if totalStr, ok := totalVal.(string); ok {
					if n, err := strconv.Atoi(totalStr); err == nil {
						totalCount = n
					}
				}
			}
			if encVal, exists := opts["encoding"]; exists {
				if encStr, ok := encVal.(string); ok {
					encoding = strings.ToLower(encStr)
				}
			}
		} else {
			filePath = inputPath
		}
	} else {
		filePath = inputPath
	}

	// Validate encoding parameter
	validEncodings := map[string]bool{
		"utf8": true, "utf-8": true, "ascii": true, "utf16le": true, "utf-16le": true,
		"utf16be": true, "utf-16be": true, "default": true,
	}
	if !validEncodings[encoding] {
		return common.MakeUDFErrorResult(fmt.Errorf("cat: unsupported encoding %q (supported: utf8, ascii, utf16le, utf16be)", encoding), nil)
	}

	// Expand ~ to home directory and get absolute path
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		if filePath == "~" {
			home, homeErr := os.UserHomeDir()
			if homeErr != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("cat: cannot determine home directory: %v", homeErr), nil)
			}
			absPath = home
		} else if len(filePath) > 0 && filePath[0] == '~' && (len(filePath) == 1 || filePath[1] == '/') {
			home, homeErr := os.UserHomeDir()
			if homeErr != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("cat: cannot determine home directory: %v", homeErr), nil)
			}
			if len(filePath) > 1 {
				absPath = filepath.Join(home, filePath[2:])
			} else {
				absPath = home
			}
			absPath, err = filepath.Abs(absPath)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("cat: cannot resolve path %q: %v", filePath, err), nil)
			}
		} else {
			return common.MakeUDFErrorResult(fmt.Errorf("cat: cannot resolve path %q: %v", filePath, err), nil)
		}
	}
	filePath = absPath

	// Check if file exists and is not a directory
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		meta := map[string]any{
			"operation": "cat",
			"file_path": filePath,
		}
		if os.IsNotExist(err) {
			return common.MakeUDFErrorResult(fmt.Errorf("cat: file does not exist: %q", filePath), meta)
		}
		if os.IsPermission(err) {
			return common.MakeUDFErrorResult(fmt.Errorf("cat: permission denied: %q", filePath), meta)
		}
		return common.MakeUDFErrorResult(fmt.Errorf("cat: cannot access file %q: %v", filePath, err), meta)
	}

	if fileInfo.IsDir() {
		meta := map[string]any{
			"operation": "cat",
			"file_path": filePath,
		}
		return common.MakeUDFErrorResult(fmt.Errorf("cat: %q is a directory, not a file", filePath), meta)
	}

	// Open file for reading
	file, err := os.Open(filePath)
	if err != nil {
		meta := map[string]any{
			"operation": "cat",
			"file_path": filePath,
		}
		return common.MakeUDFErrorResult(fmt.Errorf("cat: cannot open file %q: %v", filePath, err), meta)
	}
	defer file.Close()

	// Get the encoding decoder
	enc, err := getEncoding(encoding)
	if err != nil {
		meta := map[string]any{
			"operation": "cat",
			"file_path": filePath,
		}
		return common.MakeUDFErrorResult(fmt.Errorf("cat: %v", err), meta)
	}

	// Create a transformed reader that decodes the file content
	decodedReader := transform.NewReader(file, enc.NewDecoder())

	var content string

	// Handle -TotalCount and -Tail options
	if totalCount > 0 || tailLines > 0 {
		var lines []string
		scanner := bufio.NewScanner(decodedReader)

		for scanner.Scan() {
			line := scanner.Text()

			// Apply TotalCount limit
			if totalCount > 0 && len(lines) >= totalCount {
				break
			}

			lines = append(lines, line)
		}

		if err := scanner.Err(); err != nil {
			meta := map[string]any{
				"operation": "cat",
				"file_path": filePath,
			}
			return common.MakeUDFErrorResult(fmt.Errorf("cat: error reading file %q: %v", filePath, err), meta)
		}

		// Apply Tail limit (get last N lines)
		if tailLines > 0 && len(lines) > tailLines {
			lines = lines[len(lines)-tailLines:]
		}

		content = strings.Join(lines, "\n")
		if len(lines) > 0 {
			content += "\n"
		}
	} else {
		// Read entire file with encoding
		fileData, err := io.ReadAll(decodedReader)
		if err != nil {
			meta := map[string]any{
				"operation": "cat",
				"file_path": filePath,
			}
			return common.MakeUDFErrorResult(fmt.Errorf("cat: failed to read file %q: %v", filePath, err), meta)
		}
		content = string(fileData)
	}

	meta := map[string]any{
		"operation": "cat",
		"file_path": filePath,
		"file_size": int(fileInfo.Size()),
		"encoding":  encoding,
	}
	if tailLines > 0 {
		meta["tail"] = tailLines
	}
	if totalCount > 0 {
		meta["totalcount"] = totalCount
	}

	return common.MakeUDFSuccessResult(content, meta)
}

// parseCatArgs parses the arguments for the cat function (for testing)
func parseCatArgs(args []any) (string, CatOptions, error) {
	opts := CatOptions{
		TailLines:  -1,
		TotalCount: -1,
		Encoding:   "utf8",
	}

	if len(args) == 0 {
		return "", opts, fmt.Errorf("cat: missing path argument")
	}

	var filePath string
	if path, ok := args[0].(string); ok {
		filePath = path
	} else {
		pathVal := common.BindValue(args[0])
		if pathStr, ok := pathVal.(string); ok {
			filePath = pathStr
		} else {
			return "", opts, fmt.Errorf("cat: argument must be a string file path, got %T", args[0])
		}
	}

	if len(args) > 1 {
		optsVal := common.BindValue(args[1])
		if optsMap, ok := optsVal.(map[string]any); ok {
			if tailVal, exists := optsMap["tail"]; exists {
				if tailNum, ok := tailVal.(float64); ok {
					opts.TailLines = int(tailNum)
				} else if tailStr, ok := tailVal.(string); ok {
					if n, err := strconv.Atoi(tailStr); err == nil {
						opts.TailLines = n
					}
				}
			}
			if totalVal, exists := optsMap["totalcount"]; exists {
				if totalNum, ok := totalVal.(float64); ok {
					opts.TotalCount = int(totalNum)
				} else if totalStr, ok := totalVal.(string); ok {
					if n, err := strconv.Atoi(totalStr); err == nil {
						opts.TotalCount = n
					}
				}
			}
			if encVal, exists := optsMap["encoding"]; exists {
				if encStr, ok := encVal.(string); ok {
					opts.Encoding = strings.ToLower(encStr)
				}
			}
		}
	}

	return filePath, opts, nil
}
