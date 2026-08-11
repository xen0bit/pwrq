// Package datetime provides PowerShell-style date and time cmdlets.
// This file implements New-TimeSpan functionality.
package datetime

import (
	"fmt"
	"time"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// TimeSpan represents a time interval
type TimeSpan struct {
	Days         int     `json:"Days"`
	Hours        int     `json:"Hours"`
	Minutes      int     `json:"Minutes"`
	Seconds      int     `json:"Seconds"`
	Milliseconds int     `json:"Milliseconds"`
	Ticks        int64   `json:"Ticks"`
	TotalDays    float64 `json:"TotalDays"`
	TotalHours   float64 `json:"TotalHours"`
	TotalMinutes float64 `json:"TotalMinutes"`
	TotalSeconds float64 `json:"TotalSeconds"`
	Duration     string  `json:"Duration"`
}

// NewTimeSpanOptions holds options for new_timespan
type NewTimeSpanOptions struct {
	Start        any // Start time (string, int timestamp, or map with DateTime)
	End          any // End time (string, int timestamp, or map with DateTime)
	Days         int
	Hours        int
	Minutes      int
	Seconds      int
	Milliseconds int
}

// RegisterNewTimeSpan registers the new_timespan function with gojq
// PowerShell compatibility: New-TimeSpan
// Usage:
//   - new_timespan() - TimeSpan from now to now (zero)
//   - new_timespan({"Days": 1; "Hours": 2}) - Create TimeSpan with duration
//   - new_timespan({"Start": "2024-01-01"; "End": "2024-12-31"}) - TimeSpan between dates
//   - new_timespan("2024-01-01"; "2024-12-31") - Start and End as separate args
func RegisterNewTimeSpan() gojq.CompilerOption {
	return gojq.WithFunction("new_timespan", 0, 2, func(v any, args []any) any {
		opts := NewTimeSpanOptions{}

		// Parse arguments
		if len(args) > 0 {
			firstArg := common.BindValue(args[0])

			if firstArg != nil {
				switch val := firstArg.(type) {
				case string:
					// First argument is Start date
					opts.Start = val
					if len(args) > 1 {
						if secondArg := common.BindValue(args[1]); secondArg != nil {
							switch val2 := secondArg.(type) {
							case string:
								opts.End = val2
							case int:
								opts.End = val2
							case float64:
								opts.End = int(val2)
							}
						}
					}
				case map[string]any:
					parseNewTimeSpanOptions(&opts, val)
				case int:
					// Treat as timestamp for Start
					opts.Start = val
				case float64:
					opts.Start = int(val)
				}
			}
		}

		// Parse Start and End times
		var startTime, endTime time.Time
		var hasStart, hasEnd bool

		if opts.Start != nil {
			switch val := opts.Start.(type) {
			case string:
				parsed, err := parseDateString(val)
				if err != nil {
					return common.MakeUDFErrorResult(fmt.Errorf("new_timespan: failed to parse Start date %q: %w", val, err), nil)
				}
				startTime = parsed
				hasStart = true
			case int:
				startTime = time.Unix(int64(val), 0)
				hasStart = true
			}
		}

		if opts.End != nil {
			switch val := opts.End.(type) {
			case string:
				parsed, err := parseDateString(val)
				if err != nil {
					return common.MakeUDFErrorResult(fmt.Errorf("new_timespan: failed to parse End date %q: %w", val, err), nil)
				}
				endTime = parsed
				hasEnd = true
			case int:
				endTime = time.Unix(int64(val), 0)
				hasEnd = true
			}
		}

		// Calculate the duration
		var duration time.Duration

		if hasStart && hasEnd {
			// TimeSpan between two dates
			duration = endTime.Sub(startTime)
		} else if hasStart && !hasEnd {
			// TimeSpan from Start to now
			duration = time.Since(startTime)
		} else if !hasStart && hasEnd {
			// TimeSpan from now to End
			duration = endTime.Sub(time.Now())
		} else {
			// No dates specified, use duration components
			duration = time.Duration(opts.Days)*24*time.Hour +
				time.Duration(opts.Hours)*time.Hour +
				time.Duration(opts.Minutes)*time.Minute +
				time.Duration(opts.Seconds)*time.Second +
				time.Duration(opts.Milliseconds)*time.Millisecond
		}

		// Create TimeSpan object
		timeSpan := createTimeSpan(duration)

		// Update $? automatic variable
		ss := common.GetSessionState()
		if ss != nil {
			ss.SetVariable("?", true, sessionstate.None)
		}

		return common.MakeUDFSuccessResult(timeSpanToMap(timeSpan), map[string]any{
			"operation": "new_timespan",
		})
	})
}

// parseNewTimeSpanOptions parses options from a map
func parseNewTimeSpanOptions(opts *NewTimeSpanOptions, optsMap map[string]any) {
	if startVal, exists := optsMap["Start"]; exists {
		opts.Start = startVal
	}
	if endVal, exists := optsMap["End"]; exists {
		opts.End = endVal
	}
	if daysVal, exists := optsMap["Days"]; exists {
		switch d := daysVal.(type) {
		case int:
			opts.Days = d
		case float64:
			opts.Days = int(d)
		}
	}
	if hoursVal, exists := optsMap["Hours"]; exists {
		switch h := hoursVal.(type) {
		case int:
			opts.Hours = h
		case float64:
			opts.Hours = int(h)
		}
	}
	if minutesVal, exists := optsMap["Minutes"]; exists {
		switch m := minutesVal.(type) {
		case int:
			opts.Minutes = m
		case float64:
			opts.Minutes = int(m)
		}
	}
	if secondsVal, exists := optsMap["Seconds"]; exists {
		switch s := secondsVal.(type) {
		case int:
			opts.Seconds = s
		case float64:
			opts.Seconds = int(s)
		}
	}
	if msVal, exists := optsMap["Milliseconds"]; exists {
		switch m := msVal.(type) {
		case int:
			opts.Milliseconds = m
		case float64:
			opts.Milliseconds = int(m)
		}
	}
}

// createTimeSpan creates a TimeSpan struct from a duration
func createTimeSpan(duration time.Duration) TimeSpan {
	// Handle negative durations
	negative := duration < 0
	absDuration := duration
	if negative {
		absDuration = -duration
	}

	// Extract components using proper remainder calculations
	days := int(absDuration / (24 * time.Hour))
	remaining := absDuration - time.Duration(days)*24*time.Hour

	hours := int(remaining / time.Hour)
	remaining = remaining - time.Duration(hours)*time.Hour

	minutes := int(remaining / time.Minute)
	remaining = remaining - time.Duration(minutes)*time.Minute

	seconds := int(remaining / time.Second)
	remaining = remaining - time.Duration(seconds)*time.Second

	milliseconds := int(remaining / time.Millisecond)

	// 1 tick = 100 nanoseconds
	ticks := absDuration.Nanoseconds() / 100

	totalDays := float64(absDuration) / float64(24*time.Hour)
	totalHours := float64(absDuration) / float64(time.Hour)
	totalMinutes := float64(absDuration) / float64(time.Minute)
	totalSeconds := float64(absDuration) / float64(time.Second)

	// Format duration as string (e.g., "1.02:03:04.0050000")
	durationStr := formatDuration(duration)

	return TimeSpan{
		Days:         days,
		Hours:        hours,
		Minutes:      minutes,
		Seconds:      seconds,
		Milliseconds: milliseconds,
		Ticks:        ticks,
		TotalDays:    totalDays,
		TotalHours:   totalHours,
		TotalMinutes: totalMinutes,
		TotalSeconds: totalSeconds,
		Duration:     durationStr,
	}
}

// formatDuration formats a duration in PowerShell TimeSpan format
func formatDuration(d time.Duration) string {
	// Handle negative durations
	negative := d < 0
	if negative {
		d = -d
	}

	// Extract components using proper remainder calculations
	days := int(d / (24 * time.Hour))
	remaining := d - time.Duration(days)*24*time.Hour

	hours := int(remaining / time.Hour)
	remaining = remaining - time.Duration(hours)*time.Hour

	minutes := int(remaining / time.Minute)
	remaining = remaining - time.Duration(minutes)*time.Minute

	seconds := int(remaining / time.Second)
	remaining = remaining - time.Duration(seconds)*time.Second

	// Get fractional seconds as 7-digit ticks (1 tick = 100 nanoseconds)
	// This gives us the full 7-digit precision for fractional seconds
	fractionalTicks := int(remaining.Nanoseconds() / 100)

	if days > 0 {
		if negative {
			return fmt.Sprintf("-%d.%02d:%02d:%02d.%07d", days, hours, minutes, seconds, fractionalTicks)
		}
		return fmt.Sprintf("%d.%02d:%02d:%02d.%07d", days, hours, minutes, seconds, fractionalTicks)
	}

	if negative {
		return fmt.Sprintf("-%02d:%02d:%02d.%07d", hours, minutes, seconds, fractionalTicks)
	}
	return fmt.Sprintf("%02d:%02d:%02d.%07d", hours, minutes, seconds, fractionalTicks)
}

// timeSpanToMap converts a TimeSpan to a map for JSON encoding
func timeSpanToMap(ts TimeSpan) map[string]any {
	return map[string]any{
		"Days":         ts.Days,
		"Hours":        ts.Hours,
		"Minutes":      ts.Minutes,
		"Seconds":      ts.Seconds,
		"Milliseconds": ts.Milliseconds,
		"Ticks":        int(ts.Ticks),
		"TotalDays":    ts.TotalDays,
		"TotalHours":   ts.TotalHours,
		"TotalMinutes": ts.TotalMinutes,
		"TotalSeconds": ts.TotalSeconds,
		"Duration":     ts.Duration,
	}
}
