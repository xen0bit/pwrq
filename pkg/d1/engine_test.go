package d1

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/xen0bit/pwrq/pkg/d1/internal/testdata"
	"github.com/xen0bit/pwrq/pkg/d1/prompt"
	"github.com/xen0bit/pwrq/pkg/d1/vision"
)

// These run the real model, so they need PWRQ_TEST_D1_DIR (a folder with
// onnx/decoder_q4.onnx, onnx/vision_encoder_q8.onnx and their data,
// tokenizer.json, calibration.json, config.json) and PWRQ_TEST_ORT_LIB (an
// onnxruntime library). CI runs without them; the prompt and readout are
// checked there against the same reference in pkg/d1/prompt.
func open(t *testing.T) *Model {
	t.Helper()
	dir, lib := os.Getenv("PWRQ_TEST_D1_DIR"), os.Getenv("PWRQ_TEST_ORT_LIB")
	if dir == "" || lib == "" {
		t.Skip("set PWRQ_TEST_D1_DIR and PWRQ_TEST_ORT_LIB to run the model")
	}
	m, err := Open(context.Background(), Options{Device: CPU, ModelDir: dir, ORTLib: lib, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	return m
}

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
	Answers struct {
		Answers map[string]any `json:"answers"`
		Usage   struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage"`
	} `json:"answers"`
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

// TestMatchesReference answers every reference case and compares it with
// the answers d1's own Python code gave over the same ONNX graphs.
func TestMatchesReference(t *testing.T) {
	m := open(t)
	t.Logf("%+v", m.Info())
	var ref struct {
		Cases []refCase `json:"cases"`
	}
	testdata.Load(t, "reference.json", &ref)
	for _, c := range ref.Cases {
		t.Run(c.Name, func(t *testing.T) {
			state, err := prompt.DecodeOrdered(c.State)
			if err != nil {
				t.Fatal(err)
			}
			qraw, err := prompt.DecodeOrdered(c.Questions)
			if err != nil {
				t.Fatal(err)
			}
			questions, err := prompt.ParseQuestions(qraw)
			if err != nil {
				t.Fatal(err)
			}
			req := Request{State: state, Questions: questions}
			for _, md := range c.Media {
				if md.Type == "image" {
					req.Media = append(req.Media, Media{Picture: picture(t, md.Path)})
					continue
				}
				var frames []*vision.Picture
				for _, f := range md.Frames {
					frames = append(frames, picture(t, f))
				}
				req.Media = append(req.Media, Media{Frames: frames, FPS: md.FPS})
			}
			resp, err := m.Ask(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("%d tokens in %.2fs", resp.InputTokens, resp.Seconds)
			if resp.InputTokens != c.Answers.Usage.InputTokens {
				t.Errorf("input tokens %d, want %d", resp.InputTokens, c.Answers.Usage.InputTokens)
			}
			got := normalize(resp.Answers)
			for _, d := range diff("", got, c.Answers.Answers) {
				t.Error(d)
			}
		})
	}
}

// normalize round-trips a value through JSON, so ints and floats compare as
// the reference's decoded numbers do.
func normalize(v any) any {
	raw, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(raw, &out)
	return out
}

// diff lists where got and want differ: numbers by more than a tolerance
// that allows for ONNX Runtime 1.30 here and 1.32 in the reference, and for
// a box edge moving one thousandth.
func diff(at string, got, want any) []string {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return []string{fmt.Sprintf("%s: got %v, want an object", at, got)}
		}
		var keys []string
		for k := range w {
			keys = append(keys, k)
		}
		for k := range g {
			if _, ok := w[k]; !ok {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		var out []string
		for _, k := range keys {
			out = append(out, diff(at+"."+k, g[k], w[k])...)
		}
		return out
	case []any:
		g, ok := got.([]any)
		if !ok || len(g) != len(w) {
			return []string{fmt.Sprintf("%s: got %v, want %v", at, got, want)}
		}
		var out []string
		for i := range w {
			out = append(out, diff(fmt.Sprintf("%s[%d]", at, i), g[i], w[i])...)
		}
		return out
	case float64:
		g, ok := got.(float64)
		if !ok || math.Abs(g-w) > 3e-3 {
			return []string{fmt.Sprintf("%s: got %v, want %v", at, got, want)}
		}
		return nil
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		return []string{fmt.Sprintf("%s: got %v, want %v", at, got, want)}
	}
	return nil
}
