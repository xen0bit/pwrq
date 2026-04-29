// Package pipeline provides PowerShell-style pipeline infrastructure for pwrq.
package pipeline

import (
	"context"
	"sync"

	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
)

// PipelineContext holds the execution context for a pipeline.
// It provides access to session state, cancellation, and pipeline metadata.
type PipelineContext struct {
	context.Context

	// SessionState provides access to variables, aliases, and drives
	SessionState *sessionstate.SessionState

	// PipelineID is a unique identifier for this pipeline execution
	PipelineID string

	// CommandName is the name of the currently executing cmdlet
	CommandName string

	// PipelinePosition is the position of this cmdlet in the pipeline (0-based)
	PipelinePosition int

	// IsLastInPipeline indicates if this is the last cmdlet in the pipeline
	IsLastInPipeline bool

	// Cancellation channel for stopping pipeline execution
	cancel context.CancelFunc

	// Error channel for pipeline errors
	errChan chan error

	// lastError caches the last error for idempotent reads
	lastError error

	// mu protects concurrent access to the context
	mu sync.RWMutex
}

// NewPipelineContext creates a new pipeline context.
func NewPipelineContext(sessionState *sessionstate.SessionState, pipelineID string) *PipelineContext {
	ctx, cancel := context.WithCancel(context.Background())

	return &PipelineContext{
		Context:      ctx,
		SessionState: sessionState,
		PipelineID:   pipelineID,
		cancel:       cancel,
		errChan:      make(chan error, 1),
	}
}

// Cancel stops the pipeline execution.
func (pc *PipelineContext) Cancel() {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	if pc.cancel != nil {
		pc.cancel()
	}
}

// IsCancelled returns whether the pipeline has been cancelled.
func (pc *PipelineContext) IsCancelled() bool {
	return pc.Err() != nil
}

// ReportError reports an error from the pipeline.
// Errors are buffered and cached for idempotent retrieval.
func (pc *PipelineContext) ReportError(err error) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Cache the error for idempotent reads
	pc.lastError = err

	// Try to send to channel (non-blocking with buffer)
	select {
	case pc.errChan <- err:
	default:
		// Channel full - that's ok, error is cached
	}
}

// GetError returns the last reported error, or nil if none.
// This is idempotent - multiple calls return the same error.
func (pc *PipelineContext) GetError() error {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.lastError
}

// WithCommand creates a new context for a specific command.
func (pc *PipelineContext) WithCommand(name string, position int, isLast bool) *PipelineContext {
	return &PipelineContext{
		Context:          pc.Context,
		SessionState:     pc.SessionState,
		PipelineID:       pc.PipelineID,
		CommandName:      name,
		PipelinePosition: position,
		IsLastInPipeline: isLast,
		cancel:           pc.cancel,
		errChan:          pc.errChan,
	}
}

// GetVariable retrieves a variable from the session state.
func (pc *PipelineContext) GetVariable(name string) (any, error) {
	if pc.SessionState == nil {
		return nil, nil
	}
	return pc.SessionState.GetVariable(name)
}

// SetVariable sets a variable in the session state.
func (pc *PipelineContext) SetVariable(name string, value any) error {
	if pc.SessionState == nil {
		return nil
	}
	return pc.SessionState.SetVariable(name, value, 0)
}

// GetAlias retrieves an alias from the session state.
func (pc *PipelineContext) GetAlias(name string) (string, bool) {
	if pc.SessionState == nil {
		return "", false
	}
	return pc.SessionState.GetAlias(name)
}

// ResolveCommand resolves a command name, checking for aliases first.
func (pc *PipelineContext) ResolveCommand(name string) string {
	if pc.SessionState == nil {
		return name
	}
	if alias, ok := pc.SessionState.GetAlias(name); ok {
		return alias
	}
	return name
}

// CmdletWithBase is an optional interface for cmdlets that need base initialization.
// Cmdlets can implement this to receive their CmdletBase automatically.
type CmdletWithBase interface {
	Cmdlet
	SetBase(*CmdletBase)
}

// PipelineStage represents a single stage in the pipeline.
type PipelineStage struct {
	// Cmdlet is the cmdlet to execute
	Cmdlet Cmdlet

	// Name is the name of the cmdlet
	Name string

	// Position is the position in the pipeline
	Position int

	// IsLast indicates if this is the last stage
	IsLast bool

	// Input receives objects from the previous stage
	Input chan any

	// Output sends objects to the next stage
	Output chan any

	// Done signals when the stage has completed
	Done chan struct{}
}

