package llm

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/xen0bit/pwrq/pkg/d1"
	"github.com/xen0bit/pwrq/pkg/d1/prompt"
	"github.com/xen0bit/pwrq/pkg/d1/video"
	"github.com/xen0bit/pwrq/pkg/d1/vision"
)

// Reading a video in segments. A clip is read within one pixel budget
// (vision.VideoMaxPixels), so the longer it is the smaller its frames: an
// hour of 1080p is 768 frames of 128x64. {Segment: n} reads it n seconds at
// a time instead, each segment a clip of its own at the detail its length
// allows - two seconds of 1080p at full size - one model pass a segment,
// and combines the answers. Measured on three minutes of 1080p with "GATE
// 17" in 44-pixel text for four seconds: read whole, "is GATE 17 shown?"
// came back 0.71 and the gate number at 38%; in 2s segments 0.99 for the
// three that show it and 0.006 for the median one, the number read right;
// in 10s segments 0.98 for its one, in a fifth of the time.
//
// How answers combine, per type:
//
//   - noul: .noul is the largest of the segments' - "does the video show
//     X" is "does some segment" - and the most a calibrated P(somewhere)
//     can honestly be put at without knowing how segments' errors
//     correlate: at least the largest, at most their sum. Multiplying their
//     no's as if independent turns many small false alarms into a
//     confident yes. .all is the smallest ("always"), .mean the share of
//     the video it holds for.
//   - choice: the options' probabilities averaged over segments, weighted
//     by length - what the video mostly is - and the choice read from
//     them; with multiple, each option's largest, as noul.
//   - score: the levels' probabilities averaged, read out as one answer is.
//   - text, number, date and estimates read the state, not the picture: the
//     answer of the segment most sure of one.
//
// Every combined answer carries .segments: [{start, end, ...}], each
// segment's own answer, which is when in the video something happens.

// segmented is the one Media item a call reads in segments.
type segmented struct {
	at           int // its position in the media list
	starts, ends []float64
	load         func(k int) (d1.Media, error)
	close        func()
}

// windows cuts start..stop into seg-second pieces; a last piece under half
// a segment joins the one before rather than being read on its own.
func windows(start, stop, seg float64) (starts, ends []float64) {
	for s := start; s < stop-1e-6; s += seg {
		e := math.Min(s+seg, stop)
		if len(starts) > 0 && e-s < seg/2 {
			ends[len(ends)-1] = e
			break
		}
		starts = append(starts, s)
		ends = append(ends, e)
	}
	return starts, ends
}

// segmentFile reads a video file in segments, decoding each when it is
// asked about: two seconds of 1080p is four full-size frames, and an
// hour's worth held at once would not fit in memory. Its cache key is the
// file's identity (path, size, modification time) and the cut.
func segmentFile(path string, start, end, seg float64) (*segmented, string, error) {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return nil, "", err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, "", err
	}
	c, err := video.Open(abs)
	if err != nil {
		return nil, "", err
	}
	stop := c.Info().Duration
	if end > 0 && end < stop {
		stop = end
	}
	if start >= stop {
		_ = c.Close()
		return nil, "", fmt.Errorf("%s: Start %gs is past the end (%.1fs)", filepath.Base(path), start, stop)
	}
	starts, ends := windows(start, stop, seg)
	s := &segmented{
		starts: starts, ends: ends,
		close: func() { _ = c.Close() },
		load: func(k int) (d1.Media, error) {
			frames, err := c.Sample(starts[k], ends[k])
			if err != nil {
				return d1.Media{}, fmt.Errorf("%s, %gs to %gs: %w", filepath.Base(path), starts[k], ends[k], err)
			}
			m := d1.Media{}
			for _, f := range frames {
				m.Frames = append(m.Frames, f.Picture)
				m.Times = append(m.Times, f.Time)
			}
			return m, nil
		},
	}
	digest := fmt.Sprintf("video-segments:%s:%d:%d:%g:%g:%g", abs, st.Size(), st.ModTime().UnixNano(), start, end, seg)
	return s, digest, nil
}

// segmentFrames reads frames passed in, at fps, in segments.
func segmentFrames(frames []*vision.Picture, fps, seg float64) *segmented {
	dur := float64(len(frames)) / fps
	starts, ends := windows(0, dur, seg)
	return &segmented{
		starts: starts, ends: ends,
		close: func() {},
		load: func(k int) (d1.Media, error) {
			lo := int(math.Round(starts[k] * fps))
			hi := min(len(frames), int(math.Round(ends[k]*fps)))
			if hi-lo < 2 {
				return d1.Media{}, fmt.Errorf("the segment from %gs has %d frames; a video is read from 2", starts[k], hi-lo)
			}
			return d1.Media{Frames: frames[lo:hi], FPS: fps}, nil
		},
	}
}

