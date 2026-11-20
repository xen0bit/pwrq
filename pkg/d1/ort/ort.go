// Package ort is a minimal binding to the ONNX Runtime C API, loaded at run
// time without cgo: purego on Unix, LoadLibraryEx on Windows. It is adapted
// from hotbutteredbeans' internal/ort, which runs one int64 input to one
// float32 output. d1's decoder takes seven named inputs, two of them float
// feature matrices, and its vision encoder takes a five-dimensional float
// tensor, so Run here takes any set of named int64 and float32 tensors and
// returns each output with its shape.
//
// The OrtApi function table is append-only across releases, so the indices
// below, taken from onnxruntime_c_api.h, hold for every library that serves
// API version apiVersion or newer.
package ort

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unsafe"
)

// apiVersion is the OrtApi version requested: 18 (ONNX Runtime 1.18) added
// AddExternalInitializersFromFilesInMemory, the newest function used here.
const apiVersion = 18

// Indices into the OrtApi function table (onnxruntime_c_api.h, struct OrtApi).
const (
	fnGetErrorMessage                       = 2
	fnCreateEnv                             = 3
	fnDisableTelemetryEvents                = 6
	fnCreateSession                         = 7
	fnCreateSessionFromArray                = 8
	fnRun                                   = 9
	fnCreateSessionOptions                  = 10
	fnSetSessionLogSeverityLevel            = 22
	fnSetIntraOpNumThreads                  = 24
	fnCreateRunOptions                      = 39
	fnRunOptionsSetTerminate                = 46
	fnCreateTensorWithDataAsOrtValue        = 49
	fnGetTensorMutableData                  = 51
	fnGetTensorElementType                  = 60
	fnGetDimensionsCount                    = 61
	fnGetDimensions                         = 62
	fnGetTensorShapeElementCount            = 64
	fnGetTensorTypeAndShape                 = 65
	fnCreateCpuMemoryInfo                   = 69
	fnReleaseStatus                         = 93
	fnReleaseMemoryInfo                     = 94
	fnReleaseSession                        = 95
	fnReleaseValue                          = 96
	fnReleaseRunOptions                     = 97
	fnReleaseTensorTypeAndShapeInfo         = 99
	fnReleaseSessionOptions                 = 100
	fnGetAvailableProviders                 = 125
	fnReleaseAvailableProviders             = 126
	fnSessionOptionsAppendCUDAV2            = 204
	fnCreateCUDAProviderOptions             = 205
	fnUpdateCUDAProviderOptions             = 206
	fnReleaseCUDAProviderOptions            = 208
	fnSessionOptionsAppendExecutionProvider = 216
	fnAddExternalInitializersFromMemory     = 279
)

const (
	logError = 3

	allocatorArena = 1 // OrtArenaAllocator
	memTypeDefault = 0 // OrtMemTypeDefault
)

// ElementType is an ONNX tensor element type (ONNXTensorElementDataType).
type ElementType int

const (
	Float32 ElementType = 1
	Int64   ElementType = 7
)

func (t ElementType) size() uintptr {
	switch t {
	case Float32:
		return 4
	case Int64:
		return 8
	}
	return 0
}

// Tensor is a dense tensor handed to or returned by a run. Exactly one of F32
// and I64 holds the data, matching Type.
type Tensor struct {
	Type  ElementType
	Shape []int64
	F32   []float32
	I64   []int64
}

// F32Tensor and I64Tensor build input tensors; they panic when the data does
// not fill the shape, which is always a bug in the caller's packing.
func F32Tensor(data []float32, shape ...int64) Tensor {
	mustFill(len(data), shape)
	return Tensor{Type: Float32, Shape: shape, F32: data}
}

func I64Tensor(data []int64, shape ...int64) Tensor {
	mustFill(len(data), shape)
	return Tensor{Type: Int64, Shape: shape, I64: data}
}

