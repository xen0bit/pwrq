// Package datetime provides PowerShell-style date and time cmdlets.
// This file implements Set-Date functionality.
package datetime

import (
	"fmt"
	"time"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// SetDateOptions holds options for set_date
type SetDateOptions struct {
	Date any // Can be string (date to parse) or int (timestamp)
}

// RegisterSetDate registers the set_date function with gojq
// PowerShell compatibility: Set-Date
// Usage:
//   - set_date("2024-01-15") - Set system date (requires privileges)
//   - set_date(1704067200) - Set from Unix timestamp
//   - set_date({"Date": "2024-01-15T12:00:00Z"})
//
// Note: On Unix systems, this typically requires root privileges
func RegisterSetDate() gojq.CompilerOption {
	return gojq.WithFunction("set_date", 0, 2, func(v any, args []any) any {
		opts := SetDateOptions{}

		// Parse arguments
		if len(args) > 0 {
			firstArg := common.BindValue(args[0])

			if firstArg != nil {
				switch val := firstArg.(type) {
				case string:
					opts.Date = val
				case int:
					opts.Date = val
				case float64:
					opts.Date = int(val)
				case map[string]any:
					if dateVal, exists := val["Date"]; exists {
						opts.Date = dateVal
					}
				}
			}
		}

		// Second argument could be options map
		if len(args) > 1 {
			if secondArg := common.BindValue(args[1]); secondArg != nil {
				if optsMap, ok := secondArg.(map[string]any); ok {
					if dateVal, exists := optsMap["Date"]; exists {
						opts.Date = dateVal
					}
				}
			}
		}

		// If no date specified, try to get from pipeline input
		if opts.Date == nil {
			if pipelineVal := common.BindValue(v); pipelineVal != nil {
				switch val := pipelineVal.(type) {
				case string:
					opts.Date = val
				case int:
					opts.Date = val
				case float64:
					opts.Date = int(val)
				}
			}
		}

		// Validate date is provided
		if opts.Date == nil {
			return common.MakeUDFErrorResult(fmt.Errorf("set_date: Date is required"), nil)
		}

		// Parse the date
		var resultTime time.Time
		switch val := opts.Date.(type) {
		case string:
			parsed, err := parseDateString(val)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("set_date: failed to parse date %q: %w", val, err), nil)
			}
			resultTime = parsed
		case int:
			resultTime = time.Unix(int64(val), 0)
		default:
			return common.MakeUDFErrorResult(fmt.Errorf("set_date: Date must be a string or Unix timestamp"), nil)
		}

		// Attempt to set the system time
		// Note: This requires appropriate privileges
		err := setSystemTime(resultTime)
		if err != nil {
			// Update $? automatic variable
			ss := common.GetSessionState()
			if ss != nil {
				_ = ss.SetVariable("?", false, sessionstate.None)
			}

			return common.MakeUDFErrorResult(fmt.Errorf("set_date: failed to set system time: %w", err), map[string]any{
				"operation": "set_date",
				"date":      resultTime.Format(time.RFC3339),
			})
		}

		// Update $? automatic variable
		ss := common.GetSessionState()
		if ss != nil {
			_ = ss.SetVariable("?", true, sessionstate.None)
		}

		// Return the new system time
		newTime := time.Now()
		return common.MakeUDFSuccessResult(map[string]any{
			"DateTime":  newTime.Format(time.RFC3339),
			"Date":      newTime.Format("2006-01-02"),
			"Time":      newTime.Format("15:04:05"),
			"Year":      newTime.Year(),
			"Month":     int(newTime.Month()),
			"Day":       newTime.Day(),
			"Hour":      newTime.Hour(),
			"Minute":    newTime.Minute(),
			"Second":    newTime.Second(),
			"Timestamp": newTime.Unix(),
		}, map[string]any{
			"operation": "set_date",
			"success":   true,
		})
	})
}

// setSystemTime sets the system time
// Platform-specific implementations are in set_date_unix.go and set_date_windows.go
// This operation requires elevated privileges (root on Unix, Administrator on Windows)
