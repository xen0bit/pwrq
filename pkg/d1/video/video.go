// Package video reads video files for d1 in pure Go: MP4 and QuickTime
// (.mp4, .m4v, .mov) and Matroska and WebM (.mkv, .webm) holding H.264,
// HEVC, VP8, VP9 or AV1 - what phones, cameras, screen recorders, browsers
// and video sites write. It decodes only the frames d1 reads - two a
// second, at least four and at most 768, as the checkpoint's video
// processor samples - and resizes each to the processor's size as it comes
// out, so a long recording costs neither hours of decoding nor gigabytes of
// full-size frames.
//
// The codecs are go.264 (BSD-2), gen2brain/h265 (MIT), govpx (BSD-3, VP8
// and VP9 profile 0) and gen2brain/gav1d (BSD-2, a port of dav1d), all
// written in Go and checked bit-exact against ffmpeg on this package's
// fixtures; MP4 is read with mp4ff and Matroska here. No cgo and no
// ffmpeg, so every release architecture can read video. Anything else -
// AVI, MPEG-TS, ProRes, 10-bit VP9 - is refused with a pointer to passing
// its frames instead.
package video

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/xen0bit/pwrq/pkg/d1/vision"
)

// Info describes a video file's picture track.
type Info struct {
	Codec    string  // "h264", "hevc", "vp8", "vp9" or "av1"
	Frames   int     // frames in the track
	FPS      float64 // their average rate
	Duration float64 // seconds
	Rotation int     // degrees clockwise the picture is turned when shown
}

// Clip is an open video file.
type Clip struct {
	f     *os.File
	t     *track
	order []int     // presentation order -> decoding index
	times []float64 // presentation order -> seconds from the first frame
	info  Info
}

// Open reads a video file's sample table.
func Open(path string) (*Clip, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	t, err := readAny(f)
	if err == nil {
		err = markHidden(f, t)
	}
	if err == nil && len(t.order()) == 0 {
		err = errors.New("the video track shows no frames")
	}
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	c := &Clip{f: f, t: t, order: t.order()}
	first := t.samples[c.order[0]].pts
	c.times = make([]float64, len(c.order))
	for k, i := range c.order {
		c.times[k] = float64(t.samples[i].pts-first) / t.timescale
	}
	n := len(c.order)
	// The duration runs to the end of the last frame: one average frame
	// past its start.
	dur := c.times[n-1]
	if n > 1 {
		dur += dur / float64(n-1)
	}
	fps := 0.0
	if dur > 0 {
		fps = float64(n) / dur
	}
	c.info = Info{Codec: t.codec, Frames: n, FPS: fps, Duration: dur, Rotation: t.rotation}
	return c, nil
}

// readAny reads the track of an MP4/QuickTime or a Matroska/WebM file,
// told apart by their first bytes.
func readAny(f *os.File) (*track, error) {
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	var head [4]byte
	if _, err := f.ReadAt(head[:], 0); err != nil {
		return nil, fmt.Errorf("not a video file: %w", err)
	}
	if isMatroska(head[:]) {
		return readMatroska(f, st.Size())
	}
	return readTrack(f)
}

// Info describes the clip.
func (c *Clip) Info() Info { return c.info }

// Close closes the file.
func (c *Clip) Close() error { return c.f.Close() }

// Frame is one decoded frame and when it is shown.
type Frame struct {
	Picture *vision.Picture
	Time    float64 // seconds from the clip's first frame
}

