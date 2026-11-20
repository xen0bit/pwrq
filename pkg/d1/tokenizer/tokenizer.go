// Package tokenizer is d1's byte-level BPE (LFM3's tokenizer.json) in pure
// Go, reproducing Hugging Face tokenizers id for id and offset for offset.
//
// It grew from hotbutteredbeans' tokenizer, which shares the pre-tokenizer.
// d1 needs three things that one did not:
//
//   - ignore_merges: a pre-token that is itself in the vocabulary is one
//     token, whatever the merges would have made of it;
//   - offsets: d1 finds its answer slots by the character a marker's colon
//     ends at, and reads text answers as spans of the state, both through
//     the offset mapping;
//   - no implicit BOS: every d1 prompt writes <|startoftext|> itself, and
//     encodes with add_special_tokens=False.
//
// Offsets are in characters (Unicode code points), as Python's tokenizers
// reports them, so the prompt code ported from Python indexes the same way.
package tokenizer

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// Tokenizer is safe for concurrent use.
type Tokenizer struct {
	vocab        map[string]int32
	inverse      []string // id -> token text (byte-level characters, or an added token's content)
	special      map[int32]bool
	ranks        map[[2]string]int
	added        [2][]addedToken // [0]: matched on the raw text; [1]: then on what is left
	ignoreMerges bool
	byteChar     [256]string
	charByte     map[rune]byte
	cacheMu      sync.RWMutex
	cache        map[string][]int32
}

type addedToken struct {
	content string
	id      int32
	special bool
}

type tokenizerFile struct {
	AddedTokens []struct {
		ID         int32  `json:"id"`
		Content    string `json:"content"`
		Normalized bool   `json:"normalized"`
		Lstrip     bool   `json:"lstrip"`
		Rstrip     bool   `json:"rstrip"`
		SingleWord bool   `json:"single_word"`
		Special    bool   `json:"special"`
	} `json:"added_tokens"`
	Normalizer   json.RawMessage `json:"normalizer"`
	PreTokenizer json.RawMessage `json:"pre_tokenizer"`
	Model        struct {
		Type         string            `json:"type"`
		Vocab        map[string]int32  `json:"vocab"`
		Merges       []json.RawMessage `json:"merges"`
		ByteFallback bool              `json:"byte_fallback"`
		IgnoreMerges bool              `json:"ignore_merges"`
	} `json:"model"`
}

// The pre-tokenizer this implementation reproduces; any other is refused.
const supportedSplit = `(?i:'s|'t|'re|'ve|'m|'ll|'d)|[^\r\n\p{L}\p{N}]?\p{L}+|\p{N}{1,3}| ?[^\s\p{L}\p{N}]+[\r\n]*|\s*[\r\n]+|\s+(?!\S)|\s+`

