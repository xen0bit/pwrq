package llm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/pipeline"
	"github.com/xen0bit/pwrq/pkg/core/typed"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// hbb is Hot Buttered Beans, a Go CLI that runs secjev-encoder: a small encoder
// model that reads one window of source and answers a fixed set of CWE questions
// in a single forward pass. It is the same kind of answer System One gives — a
// probability per question — from a model that runs in-process on the machine,
// with no endpoint, no prompt and no context budget to discover.
//
// pwrq drives it as a child: `hbb serve --stdio` loads the model once and then
// scores the files it is told about, one JSON line each way. The child is kept
// for the life of the process, because loading the model costs seconds and a
// sweep asks about thousands of files.
//
// Everything about the child that could change an answer is part of the pool
// key, so two calls with different models never share one.

// EnvHbbBin names the hbb binary when no Bin option is given.
const EnvHbbBin = "PWRQ_HBB_BIN"

// EnvHbbUrl and EnvHbbToken name a remote hbb (hbb serve --listen) and its bearer
// token when no Url or Token option is given.
const (
	EnvHbbUrl   = "PWRQ_HBB_URL"
	EnvHbbToken = "PWRQ_HBB_TOKEN"
)

// hbbOptions are the options invoke_hbb and get_hbb accept. Like the LLM
// cmdlets, an unknown name is an error rather than ignored: an option that
// silently did nothing is a different model than the caller thinks they ran.
type hbbOptions struct {
	// Url, when set, is an `hbb serve --listen` on another machine: no child is
	// started, and the options that describe a child are errors beside it.
	Url           string `param:"Url"`
	Token         string `param:"Token"`
	Bin           string `param:"Bin"`
	Device        string `param:"Device"`
	ModelDir      string `param:"ModelDir"`
	ModelRepo     string `param:"ModelRepo"`
	ModelRevision string `param:"ModelRevision"`
	ModelVariant  string `param:"ModelVariant"`
	GpuId         int    `param:"GpuId"`
	Threads       int    `param:"Threads"`
	CacheDir      string `param:"CacheDir"`
	Offline       bool   `param:"Offline"`
	// Timeout is seconds to wait for one file, and for the model to load.
	Timeout int `param:"Timeout"`
}

// RegisterInvokeHbb registers invoke_hbb, which scores one file.
//
//	invoke_hbb($file)
//	invoke_hbb($file; $options)
//
// $file is {Path, Full} to read it from disk, or {Path, Text}; Path is what the
// window headers show and its extension picks the language unless Lang is given.
// The result is {Path, Lang, Lines, Windows: [{From, To, P}], Tokens, Cached},
// {Path, Skip} when the model has no language for the file, or {Path, Error} when
// it could not read or score that file. P holds only the
// questions asked of that language, keyed by question id (cwe_89).
func RegisterInvokeHbb() gojq.CompilerOption {
	const op = "invoke_hbb"
	return common.WithFunction(op, 1, 2, func(v any, args []any) any {
		var rawOpts any
		if len(args) == 2 {
			rawOpts = args[1]
		}
		o, err := bindHbbOptions(op, rawOpts)
		if err != nil {
			return err
		}
		file, ok := common.BindValue(args[0]).(map[string]any)
		if !ok {
			return fmt.Errorf("%s: the file must be an object with Path and Full or Text, got %s", op, jsonType(args[0]))
		}
		remote := o.Url != ""
		req := map[string]any{}
		for _, k := range [][2]string{{"Path", "path"}, {"Full", "file"}, {"Text", "text"}, {"Lang", "lang"}} {
			if s, ok := file[k[0]].(string); ok && s != "" {
				req[k[1]] = s
			}
		}
		if req["path"] == nil {
			return fmt.Errorf("%s: the file has no Path; it is what the window headers show", op)
		}
		if req["file"] == nil && req["text"] == nil {
			return fmt.Errorf("%s: the file has neither Full (where to read it) nor Text", op)
		}
		p, err := hbbProcess(op, o)
		if err != nil {
			return err
		}
		if remote && req["file"] != nil {
			// The server does not share this machine's disk: it is sent the bytes.
			path, _ := req["file"].(string)
			data, rerr := os.ReadFile(path)
			if rerr != nil {
				return hbbFileObject(map[string]any{"path": req["path"], "error": rerr.Error()})
			}
			delete(req, "file")
			req["text"] = string(data)
		}
		resp, err := p.ask(req, o.timeout())
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
		// A file hbb could not score is that file's Error, in the result, and the
		// query goes on to the next one. Only a failure of hbb itself -- it
		// died, it timed out, it could not start -- is raised, because every
		// later file would fail the same way.
		recordHbbCall(resp)
		return hbbFileObject(resp)
	})
}

