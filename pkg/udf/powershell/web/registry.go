// Package web provides PowerShell-style web and network cmdlets.
// This file registers all web-related cmdlets.
package web

import (
	"github.com/itchyny/gojq"
)

// RegisterAll returns all web cmdlet compiler options
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterInvokeWebRequest(),
		RegisterInvokeRestMethod(),
		RegisterTestConnection(),
	}
}
