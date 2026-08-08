package datetime

import (
	"testing"
	"time"
)

func TestParseDateString(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"RFC3339", "2024-01-15T10:30:00Z", false},
		{"ISO8601", "2024-01-15T10:30:00", false},
		{"Date only", "2024-01-15", false},
		{"US format", "01/15/2024", false},
		{"Long format", "January 15, 2024", false},
		{"Short format", "Jan 15, 2024", false},
		{"Invalid", "not-a-date", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseDateString(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDateString(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestFormatDateTime(t *testing.T) {
	testTime := time.Date(2024, 1, 15, 14, 30, 45, 123000000, time.UTC)

	tests := []struct {
		format string
		want   string
	}{
		{"yyyy-MM-dd", "2024-01-15"},
		{"yyyy/MM/dd", "2024/01/15"},
		{"dd/MM/yyyy", "15/01/2024"},
		{"MMMM dd, yyyy", "January 15, 2024"},
		{"MMM dd, yyyy", "Jan 15, 2024"},
		{"dddd, MMMM dd", "Monday, January 15"},
		{"HH:mm:ss", "14:30:45"},
		{"yyyy-MM-dd HH:mm:ss", "2024-01-15 14:30:45"},
		{"yyyy-MM-ddTHH:mm:ss", "2024-01-15T14:30:45"},
		{"MM/dd/yyyy hh:mm tt", "01/15/2024 02:30 PM"},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			got := formatDateTime(testTime, tt.format)
			if got != tt.want {
				t.Errorf("formatDateTime(%q) = %v, want %v", tt.format, got, tt.want)
			}
		})
	}
}

func TestCreateTimeSpan(t *testing.T) {
	tests := []struct {
		name        string
		duration    time.Duration
		wantDays    int
		wantHours   int
		wantMinutes int
		wantSeconds int
	}{
		{"Zero", 0, 0, 0, 0, 0},
		{"One hour", time.Hour, 0, 1, 0, 0},
		{"One day", 24 * time.Hour, 1, 0, 0, 0},
		{"Complex", 25*time.Hour + 30*time.Minute + 45*time.Second, 1, 1, 30, 45},
		{"With milliseconds", time.Hour + 500*time.Millisecond, 0, 1, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := createTimeSpan(tt.duration)
			if got.Days != tt.wantDays {
				t.Errorf("Days = %d, want %d", got.Days, tt.wantDays)
			}
			if got.Hours != tt.wantHours {
				t.Errorf("Hours = %d, want %d", got.Hours, tt.wantHours)
			}
			if got.Minutes != tt.wantMinutes {
				t.Errorf("Minutes = %d, want %d", got.Minutes, tt.wantMinutes)
			}
			if got.Seconds != tt.wantSeconds {
				t.Errorf("Seconds = %d, want %d", got.Seconds, tt.wantSeconds)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{"Zero", 0, "00:00:00.0000000"},
		{"One hour", time.Hour, "01:00:00.0000000"},
		{"One day", 24 * time.Hour, "1.00:00:00.0000000"},
		{"Complex", 25*time.Hour + 30*time.Minute + 45*time.Second + 123*time.Millisecond, "1.01:30:45.1230000"},
		{"Sub-millisecond precision", time.Hour + 500*time.Millisecond + 500*time.Microsecond, "01:00:00.5005000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.duration)
			if got != tt.want {
				t.Errorf("formatDuration(%v) = %v, want %v", tt.duration, got, tt.want)
			}
		})
	}
}

func TestGetDateOptionsZeroValues(t *testing.T) {
	// Test that zero values for Hour, Minute, Second are properly recognized
	tests := []struct {
		name      string
		optsMap   map[string]any
		wantHour  int
		wantMin   int
		wantSec   int
		hourSet   bool
		minuteSet bool
		secondSet bool
	}{
		{"Hour zero", map[string]any{"Hour": 0}, 0, 0, 0, true, false, false},
		{"Minute zero", map[string]any{"Minute": 0}, 0, 0, 0, false, true, false},
		{"Second zero", map[string]any{"Second": 0}, 0, 0, 0, false, false, true},
		{"All zero", map[string]any{"Hour": 0, "Minute": 0, "Second": 0}, 0, 0, 0, true, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := GetDateOptions{}
			parseGetDateOptions(&opts, tt.optsMap)

			if opts.Hour != tt.wantHour {
				t.Errorf("Hour = %d, want %d", opts.Hour, tt.wantHour)
			}
			if opts.Minute != tt.wantMin {
				t.Errorf("Minute = %d, want %d", opts.Minute, tt.wantMin)
			}
			if opts.Second != tt.wantSec {
				t.Errorf("Second = %d, want %d", opts.Second, tt.wantSec)
			}
			if opts.HourSet != tt.hourSet {
				t.Errorf("HourSet = %v, want %v", opts.HourSet, tt.hourSet)
			}
			if opts.MinuteSet != tt.minuteSet {
				t.Errorf("MinuteSet = %v, want %v", opts.MinuteSet, tt.minuteSet)
			}
			if opts.SecondSet != tt.secondSet {
				t.Errorf("SecondSet = %v, want %v", opts.SecondSet, tt.secondSet)
			}
		})
	}
}
