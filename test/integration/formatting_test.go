package integration

import (
	"strings"
	"testing"
)

// TestFormattersReturnText locks in what Phase 1 changed: a formatter is a
// transform that returns a string, not an object that happens to print. If it
// returned an object, -r would print JSON and the output would be unusable as
// text.
func TestFormattersReturnText(t *testing.T) {
	for _, fn := range []string{"format_table", "format_list"} {
		t.Run(fn, func(t *testing.T) {
			got := strings.TrimSpace(mustRun(t, people, "-c", fn+`(.) | type`))
			if got != `"string"` {
				t.Errorf("%s returns %s, want \"string\"", fn, got)
			}
		})
	}
}

// TestFormatTableAligns is the regression guard for a table whose columns did
// not line up: without -AutoSize the widths were narrowed back to the header
// length, so any value longer than its header overflowed and pushed every
// column after it out of position.
func TestFormatTableAligns(t *testing.T) {
	out := mustRun(t, people, "-r", `format_table(.; {property: ["Name", "Age"]})`)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 5 {
		t.Fatalf("expected a header, a rule and three rows, got %d lines:\n%s", len(lines), out)
	}

	// The rule under a header has to be as wide as the widest cell beneath it,
	// which is "Alice" - five characters, not the four of "Name".
	header, rule := lines[0], lines[1]
	if !strings.Contains(rule, "-----") {
		t.Errorf("the Name column rule should span its widest value, got %q", rule)
	}

	// Every column starts at the same offset on every line.
	want := strings.Index(header, "Age")
	if want < 0 {
		t.Fatalf("header %q has no Age column", header)
	}
	for _, line := range lines[1:] {
		if len(line) <= want || line[want] == ' ' {
			t.Errorf("the Age column does not start at offset %d on %q", want, line)
		}
	}
}

// TestFormatTableSelectsProperties checks that -Property narrows the table
// rather than being ignored.
func TestFormatTableSelectsProperties(t *testing.T) {
	out := mustRun(t, people, "-r", `format_table(.; {property: ["Name"]})`)
	if strings.Contains(out, "Age") {
		t.Errorf("format_table with property [Name] still shows Age:\n%s", out)
	}
	for _, name := range []string{"Alice", "Bob", "Carol"} {
		if !strings.Contains(out, name) {
			t.Errorf("format_table dropped %s:\n%s", name, out)
		}
	}
}

// TestFormatListPairsPropertiesWithValues checks Format-List's shape: one
// "Name : Value" line per property, blank line between records.
func TestFormatListPairsPropertiesWithValues(t *testing.T) {
	out := mustRun(t, people, "-r", `format_list(.; {property: ["Name", "Age"]})`)
	for _, want := range []string{"Name", "Alice", "Age", "30", "Bob", "25"} {
		if !strings.Contains(out, want) {
			t.Errorf("format_list dropped %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "\n\n") {
		t.Errorf("format_list should separate records with a blank line:\n%s", out)
	}
}

// TestFormattersOnCmdletOutput is the case that matters in practice: a
// formatter has to accept what another cmdlet emits, not only hand-written
// JSON.
func TestFormattersOnCmdletOutput(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "hello")
	writeFile(t, dir, "b.txt", "world")

	out := mustRun(t, "null", "-r",
		`[get_childitem("`+dir+`")] | format_table(.; {property: ["Name", "Length"]})`)
	for _, want := range []string{"Name", "Length", "a.txt", "b.txt", "5"} {
		if !strings.Contains(out, want) {
			t.Errorf("format_table of get_childitem output dropped %q:\n%s", want, out)
		}
	}
}
