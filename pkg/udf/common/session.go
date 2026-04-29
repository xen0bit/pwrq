// Package common provides shared utilities for UDFs.
package common

import (
	"sync"

	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
)

var (
	sessionStateMu sync.RWMutex
	globalSession  *sessionstate.SessionState
)

// SetGlobalSessionState sets the global session state that UDFs can access.
// This should be called once during CLI initialization.
func SetGlobalSessionState(ss *sessionstate.SessionState) {
	sessionStateMu.Lock()
	defer sessionStateMu.Unlock()
	globalSession = ss
}

// GetSessionState returns the global session state.
// UDFs can call this to access variables, aliases, and drives.
func GetSessionState() *sessionstate.SessionState {
	sessionStateMu.RLock()
	defer sessionStateMu.RUnlock()
	return globalSession
}

// GetVariable retrieves a variable from the session state.
// Returns nil if the variable doesn't exist or session state is not initialized.
func GetVariable(name string) any {
	ss := GetSessionState()
	if ss == nil {
		return nil
	}
	val, err := ss.GetVariable(name)
	if err != nil {
		return nil
	}
	return val
}

// SetVariable sets a variable in the session state.
// Returns an error if session state is not initialized.
func SetVariable(name string, value any) error {
	ss := GetSessionState()
	if ss == nil {
		return nil
	}
	return ss.SetVariable(name, value, 0)
}

// GetAlias retrieves an alias from the session state.
// Returns empty string if the alias doesn't exist or session state is not initialized.
func GetAlias(name string) string {
	ss := GetSessionState()
	if ss == nil {
		return ""
	}
	cmd, ok := ss.GetAlias(name)
	if !ok {
		return ""
	}
	return cmd
}

// ResolveAlias resolves a command name through the alias chain.
// Returns the final command name (may be the same as input if not an alias).
func ResolveAlias(cmd string) string {
	ss := GetSessionState()
	if ss == nil {
		return cmd
	}

	// Follow alias chain (in case of nested aliases)
	seen := make(map[string]bool)
	current := cmd
	for {
		if seen[current] {
			// Circular alias detected, return as-is
			return cmd
		}
		seen[current] = true

		resolved, ok := ss.GetAlias(current)
		if !ok {
			return current
		}
		current = resolved
	}
}

// GetPreferenceVariable retrieves a preference variable value.
// Returns the default value if the variable doesn't exist.
func GetPreferenceVariable(name string, defaultValue string) string {
	val := GetVariable(name)
	if val == nil {
		return defaultValue
	}
	if str, ok := val.(string); ok {
		return str
	}
	return defaultValue
}

// GetErrorActionPreference returns the current error action preference.
func GetErrorActionPreference() string {
	return GetPreferenceVariable("ErrorActionPreference", "Continue")
}

// GetVerbosePreference returns the current verbose preference.
func GetVerbosePreference() string {
	return GetPreferenceVariable("VerbosePreference", "Continue")
}

// GetDebugPreference returns the current debug preference.
func GetDebugPreference() string {
	return GetPreferenceVariable("DebugPreference", "Continue")
}

// GetWarningPreference returns the current warning preference.
func GetWarningPreference() string {
	return GetPreferenceVariable("WarningPreference", "Continue")
}