// Parse reads a tokenizer.json. It refuses a file whose pipeline differs from
// the one implemented here, rather than tokenizing differently from the
// model's training.
func Parse(raw []byte) (*Tokenizer, error) {
	var f tokenizerFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	if f.Model.Type != "BPE" || f.Model.ByteFallback {
		return nil, fmt.Errorf("unsupported model (want byte-level BPE without byte fallback)")
	}
	if n := strings.TrimSpace(string(f.Normalizer)); n != "" && n != "null" {
		return nil, fmt.Errorf("unsupported normalizer %s", n)
	}
	var pre struct {
		Type          string `json:"type"`
		Pretokenizers []struct {
			Type    string `json:"type"`
			Pattern struct {
				Regex string `json:"Regex"`
			} `json:"pattern"`
			Behavior       string `json:"behavior"`
			Invert         bool   `json:"invert"`
			AddPrefixSpace bool   `json:"add_prefix_space"`
			UseRegex       bool   `json:"use_regex"`
		} `json:"pretokenizers"`
	}
	if err := json.Unmarshal(f.PreTokenizer, &pre); err != nil || len(pre.Pretokenizers) != 2 ||
		pre.Pretokenizers[0].Type != "Split" || pre.Pretokenizers[0].Pattern.Regex != supportedSplit ||
		pre.Pretokenizers[0].Behavior != "Isolated" || pre.Pretokenizers[0].Invert ||
		pre.Pretokenizers[1].Type != "ByteLevel" || pre.Pretokenizers[1].AddPrefixSpace || pre.Pretokenizers[1].UseRegex {
		return nil, fmt.Errorf("unsupported pre-tokenizer")
	}
	t := &Tokenizer{
		vocab:        f.Model.Vocab,
		special:      map[int32]bool{},
		ranks:        make(map[[2]string]int, len(f.Model.Merges)),
		ignoreMerges: f.Model.IgnoreMerges,
		cache:        map[string][]int32{},
		charByte:     map[rune]byte{},
	}
	for i, m := range f.Model.Merges {
		var pair []string
		if err := json.Unmarshal(m, &pair); err != nil {
			var s string
			if err := json.Unmarshal(m, &s); err != nil {
				return nil, fmt.Errorf("merge %d: %w", i, err)
			}
			pair = strings.SplitN(s, " ", 2)
		}
		if len(pair) != 2 {
			return nil, fmt.Errorf("merge %d is not a pair", i)
		}
		k := [2]string{pair[0], pair[1]}
		if _, dup := t.ranks[k]; !dup {
			t.ranks[k] = i
		}
	}
	maxID := int32(0)
	for _, id := range t.vocab {
		maxID = max(maxID, id)
	}
	for _, a := range f.AddedTokens {
		if a.Lstrip || a.Rstrip || a.SingleWord {
			return nil, fmt.Errorf("added token %q: lstrip/rstrip/single_word not supported", a.Content)
		}
		g := 0
		if a.Normalized {
			g = 1
		}
		t.added[g] = append(t.added[g], addedToken{a.Content, a.ID, a.Special})
		if a.Special {
			t.special[a.ID] = true
		}
		maxID = max(maxID, a.ID)
	}
	for g := range t.added { // longest first, so the first match at a position is the longest
		sort.SliceStable(t.added[g], func(i, j int) bool { return len(t.added[g][i].content) > len(t.added[g][j].content) })
	}
	t.inverse = make([]string, maxID+1)
	for s, id := range t.vocab {
		t.inverse[id] = s
	}
	for g := range t.added {
		for _, a := range t.added[g] {
			t.inverse[a.id] = a.content
		}
	}
	// GPT-2's bytes_to_unicode
	n := 0
	for b := 0; b < 256; b++ {
		var r rune
		if (b >= '!' && b <= '~') || (b >= 0xA1 && b <= 0xAC) || (b >= 0xAE && b <= 0xFF) {
			r = rune(b)
		} else {
			r = rune(256 + n)
			n++
		}
		t.byteChar[b] = string(r)
		t.charByte[r] = byte(b)
	}
	return t, nil
}

// VocabSize is one more than the largest id.
func (t *Tokenizer) VocabSize() int { return len(t.inverse) }

// ID returns the id of a token's exact text: an added token's content, or a
// vocabulary entry in byte-level characters.
func (t *Tokenizer) ID(token string) (int32, bool) {
	for g := range t.added {
		for _, a := range t.added[g] {
			if a.content == token {
				return a.id, true
			}
		}
	}
	id, ok := t.vocab[token]
	return id, ok
}

// Specials lists the special added tokens' contents, the ones d1's prompt
// must never let a caller's text produce.
func (t *Tokenizer) Specials() []string {
	var out []string
	for g := range t.added {
		for _, a := range t.added[g] {
			if a.special {
				out = append(out, a.content)
			}
		}
	}
	sort.Strings(out)
	return out
}

// Encode returns text's ids, with no special token added.
func (t *Tokenizer) Encode(text string) []int32 {
	ids, _ := t.encode(text, false)
	return ids
}

// Offset is a token's span in the text, in characters, end exclusive.
type Offset struct{ Start, End int }

// EncodeOffsets returns text's ids and each one's character span, as
// tokenizer(text, add_special_tokens=False, return_offsets_mapping=True).
func (t *Tokenizer) EncodeOffsets(text string) ([]int32, []Offset) {
	return t.encode(text, true)
}

func (t *Tokenizer) encode(text string, offsets bool) ([]int32, []Offset) {
	var ids []int32
	var byteOffs [][2]int
	for _, seg := range t.splitAdded(text) {
		if seg.id >= 0 {
			ids = append(ids, seg.id)
			if offsets {
				byteOffs = append(byteOffs, [2]int{seg.start, seg.start + len(seg.text)})
			}
			continue
		}
		at := seg.start
		for _, piece := range splitPieces(seg.text) {
			toks := t.bpe(piece)
			ids = append(ids, toks...)
			if offsets {
				// Each byte maps to one byte-level character, so a token's
				// length in characters is its length in bytes of the piece.
				p := at
				for _, id := range toks {
					n := utf8.RuneCountInString(t.inverse[id])
					byteOffs = append(byteOffs, [2]int{p, p + n})
					p += n
				}
			}
			at += len(piece)
		}
	}
	if !offsets {
		return ids, nil
	}
	return ids, charOffsets(text, byteOffs)
}

