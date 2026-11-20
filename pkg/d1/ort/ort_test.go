package ort

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// These run the real d1 vision encoder, so they need PWRQ_TEST_ORT_LIB (an
// onnxruntime library) and PWRQ_TEST_D1_DIR (a folder holding
// onnx/vision_encoder_q8.onnx and its .onnx_data). Without them they skip,
// which is how CI runs them: the model is two gigabytes.

func setup(t *testing.T) (*Library, string) {
	t.Helper()
	lib, dir := os.Getenv("PWRQ_TEST_ORT_LIB"), os.Getenv("PWRQ_TEST_D1_DIR")
	if lib == "" || dir == "" {
		t.Skip("set PWRQ_TEST_ORT_LIB and PWRQ_TEST_D1_DIR")
	}
	l, err := Load(lib)
	if err != nil {
		t.Fatal(err)
	}
	return l, dir
}

// pixels is a deterministic stand-in for a preprocessed image of gh x gw
// blocks of 2x2 patches, in the encoder's [gh, gw, 2, 2, 1536] layout.
func pixels(gh, gw int64) Tensor {
	n := gh * gw * 2 * 2 * 1536
	data := make([]float32, n)
	for i := range data {
		data[i] = float32(math.Sin(float64(i)*0.001)) * 0.5
	}
	return F32Tensor(data, gh, gw, 2, 2, 1536)
}

func TestVisionEncoderRuns(t *testing.T) {
	l, dir := setup(t)
	t.Logf("onnxruntime %s, providers %v", l.Version, must(l.Providers()))
	s, err := l.NewSession(ModelSource{Path: filepath.Join(dir, "onnx", "vision_encoder_q8.onnx")}, SessionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	start := time.Now()
	out, err := s.Run(context.Background(), map[string]Tensor{"pixel_values": pixels(3, 4)}, []string{"image_features"})
	if err != nil {
		t.Fatal(err)
	}
	f := out["image_features"]
	t.Logf("image_features %v in %v", f.Shape, time.Since(start))
	// Each 2x2 block of patches merges into one token of the decoder's width.
	if len(f.Shape) != 2 || f.Shape[0] != 12 || f.Shape[1] != 2048 {
		t.Fatalf("shape %v, want [12 2048]", f.Shape)
	}
	for i, v := range f.F32 {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			t.Fatalf("feature %d is %v", i, v)
		}
	}

	// The same input twice gives the same features: the CPU path is
	// deterministic, which is what makes the response cache honest.
	again, err := s.Run(context.Background(), map[string]Tensor{"pixel_values": pixels(3, 4)}, []string{"image_features"})
	if err != nil {
		t.Fatal(err)
	}
	for i := range f.F32 {
		if f.F32[i] != again["image_features"].F32[i] {
			t.Fatalf("feature %d differs between runs", i)
		}
	}
}

func TestCancelledRunStops(t *testing.T) {
	l, dir := setup(t)
	s, err := l.NewSession(ModelSource{Path: filepath.Join(dir, "onnx", "vision_encoder_q8.onnx")}, SessionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Run(ctx, map[string]Tensor{"pixel_values": pixels(2, 2)}, []string{"image_features"}); err != context.Canceled {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}

func TestShapeMismatchPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("no panic for 3 values in shape [2 2]")
		}
	}()
	F32Tensor([]float32{1, 2, 3}, 2, 2)
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