func mustFill(n int, shape []int64) {
	want := int64(1)
	for _, d := range shape {
		want *= d
	}
	if int64(n) != want {
		panic(fmt.Sprintf("ort: %d values do not fill shape %v", n, shape))
	}
}

// Library is a loaded onnxruntime shared library.
type Library struct {
	Path    string
	Version string
	api     unsafe.Pointer // const OrtApi*

	mu  sync.Mutex
	env uintptr // OrtEnv*, created on first use
}

var (
	loadMu sync.Mutex
	loaded = map[string]*Library{}
)

// Load opens the onnxruntime library at path. It must be a full path: the
// system's search path is never used, so an unrelated onnxruntime on it cannot
// be picked up by accident. Loading the same path twice returns the same
// Library.
func Load(path string) (*Library, error) {
	loadMu.Lock()
	defer loadMu.Unlock()
	if l, ok := loaded[path]; ok {
		return l, nil
	}
	if !strings.HasPrefix(path, "/proc/self/fd/") {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
	}
	base, err := openLibrary(path) // const OrtApiBase*
	if err != nil {
		return nil, fmt.Errorf("loading %s: %w", path, err)
	}
	// struct OrtApiBase { const OrtApi* (*GetApi)(uint32_t); const char* (*GetVersionString)(void); }
	getAPI := *(*uintptr)(base)
	getVersion := *(*uintptr)(unsafe.Add(base, ptrSize))
	version := goString(call(getVersion))
	api := call(getAPI, apiVersion)
	if api == 0 {
		return nil, fmt.Errorf("%s: onnxruntime %s does not serve C API version %d (1.18 or newer is required)", path, version, apiVersion)
	}
	l := &Library{Path: path, Version: version, api: toPointer(api)}
	loaded[path] = l
	return l, nil
}

const ptrSize = unsafe.Sizeof(uintptr(0))

// fn returns the address of the OrtApi function at index i.
func (l *Library) fn(i int) uintptr { return *(*uintptr)(unsafe.Add(l.api, uintptr(i)*ptrSize)) }

// status turns an OrtStatus* into an error, releasing it.
func (l *Library) status(st uintptr) error {
	if st == 0 {
		return nil
	}
	msg := goString(call(l.fn(fnGetErrorMessage), st))
	call(l.fn(fnReleaseStatus), st)
	return errors.New(strings.TrimSpace(msg))
}

func (l *Library) do(i int, args ...uintptr) error { return l.status(call(l.fn(i), args...)) }

func (l *Library) environment() (uintptr, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.env != 0 {
		return l.env, nil
	}
	id := cString("pwrq")
	var env uintptr
	if err := l.do(fnCreateEnv, logError, ptrOf(id), uintptr(unsafe.Pointer(&env))); err != nil {
		return 0, fmt.Errorf("creating the onnxruntime environment: %w", err)
	}
	runtime.KeepAlive(id)
	_ = l.do(fnDisableTelemetryEvents, env)
	l.env = env
	return env, nil
}

// Providers lists the execution providers this library was built with.
func (l *Library) Providers() ([]string, error) {
	var arr uintptr
	var n int32
	if err := l.do(fnGetAvailableProviders, uintptr(unsafe.Pointer(&arr)), uintptr(unsafe.Pointer(&n))); err != nil {
		return nil, err
	}
	out := make([]string, n)
	for i := range out {
		out[i] = goString(*(*uintptr)(unsafe.Add(toPointer(arr), uintptr(i)*ptrSize)))
	}
	_ = l.do(fnReleaseAvailableProviders, arr, uintptr(n))
	return out, nil
}

// HasProvider reports whether the library was built with the named execution
// provider (e.g. "CUDAExecutionProvider").
func (l *Library) HasProvider(name string) bool {
	ps, err := l.Providers()
	if err != nil {
		return false
	}
	for _, p := range ps {
		if p == name {
			return true
		}
	}
	return false
}

