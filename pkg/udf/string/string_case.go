// Case conversion, including the identifier styles.
package string

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/itchyny/gojq"
)

// The word-splitting rule shared by the case converters. A word boundary is any
// run of non-alphanumerics, or a camelCase transition (lowercase followed by
// uppercase), so "helloWorld foo_bar" splits into hello, world, foo, bar.
var (
	camelBoundary = regexp.MustCompile(`([a-z0-9])([A-Z])`)
	wordBoundary  = regexp.MustCompile(`[^A-Za-z0-9]+`)
)

func splitWords(s string) []string {
	s = camelBoundary.ReplaceAllString(s, `${1} ${2}`)
	parts := wordBoundary.Split(s, -1)
	words := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			words = append(words, strings.ToLower(p))
		}
	}
	return words
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	return strings.ToUpper(string(runes[:1])) + string(runes[1:])
}

// RegisterSlugify registers slugify, which turns any string into a
// lowercase-hyphenated identifier fit for URLs and file names.
func RegisterSlugify() gojq.CompilerOption {
	return registerCaseConverter("slugify", func(s string) string {
		return strings.Join(splitWords(s), "-")
	})
}

// RegisterSnakeCase registers snake_case, joining words with underscores.
func RegisterSnakeCase() gojq.CompilerOption {
	return registerCaseConverter("snake_case", func(s string) string {
		return strings.Join(splitWords(s), "_")
	})
}

// RegisterKebabCase registers kebab_case, joining words with hyphens.
func RegisterKebabCase() gojq.CompilerOption {
	return registerCaseConverter("kebab_case", func(s string) string {
		return strings.Join(splitWords(s), "-")
	})
}

// RegisterCamelCase registers camel_case, joining words with the first word
// lower and the rest capitalized.
func RegisterCamelCase() gojq.CompilerOption {
	return registerCaseConverter("camel_case", func(s string) string {
		words := splitWords(s)
		for i := 1; i < len(words); i++ {
			words[i] = capitalize(words[i])
		}
		return strings.Join(words, "")
	})
}

// RegisterPascalCase registers pascal_case, joining words each capitalized.
func RegisterPascalCase() gojq.CompilerOption {
	return registerCaseConverter("pascal_case", func(s string) string {
		words := splitWords(s)
		for i := range words {
			words[i] = capitalize(words[i])
		}
		return strings.Join(words, "")
	})
}

// RegisterTitleCase registers title_case, capitalizing the first letter of
// every word.
func RegisterTitleCase() gojq.CompilerOption {
	return registerCaseConverter("title_case", func(s string) string {
		words := splitWords(s)
		for i := range words {
			words[i] = capitalize(words[i])
		}
		return strings.Join(words, " ")
	})
}

// RegisterSentenceCase registers sentence_case, first letter capitalized and
// the rest lowercased.
func RegisterSentenceCase() gojq.CompilerOption {
	return registerTextFn("sentence_case", func(s string) any {
		s = strings.ToLower(s)
		for i, r := range s {
			if unicode.IsLetter(r) {
				runes := []rune(s)
				runes[i] = unicode.ToUpper(r)
				return string(runes)
			}
		}
		return s
	})
}

// RegisterSwapCase registers swap_case, uppercase and lowercase letters
// exchanged.
func RegisterSwapCase() gojq.CompilerOption {
	return registerTextFn("swap_case", func(s string) any {
		var b strings.Builder
		for _, r := range s {
			switch {
			case unicode.IsUpper(r):
				b.WriteRune(unicode.ToLower(r))
			case unicode.IsLower(r):
				b.WriteRune(unicode.ToUpper(r))
			default:
				b.WriteRune(r)
			}
		}
		return b.String()
	})
}

// RegisterCapitalizeFirst registers capitalize_first, the first letter
// uppercased with the rest left untouched.
func RegisterCapitalizeFirst() gojq.CompilerOption {
	return registerTextFn("capitalize_first", func(s string) any {
		for i, r := range s {
			if unicode.IsLetter(r) {
				runes := []rune(s)
				runes[i] = unicode.ToUpper(r)
				return string(runes)
			}
		}
		return s
	})
}
