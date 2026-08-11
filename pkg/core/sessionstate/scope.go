// Package sessionstate provides PowerShell-style session state management for pwrq.
// It implements scope hierarchy, variable storage, and PSDrive support.
package sessionstate

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// ScopeType represents the type of scope in the hierarchy.
type ScopeType int

const (
	ScopeGlobal ScopeType = iota
	ScopeScript
	ScopeLocal
	ScopePrivate
)

func (s ScopeType) String() string {
	switch s {
	case ScopeGlobal:
		return "Global"
	case ScopeScript:
		return "Script"
	case ScopeLocal:
		return "Local"
	case ScopePrivate:
		return "Private"
	default:
		return "Unknown"
	}
}

// VariableEntry represents a variable in the session state.
type VariableEntry struct {
	Name        string
	Value       any
	Description string
	Options     VariableOptions
	Scope       ScopeType
}

// VariableOptions represents variable options (like PowerShell's -Option).
// These are bitflags that can be combined using bitwise OR.
type VariableOptions int

const (
	None     VariableOptions = iota
	ReadOnly VariableOptions = 1 << iota
	Constant
	Private
	AllScope
)

// Scope represents a single scope in the hierarchy.
type Scope struct {
	Type      ScopeType
	Variables map[string]*VariableEntry
	Parent    *Scope // Parent scope for lookup chain
}

// NewScope creates a new scope with the given type and parent.
func NewScope(scopeType ScopeType, parent *Scope) *Scope {
	return &Scope{
		Type:      scopeType,
		Variables: make(map[string]*VariableEntry),
		Parent:    parent,
	}
}

// SessionState represents the complete session state with scope hierarchy and drives.
type SessionState struct {
	mu             sync.RWMutex
	GlobalScope    *Scope
	CurrentScope   *Scope
	Drives         map[string]*PSDrive
	AliasMap       map[string]string   // command alias -> actual command
	Stderr         io.Writer           // stderr output stream for verbose/debug/warning
	LocationStacks map[string][]string // named location stacks for pushd/popd
}

// PSDrive represents a PowerShell-style drive (like FileSystem:, Env:, Variable:).
type PSDrive struct {
	Name        string
	Root        string
	Description string
	Items       map[string]any
}

// NewSessionState creates a new session state with initialized scopes and drives.
func NewSessionState() *SessionState {
	globalScope := NewScope(ScopeGlobal, nil)

	ss := &SessionState{
		GlobalScope:    globalScope,
		CurrentScope:   globalScope,
		Drives:         make(map[string]*PSDrive),
		AliasMap:       make(map[string]string),
		Stderr:         os.Stderr,
		LocationStacks: make(map[string][]string),
	}

	// Initialize standard drives
	ss.initializeDrives()

	return ss
}

// initializeDrives sets up the standard PowerShell drives.
func (ss *SessionState) initializeDrives() {
	// Variable: drive - variables in session
	ss.Drives["Variable"] = &PSDrive{
		Name:        "Variable",
		Root:        "",
		Description: "Variables in the current session",
		Items:       make(map[string]any),
	}

	// Env: drive - environment variables
	ss.Drives["Env"] = &PSDrive{
		Name:        "Env",
		Root:        "",
		Description: "Environment variables",
		Items:       make(map[string]any),
	}
	ss.syncEnvDrive()

	// Alias: drive - command aliases
	ss.Drives["Alias"] = &PSDrive{
		Name:        "Alias",
		Root:        "",
		Description: "Command aliases",
		Items:       make(map[string]any),
	}

	// Function: drive - user-defined functions
	ss.Drives["Function"] = &PSDrive{
		Name:        "Function",
		Root:        "",
		Description: "User-defined functions",
		Items:       make(map[string]any),
	}
}

// syncEnvDrive populates the Env: drive with current environment variables.
func (ss *SessionState) syncEnvDrive() {
	if drive, ok := ss.Drives["Env"]; ok {
		for _, env := range os.Environ() {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				drive.Items[parts[0]] = parts[1]
			}
		}
	}
}

// NewScriptScope creates a new script scope with the current scope as parent.
func (ss *SessionState) NewScriptScope() *Scope {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	scriptScope := NewScope(ScopeScript, ss.CurrentScope)
	ss.CurrentScope = scriptScope
	return scriptScope
}

