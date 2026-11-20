package prompt_test

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/xen0bit/pwrq/pkg/d1/internal/testdata"
	"github.com/xen0bit/pwrq/pkg/d1/prompt"
	"github.com/xen0bit/pwrq/pkg/d1/tokenizer"
	"github.com/xen0bit/pwrq/pkg/d1/vision"
)

// reference.json is written by internal/tools/d1fixtures/reference.py from
// d1-2B's own Python code; see that file for what each case covers.
type refCase struct {
	Name      string          `json:"name"`
	State     json.RawMessage `json:"state"`
	Questions json.RawMessage `json:"questions"`
	Media     []struct {
		Type   string   `json:"type"`
		Path   string   `json:"path"`
		Frames []string `json:"frames"`
		FPS    float64  `json:"fps"`
	} `json:"media"`
	Prompts  []string `json:"prompts"`
	Forwards []struct {
		InputIDs      [][]int32 `json:"input_ids"`
		QuestionIDs   [][]int64 `json:"question_ids"`
		SlotPositions []int     `json:"slot_positions"`
		ImageCalls    []stats   `json:"image_calls"`
		VideoCalls    []stats   `json:"video_calls"`
	} `json:"forwards"`
	Answers json.RawMessage `json:"answers"`
}

type stats struct {
	Shape []int     `json:"shape"`
	Sum   float64   `json:"sum"`
	SumSq float64   `json:"sumsq"`
	Head  []float64 `json:"head"`
}

func loadCases(t *testing.T) []refCase {
	t.Helper()
	var ref struct {
		Cases []refCase `json:"cases"`
	}
	testdata.Load(t, "reference.json", &ref)
	if len(ref.Cases) < 10 {
		t.Fatalf("only %d reference cases", len(ref.Cases))
	}
	return ref.Cases
}

func builder(t *testing.T) *prompt.Builder {
	t.Helper()
	tok, err := tokenizer.Parse(testdata.TokenizerJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	return prompt.NewBuilder(tok, prompt.Default)
}

func picture(t *testing.T, name string) *vision.Picture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(testdata.Dir(), name))
	if err != nil {
		t.Fatal(err)
	}
	p, err := vision.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// media prepares a case's pictures and videos as the engine does.
func media(t *testing.T, c refCase) (string, []vision.Media) {
	t.Helper()
	var markup strings.Builder
	var out []vision.Media
	for _, m := range c.Media {
		var vm vision.Media
		var err error
		if m.Type == "image" {
			vm, err = vision.Image(picture(t, m.Path), vision.DefaultMaxPixels, "<|image|>")
		} else {
			var frames []*vision.Picture
			for _, f := range m.Frames {
				frames = append(frames, picture(t, f))
			}
			vm, err = vision.Video(frames, m.FPS, "<|video|>")
		}
		if err != nil {
			t.Fatal(err)
		}
		markup.WriteString(vm.Markup)
		out = append(out, vm)
	}
	return markup.String(), out
}

func decodeState(t *testing.T, raw json.RawMessage) any {
	t.Helper()
	v, err := prompt.DecodeOrdered(raw)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// TestPixelsMatchReference compares every vision encoder input the Go
// preprocessing builds with the processor's.
func TestPixelsMatchReference(t *testing.T) {
	for _, c := range loadCases(t) {
		if len(c.Media) == 0 {
			continue
		}
		_, ms := media(t, c)
		var want []stats
		for _, f := range c.Forwards[:1] {
			want = append(want, f.ImageCalls...)
			want = append(want, f.VideoCalls...)
		}
		var got []vision.Input
		for _, m := range ms {
			if !m.Video {
				got = append(got, m.Inputs...)
			}
		}
		for _, m := range ms {
			if m.Video {
				got = append(got, m.Inputs...)
			}
		}
		if len(got) != len(want) {
			t.Fatalf("%s: %d encoder calls, want %d", c.Name, len(got), len(want))
		}
		for i, in := range got {
			w := want[i]
			shape := []int{in.GH, in.GW, 2, 2, vision.PatchDim}
			if !slices.Equal(shape, w.Shape) {
				t.Errorf("%s call %d: shape %v, want %v", c.Name, i, shape, w.Shape)
				continue
			}
			var sum, sumsq, worst float64
			for _, v := range in.Pixels {
				sum += float64(v)
				sumsq += float64(v) * float64(v)
			}
			for k, h := range w.Head {
				worst = math.Max(worst, math.Abs(float64(in.Pixels[k])-h))
			}
			t.Logf("%s call %d: %v sum %.6f/%.6f worst %g", c.Name, i, shape, sum, w.Sum, worst)
			// The same float32 values: only summation order may differ. A
			// formula one ulp off (x/255-0.5)/0.5 against the processor's
			// (x-127.5)/127.5) moves the sum by about 0.01 - and the
			// decoder's answers by a percent - so this is near exact.
			if math.Abs(sum-w.Sum) > 1e-4 || math.Abs(sumsq-w.SumSq) > 1e-4 || worst != 0 {
				t.Errorf("%s call %d: sum %.6f (want %.6f), sumsq %.6f (want %.6f), head off by %g",
					c.Name, i, sum, w.Sum, sumsq, w.SumSq, worst)
			}
		}
	}
}

// TestRowsMatchReference checks every row the Go prompt builder packs -
// token ids, each token's question, and the answer slots - against the rows
// the reference fed the ONNX decoder.
func TestRowsMatchReference(t *testing.T) {
	b := builder(t)
	const pad = 124893
	for _, c := range loadCases(t) {
		markup, _ := media(t, c)
		questions, err := prompt.ParseQuestions(decodeState(t, c.Questions))
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		state := decodeState(t, c.State)
		rows, err := b.Rows(state, questions, markup, 0)
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		type refRow struct {
			ids   []int32
			qids  []int64
			slots []int64
		}
		var want []refRow
		for _, f := range c.Forwards {
			width := len(f.InputIDs[0])
			perRow := make([][]int64, len(f.InputIDs))
			for _, flat := range f.SlotPositions {
				perRow[flat/width] = append(perRow[flat/width], int64(flat%width))
			}
			for r, ids := range f.InputIDs {
				n := len(ids)
				for n > 0 && ids[n-1] == pad {
					n--
				}
				want = append(want, refRow{ids[:n], f.QuestionIDs[r][:n], perRow[r]})
			}
		}
		if len(rows) != len(want) {
			t.Errorf("%s: %d rows, want %d", c.Name, len(rows), len(want))
			continue
		}
		for _, row := range rows {
			found := false
			for _, w := range want {
				if slices.Equal(row.IDs, w.ids) {
					t.Logf("%s: row of %d tokens, %d slots matched", c.Name, len(row.IDs), len(row.Slots))
					found = true
					if !slices.Equal(row.QuestionIDs, w.qids) {
						t.Errorf("%s: question ids differ", c.Name)
					}
					if !slices.Equal(row.SlotPositions(), w.slots) {
						t.Errorf("%s: slots %v, want %v", c.Name, row.SlotPositions(), w.slots)
					}
				}
			}
			if !found {
				got := b.Tokenizer().Decode(row.IDs)
				t.Errorf("%s: no reference row matches\n got: %q", c.Name, trunc(got))
				for i, p := range c.Prompts {
					t.Logf("want %d: %q", i, trunc(p))
				}
			}
		}
	}
}

func trunc(s string) string {
	s = strings.ReplaceAll(s, "<|image|>", "")
	s = strings.ReplaceAll(s, "<|video|>", "")
	s = strings.ReplaceAll(s, "<|pad|>", "")
	if len(s) > 3000 {
		return s[:3000]
	}
	return s
}
