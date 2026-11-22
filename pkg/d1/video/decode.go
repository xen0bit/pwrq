package video

import (
	"errors"
	"fmt"

	"github.com/gen2brain/h265/hevc"
	h264 "github.com/oops1/go.264"
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

func newDecoder(t *track) (decoder, error) {
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
	}
	return nil, fmt.Errorf("no decoder for %s", t.codec)
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
