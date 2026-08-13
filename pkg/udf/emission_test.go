package udf

import (
	"strings"
	"testing"

	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// TestEveryDocumentedCommandHasKnownEmission pins the OUTPUT section get_help
// prints for every cmdlet.
//
// Whether a cmdlet emits one value or a stream decides whether a caller must
// collect with [...] or must not, and getting it wrong is the most common way
// a query fails. get_help answers that from what the registration wrappers
// observed, and a name they never saw defaults to "a single value" - which is
// a confident wrong answer for a streaming cmdlet.
//
// So a documented name the wrappers do not know about is a bug in the docs,
// not a gap to paper over. This catches it at build time rather than leaving a
// caller to discover it from an "expected an object but got: array".
func TestEveryDocumentedCommandHasKnownEmission(t *testing.T) {
	DefaultRegistry()

	var unknown []string
	for _, meta := range GetFunctionMetadata() {
		if _, known := common.IsStreaming(meta.Name); !known {
			unknown = append(unknown, meta.Name)
		}
	}

	if len(unknown) > 0 {
		t.Errorf("%d documented command(s) were never registered through common.WithFunction "+
			"or common.WithIterFunction, so get_help would guess at their output shape:\n    %s",
			len(unknown), strings.Join(unknown, "\n    "))
	}
}

// TestStreamingCommandsReportStreaming checks the catalog get_help reads from
// actually carries the flag through, rather than defaulting every command to
// the same answer.
func TestStreamingCommandsReportStreaming(t *testing.T) {
	DefaultRegistry()

	// The cmdlets that enumerate something stream, one value per result; the
	// ones that compute a single answer do not. select_string is pinned here
	// because it used to be the exception - it returned one array while every
	// other enumerator streamed, which is the inconsistency callers tripped over.
	want := map[string]bool{
		"get_childitem": true,
		"find":          true,
		"get_process":   true,
		"select_string": true,
		"cat":           false,
		"sha256":        false,
	}

	got := make(map[string]bool)
	for _, c := range buildCatalog() {
		if _, ok := want[c.Name]; ok {
			got[c.Name] = c.Streaming
		}
	}

	for name, wantStreaming := range want {
		streaming, ok := got[name]
		if !ok {
			t.Errorf("%s is missing from the catalog", name)
			continue
		}
		if streaming != wantStreaming {
			t.Errorf("%s: Streaming = %v, want %v", name, streaming, wantStreaming)
		}
	}
}

// TestEmitsDescriptionTellsCallerWhatToDo checks the help text names the
// bracket decision explicitly, since that is the whole point of printing it.
func TestEmitsDescriptionTellsCallerWhatToDo(t *testing.T) {
	for _, c := range buildCatalog() {
		desc := c.EmitsDescription()
		if !strings.Contains(desc, "[...]") {
			t.Fatalf("%s: OUTPUT text does not mention collecting with [...]: %q", c.Name, desc)
		}
	}
}
