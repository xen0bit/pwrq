// Package assets finds, fetches and verifies what pwrq's decision model
// runs on: d1-2B's ONNX graphs and tokenizer, and the ONNX Runtime library.
// It is adapted from hotbutteredbeans' assets package.
//
// Nothing is compiled in - two gigabytes of weights would not link - so the
// files are downloaded on first use and cached. Everything is pinned by
// d1.lock.json (sha256 and size of every file), so every machine runs the
// same bytes. Look-up order, first match wins:
//
//	model:   ModelDir (PWRQ_D1_MODEL_DIR) > cache > download from Hugging Face
//	runtime: ORTLib (PWRQ_D1_ORT_LIB, ONNXRUNTIME_LIB) > cache > download from PyPI
//
// Downloads resume, verify, and lock against a second pwrq fetching the
// same file. The cache is PWRQ_D1_CACHE_DIR, else pwrq/d1 in the user cache
// directory.
package assets

import (
	"context"
	"crypto/sha256"
	_ "embed" // the lock file
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

//go:embed d1.lock.json
var lockJSON []byte

// File is a pinned file.
type File struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// Wheel is a pinned PyPI wheel and the shared libraries taken from it.
type Wheel struct {
	Name   string   `json:"name"`
	File   string   `json:"file"`
	URL    string   `json:"url"`
	SHA256 string   `json:"sha256"`
	Size   int64    `json:"size"`
	Libs   []string `json:"libs,omitempty"`
}

// Lock is d1.lock.json.
type Lock struct {
	Model struct {
		Repo     string          `json:"repo"`
		Revision string          `json:"revision"`
		CPU      []string        `json:"cpu"`
		GPU      []string        `json:"gpu"`
		Files    map[string]File `json:"files"`
	} `json:"model"`
	ONNXRuntime struct {
		Version string           `json:"version"`
		CPU     map[string]Wheel `json:"cpu"`
		CUDA    map[string]Wheel `json:"cuda"`
	} `json:"onnxruntime"`
	CUDA map[string][]Wheel `json:"cuda"`
}

var (
	lockOnce sync.Once
	lock     Lock
	lockErr  error
)

// Locked returns the pins.
func Locked() (*Lock, error) {
	lockOnce.Do(func() { lockErr = json.Unmarshal(lockJSON, &lock) })
	return &lock, lockErr
}

// Platform is GOOS/GOARCH.
func Platform() string { return runtime.GOOS + "/" + runtime.GOARCH }

// Event reports progress: a file being fetched (Done of Total bytes, Total
// < 0 when unknown), or a step such as "loading".
type Event struct {
	Step  string // "fetching", "verifying", "unpacking"
	Name  string
	Done  int64
	Total int64
}

// Options say where assets come from.
type Options struct {
	CacheDir string
	Offline  bool
	Progress func(Event)
	Log      func(format string, args ...any)
}

func (o *Options) logf(format string, args ...any) {
	if o != nil && o.Log != nil {
		o.Log(format, args...)
	}
}

func (o *Options) progress(e Event) {
	if o != nil && o.Progress != nil {
		o.Progress(e)
	}
}

// Cache is the cache root in use.
func (o *Options) Cache() string {
	if o != nil && o.CacheDir != "" {
		return o.CacheDir
	}
	return CacheDir()
}

// CacheDir is PWRQ_D1_CACHE_DIR, else pwrq/d1 in the user cache directory
// (~/.cache/pwrq/d1, ~/Library/Caches/pwrq/d1, %LocalAppData%\pwrq\d1).
func CacheDir() string {
	if d := os.Getenv("PWRQ_D1_CACHE_DIR"); d != "" {
		return d
	}
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "pwrq", "d1")
	}
	return filepath.Join(os.TempDir(), "pwrq-d1-cache")
}

// ErrOffline is returned when an asset is missing and downloads are off.
var ErrOffline = errors.New("not available offline")

// ErrNotFetched is returned for an optional asset (the GPU stack) that is
// not in the cache and was not asked to be fetched.
var ErrNotFetched = errors.New("not fetched")

var client = &http.Client{Transport: http.DefaultTransport} // honours HTTPS_PROXY

// UserAgent is sent with every download; cli sets the version in it.
var UserAgent = "pwrq"

