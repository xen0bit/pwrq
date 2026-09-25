package tui

// Tokenisers for the two languages the TUI edits: jq (as pwrq extends it) and
// JSON.
//
// They are a port of the browser page's (pkg/web/src/js/highlight.js), and
// the tests pin the same cases, so both editors colour a query alike and
// agree on where completion may be offered. They are lexers, not parsers:
// they know what a token is, not what it means, which is enough for colour and
// means a half-written query - what an editor mostly contains - still
// highlights sensibly.
//
// Positions are in runes, because that is what the editor's cursor counts.

// Token kinds, named as the page names them.
const (
	tokComment  = "comment"
	tokString   = "string"
	tokInterp   = "interp"
	tokFormat   = "format"
	tokVariable = "variable"
	tokNumber   = "number"
	tokField    = "field"
	tokKeyword  = "keyword"
	tokLiteral  = "literal"
	tokCmdlet   = "cmdlet"
	tokBuiltin  = "builtin"
	tokIdent    = "ident"
	tokSpace    = "space"
	tokPunct    = "punct"
	tokKey      = "key"
)

// Token is a span of source and what kind of thing it is.
type Token struct {
	Kind       string
	Start, End int
}

var keywords = map[string]bool{
	"def": true, "as": true, "if": true, "then": true, "elif": true, "else": true, "end": true,
	"reduce": true, "foreach": true, "try": true, "catch": true, "label": true, "import": true,
	"include": true, "and": true, "or": true, "not": true, "__loc__": true,
}

var literals = map[string]bool{"true": true, "false": true, "null": true}

// Vocabulary is what the query tokeniser needs to tell a cmdlet from a jq
// builtin from an unknown name.
type Vocabulary struct {
	Cmdlets  map[string]bool
	Builtins map[string]bool
}

func isWord(r rune) bool {
	return r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func isSpace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	}
	return false
}

// at is src[i], or zero past either end.
func at(src []rune, i int) rune {
	if i < 0 || i >= len(src) {
		return 0
	}
	return src[i]
}

// TokenizeQuery walks jq source once, left to right. Every rune of the source
// belongs to exactly one token.
func TokenizeQuery(src []rune, vocab Vocabulary) []Token {
	var tokens []Token
	push := func(kind string, start, end int) {
		if end > start {
			tokens = append(tokens, Token{kind, start, end})
		}
	}

	i := 0
	for i < len(src) {
		c := src[i]
		start := i

		switch {
		case c == '#':
			for i < len(src) && src[i] != '\n' {
				i++
			}
			push(tokComment, start, i)

		case c == '"':
			// A string, including \( ... ) interpolation, which is coloured as
			// code rather than text.
			i++
			segment := start
			for i < len(src) {
				if src[i] == '\\' && at(src, i+1) == '(' {
					push(tokString, segment, i)
					open := i
					i += 2
					depth := 1
					for i < len(src) && depth > 0 {
						switch src[i] {
						case '(':
							depth++
						case ')':
							depth--
						case '"':
							// a nested string inside the interpolation
							i++
							for i < len(src) && src[i] != '"' {
								if src[i] == '\\' {
									i++
								}
								i++
							}
						}
						i++
					}
					if i > len(src) {
						i = len(src)
					}
					push(tokInterp, open, i)
					segment = i
					continue
				}
				if src[i] == '\\' {
					i += 2
					continue
				}
				if src[i] == '"' {
					i++
					break
				}
				i++
			}
			if i > len(src) {
				i = len(src)
			}
			push(tokString, segment, i)

		case c == '@' || c == '$':
			i++
			for i < len(src) && isWord(src[i]) {
				i++
			}
			if c == '@' {
				push(tokFormat, start, i)
			} else {
				push(tokVariable, start, i)
			}

		case isDigit(c) || c == '.' && isDigit(at(src, i+1)):
			for i < len(src) && (isDigit(src[i]) || src[i] == '.' || src[i] == '_' ||
				src[i] == 'e' || src[i] == 'E' || src[i] == '+' || src[i] == '-') {
				// stop at a sign that is not an exponent's
				if (src[i] == '+' || src[i] == '-') && at(src, i-1) != 'e' && at(src, i-1) != 'E' {
					break
				}
				if src[i] == '.' && !isDigit(at(src, i+1)) {
					break
				}
				i++
			}
			push(tokNumber, start, i)

		case c == '.':
			// .field, .["key"], .. and a bare .
			i++
			if at(src, i) == '.' {
				i++
				push(tokField, start, i)
				continue
			}
			for i < len(src) && isWord(src[i]) {
				i++
			}
			push(tokField, start, i)

		case c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			for i < len(src) && (isWord(src[i]) || src[i] == ':') {
				i++
			}
			word := string(src[start:i])
			kind := tokIdent
			switch {
			case keywords[word]:
				kind = tokKeyword
			case literals[word]:
				kind = tokLiteral
			case vocab.Cmdlets[word]:
				kind = tokCmdlet
			case vocab.Builtins[word]:
				kind = tokBuiltin
			}
			push(kind, start, i)

		case isSpace(c):
			for i < len(src) && isSpace(src[i]) {
				i++
			}
			push(tokSpace, start, i)

		default:
			i++
			push(tokPunct, start, i)
		}
	}
	return tokens
}