// charOffsets turns byte spans into character spans the way tokenizers
// aligns them: a token holding part of a multi-byte character covers that
// whole character.
func charOffsets(text string, spans [][2]int) []Offset {
	// charAt[b] is the index of the character byte b belongs to.
	charAt := make([]int, len(text)+1)
	c := 0
	for i := 0; i < len(text); {
		_, size := utf8.DecodeRuneInString(text[i:])
		for k := 0; k < size; k++ {
			charAt[i+k] = c
		}
		i += size
		c++
	}
	charAt[len(text)] = c
	out := make([]Offset, len(spans))
	for i, s := range spans {
		start := charAt[s[0]]
		end := c
		if s[1] < len(text) {
			end = charAt[s[1]]
			// s[1] inside a character: the token reaches that character's end.
			if s[1] > 0 && charAt[s[1]-1] == end {
				end++
			}
		}
		if s[1] == s[0] {
			end = start
		}
		out[i] = Offset{start, end}
	}
	return out
}

// Decode returns the text of ids, special tokens included, as
// tokenizer.decode(ids) does. Bytes that do not form UTF-8 are replaced
// with U+FFFD, as Python's decode with errors="replace" does.
func (t *Tokenizer) Decode(ids []int32) string {
	var b strings.Builder
	var pending []byte
	flush := func() {
		if len(pending) > 0 {
			b.WriteString(strings.ToValidUTF8(string(pending), "�"))
			pending = pending[:0]
		}
	}
	for _, id := range ids {
		if id < 0 || int(id) >= len(t.inverse) {
			continue
		}
		s := t.inverse[id]
		if t.isAdded(id) {
			flush()
			b.WriteString(s)
			continue
		}
		for _, r := range s {
			if by, ok := t.charByte[r]; ok {
				pending = append(pending, by)
			}
		}
	}
	flush()
	return b.String()
}

func (t *Tokenizer) isAdded(id int32) bool {
	if t.special[id] {
		return true
	}
	for _, a := range t.added[0] {
		if a.id == id {
			return true
		}
	}
	for _, a := range t.added[1] {
		if a.id == id {
			return true
		}
	}
	return false
}

type segment struct {
	text  string
	start int   // byte offset in the whole text
	id    int32 // -1: ordinary text
}

func (t *Tokenizer) splitAdded(text string) []segment {
	segs := []segment{{text, 0, -1}}
	for g := range t.added {
		if len(t.added[g]) == 0 {
			continue
		}
		var next []segment
		for _, s := range segs {
			if s.id >= 0 {
				next = append(next, s)
				continue
			}
			next = append(next, splitOn(s, t.added[g])...)
		}
		segs = next
	}
	return segs
}

// splitOn cuts a segment at the added tokens, leftmost first and the longest
// at a position.
func splitOn(s segment, toks []addedToken) []segment {
	var first [256]bool
	for _, a := range toks {
		first[a.content[0]] = true
	}
	text := s.text
	var out []segment
	last := 0
	for i := 0; i < len(text); i++ {
		if !first[text[i]] {
			continue
		}
		for _, a := range toks {
			if strings.HasPrefix(text[i:], a.content) {
				if i > last {
					out = append(out, segment{text[last:i], s.start + last, -1})
				}
				out = append(out, segment{a.content, s.start + i, a.id})
				i += len(a.content) - 1
				last = i + 1
				break
			}
		}
	}
	if last < len(text) {
		out = append(out, segment{text[last:], s.start + last, -1})
	}
	return out
}

func isL(r rune) bool { return unicode.IsLetter(r) }
func isN(r rune) bool { return unicode.IsNumber(r) }

// isS is Oniguruma's \s for UTF-8: Unicode's White_Space property.
func isS(r rune) bool { return unicode.IsSpace(r) }

// foldEq reports whether r matches the ASCII letter c case-insensitively, by
// Unicode simple case folding (so U+017F LONG S matches 's').
func foldEq(r rune, c rune) bool {
	if r == c {
		return true
	}
	for f := unicode.SimpleFold(c); f != c; f = unicode.SimpleFold(f) {
		if f == r {
			return true
		}
	}
	return false
}

var contractions = [][]rune{{'s'}, {'t'}, {'r', 'e'}, {'v', 'e'}, {'m'}, {'l', 'l'}, {'d'}}

