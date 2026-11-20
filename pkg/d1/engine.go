// Package d1 runs LiquidAI's d1-2B decision model inside pwrq: a 2B
// vision-language model that answers typed questions about a state - text,
// a JSON document, pictures, a video - in one forward pass, with calibrated
// probabilities and no generated tokens.
//
// The model is ONNX (decoder q4 for the CPU, q4f16 for CUDA; vision encoder
// q8), run by ONNX Runtime loaded at run time without cgo. Device auto tries
// CUDA when an NVIDIA GPU and its libraries are present and falls back to the
// CPU, saying why in Info.Fallback. The pinned files are downloaded on
// first use and cached (see package assets); `pwrq --d1 fetch` does it
// ahead of time.
//
// The prompt and the readout are pkg/d1/prompt, a port of the checkpoint's
// own Python, checked in CI against what that code produced.
package d1

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xen0bit/pwrq/pkg/d1/assets"
	"github.com/xen0bit/pwrq/pkg/d1/ort"
	"github.com/xen0bit/pwrq/pkg/d1/prompt"
	"github.com/xen0bit/pwrq/pkg/d1/tokenizer"
)

// Devices a model may run on.
const (
	Auto   = "auto"
	CPU    = "cpu"
	GPU    = "gpu"
	CoreML = "coreml"
)

// Options say where the model comes from and what it runs on. The zero
// value plus OptionsFromEnv is what pwrq uses.
type Options struct {
	Device   string // auto (default), cpu, gpu (CUDA), coreml
	ModelDir string // a folder with onnx/, tokenizer.json, calibration.json: used as is
	CacheDir string
	ORTLib   string // an onnxruntime library to load instead of the pinned one
	Offline  bool   // never download
	// FetchGPU lets device auto download the CUDA stack (several gigabytes).
	// Off, auto uses the GPU only when it is already there; device gpu always
	// fetches what it needs.
	FetchGPU bool
	Threads  int // intra-op threads; 0: ONNX Runtime's default
	GPUID    int
	Verbose  bool
	Log      func(format string, args ...any)
}

// OptionsFromEnv reads PWRQ_D1_DEVICE, PWRQ_D1_MODEL_DIR, PWRQ_D1_CACHE_DIR,
// PWRQ_D1_ORT_LIB (or ONNXRUNTIME_LIB), PWRQ_D1_OFFLINE, PWRQ_D1_THREADS and
// PWRQ_D1_GPU_ID.
func OptionsFromEnv() Options {
	o := Options{
		Device:   os.Getenv("PWRQ_D1_DEVICE"),
		ModelDir: os.Getenv("PWRQ_D1_MODEL_DIR"),
		CacheDir: os.Getenv("PWRQ_D1_CACHE_DIR"),
		ORTLib:   assets.LibFromEnv(),
	}
	switch strings.ToLower(os.Getenv("PWRQ_D1_OFFLINE")) {
	case "1", "true", "yes", "on":
		o.Offline = true
	}
	o.Threads, _ = strconv.Atoi(os.Getenv("PWRQ_D1_THREADS"))
	o.GPUID, _ = strconv.Atoi(os.Getenv("PWRQ_D1_GPU_ID"))
	return o
}

func (o Options) device() (string, error) {
	switch d := strings.ToLower(strings.TrimSpace(o.Device)); d {
	case "", Auto:
		return Auto, nil
	case CPU:
		return CPU, nil
	case GPU, "cuda":
		return GPU, nil
	case CoreML:
		return CoreML, nil
	default:
		return "", fmt.Errorf("device %q is not one of auto, cpu, gpu, coreml", o.Device)
	}
}

func (o Options) logf(format string, args ...any) {
	if o.Log != nil {
		o.Log(format, args...)
	}
}

// Info describes a loaded model.
type Info struct {
	Repo          string  `json:"repo"`
	Revision      string  `json:"revision"`
	Variant       string  `json:"variant"`
	Device        string  `json:"device"`
	Fallback      string  `json:"fallback,omitempty"`
	ModelSource   string  `json:"model_source"`
	ModelDir      string  `json:"model_dir,omitempty"`
	Runtime       string  `json:"runtime"`
	RuntimeSource string  `json:"runtime_source"`
	ORTVersion    string  `json:"ort_version"`
	Calibrated    bool    `json:"calibrated"`
	LoadSeconds   float64 `json:"load_seconds"`
}

