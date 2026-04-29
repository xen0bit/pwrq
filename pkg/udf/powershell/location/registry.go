package location

import (
	"github.com/itchyny/gojq"
)

// RegisterAll registers all location management cmdlets
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterGetLocation(),
		RegisterSetLocation(),
		RegisterPushLocation(),
		RegisterPopLocation(),
	}
}
