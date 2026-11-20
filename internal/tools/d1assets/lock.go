package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/xen0bit/pwrq/pkg/d1/assets"
)

// The pins `lock` writes. Change these, run `make d1.lock`, and commit the
// lock: every build and every download then agrees on the same bytes.
const (
	modelRepo     = "LiquidAI/d1-2B-ONNX"
	modelRevision = "2fd61486312020c30a1e3aa5aa677424672b2b37"
	ortVersion    = "1.30.0"
	// The CPython tag of the wheels the runtime is taken from. Only the shared
	// library inside is used; the tag just picks one of the identical copies.
	pyTag = "cp312"
)

// cpuFiles are what a CPU run needs; gpuFiles are fetched only for CUDA.
var (
	cpuFiles = []string{
		"onnx/decoder_q4.onnx", "onnx/decoder_q4.onnx_data",
		"onnx/vision_encoder_q8.onnx", "onnx/vision_encoder_q8.onnx_data",
		"tokenizer.json", "calibration.json", "config.json", "processor_config.json", "LICENSE",
	}
	gpuFiles = []string{"onnx/decoder_q4f16.onnx", "onnx/decoder_q4f16.onnx_data"}
)

// wheelPlatforms maps a Go platform to the PyPI wheel platform tag that
// carries ONNX Runtime's shared library, and the library's path inside.
var wheelPlatforms = map[string]struct{ tag, lib string }{
	"linux/amd64":   {"manylinux_2_28_x86_64", "onnxruntime/capi/libonnxruntime.so." + ortVersion},
	"linux/arm64":   {"manylinux_2_28_aarch64", "onnxruntime/capi/libonnxruntime.so." + ortVersion},
	"darwin/arm64":  {"macosx_14_0_arm64", "onnxruntime/capi/libonnxruntime." + ortVersion + ".dylib"},
	"windows/amd64": {"win_amd64", "onnxruntime/capi/onnxruntime.dll"},
	"windows/arm64": {"win_arm64", "onnxruntime/capi/onnxruntime.dll"},
}

// gpuPlatforms are those onnxruntime-gpu ships for.
var gpuPlatforms = map[string]struct{ tag, lib string }{
	"linux/amd64":   {"manylinux_2_28_x86_64", "onnxruntime/capi/libonnxruntime.so." + ortVersion},
	"windows/amd64": {"win_amd64", "onnxruntime/capi/onnxruntime.dll"},
}

// cudaWheels are NVIDIA's CUDA 13 and cuDNN 9 libraries, the versions
// onnxruntime-gpu 1.30 was built against.
var cudaWheels = []struct{ name, version string }{
	{"nvidia-cuda-runtime", "13.0.96"},
	{"nvidia-cublas", "13.1.1.3"},
	{"nvidia-curand", "10.4.0.35"},
	{"nvidia-cufft", "12.0.0.61"},
	{"nvidia-cudnn-cu13", "9.24.0.43"},
	{"nvidia-cuda-nvrtc", "13.0.88"},
}

var cudaTags = map[string][]string{
	"linux/amd64":   {"manylinux_2_27_x86_64", "manylinux2014_x86_64", "manylinux_2_17_x86_64", "manylinux_2_28_x86_64", "manylinux_2_12_x86_64"},
	"windows/amd64": {"win_amd64"},
}

type pypiRelease struct {
	URLs []struct {
		Filename string `json:"filename"`
		URL      string `json:"url"`
		Size     int64  `json:"size"`
		Digests  struct {
			SHA256 string `json:"sha256"`
		} `json:"digests"`
	} `json:"urls"`
}

