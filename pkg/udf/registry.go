package udf

import (
	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/find"
)

// Registry holds all user-defined functions
type Registry struct {
	functions []gojq.CompilerOption
}

// NewRegistry creates a new UDF registry
func NewRegistry() *Registry {
	return &Registry{
		functions: make([]gojq.CompilerOption, 0),
	}
}

// Register adds a compiler option to the registry
func (r *Registry) Register(option gojq.CompilerOption) {
	r.functions = append(r.functions, option)
}

// Options returns all registered compiler options
func (r *Registry) Options() []gojq.CompilerOption {
	return r.functions
}

// DefaultRegistry returns the default registry with all built-in UDFs
func DefaultRegistry() *Registry {
	reg := NewRegistry()

	// Register all built-in UDFs
	reg.Register(find.RegisterFind())

	return reg
}
