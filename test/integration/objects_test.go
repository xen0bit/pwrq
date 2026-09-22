package integration

import (
	"strings"
	"testing"
)

// people is the fixture the object cmdlets run against. It is fed on stdin
// rather than built in the query on purpose: the CLI decodes input with
// UseNumber(), so Age arrives as a json.Number, and a cmdlet that only
// recognises float64 sees no number at all. Every numeric bug these tests
// caught - a sum of zero, 9 sorting after 100 - was invisible to unit tests
// that constructed their input as Go float64s.
const people = `[{"Name":"Alice","Age":30,"Dept":"Eng"},` +
	`{"Name":"Bob","Age":25,"Dept":"Sales"},` +
	`{"Name":"Carol","Age":35,"Dept":"Eng"}]`

// TestSelectObject covers Select-Object's projection and its -First/-Last/-Skip
// window. The window options are read from an options map whose numbers gojq
// represents as int, not float64.
func TestSelectObject(t *testing.T) {
	cases := []struct{ name, query, want string }{
		{"property list", `select_object(.; {property: ["Name"]})`,
			`[{"Name":"Alice"},{"Name":"Bob"},{"Name":"Carol"}]`},
		{"property from pipe", `select_object("Name") | map(.Name)`,
			`["Alice","Bob","Carol"]`},
		{"first", `select_object(.; {first: 2}) | map(.Name)`,
			`["Alice","Bob"]`},
		{"last", `select_object(.; {last: 1}) | .Name`,
			`"Carol"`},
		{"skip", `select_object(.; {skip: 1}) | map(.Name)`,
			`["Bob","Carol"]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.TrimSpace(mustRun(t, people, "-c", tc.query))
			if got != tc.want {
				t.Errorf("%s\n got %s\nwant %s", tc.query, got, tc.want)
			}
		})
	}
}

// TestSelectObjectFirstAndLastConflict checks the one combination PowerShell
// rejects rather than guessing at.
func TestSelectObjectFirstAndLastConflict(t *testing.T) {
	_, stderr, code := run(t, people, "-c", `select_object(.; {first: 1, last: 1})`)
	if code == 0 {
		t.Error("-First with -Last should fail rather than pick one")
	}
	if !strings.Contains(stderr, "First") {
		t.Errorf("the error should name the conflicting parameter, got %q", stderr)
	}
}

// TestSortObject covers Sort-Object. Numbers have to compare as numbers: the
// property values arrive as json.Number, and comparing them as text sorted 100
// before 9.
func TestSortObject(t *testing.T) {
	cases := []struct{ name, input, query, want string }{
		{"by property", people, `sort_object(.; {property: "Age"}) | map(.Name)`,
			`["Bob","Alice","Carol"]`},
		{"descending flag", people, `sort_object(.; {property: "Age", descending: true}) | map(.Name)`,
			`["Carol","Alice","Bob"]`},
		{"descending suffix", people, `sort_object(.; {property: "Age desc"}) | map(.Name)`,
			`["Carol","Alice","Bob"]`},
		{"numeric not lexical", `[{"n":10},{"n":9},{"n":100}]`,
			`sort_object(.; {property: "n"}) | map(.n)`, `[9,10,100]`},
		{"stable on ties", `[{"n":1,"k":"a"},{"n":1,"k":"b"}]`,
			`sort_object(.; {property: "n"}) | map(.k)`, `["a","b"]`},
		{"unique", `[{"n":1},{"n":1},{"n":2}]`,
			`sort_object(.; {property: "n", unique: true}) | map(.n)`, `[1,2]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.TrimSpace(mustRun(t, tc.input, "-c", tc.query))
			if got != tc.want {
				t.Errorf("%s\n got %s\nwant %s", tc.query, got, tc.want)
			}
		})
	}
}

// TestGroupObject checks the GroupInfo shape, which is what makes a grouping
// navigable with jq afterwards.
func TestGroupObject(t *testing.T) {
	got := strings.TrimSpace(mustRun(t, people, "-c",
		`group_object(.; {property: "Dept"}) | map({Name, Count})`))
	const want = `[{"Count":2,"Name":"Eng"},{"Count":1,"Name":"Sales"}]`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}

	// The members travel with the group, not just a count.
	got = strings.TrimSpace(mustRun(t, people, "-c",
		`group_object(.; {property: "Dept"})[0].Group | map(.Name)`))
	if got != `["Alice","Carol"]` {
		t.Errorf("Group members: got %s", got)
	}

	// And the group carries its PowerShell type.
	got = strings.TrimSpace(mustRun(t, people, "-c",
		`[group_object(.; {property: "Dept"})[].PwrqType] | unique`))
	if got != `["Pwrq.Group"]` {
		t.Errorf("PwrqType: got %s", got)
	}
}