// NewLocalScope creates a new local scope (e.g., for a function or block).
func (ss *SessionState) NewLocalScope() *Scope {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	localScope := NewScope(ScopeLocal, ss.CurrentScope)
	ss.CurrentScope = localScope
	return localScope
}

// PopScope removes the current scope and returns to the parent.
func (ss *SessionState) PopScope() {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if ss.CurrentScope.Parent != nil {
		ss.CurrentScope = ss.CurrentScope.Parent
	}
}

// SetVariable sets a variable in the session state.
func (ss *SessionState) SetVariable(name string, value any, options VariableOptions) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	// Remove leading $ if present
	name = strings.TrimPrefix(name, "$")

	// Validate variable name
	if err := validateVariableName(name); err != nil {
		return err
	}

	// Parse scope prefix from name (e.g., "global:var", "script:var", "local:var")
	targetScope, variableName := ss.parseScopeFromName(name)

	// Find or create the target scope
	scope := ss.findOrCreateScope(targetScope)
	if scope == nil {
		return fmt.Errorf("failed to locate or create target scope")
	}

	// Check if variable already exists in this scope
	if existing, ok := scope.Variables[variableName]; ok {
		// Enforce ReadOnly option
		if existing.Options&ReadOnly != 0 {
			return fmt.Errorf("cannot overwrite read-only variable %q", variableName)
		}
		// Enforce Constant option
		if existing.Options&Constant != 0 {
			return fmt.Errorf("cannot overwrite constant variable %q", variableName)
		}
	}

	// Create or update the variable entry
	entry := &VariableEntry{
		Name:    variableName,
		Value:   value,
		Options: options,
		Scope:   scope.Type,
	}
	scope.Variables[variableName] = entry

	// Update Variable: drive
	if drive, ok := ss.Drives["Variable"]; ok {
		drive.Items[variableName] = value
	}

	// Propagate AllScope variables to child scopes
	if options&AllScope != 0 {
		ss.propagateAllScopeVariable(variableName, entry)
	}

	return nil
}

// validateVariableName validates that a variable name is legal.
// PowerShell rules: must start with letter or underscore, can contain letters,
// digits, underscores. No empty names.
func validateVariableName(name string) error {
	// Handle scoped names - extract just the variable part
	if idx := strings.Index(name, ":"); idx != -1 {
		name = name[idx+1:]
	}

	if name == "" {
		return fmt.Errorf("variable name cannot be empty")
	}

	// First character must be letter or underscore
	first := name[0]
	if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z') || first == '_') {
		return fmt.Errorf("variable name must start with a letter or underscore, got %q", first)
	}

	// Remaining characters must be alphanumeric or underscore
	for i := 1; i < len(name); i++ {
		c := name[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return fmt.Errorf("variable name contains invalid character %q at position %d", c, i)
		}
	}

	return nil
}

// parseScopeFromName extracts the scope prefix from a variable name.
// Returns the target scope type and the variable name without prefix.
// Supported prefixes: global:, script:, local:, private:
// If no prefix, returns ScopeLocal (current scope).
func (ss *SessionState) parseScopeFromName(name string) (ScopeType, string) {
	idx := strings.Index(name, ":")
	if idx == -1 {
		// No scope prefix, use current scope
		return ss.CurrentScope.Type, name
	}

	prefix := strings.ToLower(name[:idx])
	variableName := name[idx+1:]

	switch prefix {
	case "global":
		return ScopeGlobal, variableName
	case "script":
		// Find the script scope (first ScopeScript in chain)
		return ScopeScript, variableName
	case "local":
		return ScopeLocal, variableName
	case "private":
		return ScopePrivate, variableName
	default:
		// Unknown scope, default to local
		return ss.CurrentScope.Type, name
	}
}

// findOrCreateScope locates or creates the target scope based on scope type.
func (ss *SessionState) findOrCreateScope(targetType ScopeType) *Scope {
	switch targetType {
	case ScopeGlobal:
		return ss.GlobalScope

	case ScopeScript:
		// Find the script scope in the chain
		scope := ss.CurrentScope
		for scope != nil {
			if scope.Type == ScopeScript {
				return scope
			}
			scope = scope.Parent
		}
		// No script scope found, create one
		scriptScope := NewScope(ScopeScript, ss.GlobalScope)
		return scriptScope

	case ScopeLocal:
		return ss.CurrentScope

	case ScopePrivate:
		// Private scope is like local but not visible to children
		// For simplicity, treat as local scope
		return ss.CurrentScope

	default:
		return ss.CurrentScope
	}
}