// fetch downloads url to dst, checking its size and sha256 and resuming a
// partial download an earlier run left. Concurrent fetches of the same dst
// wait for each other.
func fetch(ctx context.Context, o *Options, url, dst, sum string, size int64, header http.Header) error {
	if o.Offline {
		return fmt.Errorf("%s: %w (unset PWRQ_D1_OFFLINE to download it, or run `pwrq --d1 fetch`)", filepath.Base(dst), ErrOffline)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	unlock, err := lockFile(dst + ".lock")
	if err != nil {
		return err
	}
	defer unlock()
	if ok, _ := verified(dst, sum, size); ok { // another process finished it while we waited
		return nil
	}
	part := dst + ".part"
	f, err := os.OpenFile(part, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	have, _ := f.Seek(0, io.SeekEnd)
	if size > 0 && have > size {
		have = 0
	}
	for attempt := 0; ; attempt++ {
		err = get(ctx, o, url, f, &have, size, header, filepath.Base(dst))
		if err == nil || ctx.Err() != nil || attempt == 4 || !retryable(err) {
			break
		}
		o.logf("download of %s interrupted (%v), retrying", filepath.Base(dst), err)
		select {
		case <-time.After(time.Duration(1<<attempt) * time.Second):
		case <-ctx.Done():
		}
	}
	if err != nil {
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	o.progress(Event{Step: "verifying", Name: filepath.Base(dst), Done: have, Total: size})
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if (size > 0 && n != size) || (sum != "" && got != sum) {
		_ = f.Close()
		_ = os.Remove(part)
		return fmt.Errorf("%s: got %d bytes with sha256 %s, want %d bytes with sha256 %s; the partial file was removed, try again",
			url, n, got, size, sum)
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(part, dst); err != nil {
		return err
	}
	return markVerified(dst, got)
}

type httpError struct{ code int }

func (e httpError) Error() string { return fmt.Sprintf("HTTP %d %s", e.code, http.StatusText(e.code)) }

func retryable(err error) bool {
	var he httpError
	if errors.As(err, &he) {
		return he.code == 429 || he.code >= 500
	}
	return true // a network error
}

func get(ctx context.Context, o *Options, url string, f *os.File, have *int64, size int64, header http.Header, name string) error {
	if size > 0 && *have == size {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	for k, vs := range header {
		req.Header[k] = vs
	}
	req.Header.Set("User-Agent", UserAgent)
	if *have > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", *have))
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusPartialContent && *have > 0:
	case resp.StatusCode == http.StatusOK:
		if err := f.Truncate(0); err != nil {
			return err
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return err
		}
		*have = 0
	case resp.StatusCode == http.StatusRequestedRangeNotSatisfiable:
		*have = 0
		_ = f.Truncate(0)
		_, _ = f.Seek(0, io.SeekStart)
		return httpError{resp.StatusCode}
	default:
		return httpError{resp.StatusCode}
	}
	total := size
	if total <= 0 && resp.ContentLength > 0 {
		total = *have + resp.ContentLength
	}
	buf := make([]byte, 1<<20)
	last := time.Time{}
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				return err
			}
			*have += int64(n)
			if time.Since(last) > 200*time.Millisecond {
				o.progress(Event{Step: "fetching", Name: name, Done: *have, Total: total})
				last = time.Now()
			}
		}
		if rerr == io.EOF {
			o.progress(Event{Step: "fetching", Name: name, Done: *have, Total: total})
			return nil
		}
		if rerr != nil {
			return rerr
		}
	}
}

// A verified file has a sidecar "<file>.sha256" holding its sha256, size and
// mtime, so a two-gigabyte model is hashed once, not on every run.

func markVerified(path, sum string) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path+".sha256", fmt.Appendf(nil, "%s %d %d\n", sum, st.Size(), st.ModTime().UnixNano()), 0o644)
}

// verified reports whether path exists and matches sum and size.
func verified(path, sum string, size int64) (bool, error) {
	st, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	if size > 0 && st.Size() != size {
		return false, nil
	}
	raw, err := os.ReadFile(path + ".sha256")
	if err == nil {
		var s string
		var n, mt int64
		if _, err := fmt.Sscanf(string(raw), "%s %d %d", &s, &n, &mt); err == nil &&
			n == st.Size() && mt == st.ModTime().UnixNano() {
			return sum == "" || s == sum, nil
		}
	}
	if sum == "" {
		return false, nil
	}
	got, err := hashFile(path)
	if err != nil {
		return false, err
	}
	if got != sum {
		return false, nil
	}
	return true, markVerified(path, got)
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// safeName turns a repository name into one path element.
func safeName(s string) string {
	return strings.NewReplacer("/", "__", "\\", "__", ":", "_").Replace(s)
}

// HumanBytes writes a size the way a progress line wants it.
func HumanBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.2f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f kB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
