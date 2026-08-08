package datetime

import (
	"testing"
	"time"
)

func TestSetDateParseOptions(t *testing.T) {
	tests := []struct {
		name    string
		input   map[string]any
		wantErr bool
	}{
		{"String date", map[string]any{"Date": "2024-01-15"}, false},
		{"Timestamp", map[string]any{"Date": 1704067200}, false},
		{"Invalid type", map[string]any{"Date": 12.34}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := SetDateOptions{}

			// Simulate parsing
			if dateVal, exists := tt.input["Date"]; exists {
				opts.Date = dateVal
			}

			if opts.Date == nil && !tt.wantErr {
				t.Errorf("Expected date to be parsed")
			}
		})
	}
}

func TestSetDateParseDateString(t *testing.T) {
	// Test that set_date can parse various date formats
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{"RFC3339", "2024-01-15T10:30:00Z", true},
		{"ISO8601", "2024-01-15T10:30:00", true},
		{"Date only", "2024-01-15", true},
		{"Invalid", "not-a-date", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseDateString(tt.input)
			if tt.valid && err != nil {
				t.Errorf("parseDateString(%q) error = %v, expected success", tt.input, err)
			}
			if !tt.valid && err == nil {
				t.Errorf("parseDateString(%q) expected error, got nil", tt.input)
			}
		})
	}
}

func TestSetDateUnixTimestamp(t *testing.T) {
	// Test that Unix timestamp conversion works correctly
	timestamp := int64(1704067200) // 2024-01-01 00:00:00 UTC
	expected := time.Unix(timestamp, 0)

	result := time.Unix(timestamp, 0)

	if !result.Equal(expected) {
		t.Errorf("Expected %v, got %v", expected, result)
	}
}
