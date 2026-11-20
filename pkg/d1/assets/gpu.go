package assets

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
)

// GPU is what was found of an NVIDIA GPU stack.
type GPU struct {
	Driver  string   // the driver version, "" when no NVIDIA driver was found
	Libs    []string // the CUDA libraries to preload, when all were found
	Missing []string // what is missing when they were not
	Where   []string // the folders the libraries came from
}

// Usable reports whether the CUDA execution provider can run: a driver new
// enough for CUDA 13 and every library it loads.
func (g GPU) Usable() bool { return g.Driver != "" && len(g.Missing) == 0 && driverOK(g.Driver) }

// Why is a one-line reason the GPU cannot be used.
func (g GPU) Why() string {
	switch {
	case !CUDAPlatform():
		return fmt.Sprintf("ONNX Runtime has no CUDA build for %s", Platform())
	case g.Driver == "":
		return "no NVIDIA driver found"
	case !driverOK(g.Driver):
		return fmt.Sprintf("NVIDIA driver %s is too old for CUDA 13 (580 or newer)", g.Driver)
	case len(g.Missing) > 0:
		return fmt.Sprintf("CUDA 13 / cuDNN 9 libraries not found (%s); `pwrq --d1 fetch-gpu` installs them",
			strings.Join(g.Missing, ", "))
	}
	return ""
}

// CUDAPlatform reports whether this platform has a CUDA build of ONNX Runtime.
func CUDAPlatform() bool {
	l, err := Locked()
	if err != nil {
		return false
	}
	_, ok := l.ONNXRuntime.CUDA[Platform()]
	return ok
}

func driverOK(v string) bool {
	var major int
	_, _ = fmt.Sscanf(v, "%d", &major)
	return major == 0 || major >= 580 // 0: version unknown (Windows), let the provider decide
}

var driverRE = regexp.MustCompile(`Kernel Module(?:\s+for\s+\S+)?\s+(\d+\.\d+(?:\.\d+)?)`)

func nvidiaDriver() string {
	switch runtime.GOOS {
	case "linux":
		if raw, err := os.ReadFile("/proc/driver/nvidia/version"); err == nil {
			if m := driverRE.FindSubmatch(raw); m != nil {
				return string(m[1])
			}
			return "unknown"
		}
	case "windows":
		root := os.Getenv("SystemRoot")
		if root == "" {
			root = `C:\Windows`
		}
		if _, err := os.Stat(filepath.Join(root, "System32", "nvcuda.dll")); err == nil {
			return "installed"
		}
	}
	return ""
}

// The libraries ONNX Runtime 1.30's CUDA execution provider (CUDA 13) loads,
// by the name the loader looks for; cuDNN's sub-libraries are taken from the
// folder libcudnn is found in.
var cudaLibs = map[string][]string{
	"linux":   {"libcudart.so.13", "libcublasLt.so.13", "libcublas.so.13", "libcurand.so.10", "libcufft.so.12", "libcudnn.so.9"},
	"windows": {"cudart64_13.dll", "cublasLt64_13.dll", "cublas64_13.dll", "curand64_10.dll", "cufft64_12.dll", "cudnn64_9.dll"},
}

var cudnnSub = map[string]string{"linux": "libcudnn_*.so.9", "windows": "cudnn_*64_9.dll"}

// FindGPU looks for the NVIDIA driver and the CUDA libraries.
func FindGPU(o *Options) GPU {
	g := GPU{Driver: nvidiaDriver()}
	if g.Driver == "" || !CUDAPlatform() {
		return g
	}
	dirs := cudaDirs(o)
	for _, name := range cudaLibs[runtime.GOOS] {
		found := ""
		for _, d := range dirs {
			p := filepath.Join(d, name)
			if _, err := os.Stat(p); err == nil {
				found = p
				break
			}
		}
		if found == "" {
			g.Missing = append(g.Missing, name)
			continue
		}
		g.Libs = append(g.Libs, found)
		if d := filepath.Dir(found); !slices.Contains(g.Where, d) {
			g.Where = append(g.Where, d)
		}
		if strings.HasPrefix(name, "libcudnn.") || strings.HasPrefix(name, "cudnn64") {
			subs, _ := filepath.Glob(filepath.Join(filepath.Dir(found), cudnnSub[runtime.GOOS]))
			g.Libs = append(g.Libs, subs...)
		}
	}
	return g
}

