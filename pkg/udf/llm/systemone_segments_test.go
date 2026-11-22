package llm

import (
	"math"
	"path/filepath"
	"reflect"
	"testing"
)

func TestWindows(t *testing.T) {
	for _, tc := range []struct {
		start, stop, seg float64
		starts, ends     []float64
	}{
		{0, 10, 2, []float64{0, 2, 4, 6, 8}, []float64{2, 4, 6, 8, 10}},
		{0, 10.5, 2, []float64{0, 2, 4, 6, 8}, []float64{2, 4, 6, 8, 10.5}}, // a short tail joins the last
		{0, 11.5, 2, []float64{0, 2, 4, 6, 8, 10}, []float64{2, 4, 6, 8, 10, 11.5}},
		{5, 6, 10, []float64{5}, []float64{6}}, // shorter than one segment: read whole
	} {
		s, e := windows(tc.start, tc.stop, tc.seg)
		if !reflect.DeepEqual(s, tc.starts) || !reflect.DeepEqual(e, tc.ends) {
			t.Errorf("windows(%g, %g, %g) = %v %v, want %v %v", tc.start, tc.stop, tc.seg, s, e, tc.starts, tc.ends)
		}
	}
}

// TestCombineSegments: noul is the largest (somewhere), all the smallest,
// mean the share of time; a choice averages its options by length; a
// score reads the averaged levels; every answer keeps its timeline.
func TestCombineSegments(t *testing.T) {
	starts, ends := []float64{0, 10, 20}, []float64{10, 20, 40}
	answers := []map[string]any{
		{
			"seen": map[string]any{"type": "noul", "noul": 0.1},
			"what": map[string]any{"type": "choice", "choice": "a", "confidence": 0.8, "probabilities": map[string]any{"a": 0.8, "b": 0.2}},
			"many": map[string]any{"type": "choice", "choices": []any{}, "probabilities": map[string]any{"x": 0.1, "y": 0.3}},
			"lvl":  map[string]any{"type": "score", "score": 0.0, "probabilities": map[string]any{"0": 0.9, "1": 0.1, "2": 0.0}, "legend": map[string]any{}},
		},
		{
			"seen": map[string]any{"type": "noul", "noul": 0.9},
			"what": map[string]any{"type": "choice", "choice": "b", "confidence": 0.6, "probabilities": map[string]any{"a": 0.4, "b": 0.6}},
			"many": map[string]any{"type": "choice", "choices": []any{"x"}, "probabilities": map[string]any{"x": 0.7, "y": 0.2}},
			"lvl":  map[string]any{"type": "score", "score": 2.0, "probabilities": map[string]any{"0": 0.0, "1": 0.2, "2": 0.8}, "legend": map[string]any{}},
		},
		{
			"seen": map[string]any{"type": "noul", "noul": 0.2},
			"what": map[string]any{"type": "choice", "choice": "a", "confidence": 0.7, "probabilities": map[string]any{"a": 0.7, "b": 0.3}},
			"many": map[string]any{"type": "choice", "choices": []any{}, "probabilities": map[string]any{"x": 0.2, "y": 0.1}},
			"lvl":  map[string]any{"type": "score", "score": 0.0, "probabilities": map[string]any{"0": 0.6, "1": 0.4, "2": 0.0}, "legend": map[string]any{}},
		},
	}
	got := combineSegments(answers, starts, ends, "")
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

	seen := got["seen"].(map[string]any)
	if !near(seen["noul"].(float64), 0.9) || !near(seen["all"].(float64), 0.1) || !near(seen["mean"].(float64), 0.25*0.1+0.25*0.9+0.5*0.2) {
		t.Errorf("noul combined to %v", seen)
	}
	if segs := seen["segments"].([]any); len(segs) != 3 || segs[1].(map[string]any)["start"] != 10.0 || segs[1].(map[string]any)["noul"] != 0.9 {
		t.Errorf("noul timeline %v", seen["segments"])
	}

	what := got["what"].(map[string]any)
	pa := 0.25*0.8 + 0.25*0.4 + 0.5*0.7
	if what["choice"] != "a" || !near(what["confidence"].(float64), pa) {
		t.Errorf("choice combined to %v, want a at %v", what, pa)
	}

	many := got["many"].(map[string]any)
	if !reflect.DeepEqual(many["choices"], []any{"x"}) || !near(many["probabilities"].(map[string]any)["x"].(float64), 0.7) {
		t.Errorf("multiple choice combined to %v", many)
	}

	// Levels averaged: 0: .225+0+.3 = .525, so the median level is 0.
	if lvl := got["lvl"].(map[string]any); lvl["score"] != 0.0 {
		t.Errorf("score combined to %v, want the averaged distribution's median, 0", lvl)
	}
}

func TestMediaReadsAVideoInSegments(t *testing.T) {
	clip := filepath.Join("..", "..", "d1", "video", "testdata", "h264.mp4")
	if _, _, _, err := parseMedia("invoke_systemone", []any{"x.png", map[string]any{"Type": "video", "Path": clip, "Segment": 1}}); err == nil {
		t.Fatal("a missing picture should fail")
	}
	media, digest, seg, err := parseMedia("invoke_systemone", []any{map[string]any{"Type": "video", "Path": clip, "Segment": 1}})
	if err != nil {
		t.Fatal(err)
	}
	defer seg.close()
	if len(media) != 1 || seg.at != 0 || len(seg.starts) != 3 || len(digest) != 1 {
		t.Fatalf("3s in 1s segments: %d media, at %d, %d segments", len(media), seg.at, len(seg.starts))
	}
	for k := range seg.starts {
		m, err := seg.load(k)
		if err != nil {
			t.Fatalf("segment %d: %v", k, err)
		}
		if len(m.Frames) != 4 || m.Times[0] != 0 {
			t.Errorf("segment %d: %d frames from %v, want 4 (the minimum) from 0", k, len(m.Frames), m.Times[0])
		}
	}
	for _, bad := range []any{0.5, "ten"} {
		if _, _, _, err := parseMedia("invoke_systemone", []any{map[string]any{"Type": "video", "Path": clip, "Segment": bad}}); err == nil {
			t.Errorf("Segment %v should be refused", bad)
		}
	}
	two := map[string]any{"Type": "video", "Path": clip, "Segment": 1}
	if _, _, _, err := parseMedia("invoke_systemone", []any{two, two}); err == nil {
		t.Error("two segmented videos in one call should be refused")
	}
}