// RegisterGetHbb registers get_hbb, which reports what the model is: its name,
// device, window rules and every question with the languages it is asked of.
// It is how a caller builds the question set from the model rather than from a
// copy of it that could drift.
func RegisterGetHbb() gojq.CompilerOption {
	const op = "get_hbb"
	return common.WithFunction(op, 0, 1, func(v any, args []any) any {
		var rawOpts any
		if len(args) == 1 {
			rawOpts = args[0]
		}
		o, err := bindHbbOptions(op, rawOpts)
		if err != nil {
			return err
		}
		p, err := hbbProcess(op, o)
		if err != nil {
			return err
		}
		ready := p.info()
		out := map[string]any{typed.TypeKey: "Pwrq.Hbb.Info"}
		for k, name := range map[string]string{"version": "Version", "model": "Model", "device": "Device",
			"window": "Window", "labels": "Labels", "questions": "Questions"} {
			out[name] = ready[k]
		}
		return out
	})
}

func hbbFileObject(resp map[string]any) map[string]any {
	out := map[string]any{typed.TypeKey: "Pwrq.Hbb.File"}
	for k, name := range map[string]string{"path": "Path", "lang": "Lang", "lines": "Lines",
		"skip": "Skip", "error": "Error", "tokens": "Tokens", "cached": "Cached"} {
		if v, ok := resp[k]; ok {
			out[name] = v
		}
	}
	ws, _ := resp["windows"].([]any)
	windows := make([]any, 0, len(ws))
	for _, w := range ws {
		m, _ := w.(map[string]any)
		windows = append(windows, map[string]any{"From": m["from"], "To": m["to"], "P": m["p"]})
	}
	out["Windows"] = windows
	return out
}

// recordHbbCall adds one file to the process totals get_llm_usage reports, so a
// sweep's accounting reads the same whichever model answered. No ceiling
// applies: a local model has no bill, and the ceiling exists to protect one.
func recordHbbCall(resp map[string]any) {
	usageMu.Lock()
	defer usageMu.Unlock()
	usage.Calls++
	if n, ok := resp["tokens"].(float64); ok {
		usage.InputTokens += int(n)
	}
}

func (o hbbOptions) timeout() time.Duration {
	if o.Timeout > 0 {
		return time.Duration(o.Timeout) * time.Second
	}
	return 10 * time.Minute
}

func bindHbbOptions(op string, raw any) (hbbOptions, error) {
	var o hbbOptions
	opts, err := optionsArg(op, raw)
	if err != nil {
		return o, err
	}
	known := paramNames(&o)
	normalized := make(map[string]any, len(opts))
	for k, v := range opts {
		if _, ok := known[strings.ToLower(k)]; !ok {
			return o, fmt.Errorf("%s: unknown option %q; expected one of %s",
				op, k, strings.Join(sortedNames(known), ", "))
		}
		normalized[k] = normalizeNumbers(v)
	}
	if len(normalized) > 0 {
		if err := pipeline.BindParameters(normalized, &o); err != nil {
			return o, fmt.Errorf("%s: %w", op, err)
		}
	}
	if o.Url == "" {
		o.Url = os.Getenv(EnvHbbUrl)
	}
	if o.Url != "" {
		if o.Token == "" {
			o.Token = os.Getenv(EnvHbbToken)
		}
		if set := o.localOptions(); len(set) > 0 {
			return o, fmt.Errorf("%s: Url names an hbb on another machine, which has its own model; "+
				"%s describe a local one and cannot be given with it", op, strings.Join(set, ", "))
		}
		return o, nil
	}
	if o.Token != "" {
		return o, fmt.Errorf("%s: Token is for an hbb at a Url", op)
	}
	if o.Bin == "" {
		o.Bin = os.Getenv(EnvHbbBin)
	}
	if o.Bin == "" {
		o.Bin = "hbb"
	}
	return o, nil
}

// args are the flags that make the child the model the options describe.
func (o hbbOptions) args() []string {
	a := []string{"serve", "--stdio", "--quiet"}
	add := func(flag, v string) {
		if v != "" {
			a = append(a, flag, v)
		}
	}
	add("--device", o.Device)
	add("--model-dir", o.ModelDir)
	add("--model-repo", o.ModelRepo)
	add("--model-revision", o.ModelRevision)
	add("--model-variant", o.ModelVariant)
	add("--cache-dir", o.CacheDir)
	if o.GpuId > 0 {
		a = append(a, "--gpu-id", fmt.Sprint(o.GpuId))
	}
	if o.Threads > 0 {
		a = append(a, "--threads", fmt.Sprint(o.Threads))
	}
	if o.Offline {
		a = append(a, "--offline")
	}
	return a
}

// hbbProc is one running `hbb serve`. Requests are answered in order, so one
// mutex serialises them; the model is one session on one device and could not
// overlap them anyway.
type hbbProc struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	out    *bufio.Reader
	ready  map[string]any
	stderr *tailBuffer
	nextID int
	dead   error
}

var (
	hbbPoolMu sync.Mutex
	hbbPool   = map[string]*hbbProc{}
)

