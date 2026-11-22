// Package vision prepares pictures and videos for d1's vision encoder, as
// the checkpoint's processor does (image_processing_lfm3_vl.py,
// video_processing_lfm3_vl.py, media.py), in pure Go:
//
//   - a picture is capped at the checkpoint's pixel budget (bicubic);
//   - resized so both sides are multiples of 32 within the processor's
//     bounds (smart_resize), bicubic with antialiasing;
//   - scaled to [-1, 1] (rescale by 1/255, mean and std 0.5);
//   - cut into 16x16 patches, each still picture repeated over the two
//     temporal slots, laid out block-major as the encoder takes them:
//     [rows/2, cols/2, 2, 2, 3*2*16*16].
//
// The resampler is PIL's, in its fixed-point arithmetic, horizontal pass
// then vertical with 8-bit rounding between: torchvision's uint8 antialias
// kernel is built to match PIL, and the reference's pictures go through
// both. Each encoder call yields (rows/2)*(cols/2) feature rows, one image
// token each, and the markup the prompt places them with.
package vision

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"  // decoders a caller's pictures may need
	_ "image/jpeg" // decoders a caller's pictures may need
	_ "image/png"  // decoders a caller's pictures may need
	"math"
	"strings"

	_ "golang.org/x/image/bmp"  // decoders a caller's pictures may need
	_ "golang.org/x/image/webp" // decoders a caller's pictures may need
)

const (
	patch    = 16
	merge    = 2
	temporal = 2
	factor   = patch * merge
	channels = 3
	// PatchDim is one patch's width: channels x temporal x 16 x 16.
	PatchDim = channels * temporal * patch * patch

	imageMinPixels = 65536
	imageMaxPixels = 16777216
	videoMinPixels = 4096
	// VideoMaxPixels is the pixels a clip's sampled frames are resized to in
	// all - the processor's size.longest_edge - and so its tokens: one per
	// 32x32 pixels of each pair of frames, 4096 here. The checkpoint's own
	// is 25165824, about 12k tokens, and the decoder's attention over that
	// many asks for 9.5 GB at once: measured on a 16 GB GPU, ten seconds of
	// 1080p failed to allocate. A third of it fits beside the model on such
	// a card and in a few GB of RAM, and a clip under it - most clips below
	// HD, or a few seconds of HD - is read exactly as the checkpoint would.
	VideoMaxPixels = 8388608
	videoFPS       = 2.0
	minFrames      = 4
	maxFrames      = 768

	// DefaultMaxPixels is d1-2B's picture budget (d1_vision_max_pixels).
	DefaultMaxPixels = 1638400
)

// Picture is an RGB raster, 8 bits a channel, row-major.
type Picture struct {
	W, H int
	Pix  []uint8 // len W*H*3
}

// Decode reads a PNG, JPEG, GIF, WebP or BMP as RGB. Alpha is dropped, not
// composited, as PIL's convert("RGB") does.
func Decode(data []byte) (*Picture, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decoding picture: %w", err)
	}
	return FromImage(img), nil
}

// FromImage converts any image to RGB.
func FromImage(img image.Image) *Picture {
	b := img.Bounds()
	p := &Picture{W: b.Dx(), H: b.Dy(), Pix: make([]uint8, b.Dx()*b.Dy()*3)}
	i := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			p.Pix[i], p.Pix[i+1], p.Pix[i+2] = c.R, c.G, c.B
			i += 3
		}
	}
	return p
}

// --- PIL's bicubic resampling ---

const precisionBits = 32 - 8 - 2

func bicubic(x float64) float64 {
	const a = -0.5
	if x < 0 {
		x = -x
	}
	if x < 1 {
		return ((a+2)*x-(a+3))*x*x + 1
	}
	if x < 2 {
		return (((x-5)*x+8)*x - 4) * a
	}
	return 0
}

type coeffs struct {
	bounds    []int // per output: xmin, count
	kk        []int32
	ksize     int
	precision int
}