// askSegments answers a call once a segment and combines the answers.
func askSegments(ctx context.Context, req *localRequest, ask func(media []d1.Media) (*d1.Response, error), readout string) (map[string]any, int, float64, error) {
	seg := req.seg
	var answers []map[string]any
	tokens, seconds := 0, 0.0
	for k := range seg.starts {
		if err := ctx.Err(); err != nil {
			return nil, 0, 0, err
		}
		m, err := seg.load(k)
		if err != nil {
			return nil, 0, 0, err
		}
		media := append([]d1.Media(nil), req.media...)
		media[seg.at] = m
		resp, err := ask(media)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("the segment from %gs to %gs: %w", seg.starts[k], seg.ends[k], err)
		}
		answers = append(answers, resp.Answers)
		tokens += resp.InputTokens
		seconds += resp.Seconds
	}
	return combineSegments(answers, seg.starts, seg.ends, readout), tokens, seconds, nil
}

// combineSegments combines each question's answers over the segments.
func combineSegments(answers []map[string]any, starts, ends []float64, readout string) map[string]any {
	out := map[string]any{}
	if len(answers) == 0 {
		return out
	}
	weight := make([]float64, len(starts))
	total := 0.0
	for k := range starts {
		weight[k] = ends[k] - starts[k]
		total += weight[k]
	}
	for k := range weight {
		weight[k] /= total
	}
	for id := range answers[0] {
		per := make([]map[string]any, len(answers))
		for k, a := range answers {
			per[k], _ = a[id].(map[string]any)
		}
		out[id] = combineOne(per, starts, ends, weight, readout)
	}
	return out
}

func combineOne(per []map[string]any, starts, ends, weight []float64, readout string) map[string]any {
	segments := make([]any, len(per))
	timeline := func(k int, fields ...string) map[string]any {
		s := map[string]any{"start": starts[k], "end": ends[k]}
		for _, f := range fields {
			s[f] = per[k][f]
		}
		return s
	}
	kind, _ := per[0]["type"].(string)
	switch {
	case kind == prompt.Noul:
		hi, lo, mean := 0.0, 1.0, 0.0
		for k, a := range per {
			p := num(a["noul"])
			hi, lo = math.Max(hi, p), math.Min(lo, p)
			mean += weight[k] * p
			segments[k] = timeline(k, "noul")
		}
		return map[string]any{"type": kind, "noul": hi, "all": lo, "mean": mean, "segments": segments}

	case kind == prompt.Choice && per[0]["choices"] != nil:
		probs := map[string]any{}
		var chosen []any
		for _, name := range probNames(per[0]["probabilities"]) {
			hi := 0.0
			for _, a := range per {
				ps, _ := a["probabilities"].(map[string]any)
				hi = math.Max(hi, num(ps[name]))
			}
			probs[name] = hi
			if hi >= 0.5 {
				chosen = append(chosen, name)
			}
		}
		for k := range per {
			segments[k] = timeline(k, "choices")
		}
		return map[string]any{"type": kind, "choices": nonNil(chosen), "probabilities": probs, "segments": segments}

	case kind == prompt.Choice:
		probs := map[string]any{}
		best, bestP := "", -1.0
		for _, name := range probNames(per[0]["probabilities"]) {
			mean := 0.0
			for k, a := range per {
				ps, _ := a["probabilities"].(map[string]any)
				mean += weight[k] * num(ps[name])
			}
			probs[name] = mean
			if mean > bestP {
				best, bestP = name, mean
			}
		}
		for k := range per {
			segments[k] = timeline(k, "choice", "confidence")
		}
		return map[string]any{"type": kind, "choice": best, "confidence": bestP, "probabilities": probs, "segments": segments}

	case kind == prompt.Score:
		levels := probNames(per[0]["probabilities"])
		sort.Slice(levels, func(i, j int) bool { return atoi(levels[i]) < atoi(levels[j]) })
		dist := make([]float64, len(levels))
		probs := map[string]any{}
		for i, l := range levels {
			for k, a := range per {
				ps, _ := a["probabilities"].(map[string]any)
				dist[i] += weight[k] * num(ps[l])
			}
			probs[l] = dist[i]
		}
		conf := 0.0
		for _, p := range dist {
			conf = math.Max(conf, p)
		}
		for k := range per {
			segments[k] = timeline(k, "score")
		}
		return map[string]any{"type": kind, "score": prompt.ScoreOf(dist, readout), "confidence": conf,
			"probabilities": probs, "legend": per[0]["legend"], "segments": segments}
	}

	// A read of the state: the surest segment's answer.
	best, bestP := 0, -1.0
	for k, a := range per {
		p := num(a["found"])
		if _, ok := a["found"]; !ok {
			p = num(a["confidence"])
		}
		if p > bestP {
			best, bestP = k, p
		}
		s := map[string]any{"start": starts[k], "end": ends[k]}
		for f, v := range a {
			if f != "type" {
				s[f] = v
			}
		}
		segments[k] = s
	}
	out := map[string]any{}
	for f, v := range per[best] {
		out[f] = v
	}
	out["segments"] = segments
	return out
}

func num(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	}
	return 0
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

func probNames(v any) []string {
	m, _ := v.(map[string]any)
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func nonNil(v []any) []any {
	if v == nil {
		return []any{}
	}
	return v
}
