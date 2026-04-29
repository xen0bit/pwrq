package variables

import (
	"github.com/itchyny/gojq"
)

// RegisterAll registers all variable management cmdlets
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterSetVariable(),
		RegisterGetVariable(),
		RegisterRemoveVariable(),
	}
}