// propagateAllScopeVariable propagates an AllScope variable to all child scopes.
func (ss *SessionState) propagateAllScopeVariable(name string, entry *VariableEntry) {
	// Walk through all scopes and add this variable if not already present
	walkScopes := func(scope *Scope) {
		if scope == nil {
			return
		}

		// Add variable if not already in this scope
		if _, exists := scope.Variables[name]; !exists {
			scope.Variables[name] = &VariableEntry{
				Name:    entry.Name,
				Value:   entry.Value,
				Options: entry.Options,
				Scope:   scope.Type,
			}
		}

		// Recurse through child scopes
		// Note: We don't have direct child pointers, so we'd need to track them
		// For now, AllScope means visible in all scopes via lookup chain
	}

	walkScopes(ss.GlobalScope)
}

// GetVariable retrieves a variable value, searching up the scope chain.
func (ss *SessionState) GetVariable(name string) (any, error) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	// Remove leading $ if present
	name = strings.TrimPrefix(name, "$")

	// Search from current scope up to global
	scope := ss.CurrentScope
	for scope != nil {
		if entry, ok := scope.Variables[name]; ok {
			return entry.Value, nil
		}
		scope = scope.Parent
	}

	return nil, fmt.Errorf("variable %q not found", name)
}

// GetVariableEntry retrieves the full variable entry (with metadata).
func (ss *SessionState) GetVariableEntry(name string) (*VariableEntry, error) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	name = strings.TrimPrefix(name, "$")

	scope := ss.CurrentScope
	for scope != nil {
		if entry, ok := scope.Variables[name]; ok {
			return entry, nil
		}
		scope = scope.Parent
	}

	return nil, fmt.Errorf("variable %q not found", name)
}

// RemoveVariable removes a variable from the session state.
func (ss *SessionState) RemoveVariable(name string) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	name = strings.TrimPrefix(name, "$")

	// Find and remove from the scope where it's defined
	scope := ss.CurrentScope
	for scope != nil {
		if entry, ok := scope.Variables[name]; ok {
			// Enforce ReadOnly option
			if entry.Options&ReadOnly != 0 {
				return fmt.Errorf("cannot remove read-only variable %q", name)
			}
			// Enforce Constant option
			if entry.Options&Constant != 0 {
				return fmt.Errorf("cannot remove constant variable %q", name)
			}
			delete(scope.Variables, name)
			// Also remove from Variable: drive
			if drive, ok := ss.Drives["Variable"]; ok {
				delete(drive.Items, name)
			}
			return nil
		}
		scope = scope.Parent
	}

	return fmt.Errorf("variable %q not found", name)
}

// GetVariables returns all variables visible from the current scope.
func (ss *SessionState) GetVariables() map[string]*VariableEntry {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	result := make(map[string]*VariableEntry)

	// Walk from global down to current, allowing shadowing
	seen := make(map[string]bool)

	// Build scope chain from global to current
	var chain []*Scope
	scope := ss.CurrentScope
	for scope != nil {
		chain = append([]*Scope{scope}, chain...)
		scope = scope.Parent
	}

	// Process from global to current (later scopes shadow earlier)
	for _, s := range chain {
		for name, entry := range s.Variables {
			if !seen[name] {
				result[name] = entry
				seen[name] = true
			}
		}
	}

	return result
}

// SetAlias creates a command alias.
func (ss *SessionState) SetAlias(name, command string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	ss.AliasMap[name] = command

	// Also update Alias: drive
	if drive, ok := ss.Drives["Alias"]; ok {
		drive.Items[name] = command
	}
}

// GetAlias retrieves a command alias.
func (ss *SessionState) GetAlias(name string) (string, bool) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	command, ok := ss.AliasMap[name]
	return command, ok
}

// GetAliases returns all aliases.
func (ss *SessionState) GetAliases() map[string]string {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	result := make(map[string]string)
	for k, v := range ss.AliasMap {
		result[k] = v
	}
	return result
}

// RemoveAlias removes an alias.
func (ss *SessionState) RemoveAlias(name string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	delete(ss.AliasMap, name)

	// Also remove from Alias: drive
	if drive, ok := ss.Drives["Alias"]; ok {
		delete(drive.Items, name)
	}
}