func getJSON(ctx context.Context, u string, body io.Reader, into any) error {
	method := http.MethodGet
	if body != nil {
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if tok := os.Getenv("HF_TOKEN"); tok != "" && strings.Contains(u, "huggingface.co") {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: HTTP %d", u, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(into)
}

func pypi(ctx context.Context, name, version string) (pypiRelease, error) {
	var r pypiRelease
	err := getJSON(ctx, fmt.Sprintf("https://pypi.org/pypi/%s/%s/json", name, version), nil, &r)
	return r, err
}

func pickWheel(r pypiRelease, name string, tags []string, needPy bool) (assets.Wheel, bool) {
	for _, tag := range tags {
		for _, u := range r.URLs {
			if !strings.HasSuffix(u.Filename, ".whl") || !strings.Contains(u.Filename, tag) {
				continue
			}
			if needPy && !strings.Contains(u.Filename, "-"+pyTag+"-") {
				continue
			}
			return assets.Wheel{Name: name, File: u.Filename, URL: u.URL, SHA256: u.Digests.SHA256, Size: u.Size}, true
		}
	}
	return assets.Wheel{}, false
}

func lock(ctx context.Context, out string) error {
	l := assets.Lock{}
	l.Model.Repo, l.Model.Revision = modelRepo, modelRevision
	l.Model.CPU, l.Model.GPU = cpuFiles, gpuFiles
	l.Model.Files = map[string]assets.File{}

	all := append(append([]string(nil), cpuFiles...), gpuFiles...)
	form := url.Values{}
	for _, p := range all {
		form.Add("paths", p)
	}
	var info []struct {
		Path string `json:"path"`
		Size int64  `json:"size"`
		LFS  *struct {
			OID string `json:"oid"`
		} `json:"lfs"`
	}
	endpoint := fmt.Sprintf("https://huggingface.co/api/models/%s/paths-info/%s", modelRepo, modelRevision)
	if err := getJSON(ctx, endpoint, strings.NewReader(form.Encode()), &info); err != nil {
		return err
	}
	for _, f := range info {
		sum := ""
		if f.LFS != nil {
			sum = f.LFS.OID
		} else { // a small file kept in git: hash its bytes
			raw, err := download(ctx, fmt.Sprintf("https://huggingface.co/%s/resolve/%s/%s", modelRepo, modelRevision, f.Path))
			if err != nil {
				return err
			}
			h := sha256.Sum256(raw)
			sum = hex.EncodeToString(h[:])
		}
		l.Model.Files[f.Path] = assets.File{SHA256: sum, Size: f.Size}
	}
	for _, p := range all {
		if _, ok := l.Model.Files[p]; !ok {
			return fmt.Errorf("%s has no %s at %s", modelRepo, p, modelRevision)
		}
	}

	l.ONNXRuntime.Version = ortVersion
	l.ONNXRuntime.CPU, l.ONNXRuntime.CUDA = map[string]assets.Wheel{}, map[string]assets.Wheel{}
	cpu, err := pypi(ctx, "onnxruntime", ortVersion)
	if err != nil {
		return err
	}
	for plat, w := range wheelPlatforms {
		wh, ok := pickWheel(cpu, "onnxruntime", []string{w.tag}, true)
		if !ok {
			return fmt.Errorf("no onnxruntime %s wheel for %s", ortVersion, plat)
		}
		wh.Libs = []string{w.lib}
		l.ONNXRuntime.CPU[plat] = wh
	}
	gpu, err := pypi(ctx, "onnxruntime-gpu", ortVersion)
	if err != nil {
		return err
	}
	for plat, w := range gpuPlatforms {
		wh, ok := pickWheel(gpu, "onnxruntime-gpu", []string{w.tag}, true)
		if !ok {
			return fmt.Errorf("no onnxruntime-gpu %s wheel for %s", ortVersion, plat)
		}
		wh.Libs = []string{w.lib}
		l.ONNXRuntime.CUDA[plat] = wh
	}

	l.CUDA = map[string][]assets.Wheel{}
	for plat, tags := range cudaTags {
		for _, c := range cudaWheels {
			r, err := pypi(ctx, c.name, c.version)
			if err != nil {
				return err
			}
			wh, ok := pickWheel(r, c.name, tags, false)
			if !ok {
				return fmt.Errorf("no %s %s wheel for %s", c.name, c.version, plat)
			}
			l.CUDA[plat] = append(l.CUDA[plat], wh)
		}
		sort.SliceStable(l.CUDA[plat], func(i, j int) bool { return l.CUDA[plat][i].Name < l.CUDA[plat][j].Name })
	}

	raw, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(out, append(raw, '\n'), 0o644)
}

func download(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", u, resp.StatusCode)
	}
	var b bytes.Buffer
	_, err = io.Copy(&b, resp.Body)
	return b.Bytes(), err
}
