//go:build (linux || darwin) && (amd64 || arm64)

package ort

import (
	"unsafe"

	"github.com/ebitengine/purego"
)

// Supported reports whether this platform can load ONNX Runtime at all.
// ONNX Runtime publishes builds only for amd64 and arm64, and purego's
// calling convention support follows the same line.
const Supported = true

func openLibrary(path string) (unsafe.Pointer, error) {
	h, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil, err
	}
	sym, err := purego.Dlsym(h, "OrtGetApiBase")
	if err != nil {
		return nil, err
	}
	return toPointer(call(sym)), nil
}

func call(fn uintptr, args ...uintptr) uintptr {
	r, _, _ := purego.SyscallN(fn, args...)
	return r
}

// osString is a NUL-terminated ORTCHAR_T string: UTF-8 (char) outside Windows.
func osString(s string) []byte { return cString(s) }

// Preload loads libraries globally, so that ONNX Runtime's providers find them
// by name when they load later (LD_LIBRARY_PATH is read only at process
// start). Libraries that fail because a dependency is not loaded yet are
// retried after the others.
func Preload(paths []string) error {
	return preload(paths, func(p string) error {
		_, err := purego.Dlopen(p, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		return err
	})
}
