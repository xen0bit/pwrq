// Package pipeline provides PowerShell-style pipeline infrastructure for pwrq.
// It implements cmdlet base classes, parameter binding, and pipeline context.
package pipeline

import (
	"fmt"
	"reflect"

	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
)

// CmdletBase provides common cmdlet functionality for all PowerShell-style cmdlets.
// It handles session state access, pipeline input/output, and error reporting.
type CmdletBase struct {
	// SessionState provides access to variables, aliases, and drives
	SessionState *sessionstate.SessionState

	// PipelineInput holds the current input object from the pipeline
	PipelineInput any

	// OutputWriter writes objects to the pipeline output
	OutputWriter func(any)

	// ErrorWriter writes errors to the error stream
	ErrorWriter func(error)
}

// WriteObject writes an object to the pipeline output.
// If enumerate is true and the object is a slice/array, each element is written separately.
func (c *CmdletBase) WriteObject(obj any, enumerate bool) {
	if c.OutputWriter == nil {
		return
	}

	if enumerate {
		val := reflect.ValueOf(obj)
		if val.Kind() == reflect.Slice || val.Kind() == reflect.Array {
			for i := 0; i < val.Len(); i++ {
				c.OutputWriter(val.Index(i).Interface())
			}
			return
		}
	}

	c.OutputWriter(obj)
}

// WriteError writes an error to the error stream.
func (c *CmdletBase) WriteError(err error) {
	if c.ErrorWriter != nil {
		c.ErrorWriter(err)
	}
}

// WriteVerbose writes a verbose message if verbose output is enabled.
// Checks $VerbosePreference variable in SessionState.
func (c *CmdletBase) WriteVerbose(message string) {
	if c.SessionState == nil {
		return
	}

	// Check verbose preference
	pref, _ := c.SessionState.GetVariable("VerbosePreference")
	shouldWrite := false

	if prefStr, ok := pref.(string); ok {
		shouldWrite = prefStr == "Continue" || prefStr == "Inquire"
	} else {
		// Default to not writing if not set
		shouldWrite = false
	}

	if shouldWrite {
		fmt.Fprintf(c.SessionState.Stderr, "VERBOSE: %s\n", message)
	}
}

// WriteDebug writes a debug message if debug output is enabled.
// Checks $DebugPreference variable in SessionState.
func (c *CmdletBase) WriteDebug(message string) {
	if c.SessionState == nil {
		return
	}

	// Check debug preference
	pref, _ := c.SessionState.GetVariable("DebugPreference")
	shouldWrite := false

	if prefStr, ok := pref.(string); ok {
		shouldWrite = prefStr == "Continue" || prefStr == "Inquire"
	} else {
		// Default to not writing if not set
		shouldWrite = false
	}

	if shouldWrite {
		fmt.Fprintf(c.SessionState.Stderr, "DEBUG: %s\n", message)
	}
}

// WriteWarning writes a warning message if warning output is enabled.
// Checks $WarningPreference variable in SessionState.
func (c *CmdletBase) WriteWarning(message string) {
	if c.SessionState == nil {
		return
	}

	// Check warning preference
	pref, _ := c.SessionState.GetVariable("WarningPreference")
	shouldWrite := false

	if prefStr, ok := pref.(string); ok {
		shouldWrite = prefStr == "Continue" || prefStr == "Inquire"
	} else {
		// Default to Continue if not set (PowerShell default behavior)
		shouldWrite = true
	}

	if shouldWrite {
		fmt.Fprintf(c.SessionState.Stderr, "WARNING: %s\n", message)
	}
}

// Cmdlet defines the interface for all cmdlets.
// Cmdlets follow the Begin-Process-End pattern from PowerShell.
type Cmdlet interface {
	// BeginProcessing is called once before any input is processed.
	// Use this for initialization and setup.
	BeginProcessing()

	// ProcessRecord is called for each input object in the pipeline.
	// It should return the transformed object(s) or nil to filter out.
	ProcessRecord(input any) any

	// EndProcessing is called after all input has been processed.
	// Use this for cleanup and final output.
	EndProcessing()
}

// SimpleCmdlet is a helper for cmdlets that only need ProcessRecord.
// It provides empty implementations of BeginProcessing and EndProcessing.
type SimpleCmdlet struct {
	CmdletBase
}

// BeginProcessing provides a no-op implementation.
func (s *SimpleCmdlet) BeginProcessing() {}

// EndProcessing provides a no-op implementation.
func (s *SimpleCmdlet) EndProcessing() {}