// Provider is an execution provider a session runs on.
type Provider string

const (
	CPU    Provider = "cpu"
	CUDA   Provider = "cuda"
	CoreML Provider = "coreml"
)

// SessionOptions configure a session.
type SessionOptions struct {
	Provider Provider
	Device   int // CUDA device id
	Threads  int // intra-op threads; 0: ONNX Runtime's default (the physical cores)
	Verbose  bool
}

// ModelSource is a graph to load: a file on disk, or the graph's bytes plus
// the external data files it references, keyed by the name the graph uses for
// them.
type ModelSource struct {
	Path     string
	Graph    []byte
	External map[string][]byte
}

// Session is one loaded graph. Runs on one session are serialised, which is
// what the d1 engine wants anyway: a second concurrent forward pass on a 2B
// model would only share the same cores.
type Session struct {
	lib  *Library
	sess uintptr
	mem  uintptr
	keep []any // memory ONNX Runtime may still read (external weights)

	runMu sync.Mutex
}

// NewSession loads a graph.
func (l *Library) NewSession(src ModelSource, o SessionOptions) (*Session, error) {
	env, err := l.environment()
	if err != nil {
		return nil, err
	}
	var opts uintptr
	if err := l.do(fnCreateSessionOptions, uintptr(unsafe.Pointer(&opts))); err != nil {
		return nil, err
	}
	defer call(l.fn(fnReleaseSessionOptions), opts)
	if !o.Verbose {
		_ = l.do(fnSetSessionLogSeverityLevel, opts, logError)
	}
	if o.Threads > 0 {
		if err := l.do(fnSetIntraOpNumThreads, opts, uintptr(o.Threads)); err != nil {
			return nil, err
		}
	}
	if err := l.appendProvider(opts, o); err != nil {
		return nil, err
	}

	s := &Session{lib: l}
	if src.Path != "" {
		p := osString(src.Path)
		err = l.do(fnCreateSession, env, ptrOf(p), opts, uintptr(unsafe.Pointer(&s.sess)))
		runtime.KeepAlive(p)
	} else {
		if len(src.External) > 0 {
			if err := l.addExternal(opts, src.External); err != nil {
				return nil, err
			}
			for _, b := range src.External {
				s.keep = append(s.keep, b)
			}
		}
		err = l.do(fnCreateSessionFromArray, env, ptrOf(src.Graph), uintptr(len(src.Graph)), opts,
			uintptr(unsafe.Pointer(&s.sess)))
		runtime.KeepAlive(src.Graph)
	}
	if err != nil {
		return nil, err
	}
	if err := l.do(fnCreateCpuMemoryInfo, allocatorArena, memTypeDefault, uintptr(unsafe.Pointer(&s.mem))); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func (l *Library) appendProvider(opts uintptr, o SessionOptions) error {
	switch o.Provider {
	case "", CPU:
		return nil
	case CUDA:
		var cuda uintptr
		if err := l.do(fnCreateCUDAProviderOptions, uintptr(unsafe.Pointer(&cuda))); err != nil {
			return fmt.Errorf("CUDA: %w", err)
		}
		defer call(l.fn(fnReleaseCUDAProviderOptions), cuda)
		keys, vals := cStrings("device_id"), cStrings(fmt.Sprint(o.Device))
		if err := l.do(fnUpdateCUDAProviderOptions, cuda, ptrOf(keys.ptrs), ptrOf(vals.ptrs), 1); err != nil {
			return fmt.Errorf("CUDA: %w", err)
		}
		runtime.KeepAlive(keys)
		runtime.KeepAlive(vals)
		if err := l.do(fnSessionOptionsAppendCUDAV2, opts, cuda); err != nil {
			return fmt.Errorf("CUDA: %w", err)
		}
		return nil
	case CoreML:
		name := cString("CoreML")
		err := l.do(fnSessionOptionsAppendExecutionProvider, opts, ptrOf(name), 0, 0, 0)
		runtime.KeepAlive(name)
		if err != nil {
			return fmt.Errorf("CoreML: %w", err)
		}
		return nil
	}
	return fmt.Errorf("unknown execution provider %q", o.Provider)
}

func (l *Library) addExternal(opts uintptr, files map[string][]byte) error {
	names := make([][]byte, 0, len(files))
	bufs := make([]uintptr, 0, len(files))
	lens := make([]uintptr, 0, len(files))
	for name, b := range files {
		names = append(names, osString(name))
		bufs = append(bufs, ptrOf(b))
		lens = append(lens, uintptr(len(b)))
	}
	namePtrs := make([]uintptr, len(names))
	for i, n := range names {
		namePtrs[i] = ptrOf(n)
	}
	err := l.do(fnAddExternalInitializersFromMemory, opts, ptrOf(namePtrs), ptrOf(bufs), ptrOf(lens), uintptr(len(files)))
	runtime.KeepAlive(names)
	runtime.KeepAlive(files)
	return err
}

// emptyScratch backs a tensor with no elements. d1's decoder always takes
// video_features, usually as [0, 2048], and ONNX Runtime rejects a null data
// pointer even when the byte length is zero.
var emptyScratch [8]byte

// Run feeds the named inputs and returns the named outputs, each with its
// shape. Cancelling ctx asks ONNX Runtime to terminate the run, which it
// checks between kernels, so an editor's stale suggestion stops within a
// layer rather than finishing a whole forward pass nobody will read.
func (s *Session) Run(ctx context.Context, inputs map[string]Tensor, outputs []string) (map[string]Tensor, error) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l := s.lib

	var runOpts uintptr
	if err := l.do(fnCreateRunOptions, uintptr(unsafe.Pointer(&runOpts))); err != nil {
		return nil, err
	}
	defer call(l.fn(fnReleaseRunOptions), runOpts)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = l.do(fnRunOptionsSetTerminate, runOpts)
		case <-done:
		}
	}()

	var keep []any
	inNames := make([]uintptr, 0, len(inputs))
	inVals := make([]uintptr, 0, len(inputs))
	defer func() {
		for _, v := range inVals {
			call(l.fn(fnReleaseValue), v)
		}
	}()
	for name, t := range inputs {
		n := cString(name)
		keep = append(keep, n, t.Shape)
		var data uintptr
		var count int
		switch t.Type {
		case Float32:
			data, count = ptrOf(t.F32), len(t.F32)
			keep = append(keep, t.F32)
		case Int64:
			data, count = ptrOf(t.I64), len(t.I64)
			keep = append(keep, t.I64)
		default:
			return nil, fmt.Errorf("input %s: unsupported element type %d", name, t.Type)
		}
		if count == 0 {
			data = uintptr(unsafe.Pointer(&emptyScratch[0]))
		}
		var v uintptr
		if err := l.do(fnCreateTensorWithDataAsOrtValue, s.mem, data, uintptr(count)*t.Type.size(),
			ptrOf(t.Shape), uintptr(len(t.Shape)), uintptr(t.Type), uintptr(unsafe.Pointer(&v))); err != nil {
			return nil, fmt.Errorf("input %s: %w", name, err)
		}
		inNames = append(inNames, ptrOf(n))
		inVals = append(inVals, v)
	}
	outNames := make([]uintptr, len(outputs))
	for i, name := range outputs {
		n := cString(name)
		keep = append(keep, n)
		outNames[i] = ptrOf(n)
	}
	outVals := make([]uintptr, len(outputs))

	err := l.do(fnRun, s.sess, runOpts, ptrOf(inNames), ptrOf(inVals), uintptr(len(inVals)),
		ptrOf(outNames), uintptr(len(outNames)), ptrOf(outVals))
	runtime.KeepAlive(keep)
	runtime.KeepAlive(s)
	defer func() {
		for _, v := range outVals {
			if v != 0 {
				call(l.fn(fnReleaseValue), v)
			}
		}
	}()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}

	out := make(map[string]Tensor, len(outputs))
	for i, name := range outputs {
		t, err := l.readTensor(outVals[i])
		if err != nil {
			return nil, fmt.Errorf("output %s: %w", name, err)
		}
		out[name] = t
	}
	return out, nil
}

