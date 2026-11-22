package video

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	mp4av1 "github.com/Eyevinn/mp4ff/av1"
	"github.com/Eyevinn/mp4ff/avc"
	"github.com/Eyevinn/mp4ff/hevc"
)

// Matroska and WebM: an EBML tree whose clusters hold each track's frames
// as blocks. Only the element headers and each block's first bytes are
// read here; a frame's bytes stay in the file until it is decoded.
//
// A recording a browser makes (MediaRecorder) is written as it happens, so
// its segment and clusters have unknown sizes: a cluster then ends where
// the next element that cannot be inside one begins. That is the WebM most
// people have, and the reason this is not a reader of sized trees alone.

const (
	idEBML        = 0x1A45DFA3
	idDocType     = 0x4282
	idSegment     = 0x18538067
	idInfo        = 0x1549A966
	idTimecodeSc  = 0x2AD7B1
	idTracks      = 0x1654AE6B
	idTrackEntry  = 0xAE
	idTrackNumber = 0xD7
	idTrackType   = 0x83
	idCodecID     = 0x86
	idCodecPriv   = 0x63A2
	idEncodings   = 0x6D80
	idVideo       = 0xE0
	idColour      = 0x55B0
	idMatrix      = 0x55B1
	idRange       = 0x55B9
	idProjection  = 0x7670
	idPoseRoll    = 0x7675
	idCluster     = 0x1F43B675
	idTimecode    = 0xE7
	idSimpleBlock = 0xA3
	idBlockGroup  = 0xA0
	idBlock       = 0xA1
	idRefBlock    = 0xFB
)

// segmentLevel are the elements that sit directly in a segment: in a
// cluster of unknown size, meeting one is the cluster's end.
var segmentLevel = map[uint32]bool{
	idCluster: true, idInfo: true, idTracks: true,
	0x114D9B74: true, // SeekHead
	0x1C53BB6B: true, // Cues
	0x1043A770: true, // Chapters
	0x1254C367: true, // Tags
	0x1941A469: true, // Attachments
}

const unknownSize = -1

// ebmlReader reads a file through a window, since the headers it walks are
// a few bytes each and many: an hour of video is a hundred thousand blocks.
type ebmlReader struct {
	r    io.ReaderAt
	size int64
	buf  []byte
	at   int64 // file offset of buf[0]
}

func (e *ebmlReader) bytes(off int64, n int) ([]byte, error) {
	if off < 0 || off+int64(n) > e.size {
		return nil, io.ErrUnexpectedEOF
	}
	if off < e.at || off+int64(n) > e.at+int64(len(e.buf)) {
		want := max(n, 64<<10)
		want = int(min(int64(want), e.size-off))
		if cap(e.buf) < want {
			e.buf = make([]byte, want)
		}
		e.buf = e.buf[:want]
		if _, err := e.r.ReadAt(e.buf, off); err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		e.at = off
	}
	return e.buf[off-e.at : off-e.at+int64(n)], nil
}

// vint reads a variable-length integer at off: an element ID keeps its
// length marker, a size does not, and a size of all ones is unknown.
func (e *ebmlReader) vint(off int64, id bool) (v int64, n int, err error) {
	b, err := e.bytes(off, 1)
	if err != nil {
		return 0, 0, err
	}
	first := b[0]
	n = 1
	for mask := byte(0x80); n <= 8 && first&mask == 0; mask >>= 1 {
		n++
	}
	if n > 8 || (id && n > 4) {
		return 0, 0, errors.New("a malformed element header")
	}
	b, err = e.bytes(off, n)
	if err != nil {
		return 0, 0, err
	}
	if id {
		for _, c := range b {
			v = v<<8 | int64(c)
		}
		return v, n, nil
	}
	v = int64(first & (0xFF >> n))
	ones := v == int64(0xFF>>n)
	for _, c := range b[1:] {
		v = v<<8 | int64(c)
		ones = ones && c == 0xFF
	}
	if ones {
		return unknownSize, n, nil
	}
	return v, n, nil
}

// element is one element's ID and where its data lies; end is the file's
// end for one of unknown size.
type element struct {
	id         uint32
	start, end int64
	unknown    bool
}

func (e *ebmlReader) element(off, limit int64) (element, error) {
	id, n, err := e.vint(off, true)
	if err != nil {
		return element{}, err
	}
	size, m, err := e.vint(off+int64(n), false)
	if err != nil {
		return element{}, err
	}
	el := element{id: uint32(id), start: off + int64(n+m)}
	if size == unknownSize {
		el.unknown, el.end = true, limit
	} else {
		el.end = el.start + size
		if el.end > limit {
			// A file cut short - a recording that stopped - keeps what it has.
			el.end = limit
		}
	}
	return el, nil
}

