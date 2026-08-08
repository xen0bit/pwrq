package pipeline

import (
	"testing"
	"time"

	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
)

// TestPipelineContext tests pipeline context functionality.
func TestPipelineContext(t *testing.T) {
	t.Run("creates context with session state", func(t *testing.T) {
		ss := sessionstate.NewSessionState()
		ctx := NewPipelineContext(ss, "test-pipeline")

		if ctx.SessionState != ss {
			t.Errorf("SessionState not set correctly")
		}
		if ctx.PipelineID != "test-pipeline" {
			t.Errorf("PipelineID not set correctly: %s", ctx.PipelineID)
		}
	})

	t.Run("cancellation works", func(t *testing.T) {
		ss := sessionstate.NewSessionState()
		ctx := NewPipelineContext(ss, "test")

		if ctx.IsCancelled() {
			t.Fatal("context should not be cancelled initially")
		}

		ctx.Cancel()

		// Give it a moment to propagate
		time.Sleep(10 * time.Millisecond)

		if !ctx.IsCancelled() {
			t.Error("context should be cancelled after Cancel()")
		}
	})

	t.Run("error reporting is idempotent", func(t *testing.T) {
		ss := sessionstate.NewSessionState()
		ctx := NewPipelineContext(ss, "test")

		testErr := &testError{"test error"}
		ctx.ReportError(testErr)

		// First read
		err1 := ctx.GetError()
		if err1 == nil || err1.Error() != "test error" {
			t.Errorf("first GetError failed: %v", err1)
		}

		// Second read should return same error (idempotent)
		err2 := ctx.GetError()
		if err2 == nil || err2.Error() != "test error" {
			t.Errorf("second GetError failed: %v", err2)
		}
	})

	t.Run("with command creates child context", func(t *testing.T) {
		ss := sessionstate.NewSessionState()
		ctx := NewPipelineContext(ss, "test")

		child := ctx.WithCommand("get_childitem", 0, true)

		if child.CommandName != "get_childitem" {
			t.Errorf("CommandName not set: %s", child.CommandName)
		}
		if child.PipelinePosition != 0 {
			t.Errorf("PipelinePosition not set: %d", child.PipelinePosition)
		}
		if !child.IsLastInPipeline {
			t.Error("IsLastInPipeline not set")
		}
		if child.SessionState != ss {
			t.Error("SessionState not inherited")
		}
	})

	t.Run("get variable from session state", func(t *testing.T) {
		ss := sessionstate.NewSessionState()
		ss.SetVariable("testVar", "testValue", 0)

		ctx := NewPipelineContext(ss, "test")

		val, err := ctx.GetVariable("testVar")
		if err != nil {
			t.Fatalf("GetVariable failed: %v", err)
		}
		if val != "testValue" {
			t.Errorf("expected 'testValue', got %v", val)
		}
	})

	t.Run("set variable in session state", func(t *testing.T) {
		ss := sessionstate.NewSessionState()
		ctx := NewPipelineContext(ss, "test")

		err := ctx.SetVariable("newVar", "newValue")
		if err != nil {
			t.Fatalf("SetVariable failed: %v", err)
		}

		val, _ := ss.GetVariable("newVar")
		if val != "newValue" {
			t.Errorf("expected 'newValue', got %v", val)
		}
	})

	t.Run("alias resolution", func(t *testing.T) {
		ss := sessionstate.NewSessionState()
		ss.SetAlias("gci", "get_childitem")

		ctx := NewPipelineContext(ss, "test")

		resolved := ctx.ResolveCommand("gci")
		if resolved != "get_childitem" {
			t.Errorf("expected 'get_childitem', got %s", resolved)
		}

		// Non-alias should return as-is
		resolved = ctx.ResolveCommand("get_childitem")
		if resolved != "get_childitem" {
			t.Errorf("expected 'get_childitem', got %s", resolved)
		}
	})

	t.Run("handles nil session state", func(t *testing.T) {
		ctx := NewPipelineContext(nil, "test")

		// These should not panic
		_, err := ctx.GetVariable("test")
		if err != nil {
			t.Errorf("GetVariable with nil session should return nil, not error: %v", err)
		}

		err = ctx.SetVariable("test", "value")
		if err != nil {
			t.Errorf("SetVariable with nil session should not error: %v", err)
		}

		resolved := ctx.ResolveCommand("test")
		if resolved != "test" {
			t.Errorf("ResolveCommand with nil session should return input: %s", resolved)
		}
	})
}

