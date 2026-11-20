package tokenizer

import (
	"slices"
	"testing"

	"github.com/xen0bit/pwrq/pkg/d1/internal/testdata"
)

func load(t *testing.T) *Tokenizer {
	t.Helper()
	tok, err := Parse(testdata.TokenizerJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// TestMatchesHuggingFace checks ids, character offsets and decoding against
// tokenizers itself (internal/tools/d1fixtures/tokenizer_fixtures.py).
func TestMatchesHuggingFace(t *testing.T) {
	tok := load(t)
	var fx struct {
		Cases []struct {
			Text    string   `json:"text"`
			IDs     []int32  `json:"ids"`
			Offsets [][2]int `json:"offsets"`
			Decoded string   `json:"decoded"`
		} `json:"cases"`
	}
	testdata.Load(t, "tokenizer_fixtures.json", &fx)
	if len(fx.Cases) < 20 {
		t.Fatalf("only %d cases", len(fx.Cases))
	}
	for _, c := range fx.Cases {
		ids, offs := tok.EncodeOffsets(c.Text)
		if !slices.Equal(ids, c.IDs) {
			t.Errorf("%q:\n got %v\nwant %v", c.Text, ids, c.IDs)
			continue
		}
		if plain := tok.Encode(c.Text); !slices.Equal(plain, ids) {
			t.Errorf("%q: Encode and EncodeOffsets disagree", c.Text)
		}
		for i, o := range offs {
			if o.Start != c.Offsets[i][0] || o.End != c.Offsets[i][1] {
				t.Errorf("%q: token %d offset %v, want %v", c.Text, i, o, c.Offsets[i])
				break
			}
		}
		if got := tok.Decode(ids); got != c.Decoded {
			t.Errorf("%q: decoded %q, want %q", c.Text, got, c.Decoded)
		}
	}
}

func TestSpecialTokens(t *testing.T) {
	tok := load(t)
	for name, want := range map[string]int32{
		"<|startoftext|>": 124894, "<|im_end|>": 124900, "<|pad|>": 124893,
		"<|image|>": 125030, "<|video|>": 125029, "<|vision_start|>": 125009, "<|vision_end|>": 125010,
	} {
		if id, ok := tok.ID(name); !ok || id != want {
			t.Errorf("%s = %d, %v; want %d", name, id, ok, want)
		}
	}
	specials := tok.Specials()
	if !slices.Contains(specials, "<|im_end|>") || slices.Contains(specials, "<think>") {
		t.Errorf("specials %v: want <|im_end|> and not the non-special <think>", specials)
	}
	if tok.VocabSize() != 125031 {
		t.Errorf("vocab size %d", tok.VocabSize())
	}
}

// d1 reads numbers 0-999 as single tokens (d1_numbers; the spaced form " 7"
// is two, digits never taking a leading space, and the readout skips it): a box edge is one of
// a thousand of them, so a tokenizer that split them would silently read
// boxes on the wrong grid.
func TestNumbersAreSingleTokens(t *testing.T) {
	tok := load(t)
	for n := range 1000 {
		s := itoa(n)
		if ids := tok.Encode(s); len(ids) != 1 {
			t.Fatalf("%q is %d tokens", s, len(ids))
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

func TestRefusesOtherPipelines(t *testing.T) {
	if _, err := Parse([]byte(`{"model":{"type":"WordPiece"}}`)); err == nil {
		t.Fatal("parsed a WordPiece tokenizer")
	}
}