func (e *ebmlReader) uint(el element) (uint64, error) {
	n := el.end - el.start
	if n > 8 {
		return 0, errors.New("an integer element is too long")
	}
	b, err := e.bytes(el.start, int(n))
	if err != nil {
		return 0, err
	}
	var v uint64
	for _, c := range b {
		v = v<<8 | uint64(c)
	}
	return v, nil
}

func (e *ebmlReader) float(el element) (float64, error) {
	b, err := e.bytes(el.start, int(el.end-el.start))
	if err != nil {
		return 0, err
	}
	switch len(b) {
	case 4:
		return float64(math.Float32frombits(binary.BigEndian.Uint32(b))), nil
	case 8:
		return math.Float64frombits(binary.BigEndian.Uint64(b)), nil
	case 0:
		return 0, nil
	}
	return 0, errors.New("a float element is not 4 or 8 bytes")
}

func (e *ebmlReader) data(el element) ([]byte, error) {
	b, err := e.bytes(el.start, int(el.end-el.start))
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), b...), nil
}

// children calls fn for each element inside parent, in order.
func (e *ebmlReader) children(parent element, fn func(element) error) error {
	for off := parent.start; off < parent.end; {
		el, err := e.element(off, parent.end)
		if err != nil {
			return err
		}
		if el.unknown {
			return fmt.Errorf("element %x has an unknown size where only a cluster may", el.id)
		}
		if err := fn(el); err != nil {
			return err
		}
		off = el.end
	}
	return nil
}

// isMatroska reports whether a file starts with an EBML header.
func isMatroska(head []byte) bool {
	return len(head) >= 4 && binary.BigEndian.Uint32(head) == idEBML
}

