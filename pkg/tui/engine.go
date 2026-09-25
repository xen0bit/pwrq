package tui

import (
	"context"
	"strings"
	"sync"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/ideengine"
	"github.com/xen0bit/pwrq/pkg/udf"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// Limits the TUI runs under. The result limit and timeout start where the
// page's do but reach further, because this runs on the machine and has a
// user who can press Esc; the ceilings are the native page's.
var tuiLimits = ideengine.Limits{
	DefaultResults:   10000,
	MaxResults:       1000000,
	DefaultTimeoutMs: 60000,
	MaxTimeoutMs:     3600000,
	MaxOutputBytes:   64 << 20,
}

// runner is the engine as the TUI uses it: the full native vocabulary, with
// debug and stderr captured rather than written to a terminal the UI owns.
type runner struct {
	eng *ideengine.Engine

	// mu serialises runs here as well as in the engine, so that what the sink
	// holds after a run is that run's alone.
	mu   sync.Mutex
	sink *sink
}

func newRunner(extra []gojq.CompilerOption, version string) *runner {
	s := &sink{}
	options := append([]gojq.CompilerOption{}, extra...)
	options = append(options,
		common.WithFunction("debug", 0, 0, s.debug),
		common.WithFunction("stderr", 0, 0, s.stderr),
	)
	return &runner{
		sink: s,
		eng: ideengine.New(ideengine.Config{
			Registry:          udf.DefaultRegistry(),
			Version:           version,
			RunOptions:        options,
			CompileOnValidate: true,
			PrivateSession:    true,
			Limits:            tuiLimits,
		}),
	}
}

// run evaluates a request and returns what it printed to debug and stderr
// beside its result.
func (r *runner) run(ctx context.Context, req ideengine.RunRequest) (ideengine.RunResponse, []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sink.reset()
	resp := r.eng.Run(ctx, req)
	return resp, r.sink.lines()
}

// sink collects what a query writes with debug and stderr, formatted as the
// CLI formats it.
type sink struct {
	mu  sync.Mutex
	buf strings.Builder
}

// maxCaptured keeps a query that debugs in a loop from filling memory.
const maxCaptured = 1 << 20

func (s *sink) write(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.buf.Len()+len(text) > maxCaptured {
		return
	}
	s.buf.WriteString(text)
}

func (s *sink) debug(v any, _ []any) any {
	if b, err := gojq.Marshal([]any{"DEBUG:", v}); err == nil {
		s.write(string(b) + "\n")
	}
	return v
}

func (s *sink) stderr(v any, _ []any) any {
	if text, ok := v.(string); ok {
		s.write(text)
	} else if b, err := gojq.Marshal(v); err == nil {
		s.write(string(b))
	}
	return v
}

func (s *sink) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf.Reset()
}

func (s *sink) lines() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	text := strings.TrimRight(s.buf.String(), "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}