// localOptions names the options that only make sense for a child hbb.
func (o hbbOptions) localOptions() []string {
	var set []string
	for _, f := range []struct {
		name string
		set  bool
	}{{"Bin", o.Bin != ""}, {"Device", o.Device != ""}, {"ModelDir", o.ModelDir != ""},
		{"ModelRepo", o.ModelRepo != ""}, {"ModelRevision", o.ModelRevision != ""},
		{"ModelVariant", o.ModelVariant != ""}, {"GpuId", o.GpuId != 0}, {"Threads", o.Threads != 0},
		{"CacheDir", o.CacheDir != ""}, {"Offline", o.Offline}} {
		if f.set {
			set = append(set, f.name)
		}
	}
	return set
}

// hbbBackend is an hbb that answers: a child on stdio, or a server over HTTP.
type hbbBackend interface {
	// info is the document hbb gave on starting (stdio) or at /v1/info (HTTP).
	info() map[string]any
	ask(req map[string]any, timeout time.Duration) (map[string]any, error)
}

func (p *hbbProc) info() map[string]any { return p.ready }

func hbbProcess(op string, o hbbOptions) (hbbBackend, error) {
	if o.Url != "" {
		return hbbRemote(op, o)
	}
	key := strings.Join(append([]string{o.Bin}, o.args()...), "\x00")
	hbbPoolMu.Lock()
	defer hbbPoolMu.Unlock()
	if p := hbbPool[key]; p != nil && p.dead == nil {
		return p, nil
	}
	p, err := startHbb(op, o)
	if err != nil {
		return nil, err
	}
	hbbPool[key] = p
	return p, nil
}

func startHbb(op string, o hbbOptions) (*hbbProc, error) {
	cmd := exec.Command(o.Bin, o.args()...)
	stderr := &tailBuffer{max: 4096}
	cmd.Stderr = stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%s: starting %s: %w (put hbb on PATH, set %s, or pass {Bin: path})", op, o.Bin, err, EnvHbbBin)
	}
	p := &hbbProc{cmd: cmd, stdin: stdin, out: bufio.NewReaderSize(stdout, 1<<20), stderr: stderr}
	ready, err := p.read(o.timeout())
	if err != nil {
		p.kill()
		return nil, fmt.Errorf("%s: %s did not become ready: %w", op, o.Bin, err)
	}
	if ok, _ := ready["ready"].(bool); !ok {
		p.kill()
		return nil, fmt.Errorf("%s: %s said %v instead of ready", op, o.Bin, ready)
	}
	p.ready = ready
	return p, nil
}

// read takes one line from the child, or fails when it does not come in time or
// the child has gone. A child that died says why on stderr, which is the
// message a caller needs.
func (p *hbbProc) read(timeout time.Duration) (map[string]any, error) {
	type result struct {
		line []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		line, err := p.out.ReadBytes('\n')
		done <- result{line, err}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			p.dead = r.err
			return nil, fmt.Errorf("%w%s", r.err, p.stderr.suffix())
		}
		var m map[string]any
		if err := json.Unmarshal(r.line, &m); err != nil {
			p.dead = err
			return nil, fmt.Errorf("unreadable reply %q: %w", truncateForDebug(string(r.line)), err)
		}
		return m, nil
	case <-time.After(timeout):
		// A late answer would be read as the reply to the next request, so the
		// child is not reusable after a timeout.
		p.dead = fmt.Errorf("timed out after %s", timeout)
		p.kill()
		return nil, p.dead
	}
}

func (p *hbbProc) ask(req map[string]any, timeout time.Duration) (map[string]any, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dead != nil {
		return nil, fmt.Errorf("hbb is not running: %w", p.dead)
	}
	p.nextID++
	req["id"] = p.nextID
	line, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if _, err := p.stdin.Write(append(line, '\n')); err != nil {
		p.dead = err
		return nil, fmt.Errorf("%w%s", err, p.stderr.suffix())
	}
	resp, err := p.read(timeout)
	if err != nil {
		return nil, err
	}
	if id, _ := resp["id"].(float64); int(id) != p.nextID {
		p.dead = fmt.Errorf("reply %v out of order, expected %d", resp["id"], p.nextID)
		p.kill()
		return nil, p.dead
	}
	return resp, nil
}

func (p *hbbProc) kill() {
	_ = p.stdin.Close()
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	go func() { _ = p.cmd.Wait() }()
}

// tailBuffer keeps the end of the child's stderr, for the error message of a
// child that died.
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func (t *tailBuffer) Write(b []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, b...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(b), nil
}

func (t *tailBuffer) suffix() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := strings.TrimSpace(string(bytes.ToValidUTF8(t.buf, nil)))
	if s == "" {
		return ""
	}
	return ": " + s
}

// hbbPoolKeys lists the running children, for the tests.
func hbbPoolKeys() []string {
	hbbPoolMu.Lock()
	defer hbbPoolMu.Unlock()
	var ks []string
	for k := range hbbPool {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
