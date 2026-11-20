package assets

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Model is d1-2B's files, resolved, on disk.
type Model struct {
	Source   string // "dir", "cache", "download"
	Dir      string // the folder holding the files, when they are on disk
	Revision string
	Variant  string // "q4" (CPU) or "q4f16" (CUDA)
	// Files holds small files already read, by name.
	Files map[string][]byte
}

// Decoder and Vision name the graphs a variant runs.
func (m *Model) Decoder() string {
	if m.Variant == "q4f16" {
		return "onnx/decoder_q4f16.onnx"
	}
	return "onnx/decoder_q4.onnx"
}

// Vision is the vision encoder's graph; both variants use the q8 one, which
// has no GroupQueryAttention, so it runs on CUDA as it is.
func (m *Model) Vision() string { return "onnx/vision_encoder_q8.onnx" }

// Path is a file's path on disk.
func (m *Model) Path(name string) string {
	if m.Dir == "" {
		return ""
	}
	return filepath.Join(m.Dir, filepath.FromSlash(name))
}

// Bytes reads a small file: tokenizer.json, calibration.json, config.json.
func (m *Model) Bytes(name string) ([]byte, error) {
	if b, ok := m.Files[name]; ok {
		return b, nil
	}
	return os.ReadFile(m.Path(name))
}

// ModelOptions say which model to use and whether it may be fetched.
type ModelOptions struct {
	Options
	Dir   string // a folder the user named: used as is
	GPU   bool   // the q4f16 decoder, for CUDA
	Fetch bool   // download what is missing
}

// HFEndpoint is Hugging Face, or HF_ENDPOINT.
func HFEndpoint() string {
	if e := os.Getenv("HF_ENDPOINT"); e != "" {
		return strings.TrimRight(e, "/")
	}
	return "https://huggingface.co"
}

// ModelCacheDir is where the pinned model is kept.
func ModelCacheDir(o *Options) (string, error) {
	l, err := Locked()
	if err != nil {
		return "", err
	}
	return filepath.Join(o.Cache(), "models", safeName(l.Model.Repo), l.Model.Revision), nil
}

// wanted lists the files a variant needs.
func wanted(l *Lock, gpu bool) []string {
	if !gpu {
		return l.Model.CPU
	}
	var out []string
	for _, f := range l.Model.CPU {
		if !strings.HasPrefix(f, "onnx/decoder_q4.") {
			out = append(out, f)
		}
	}
	return append(out, l.Model.GPU...)
}

// ResolveModel finds the model.
func ResolveModel(ctx context.Context, o ModelOptions) (*Model, error) {
	l, err := Locked()
	if err != nil {
		return nil, err
	}
	variant := "q4"
	if o.GPU {
		variant = "q4f16"
	}
	if o.Dir != "" {
		m := &Model{Source: "dir", Dir: o.Dir, Revision: "local", Variant: variant, Files: map[string][]byte{}}
		for _, f := range []string{m.Decoder(), m.Vision(), "tokenizer.json"} {
			if _, err := os.Stat(m.Path(f)); err != nil {
				return nil, fmt.Errorf("model folder %s: %w", o.Dir, err)
			}
		}
		return m, nil
	}
	dir, err := ModelCacheDir(&o.Options)
	if err != nil {
		return nil, err
	}
	m := &Model{Source: "cache", Dir: dir, Revision: l.Model.Revision, Variant: variant, Files: map[string][]byte{}}
	var missing []string
	for _, name := range wanted(l, o.GPU) {
		f := l.Model.Files[name]
		if ok, _ := verified(m.Path(name), f.SHA256, f.Size); !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return m, nil
	}
	if !o.Fetch {
		return nil, fmt.Errorf("d1-2B is not in the cache (%s): %w", strings.Join(missing, ", "), ErrNotFetched)
	}
	header := http.Header{}
	if tok := os.Getenv("HF_TOKEN"); tok != "" {
		header.Set("Authorization", "Bearer "+tok)
	}
	for _, name := range missing {
		f := l.Model.Files[name]
		url := fmt.Sprintf("%s/%s/resolve/%s/%s", HFEndpoint(), l.Model.Repo, l.Model.Revision, path.Clean(name))
		o.logf("downloading %s (%s)", name, HumanBytes(f.Size))
		if err := fetch(ctx, &o.Options, url, m.Path(name), f.SHA256, f.Size, header); err != nil {
			return nil, err
		}
	}
	m.Source = "download"
	return m, nil
}
