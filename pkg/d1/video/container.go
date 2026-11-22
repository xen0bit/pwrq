package video

import (
	"errors"
	"fmt"
	"io"
	"math"
	"sort"

	"github.com/Eyevinn/mp4ff/mp4"
)

// track is a video file's first video track: where each sample's bytes
// are, when it is shown, and what its decoder needs before the first one.
type track struct {
	codec      string // "h264", "hevc", "vp8", "vp9" or "av1"
	timescale  float64
	samples    []sample // in decoding order
	params     [][]byte // parameter set NAL units (VPS, SPS, PPS), or AV1's configuration OBUs
	lengthSize int      // bytes of the length before each NAL unit in a sample
	rotation   int      // degrees clockwise the picture is turned when shown

	// The container's colour description, where the bitstream may not
	// carry one: matrix is the CICP code (1 BT.709, 5/6 BT.601, 0/2
	// unspecified).
	matrix    int
	fullRange bool
}

// inOrder reports whether the codec shows frames in the order they are
// decoded: VP8, VP9 and AV1 hide what they reorder inside a sample.
func (t *track) inOrder() bool { return t.codec == "vp8" || t.codec == "vp9" || t.codec == "av1" }

// otherCodecs is what is said of a codec pwrq does not decode.
const otherCodecs = "pwrq decodes H.264, HEVC (H.265), VP8, VP9 and AV1 itself, and anything else through its frames: get_help(\"invoke_systemone\") shows how with ffmpeg"

type sample struct {
	offset uint64
	size   uint32
	pts    int64 // presentation time, in timescale units
	sync   bool  // decoding can start here
	hidden bool  // decoded as a reference, never shown (a VP8 alt-ref frame)
}

// errFragmented and errNoVideo are the shapes of file the reader refuses
// with what to do instead.
var (
	errFragmented = errors.New("a fragmented MP4 (moof segments) is not read; remux it with `ffmpeg -i in.mp4 -c copy out.mp4`, or pass its frames")
	errNoVideo    = errors.New("the file has no video track")
)

// readTrack reads an MP4 or QuickTime file's sample table, leaving the
// media data where it is: a long recording is read a sample at a time.
func readTrack(r io.ReadSeeker) (*track, error) {
	f, err := mp4.DecodeFile(r, mp4.WithDecodeMode(mp4.DecModeLazyMdat))
	if err != nil {
		return nil, fmt.Errorf("not an MP4 or QuickTime file: %w", err)
	}
	if f.Moov == nil {
		return nil, errors.New("not an MP4 or QuickTime file: no moov box")
	}
	if len(f.Segments) > 0 {
		return nil, errFragmented
	}
	for _, trak := range f.Moov.Traks {
		if trak.Mdia == nil || trak.Mdia.Hdlr == nil || trak.Mdia.Hdlr.HandlerType != "vide" {
			continue
		}
		return readVideoTrak(trak)
	}
	return nil, errNoVideo
}

