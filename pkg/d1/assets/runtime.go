package assets

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
)

// Runtime is a resolved ONNX Runtime library.
type Runtime struct {
	Path   string // the library to load
	CUDA   bool   // the GPU build (which runs on the CPU too)
	Source string // "flag", "cache", "download"
}

// RuntimeOptions say which ONNX Runtime to use.
type RuntimeOptions struct {
	Options
	Lib   string // a library the user named: used as is
	CUDA  bool   // the GPU build
	Fetch bool   // download it when missing
}

// companions are the libraries ONNX Runtime loads from its own folder.
var companions = regexp.MustCompile(`^(lib)?onnxruntime_providers_(shared|cuda)\.(so|dll|dylib)$`)

func runtimeDir(o *Options, version, flavour string) string {
	return filepath.Join(o.Cache(), "runtime", fmt.Sprintf("onnxruntime-%s-%s-%s-%s", version, runtime.GOOS, runtime.GOARCH, flavour))
}

// complete returns the library in a runtime folder whose unpacking finished.
func complete(dir string) (string, bool) {
	raw, err := os.ReadFile(filepath.Join(dir, ".complete"))
	if err != nil {
		return "", false
	}
	p := filepath.Join(dir, strings.TrimSpace(string(raw)))
	if _, err := os.Stat(p); err != nil {
		return "", false
	}
	return p, true
}

// LibFromEnv is the library PWRQ_D1_ORT_LIB or ONNXRUNTIME_LIB names.
func LibFromEnv() string {
	for _, k := range []string{"PWRQ_D1_ORT_LIB", "ONNXRUNTIME_LIB"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// ResolveRuntime finds the ONNX Runtime library.
func ResolveRuntime(ctx context.Context, opt RuntimeOptions) (*Runtime, error) {
	if opt.Lib != "" {
		if _, err := os.Stat(opt.Lib); err != nil {
			return nil, fmt.Errorf("onnxruntime library: %w", err)
		}
		return &Runtime{Path: opt.Lib, Source: "flag", CUDA: strings.Contains(strings.ToLower(opt.Lib), "gpu")}, nil
	}
	l, err := Locked()
	if err != nil {
		return nil, err
	}
	plat := Platform()
	pins, flavour := l.ONNXRuntime.CPU, "cpu"
	if opt.CUDA {
		pins, flavour = l.ONNXRuntime.CUDA, "cuda"
	}
	wheel, ok := pins[plat]
	if !ok {
		if opt.CUDA {
			return nil, fmt.Errorf("ONNX Runtime has no CUDA build for %s", plat)
		}
		return nil, fmt.Errorf("ONNX Runtime has no build for %s", plat)
	}
	dir := runtimeDir(&opt.Options, l.ONNXRuntime.Version, flavour)
	if p, ok := complete(dir); ok {
		return &Runtime{Path: p, CUDA: opt.CUDA, Source: "cache"}, nil
	}
	if !opt.Fetch {
		return nil, fmt.Errorf("ONNX Runtime (%s) is not in the cache: %w", flavour, ErrNotFetched)
	}
	p, err := fetchWheel(ctx, &opt.Options, wheel, dir, func(name string) bool {
		return slices.Contains(wheel.Libs, name) || companions.MatchString(path.Base(name)) && strings.HasPrefix(name, "onnxruntime/capi/")
	}, path.Base(wheel.Libs[0]))
	if err != nil {
		return nil, err
	}
	return &Runtime{Path: p, CUDA: opt.CUDA, Source: "download"}, nil
}

// fetchWheel downloads a wheel and unpacks the files keep picks, flat, into
// dir; it returns the path of main inside dir.
func fetchWheel(ctx context.Context, o *Options, w Wheel, dir string, keep func(string) bool, main string) (string, error) {
	tmp := filepath.Join(o.Cache(), "downloads", w.File)
	o.logf("downloading %s (%s)", w.File, HumanBytes(w.Size))
	if err := fetch(ctx, o, w.URL, tmp, w.SHA256, w.Size, nil); err != nil {
		return "", err
	}
	o.progress(Event{Step: "unpacking", Name: w.File})
	if err := unzip(tmp, dir, keep); err != nil {
		return "", fmt.Errorf("unpacking %s: %w", w.File, err)
	}
	for _, f := range []string{tmp, tmp + ".sha256", tmp + ".lock"} {
		_ = os.Remove(f)
	}
	if main == "" {
		return dir, nil
	}
	p := filepath.Join(dir, main)
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("%s has no %s", w.File, main)
	}
	if err := os.WriteFile(filepath.Join(dir, ".complete"), []byte(main+"\n"), 0o644); err != nil {
		return "", err
	}
	return p, nil
}

func unzip(archive, dir string, keep func(string) bool) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer func() { _ = zr.Close() }()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || !keep(f.Name) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		err = writeFileFrom(filepath.Join(dir, path.Base(f.Name)), rc, 0o755)
		_ = rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func writeFileFrom(dst string, r io.Reader, perm os.FileMode) error {
	tmp := dst + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}
