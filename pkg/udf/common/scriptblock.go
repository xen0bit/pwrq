package common

import (
	"fmt"
	"sync"

	"github.com/itchyny/gojq"
)

// A ScriptBlock is a jq expression supplied as a string to a cmdlet, as in
//
//	where_object($items; {script: ".Age > 18 and (.Name | startswith(\"A\"))"})
//
// It is compiled once and evaluated per object. Earlier versions hand-parsed
// these strings, splitting on " -gt " and " | contains(" and a dozen other
// hardcoded shapes; anything outside that list silently misbehaved. Handing the
// string to gojq means a script block is exactly jq, with no second dialect to
// learn or to keep in step.
type ScriptBlock struct {
	source string
	code   *gojq.Code
}

var (
	scriptOptionsMu sync.RWMutex
	scriptOptions   []gojq.CompilerOption
)

// SetScriptBlockOptions supplies the compiler options script blocks are built
// with, so an expression inside a cmdlet can call the same UDFs as the query
// around it. The CLI sets this at startup; it is a hook rather than a direct
// import because the UDF registry imports this package.
func SetScriptBlockOptions(options []gojq.CompilerOption) {
	scriptOptionsMu.Lock()
	defer scriptOptionsMu.Unlock()
	scriptOptions = options
}

// CompileScriptBlock compiles a jq expression for repeated evaluation.
func CompileScriptBlock(source string) (*ScriptBlock, error) {
	query, err := gojq.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("invalid script block %q: %w", source, err)
	}

	scriptOptionsMu.RLock()
	options := scriptOptions
	scriptOptionsMu.RUnlock()

	code, err := gojq.Compile(query, options...)
	if err != nil {
		return nil, fmt.Errorf("invalid script block %q: %w", source, err)
	}
	return &ScriptBlock{source: source, code: code}, nil
}

// Source returns the expression the block was compiled from.
func (s *ScriptBlock) Source() string { return s.source }

// Eval runs the block against one object and returns its first output.
//
// A block producing no output yields nil, matching how jq treats an expression
// that filters everything away.
func (s *ScriptBlock) Eval(input any) (any, error) {
	iter := s.code.Run(input)
	v, ok := iter.Next()
	if !ok {
		return nil, nil
	}
	if err, isErr := v.(error); isErr {
		return nil, fmt.Errorf("script block %q: %w", s.source, err)
	}
	return v, nil
}

// EvalBool runs the block and applies jq's truthiness: false and null are
// false, every other value is true. This is what `select()` does, so a script
// block filters the way the equivalent jq expression would.
func (s *ScriptBlock) EvalBool(input any) (bool, error) {
	v, err := s.Eval(input)
	if err != nil {
		return false, err
	}
	return Truthy(v), nil
}

// Truthy reports jq's notion of truth: only false and null are false.
func Truthy(v any) bool {
	switch val := v.(type) {
	case nil:
		return false
	case bool:
		return val
	default:
		return true
	}
}
