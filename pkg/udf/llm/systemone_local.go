package llm

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/xen0bit/pwrq/pkg/core/typed"
	"github.com/xen0bit/pwrq/pkg/d1"
	"github.com/xen0bit/pwrq/pkg/d1/assets"
	"github.com/xen0bit/pwrq/pkg/d1/prompt"
	"github.com/xen0bit/pwrq/pkg/d1/video"
	"github.com/xen0bit/pwrq/pkg/d1/vision"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// The local backend: d1-2B, fetched and cached on first use, answering in
// this process. It is the default; the HTTP backend (TypeSafe, llama.cpp's
// /v1/systemone) is chosen by naming it - {Backend: "remote"}, a Model, a
// BaseUrl, or the TypeSafe and System One environment variables.

// EnvSystemOneBackend picks the backend when no option does: local or remote.
const EnvSystemOneBackend = "PWRQ_SYSTEMONE_BACKEND"

const (
	backendLocal  = "local"
	backendRemote = "remote"
	// localModel names the local model in reports.
	localModel = "LiquidAI/d1-2B"
)

// D1Log, when set, receives the local model's progress lines (downloads,
// the device chosen and why). The CLI points it at stderr; a front end that
// owns the screen leaves it nil.
var D1Log func(format string, args ...any)

// localOptionNames are the options only the local backend reads, lowercased;
// remoteOptionNames only the remote one.
var (
	localOptionNames  = []string{"media", "device", "modeldir", "threads", "setsize", "scorereadout", "abstainbelow", "uncalibrated"}
	remoteOptionNames = []string{"model", "apikey", "baseurl", "timeout", "retries", "maxcalls", "priceinput", "priceoutput"}
)

func hasAny(opts map[string]any, names []string) string {
	for k := range opts {
		for _, n := range names {
			if strings.EqualFold(k, n) {
				return k
			}
		}
	}
	return ""
}

// chooseBackend decides where a call goes. An explicit Backend wins; then
// anything that only makes sense remotely - a model name, a base URL, a key
// - routes remote, so a configuration written for TypeSafe or llama.cpp
// keeps working; otherwise the call runs locally.
func chooseBackend(op string, o options, opts map[string]any) (string, error) {
	backend := strings.ToLower(o.Backend)
	if backend == "" {
		backend = strings.ToLower(os.Getenv(EnvSystemOneBackend))
	}
	switch backend {
	case backendLocal, backendRemote:
	case "":
		backend = backendLocal
		if hasAny(opts, []string{"model", "baseurl", "apikey"}) != "" ||
			os.Getenv(EnvSystemOneModel) != "" || os.Getenv(EnvTypeSafeBase) != "" ||
			strings.HasPrefix(os.Getenv(EnvModel), "systemone/") {
			backend = backendRemote
		}
	default:
		return "", fmt.Errorf("%s: Backend %q is not local or remote", op, o.Backend)
	}
	if backend == backendLocal {
		if k := hasAny(opts, remoteOptionNames); k != "" {
			return "", fmt.Errorf("%s: option %q belongs to the remote backend; the local d1-2B takes Device, ModelDir, Threads, Media, SetSize, ScoreReadout, AbstainBelow, Uncalibrated, Cache and CacheDir (or pass {Backend: \"remote\"})", op, k)
		}
	} else if k := hasAny(opts, localOptionNames); k != "" {
		return "", fmt.Errorf("%s: option %q belongs to the local d1-2B backend, not the remote one", op, k)
	}
	return backend, nil
}

// d1Options are the local model's options: the environment's, then the
// call's.
func d1Options(o options) d1.Options {
	do := d1.OptionsFromEnv()
	if o.Device != "" {
		do.Device = o.Device
	}
	if o.ModelDir != "" {
		do.ModelDir = o.ModelDir
	}
	if o.Threads > 0 {
		do.Threads = o.Threads
	}
	do.Log = D1Log
	return do
}

// localRequest is a parsed local call.
type localRequest struct {
	state     any
	questions []prompt.Named
	media     []d1.Media
	digest    []string // the media's sha256s, for the cache key
	raw       any      // the questions as given, for the cache key
	seg       *segmented
}

// close releases a video read in segments.
func (r *localRequest) close() {
	if r != nil && r.seg != nil {
		r.seg.close()
	}
}

