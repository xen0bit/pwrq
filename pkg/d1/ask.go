package d1

import (
	"context"
	"fmt"
	"time"

	"github.com/xen0bit/pwrq/pkg/d1/ort"
	"github.com/xen0bit/pwrq/pkg/d1/prompt"
	"github.com/xen0bit/pwrq/pkg/d1/vision"
)

// Media is one picture or video of a request.
type Media struct {
	Picture *vision.Picture   // a picture
	Frames  []*vision.Picture // a video's frames, all one size
	FPS     float64           // the frames' rate
	// Times, when set, says Frames are already the sample the model reads,
	// resized by vision.VideoSize - as pkg/d1/video decodes a file - each at
	// this many seconds into the clip; FPS is then unused.
	Times []float64
}

// Request is a state, its questions, and its media.
type Request struct {
	State     any // text, a JSON value, or nil with media alone
	Questions []prompt.Named
	Media     []Media
	// SetSize is how many questions are read together after the state: the
	// model's trained size (16) when 0. Larger is faster and costs some
	// accuracy on text.
	SetSize int
	Answer  prompt.AnswerOptions
	// Uncalibrated skips calibration.json: option scores without the
	// release's temperature, typed answers without its tables.
	Uncalibrated bool
}

// Response is a request's answers and what it cost.
type Response struct {
	Answers     map[string]any
	InputTokens int
	Seconds     float64
}

// MaxRowTokens bounds one packed row. The model reads 128k positions, the
// card says answers grow less reliable past 64k of state, and a CPU forward
// pass over that many takes minutes: past it the request is refused with
// what to change rather than left to run.
const MaxRowTokens = 65536

