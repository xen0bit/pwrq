package video

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/gen2brain/gav1d/av1"
	"github.com/gen2brain/h265/hevc"
	vp9alt "github.com/mgvs/go-vp9/decoder"
	"github.com/mgvs/go-vp9/refbuf"
	h264 "github.com/oops1/go.264"
	"github.com/thesyncim/govpx"
)

// planes is one decoded picture, cropped: luma and two chroma planes, at 8
// bits (Y, Cb, Cr) or more (Y16, Cb16, Cr16, with depth).
type planes struct {
	w, h             int
	cw, ch           int // chroma plane size
	y, cb, cr        []byte
	y16, cb16, cr16  []uint16
	strideY, strideC int
	depth            int
	bt709, fullRange bool
	release          func()
}

// decoder is a codec fed one sample at a time, giving back pictures in
// presentation order as they become ready.
type decoder interface {
	decode(sample []byte) ([]planes, error)
	flush() ([]planes, error)
}

// newDecoder makes a decoder for the track. alt asks for the second
// decoder a codec has, where it has one: see vp9AltDecoder.
func newDecoder(t *track, alt bool) (decoder, error) {
	if alt && t.codec == "vp9" {
		return &vp9AltDecoder{t: t, dec: vp9alt.New()}, nil
	}
	switch t.codec {
	case "h264":
		d := &avcDecoder{dec: h264.NewDecoderWithConfig(h264.DecoderConfig{ForceSoftware: true}), lengthSize: t.lengthSize}
		for _, p := range t.params {
			d.head = append(append(d.head, 0, 0, 0, 1), p...)
		}
		return d, nil
	case "hevc":
		d := &hevcDecoder{dec: &hevc.Decoder{}, lengthSize: t.lengthSize}
		for _, p := range t.params {
			nal, ok := hevc.ParseNAL(p)
			if !ok {
				return nil, errors.New("the HEVC parameter sets are malformed")
			}
			if _, err := d.dec.DecodeNAL(nal); err != nil {
				return nil, fmt.Errorf("the HEVC parameter sets: %w", err)
			}
		}
		return d, nil
	case "vp8":
		dec, err := govpx.NewVP8Decoder(govpx.DecoderOptions{})
		if err != nil {
			return nil, err
		}
		return &vpxDecoder{t: t, decode8: dec}, nil
	case "vp9":
		dec, err := govpx.NewVP9Decoder(govpx.VP9DecoderOptions{})
		if err != nil {
			return nil, err
		}
		return &vpxDecoder{t: t, decode9: dec}, nil
	case "av1":
		// One frame at a time: runs already decode in parallel, a group of
		// pictures each, and a frame's tiles still spread over cores.
		d := &av1Decoder{t: t, dec: &av1.Decoder{FrameThreads: 1}}
		for _, p := range t.params {
			if _, err := d.dec.DecodeOBUs(p); err != nil {
				return nil, fmt.Errorf("the AV1 configuration: %w", err)
			}
		}
		return d, nil
	}
	return nil, fmt.Errorf("no decoder for %s", t.codec)
}

// bt709 is whether a picture of height h with CICP matrix code m is
// BT.709: when said so, or when unsaid and HD, as players assume.
func bt709(m, h int) bool { return m == 1 || ((m == 0 || m == 2) && h >= 720) }

// vpxDecoder is VP8 or VP9 (profile 0: 8-bit 4:2:0) through govpx, checked
// against libvpx. A VP9 sample holding an alt-ref frame is a superframe
// showing one picture; VP8 stores its alt-refs as samples of their own,
// shown never (sample.hidden).
type vpxDecoder struct {
	t       *track
	decode8 *govpx.VP8Decoder
	decode9 *govpx.VP9Decoder
}

func (d *vpxDecoder) decode(s []byte) ([]planes, error) {
	var out []planes
	next := func() (govpx.Image, bool) { return d.decode8.NextFrame() }
	if d.decode9 != nil {
		if err := d.decode9.Decode(s); err != nil {
			if errors.Is(err, govpx.ErrVP9NotImplemented) {
				return nil, errors.New("VP9 beyond profile 0 (10-bit, or 4:2:2/4:4:4) is not decoded")
			}
			return nil, err
		}
		next = d.decode9.NextFrame
	} else if err := d.decode8.Decode(s); err != nil {
		return nil, err
	}
	for {
		img, ok := next()
		if !ok {
			return out, nil
		}
		out = append(out, planes{
			w: img.Width, h: img.Height, cw: (img.Width + 1) / 2, ch: (img.Height + 1) / 2,
			y: img.Y, cb: img.U, cr: img.V, strideY: img.YStride, strideC: img.UStride, depth: 8,
			bt709: d.decode9 != nil && bt709(d.t.matrix, img.Height), fullRange: d.t.fullRange,
		})
	}
}