// Model is a loaded d1-2B. Its methods are safe for concurrent use; forward
// passes are serialised, one session on one device.
type Model struct {
	info    Info
	builder *prompt.Builder
	cal     *prompt.Calibration
	dec     *ort.Session
	vis     *ort.Session
	mu      sync.Mutex
}

// Info describes the model.
func (m *Model) Info() Info { return m.info }

// Builder is the model's prompt builder.
func (m *Model) Builder() *prompt.Builder { return m.builder }

// Open loads the model.
func Open(ctx context.Context, o Options) (*Model, error) {
	if !ort.Supported {
		return nil, fmt.Errorf("d1 needs ONNX Runtime, which has no build for %s/%s (it runs on linux, macOS and Windows, amd64 and arm64)", runtime.GOOS, runtime.GOARCH)
	}
	device, err := o.device()
	if err != nil {
		return nil, err
	}
	start := time.Now()
	ao := assets.Options{CacheDir: o.CacheDir, Offline: o.Offline, Log: o.Log, Progress: reportAsset}
	setStatus(Status{State: StateLoading})

	var fallback string
	if device == GPU || device == Auto && assets.CUDAPlatform() {
		m, err := openGPU(ctx, o, ao, device == GPU || o.FetchGPU)
		if err == nil {
			m.info.LoadSeconds = time.Since(start).Seconds()
			return m, nil
		}
		if device == GPU {
			return nil, fmt.Errorf("GPU: %w (use device cpu to run on the CPU)", err)
		}
		fallback = err.Error()
		o.logf("d1: running on the CPU: %s", fallback)
	} else if device == Auto {
		fallback = fmt.Sprintf("ONNX Runtime has no CUDA build for %s", assets.Platform())
	}

	rt, err := assets.ResolveRuntime(ctx, assets.RuntimeOptions{Options: ao, Lib: o.ORTLib, Fetch: true})
	if err != nil {
		return nil, err
	}
	lib, err := ort.Load(rt.Path)
	if err != nil {
		return nil, err
	}
	model, err := assets.ResolveModel(ctx, assets.ModelOptions{Options: ao, Dir: o.ModelDir, Fetch: true})
	if err != nil {
		return nil, err
	}
	so := ort.SessionOptions{Provider: ort.CPU, Threads: o.Threads, Verbose: o.Verbose}
	used := CPU
	if device == CoreML {
		so.Provider, used = ort.CoreML, CoreML
	}
	m, err := build(lib, model, rt, so, used)
	if err != nil {
		return nil, err
	}
	m.info.Fallback = fallback
	m.info.LoadSeconds = time.Since(start).Seconds()
	return m, nil
}

func openGPU(ctx context.Context, o Options, ao assets.Options, fetch bool) (*Model, error) {
	g := assets.FindGPU(&ao)
	if !g.Usable() && fetch && g.Driver != "" {
		if err := assets.FetchGPU(ctx, &ao); err != nil {
			return nil, err
		}
		g = assets.FindGPU(&ao)
	}
	if !g.Usable() {
		return nil, errors.New(g.Why())
	}
	rt, err := assets.ResolveRuntime(ctx, assets.RuntimeOptions{Options: ao, Lib: o.ORTLib, CUDA: true, Fetch: fetch})
	if err != nil {
		if errors.Is(err, assets.ErrNotFetched) {
			return nil, errors.New("the GPU build of ONNX Runtime is not in the cache; `pwrq --d1 fetch-gpu` installs it")
		}
		return nil, err
	}
	lib, err := ort.Load(rt.Path)
	if err != nil {
		return nil, err
	}
	if !lib.HasProvider("CUDAExecutionProvider") {
		return nil, fmt.Errorf("%s is not a GPU build of ONNX Runtime", rt.Path)
	}
	if err := ort.Preload(g.Libs); err != nil {
		return nil, fmt.Errorf("loading the CUDA libraries: %w", err)
	}
	model, err := assets.ResolveModel(ctx, assets.ModelOptions{Options: ao, Dir: o.ModelDir, GPU: o.ModelDir == "", Fetch: fetch})
	if err != nil {
		if errors.Is(err, assets.ErrNotFetched) {
			return nil, errors.New("the q4f16 decoder for CUDA is not in the cache; `pwrq --d1 fetch-gpu` downloads it")
		}
		return nil, err
	}
	return build(lib, model, rt, ort.SessionOptions{Provider: ort.CUDA, Device: o.GPUID, Threads: o.Threads, Verbose: o.Verbose}, GPU)
}