// parseLocal checks a call's state, questions and media.
func parseLocal(op string, state any, rawQuestions any, o options) (*localRequest, error) {
	questions, err := prompt.ParseQuestions(rawQuestions)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	media, digest, seg, err := parseMedia(op, o.Media)
	if err != nil {
		return nil, err
	}
	req := &localRequest{media: media, digest: digest, seg: seg}
	fail := func(err error) (*localRequest, error) {
		req.close()
		return nil, err
	}
	switch s := state.(type) {
	case nil:
		if len(media) == 0 {
			return fail(fmt.Errorf("%s: the state is null and there is no Media; pass text, a document, or {Media: [...]}", op))
		}
	case string:
		if strings.TrimSpace(s) == "" && len(media) == 0 {
			return fail(fmt.Errorf("%s: state is empty", op))
		}
	case map[string]any, []any:
	default:
		return fail(fmt.Errorf("%s: state must be text, an object, an array, or null with Media, got %s; it comes from the pipeline, or the first of three arguments", op, jsonType(state)))
	}
	req.state, req.questions, req.raw = state, questions, rawQuestions
	return req, nil
}

// parseMedia reads the Media option: each item a picture's or a video
// file's path, a get_childitem object, {Type: "image", Path | Data},
// {Type: "video", Path, Start, End} for a video file, or {Type: "video",
// Frames: [path | Data...], FPS} for frames extracted already. Data is
// base64. A path is a video when it is an MP4 or QuickTime file (by its
// extension, or its first box when the extension says nothing).
func parseMedia(op string, items []any) (out []d1.Media, digest []string, seg *segmented, err error) {
	defer func() {
		if err != nil && seg != nil {
			seg.close()
			seg = nil
		}
	}()
	for i, item := range items {
		fail := func(err error) ([]d1.Media, []string, *segmented, error) {
			return nil, nil, seg, fmt.Errorf("%s: Media[%d]: %w", op, i, err)
		}
		obj, isObj := common.BindValue(item).(map[string]any)
		kind := ""
		if isObj {
			if t, ok := field(obj, "Type").(string); ok && t != "" {
				kind = strings.ToLower(t)
			}
		}
		path, hasPath := mediaPath(item)
		if kind == "" {
			kind = "image"
			if hasPath && video.IsVideo(path) {
				kind = "video"
			} else if hasPath && otherVideo[strings.ToLower(filepath.Ext(path))] {
				return fail(fmt.Errorf("%s is a video pwrq does not decode (it reads MP4, QuickTime, Matroska and WebM with H.264, HEVC, VP8, VP9 or AV1); pass its frames instead, {Type: \"video\", Frames: [...], FPS: n} - get_help(\"invoke_systemone\") shows how with ffmpeg", filepath.Base(path)))
			}
		}
		switch kind {
		case "image":
			raw, err := mediaBytes(item)
			if err != nil {
				return fail(err)
			}
			p, err := vision.Decode(raw)
			if err != nil {
				return fail(err)
			}
			out = append(out, d1.Media{Picture: p})
			digest = append(digest, sha(raw))
		case "video":
			var segment float64
			if isObj {
				if v := field(obj, "Segment"); v != nil {
					var ok bool
					if segment, ok = optFloat(v); !ok || segment < 1 {
						return fail(errors.New("a Segment is the seconds of video read at a time: 1 or more"))
					}
					if seg != nil {
						return fail(fmt.Errorf("one video a call is read in segments"))
					}
				}
			}
			frames, _ := field(obj, "Frames").([]any)
			if len(frames) == 0 {
				if !hasPath {
					return fail(fmt.Errorf(`a video is a file's path, {Type: "video", Path, Start, End}, or {Type: "video", Frames: [paths or base64 pictures], FPS: n}`))
				}
				var start, end float64
				if isObj {
					start, _ = optFloat(field(obj, "Start"))
					end, _ = optFloat(field(obj, "End"))
				}
				if segment > 0 {
					s, d, err := segmentFile(path, start, end, segment)
					if err != nil {
						return fail(err)
					}
					seg, s.at = s, len(out)
					out = append(out, d1.Media{}) // each segment's frames in turn
					digest = append(digest, d)
					continue
				}
				m, d, err := videoFile(path, start, end)
				if err != nil {
					return fail(err)
				}
				out = append(out, m)
				digest = append(digest, d)
				continue
			}
			fps, ok := optFloat(field(obj, "FPS"))
			if !ok || fps <= 0 {
				return fail(fmt.Errorf("a video's frames need their frame rate, FPS, above 0"))
			}
			m := d1.Media{FPS: fps}
			h := sha256.New()
			for k, f := range frames {
				raw, err := mediaBytes(f)
				if err != nil {
					return fail(fmt.Errorf("frame %d: %w", k, err))
				}
				p, err := vision.Decode(raw)
				if err != nil {
					return fail(fmt.Errorf("frame %d: %w", k, err))
				}
				m.Frames = append(m.Frames, p)
				h.Write(raw)
			}
			if segment > 0 {
				seg = segmentFrames(m.Frames, fps, segment)
				seg.at = len(out)
				out = append(out, d1.Media{})
				digest = append(digest, fmt.Sprintf("video-segments:%g:%g:%x", fps, segment, h.Sum(nil)))
				continue
			}
			out = append(out, m)
			digest = append(digest, fmt.Sprintf("video:%g:%x", fps, h.Sum(nil)))
		case "audio":
			return fail(fmt.Errorf("d1-2B reads pictures and video; audio needs d1-Omni, which pwrq does not ship"))
		default:
			return fail(fmt.Errorf("media type %q is not image or video", kind))
		}
	}
	return out, digest, seg, nil
}

