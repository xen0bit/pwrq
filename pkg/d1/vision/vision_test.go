package vision_test

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xen0bit/pwrq/pkg/d1/internal/testdata"
	"github.com/xen0bit/pwrq/pkg/d1/vision"
)

// TestMatchesProcessor compares the Go preprocessing with d1's processor
// (internal/tools/d1fixtures/vision_fixtures.py) on pictures that reach the
// pixel cap, scale far up and down, have extreme aspects, and carry alpha.
func TestMatchesProcessor(t *testing.T) {
	var fx struct {
		Cases []struct {
			Name       string
			Grid       []int
			Tokens     int
			Sum, SumSq float64
			Head, Tail []float64
		}
	}
	testdata.Load(t, "vision_fixtures.json", &fx)
	for _, c := range fx.Cases {
		raw, err := os.ReadFile(filepath.Join(testdata.Dir(), c.Name))
		if err != nil {
			t.Fatal(err)
		}
		p, err := vision.Decode(raw)
		if err != nil {
			t.Fatal(err)
		}
		m, err := vision.Image(p, vision.DefaultMaxPixels, "<|image|>")
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		in := m.Inputs[0]
		if in.GH*2 != c.Grid[1] || in.GW*2 != c.Grid[2] {
			t.Errorf("%s: grid %dx%d, want %v", c.Name, in.GH*2, in.GW*2, c.Grid)
			continue
		}
		if got := strings.Count(m.Markup, "<|image|>"); got != c.Tokens || got != in.Tokens() {
			t.Errorf("%s: %d image tokens, want %d", c.Name, got, c.Tokens)
		}
		var sum, sumsq float64
		for _, v := range in.Pixels {
			sum += float64(v)
			sumsq += float64(v) * float64(v)
		}
		worst := 0.0
		for i, h := range c.Head {
			worst = math.Max(worst, math.Abs(float64(in.Pixels[i])-h))
		}
		for i, h := range c.Tail {
			worst = math.Max(worst, math.Abs(float64(in.Pixels[len(in.Pixels)-len(c.Tail)+i])-h))
		}
		if math.Abs(sum-c.Sum) > 1e-3 || math.Abs(sumsq-c.SumSq) > 1e-3 || worst != 0 {
			t.Errorf("%s: sum %.6f (want %.6f), sumsq %.6f (want %.6f), ends off by %g", c.Name, sum, c.Sum, sumsq, c.SumSq, worst)
		}
	}
}

func TestSmartResize(t *testing.T) {
	for _, c := range []struct{ h, w, wantH, wantW int }{
		{240, 320, 256, 320},
		{61, 97, 224, 352}, // the reference grid: 14x22 patches
		{1024, 1024, 1024, 1024},
	} {
		h, w, err := vision.SmartResize(c.h, c.w, 65536, 16777216)
		if err != nil || h != c.wantH || w != c.wantW {
			t.Errorf("SmartResize(%d, %d) = %d, %d, %v; want %d, %d", c.h, c.w, h, w, err, c.wantH, c.wantW)
		}
	}
	if _, _, err := vision.SmartResize(1, 300, 65536, 16777216); err == nil {
		t.Error("an aspect past 200 was accepted")
	}
}

func TestSampleFrames(t *testing.T) {
	// 8 frames at 4 fps is two seconds: four frames at the processor's 2 fps
	// (its minimum), spread over the clip with banker's rounding.
	got := vision.SampleFrames(8, 4)
	want := []int{0, 2, 5, 7}
	if len(got) != len(want) {
		t.Fatalf("%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%v, want %v", got, want)
		}
	}
	if n := len(vision.SampleFrames(3, 30)); n != 3 {
		t.Errorf("a 3-frame clip sampled %d frames", n)
	}
}

func TestVideoRefusesMixedSizes(t *testing.T) {
	a := &vision.Picture{W: 64, H: 64, Pix: make([]uint8, 64*64*3)}
	b := &vision.Picture{W: 32, H: 64, Pix: make([]uint8, 32*64*3)}
	if _, err := vision.Video([]*vision.Picture{a, b}, 2, "<|video|>"); err == nil {
		t.Error("frames of two sizes were accepted")
	}
}