func build(lib *ort.Library, model *assets.Model, rt *assets.Runtime, so ort.SessionOptions, device string) (*Model, error) {
	tokRaw, err := model.Bytes("tokenizer.json")
	if err != nil {
		return nil, err
	}
	tok, err := tokenizer.Parse(tokRaw)
	if err != nil {
		return nil, fmt.Errorf("tokenizer.json: %w", err)
	}
	cfg := prompt.Default
	if raw, err := model.Bytes("config.json"); err == nil {
		cfg = configFrom(raw, cfg)
	}
	var cal *prompt.Calibration
	if raw, err := model.Bytes("calibration.json"); err == nil {
		if cal, err = prompt.ParseCalibration(raw); err != nil {
			return nil, err
		}
	}
	dec, err := lib.NewSession(ort.ModelSource{Path: model.Path(model.Decoder())}, so)
	if err != nil {
		return nil, fmt.Errorf("loading %s: %w", model.Decoder(), err)
	}
	vis, err := lib.NewSession(ort.ModelSource{Path: model.Path(model.Vision())}, so)
	if err != nil {
		dec.Close()
		return nil, fmt.Errorf("loading %s: %w", model.Vision(), err)
	}
	l, _ := assets.Locked()
	m := &Model{
		builder: prompt.NewBuilder(tok, cfg),
		cal:     cal,
		dec:     dec,
		vis:     vis,
		info: Info{
			Repo: l.Model.Repo, Revision: model.Revision, Variant: model.Variant, Device: device,
			ModelSource: model.Source, ModelDir: model.Dir, Runtime: rt.Path, RuntimeSource: rt.Source,
			ORTVersion: lib.Version, Calibrated: cal != nil,
		},
	}
	setStatus(Status{State: StateReady, Device: device})
	return m, nil
}

// configFrom applies a checkpoint's d1_ keys over the defaults.
func configFrom(raw []byte, cfg prompt.Config) prompt.Config {
	var c struct {
		Codes     *int    `json:"d1_codes"`
		Numbers   *bool   `json:"d1_numbers"`
		SetSize   *int    `json:"d1_set_size"`
		MaxPixels *int    `json:"d1_vision_max_pixels"`
		BoxEdges  *string `json:"d1_box_edges"`
		Prompt    *int    `json:"d1_prompt"`
	}
	if json.Unmarshal(raw, &c) != nil {
		return cfg
	}
	if c.Codes != nil {
		cfg.Codes = *c.Codes
	}
	if c.Numbers != nil {
		cfg.Numbers = *c.Numbers
	}
	if c.SetSize != nil && *c.SetSize >= 1 {
		cfg.SetSize = *c.SetSize
	}
	if c.MaxPixels != nil && *c.MaxPixels > 0 {
		cfg.VisionMax = *c.MaxPixels
	}
	if c.BoxEdges != nil && (*c.BoxEdges == "median" || *c.BoxEdges == "interp") {
		cfg.BoxEdges = *c.BoxEdges
	}
	if c.Prompt != nil {
		cfg.Version = *c.Prompt
	}
	return cfg
}

// Close frees the sessions.
func (m *Model) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dec != nil {
		m.dec.Close()
		m.dec = nil
	}
	if m.vis != nil {
		m.vis.Close()
		m.vis = nil
	}
}
