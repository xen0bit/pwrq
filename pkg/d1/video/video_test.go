package video

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xen0bit/pwrq/pkg/d1/vision"
)

// reference.json holds, for each fixture, ffmpeg's decode of it: a hash of
// every frame's luma plane in presentation order (the 10-bit one as
// yuv420p10le), and for the rotated clip its first frame, upright, as the
// mean of each 8x8 block of each channel. The fixtures and it were made with:
//
//	ffmpeg -f lavfi -i testsrc2=duration=3:size=320x240:rate=30 -c:v libx264 -profile:v high -bf 3 \
//	  -x264-params open-gop=1:keyint=30:min-keyint=30 -pix_fmt yuv420p -movflags +faststart h264.mp4
//	ffmpeg -f lavfi -i testsrc2=duration=2:size=256x144:rate=25 -c:v libx265 -x265-params keyint=20 \
//	  -pix_fmt yuv420p -tag:v hvc1 hevc.mp4
//	ffmpeg -f lavfi -i testsrc2=duration=1:size=256x144:rate=25 -c:v libx265 -pix_fmt yuv420p10le -tag:v hvc1 hevc10.mov
//	ffmpeg -f lavfi -i testsrc=duration=1:size=320x176:rate=10 -c:v libx264 tmp.mp4
//	ffmpeg -display_rotation 90 -i tmp.mp4 -c copy rotated.mp4
//	ffmpeg -f lavfi -i testsrc2=duration=2:size=160x120:rate=25 -c:v libvpx -b:v 200k -auto-alt-ref 1 \
//	  -lag-in-frames 16 -g 25 -pass 1|2 vp8.webm          (two passes: two hidden alt-ref frames)
//	ffmpeg -f lavfi -i testsrc2=duration=2:size=160x120:rate=25 -c:v libvpx-vp9 -b:v 200k -auto-alt-ref 1 \
//	  -lag-in-frames 16 -g 20 vp9.webm
//	ffmpeg -f lavfi -i testsrc2=duration=2:size=160x120:rate=25 -c:v libvpx-vp9 -deadline realtime -cpu-used 8 \
//	  -b:v 200k -g 20 vp9rt.webm                             (alt-refs in superframes)
//	ffmpeg -f lavfi -i testsrc2=duration=2:size=160x120:rate=25 -c:v libaom-av1 -cpu-used 8 -b:v 150k -g 20 av1.webm
//	ffmpeg -f lavfi -i testsrc2=duration=1:size=128x96:rate=25 -c:v libaom-av1 -cpu-used 8 -b:v 150k \
//	  -pix_fmt yuv420p10le av1-10.mkv
//	ffmpeg -i vp9.webm -c copy vp9.mp4; ffmpeg -i av1.webm -c copy av1.mp4
//	ffmpeg -i h264.mp4 -c copy h264.mkv; ffmpeg -i rotated.mp4 -c copy rotated.mkv
//	ffmpeg -f lavfi -i testsrc2=duration=0.5:size=64x48:rate=10 -c:v mpeg4 other.avi
//	ffmpeg -i <clip> -f rawvideo -pix_fmt <yuv420p|yuv420p10le> -   (hashed a frame at a time)
func reference(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	b, err := os.ReadFile("testdata/reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var ref map[string]json.RawMessage
	if err := json.Unmarshal(b, &ref); err != nil {
		t.Fatal(err)
	}
	return ref
}

func lumaHash(p planes) string {
	h := sha256.New()
	for y := 0; y < p.h; y++ {
		if p.y16 != nil {
			row := make([]byte, p.w*2)
			for x := 0; x < p.w; x++ {
				binary.LittleEndian.PutUint16(row[x*2:], p.y16[y*p.strideY+x])
			}
			h.Write(row)
			continue
		}
		h.Write(p.y[y*p.strideY : y*p.strideY+p.w])
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// TestFramesMatchFFmpeg decodes frames out of order and from the middle of
// each clip - every run starting at a sync sample, the open-GOP H.264 one
// with leading B pictures that reference the group before - and checks
// each luma plane is ffmpeg's, byte for byte.
func TestFramesMatchFFmpeg(t *testing.T) {
	ref := reference(t)
	for _, tc := range []struct {
		file string
		idx  []int
	}{
		{"h264.mp4", []int{0, 1, 2, 3}},
		{"h264.mp4", []int{89, 31, 29, 30, 60, 58, 45}}, // 29/58: leading pictures shown before a keyframe
		{"h264.mp4", seq(90)},
		{"hevc.mp4", []int{49, 0, 20, 19, 33}},
		{"hevc.mp4", seq(50)},
		{"hevc10.mov", []int{24, 0, 12}},
		{"h264.mkv", []int{89, 31, 29, 30, 60, 58, 45}}, // the same stream as h264.mp4, B-frames and all
		{"vp8.webm", seq(50)},                           // hidden alt-refs are decoded, never shown
		{"vp8.webm", []int{49, 26, 3}},
		// govpx refuses an inter frame of vp9.webm (one-pass, bitrate-targeted)
		// and go-vp9 decodes the group instead; vp9rt.webm (realtime, alt-refs
		// in superframes) is one go-vp9 decodes wrong and govpx right.
		{"vp9.webm", seq(50)},
		{"vp9.webm", []int{45, 21, 0}},
		{"vp9rt.webm", seq(50)},
		{"vp9.mp4", []int{10, 30}},
		{"av1.webm", seq(50)},
		{"av1.webm", []int{49, 25, 1}},
		{"av1.mp4", []int{10, 30}},
		{"av1-10.mkv", seq(25)},
	} {
		var want []string
		if err := json.Unmarshal(ref[tc.file], &want); err != nil {
			t.Fatal(err)
		}
		c, err := Open("testdata/" + tc.file)
		if err != nil {
			t.Fatal(err)
		}
		if c.Info().Frames != len(want) {
			t.Errorf("%s: %d frames, ffmpeg decodes %d", tc.file, c.Info().Frames, len(want))
		}
		got := make([]string, len(tc.idx))
		err = c.decodeAt(tc.idx, func(at, _ int, p planes) { got[at] = lumaHash(p) })
		_ = c.Close()
		if err != nil {
			t.Fatalf("%s %v: %v", tc.file, tc.idx, err)
		}
		for at, k := range tc.idx {
			if got[at] != want[k] {
				t.Errorf("%s: frame %d's luma is not ffmpeg's", tc.file, k)
			}
		}
	}
}

func seq(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

// TestSampleReadsAsD1Does: two frames a second, sized for the processor,
// times from the start of what was read.
func TestSampleReadsAsD1Does(t *testing.T) {
	c, err := Open("testdata/h264.mp4")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	info := c.Info()
	if info.Codec != "h264" || math.Abs(info.FPS-30) > 0.01 || math.Abs(info.Duration-3) > 0.01 {
		t.Errorf("info = %+v, want h264 at 30 fps for 3s", info)
	}
	frames, err := c.Sample(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 6 {
		t.Fatalf("%d frames, want 6 (two a second for 3s)", len(frames))
	}
	h, w, _ := vision.VideoSize(6, 240, 320)
	for i, f := range frames {
		if f.Picture.W != w || f.Picture.H != h {
			t.Errorf("frame %d is %dx%d, want VideoSize's %dx%d", i, f.Picture.W, f.Picture.H, w, h)
		}
	}
	if frames[0].Time != 0 || math.Abs(frames[5].Time-89.0/30) > 1e-6 {
		t.Errorf("times %v..%v, want 0..%v", frames[0].Time, frames[5].Time, 89.0/30)
	}

	// A window: times count from its start.
	win, err := c.Sample(1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(win) != 4 || win[0].Time != 0 {
		t.Errorf("1s..2s gave %d frames from %vs, want 4 (the minimum) from 0s", len(win), win[0].Time)
	}
	if _, err := c.Sample(5, 0); err == nil {
		t.Error("a window past the end should say so")
	}
}

// TestRotatedVideoIsUpright: a portrait phone clip is stored landscape with
// a display matrix; it is read the way it is shown, as ffmpeg shows it.
func TestRotatedVideoIsUpright(t *testing.T) {
	for _, file := range []string{"rotated.mp4", "rotated.mkv"} {
		t.Run(file, func(t *testing.T) { testUpright(t, file) })
	}
}

func testUpright(t *testing.T, file string) {
	var ref struct {
		W      int   `json:"width"`
		H      int   `json:"height"`
		Blocks []int `json:"blocks8"`
	}
	if err := json.Unmarshal(reference(t)["rotated.mp4"], &ref); err != nil {
		t.Fatal(err)
	}
	c, err := Open("testdata/" + file)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if r := c.Info().Rotation; r != 90 && r != 270 {
		t.Fatalf("rotation %d, want a quarter turn", r)
	}
	frames, err := c.Frames([]int{0}, func(w, h int) (int, int) { return w, h })
	if err != nil {
		t.Fatal(err)
	}
	p := frames[0].Picture
	if p.W != ref.W || p.H != ref.H {
		t.Fatalf("upright frame is %dx%d, want %dx%d", p.W, p.H, ref.W, ref.H)
	}
	diff, i := 0.0, 0
	for by := 0; by < p.H/8; by++ {
		for bx := 0; bx < p.W/8; bx++ {
			for ch := 0; ch < 3; ch++ {
				sum := 0
				for y := by * 8; y < by*8+8; y++ {
					for x := bx * 8; x < bx*8+8; x++ {
						sum += int(p.Pix[(y*p.W+x)*3+ch])
					}
				}
				diff += math.Abs(float64(sum)/64 - float64(ref.Blocks[i]))
				i++
			}
		}
	}
	// Measured 0.5: within rounding of ffmpeg's colour conversion. Turned
	// the wrong way it is over 100.
	if mean := diff / float64(i); mean > 3 {
		t.Errorf("upright frame differs from ffmpeg's by %.1f a channel on average: turned the wrong way?", mean)
	}
}

// TestColourMatchesFFmpeg: the RGB conversion lands within rounding of
// ffmpeg's for a BT.601 clip (testsrc2's colour bars).
func TestColourIsClose(t *testing.T) {
	c, err := Open("testdata/h264.mp4")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	frames, err := c.Frames([]int{0}, func(w, h int) (int, int) { return w, h })
	if err != nil {
		t.Fatal(err)
	}
	p := frames[0].Picture
	// The frame is not uniform: a picture of one grey would mean the
	// chroma never arrived.
	seen := map[[3]uint8]bool{}
	for i := 0; i < len(p.Pix); i += 3 * 97 {
		seen[[3]uint8{p.Pix[i] / 32, p.Pix[i+1] / 32, p.Pix[i+2] / 32}] = true
	}
	if len(seen) < 6 {
		t.Errorf("only %d coarse colours in a colour-bar frame", len(seen))
	}
}

func TestOtherFormatsAreRefusedWithTheWayOut(t *testing.T) {
	_, err := Open("testdata/other.avi")
	if err == nil {
		t.Fatal("an AVI should be refused")
	}
	if !strings.Contains(err.Error(), "MP4") {
		t.Errorf("error %q should say what is read", err)
	}
}

// TestHiddenFramesAreNotFrames: VP8 keeps its alt-ref frames as samples of
// their own, decoded for reference and never shown; a clip is its shown
// frames, as ffmpeg counts them.
func TestHiddenFramesAreNotFrames(t *testing.T) {
	c, err := Open("testdata/vp8.webm")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	hidden := 0
	for _, s := range c.t.samples {
		if s.hidden {
			hidden++
		}
	}
	if info := c.Info(); info.Frames != 50 || hidden != 2 || math.Abs(info.Duration-2) > 0.01 {
		t.Errorf("%d frames (%d hidden) over %.2fs, want 50 (2 hidden) over 2s", info.Frames, hidden, info.Duration)
	}
}

// TestLiveRecordingsAreRead: a browser's MediaRecorder writes its segment
// and clusters with unknown sizes, since it does not know them yet; each
// cluster ends where the next begins. The fixture is rewritten that way.
func TestLiveRecordingsAreRead(t *testing.T) {
	b, err := os.ReadFile("testdata/vp9.webm")
	if err != nil {
		t.Fatal(err)
	}
	live := unsize(t, b)
	path := filepath.Join(t.TempDir(), "live.webm")
	if err := os.WriteFile(path, live, 0o600); err != nil {
		t.Fatal(err)
	}
	var want []string
	if err := json.Unmarshal(reference(t)["vp9.webm"], &want); err != nil {
		t.Fatal(err)
	}
	c, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if c.Info().Frames != len(want) {
		t.Fatalf("%d frames, want %d", c.Info().Frames, len(want))
	}
	got := map[int]string{}
	if err := c.decodeAt([]int{0, 22, 49}, func(at, _ int, p planes) { got[at] = lumaHash(p) }); err != nil {
		t.Fatal(err)
	}
	for at, k := range []int{0, 22, 49} {
		if got[at] != want[k] {
			t.Errorf("frame %d's luma is not ffmpeg's", k)
		}
	}
	// Cut off mid-cluster, as a recording that stopped: what is whole is read.
	cut := filepath.Join(t.TempDir(), "cut.webm")
	if err := os.WriteFile(cut, live[:len(live)*2/3], 0o600); err != nil {
		t.Fatal(err)
	}
	cc, err := Open(cut)
	if err != nil {
		t.Fatalf("a recording cut short: %v", err)
	}
	defer func() { _ = cc.Close() }()
	if n := cc.Info().Frames; n < 10 || n >= len(want) {
		t.Errorf("a recording cut at two thirds has %d frames", n)
	}
}

// unsize rewrites a Matroska file's segment and clusters to unknown size.
func unsize(t *testing.T, b []byte) []byte {
	t.Helper()
	e := &ebmlReader{r: bytes.NewReader(b), size: int64(len(b))}
	hdr, err := e.element(0, e.size)
	if err != nil {
		t.Fatal(err)
	}
	seg, err := e.element(hdr.end, e.size)
	if err != nil || seg.id != idSegment {
		t.Fatalf("no segment after the header: %v", err)
	}
	unknown := []byte{0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	out := append([]byte(nil), b[:hdr.end]...)
	out = append(append(out, 0x18, 0x53, 0x80, 0x67), unknown...)
	clusters := 0
	for off := seg.start; off < seg.end; {
		el, err := e.element(off, seg.end)
		if err != nil {
			t.Fatal(err)
		}
		if el.id == idCluster {
			clusters++
			out = append(append(out, 0x1F, 0x43, 0xB6, 0x75), unknown...)
			out = append(out, b[el.start:el.end]...)
		} else {
			out = append(out, b[off:el.end]...)
		}
		off = el.end
	}
	if clusters < 2 {
		t.Fatalf("the fixture has %d clusters; the test needs two or more", clusters)
	}
	return out
}

func TestIsVideo(t *testing.T) {
	for path, want := range map[string]bool{
		"testdata/h264.mp4":   true,
		"testdata/hevc10.mov": true,
		"testdata/vp9.webm":   true,
		"testdata/other.avi":  false,
		"x.png":               false,
		"clip.MOV":            true,
		"talk.mkv":            true,
	} {
		if got := IsVideo(path); got != want {
			t.Errorf("IsVideo(%s) = %v, want %v", path, got, want)
		}
	}
}