// TestPipeline tests pipeline execution.
func TestPipeline(t *testing.T) {
	t.Run("creates pipeline with stages", func(t *testing.T) {
		ss := sessionstate.NewSessionState()
		pipeline := NewPipeline(ss, "test")

		if pipeline.Context == nil {
			t.Fatal("Context not created")
		}
		if len(pipeline.Stages) != 0 {
			t.Fatalf("expected 0 stages, got %d", len(pipeline.Stages))
		}
	})

	t.Run("adds stages correctly", func(t *testing.T) {
		ss := sessionstate.NewSessionState()
		pipeline := NewPipeline(ss, "test")

		cmdlet := &testCmdlet{}
		stage := pipeline.AddStage(cmdlet, "test-cmd")

		if stage == nil {
			t.Fatal("AddStage returned nil")
		}
		if stage.Name != "test-cmd" {
			t.Errorf("expected name 'test-cmd', got %s", stage.Name)
		}
		if stage.Position != 0 {
			t.Errorf("expected position 0, got %d", stage.Position)
		}
		if len(pipeline.Stages) != 1 {
			t.Fatalf("expected 1 stage, got %d", len(pipeline.Stages))
		}
	})

	t.Run("connects stage outputs", func(t *testing.T) {
		ss := sessionstate.NewSessionState()
		pipeline := NewPipeline(ss, "test")

		pipeline.AddStage(&testCmdlet{}, "cmd1")
		pipeline.AddStage(&testCmdlet{}, "cmd2")

		if len(pipeline.Stages) != 2 {
			t.Fatalf("expected 2 stages, got %d", len(pipeline.Stages))
		}

		// Stage 0 output should connect to stage 1 input
		// (This is tested implicitly through execution)
	})
}

// TestInitCmdletBase tests the CmdletWithBase interface.
func TestInitCmdletBase(t *testing.T) {
	t.Run("initializes cmdlet with base", func(t *testing.T) {
		ss := sessionstate.NewSessionState()
		ctx := NewPipelineContext(ss, "test")

		pipeline := NewPipeline(ss, "test")
		stage := pipeline.AddStage(&testCmdletWithBase{}, "test")

		// Manually call initCmdletBase to test
		initCmdletBase(stage.Cmdlet, ctx, stage, pipeline)

		// Check that the base was set
		cmdlet := stage.Cmdlet.(*testCmdletWithBase)
		if cmdlet.Base == nil {
			t.Fatal("CmdletBase not set")
		}
		if cmdlet.Base.SessionState != ss {
			t.Errorf("SessionState not set in base")
		}
		if cmdlet.Base.OutputWriter == nil {
			t.Error("OutputWriter not set in base")
		}
		if cmdlet.Base.ErrorWriter == nil {
			t.Error("ErrorWriter not set in base")
		}
	})

	t.Run("handles cmdlet without base", func(t *testing.T) {
		ss := sessionstate.NewSessionState()
		ctx := NewPipelineContext(ss, "test")

		pipeline := NewPipeline(ss, "test")
		stage := pipeline.AddStage(&testCmdlet{}, "test")

		// Should not panic
		initCmdletBase(stage.Cmdlet, ctx, stage, pipeline)
	})
}

// testCmdlet is a simple cmdlet implementation for testing.
type testCmdlet struct{}

func (c *testCmdlet) BeginProcessing()            {}
func (c *testCmdlet) ProcessRecord(input any) any { return input }
func (c *testCmdlet) EndProcessing()              {}

// testCmdletWithBase implements CmdletWithBase.
type testCmdletWithBase struct {
	Base *CmdletBase
}

func (c *testCmdletWithBase) SetBase(base *CmdletBase) {
	c.Base = base
}
func (c *testCmdletWithBase) BeginProcessing() {}
func (c *testCmdletWithBase) ProcessRecord(input any) any {
	if c.Base != nil {
		c.Base.WriteObject(input, false)
	}
	return nil
}
func (c *testCmdletWithBase) EndProcessing() {}