// TokenizeJSON colours JSON, including the multi-value streams the input pane
// accepts. Malformed text still tokenises: it is what a half-typed document
// looks like.
func TokenizeJSON(src []rune) []Token {
	var tokens []Token
	push := func(kind string, start, end int) {
		if end > start {
			tokens = append(tokens, Token{kind, start, end})
		}
	}

	i := 0
	for i < len(src) {
		c := src[i]
		start := i

		switch {
		case c == '"':
			i++
			for i < len(src) {
				if src[i] == '\\' {
					i += 2
					continue
				}
				if src[i] == '"' {
					i++
					break
				}
				i++
			}
			if i > len(src) {
				i = len(src)
			}
			// A string followed by a colon is a key, which is worth its own
			// colour: it is the shape of the data rather than the data.
			j := i
			for j < len(src) && isSpace(src[j]) {
				j++
			}
			if at(src, j) == ':' {
				push(tokKey, start, i)
			} else {
				push(tokString, start, i)
			}

		case c == '-' || isDigit(c):
			i++
			for i < len(src) && (isDigit(src[i]) || src[i] == '.' || src[i] == 'e' || src[i] == 'E' || src[i] == '+' || src[i] == '-') {
				i++
			}
			push(tokNumber, start, i)

		case c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			for i < len(src) && (src[i] >= 'a' && src[i] <= 'z' || src[i] >= 'A' && src[i] <= 'Z') {
				i++
			}
			if literals[string(src[start:i])] {
				push(tokLiteral, start, i)
			} else {
				push(tokIdent, start, i)
			}

		case isSpace(c):
			for i < len(src) && isSpace(src[i]) {
				i++
			}
			push(tokSpace, start, i)

		default:
			i++
			push(tokPunct, start, i)
		}
	}
	return tokens
}

// Word is the identifier being typed at a cursor: its text and where it
// starts, so accepting a completion can replace it. A leading $ or @ is part
// of it, and Prefix says which.
type Word struct {
	Text       string
	Start, End int
	Prefix     rune
}

// WordAt reports the identifier ending at a cursor position.
func WordAt(src []rune, cursor int) Word {
	start := cursor
	for start > 0 && isWord(src[start-1]) {
		start--
	}
	var prefix rune
	if start > 0 && (src[start-1] == '$' || src[start-1] == '@') {
		prefix = src[start-1]
		start--
	}
	return Word{Text: string(src[start:cursor]), Start: start, End: cursor, Prefix: prefix}
}

// InLiteral reports whether a cursor sits inside a string or a comment, where
// offering a function name would be wrong.
func InLiteral(tokens []Token, cursor int) bool {
	for _, token := range tokens {
		if cursor > token.Start && cursor <= token.End {
			return token.Kind == tokString || token.Kind == tokComment
		}
	}
	return false
}