// precompute is PIL's precompute_coeffs with the fixed point of its
// normalize_coeffs_8bpc (int16Weights false: 22 bits, int32), or of
// torchvision's uint8 antialias kernel (true: int16 weights at the largest
// precision the axis's biggest weight fits, as Pillow-SIMD does). The two
// round a few upscaled pixels apart; the processor uses torchvision's and
// media.py's picture cap PIL's.
func precompute(inSize, outSize int, int16Weights bool) coeffs {
	scale := float64(inSize) / float64(outSize)
	filterscale := math.Max(scale, 1)
	support := 2 * filterscale
	ksize := int(math.Ceil(support))*2 + 1
	c := coeffs{bounds: make([]int, outSize*2), kk: make([]int32, outSize*ksize), ksize: ksize}
	k := make([]float64, ksize)
	all := make([]float64, outSize*ksize)
	for xx := 0; xx < outSize; xx++ {
		center := (float64(xx) + 0.5) * scale
		ww := 0.0
		ss := 1 / filterscale
		xmin := int(center - support + 0.5)
		if xmin < 0 {
			xmin = 0
		}
		xmax := int(center + support + 0.5)
		if xmax > inSize {
			xmax = inSize
		}
		xmax -= xmin
		for x := 0; x < xmax; x++ {
			w := bicubic((float64(x+xmin) - center + 0.5) * ss)
			k[x] = w
			ww += w
		}
		for x := 0; x < xmax; x++ {
			if ww != 0 {
				k[x] /= ww
			}
		}
		for x := 0; x < xmax; x++ {
			all[xx*ksize+x] = k[x]
		}
		c.bounds[xx*2], c.bounds[xx*2+1] = xmin, xmax
	}
	c.precision = precisionBits
	if int16Weights {
		maxWeight := 0.0
		for _, w := range all {
			maxWeight = math.Max(maxWeight, w)
		}
		p := 0
		for ; p < 22; p++ {
			if int(0.5+maxWeight*float64(int(1)<<(p+1))) >= 1<<15 {
				break
			}
		}
		c.precision = p
	}
	for i, v := range all {
		if v < 0 {
			c.kk[i] = int32(-0.5 + v*float64(int64(1)<<c.precision))
		} else {
			c.kk[i] = int32(0.5 + v*float64(int64(1)<<c.precision))
		}
	}
	return c
}