func (d *vpxDecoder) flush() ([]planes, error) { return nil, nil }

// vp9AltDecoder is VP9 through mgvs/go-vp9, for a group of pictures govpx
// refused. Neither decoder is right about every stream: on six ordinary
// libvpx encodes govpx was bit-exact wherever it decoded and refused two
// (one-pass and two-pass bitrate-targeted, an entropy desync at one inter
// frame), and go-vp9 decoded those two bit-exact but two others silently
// wrong. So govpx goes first, and its refusal - never a wrong picture -
// is what hands a group over.
type vp9AltDecoder struct {
	t   *track
	dec *vp9alt.Decoder
}

func (d *vp9AltDecoder) decode(s []byte) ([]planes, error) {
	f, err := d.dec.DecodeFrame(s)
	if err != nil || f == nil {
		return nil, err
	}
	b := refbuf.BORDER // on every side of every plane
	return []planes{{
		w: f.Width, h: f.Height, cw: (f.Width + 1) / 2, ch: (f.Height + 1) / 2,
		y: f.Y[b*f.YStride+b:], cb: f.U[b*f.UVStride+b:], cr: f.V[b*f.UVStride+b:],
		strideY: f.YStride, strideC: f.UVStride, depth: 8,
		bt709: bt709(d.t.matrix, f.Height), fullRange: d.t.fullRange,
	}}, nil
}

func (d *vp9AltDecoder) flush() ([]planes, error) { return nil, nil }

// av1Decoder is AV1 through gav1d, a port of dav1d checked bit-exact
// against it: a sample is a temporal unit, showing one picture.
type av1Decoder struct {
	t   *track
	dec *av1.Decoder
}

func (d *av1Decoder) decode(s []byte) ([]planes, error) {
	pics, err := d.dec.DecodeOBUs(s)
	out := make([]planes, 0, len(pics))
	for _, p := range pics {
		out = append(out, av1Planes(p, d.t))
	}
	return out, err
}

func (d *av1Decoder) flush() ([]planes, error) { return nil, nil }

func av1Planes(p *av1.Picture, t *track) planes {
	m := p.MatrixCoeffs
	if m == 2 && t.matrix != 0 {
		m = t.matrix
	}
	pl := planes{
		w: p.Width, h: p.Height, depth: p.BitDepth,
		bt709: bt709(m, p.Height), fullRange: p.FullRange,
		release: p.Release,
	}
	sx, sy := 2, 2
	switch p.Layout {
	case av1.LayoutI422:
		sy = 1
	case av1.LayoutI444:
		sx, sy = 1, 1
	}
	pl.cw, pl.ch = (p.Width+sx-1)/sx, (p.Height+sy-1)/sy
	mono := p.Layout == av1.LayoutI400
	if p.BitDepth <= 8 {
		pl.y, pl.strideY = p.Data[0], p.Stride[0]
		if !mono {
			pl.cb, pl.cr, pl.strideC = p.Data[1], p.Data[2], p.Stride[1]
		}
		return pl
	}
	// Above 8 bits a sample is a native-endian uint16 and strides are in
	// bytes.
	wide := func(b []byte, stride, w, h int) []uint16 {
		out := make([]uint16, w*h)
		for y := 0; y < h; y++ {
			row := b[y*stride:]
			for x := 0; x < w; x++ {
				out[y*w+x] = binary.NativeEndian.Uint16(row[2*x:])
			}
		}
		return out
	}
	pl.y16, pl.strideY = wide(p.Data[0], p.Stride[0], p.Width, p.Height), p.Width
	if !mono {
		pl.cb16 = wide(p.Data[1], p.Stride[1], pl.cw, pl.ch)
		pl.cr16 = wide(p.Data[2], p.Stride[2], pl.cw, pl.ch)
		pl.strideC = pl.cw
	}
	return pl
}

// markHidden finds the VP8 and VP9 samples that are decoded and never
// shown, from each one's first bytes: VP8's frame tag says so outright; a
// VP9 sample is shown if it is a superframe (an alt-ref with the frame
// shown after it) or its header says show_frame or show_existing_frame.
func markHidden(r interface {
	ReadAt([]byte, int64) (int, error)
}, t *track) error {
	if t.codec != "vp8" && t.codec != "vp9" {
		return nil
	}
	var b [1]byte
	for i := range t.samples {
		s := &t.samples[i]
		if s.size == 0 {
			s.hidden = true
			continue
		}
		if _, err := r.ReadAt(b[:], int64(s.offset)); err != nil {
			return err
		}
		if t.codec == "vp8" {
			s.hidden = b[0]&0x10 == 0
			continue
		}
		var last [1]byte
		if _, err := r.ReadAt(last[:], int64(s.offset)+int64(s.size)-1); err != nil {
			return err
		}
		if last[0]&0xE0 == 0xC0 {
			continue // a superframe
		}
		h := b[0]
		if h>>6 != 2 {
			continue // not a frame header this reads; let the decoder say
		}
		profile := (h>>5)&1 | (h>>4)&1<<1
		bits := h << 4 // show_existing_frame first
		if profile == 3 {
			bits <<= 1
		}
		showExisting := bits&0x80 != 0
		showFrame := bits&0x20 != 0
		s.hidden = !showExisting && !showFrame
	}
	return nil
}