// Sample reads a clip the way d1 reads video: frames between start and end
// seconds (end 0 is the end of the clip), at two a second - at least four,
// at most 768 - each resized by vision.VideoSize. Times are seconds from
// start.
func (c *Clip) Sample(start, end float64) ([]Frame, error) {
	lo := sort.SearchFloat64s(c.times, start-1e-9)
	hi := len(c.times)
	if end > 0 {
		hi = sort.SearchFloat64s(c.times, end-1e-9)
	}
	if hi-lo < 2 {
		return nil, fmt.Errorf("the clip has %d frames between %gs and %gs; d1 reads 2 or more (it is %.1fs long)", max(hi-lo, 0), start, end, c.info.Duration)
	}
	fps := c.info.FPS
	if fps <= 0 {
		fps = 2 // one-frame-a-timestamp clips: sample them all
	}
	picked := vision.SampleFrames(hi-lo, fps)
	idx := make([]int, len(picked))
	for i, k := range picked {
		idx[i] = lo + k
	}
	n := len(idx)
	frames, err := c.Frames(idx, func(w, h int) (int, int) {
		th, tw, err := vision.VideoSize(n, h, w)
		if err != nil {
			return w, h
		}
		return tw, th
	})
	if err != nil {
		return nil, err
	}
	t0 := c.times[lo]
	for i := range frames {
		frames[i].Time -= t0
	}
	return frames, nil
}

