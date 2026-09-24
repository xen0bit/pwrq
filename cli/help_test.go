package cli

import (
	"strings"
	"testing"
)

// TestHelpHeaderPrecedesTheHelpFlag guards a regression: the "Help Option:"
// header was inserted before the last struct field, so appending --mcp-http
// after Help moved the header onto the wrong flag.
func TestHelpHeaderPrecedesTheHelpFlag(t *testing.T) {
	got := formatFlags(&flagopts{})
	lines := strings.Split(got, "\n")
	for i, line := range lines {
		if !strings.Contains(line, "--help") {
			continue
		}
		if i == 0 || lines[i-1] != "Help Option:" {
			t.Fatalf("--help is not under the Help Option header:\n%s", got)
		}
		return
	}
	t.Fatalf("--help not found in the options listing:\n%s", got)
}
