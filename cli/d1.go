package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"sync"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/xen0bit/pwrq/pkg/d1"
	"github.com/xen0bit/pwrq/pkg/d1/assets"
	"github.com/xen0bit/pwrq/pkg/udf/llm"
)

// runD1 is `--d1 info|fetch|fetch-gpu`: the decision model's files
// and where it would run, outside any query. It is a flag rather than a
// `pwrq d1` subcommand because pwrq's first argument is a jq program, and
// "d1" is one.
func (cli *cli) runD1(command string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	o := d1.OptionsFromEnv()
	ao := assets.Options{CacheDir: o.CacheDir, Offline: o.Offline, Log: cli.d1Logf, Progress: newProgress(cli.errStream).asset}
	switch command {
	case "info":
		return cli.d1Info(ctx, o, ao)
	case "fetch":
		if _, err := assets.ResolveRuntime(ctx, assets.RuntimeOptions{Options: ao, Lib: o.ORTLib, Fetch: true}); err != nil {
			return err
		}
		if _, err := assets.ResolveModel(ctx, assets.ModelOptions{Options: ao, Dir: o.ModelDir, Fetch: true}); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cli.errStream, "d1-2B and ONNX Runtime are in %s\n", ao.Cache())
		return nil
	case "fetch-gpu":
		g := assets.FindGPU(&ao)
		if g.Driver == "" || !assets.CUDAPlatform() {
			return errors.New(g.Why())
		}
		if err := assets.FetchGPU(ctx, &ao); err != nil {
			return err
		}
		if g = assets.FindGPU(&ao); !g.Usable() {
			return fmt.Errorf("fetched, but the GPU is still unusable: %s", g.Why())
		}
		_, _ = fmt.Fprintf(cli.errStream, "the CUDA stack and the q4f16 decoder are in %s\n", ao.Cache())
		return nil
	}
	return fmt.Errorf("--d1 takes info, fetch or fetch-gpu, not %q", command)
}

// d1Info reports the pins, what is in the cache, the GPU, and - when the
// files are there - the model loaded, with its device and fallback reason.
func (cli *cli) d1Info(ctx context.Context, o d1.Options, ao assets.Options) error {
	l, err := assets.Locked()
	if err != nil {
		return err
	}
	g := assets.FindGPU(&ao)
	cached := func(gpu bool) bool {
		_, err := assets.ResolveModel(ctx, assets.ModelOptions{Options: ao, GPU: gpu})
		return err == nil
	}
	missing := g.Missing
	sort.Strings(missing)
	out := map[string]any{
		"model":       map[string]any{"repo": l.Model.Repo, "revision": l.Model.Revision},
		"onnxruntime": l.ONNXRuntime.Version,
		"platform":    assets.Platform(),
		"cache":       ao.Cache(),
		"cached":      map[string]any{"cpu": cached(false), "gpu": cached(true)},
		"gpu":         map[string]any{"driver": g.Driver, "usable": g.Usable(), "why": g.Why(), "missing": missing},
	}
	o.Offline = true // info never downloads
	o.Log = cli.d1Logf
	if m, err := d1.Open(ctx, o); err == nil {
		out["loaded"] = m.Info()
		m.Close()
	} else {
		out["loaded"] = nil
		out["error"] = err.Error()
	}
	enc := json.NewEncoder(cli.outStream)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func (cli *cli) d1Logf(format string, args ...any) {
	_, _ = fmt.Fprintf(cli.errStream, "pwrq: "+format+"\n", args...)
}

// watchD1 shows the decision model's first-use download on stderr while a
// query runs, when stderr is a terminal: two gigabytes fetched in silence
// look like a hang. It returns the function that stops watching.
func (cli *cli) watchD1() func() {
	f, ok := cli.errStream.(*os.File)
	if !ok || !isatty.IsTerminal(f.Fd()) {
		return func() {}
	}
	llm.D1Log = cli.d1Logf
	p := newProgress(cli.errStream)
	stop := d1.Watch(func(s d1.Status) {
		if s.State == d1.StateFetching {
			p.asset(assets.Event{Step: "fetching", Name: s.File, Done: s.Done, Total: s.Total})
		} else {
			p.done()
		}
	})
	return func() { stop(); p.done(); llm.D1Log = nil }
}

// progress draws one updating line per file.
type progress struct {
	mu   sync.Mutex
	w    io.Writer
	tty  bool
	name string
	last time.Time
	open bool
}

func newProgress(w io.Writer) *progress {
	f, ok := w.(*os.File)
	return &progress{w: w, tty: ok && isatty.IsTerminal(f.Fd())}
}

func (p *progress) asset(e assets.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.tty {
		if e.Name != p.name { // a line per file when the output is a log
			p.name = e.Name
			_, _ = fmt.Fprintf(p.w, "pwrq: %s %s\n", e.Step, e.Name)
		}
		return
	}
	if e.Name == p.name && time.Since(p.last) < 100*time.Millisecond && e.Done != e.Total {
		return
	}
	p.name, p.last, p.open = e.Name, time.Now(), true
	line := fmt.Sprintf("%s %s", e.Step, e.Name)
	if e.Total > 0 {
		line += fmt.Sprintf("  %5.1f%%  %s / %s", 100*float64(e.Done)/float64(e.Total), assets.HumanBytes(e.Done), assets.HumanBytes(e.Total))
	}
	_, _ = fmt.Fprintf(p.w, "\r\033[K%s", line)
}

func (p *progress) done() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.open {
		_, _ = fmt.Fprint(p.w, "\r\033[K")
		p.open = false
	}
}
