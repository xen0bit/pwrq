package udf

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/xen0bit/pwrq/pkg/core/queryrun"
	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
	"github.com/xen0bit/pwrq/pkg/core/shape"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// These tests are what make the shape catalogue worth trusting.
//
// A declared property list is a second copy of a fact the cmdlet already knows,
// and every second copy drifts. The defence is not to review the declarations
// but to run the cmdlets and compare: shape.Build records the difference
// between what was declared and what was emitted, so exercising a cmdlet is
// enough to catch a declaration that has fallen behind. That is the same bargain
// TestEveryDocumentedCommandHasKnownEmission strikes for the streaming flag.

// unsafeToRun are the cmdlets whose documented example must not run in a test:
// it would write to the filesystem outside the temp directory, start or stop a
// process or service, reach the network, or spend money at an API.
//
// Skipping them costs coverage rather than correctness. A skipped cmdlet is one
// whose shape is not checked here, never one that is wrongly reported as fine.
var unsafeToRun = regexp.MustCompile(`^(` + strings.Join([]string{
	// Writes, moves and deletes.
	"out_file", "set_content", "add_content", "new_item", "move_item",
	"copy_item", "rm", "rms", "mkdir", "expand_archive", "compress_archive",
	"out_sqlite", "invoke_sqlite_command", "set_date",
	// Process and service control.
	"sh", "start_process", "stop_process", "start_service", "stop_service",
	// Network and paid APIs.
	"http", "http_serve", "invoke_web_request", "censys_.*", "get_censys_.*",
	"set_censys_.*", "add_censys_.*", "remove_censys_.*",
	"invoke_llm.*", "invoke_agent.*", "get_llm_.*",
	// Mutates the session other tests share.
	"set_variable", "set_location", "set_path", "set_alias",
}, "|") + `)$`)

// runExamples evaluates the first documented example of every cmdlet the filter
// admits, in a temporary working directory, and reports what each produced.
//
// Failures are ignored on purpose. Many examples name a file that does not
// exist here, or take their input from a pipeline this harness does not supply,
// and an example that could not run tells us nothing about the shape its cmdlet
// emits. What matters is that every example which *did* run is checked.
func runExamples(t *testing.T, admit func(FunctionMetadata) bool) map[string]any {
	t.Helper()

	reg := DefaultRegistry()
	common.SetGlobalSessionState(sessionstate.NewSessionState())
	t.Cleanup(func() { common.SetGlobalSessionState(nil) })

	runner := &queryrun.Runner{Options: reg.Options()}
	t.Chdir(t.TempDir())

	produced := make(map[string]any)
	for _, meta := range GetFunctionMetadata() {
		if len(meta.Examples) == 0 || unsafeToRun.MatchString(meta.Name) || !admit(meta) {
			continue
		}

		// A streaming cmdlet is wrapped in first(...) so the harness sees one
		// emitted value rather than the stream, which is what the shape
		// describes.
		program := meta.Examples[0]
		if streaming, _ := common.IsStreaming(meta.Name); streaming {
			program = "first(" + program + ")"
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		res := runner.Run(ctx, &queryrun.Request{
			Query:      program,
			NullInput:  true,
			Compact:    true,
			MaxResults: 1,
		})
		cancel()

		// A value and an error are not exclusive here: MaxResults stopping a
		// streaming cmdlet after its first value reports the limit as the
		// reason the run ended, which is exactly the outcome being asked for.
		// So this looks at what was produced, not at whether Run was happy.
		if len(res.Values) == 0 {
			continue
		}
		var value any
		if err := json.Unmarshal([]byte(res.Values[0]), &value); err != nil {
			continue
		}
		produced[meta.Name] = value
	}
	return produced
}

// TestDeclaredShapesMatchWhatCmdletsEmit runs the cmdlets and asserts that no
// declaration disagreed with the object it built.
//
// shape.Build records rather than returns a mismatch, so that a stale
// declaration degrades the catalogue instead of breaking a user's query. This
// test is the other half of that bargain: the recording is only worth doing if
// something checks it.
func TestDeclaredShapesMatchWhatCmdletsEmit(t *testing.T) {
	shape.ResetDiscrepancies()

	produced := runExamples(t, func(meta FunctionMetadata) bool {
		return common.ShapeOf(meta.Name).Specified()
	})
	if len(produced) == 0 {
		t.Fatal("no cmdlet with a declared shape could be run, so this test proves nothing")
	}

	if found := shape.Discrepancies(); len(found) > 0 {
		lines := make([]string, len(found))
		for i, d := range found {
			lines[i] = d.String()
		}
		t.Errorf("%d shape declaration(s) disagree with what the cmdlet emitted:\n    %s",
			len(found), strings.Join(lines, "\n    "))
	}
}

// TestObjectProducersDeclareAShape finds the cmdlets that emit an object and
// have not said what is in it.
//
// This is the test that keeps the catalogue from quietly going stale as cmdlets
// are added. It is also how the gap was found in the first place: before this
// change, 34 of the 43 object producers carried no type name and no property
// list, get_process and get_service among them, and nothing anywhere said so.
func TestObjectProducersDeclareAShape(t *testing.T) {
	produced := runExamples(t, func(FunctionMetadata) bool { return true })

	var undeclared []string
	for name, value := range produced {
		if _, isObject := value.(map[string]any); !isObject {
			continue
		}
		if common.ShapeOf(name).Specified() {
			continue
		}
		undeclared = append(undeclared, name)
	}
	sort.Strings(undeclared)

	if len(undeclared) > 0 {
		t.Errorf("%d cmdlet(s) emit an object but declare no shape, so get_help and the "+
			"MCP catalogue cannot say what keys a caller gets:\n    %s",
			len(undeclared), strings.Join(undeclared, "\n    "))
	}
}