func readVideoTrak(trak *mp4.TrakBox) (*track, error) {
	if trak.Mdia.Minf == nil || trak.Mdia.Minf.Stbl == nil || trak.Mdia.Mdhd == nil {
		return nil, errors.New("the video track has no sample table")
	}
	stbl := trak.Mdia.Minf.Stbl
	t := &track{timescale: float64(trak.Mdia.Mdhd.Timescale)}
	if t.timescale <= 0 {
		return nil, errors.New("the video track has no timescale")
	}
	if trak.Tkhd != nil {
		t.rotation = rotation(trak.Tkhd.Matrix)
	}
	if stbl.Stsd == nil {
		return nil, errors.New("the video track has no sample description")
	}
	switch {
	case stbl.Stsd.AvcX != nil && stbl.Stsd.AvcX.AvcC != nil:
		c := stbl.Stsd.AvcX.AvcC
		t.codec = "h264"
		t.lengthSize = 4
		t.params = append(append(t.params, c.SPSnalus...), c.PPSnalus...)
	case stbl.Stsd.HvcX != nil && stbl.Stsd.HvcX.HvcC != nil:
		c := stbl.Stsd.HvcX.HvcC
		t.codec = "hevc"
		t.lengthSize = int(c.LengthSizeMinusOne) + 1
		for _, a := range c.NaluArrays {
			t.params = append(t.params, a.Nalus...)
		}
	case stbl.Stsd.Av01 != nil:
		t.codec = "av1"
		if c := stbl.Stsd.Av01.Av1C; c != nil && len(c.ConfigOBUs) > 0 {
			t.params = [][]byte{c.ConfigOBUs}
		}
	case stbl.Stsd.VpXX != nil:
		t.codec = map[string]string{"vp08": "vp8", "vp09": "vp9"}[stbl.Stsd.VpXX.Type()]
		if t.codec == "" {
			return nil, fmt.Errorf("the video is %s; %s", stbl.Stsd.VpXX.Type(), otherCodecs)
		}
		if c := stbl.Stsd.VpXX.VppC; c != nil {
			t.matrix, t.fullRange = int(c.MatrixCoefficients), c.VideoFullRangeFlag == 1
		}
	default:
		name := "unknown"
		if len(stbl.Stsd.Children) > 0 {
			name = stbl.Stsd.Children[0].Type()
		}
		return nil, fmt.Errorf("the video is %s; %s", name, otherCodecs)
	}
	if stbl.Stsz == nil || stbl.Stsc == nil || stbl.Stts == nil || (stbl.Stco == nil && stbl.Co64 == nil) {
		return nil, errors.New("the video track's sample table is incomplete")
	}

	n := int(stbl.Stsz.SampleNumber)
	if n == 0 {
		return nil, errors.New("the video track has no frames")
	}
	t.samples = make([]sample, n)
	size := func(i int) uint32 { // i 0-based
		if stbl.Stsz.SampleUniformSize != 0 {
			return stbl.Stsz.SampleUniformSize
		}
		return stbl.Stsz.SampleSize[i]
	}
	if stbl.Stsz.SampleUniformSize == 0 && len(stbl.Stsz.SampleSize) < n {
		return nil, errors.New("the video track's sample sizes are truncated")
	}

	// Offsets: chunk by chunk, each chunk's samples back to back.
	var chunks []uint64
	if stbl.Stco != nil {
		for _, o := range stbl.Stco.ChunkOffset {
			chunks = append(chunks, uint64(o))
		}
	} else {
		chunks = stbl.Co64.ChunkOffset
	}
	next := 0
	for c := range chunks {
		ch, err := stbl.Stsc.GetChunk(uint32(c + 1))
		if err != nil {
			return nil, fmt.Errorf("the video track's chunk table: %w", err)
		}
		off := chunks[c]
		for k := uint32(0); k < ch.NrSamples && next < n; k++ {
			i := int(ch.StartSampleNr+k) - 1
			if i != next {
				return nil, errors.New("the video track's chunk table is out of order")
			}
			t.samples[i].offset = off
			t.samples[i].size = size(i)
			off += uint64(t.samples[i].size)
			next++
		}
	}
	if next != n {
		return nil, fmt.Errorf("the video track's chunk table covers %d of %d frames", next, n)
	}

	// Times: decoding times from stts, plus the composition offset.
	dts := int64(0)
	i := 0
	for e, count := range stbl.Stts.SampleCount {
		for k := uint32(0); k < count && i < n; k++ {
			t.samples[i].pts = dts
			if stbl.Ctts != nil {
				t.samples[i].pts += int64(stbl.Ctts.GetCompositionTimeOffset(uint32(i + 1)))
			}
			dts += int64(stbl.Stts.SampleTimeDelta[e])
			i++
		}
	}
	for ; i < n; i++ {
		t.samples[i].pts = dts
	}
	for i := range t.samples {
		t.samples[i].sync = stbl.Stss == nil || stbl.Stss.IsSyncSample(uint32(i+1))
	}
	if !t.samples[0].sync {
		t.samples[0].sync = true // a stream starts somewhere
	}
	return t, nil
}

// rotation reads a track header's display matrix as a turn clockwise:
// 0, 90, 180 or 270. Phones record portrait video as landscape and say so
// here.
func rotation(m [9]int32) int {
	a, b := float64(m[0]), float64(m[1])
	if a == 0 && b == 0 {
		return 0
	}
	deg := int(math.Round(math.Atan2(b, a)*180/math.Pi/90)) * 90
	return ((deg % 360) + 360) % 360
}

// order is the samples shown, in presentation order.
func (t *track) order() []int {
	idx := make([]int, 0, len(t.samples))
	for i, s := range t.samples {
		if !s.hidden {
			idx = append(idx, i)
		}
	}
	sort.SliceStable(idx, func(a, b int) bool { return t.samples[idx[a]].pts < t.samples[idx[b]].pts })
	return idx
}

// syncBefore is the last sync sample at or before decoding index i.
func (t *track) syncBefore(i int) int {
	for ; i > 0; i-- {
		if t.samples[i].sync {
			return i
		}
	}
	return 0
}