// Pipeline represents a complete pipeline of cmdlets.
type Pipeline struct {
	// Context is the pipeline execution context
	Context *PipelineContext

	// Stages are the cmdlets in the pipeline
	Stages []*PipelineStage

	// FinalOutput receives the final pipeline output
	FinalOutput chan any

	// Errors receives pipeline errors
	Errors chan error
}

// NewPipeline creates a new pipeline.
func NewPipeline(sessionState *sessionstate.SessionState, pipelineID string) *Pipeline {
	return &Pipeline{
		Context:     NewPipelineContext(sessionState, pipelineID),
		Stages:      make([]*PipelineStage, 0),
		FinalOutput: make(chan any, 100),
		Errors:      make(chan error, 10),
	}
}

// AddStage adds a cmdlet stage to the pipeline.
func (p *Pipeline) AddStage(cmdlet Cmdlet, name string) *PipelineStage {
	stage := &PipelineStage{
		Cmdlet:   cmdlet,
		Name:     name,
		Position: len(p.Stages),
		Input:    make(chan any, 100),
		Output:   make(chan any, 100),
		Done:     make(chan struct{}),
	}

	if len(p.Stages) > 0 {
		// Connect to previous stage's output
		prevStage := p.Stages[len(p.Stages)-1]
		prevStage.Output = stage.Input
	}

	p.Stages = append(p.Stages, stage)
	return stage
}

// Execute runs the pipeline.
func (p *Pipeline) Execute(input <-chan any) {
	if len(p.Stages) == 0 {
		return
	}

	// Mark the last stage
	p.Stages[len(p.Stages)-1].IsLast = true
	p.Stages[len(p.Stages)-1].Output = p.FinalOutput

	// Start all stages
	for _, stage := range p.Stages {
		go p.executeStage(stage, input)
	}
}

// executeStage runs a single pipeline stage.
func (p *Pipeline) executeStage(stage *PipelineStage, initialInput <-chan any) {
	defer close(stage.Done)

	ctx := p.Context.WithCommand(stage.Name, stage.Position, stage.IsLast)

	// Initialize cmdlet - set up output and error writers
	initCmdletBase(stage.Cmdlet, ctx, stage, p)

	// Begin processing
	stage.Cmdlet.BeginProcessing()

	// Process input
	input := initialInput
	if stage.Position > 0 {
		input = stage.Input
	}

	for {
		select {
		case <-ctx.Done():
			// Drain remaining input to prevent upstream goroutines from blocking
			for range input {
				// Discard input
			}
			return
		case obj, ok := <-input:
			if !ok {
				// Input closed, move to end processing
				stage.Cmdlet.EndProcessing()
				return
			}

			// Process the record
			result := stage.Cmdlet.ProcessRecord(obj)
			if result != nil && !stage.IsLast {
				select {
				case stage.Output <- result:
				case <-ctx.Done():
					// Drain remaining input on cancellation
					for range input {
						// Discard input
					}
					return
				}
			}
		}
	}
}

// Wait waits for all stages to complete.
func (p *Pipeline) Wait() {
	for _, stage := range p.Stages {
		<-stage.Done
	}
	close(p.FinalOutput)
	close(p.Errors)
}

// initCmdletBase initializes a cmdlet's base functionality.
// All cmdlets receive a CmdletBase with properly configured output and error writers.
func initCmdletBase(cmdlet Cmdlet, ctx *PipelineContext, stage *PipelineStage, p *Pipeline) {
	// Validate SessionState (use empty session if nil)
	sessionState := ctx.SessionState
	if sessionState == nil {
		sessionState = sessionstate.NewSessionState()
	}

	// Create output writer with explicit cancellation check before each send
	outputWriter := func(obj any) {
		// Check cancellation before attempting to send
		if ctx.IsCancelled() {
			return
		}
		if !stage.IsLast {
			select {
			case stage.Output <- obj:
			case <-ctx.Done():
				// Context cancelled, don't block on send
				return
			}
		} else {
			select {
			case p.FinalOutput <- obj:
			case <-ctx.Done():
				// Context cancelled, don't block on send
				return
			}
		}
	}

	// Create error writer with explicit cancellation check before each send
	errorWriter := func(err error) {
		// Check cancellation before attempting to send
		if ctx.IsCancelled() {
			return
		}
		select {
		case p.Errors <- err:
		case <-ctx.Done():
			// Context cancelled, don't block on send
			return
		}
	}

	base := &CmdletBase{
		SessionState:  sessionState,
		PipelineInput: nil,
		OutputWriter:  outputWriter,
		ErrorWriter:   errorWriter,
	}

	// Set base on all cmdlets that support it
	if cmdletWithBase, ok := cmdlet.(CmdletWithBase); ok {
		cmdletWithBase.SetBase(base)
	}
}
