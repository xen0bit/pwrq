package udf

import (
	"sort"
	"testing"
)

// TestNoBuiltinShadowing is the guard that keeps pwrq a gojq superset.
//
// gojq checks custom functions after builtins, so a registered UDF that shares a
// builtin's name never fires - it does not error, it just quietly does nothing,
// and the user gets jq's function when they asked for pwrq's. Alias definitions
// are worse: a `def` does take precedence, so an alias named `select` would
// change what every existing jq program means.
func TestNoBuiltinShadowing(t *testing.T) {
	builtins, err := jqBuiltins()
	if err != nil {
		t.Fatalf("discovering jq builtins: %v", err)
	}

	reg := DefaultRegistry()
	sigs, err := reg.Signatures()
	if err != nil {
		t.Fatalf("discovering registered functions: %v", err)
	}

	for sig := range sigs {
		if builtins[sig] {
			t.Errorf("UDF %s shadows a jq builtin and will never be called", sig)
		}
	}

	// Signatures() reports registered-minus-builtin, so a UDF that fully
	// collides with a builtin is invisible to it - the union is the same set.
	// The documented arity range is the declaration of intent, so check that
	// against the builtins directly to catch what discovery cannot see.
	for _, meta := range GetFunctionMetadata() {
		for arity := meta.MinArgs; arity <= meta.MaxArgs; arity++ {
			sig := Signature{Name: meta.Name, Arity: arity}
			if builtins[sig] {
				t.Errorf("%s is documented but jq provides that signature, so pwrq's version never runs", sig)
			}
		}
	}

	defs, err := reg.AliasFuncDefs(StandardAliases)
	if err != nil {
		t.Fatalf("building aliases: %v", err)
	}
	for _, def := range defs {
		sig := Signature{Name: def.Name, Arity: len(def.Args)}
		if builtins[sig] {
			t.Errorf("alias %s shadows a jq builtin, changing the meaning of existing jq programs", sig)
		}
	}
}

// TestAliasesResolve checks every alias names something that exists. Without
// this an alias is a compile error the user only discovers by typing it.
func TestAliasesResolve(t *testing.T) {
	reg := DefaultRegistry()
	if _, err := reg.AliasFuncDefs(StandardAliases); err != nil {
		t.Fatal(err)
	}

	sigs, err := reg.Signatures()
	if err != nil {
		t.Fatal(err)
	}
	names := make(map[string]bool)
	for sig := range sigs {
		names[sig.Name] = true
	}
	for _, alias := range StandardAliases {
		if names[alias.Name] {
			t.Errorf("alias %q collides with the registered function of the same name", alias.Name)
		}
	}
}

// TestUDFListMatchesRegistry keeps `pwrq --udf-list` honest.
//
// The documented list used to be maintained by hand beside the registrations:
// 20 registered functions were undiscoverable and 8 documented names did not
// exist. Comparing the declared list against what gojq actually registers means
// that can no longer happen quietly.
func TestUDFListMatchesRegistry(t *testing.T) {
	registered, err := DefaultRegistry().Names()
	if err != nil {
		t.Fatalf("discovering registered functions: %v", err)
	}

	documented := make(map[string]bool)
	for _, meta := range GetFunctionMetadata() {
		if documented[meta.Name] {
			t.Errorf("%s is documented twice", meta.Name)
		}
		documented[meta.Name] = true
	}

	actual := make(map[string]bool, len(registered))
	for _, name := range registered {
		actual[name] = true
	}

	var undocumented, phantom []string
	for _, name := range registered {
		if !documented[name] {
			undocumented = append(undocumented, name)
		}
	}
	for name := range documented {
		if !actual[name] {
			phantom = append(phantom, name)
		}
	}
	sort.Strings(undocumented)
	sort.Strings(phantom)

	if len(undocumented) > 0 {
		t.Errorf("registered but absent from --udf-list: %v", undocumented)
	}
	if len(phantom) > 0 {
		t.Errorf("listed by --udf-list but not registered: %v", phantom)
	}
}

// TestMetadataArityMatches catches a documented arity that no longer matches the
// function, which would send users to an error message instead of a result.
func TestMetadataArityMatches(t *testing.T) {
	sigs, err := DefaultRegistry().Signatures()
	if err != nil {
		t.Fatal(err)
	}
	for _, meta := range GetFunctionMetadata() {
		for arity := meta.MinArgs; arity <= meta.MaxArgs; arity++ {
			if !sigs[Signature{Name: meta.Name, Arity: arity}] {
				t.Errorf("%s is documented as accepting %d argument(s), but is not registered at that arity",
					meta.Name, arity)
			}
		}
	}
}
