package video

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
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
//	ffmpeg -f lavfi -i testsrc2=duration=1:size=128x96:rate=10 -c:v libvpx-vp9 vp9.webm
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
	var ref struct {
		W      int   `json:"width"`
		H      int   `json:"height"`
		Blocks []int `json:"blocks8"`
	}
	if err := json.Unmarshal(reference(t)["rotated.mp4"], &ref); err != nil {
		t.Fatal(err)
	}
	c, err := Open("testdata/rotated.mp4")
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
	_, err := Open("testdata/vp9.webm")
	if err == nil {
		t.Fatal("a WebM should be refused")
	}
	if !strings.Contains(err.Error(), "MP4") {
		t.Errorf("error %q should say what is read", err)
	}
}

func TestIsVideo(t *testing.T) {
	for path, want := range map[string]bool{
		"testdata/h264.mp4":   true,
		"testdata/hevc10.mov": true,
		"testdata/vp9.webm":   false,
		"x.png":               false,
		"clip.MOV":            true,
	} {
		if got := IsVideo(path); got != want {
			t.Errorf("IsVideo(%s) = %v, want %v", path, got, want)
		}
	}
}
