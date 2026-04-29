package formatting

import (
	"github.com/itchyny/gojq"
)

// RegisterAll registers all formatting cmdlets
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterFormatList(),
		RegisterFormatTable(),
	}
}
