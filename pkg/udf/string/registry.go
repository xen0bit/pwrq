package string

import "github.com/itchyny/gojq"

// RegisterAll registers every string cmdlet, the existing four and the case
// and text utilities.
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterUpper(),
		RegisterLower(),
		RegisterReverse(),
		RegisterReplace(),
		RegisterSlugify(),
		RegisterSnakeCase(),
		RegisterKebabCase(),
		RegisterCamelCase(),
		RegisterPascalCase(),
		RegisterTitleCase(),
		RegisterTruncate(),
		RegisterPadLeft(),
		RegisterPadRight(),
		RegisterMask(),
		RegisterCountOccurrences(),
	}
}