// readTensor copies an output value into Go memory, so the value can be
// released before the caller reads it.
func (l *Library) readTensor(v uintptr) (Tensor, error) {
	var info uintptr
	if err := l.do(fnGetTensorTypeAndShape, v, uintptr(unsafe.Pointer(&info))); err != nil {
		return Tensor{}, err
	}
	defer call(l.fn(fnReleaseTensorTypeAndShapeInfo), info)
	var typ int32
	if err := l.do(fnGetTensorElementType, info, uintptr(unsafe.Pointer(&typ))); err != nil {
		return Tensor{}, err
	}
	var rank uintptr
	if err := l.do(fnGetDimensionsCount, info, uintptr(unsafe.Pointer(&rank))); err != nil {
		return Tensor{}, err
	}
	shape := make([]int64, rank)
	if rank > 0 {
		if err := l.do(fnGetDimensions, info, ptrOf(shape), rank); err != nil {
			return Tensor{}, err
		}
	}
	var n uintptr
	if err := l.do(fnGetTensorShapeElementCount, info, uintptr(unsafe.Pointer(&n))); err != nil {
		return Tensor{}, err
	}
	t := Tensor{Type: ElementType(typ), Shape: shape}
	if n == 0 {
		return t, nil
	}
	var data uintptr
	if err := l.do(fnGetTensorMutableData, v, uintptr(unsafe.Pointer(&data))); err != nil {
		return Tensor{}, err
	}
	switch t.Type {
	case Float32:
		t.F32 = append([]float32(nil), unsafe.Slice((*float32)(toPointer(data)), n)...)
	case Int64:
		t.I64 = append([]int64(nil), unsafe.Slice((*int64)(toPointer(data)), n)...)
	default:
		return Tensor{}, fmt.Errorf("unsupported element type %d", typ)
	}
	return t, nil
}

