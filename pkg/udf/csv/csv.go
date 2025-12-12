package csv

import (
	"encoding/csv"
	"fmt"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterCSVParse registers the csv_parse function with gojq
func RegisterCSVParse() gojq.CompilerOption {
	return gojq.WithFunction("csv_parse", 0, 3, func(v any, args []any) any {
		// Parse arguments: optional delimiter, optional file flag
		var delimiter rune = ','
		var inputVal any
		var isFile bool

		if len(args) > 0 {
			// Check if first arg is delimiter (string) or file flag (bool)
			if delimStr, ok := args[0].(string); ok && len(delimStr) > 0 {
				delimiter = rune(delimStr[0])
				// Check for file flag as second arg
				if len(args) > 1 {
					if fileFlag, ok := args[1].(bool); ok {
						isFile = fileFlag
						inputVal = v
					} else {
						inputVal = args[1]
						if len(args) > 2 {
							if fileFlag, ok := args[2].(bool); ok {
								isFile = fileFlag
							}
						}
					}
				} else {
					inputVal = v
				}
			} else if fileFlag, ok := args[0].(bool); ok {
				isFile = fileFlag
				inputVal = v
			} else {
				inputVal = args[0]
			}
		} else {
			inputVal = v
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var input string
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return fmt.Errorf("csv_parse: file argument requires string path, got %T", inputVal)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				return fmt.Errorf("csv_parse: %v", err)
			}

			input = string(fileData)
			filePath = absPath
			fileSize = size
		} else {
			switch val := inputVal.(type) {
			case string:
				input = val
			case []byte:
				input = string(val)
			default:
				if str, ok := val.(fmt.Stringer); ok {
					input = str.String()
				} else {
					return fmt.Errorf("csv_parse: argument must be a string, got %T", val)
				}
			}
		}

		// Parse CSV
		reader := csv.NewReader(strings.NewReader(input))
		reader.Comma = delimiter
		records, err := reader.ReadAll()
		if err != nil {
			return fmt.Errorf("csv_parse: failed to parse CSV: %v", err)
		}

		// Convert to array of arrays
		result := make([]any, len(records))
		for i, record := range records {
			row := make([]any, len(record))
			for j, field := range record {
				row[j] = field
			}
			result[i] = row
		}

		meta := map[string]any{
			"operation": "csv_parse",
			"delimiter": string(delimiter),
			"rows":      len(records),
		}

		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
		} else {
			meta["input_length"] = len(input)
		}

		// Return array directly (not wrapped in _val/_meta) for easier manipulation
		return result
	})
}

// RegisterCSVStringify registers the csv_stringify function with gojq
func RegisterCSVStringify() gojq.CompilerOption {
	return gojq.WithFunction("csv_stringify", 0, 3, func(v any, args []any) any {
		// Parse arguments: optional delimiter, optional file flag
		var delimiter rune = ','
		var inputVal any
		var isFile bool

		if len(args) > 0 {
			// Check if first arg is delimiter (string) or file flag (bool)
			if delimStr, ok := args[0].(string); ok && len(delimStr) > 0 {
				delimiter = rune(delimStr[0])
				// Check for file flag as second arg
				if len(args) > 1 {
					if fileFlag, ok := args[1].(bool); ok {
						isFile = fileFlag
						inputVal = v
					} else {
						inputVal = args[1]
						if len(args) > 2 {
							if fileFlag, ok := args[2].(bool); ok {
								isFile = fileFlag
							}
						}
					}
				} else {
					inputVal = v
				}
			} else if fileFlag, ok := args[0].(bool); ok {
				isFile = fileFlag
				inputVal = v
			} else {
				inputVal = args[0]
			}
		} else {
			inputVal = v
		}

		inputVal = common.ExtractUDFValue(inputVal)

		// Input should be an array of arrays
		var records [][]string
		switch val := inputVal.(type) {
		case []any:
			records = make([][]string, len(val))
			for i, row := range val {
				switch rowVal := row.(type) {
				case []any:
					records[i] = make([]string, len(rowVal))
					for j, field := range rowVal {
						records[i][j] = fmt.Sprintf("%v", field)
					}
				default:
					return fmt.Errorf("csv_stringify: each row must be an array, got %T at index %d", rowVal, i)
				}
			}
		default:
			return fmt.Errorf("csv_stringify: input must be an array of arrays, got %T", val)
		}

		// Convert to CSV
		var buf strings.Builder
		writer := csv.NewWriter(&buf)
		writer.Comma = delimiter
		if err := writer.WriteAll(records); err != nil {
			return fmt.Errorf("csv_stringify: failed to write CSV: %v", err)
		}
		writer.Flush()

		result := buf.String()

		meta := map[string]any{
			"operation": "csv_stringify",
			"delimiter": string(delimiter),
			"rows":      len(records),
			"output_length": len(result),
		}

		if isFile {
			filePathStr, ok := inputVal.(string)
			if ok {
				_, absPath, size, err := common.ReadFileFromPath(filePathStr)
				if err == nil {
					meta["file_path"] = absPath
					meta["file_size"] = int(size)
				}
			}
		}

		return map[string]any{
			"_val":  result,
			"_meta": meta,
		}
	})
}

