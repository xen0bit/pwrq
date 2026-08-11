// Package datetime provides PowerShell-style date and time cmdlets.
// This file implements Get-Date functionality.
package datetime

import (
	"fmt"
	"time"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// GetDateOptions holds options for get_date
type GetDateOptions struct {
	Date        any // Can be string (date to parse) or int (timestamp)
	Format      string
	DisplayHint string // Date, Time, DateTime
	Year        int
	YearSet     bool
	Month       int
	MonthSet    bool
	Day         int
	DaySet      bool
	Hour        int
	HourSet     bool
	Minute      int
	MinuteSet   bool
	Second      int
	SecondSet   bool
}

// RegisterGetDate registers the get_date function with gojq
// PowerShell compatibility: Get-Date
// Usage:
//   - get_date() - Current date/time
//   - get_date("2024-01-15") - Parse date string
//   - get_date(1704067200) - Unix timestamp
//   - get_date({"Format": "yyyy-MM-dd"})
//   - get_date("2024-01-15"; {"Format": "dddd, MMMM dd"})
func RegisterGetDate() gojq.CompilerOption {
	return gojq.WithFunction("get_date", 0, 2, func(v any, args []any) any {
		opts := GetDateOptions{}

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
					parseGetDateOptions(&opts, val)
				}
			}
		}

		// Second argument could be options map
		if len(args) > 1 {
			if secondArg := common.BindValue(args[1]); secondArg != nil {
				if optsMap, ok := secondArg.(map[string]any); ok {
					// Merge options (second arg takes precedence)
					parseGetDateOptions(&opts, optsMap)
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

		// Get the time value
		var resultTime time.Time

		if opts.Date == nil {
			// No date specified, use current time
			resultTime = time.Now()
		} else {
			switch val := opts.Date.(type) {
			case string:
				// Try to parse the date string
				parsed, err := parseDateString(val)
				if err != nil {
					return common.MakeUDFErrorResult(fmt.Errorf("get_date: failed to parse date %q: %w", val, err), nil)
				}
				resultTime = parsed
			case int:
				// Unix timestamp
				resultTime = time.Unix(int64(val), 0)
			}
		}

		// Apply date component overrides if specified
		if opts.YearSet {
			resultTime = time.Date(opts.Year, resultTime.Month(), resultTime.Day(),
				resultTime.Hour(), resultTime.Minute(), resultTime.Second(),
				resultTime.Nanosecond(), resultTime.Location())
		}
		if opts.MonthSet {
			resultTime = time.Date(resultTime.Year(), time.Month(opts.Month), resultTime.Day(),
				resultTime.Hour(), resultTime.Minute(), resultTime.Second(),
				resultTime.Nanosecond(), resultTime.Location())
		}
		if opts.DaySet {
			resultTime = time.Date(resultTime.Year(), resultTime.Month(), opts.Day,
				resultTime.Hour(), resultTime.Minute(), resultTime.Second(),
				resultTime.Nanosecond(), resultTime.Location())
		}
		if opts.HourSet {
			resultTime = time.Date(resultTime.Year(), resultTime.Month(), resultTime.Day(),
				opts.Hour, resultTime.Minute(), resultTime.Second(),
				resultTime.Nanosecond(), resultTime.Location())
		}
		if opts.MinuteSet {
			resultTime = time.Date(resultTime.Year(), resultTime.Month(), resultTime.Day(),
				resultTime.Hour(), opts.Minute, resultTime.Second(),
				resultTime.Nanosecond(), resultTime.Location())
		}
		if opts.SecondSet {
			resultTime = time.Date(resultTime.Year(), resultTime.Month(), resultTime.Day(),
				resultTime.Hour(), resultTime.Minute(), opts.Second,
				resultTime.Nanosecond(), resultTime.Location())
		}

		// Update $? automatic variable
		ss := common.GetSessionState()
		if ss != nil {
			_ = ss.SetVariable("?", true, sessionstate.None)
		}

		// Format the output if Format is specified
		if opts.Format != "" {
			formatted := formatDateTime(resultTime, opts.Format)
			return common.MakeUDFSuccessResult(formatted, map[string]any{
				"operation": "get_date",
			})
		}

		// Return a structured date object
		dateObj := map[string]any{
			"DateTime":    resultTime.Format(time.RFC3339),
			"Date":        resultTime.Format("2006-01-02"),
			"Time":        resultTime.Format("15:04:05"),
			"Year":        resultTime.Year(),
			"Month":       int(resultTime.Month()),
			"MonthName":   resultTime.Month().String(),
			"Day":         resultTime.Day(),
			"DayOfWeek":   resultTime.Weekday().String(),
			"DayOfYear":   resultTime.YearDay(),
			"Hour":        resultTime.Hour(),
			"Minute":      resultTime.Minute(),
			"Second":      resultTime.Second(),
			"Millisecond": resultTime.Nanosecond() / 1000000,
			"Timestamp":   int(resultTime.Unix()),
			"Timezone":    resultTime.Location().String(),
			"IsDST":       resultTime.IsDST(),
		}

		// Apply DisplayHint if specified
		switch opts.DisplayHint {
		case "Date":
			return common.MakeUDFSuccessResult(resultTime.Format("2006-01-02"), map[string]any{
				"operation": "get_date",
			})
		case "Time":
			return common.MakeUDFSuccessResult(resultTime.Format("15:04:05"), map[string]any{
				"operation": "get_date",
			})
		default:
			return common.MakeUDFSuccessResult(dateObj, map[string]any{
				"operation": "get_date",
			})
		}
	})
}

// parseGetDateOptions parses options from a map
func parseGetDateOptions(opts *GetDateOptions, optsMap map[string]any) {
	if formatVal, exists := optsMap["Format"]; exists {
		if fStr, ok := formatVal.(string); ok {
			opts.Format = fStr
		}
	}
	if displayHintVal, exists := optsMap["DisplayHint"]; exists {
		if dStr, ok := displayHintVal.(string); ok {
			opts.DisplayHint = dStr
		}
	}
	if yearVal, exists := optsMap["Year"]; exists {
		switch y := yearVal.(type) {
		case int:
			opts.Year = y
			opts.YearSet = true
		case float64:
			opts.Year = int(y)
			opts.YearSet = true
		}
	}
	if monthVal, exists := optsMap["Month"]; exists {
		switch m := monthVal.(type) {
		case int:
			opts.Month = m
			opts.MonthSet = true
		case float64:
			opts.Month = int(m)
			opts.MonthSet = true
		}
	}
	if dayVal, exists := optsMap["Day"]; exists {
		switch d := dayVal.(type) {
		case int:
			opts.Day = d
			opts.DaySet = true
		case float64:
			opts.Day = int(d)
			opts.DaySet = true
		}
	}
	if hourVal, exists := optsMap["Hour"]; exists {
		switch h := hourVal.(type) {
		case int:
			opts.Hour = h
			opts.HourSet = true
		case float64:
			opts.Hour = int(h)
			opts.HourSet = true
		}
	}
	if minuteVal, exists := optsMap["Minute"]; exists {
		switch m := minuteVal.(type) {
		case int:
			opts.Minute = m
			opts.MinuteSet = true
		case float64:
			opts.Minute = int(m)
			opts.MinuteSet = true
		}
	}
	if secondVal, exists := optsMap["Second"]; exists {
		switch s := secondVal.(type) {
		case int:
			opts.Second = s
			opts.SecondSet = true
		case float64:
			opts.Second = int(s)
			opts.SecondSet = true
		}
	}
}

// parseDateString attempts to parse a date string using multiple formats
func parseDateString(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
		"01/02/2006",
		"02/01/2006",
		"2006/01/02",
		"January 2, 2006",
		"Jan 2, 2006",
		"2 Jan 2006",
		"Mon Jan 2 15:04:05 2006",
		time.ANSIC,
		time.UnixDate,
		time.RubyDate,
		time.RFC822,
		time.RFC822Z,
		time.RFC850,
		time.RFC1123,
		time.RFC1123Z,
	}

	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse date")
}