// splitPieces applies the pre-tokenizer pattern: the alternatives tried in
// order at each position, as a backtracking engine does, the matches cutting
// the text into pieces. Go's regexp has no look-ahead for \s+(?!\S), so the
// pattern is matched by hand.
func splitPieces(s string) []string {
	rs := []rune(s)
	n := len(rs)
	var out []string
	off := make([]int, n+1)
	o := 0
	for i := range rs {
		off[i] = o
		_, sz := utf8.DecodeRuneInString(s[o:])
		o += sz
	}
	off[n] = len(s)
	at := func(i int) rune {
		if i < n {
			return rs[i]
		}
		return -1
	}
	i := 0
	for i < n {
		j := match(rs, i, n, at)
		out = append(out, s[off[i]:off[j]])
		i = j
	}
	return out
}

// match returns the end of the pattern's match at i (always > i: every
// character is matched by some alternative).
func match(rs []rune, i, n int, at func(int) rune) int {
	c := rs[i]
	// (?i:'s|'t|'re|'ve|'m|'ll|'d)
	if c == '\'' {
		for _, w := range contractions {
			k := 0
			for k < len(w) && i+1+k < n && foldEq(rs[i+1+k], w[k]) {
				k++
			}
			if k == len(w) {
				return i + 1 + k
			}
		}
	}
	// [^\r\n\p{L}\p{N}]?\p{L}+
	if c != '\r' && c != '\n' && !isL(c) && !isN(c) && isL(at(i+1)) {
		j := i + 2
		for j < n && isL(rs[j]) {
			j++
		}
		return j
	}
	if isL(c) {
		j := i + 1
		for j < n && isL(rs[j]) {
			j++
		}
		return j
	}
	// \p{N}{1,3}
	if isN(c) {
		j := i + 1
		for j < n && j < i+3 && isN(rs[j]) {
			j++
		}
		return j
	}
	// ' ?[^\s\p{L}\p{N}]+[\r\n]*'
	other := func(r rune) bool { return r >= 0 && !isS(r) && !isL(r) && !isN(r) }
	if other(c) || (c == ' ' && other(at(i+1))) {
		j := i
		if c == ' ' {
			j++
		}
		for j < n && other(rs[j]) {
			j++
		}
		for j < n && (rs[j] == '\r' || rs[j] == '\n') {
			j++
		}
		return j
	}
	if !isS(c) {
		return i + 1 // unreachable: a character is a letter, a number, whitespace or other
	}
	end := i
	for end < n && isS(rs[end]) {
		end++
	}
	// \s*[\r\n]+ : up to the run's last line break
	for k := end - 1; k >= i; k-- {
		if rs[k] == '\r' || rs[k] == '\n' {
			return k + 1
		}
	}
	// \s+(?!\S) : the run, less its last character when a non-space follows
	if end == n {
		return end
	}
	if end-1 > i {
		return end - 1
	}
	// \s+
	return end
}

const maxCache = 1 << 18

func (t *Tokenizer) bpe(piece string) []int32 {
	t.cacheMu.RLock()
	ids, ok := t.cache[piece]
	t.cacheMu.RUnlock()
	if ok {
		return ids
	}
	var b strings.Builder
	for i := 0; i < len(piece); i++ {
		b.WriteString(t.byteChar[piece[i]])
	}
	mapped := b.String()
	if id, ok := t.vocab[mapped]; ok && (t.ignoreMerges || utf8.RuneCountInString(mapped) == 1) {
		ids = []int32{id}
	} else {
		parts := make([]string, 0, len(mapped))
		for _, r := range mapped {
			parts = append(parts, string(r))
		}
		for len(parts) > 1 {
			best, at := -1, -1
			for k := 0; k+1 < len(parts); k++ {
				if r, ok := t.ranks[[2]string{parts[k], parts[k+1]}]; ok && (best < 0 || r < best) {
					best, at = r, k
				}
			}
			if at < 0 {
				break
			}
			parts[at] = parts[at] + parts[at+1]
			parts = append(parts[:at+1], parts[at+2:]...)
		}
		ids = make([]int32, 0, len(parts))
		for _, p := range parts {
			if id, ok := t.vocab[p]; ok {
				ids = append(ids, id)
			}
		}
	}
	t.cacheMu.Lock()
	if len(t.cache) >= maxCache {
		t.cache = map[string][]int32{}
	}
	t.cache[piece] = ids
	t.cacheMu.Unlock()
	return ids
}
