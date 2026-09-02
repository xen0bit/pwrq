package udf

import (
	"testing"

	"github.com/xen0bit/pwrq/pkg/udf/censys"
)

// The catalogue and the registry agree about the Censys write cmdlets.
//
// Two things read the metadata table and neither of them runs a query first:
// get_command answers from it, and invoke_agent is handed it as the vocabulary
// to write pipelines against. A cmdlet listed there but left unregistered is
// worse than one that was never mentioned - the agent has been told it exists,
// so it will use it, and the failure comes back as "function not defined" from
// a name the same program advertised a moment earlier.
func TestTheCatalogueDoesNotAdvertiseTheCensysWrites(t *testing.T) {
	t.Setenv(censys.EnvWrite, "")

	listed := map[string]bool{}
	for _, meta := range GetFunctionMetadata() {
		listed[meta.Name] = true
	}
	for _, name := range censys.WriteCmdlets {
		if listed[name] {
			t.Errorf("the catalogue lists %q, which is not registered unless %s is set",
				name, censys.EnvWrite)
		}
	}
}

// And that the entries are withheld rather than lost: the documentation for a
// cmdlet somebody has deliberately turned on has to come back with it, or
// get_help answers "no such cmdlet" about something that is running.
func TestTheCatalogueCarriesTheWritesWhenTheyAreOn(t *testing.T) {
	t.Setenv(censys.EnvWrite, "1")

	listed := map[string]bool{}
	for _, meta := range GetFunctionMetadata() {
		listed[meta.Name] = true
	}
	for _, name := range censys.WriteCmdlets {
		if !listed[name] {
			t.Errorf("%s is set, %q is registered, and the catalogue does not document it",
				censys.EnvWrite, name)
		}
	}
}

// Nothing but those nine moves. The filter works from a name list, so the way
// it would go wrong is a name that matches more than it meant to.
func TestTheGateWithholdsOnlyTheWrites(t *testing.T) {
	t.Setenv(censys.EnvWrite, "1")
	whole := map[string]bool{}
	for _, meta := range GetFunctionMetadata() {
		whole[meta.Name] = true
	}

	t.Setenv(censys.EnvWrite, "")
	for _, meta := range GetFunctionMetadata() {
		delete(whole, meta.Name)
	}

	if len(whole) != len(censys.WriteCmdlets) {
		t.Errorf("turning the gate off withheld %d cmdlets, want the %d in WriteCmdlets: %v",
			len(whole), len(censys.WriteCmdlets), whole)
	}
}