// formatDateTime formats a time according to PowerShell-style format strings
func formatDateTime(t time.Time, format string) string {
	// Use a state machine to parse the format string
	result := ""
	i := 0
	for i < len(format) {
		// Check for quoted strings (literal text)
		if format[i] == '\'' || format[i] == '"' {
			quote := format[i]
			i++
			start := i
			for i < len(format) && format[i] != quote {
				i++
			}
			result += format[start:i]
			if i < len(format) {
				i++ // skip closing quote
			}
			continue
		}

		// Check for format patterns (longest first)
		remaining := format[i:]
		replaced := false

		// Year patterns
		if len(remaining) >= 4 && remaining[:4] == "yyyy" {
			result += t.Format("2006")
			i += 4
			replaced = true
		} else if len(remaining) >= 2 && remaining[:2] == "yy" {
			result += t.Format("06")
			i += 2
			replaced = true
		} else if len(remaining) >= 4 && remaining[:4] == "MMMM" {
			// Month name patterns
			result += t.Format("January")
			i += 4
			replaced = true
		} else if len(remaining) >= 3 && remaining[:3] == "MMM" {
			result += t.Format("Jan")
			i += 3
			replaced = true
		} else if len(remaining) >= 4 && remaining[:4] == "dddd" {
			// Day name patterns
			result += t.Format("Monday")
			i += 4
			replaced = true
		} else if len(remaining) >= 3 && remaining[:3] == "ddd" {
			result += t.Format("Mon")
			i += 3
			replaced = true
		} else if len(remaining) >= 2 && remaining[:2] == "HH" {
			// Time patterns (must check before month/day number patterns)
			result += t.Format("15")
			i += 2
			replaced = true
		} else if len(remaining) >= 2 && remaining[:2] == "hh" {
			result += t.Format("03")
			i += 2
			replaced = true
		} else if len(remaining) >= 2 && remaining[:2] == "mm" {
			result += t.Format("04")
			i += 2
			replaced = true
		} else if len(remaining) >= 2 && remaining[:2] == "ss" {
			result += t.Format("05")
			i += 2
			replaced = true
		} else if len(remaining) >= 2 && remaining[:2] == "tt" {
			result += t.Format("PM")
			i += 2
			replaced = true
		} else if len(remaining) >= 3 && remaining[:3] == "fff" {
			result += fmt.Sprintf("%03d", t.Nanosecond()/1000000)
			i += 3
			replaced = true
		} else if len(remaining) >= 2 && remaining[:2] == "ff" {
			result += fmt.Sprintf("%02d", t.Nanosecond()/10000000)
			i += 2
			replaced = true
		} else if len(remaining) >= 1 && remaining[:1] == "f" {
			result += fmt.Sprintf("%d", t.Nanosecond()/100000000)
			i += 1
			replaced = true
		} else if len(remaining) >= 2 && remaining[:2] == "MM" {
			// Month/Day number patterns
			result += t.Format("01")
			i += 2
			replaced = true
		} else if len(remaining) >= 2 && remaining[:2] == "dd" {
			result += t.Format("02")
			i += 2
			replaced = true
		} else if len(remaining) >= 2 && remaining[:2] == "zz" {
			_, offset := t.Zone()
			offset = offset / 3600 // convert to hours
			if offset >= 0 {
				result += fmt.Sprintf("+%02d", offset)
			} else {
				result += fmt.Sprintf("-%02d", -offset)
			}
			i += 2
			replaced = true
		} else if len(remaining) >= 1 && remaining[:1] == "H" {
			// Single character patterns
			result += fmt.Sprintf("%d", t.Hour())
			i += 1
			replaced = true
		} else if len(remaining) >= 1 && remaining[:1] == "h" {
			hour := t.Hour()
			if hour == 0 {
				hour = 12
			} else if hour > 12 {
				hour -= 12
			}
			result += fmt.Sprintf("%d", hour)
			i += 1
			replaced = true
		} else if len(remaining) >= 1 && remaining[:1] == "m" {
			result += fmt.Sprintf("%d", t.Minute())
			i += 1
			replaced = true
		} else if len(remaining) >= 1 && remaining[:1] == "s" {
			result += fmt.Sprintf("%d", t.Second())
			i += 1
			replaced = true
		} else if len(remaining) >= 1 && remaining[:1] == "t" {
			result += t.Format("pm")
			i += 1
			replaced = true
		} else if len(remaining) >= 1 && remaining[:1] == "M" {
			result += fmt.Sprintf("%d", t.Month())
			i += 1
			replaced = true
		} else if len(remaining) >= 1 && remaining[:1] == "d" {
			result += fmt.Sprintf("%d", t.Day())
			i += 1
			replaced = true
		} else if len(remaining) >= 1 && remaining[:1] == "z" {
			_, offset := t.Zone()
			offset = offset / 3600
			result += fmt.Sprintf("%d", offset)
			i += 1
			replaced = true
		}

		if !replaced {
			// Not a recognized pattern, copy literal character
			result += string(format[i])
			i++
		}
	}

	return result
}