// readMatroska reads a Matroska or WebM file's first video track.
func readMatroska(r io.ReaderAt, size int64) (*track, error) {
	e := &ebmlReader{r: r, size: size}
	hdr, err := e.element(0, size)
	if err != nil || hdr.id != idEBML {
		return nil, errors.New("not a Matroska or WebM file")
	}
	doc := ""
	if err := e.children(hdr, func(el element) error {
		if el.id == idDocType {
			b, err := e.data(el)
			doc = string(b)
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if doc != "webm" && doc != "matroska" {
		return nil, fmt.Errorf("an EBML file of type %q, not Matroska or WebM", doc)
	}

	var seg element
	for off := hdr.end; ; {
		el, err := e.element(off, size)
		if err != nil {
			return nil, errors.New("the file has no Matroska segment")
		}
		if el.id == idSegment {
			seg = el
			break
		}
		off = el.end
	}

	m := &mkvReader{e: e, scale: 1_000_000}
	for off := seg.start; off < seg.end; {
		el, err := e.element(off, seg.end)
		if err != nil {
			if m.t != nil && len(m.t.samples) > 0 {
				break // the rest of a recording that stopped mid-write
			}
			return nil, err
		}
		switch el.id {
		case idInfo:
			err = m.info(el)
		case idTracks:
			err = m.tracks(el)
		case idCluster:
			if m.t == nil {
				return nil, errNoVideo
			}
			el.end, err = m.cluster(el)
		default:
			if el.unknown {
				return nil, fmt.Errorf("element %x has an unknown size where only a cluster may", el.id)
			}
		}
		if err != nil {
			if m.t != nil && len(m.t.samples) > 0 && errors.Is(err, io.ErrUnexpectedEOF) {
				break
			}
			return nil, err
		}
		off = el.end
	}
	if m.t == nil {
		return nil, errNoVideo
	}
	if len(m.t.samples) == 0 {
		return nil, errors.New("the video track has no frames")
	}
	m.t.timescale = 1e9 / float64(m.scale)
	m.t.samples[0].sync = true
	return m.t, nil
}

type mkvReader struct {
	e      *ebmlReader
	scale  uint64 // nanoseconds a timecode unit
	number uint64 // the video track's number
	t      *track
}

func (m *mkvReader) info(el element) error {
	return m.e.children(el, func(c element) error {
		if c.id == idTimecodeSc {
			v, err := m.e.uint(c)
			if v > 0 {
				m.scale = v
			}
			return err
		}
		return nil
	})
}

func (m *mkvReader) tracks(el element) error {
	return m.e.children(el, func(c element) error {
		if c.id != idTrackEntry || m.t != nil {
			return nil
		}
		var (
			number, kind uint64
			codec        string
			private      []byte
			encoded      bool
			matrix, rng  uint64
			roll         float64
		)
		err := m.e.children(c, func(f element) error {
			var err error
			switch f.id {
			case idTrackNumber:
				number, err = m.e.uint(f)
			case idTrackType:
				kind, err = m.e.uint(f)
			case idCodecID:
				var b []byte
				b, err = m.e.data(f)
				codec = string(b)
			case idCodecPriv:
				private, err = m.e.data(f)
			case idEncodings:
				encoded = true
			case idVideo:
				err = m.e.children(f, func(v element) error {
					switch v.id {
					case idColour:
						return m.e.children(v, func(k element) error {
							var err error
							switch k.id {
							case idMatrix:
								matrix, err = m.e.uint(k)
							case idRange:
								rng, err = m.e.uint(k)
							}
							return err
						})
					case idProjection:
						return m.e.children(v, func(k element) error {
							var err error
							if k.id == idPoseRoll {
								roll, err = m.e.float(k)
							}
							return err
						})
					}
					return nil
				})
			}
			return err
		})
		if err != nil || kind != 1 {
			return err
		}
		if encoded {
			// Header stripping or zlib on frames: rare, and every
			// sample's bytes would have to be rebuilt.
			return errors.New("the video track's frames are compressed in the container (ContentEncoding), which is not read; remux it with `ffmpeg -i in.mkv -c copy out.mkv`")
		}
		t := &track{matrix: int(matrix), fullRange: rng == 2}
		// ProjectionPoseRoll turns the picture counter-clockwise.
		t.rotation = ((int(math.Round(-roll/90))*90)%360 + 360) % 360
		switch codec {
		case "V_VP8":
			t.codec = "vp8"
		case "V_VP9":
			t.codec = "vp9"
		case "V_AV1":
			t.codec = "av1"
			if len(private) > 0 {
				rec, err := mp4av1.DecodeAV1CodecConfRec(private)
				if err != nil {
					return fmt.Errorf("the AV1 track's configuration: %w", err)
				}
				if len(rec.ConfigOBUs) > 0 {
					t.params = [][]byte{rec.ConfigOBUs}
				}
			}
		case "V_MPEG4/ISO/AVC":
			rec, err := avc.DecodeAVCDecConfRec(private)
			if err != nil {
				return fmt.Errorf("the H.264 track's configuration: %w", err)
			}
			t.codec, t.lengthSize = "h264", 4
			t.params = append(append(t.params, rec.SPSnalus...), rec.PPSnalus...)
		case "V_MPEGH/ISO/HEVC":
			rec, err := hevc.DecodeHEVCDecConfRec(private)
			if err != nil {
				return fmt.Errorf("the HEVC track's configuration: %w", err)
			}
			t.codec, t.lengthSize = "hevc", int(rec.LengthSizeMinusOne)+1
			for _, a := range rec.NaluArrays {
				t.params = append(t.params, a.Nalus...)
			}
		default:
			return fmt.Errorf("the video is %s; %s", codec, otherCodecs)
		}
		m.number, m.t = number, t
		return nil
	})
}

// cluster reads a cluster's blocks of the video track, and returns where
// the cluster ends: for one of unknown size, where the next segment-level
// element begins.
func (m *mkvReader) cluster(el element) (int64, error) {
	var tc int64
	for off := el.start; off < el.end; {
		c, err := m.e.element(off, el.end)
		if err != nil {
			return off, err
		}
		if el.unknown && segmentLevel[c.id] {
			return off, nil
		}
		if c.unknown {
			return off, errors.New("a cluster holds an element of unknown size")
		}
		switch c.id {
		case idTimecode:
			v, err := m.e.uint(c)
			if err != nil {
				return off, err
			}
			tc = int64(v)
		case idSimpleBlock:
			if err := m.block(c, tc, nil); err != nil {
				return off, err
			}
		case idBlockGroup:
			var blk *element
			ref := false
			if err := m.e.children(c, func(g element) error {
				switch g.id {
				case idBlock:
					blk = &g
				case idRefBlock:
					ref = true
				}
				return nil
			}); err != nil {
				return off, err
			}
			if blk != nil {
				key := !ref
				if err := m.block(*blk, tc, &key); err != nil {
					return off, err
				}
			}
		}
		off = c.end
	}
	return el.end, nil
}

// block adds a block of the video track as a sample. A SimpleBlock says
// itself whether it is a keyframe; in a BlockGroup it is one when it
// references nothing (key).
func (m *mkvReader) block(el element, tc int64, key *bool) error {
	num, n, err := m.e.vint(el.start, false)
	if err != nil {
		return err
	}
	if uint64(num) != m.number {
		return nil
	}
	h, err := m.e.bytes(el.start+int64(n), 3)
	if err != nil {
		return err
	}
	rel := int64(int16(binary.BigEndian.Uint16(h)))
	flags := h[2]
	if flags&0x06 != 0 {
		return errors.New("the video track's frames are laced, which is not read; remux it with `ffmpeg -i in.mkv -c copy out.mkv`")
	}
	sync := flags&0x80 != 0
	if key != nil {
		sync = *key
	}
	off := el.start + int64(n) + 3
	if el.end <= off {
		return nil
	}
	m.t.samples = append(m.t.samples, sample{offset: uint64(off), size: uint32(el.end - off), pts: tc + rel, sync: sync})
	return nil
}
