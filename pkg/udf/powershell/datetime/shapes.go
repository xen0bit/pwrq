package datetime

import "github.com/xen0bit/pwrq/pkg/core/shape"

var (
	// DateShape is a moment in time, broken into the parts Get-Date exposes. It is deliberately redundant - Date, Time and DateTime
	// are the same instant rendered three ways - because the point of the
	// cmdlet is to save the caller from formatting one.
	DateShape = shape.Fixed("Pwrq.DateTime",
		shape.Prop("DateTime", shape.String, "the full instant, RFC 3339"),
		shape.Prop("Date", shape.String, "calendar date, YYYY-MM-DD"),
		shape.Prop("Time", shape.String, "wall-clock time, HH:MM:SS"),
		shape.Prop("Year", shape.Number, "four-digit year"),
		shape.Prop("Month", shape.Number, "month, 1-12"),
		shape.Prop("MonthName", shape.String, "month name in English"),
		shape.Prop("Day", shape.Number, "day of the month, 1-31"),
		shape.Prop("DayOfWeek", shape.String, "weekday name in English"),
		shape.Prop("DayOfYear", shape.Number, "day of the year, 1-366"),
		shape.Prop("Hour", shape.Number, "hour, 0-23"),
		shape.Prop("Minute", shape.Number, "minute, 0-59"),
		shape.Prop("Second", shape.Number, "second, 0-59"),
		shape.Prop("Millisecond", shape.Number, "millisecond, 0-999"),
		shape.Prop("Timestamp", shape.Number, "seconds since the Unix epoch"),
		shape.Prop("Timezone", shape.String, "zone name, or Local"),
		shape.Prop("IsDST", shape.Boolean, "whether daylight saving is in effect"),
	).Note("a Format option, or a DisplayHint of Date or Time, returns a formatted string instead of this object")

	// TimeSpanShape is a duration. The Total* properties are the whole span
	// expressed in one unit; the bare ones are the remainder after the larger
	// units have been taken out, which is the distinction callers most often
	// get backwards.
	TimeSpanShape = shape.Fixed("Pwrq.TimeSpan",
		shape.Prop("Days", shape.Number, "whole days component"),
		shape.Prop("Hours", shape.Number, "hours component, 0-23"),
		shape.Prop("Minutes", shape.Number, "minutes component, 0-59"),
		shape.Prop("Seconds", shape.Number, "seconds component, 0-59"),
		shape.Prop("Milliseconds", shape.Number, "milliseconds component, 0-999"),
		shape.Prop("TotalDays", shape.Number, "the whole span in days, fractional"),
		shape.Prop("TotalHours", shape.Number, "the whole span in hours, fractional"),
		shape.Prop("TotalMinutes", shape.Number, "the whole span in minutes, fractional"),
		shape.Prop("TotalSeconds", shape.Number, "the whole span in seconds, fractional"),
		shape.Prop("Ticks", shape.Number, "the span in 100-nanosecond ticks"),
		shape.Prop("Duration", shape.String, "the span formatted as d.hh:mm:ss.fffffff"),
	)
)