// Ask answers a request in one forward pass a row (a state and up to 16 of
// its questions), plus one vision encoder pass a picture and per two video
// frames.
func (m *Model) Ask(ctx context.Context, req Request) (*Response, error) {
	start := time.Now()
	if len(req.Questions) == 0 {
		return nil, fmt.Errorf("no questions")
	}
	if req.State == nil && len(req.Media) == 0 {
		return nil, fmt.Errorf("a request needs a state, media, or both")
	}
	images := 0
	for _, md := range req.Media {
		if md.Picture != nil {
			images++
		}
	}
	if prompt.NeedsOneImage(req.Questions) && images != 1 {
		return nil, fmt.Errorf("a box or boxes question reads exactly one picture in Media; this request has %d", images)
	}
	cfg := m.builder.Config()

	// Media first: their markup heads every row, their features fill its
	// placeholders, pictures and videos each in their order.
	var markup string
	var imageInputs, videoInputs []vision.Input
	for i, md := range req.Media {
		var vm vision.Media
		var err error
		switch {
		case md.Picture != nil:
			vm, err = vision.Image(md.Picture, cfg.VisionMax, cfg.ImageToken)
		case len(md.Frames) > 0 && md.Times != nil:
			vm, err = vision.VideoAt(md.Frames, md.Times, cfg.VideoToken)
		case len(md.Frames) > 0:
			vm, err = vision.Video(md.Frames, md.FPS, cfg.VideoToken)
		default:
			err = fmt.Errorf("empty media item")
		}
		if err != nil {
			return nil, fmt.Errorf("media %d: %w", i, err)
		}
		markup += vm.Markup
		if vm.Video {
			videoInputs = append(videoInputs, vm.Inputs...)
		} else {
			imageInputs = append(imageInputs, vm.Inputs...)
		}
	}
	rows, err := m.builder.Rows(req.State, req.Questions, markup, req.SetSize)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if len(r.IDs) > MaxRowTokens {
			return nil, fmt.Errorf("the state and its questions are %d tokens; d1 reads at most %d in one pass (trim the state, or ask fewer questions at once with SetSize)", len(r.IDs), MaxRowTokens)
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dec == nil {
		return nil, fmt.Errorf("the model is closed")
	}
	setStatus(Status{State: StateBusy, Device: m.info.Device})
	defer setStatus(Status{State: StateReady, Device: m.info.Device})

	imageFeatures, err := m.features(ctx, imageInputs)
	if err != nil {
		return nil, err
	}
	videoFeatures, err := m.features(ctx, videoInputs)
	if err != nil {
		return nil, err
	}
	outs := make([]prompt.RowOutput, len(rows))
	for i, row := range rows {
		outs[i], err = m.forward(ctx, row, imageFeatures, videoFeatures)
		if err != nil {
			return nil, err
		}
	}
	cal := m.cal
	if req.Uncalibrated {
		cal = nil
	}
	answers, err := m.builder.Answers(req.State, req.Questions, rows, outs, cal, req.Answer)
	if err != nil {
		return nil, err
	}
	return &Response{Answers: answers, InputTokens: prompt.Tokens(rows), Seconds: time.Since(start).Seconds()}, nil
}

// features runs the vision encoder over each input, concatenating rows.
func (m *Model) features(ctx context.Context, inputs []vision.Input) (ort.Tensor, error) {
	var all []float32
	n := 0
	for _, in := range inputs {
		out, err := m.vis.Run(ctx, map[string]ort.Tensor{
			"pixel_values": ort.F32Tensor(in.Pixels, int64(in.GH), int64(in.GW), 2, 2, vision.PatchDim),
		}, []string{"image_features"})
		if err != nil {
			return ort.Tensor{}, fmt.Errorf("vision encoder: %w", err)
		}
		f := out["image_features"]
		if len(f.Shape) != 2 || f.Shape[0] != int64(in.Tokens()) {
			return ort.Tensor{}, fmt.Errorf("vision encoder returned %v for %d tokens", f.Shape, in.Tokens())
		}
		all = append(all, f.F32...)
		n += int(f.Shape[0])
	}
	if all == nil {
		all = []float32{}
	}
	return ort.F32Tensor(all, int64(n), hiddenSize), nil
}

const hiddenSize = 2048

// forward runs the decoder over one row.
func (m *Model) forward(ctx context.Context, row prompt.Row, images, videos ort.Tensor) (prompt.RowOutput, error) {
	t := int64(len(row.IDs))
	ids := make([]int64, t)
	mask := make([]int64, t)
	pos := make([]int64, t)
	for i, id := range row.IDs {
		ids[i], mask[i], pos[i] = int64(id), 1, int64(i)
	}
	slots := row.SlotPositions()
	outputs := []string{"logits"}
	if row.NeedsHidden() {
		outputs = append(outputs, "hidden_states")
	}
	out, err := m.dec.Run(ctx, map[string]ort.Tensor{
		"input_ids":      ort.I64Tensor(ids, 1, t),
		"attention_mask": ort.I64Tensor(mask, 1, t),
		"position_ids":   ort.I64Tensor(pos, 1, t),
		"question_ids":   ort.I64Tensor(row.QuestionIDs, 1, t),
		"slot_positions": ort.I64Tensor(slots, int64(len(slots))),
		"image_features": images,
		"video_features": videos,
	}, outputs)
	if err != nil {
		return prompt.RowOutput{}, fmt.Errorf("decoder: %w", err)
	}
	logits := out["logits"]
	if len(logits.Shape) != 2 || logits.Shape[0] != int64(len(slots)) {
		return prompt.RowOutput{}, fmt.Errorf("decoder returned logits %v for %d slots", logits.Shape, len(slots))
	}
	v := int(logits.Shape[1])
	ro := prompt.RowOutput{Logits: make([][]float32, len(slots))}
	for i := range ro.Logits {
		ro.Logits[i] = logits.F32[i*v : (i+1)*v]
	}
	if h, ok := out["hidden_states"]; ok {
		d := int(h.Shape[len(h.Shape)-1])
		ro.Hidden = make([][]float32, t)
		for i := range ro.Hidden {
			ro.Hidden[i] = h.F32[i*d : (i+1)*d]
		}
	}
	return ro, nil
}
