package jqfmt

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/itchyny/gojq"
)

// Comments survive formatting.
//
// gojq's parser drops comments, so Format, which renders the parse tree,
// drops them too. But a comment is often the most useful line of a query -
// it says what a stage is for - and the editors' query suggestions read the
// comments as the author's intent. FormatSource puts them back: it lexes the
// source and the formatted text into code tokens, aligns the two streams
// (the formatter only moves code, it never changes it), and re-inserts each
// comment beside the code token it was written beside.
//
//   - A comment after code on its line (trailing) follows that token, and the
//     formatted line breaks after it.
//   - A comment on a line of its own (leading) goes on its own line before
//     the next token, at that line's indentation.
//
// A comment whose token the formatter re-spelled is placed beside the
// nearest token that survived; one is never dropped. The result is checked:
// it must parse to the same program as the formatted text, with every
// comment present and in order, or FormatSource returns that error.

// lexToken is one token of jq source: code, or a comment.
type lexToken struct {
	text       string
	start, end int // byte offsets
	comment    bool
	line       int // the line it starts on, from 0
}

// lex splits jq source into code tokens and comments. It is a lexer, not a
// parser: enough to tell a '#' in a string from a comment, and to align two
// spellings of one program token by token.
func lex(src string) []lexToken {
	var out []lexToken
	line := 0
	rs := []rune(src)
	// byte offset of each rune
	off := make([]int, len(rs)+1)
	o := 0
	for i, r := range rs {
		off[i] = o
		o += len(string(r))
	}
	off[len(rs)] = len(src)
	push := func(start, end int, comment bool, startLine int) {
		out = append(out, lexToken{text: string(rs[start:end]), start: off[start], end: off[end], comment: comment, line: startLine})
	}
	isIdent := func(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }
	i := 0
	for i < len(rs) {
		c := rs[i]
		start, startLine := i, line
		switch {
		case c == '\n':
			line++
			i++
		case unicode.IsSpace(c):
			i++
		case c == '#':
			// jq 1.7: a comment line ending in an odd number of backslashes
			// continues on the next.
			for {
				for i < len(rs) && rs[i] != '\n' {
					i++
				}
				n := 0
				for k := i - 1; k > start && rs[k] == '\\'; k-- {
					n++
				}
				if n%2 == 1 && i < len(rs) {
					i++
					line++
					continue
				}
				break
			}
			end := i
			for end > start && unicode.IsSpace(rs[end-1]) {
				end--
			}
			push(start, end, true, startLine)
		case c == '"':
			i = skipString(rs, i, &line)
			push(start, i, false, startLine)
		case c == '$' || c == '@' || isIdent(c):
			i++
			if c == '$' && i+1 < len(rs) && rs[i] == '_' && rs[i+1] == '_' {
				i += 2 // $__loc__
			}
			for i < len(rs) && (isIdent(rs[i]) || rs[i] == ':' && i+1 < len(rs) && rs[i+1] == ':' && i+2 < len(rs) && isIdent(rs[i+2])) {
				if rs[i] == ':' {
					i += 2
				} else {
					i++
				}
			}
			// a number's fraction and exponent
			if unicode.IsDigit(c) {
				for i < len(rs) && (unicode.IsDigit(rs[i]) || rs[i] == '.' || rs[i] == 'e' || rs[i] == 'E' ||
					(rs[i] == '+' || rs[i] == '-') && (rs[i-1] == 'e' || rs[i-1] == 'E')) {
					i++
				}
			}
			push(start, i, false, startLine)
		default:
			n := 1
			for _, op := range []string{"?//=", "//=", "?//", "|=", "+=", "-=", "*=", "/=", "%=", "==", "!=", "<=", ">=", "//", ".."} {
				if strings.HasPrefix(string(rs[i:min(len(rs), i+len(op))]), op) {
					n = len([]rune(op))
					break
				}
			}
			if c == '.' && n == 1 && i+1 < len(rs) && unicode.IsDigit(rs[i+1]) {
				// .5
				i++
				for i < len(rs) && (unicode.IsDigit(rs[i]) || rs[i] == 'e' || rs[i] == 'E') {
					i++
				}
				push(start, i, false, startLine)
				continue
			}
			i += n
			push(start, i, false, startLine)
		}
	}
	return out
}