func clip8(v int64, precision int) uint8 {
	v >>= precision
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// Resize resamples a picture to w x h as PIL's Image.resize with BICUBIC
// does.
func (p *Picture) Resize(w, h int) *Picture { return p.resize(w, h, false) }

// ResizeTorch resamples as torchvision's resize of a uint8 tensor with
// bicubic antialiasing does: the processor's resize.
func (p *Picture) ResizeTorch(w, h int) *Picture { return p.resize(w, h, true) }

func (p *Picture) resize(w, h int, int16Weights bool) *Picture {
	if w == p.W && h == p.H {
		return p
	}
	cur := p
	if w != cur.W {
		c := precompute(cur.W, w, int16Weights)
		out := &Picture{W: w, H: cur.H, Pix: make([]uint8, w*cur.H*3)}
		for y := 0; y < cur.H; y++ {
			row := cur.Pix[y*cur.W*3:]
			for xx := 0; xx < w; xx++ {
				xmin, n := c.bounds[xx*2], c.bounds[xx*2+1]
				kk := c.kk[xx*c.ksize:]
				for ch := 0; ch < 3; ch++ {
					ss := int64(1) << (c.precision - 1)
					for x := 0; x < n; x++ {
						ss += int64(row[(x+xmin)*3+ch]) * int64(kk[x])
					}
					out.Pix[(y*w+xx)*3+ch] = clip8(ss, c.precision)
				}
			}
		}
		cur = out
	}
	if h != cur.H {
		c := precompute(cur.H, h, int16Weights)
		out := &Picture{W: cur.W, H: h, Pix: make([]uint8, cur.W*h*3)}
		for yy := 0; yy < h; yy++ {
			ymin, n := c.bounds[yy*2], c.bounds[yy*2+1]
			kk := c.kk[yy*c.ksize:]
			for x := 0; x < cur.W; x++ {
				for ch := 0; ch < 3; ch++ {
					ss := int64(1) << (c.precision - 1)
					for y := 0; y < n; y++ {
						ss += int64(cur.Pix[((y+ymin)*cur.W+x)*3+ch]) * int64(kk[y])
					}
					out.Pix[(yy*cur.W+x)*3+ch] = clip8(ss, c.precision)
				}
			}
		}
		cur = out
	}
	return cur
}

// CapPixels downscales a picture to at most maxPixels, as media.py's
// cap_pixels does.
func (p *Picture) CapPixels(maxPixels int) *Picture {
	if p.W*p.H <= maxPixels {
		return p
	}
	scale := math.Sqrt(float64(maxPixels) / float64(p.W*p.H))
	return p.Resize(max(1, int(float64(p.W)*scale)), max(1, int(float64(p.H)*scale)))
}

func pyRound(x float64) float64 { return math.RoundToEven(x) }

// SmartResize is the processor's target size: both sides multiples of
// factor, the area within [minPixels, maxPixels], the aspect kept as
// closely as that allows.
func SmartResize(height, width, minPixels, maxPixels int) (int, int, error) {
	if float64(max(height, width))/float64(min(height, width)) > 200 {
		return 0, 0, fmt.Errorf("absolute aspect ratio must be smaller than 200, got %g", float64(max(height, width))/float64(min(height, width)))
	}
	hBar := int(pyRound(float64(height)/factor)) * factor
	wBar := int(pyRound(float64(width)/factor)) * factor
	if hBar*wBar > maxPixels {
		beta := math.Sqrt(float64(height*width) / float64(maxPixels))
		hBar = max(factor, int(math.Floor(float64(height)/beta/factor))*factor)
		wBar = max(factor, int(math.Floor(float64(width)/beta/factor))*factor)
	} else if hBar*wBar < minPixels {
		beta := math.Sqrt(float64(minPixels) / float64(height*width))
		hBar = int(math.Ceil(float64(height)*beta/factor)) * factor
		wBar = int(math.Ceil(float64(width)*beta/factor)) * factor
	}
	return hBar, wBar, nil
}

// videoSmartResize is the video processor's smart_resize, over all frames.
func videoSmartResize(frames, height, width int) (int, int, error) {
	if frames < temporal {
		return 0, 0, fmt.Errorf("a video needs %d frames or more, got %d", temporal, frames)
	}
	if height < factor || width < factor {
		scale := math.Max(float64(factor)/float64(height), float64(factor)/float64(width))
		height = int(float64(height) * scale)
		width = int(float64(width) * scale)
	}
	if float64(max(height, width))/float64(min(height, width)) > 200 {
		return 0, 0, fmt.Errorf("absolute aspect ratio must be smaller than 200")
	}
	hBar := int(pyRound(float64(height)/factor)) * factor
	wBar := int(pyRound(float64(width)/factor)) * factor
	tBar := int(pyRound(float64(frames)/temporal)) * temporal
	if tBar*hBar*wBar > VideoMaxPixels {
		beta := math.Sqrt(float64(frames*height*width) / VideoMaxPixels)
		hBar = max(factor, int(math.Floor(float64(height)/beta/factor))*factor)
		wBar = max(factor, int(math.Floor(float64(width)/beta/factor))*factor)
	} else if tBar*hBar*wBar < videoMinPixels {
		beta := math.Sqrt(videoMinPixels / float64(frames*height*width))
		hBar = int(math.Ceil(float64(height)*beta/factor)) * factor
		wBar = int(math.Ceil(float64(width)*beta/factor)) * factor
	}
	return hBar, wBar, nil
}

// normalized is a pixel scaled to [-1, 1] in float32 exactly as the
// processor computes it: transformers fuses the rescale into the mean and
// std (mean 0.5*255, std 0.5*255), so it is (x - 127.5) / 127.5. Rescaling
// first, (x/255 - 0.5)/0.5, lands one ulp away on a quarter of the pixels,
// and the decoder's int8-quantized matmuls turn that into answers a
// percent apart.
func normalized(v uint8) float32 {
	return (float32(v) - 127.5) / 127.5
}

// Input is one vision encoder call: pixel_values of shape
// [GH, GW, 2, 2, PatchDim], which yields GH*GW feature rows.
type Input struct {
	GH, GW int
	Pixels []float32
}

// Tokens is how many feature rows (placeholder tokens) the call yields.
func (in Input) Tokens() int { return in.GH * in.GW }

// patchify lays out two frames (a still picture passes itself twice) as the
// encoder takes them: block row, block column, the 2x2 merge, then each
// patch's channel, temporal slot, row and column.
func patchify(frames [temporal]*Picture) Input {
	w, h := frames[0].W, frames[0].H
	gh, gw := h/patch, w/patch
	in := Input{GH: gh / merge, GW: gw / merge, Pixels: make([]float32, gh*gw*PatchDim)}
	i := 0
	for bh := 0; bh < gh/merge; bh++ {
		for bw := 0; bw < gw/merge; bw++ {
			for mh := 0; mh < merge; mh++ {
				for mw := 0; mw < merge; mw++ {
					py0 := (bh*merge + mh) * patch
					px0 := (bw*merge + mw) * patch
					for ch := 0; ch < channels; ch++ {
						for t := 0; t < temporal; t++ {
							f := frames[t]
							for y := 0; y < patch; y++ {
								row := ((py0+y)*w + px0) * 3
								for x := 0; x < patch; x++ {
									in.Pixels[i] = normalized(f.Pix[row+x*3+ch])
									i++
								}
							}
						}
					}
				}
			}
		}
	}
	return in
}

// Media is one prepared picture or video: its encoder calls and the markup
// that stands for them in the prompt.
type Media struct {
	Video  bool
	Inputs []Input
	Markup string
}

// Tokens is how many placeholder tokens the media's markup holds.
func (m Media) Tokens() int {
	n := 0
	for _, in := range m.Inputs {
		n += in.Tokens()
	}
	return n
}

// Image prepares a picture: capped at maxPixels, resized, patched.
func Image(p *Picture, maxPixels int, imageToken string) (Media, error) {
	if maxPixels <= 0 {
		maxPixels = DefaultMaxPixels
	}
	p = p.CapPixels(maxPixels)
	h, w, err := SmartResize(p.H, p.W, imageMinPixels, imageMaxPixels)
	if err != nil {
		return Media{}, err
	}
	p = p.ResizeTorch(w, h)
	in := patchify([temporal]*Picture{p, p})
	return Media{
		Inputs: []Input{in},
		Markup: "<|vision_start|>" + strings.Repeat(imageToken, in.Tokens()) + "<|vision_end|>",
	}, nil
}

// SampleFrames picks the frames a clip of total frames at fps is read at:
// two a second, at least four and at most 768, evenly spaced.
func SampleFrames(total int, fps float64) []int {
	n := int(float64(total) / fps * videoFPS)
	n = min(max(n, minFrames), maxFrames, total)
	out := make([]int, n)
	for i := range out {
		if n == 1 {
			out[i] = 0
			continue
		}
		out[i] = int(pyRound(float64(i) * float64(total-1) / float64(n-1)))
	}
	return out
}

// Video prepares a clip's frames, all the same size, recorded at fps.
func Video(frames []*Picture, fps float64, videoToken string) (Media, error) {
	if len(frames) == 0 {
		return Media{}, fmt.Errorf("a video has no frames")
	}
	if !(fps > 0) || math.IsInf(fps, 0) {
		return Media{}, fmt.Errorf("a video's fps must be a positive number, got %g", fps)
	}
	for _, f := range frames[1:] {
		if f.W != frames[0].W || f.H != frames[0].H {
			return Media{}, fmt.Errorf("a video's frames must share one size: %dx%d and %dx%d", frames[0].W, frames[0].H, f.W, f.H)
		}
	}
	indices := SampleFrames(len(frames), fps)
	h, w, err := VideoSize(len(indices), frames[0].H, frames[0].W)
	if err != nil {
		return Media{}, err
	}
	sampled := make([]*Picture, len(indices))
	times := make([]float64, len(indices))
	for i, idx := range indices {
		sampled[i] = frames[idx].ResizeTorch(w, h)
		times[i] = float64(idx) / fps
	}
	return VideoAt(sampled, times, videoToken)
}

// VideoSize is the size the processor resizes each of a clip's n sampled
// frames to, from the frames' own height and width: smart_resize over the
// whole clip within VideoMaxPixels, so a longer clip's frames are smaller. A caller decoding a
// video file resizes each frame to it as it is decoded, rather than holding
// hundreds of full-size frames to resize later.
func VideoSize(n, height, width int) (h, w int, err error) {
	return videoSmartResize(n, height, width)
}

// VideoAt prepares frames already sampled from a clip and resized by
// VideoSize, each with its time in seconds from the start of the clip.
func VideoAt(frames []*Picture, times []float64, videoToken string) (Media, error) {
	if len(frames) == 0 {
		return Media{}, fmt.Errorf("a video has no frames")
	}
	if len(times) != len(frames) {
		return Media{}, fmt.Errorf("a video has %d frames and %d times", len(frames), len(times))
	}
	for _, f := range frames {
		if f.W != frames[0].W || f.H != frames[0].H {
			return Media{}, fmt.Errorf("a video's frames must share one size: %dx%d and %dx%d", frames[0].W, frames[0].H, f.W, f.H)
		}
	}
	if frames[0].W%factor != 0 || frames[0].H%factor != 0 {
		return Media{}, fmt.Errorf("a sampled video's frames must be sized by VideoSize, got %dx%d", frames[0].W, frames[0].H)
	}
	sampled := append([]*Picture(nil), frames...)
	stamps := append([]float64(nil), times...)
	for len(sampled)%temporal != 0 {
		sampled = append(sampled, sampled[len(sampled)-1])
		stamps = append(stamps, stamps[len(stamps)-1])
	}
	m := Media{Video: true}
	var b strings.Builder
	b.WriteString("<|vision_start|>")
	for t := 0; t < len(sampled); t += temporal {
		in := patchify([temporal]*Picture{sampled[t], sampled[t+1]})
		m.Inputs = append(m.Inputs, in)
		at := (stamps[t] + stamps[t+1]) / 2
		fmt.Fprintf(&b, "<%.1f seconds>", at)
		b.WriteString("<|vision_start|>" + strings.Repeat(videoToken, in.Tokens()) + "<|vision_end|>")
	}
	b.WriteString("<|vision_end|>")
	m.Markup = b.String()
	return m, nil
}
