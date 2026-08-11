package collection

import (
	"fmt"
	"testing"
)

func TestSetOperations(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{`[1,2,3] | intersection([2,3,4])`, "[2 3]"},
		{`[1,2] | union([2,3])`, "[1 2 3]"},
		{`[1,2,3] | difference([2])`, "[1 3]"},
		{`[1,2] | symmetric_difference([2,3])`, "[1 3]"},
		{`[1,1,2] | all_equal`, "false"},
		{`[1,1,1] | all_equal`, "true"},
		{`[1,2,1] | contains_duplicates`, "true"},
		{`[1,2,3] | contains_duplicates`, "false"},
	}
	for _, tt := range tests {
		got := fmt.Sprint(run(t, tt.query))
		if got != tt.want {
			t.Errorf("%s = %s, want %s", tt.query, got, tt.want)
		}
	}
}

func TestSlicingAndCombining(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{`[[1,2],[3,4]] | column(1)`, "[2 4]"},
		{`[[1,2],[3]] | column(5)`, "[<nil> <nil>]"},
		{`[{"name":"ada"},{"name":"bob"}] | lookup("name"; "bob").name`, "bob"},
		{`[{"name":"ada"}] | lookup("name"; "missing")`, "<nil>"},
		{`["file2","file10","file1"] | natural_sort`, "[file1 file2 file10]"},
		{`cartesian([1,2]; ["a","b"])`, "[[1 a] [1 b] [2 a] [2 b]]"},
	}
	for _, tt := range tests {
		got := fmt.Sprint(run(t, tt.query))
		if got != tt.want {
			t.Errorf("%s = %s, want %s", tt.query, got, tt.want)
		}
	}
}
