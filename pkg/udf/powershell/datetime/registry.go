// Package datetime provides PowerShell-style date and time cmdlets.
// This file registers all datetime cmdlets.
package datetime

import (
	"github.com/itchyny/gojq"
)

// RegisterAll returns all datetime cmdlet compiler options
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterGetDate(),
		RegisterSetDate(),
		RegisterNewTimeSpan(),
	}
}