// avcDecoder is H.264 through go.264, which reads Annex B: each sample's
// length-prefixed NAL units become start-code prefixed, after the
// parameter sets on the first.
type avcDecoder struct {
	dec        *h264.Decoder
	head       []byte
	lengthSize int
}

func (d *avcDecoder) decode(s []byte) ([]planes, error) {
	au := d.head
	d.head = nil
	for len(s) >= d.lengthSize {
		n := 0
		for _, b := range s[:d.lengthSize] {
			n = n<<8 | int(b)
		}
		s = s[d.lengthSize:]
		if n > len(s) {
			return nil, errors.New("a frame's NAL unit runs past its sample")
		}
		au = append(append(au, 0, 0, 0, 1), s[:n]...)
		s = s[n:]
	}
	frames, err := d.dec.Decode(au)
	return avcPlanes(frames), err
}

func (d *avcDecoder) flush() ([]planes, error) {
	frames, err := d.dec.Flush()
	return avcPlanes(frames), err
}

func avcPlanes(frames []*h264.Frame) []planes {
	out := make([]planes, len(frames))
	for i, f := range frames {
		out[i] = planes{
			w: f.Width, h: f.Height, cw: (f.Width + 1) / 2, ch: (f.Height + 1) / 2,
			y: f.Y, cb: f.Cb, cr: f.Cr, strideY: f.StrideY, strideC: f.StrideC, depth: 8,
			// go.264 does not surface the stream's colour description; HD
			// is BT.709 and SD BT.601, as players assume without one.
			bt709: f.Height >= 720,
		}
	}
	return out
}

// hevcDecoder is HEVC through gen2brain/h265, fed one NAL unit at a time.
type hevcDecoder struct {
	dec        *hevc.Decoder
	lengthSize int
}

func (d *hevcDecoder) decode(s []byte) ([]planes, error) {
	var out []planes
	for len(s) >= d.lengthSize {
		n := 0
		for _, b := range s[:d.lengthSize] {
			n = n<<8 | int(b)
		}
		s = s[d.lengthSize:]
		if n > len(s) {
			return out, errors.New("a frame's NAL unit runs past its sample")
		}
		nal, ok := hevc.ParseNAL(s[:n])
		s = s[n:]
		if !ok {
			continue
		}
		pics, err := d.dec.DecodeNAL(nal)
		out = append(out, hevcPlanes(pics)...)
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

func (d *hevcDecoder) flush() ([]planes, error) { return hevcPlanes(d.dec.Flush()), nil }

func hevcPlanes(pics []*hevc.Picture) []planes {
	out := make([]planes, len(pics))
	for i, p := range pics {
		pl := planes{
			w: p.CropW, h: p.CropH, depth: p.BitDepth,
			strideY: p.StrideY, strideC: p.StrideC,
			bt709: p.ColorMatrix == 1 || (p.ColorMatrix == 2 && p.CropH >= 720), fullRange: p.FullRange,
			release: p.Release,
		}
		// Chroma subsampling, from the planes' sizes: 4:2:0, 4:2:2 or 4:4:4.
		sx, sy := 1, 1
		if p.WidthC > 0 && p.WidthC < p.Width {
			sx = 2
		}
		if p.HeightC > 0 && p.HeightC < p.Height {
			sy = 2
		}
		pl.cw, pl.ch = (p.CropW+sx-1)/sx, (p.CropH+sy-1)/sy
		cx, cy := p.CropX/sx, p.CropY/sy
		if p.BitDepth > 8 || p.Y == nil {
			pl.y16 = p.Y16[p.CropY*p.StrideY+p.CropX:]
			if p.Cb16 != nil {
				pl.cb16 = p.Cb16[cy*p.StrideC+cx:]
				pl.cr16 = p.Cr16[cy*p.StrideC+cx:]
			}
		} else {
			pl.y = p.Y[p.CropY*p.StrideY+p.CropX:]
			if p.Cb != nil {
				pl.cb = p.Cb[cy*p.StrideC+cx:]
				pl.cr = p.Cr[cy*p.StrideC+cx:]
			}
		}
		out[i] = pl
	}
	return out
}
