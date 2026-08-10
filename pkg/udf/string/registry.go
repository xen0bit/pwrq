package string

import "github.com/itchyny/gojq"

// RegisterAll registers every string cmdlet, the existing four and the case,
// text, predicate and pattern utilities.
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
		RegisterIsBlank(),
		RegisterIsAlphanumeric(),
		RegisterIsAlphabetic(),
		RegisterIsNumericString(),
		RegisterIsUppercase(),
		RegisterIsLowercase(),
		RegisterIsAscii(),
		RegisterWordCount(),
		RegisterNormalizeWhitespace(),
		RegisterAcronym(),
		RegisterEscapeRegex(),
		RegisterIsRegexValid(),
		RegisterGlobToRegex(),
		RegisterMatchGlob(),
		RegisterStripANSI(),
		RegisterTemplate(),
		RegisterWrapText(),
		RegisterIndent(),
		RegisterPluralize(),
	}
}