// GetDrive retrieves a PSDrive by name.
func (ss *SessionState) GetDrive(name string) (*PSDrive, bool) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	drive, ok := ss.Drives[name]
	return drive, ok
}

// GetDrives returns all drives.
func (ss *SessionState) GetDrives() map[string]*PSDrive {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	result := make(map[string]*PSDrive)
	for k, v := range ss.Drives {
		result[k] = v
	}
	return result
}

// GetDriveItem retrieves an item from a drive.
func (ss *SessionState) GetDriveItem(driveName, itemName string) (any, error) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	drive, ok := ss.Drives[driveName]
	if !ok {
		return nil, fmt.Errorf("drive %q not found", driveName)
	}

	item, ok := drive.Items[itemName]
	if !ok {
		return nil, fmt.Errorf("item %q not found in drive %q", itemName, driveName)
	}

	return item, nil
}

// SetDriveItem sets an item in a drive.
func (ss *SessionState) SetDriveItem(driveName, itemName string, value any) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	drive, ok := ss.Drives[driveName]
	if !ok {
		return fmt.Errorf("drive %q not found", driveName)
	}

	drive.Items[itemName] = value
	return nil
}

// UpdateEnvVariable updates an environment variable and syncs the Env: drive.
func (ss *SessionState) UpdateEnvVariable(name, value string) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	// Update actual environment
	if err := os.Setenv(name, value); err != nil {
		return fmt.Errorf("failed to set environment variable: %v", err)
	}

	// Sync to Env: drive
	if drive, ok := ss.Drives["Env"]; ok {
		drive.Items[name] = value
	}

	return nil
}

// GetEnvVariable retrieves an environment variable.
func (ss *SessionState) GetEnvVariable(name string) (string, error) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	value, ok := os.LookupEnv(name)
	if !ok {
		return "", fmt.Errorf("environment variable %q not found", name)
	}

	return value, nil
}

// ResolvePath resolves a path that may use a drive prefix (e.g., "Env:PATH", "Variable:count").
func (ss *SessionState) ResolvePath(path string) (string, any, error) {
	parts := strings.SplitN(path, ":", 2)
	if len(parts) != 2 {
		// No drive prefix, treat as filesystem path
		return "", nil, fmt.Errorf("path %q does not include drive prefix", path)
	}

	driveName := parts[0]
	itemName := strings.TrimPrefix(parts[1], "/")

	drive, ok := ss.Drives[driveName]
	if !ok {
		return "", nil, fmt.Errorf("drive %q not found", driveName)
	}

	return driveName, drive.Items[itemName], nil
}

// PushLocationStack pushes a location onto the named stack.
func (ss *SessionState) PushLocationStack(stackName, location string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if ss.LocationStacks == nil {
		ss.LocationStacks = make(map[string][]string)
	}

	if _, exists := ss.LocationStacks[stackName]; !exists {
		ss.LocationStacks[stackName] = make([]string, 0)
	}

	ss.LocationStacks[stackName] = append(ss.LocationStacks[stackName], location)
}

// PopLocationStack pops a location from the named stack.
// Returns the popped location and an error if the stack is empty.
func (ss *SessionState) PopLocationStack(stackName string) (string, error) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if ss.LocationStacks == nil {
		return "", fmt.Errorf("location stack %q not found", stackName)
	}

	stack, exists := ss.LocationStacks[stackName]
	if !exists || len(stack) == 0 {
		return "", fmt.Errorf("location stack %q is empty", stackName)
	}

	// Pop the last item
	popped := stack[len(stack)-1]
	ss.LocationStacks[stackName] = stack[:len(stack)-1]

	return popped, nil
}

// GetLocationStack returns a copy of the named location stack.
func (ss *SessionState) GetLocationStack(stackName string) []string {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	if ss.LocationStacks == nil {
		return []string{}
	}

	stack, exists := ss.LocationStacks[stackName]
	if !exists {
		return []string{}
	}

	// Return a copy
	result := make([]string, len(stack))
	copy(result, stack)
	return result
}

// GetLocationStackNames returns all available location stack names.
func (ss *SessionState) GetLocationStackNames() []string {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	if ss.LocationStacks == nil {
		return []string{}
	}

	names := make([]string, 0, len(ss.LocationStacks))
	for name := range ss.LocationStacks {
		names = append(names, name)
	}
	return names
}
