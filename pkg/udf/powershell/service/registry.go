package service

import (
	"github.com/itchyny/gojq"
)

// RegisterAll registers all service management cmdlets
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterGetService(),
		RegisterStartService(),
		RegisterStopService(),
	}
}
