package filesystem

import (
	"github.com/itchyny/gojq"
)

// RegisterAll registers all filesystem cmdlets
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterGetChildItem(),
		RegisterTestPath(),
		RegisterSetContent(),
		RegisterAddContent(),
		RegisterOutFile(),
		RegisterJoinPath(),
		RegisterSplitPath(),
		RegisterResolvePath(),
		RegisterNewItem(),
		RegisterCopyItem(),
		RegisterMoveItem(),
	}
}