// otherVideo are video containers pwrq does not decode, refused by name
// rather than as a picture that would not decode.
var otherVideo = map[string]bool{".avi": true, ".wmv": true, ".flv": true,
	".mpg": true, ".mpeg": true, ".ts": true, ".m2ts": true, ".mts": true, ".ogv": true}

// videoFile reads a video file as d1 reads video: two frames a second
// between start and end seconds (end 0: to the end), at least four and at
// most 768, decoded in Go. Its cache key is the frames read.
func videoFile(path string, start, end float64) (d1.Media, string, error) {
	c, err := video.Open(filepath.Clean(path))
	if err != nil {
		return d1.Media{}, "", err
	}
	defer func() { _ = c.Close() }()
	frames, err := c.Sample(start, end)
	if err != nil {
		return d1.Media{}, "", fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	m := d1.Media{Times: make([]float64, 0, len(frames))}
	h := sha256.New()
	for _, f := range frames {
		m.Frames = append(m.Frames, f.Picture)
		m.Times = append(m.Times, f.Time)
		h.Write(f.Picture.Pix)
	}
	return m, fmt.Sprintf("video-file:%x", h.Sum(nil)), nil
}

// mediaPath is an item's file path, if it names one: a path, a file
// object, or an object's Path.
func mediaPath(item any) (string, bool) {
	if obj, ok := common.BindValue(item).(map[string]any); ok {
		if p, ok := field(obj, "Path").(string); ok && p != "" {
			return p, true
		}
		if d, ok := field(obj, "Data").(string); ok && d != "" {
			return "", false
		}
	}
	p, ok := common.BindPath(item)
	return p, ok && p != ""
}

func field(obj map[string]any, name string) any {
	for k, v := range obj {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return nil
}

func optFloat(v any) (float64, bool) {
	switch n := normalizeNumbers(v).(type) {
	case int:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// mediaBytes reads one picture: a path, a filesystem object (BindPath's
// PwrqValue, FullName or Path), or {Data: base64}.
func mediaBytes(item any) ([]byte, error) {
	if obj, ok := common.BindValue(item).(map[string]any); ok {
		if d, ok := field(obj, "Data").(string); ok && d != "" {
			raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(d))
			if err != nil {
				return nil, fmt.Errorf("the Data is not base64: %w", err)
			}
			return raw, nil
		}
		if p, ok := field(obj, "Path").(string); ok && p != "" {
			return os.ReadFile(p)
		}
	}
	path, ok := common.BindPath(item)
	if !ok || path == "" {
		return nil, fmt.Errorf("a picture is a path, a file object, or {Data: base64}, not %s", jsonType(common.BindValue(item)))
	}
	return os.ReadFile(filepath.Clean(path))
}

// askLocal answers a call on the shared local model, through the cache when
// asked.
func askLocal(ctx context.Context, op string, req *localRequest, o options) (*response, d1.Info, error) {
	answer := prompt.AnswerOptions{ScoreReadout: o.ScoreReadout}
	if o.AbstainBelow != 0 {
		v := o.AbstainBelow
		answer.AbstainBelow = &v
	}
	cache, err := openCache(op, o)
	if err != nil {
		return nil, d1.Info{}, err
	}
	key := cache.localKey(req, o)
	if hit, ok := cache.get(key); ok && hit.Answers != nil {
		recordCacheHit()
		hit.Cached = true
		var info d1.Info
		if m, ok := d1.Loaded(); ok {
			info = m.Info()
		}
		return hit, info, nil
	}
	model, err := d1.Shared(ctx, d1Options(o))
	if err != nil {
		return nil, d1.Info{}, fmt.Errorf("%s: loading d1-2B: %w", op, err)
	}
	ask := func(media []d1.Media) (*d1.Response, error) {
		return model.Ask(ctx, d1.Request{
			State: req.state, Questions: req.questions, Media: media,
			SetSize: o.SetSize, Answer: answer, Uncalibrated: o.Uncalibrated,
		})
	}
	var resp *d1.Response
	if req.seg != nil {
		answers, tokens, seconds, err := askSegments(ctx, req, ask, o.ScoreReadout)
		if err != nil {
			return nil, model.Info(), fmt.Errorf("%s: %w", op, err)
		}
		resp = &d1.Response{Answers: answers, InputTokens: tokens, Seconds: seconds}
	} else if resp, err = ask(req.media); err != nil {
		return nil, model.Info(), fmt.Errorf("%s: %w", op, err)
	}
	out := &response{
		Answers:     resp.Answers,
		Model:       localModel,
		ServedModel: fmt.Sprintf("%s (%s, %s)", localModel, model.Info().Variant, model.Info().Device),
		Provider:    backendLocal,
		InputTokens: resp.InputTokens,
		Seconds:     resp.Seconds,
	}
	recordLocalCall(resp.InputTokens)
	cache.put(key, out)
	return out, model.Info(), nil
}

// localKey is the cache key of a local call: everything that changes its
// answers, and the model revision, so a re-pinned model never reads
// another's answers.
func (c *cache) localKey(req *localRequest, o options) string {
	if c == nil {
		return ""
	}
	l, _ := assets.Locked()
	identity := struct {
		Kind         string   `json:"kind"`
		Revision     string   `json:"revision"`
		ModelDir     string   `json:"model_dir"`
		State        any      `json:"state"`
		Questions    any      `json:"questions"`
		Media        []string `json:"media"`
		SetSize      int      `json:"set_size"`
		ScoreReadout string   `json:"score_readout"`
		AbstainBelow float64  `json:"abstain_below"`
		Uncalibrated bool     `json:"uncalibrated"`
	}{"d1-local", l.Model.Revision, o.ModelDir, req.state, req.raw, req.digest, o.SetSize, o.ScoreReadout, o.AbstainBelow, o.Uncalibrated}
	encoded, err := json.Marshal(identity)
	if err != nil {
		return ""
	}
	s := sha256.Sum256(encoded)
	return hex.EncodeToString(s[:])
}

// localObject is invoke_systemone_request's report of a local call.
func localObject(resp *response, info d1.Info) map[string]any {
	out := map[string]any{
		"Answers":      resp.Answers,
		"Model":        localModel,
		"ServedModel":  resp.ServedModel,
		"Provider":     backendLocal,
		"Device":       info.Device,
		"Fallback":     nil,
		"Revision":     info.Revision,
		"InputTokens":  resp.InputTokens,
		"OutputTokens": 0,
		"TotalTokens":  resp.InputTokens,
		"Cost":         nil,
		"Cached":       resp.Cached,
		"Seconds":      resp.Seconds,

		typed.TypeKey: "Pwrq.LLM.SystemOne",
	}
	if info.Fallback != "" {
		out["Fallback"] = info.Fallback
	}
	return out
}

// systemOneInfo is get_systemone's report.
func systemOneInfo(ctx context.Context, o options) (map[string]any, error) {
	do := d1Options(o)
	ao := assets.Options{CacheDir: do.CacheDir}
	gpu := assets.FindGPU(&ao)
	m, err := d1.Shared(ctx, do)
	if err != nil {
		return nil, err
	}
	info := m.Info()
	l, _ := assets.Locked()
	files := make([]string, 0, len(l.Model.Files))
	for f := range l.Model.Files {
		files = append(files, f)
	}
	sort.Strings(files)
	out := map[string]any{
		"Model":         localModel,
		"Repo":          info.Repo,
		"Revision":      info.Revision,
		"Variant":       info.Variant,
		"Device":        info.Device,
		"Fallback":      nil,
		"ModelSource":   info.ModelSource,
		"Runtime":       info.Runtime,
		"RuntimeSource": info.RuntimeSource,
		"OrtVersion":    info.ORTVersion,
		"Calibrated":    info.Calibrated,
		"LoadSeconds":   info.LoadSeconds,
		"CacheDir":      ao.Cache(),
		"Gpu": map[string]any{
			"Driver": nilIfEmpty(gpu.Driver),
			"Usable": gpu.Usable(),
			"Why":    nilIfEmpty(gpu.Why()),
		},
		"QuestionTypes": []any{"noul", "choice", "score", "text", "number", "date", "box", "boxes"},
		typed.TypeKey:   "Pwrq.LLM.SystemOneInfo",
	}
	if info.Fallback != "" {
		out["Fallback"] = info.Fallback
	}
	return out, nil
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// localTimeout bounds a local call: a cold start fetches two gigabytes.
const localTimeout = 30 * time.Minute
