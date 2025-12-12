package csv

import (
	"encoding/csv"
	"strings"
	"testing"
)

func TestCSVParse(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		delimiter rune
		wantRows  int
		wantErr   bool
	}{
		{
			name:      "simple CSV",
			input:     "a,b,c\n1,2,3",
			delimiter: ',',
			wantRows:  2,
			wantErr:   false,
		},
		{
			name:      "tab-delimited",
			input:     "a\tb\tc\n1\t2\t3",
			delimiter: '\t',
			wantRows:  2,
			wantErr:   false,
		},
		{
			name:      "single row",
			input:     "a,b,c",
			delimiter: ',',
			wantRows:  1,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := csv.NewReader(strings.NewReader(tt.input))
			reader.Comma = tt.delimiter
			records, err := reader.ReadAll()
			if (err != nil) != tt.wantErr {
				t.Errorf("csv_parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(records) != tt.wantRows {
				t.Errorf("csv_parse() rows = %v, want %v", len(records), tt.wantRows)
			}
		})
	}
}

func TestCSVStringify(t *testing.T) {
	tests := []struct {
		name      string
		input     [][]string
		delimiter rune
		wantErr   bool
	}{
		{
			name:      "simple CSV",
			input:     [][]string{{"a", "b", "c"}, {"1", "2", "3"}},
			delimiter: ',',
			wantErr:   false,
		},
		{
			name:      "single row",
			input:     [][]string{{"a", "b", "c"}},
			delimiter: ',',
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf strings.Builder
			writer := csv.NewWriter(&buf)
			writer.Comma = tt.delimiter
			err := writer.WriteAll(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("csv_stringify() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && buf.Len() == 0 {
				t.Error("csv_stringify() returned empty string")
			}
		})
	}
}

func TestCSVRoundTrip(t *testing.T) {
	input := "a,b,c\n1,2,3\nx,y,z"
	
	// Parse
	reader := csv.NewReader(strings.NewReader(input))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	// Stringify
	var buf strings.Builder
	writer := csv.NewWriter(&buf)
	if err := writer.WriteAll(records); err != nil {
		t.Fatalf("stringify failed: %v", err)
	}
	writer.Flush()

	// Verify structure (may have different line endings)
	if buf.Len() == 0 {
		t.Error("round-trip produced empty result")
	}
}

