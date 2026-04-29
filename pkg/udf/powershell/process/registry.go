package process

import (
	"github.com/itchyny/gojq"
)

// RegisterAll registers all process management cmdlets
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterGetProcess(),
		RegisterStopProcess(),
		RegisterStartProcess(),
	}
}
