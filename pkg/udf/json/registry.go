package json

import "github.com/itchyny/gojq"

// RegisterAll registers every JSON cmdlet.
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterJSONParse(),
		RegisterJSONStringify(),
		RegisterJSONPointer(),
		RegisterJSONPointerSet(),
		RegisterQueryStringParse(),
		RegisterQueryStringBuild(),
	}
}