// Frames decodes the frames at the given presentation-order indices, in
// that order, each turned upright and resized to size(width, height) of
// the upright picture.
//
// Each group of pictures holding a wanted frame is decoded from its sync
// sample as far as its last wanted frame, the groups in parallel; see
// decodeAt.
func (c *Clip) Frames(idx []int, size func(w, h int) (int, int)) ([]Frame, error) {
	out := make([]Frame, len(idx))
	err := c.decodeAt(idx, func(at, i int, p planes) {
		pic := upright(toRGB(p), c.t.rotation)
		w, h := size(pic.W, pic.H)
		out[at] = Frame{Picture: pic.ResizeTorch(w, h), Time: c.timeOf(i)}
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// decodeAt decodes the frames at presentation-order indices idx, handing
// each to use with its position in idx and its decoding index, while its
// planes are valid. use is called from several goroutines at once, never
// twice for one position.
//
// The wanted frames are grouped by the sync sample before them, and each
// group is a run of its own: decoded from that sync sample only as far as
// its last wanted frame, on a decoder of its own, the runs in parallel. A
// group's run starts a group earlier when one of its wanted frames is a
// leading picture - shown before the sync sample, decoded after it,
// predicted from the group before - and the leading pictures of the sample
// a run starts at are not fed, since they would decode from references the
// run never had. The decoders return pictures in presentation order, so
// each picture out is the earliest-shown sample fed and not yet out.
func (c *Clip) decodeAt(idx []int, use func(at, i int, p planes)) error {
	t := c.t
	want := map[int]int{} // decoding index -> position in idx
	var targets []int
	for pos, k := range idx {
		if k < 0 || k >= len(c.order) {
			return fmt.Errorf("frame %d is outside the clip's %d", k, len(c.order))
		}
		i := c.order[k]
		if _, dup := want[i]; !dup {
			targets = append(targets, i)
		}
		want[i] = pos
	}
	sort.Ints(targets)

	// Where decoding a wanted frame starts: its group's sync sample, or the
	// group before for a leading picture.
	need := func(w int) int {
		s0 := t.syncBefore(w)
		if t.samples[w].pts < t.samples[s0].pts && s0 > 0 {
			s0 = t.syncBefore(s0 - 1)
		}
		return s0
	}
	type run struct{ start, last int }
	var runs []run
	for k := 0; k < len(targets); {
		g := t.syncBefore(targets[k])
		r := run{start: need(targets[k]), last: targets[k]}
		for k++; k < len(targets) && t.syncBefore(targets[k]) == g; k++ {
			r.start = min(r.start, need(targets[k]))
			r.last = targets[k]
		}
		runs = append(runs, r)
	}

	got := make([]bool, len(idx))
	decode := func(r run, alt bool) (err error) {
		// A decoder written for well-formed streams may panic on a damaged
		// one; that is this file's error, not the process's end.
		defer func() {
			if p := recover(); p != nil {
				err = fmt.Errorf("the decoder failed: %v", p)
			}
		}()
		dec, err := newDecoder(t, alt)
		if err != nil {
			return err
		}
		var pending []int // fed and not yet out, by presentation time
		take := func(pics []planes) {
			for _, p := range pics {
				if len(pending) > 0 {
					i := pending[0]
					pending = pending[1:]
					if at, ok := want[i]; ok && i <= r.last && i >= r.start {
						use(at, i, p)
					}
				}
				if p.release != nil {
					p.release()
				}
			}
		}
		var buf []byte
		for pos := r.start; pos <= r.last; pos++ {
			s := t.samples[pos]
			if pos != r.start && s.pts < t.samples[r.start].pts && t.syncBefore(pos) == r.start {
				continue // a leading picture of the run's first sync sample
			}
			if cap(buf) < int(s.size) {
				buf = make([]byte, s.size)
			}
			b := buf[:s.size]
			if _, err := c.f.ReadAt(b, int64(s.offset)); err != nil {
				return fmt.Errorf("reading frame %d: %w", pos, err)
			}
			if !s.hidden {
				pending = insertByPTS(pending, pos, t)
			}
			pics, err := dec.decode(b)
			if s.hidden && t.inOrder() {
				// Decoded for reference only; a decoder that hands it out
				// anyway is not showing it.
				for _, p := range pics {
					if p.release != nil {
						p.release()
					}
				}
				pics = nil
			}
			if len(pics) == 0 && t.inOrder() && !s.hidden && err == nil {
				// Shown, by its header, and yet nothing came out: it is
				// not waiting behind a reordering, so nothing will.
				for k, q := range pending {
					if q == pos {
						pending = append(pending[:k], pending[k+1:]...)
						break
					}
				}
			}
			take(pics)
			if err != nil {
				return c.decodeErr(err)
			}
		}
		pics, err := dec.flush()
		take(pics)
		if err != nil {
			return c.decodeErr(err)
		}
		return nil
	}

	// Runs overlap only where a leading picture pulled one back a group,
	// and then a frame both decode is used by the first to reach it: the
	// same frame either way. Each worker takes the next run.
	var (
		mu   sync.Mutex
		next int
		errs []error
		wg   sync.WaitGroup
	)
	claim := use
	use = func(at, i int, p planes) {
		mu.Lock()
		done := got[at]
		got[at] = true
		mu.Unlock()
		if !done {
			claim(at, i, p)
		}
	}
	workers := min(runtime.GOMAXPROCS(0), len(runs))
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				mu.Lock()
				if next >= len(runs) || len(errs) > 0 {
					mu.Unlock()
					return
				}
				r := runs[next]
				next++
				mu.Unlock()
				err := decode(r, false)
				if err != nil && t.codec == "vp9" {
					// Frames already taken from the first decoder are
					// not taken again: claim keeps the first.
					err = decode(r, true)
				}
				if err != nil {
					mu.Lock()
					errs = append(errs, err)
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	if len(errs) > 0 {
		return errs[0]
	}
	for at, ok := range got {
		if !ok {
			return fmt.Errorf("frame %d of the clip did not decode", idx[at])
		}
	}
	return nil
}

func (c *Clip) timeOf(i int) float64 {
	first := c.t.samples[c.order[0]].pts
	return float64(c.t.samples[i].pts-first) / c.t.timescale
}

func (c *Clip) decodeErr(err error) error {
	return fmt.Errorf("decoding the %s video: %w (pass its frames instead: get_help(\"invoke_systemone\") shows how with ffmpeg)", c.t.codec, err)
}

func insertByPTS(pending []int, i int, t *track) []int {
	at := sort.Search(len(pending), func(k int) bool { return t.samples[pending[k]].pts > t.samples[i].pts })
	pending = append(pending, 0)
	copy(pending[at+1:], pending[at:])
	pending[at] = i
	return pending
}

// toRGB converts a decoded picture to 8-bit RGB: BT.709 or BT.601 as the
// stream says, studio range unless it says full.
func toRGB(p planes) *vision.Picture {
	// Coefficients for R = Y + rv*V, G = Y - gu*U - gv*V, B = Y + bu*U.
	kr, kb := 0.299, 0.114
	if p.bt709 {
		kr, kb = 0.2126, 0.0722
	}
	kg := 1 - kr - kb
	ys, cs, yo := 255.0/219, 255.0/224, 16.0
	if p.fullRange {
		ys, cs, yo = 1, 1, 0
	}
	rv := 2 * (1 - kr) * cs
	bu := 2 * (1 - kb) * cs
	gu := 2 * (1 - kb) * kb / kg * cs
	gv := 2 * (1 - kr) * kr / kg * cs
	shift := p.depth - 8
	if shift < 0 {
		shift = 0
	}
	sx, sy := 1, 1
	if p.cw < p.w {
		sx = 2
	}
	if p.ch < p.h {
		sy = 2
	}
	luma := func(x, y int) float64 {
		if p.y16 != nil {
			return float64(p.y16[y*p.strideY+x]) / float64(int(1)<<shift)
		}
		return float64(p.y[y*p.strideY+x])
	}
	chroma := func(x, y int) (float64, float64) {
		x, y = x/sx, y/sy
		switch {
		case p.cb16 != nil:
			d := float64(int(1) << shift)
			return float64(p.cb16[y*p.strideC+x]) / d, float64(p.cr16[y*p.strideC+x]) / d
		case p.cb != nil:
			return float64(p.cb[y*p.strideC+x]), float64(p.cr[y*p.strideC+x])
		}
		return 128, 128 // monochrome
	}
	out := &vision.Picture{W: p.w, H: p.h, Pix: make([]uint8, p.w*p.h*3)}
	for y := 0; y < p.h; y++ {
		for x := 0; x < p.w; x++ {
			yy := (luma(x, y) - yo) * ys
			u, v := chroma(x, y)
			u, v = u-128, v-128
			o := (y*p.w + x) * 3
			out.Pix[o] = clamp(yy + rv*v)
			out.Pix[o+1] = clamp(yy - gu*u - gv*v)
			out.Pix[o+2] = clamp(yy + bu*u)
		}
	}
	return out
}

func clamp(v float64) uint8 {
	v = math.Round(v)
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// upright turns a picture by the track's display rotation.
func upright(p *vision.Picture, deg int) *vision.Picture {
	if deg == 0 {
		return p
	}
	w, h := p.W, p.H
	ow, oh := w, h
	if deg == 90 || deg == 270 {
		ow, oh = h, w
	}
	out := &vision.Picture{W: ow, H: oh, Pix: make([]uint8, len(p.Pix))}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var nx, ny int
			switch deg {
			case 90:
				nx, ny = h-1-y, x
			case 180:
				nx, ny = w-1-x, h-1-y
			case 270:
				nx, ny = y, w-1-x
			}
			copy(out.Pix[(ny*ow+nx)*3:][:3], p.Pix[(y*w+x)*3:][:3])
		}
	}
	return out
}

// IsVideo reports whether a file looks like a video this package reads: an
// extension that says so, or else an EBML header, or an ftyp box first
// whose brand is not a HEIF or AVIF picture's (those are ISO files too).
func IsVideo(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".m4v", ".mov", ".qt", ".3gp", ".webm", ".mkv":
		return true
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".heic", ".heif", ".avif":
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	var head [12]byte
	if _, err := f.ReadAt(head[:], 0); err != nil {
		return false
	}
	if isMatroska(head[:]) {
		return true
	}
	switch string(head[4:8]) {
	case "moov", "mdat", "wide":
		return true
	case "ftyp":
		switch string(head[8:12]) {
		case "heic", "heix", "heim", "heis", "mif1", "msf1", "avif", "avis":
			return false
		}
		return true
	}
	return false
}
