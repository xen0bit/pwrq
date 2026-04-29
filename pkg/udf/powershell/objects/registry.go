package objects

import (
	"github.com/itchyny/gojq"
)

// RegisterAll registers all object manipulation cmdlets
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterSelectObject(),
		RegisterWhereObject(),
		RegisterSortObject(),
		RegisterGroupObject(),
		RegisterMeasureObject(),
	}
}
