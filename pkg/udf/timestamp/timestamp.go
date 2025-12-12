package timestamp

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterTimestampToDate registers the timestamp_to_date function with gojq
func RegisterTimestampToDate() gojq.CompilerOption {
	return gojq.WithFunction("timestamp_to_date", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return fmt.Errorf("timestamp_to_date: %v", err)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var timestamp int64
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return fmt.Errorf("timestamp_to_date: file argument requires string path, got %T", inputVal)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				return fmt.Errorf("timestamp_to_date: %v", err)
			}

			// Parse timestamp from file
			tsStr := strings.TrimSpace(string(fileData))
			ts, parseErr := strconv.ParseInt(tsStr, 10, 64)
			if parseErr != nil {
				return fmt.Errorf("timestamp_to_date: invalid timestamp in file: %v", parseErr)
			}
			timestamp = ts
			filePath = absPath
			fileSize = size
		} else {
			// Handle various number types including json.Number
			switch val := inputVal.(type) {
			case float64:
				timestamp = int64(val)
			case int:
				timestamp = int64(val)
			case int64:
				timestamp = val
			case string:
				ts, parseErr := strconv.ParseInt(val, 10, 64)
				if parseErr != nil {
					return fmt.Errorf("timestamp_to_date: invalid timestamp: %v", parseErr)
				}
				timestamp = ts
			default:
				// Try to convert to string and parse (handles json.Number and other types)
				valStr := fmt.Sprintf("%v", val)
				ts, parseErr := strconv.ParseInt(valStr, 10, 64)
				if parseErr != nil {
					return fmt.Errorf("timestamp_to_date: invalid timestamp %q: %v", valStr, parseErr)
				}
				timestamp = ts
			}
		}

		// Convert timestamp to date
		// Handle both seconds and milliseconds
		var t time.Time
		if timestamp > 1e10 {
			// Likely milliseconds
			t = time.Unix(timestamp/1000, (timestamp%1000)*1e6)
		} else {
			// Likely seconds
			t = time.Unix(timestamp, 0)
		}

		dateStr := t.Format(time.RFC3339)

		meta := map[string]any{
			"operation": "timestamp_to_date",
			"timestamp": timestamp,
			"format":    "RFC3339",
		}

		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
		}

		return map[string]any{
			"_val":  dateStr,
			"_meta": meta,
		}
	})
}

// RegisterDateToTimestamp registers the date_to_timestamp function with gojq
func RegisterDateToTimestamp() gojq.CompilerOption {
	return gojq.WithFunction("date_to_timestamp", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return fmt.Errorf("date_to_timestamp: %v", err)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var dateStr string
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return fmt.Errorf("date_to_timestamp: file argument requires string path, got %T", inputVal)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				return fmt.Errorf("date_to_timestamp: %v", err)
			}

			dateStr = strings.TrimSpace(string(fileData))
			filePath = absPath
			fileSize = size
		} else {
			switch val := inputVal.(type) {
			case string:
				dateStr = val
			case []byte:
				dateStr = string(val)
			default:
				if str, ok := val.(fmt.Stringer); ok {
					dateStr = str.String()
				} else {
					return fmt.Errorf("date_to_timestamp: argument must be a string, got %T", val)
				}
			}
		}

		// Try multiple date formats
		formats := []string{
			time.RFC3339,
			time.RFC3339Nano,
			time.RFC1123,
			time.RFC1123Z,
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05Z07:00",
			"2006-01-02",
		}

		var t time.Time
		var parseErr error
		for _, format := range formats {
			t, parseErr = time.Parse(format, dateStr)
			if parseErr == nil {
				break
			}
		}

		if parseErr != nil {
			return fmt.Errorf("date_to_timestamp: unable to parse date %q: %v", dateStr, parseErr)
		}

		timestamp := t.Unix()

		meta := map[string]any{
			"operation": "date_to_timestamp",
			"date":      dateStr,
			"timestamp": timestamp,
		}

		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
		}

		return map[string]any{
			"_val":  float64(timestamp),
			"_meta": meta,
		}
	})
}

