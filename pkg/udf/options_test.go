package udf

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/xen0bit/pwrq/pkg/udf/discovery"
	"github.com/xen0bit/pwrq/pkg/udf/powershell/filesystem"
)

// TestDocumentedOptionsNameRealCmdlets stops the table describing a cmdlet
// that does not exist, which is how a hand-written table starts going stale.
func TestDocumentedOptionsNameRealCmdlets(t *testing.T) {
	DefaultRegistry()

	known := make(map[string]bool)
	for _, meta := range GetFunctionMetadata() {
		known[meta.Name] = true
	}
	for name := range documentedOptionKeys {
		if !known[name] {
			t.Errorf("options are documented for %q, which is not a cmdlet", name)
		}
	}
}

// TestEveryOptionsCmdletIsDocumented is the coverage check. A cmdlet whose
// description promises `[options]` and then lists none is the exact state this
// change set out to fix, so a new one must not be able to arrive quietly.
func TestEveryOptionsCmdletIsDocumented(t *testing.T) {
	DefaultRegistry()

	var undocumented []string
	for _, c := range discovery.Catalog() {
		if !strings.Contains(strings.ToLower(c.Description), "[options]") {
			continue
		}
		if len(c.Options) == 0 {
			undocumented = append(undocumented, c.Name)
		}
	}
	if len(undocumented) > 0 {
		sort.Strings(undocumented)
		t.Errorf("%d cmdlet(s) are documented as taking [options] but list none, "+
			"so a caller can only find the keys by guessing:\n    %s",
			len(undocumented), strings.Join(undocumented, "\n    "))
	}
}

// TestDocumentedOptionsAreWellFormed checks each entry says enough to be acted
// on: a key, the type its value must have, and what it does.
func TestDocumentedOptionsAreWellFormed(t *testing.T) {
	types := map[string]bool{
		"string": true, "number": true, "boolean": true,
		"object": true, "array": true, "any": true,
	}
	for name, options := range documentedOptionKeys {
		seen := make(map[string]bool)
		for _, o := range options {
			switch {
			case o.Name == "":
				t.Errorf("%s: an option has no name", name)
			case o.Description == "":
				t.Errorf("%s: option %s has no description", name, o.Name)
			case !types[o.Type]:
				t.Errorf("%s: option %s has type %q, which is not a JSON type", name, o.Name, o.Type)
			case seen[o.Name]:
				t.Errorf("%s: option %s is listed twice", name, o.Name)
			}
			seen[o.Name] = true
		}
	}
}

// TestGetChildItemOptionsMatchItsParamTags is the strongest guard available,
// and it is available for exactly one cmdlet.
//
// get_childitem binds its options by reflection over `param:` struct tags, so
// the tags *are* the parsing code and the documentation can be checked against
// them exactly. Every other cmdlet in the table hand-rolls a switch, where no
// such correspondence exists to check. That asymmetry is why the table carries
// the caveat it does.
func TestGetChildItemOptionsMatchItsParamTags(t *testing.T) {
	tagged := make(map[string]bool)
	fields := reflect.TypeOf(filesystem.GetChildItemOptions{})
	for i := range fields.NumField() {
		if tag := fields.Field(i).Tag.Get("param"); tag != "" {
			tagged[strings.Split(tag, ",")[0]] = true
		}
	}

	documented := make(map[string]bool)
	for _, o := range documentedOptions("get_childitem") {
		documented[o.Name] = true
	}

	for key := range tagged {
		if !documented[key] {
			t.Errorf("get_childitem reads option %q, which the catalogue does not mention", key)
		}
	}
	for key := range documented {
		if !tagged[key] {
			t.Errorf("the catalogue documents get_childitem option %q, which it does not read", key)
		}
	}
}

// TestDocumentedOptionsAreAccepted drives each documented key through the
// cmdlet that claims to read it, for the cmdlets that can be exercised without
// a network, a service manager or a running process.
//
// It is the check that catches a renamed key. Most of these parsers reject an
// unknown option outright, so passing one that has been renamed fails here;
// the ones that ignore unknown keys are covered by asserting the option's
// effect instead.
func TestDocumentedOptionsAreAccepted(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("alpha\nbeta\n"), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	// Each case names an option and a query that must succeed while passing
	// it. A parser that no longer knows the key errors, and the run fails.
	cases := []struct {
		cmdlet string
		option string
		query  string
	}{
		{"get_childitem", "Recurse", `[get_childitem("."; {Recurse: true})] | length > 0`},
		{"get_childitem", "Filter", `[get_childitem("."; {Filter: "*.txt"})] | length == 1`},
		{"get_childitem", "Directory", `[get_childitem("."; {Directory: true})] | length == 0`},
		{"select_string", "Include", `[select_string("."; "alpha"; {Include: "*.txt"})] | length == 1`},
		{"select_string", "Include", `[select_string("."; "alpha"; {Include: "*.md"})] | length == 0`},
		{"select_string", "Context", `[select_string("."; "beta"; {Context: 1})][0].Before == ["alpha"]`},
		{"select_string", "CaseSensitive", `[select_string("."; "ALPHA"; {CaseSensitive: true})] | length == 0`},
		{"select_string", "List", `[select_string("."; "alpha"; {List: true})] | length == 1`},
		{"out_file", "Append", `"x" | out_file("out.txt"; {Append: true}) | . == "x"`},
		{"out_file", "Encoding", `"x" | out_file("enc.txt"; {Encoding: "utf8"}) | . == "x"`},
		{"add_content", "Force", `"x" | add_content("add.txt"; {Force: true}) | . != null`},
		{"compare_object", "IncludeEqual", `compare_object([1]; [1]; {IncludeEqual: true}) | length == 1`},
		{"compare_object", "ExcludeDifferent", `compare_object([1]; [2]; {ExcludeDifferent: true}) | length == 0`},
		{"compare_object", "Property", `compare_object([{id: 1, v: "a"}]; [{id: 1, v: "b"}]; {Property: "id", IncludeEqual: true}) | length == 1`},
		{"where_object", "property", `[where_object([{a: 1}, {a: 2}]; {property: "a", operator: "eq", value: 1})] | length == 1`},
		{"where_object", "script", `[where_object([1, 5, 20]; {script: ". > 10"})] | length == 1`},
		{"measure_object", "sum", `measure_object([{n: 1}, {n: 2}]; {property: "n", sum: true}) | .Sum == 3`},
		{"measure_object", "average", `measure_object([{n: 1}, {n: 3}]; {property: "n", average: true}) | .Average == 2`},
		{"get_date", "Format", `get_date({Format: "2006"}) | type == "string"`},
	}

	reg := DefaultRegistry()
	options := reg.Options()

	for _, tc := range cases {
		t.Run(tc.cmdlet+"/"+tc.option, func(t *testing.T) {
			if !documents(tc.cmdlet, tc.option) {
				t.Fatalf("the table does not document %s for %s, so this case checks nothing",
					tc.option, tc.cmdlet)
			}
			got, err := runProbe(options, tc.query)
			if err != nil {
				t.Fatalf("%s rejected its documented option %s: %v", tc.cmdlet, tc.option, err)
			}
			if got != true {
				t.Errorf("%s: %s did not take effect: %s returned %v", tc.cmdlet, tc.option, tc.query, got)
			}
		})
	}
}

func documents(cmdlet, option string) bool {
	for _, o := range documentedOptions(cmdlet) {
		if o.Name == option {
			return true
		}
	}
	return false
}