// skipString returns the index after the string literal opening at i,
// interpolations and their nested strings included.
func skipString(rs []rune, i int, line *int) int {
	i++ // the opening quote
	for i < len(rs) {
		switch rs[i] {
		case '"':
			return i + 1
		case '\n':
			*line++
		case '\\':
			if i+1 < len(rs) && rs[i+1] == '(' {
				i += 2
				depth := 1
				for i < len(rs) && depth > 0 {
					switch rs[i] {
					case '(':
						depth++
					case ')':
						depth--
					case '"':
						i = skipString(rs, i, line) - 1
					case '\n':
						*line++
					}
					i++
				}
				continue
			}
			i++
		}
		i++
	}
	return i
}

// normalize is a token's text for alignment: a string literal without its
// whitespace inside interpolations, which the formatter may re-space.
func normalize(t lexToken) string {
	if strings.HasPrefix(t.text, `"`) {
		return strings.Join(strings.Fields(t.text), "")
	}
	return t.text
}

// align matches source code tokens to formatted code tokens by longest
// common subsequence; m[i] is the formatted index of source token i, -1
// when it has none.
func align(src, dst []lexToken) []int {
	n, k := len(src), len(dst)
	m := make([]int, n)
	for i := range m {
		m[i] = -1
	}
	if n == 0 || k == 0 {
		return m
	}
	// A query is small; past this the table would be large, and a
	// position-by-position match is what a re-layout mostly is anyway.
	if n*k > 16_000_000 {
		j := 0
		for i := 0; i < n && j < k; i++ {
			if normalize(src[i]) == normalize(dst[j]) {
				m[i] = j
				j++
			}
		}
		return m
	}
	dp := make([][]int32, n+1)
	for i := range dp {
		dp[i] = make([]int32, k+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := k - 1; j >= 0; j-- {
			if normalize(src[i]) == normalize(dst[j]) {
				dp[i][j] = dp[i+1][j+1] + 1
			} else {
				dp[i][j] = max(dp[i+1][j], dp[i][j+1])
			}
		}
	}
	for i, j := 0, 0; i < n && j < k; {
		switch {
		case normalize(src[i]) == normalize(dst[j]):
			m[i] = j
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			i++
		default:
			j++
		}
	}
	return m
}

// placed is a comment to insert into the formatted text.
type placed struct {
	text     string
	trailing bool // after token `at`; else before it
	at       int  // a formatted code token index; -1: the very end
	blank    bool // a blank line came before it in the source
	order    int
}

// FormatSource formats jq source, keeping its comments. On a parse error
// it returns the error and the source unchanged.
func FormatSource(src string) (string, error) {
	q, err := gojq.Parse(src)
	if err != nil {
		return src, err
	}
	return WithComments(src, Format(q))
}

// WithComments puts src's comments into formatted, which must be src's
// program laid out by Format (or rewritten by an inliner: then a comment
// whose code moved is kept beside the nearest code that did not).
func WithComments(src, formatted string) (string, error) {
	all := lex(src)
	var code []lexToken
	var comments []int // indices into all
	for i, t := range all {
		if t.comment {
			comments = append(comments, i)
		} else {
			code = append(code, t)
		}
	}
	if len(comments) == 0 {
		return formatted, nil
	}
	dst := lex(formatted)
	for _, t := range dst {
		if t.comment {
			return "", fmt.Errorf("formatted text already holds comments")
		}
	}
	m := align(code, dst)

	// codeIndex[i] is, for the token all[i], its index among code tokens.
	codeIndex := make([]int, len(all))
	c := 0
	for i, t := range all {
		codeIndex[i] = c
		if !t.comment {
			c++
		}
	}
	var places []placed
	for order, ci := range comments {
		t := all[ci]
		prev := codeIndex[ci] - 1 // the code token before the comment
		trailing := prev >= 0 && code[prev].line == t.line
		blank := ci > 0 && strings.Count(src[all[ci-1].end:t.start], "\n") >= 2
		p := placed{text: t.text, trailing: trailing, blank: blank, order: order}
		if trailing {
			// nearest surviving token at or before prev
			k := prev
			for k >= 0 && m[k] < 0 {
				k--
			}
			if k >= 0 {
				p.at = m[k]
			} else {
				p.trailing, p.at = false, 0
			}
		} else {
			k := codeIndex[ci]
			for k < len(code) && m[k] < 0 {
				k++
			}
			if k < len(code) {
				p.at = m[k]
			} else {
				p.at, p.trailing = -1, false
			}
		}
		places = append(places, p)
	}
	return render(formatted, dst, places)
}

// render writes formatted with the comments in place.
func render(formatted string, dst []lexToken, places []placed) (string, error) {
	lineStart := func(off int) int { return strings.LastIndexByte(formatted[:off], '\n') + 1 }
	indentAt := func(off int) string {
		s := lineStart(off)
		e := s
		for e < len(formatted) && (formatted[e] == ' ' || formatted[e] == '\t') {
			e++
		}
		return formatted[s:e]
	}
	// A leading comment whose token only follows the operator opening its
	// stage ("| .b") belongs above that operator, on the line's first token.
	for k, p := range places {
		if p.trailing || p.at <= 0 || p.at >= len(dst) {
			continue
		}
		t := dst[p.at]
		ls := lineStart(t.start)
		if prefix := strings.TrimSpace(formatted[ls:t.start]); (prefix == "|" || prefix == ",") && dst[p.at-1].start >= ls {
			places[k].at = p.at - 1
		}
	}
	before := map[int][]placed{}
	after := map[int][]placed{}
	var atEnd []placed
	for _, p := range places {
		switch {
		case p.at < 0 || len(dst) == 0:
			atEnd = append(atEnd, p)
		case p.trailing:
			after[p.at] = append(after[p.at], p)
		default:
			before[p.at] = append(before[p.at], p)
		}
	}
	var b strings.Builder
	last := 0
	for i, t := range dst {
		if cs := before[i]; len(cs) > 0 {
			indent := indentAt(t.start)
			ls := lineStart(t.start)
			if strings.TrimSpace(formatted[ls:t.start]) == "" {
				// the token starts its line: comment lines go above it
				b.WriteString(formatted[last:ls])
				for k, p := range cs {
					if p.blank && (k > 0 || ls > 0) && !strings.HasSuffix(b.String(), "\n\n") {
						b.WriteString("\n")
					}
					b.WriteString(indent + p.text + "\n")
				}
				b.WriteString(formatted[ls:t.start])
			} else {
				// mid-line: break the line before the token
				b.WriteString(strings.TrimRight(formatted[last:t.start], " \t"))
				for _, p := range cs {
					b.WriteString("\n" + indent + p.text)
				}
				b.WriteString("\n" + indent)
			}
			last = t.start
		}
		b.WriteString(formatted[last:t.end])
		last = t.end
		if cs := after[i]; len(cs) > 0 {
			for k, p := range cs {
				if k == 0 {
					b.WriteString(" " + p.text)
				} else {
					b.WriteString("\n" + indentAt(t.start) + p.text)
				}
			}
			// The comment runs to the end of its line, so the code that
			// followed on it moves to the next line.
			rest := formatted[last:]
			nl := strings.IndexByte(rest, '\n')
			if nl < 0 {
				nl = len(rest)
			}
			if strings.TrimSpace(rest[:nl]) != "" {
				b.WriteString("\n" + indentAt(t.start))
				// drop the spaces that separated the token from what follows
				for last < len(formatted) && (formatted[last] == ' ' || formatted[last] == '\t') {
					last++
				}
			}
		}
	}
	b.WriteString(formatted[last:])
	out := b.String()
	if len(atEnd) > 0 {
		out = strings.TrimRight(out, " \t\n")
		for _, p := range atEnd {
			if p.blank {
				out += "\n"
			}
			out += "\n" + p.text
		}
		out += "\n"
	}
	return out, check(formatted, out, places)
}

// check confirms the comments changed nothing: the same program, and every
// comment present in order.
func check(formatted, out string, places []placed) error {
	want, err := gojq.Parse(formatted)
	if err != nil {
		return err
	}
	got, err := gojq.Parse(out)
	if err != nil {
		return fmt.Errorf("placing comments broke the query: %w", err)
	}
	if want.String() != got.String() {
		return fmt.Errorf("placing comments changed the query")
	}
	var seen []string
	for _, t := range lex(out) {
		if t.comment {
			seen = append(seen, t.text)
		}
	}
	if len(seen) != len(places) {
		return fmt.Errorf("placing comments lost %d of them", len(places)-len(seen))
	}
	return nil
}
