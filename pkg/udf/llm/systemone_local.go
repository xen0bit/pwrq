package llm

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
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
}

// parseLocal checks a call's state, questions and media.
func parseLocal(op string, state any, rawQuestions any, o options) (*localRequest, error) {
	questions, err := prompt.ParseQuestions(rawQuestions)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	media, digest, err := parseMedia(op, o.Media)
	if err != nil {
		return nil, err
	}
	switch s := state.(type) {
	case nil:
		if len(media) == 0 {
			return nil, fmt.Errorf("%s: the state is null and there is no Media; pass text, a document, or {Media: [...]}", op)
		}
	case string:
		if strings.TrimSpace(s) == "" && len(media) == 0 {
			return nil, fmt.Errorf("%s: state is empty", op)
		}
	case map[string]any, []any:
	default:
		return nil, fmt.Errorf("%s: state must be text, an object, an array, or null with Media, got %s; it comes from the pipeline, or the first of three arguments", op, jsonType(state))
	}
	return &localRequest{state: state, questions: questions, media: media, digest: digest, raw: rawQuestions}, nil
}

// parseMedia reads the Media option: each item a picture's path, a
// get_childitem object, {Type: "image", Path | Data}, or {Type: "video",
// Frames: [path | Data...], FPS}. Data is base64.
func parseMedia(op string, items []any) ([]d1.Media, []string, error) {
	var out []d1.Media
	var digest []string
	for i, item := range items {
		fail := func(err error) ([]d1.Media, []string, error) {
			return nil, nil, fmt.Errorf("%s: Media[%d]: %w", op, i, err)
		}
		obj, isObj := common.BindValue(item).(map[string]any)
		kind := "image"
		if isObj {
			if t, ok := field(obj, "Type").(string); ok && t != "" {
				kind = strings.ToLower(t)
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
			frames, _ := field(obj, "Frames").([]any)
			if len(frames) == 0 {
				return fail(fmt.Errorf(`a video is {Type: "video", Frames: [paths or base64 pictures], FPS: n}; pwrq does not decode video files, so pass their frames`))
			}
			fps, ok := optFloat(field(obj, "FPS"))
			if !ok || fps <= 0 {
				return fail(fmt.Errorf("a video needs its frame rate, FPS, above 0"))
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
			out = append(out, m)
			digest = append(digest, fmt.Sprintf("video:%g:%x", fps, h.Sum(nil)))
		case "audio":
			return fail(fmt.Errorf("d1-2B reads pictures and video; audio needs d1-Omni, which pwrq does not ship"))
		default:
			return fail(fmt.Errorf("media type %q is not image or video", kind))
		}
	}
	return out, digest, nil
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
	resp, err := model.Ask(ctx, d1.Request{
		State: req.state, Questions: req.questions, Media: req.media,
		SetSize: o.SetSize, Answer: answer, Uncalibrated: o.Uncalibrated,
	})
	if err != nil {
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