// Close frees the session.
func (s *Session) Close() {
	if s.mem != 0 {
		call(s.lib.fn(fnReleaseMemoryInfo), s.mem)
		s.mem = 0
	}
	if s.sess != 0 {
		call(s.lib.fn(fnReleaseSession), s.sess)
		s.sess = 0
	}
	s.keep = nil
}

// --- helpers ---

// toPointer converts an address returned by C into an unsafe.Pointer without
// the uintptr-to-pointer conversion vet rejects; the memory is C's, so the GC
// ignores it.
func toPointer(u uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&u)) }

// ptrOf is the address of a slice's first element (0 for an empty slice). The
// caller keeps the slice alive across the call with runtime.KeepAlive.
func ptrOf[T any](s []T) uintptr {
	if len(s) == 0 {
		return 0
	}
	return uintptr(unsafe.Pointer(&s[0]))
}

func cString(s string) []byte { return append([]byte(s), 0) }

type cStringArray struct {
	strs [][]byte
	ptrs []uintptr
}

func cStrings(ss ...string) *cStringArray {
	a := &cStringArray{}
	for _, s := range ss {
		b := cString(s)
		a.strs = append(a.strs, b)
		a.ptrs = append(a.ptrs, ptrOf(b))
	}
	return a
}

func goString(p uintptr) string {
	if p == 0 {
		return ""
	}
	var b []byte
	for i := uintptr(0); ; i++ {
		c := *(*byte)(unsafe.Add(toPointer(p), i))
		if c == 0 {
			break
		}
		b = append(b, c)
	}
	return string(b)
}