// TestMeasureObject is the regression guard for the json.Number bug: every
// measurement of piped-in data reported zero, because the property values were
// json.Number and the conversion only knew float64.
func TestMeasureObject(t *testing.T) {
	got := strings.TrimSpace(mustRun(t, people, "-c",
		`measure_object(.; {property: "Age", sum: true, average: true, minimum: true, maximum: true})
		 | {Count, Sum, Average, Minimum, Maximum}`))
	const want = `{"Average":30,"Count":3,"Maximum":35,"Minimum":25,"Sum":90}`
	if got != want {
		t.Errorf("got %s\nwant %s", got, want)
	}

	// With no property named, Measure-Object counts objects.
	got = strings.TrimSpace(mustRun(t, people, "-c", `measure_object(.) | .Count`))
	if got != "3" {
		t.Errorf("count: got %s, want 3", got)
	}
}

// TestObjectCmdletsCompose is the point of them emitting plain JSON: their
// output feeds the next cmdlet and jq's own filters interchangeably.
func TestObjectCmdletsCompose(t *testing.T) {
	got := strings.TrimSpace(mustRun(t, people, "-c",
		`where_object(.; {script: ".Dept == \"Eng\""})
		 | sort_object(.; {property: "Age", descending: true})
		 | select_object(.; {property: ["Name"]})
		 | map(.Name)`))
	if got != `["Carol","Alice"]` {
		t.Errorf("got %s, want [\"Carol\",\"Alice\"]", got)
	}
}

// TestObjectCmdletsReadThePipeline covers the piped calling convention, which
// the cmdlets documented but did not implement: the pipeline value is the
// objects and the argument is the options.
func TestObjectCmdletsReadThePipeline(t *testing.T) {
	cases := []struct{ name, query, want string }{
		{"sort pipe", `sort_object({property: "Age"}) | map(.Name)`,
			`["Bob","Alice","Carol"]`},
		{"group pipe", `group_object({property: "Dept"}) | map({Name, Count})`,
			`[{"Count":2,"Name":"Eng"},{"Count":1,"Name":"Sales"}]`},
		{"where pipe", `where_object({script: ".Dept == \"Eng\""}) | map(.Name)`,
			`["Alice","Carol"]`},
		{"measure pipe", `measure_object({property: "Age", sum: true}) | .Sum`,
			`90`},
		{"select pipe", `select_object("Name") | map(.Name)`,
			`["Alice","Bob","Carol"]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.TrimSpace(mustRun(t, people, "-c", tc.query))
			if got != tc.want {
				t.Errorf("%s\n got %s\nwant %s", tc.query, got, tc.want)
			}
		})
	}
}

// TestObjectCmdletsOnCmdletOutput is the regression for BindValue collapsing a
// cmdlet object: a FileInfo carries PwrqValue (its path as a string), and the
// object cmdlets used to unwrap it to that scalar, so selecting Name returned
// "base64.go" ... as the whole result, and where_object could not see Name at
// all.
func TestObjectCmdletsOnCmdletOutput(t *testing.T) {
	dir := t.TempDir()
	mustRunDir(t, dir, "", "-n", "-c",
		`set_content("a.txt"; "x") | set_content("bb.txt"; "longer")`)

	cases := []struct{ name, query, want string }{
		{"select on a FileInfo",
			`first(get_childitem("."; {Filter: "a.txt"})) | select_object("Name") | .Name`,
			`"a.txt"`},
		{"where on a FileInfo",
			`first(get_childitem("."; {Filter: "a.txt"}))
			 | where_object(.; {property: "Name", operator: "eq", value: "a.txt"}) | length`,
			`1`},
		{"measure on a FileInfo",
			`first(get_childitem("."; {Filter: "a.txt"}))
			 | measure_object(.; {property: "Length", sum: true}) | .Sum`,
			`1`},
		{"sort an array of FileInfo",
			`[get_childitem("."; {Filter: "*.txt"})] | sort_object(.; {property: "Length"}) | .[0].Name`,
			`"a.txt"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.TrimSpace(mustRunDir(t, dir, "", "-n", "-c", tc.query))
			if got != tc.want {
				t.Errorf("%s\n got %s\nwant %s", tc.query, got, tc.want)
			}
		})
	}
}
