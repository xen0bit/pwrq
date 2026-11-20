//go:build !((linux || darwin || windows) && (amd64 || arm64))

package ort

import (
	"fmt"
	"runtime"
	"unsafe"
)

// Supported reports whether this platform can load ONNX Runtime at all. pwrq
// is released for armel, armhf, i386, ppc64le, riscv64, s390x and WASM too,
// none of which has an ONNX Runtime build; there the package still compiles,
// so the rest of pwrq does, and Load says why it cannot work.
const Supported = false

func openLibrary(string) (unsafe.Pointer, error) {
	return nil, fmt.Errorf("ONNX Runtime has no build for %s/%s", runtime.GOOS, runtime.GOARCH)
}

// call is never reached: Load fails before any function pointer exists.
func call(uintptr, ...uintptr) uintptr { panic("ort: no ONNX Runtime on this platform") }

func osString(s string) []byte { return cString(s) }

// Preload has nothing to load without ONNX Runtime.
func Preload([]string) error {
	return fmt.Errorf("ONNX Runtime has no build for %s/%s", runtime.GOOS, runtime.GOARCH)
}