// cudaDirs are the folders searched for CUDA libraries, first match wins:
// PWRQ_D1_CUDA_PATH, pwrq's own (`pwrq --d1 fetch-gpu`), the CUDA toolkit,
// pip's nvidia-* wheels in the active virtualenv or conda environment, the
// loader's path, and the system's library folders.
func cudaDirs(o *Options) []string {
	var dirs []string
	add := func(ds ...string) {
		for _, d := range ds {
			if d != "" && !slices.Contains(dirs, d) {
				dirs = append(dirs, d)
			}
		}
	}
	add(filepath.SplitList(os.Getenv("PWRQ_D1_CUDA_PATH"))...)
	add(CUDALibDir(o))
	for _, env := range []string{"CUDA_PATH", "CUDA_HOME"} {
		if r := os.Getenv(env); r != "" {
			add(filepath.Join(r, "lib64"), filepath.Join(r, "lib"), filepath.Join(r, "bin"), filepath.Join(r, "bin", "x64"))
		}
	}
	for _, env := range []string{"VIRTUAL_ENV", "CONDA_PREFIX"} {
		if r := os.Getenv(env); r != "" {
			for _, pat := range []string{"lib/python3*/site-packages/nvidia/*/lib", "lib/python3*/site-packages/nvidia/cu13/lib", "Lib/site-packages/nvidia/*/bin"} {
				m, _ := filepath.Glob(filepath.Join(r, pat))
				add(m...)
			}
		}
	}
	if runtime.GOOS == "windows" {
		add(filepath.SplitList(os.Getenv("PATH"))...)
		return dirs
	}
	add(filepath.SplitList(os.Getenv("LD_LIBRARY_PATH"))...)
	m, _ := filepath.Glob("/usr/local/cuda-13*/lib64")
	add("/usr/local/cuda/lib64")
	add(m...)
	m, _ = filepath.Glob("/usr/lib/*-linux-gnu")
	add(m...)
	add("/usr/lib64", "/usr/lib", "/usr/local/lib")
	return dirs
}

// CUDALibDir is where `pwrq --d1 fetch-gpu` puts NVIDIA's libraries.
func CUDALibDir(o *Options) string {
	return filepath.Join(o.Cache(), "cuda", runtime.GOOS+"-"+runtime.GOARCH)
}

func isSharedLib(name string) bool {
	base := path.Base(name)
	return strings.HasSuffix(base, ".dll") || strings.Contains(base, ".so")
}

// FetchGPU downloads the GPU build of ONNX Runtime, NVIDIA's CUDA 13 and
// cuDNN 9 libraries (from their PyPI wheels) and the q4f16 decoder.
func FetchGPU(ctx context.Context, o *Options) error {
	l, err := Locked()
	if err != nil {
		return err
	}
	plat := Platform()
	if _, err := ResolveRuntime(ctx, RuntimeOptions{Options: *o, CUDA: true, Fetch: true}); err != nil {
		return err
	}
	wheels, ok := l.CUDA[plat]
	if !ok {
		return fmt.Errorf("no CUDA libraries are pinned for %s", plat)
	}
	dir := CUDALibDir(o)
	for _, w := range wheels {
		marker := filepath.Join(dir, ".complete-"+w.SHA256[:16])
		if _, err := os.Stat(marker); err == nil {
			continue
		}
		if _, err := fetchWheel(ctx, o, w, dir, isSharedLib, ""); err != nil {
			return err
		}
		if err := os.WriteFile(marker, nil, 0o644); err != nil {
			return err
		}
	}
	_, err = ResolveModel(ctx, ModelOptions{Options: *o, GPU: true, Fetch: true})
	return err
}
