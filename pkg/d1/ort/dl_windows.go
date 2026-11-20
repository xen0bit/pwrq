//go:build windows && (amd64 || arm64)

package ort

import (
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Supported reports whether this platform can load ONNX Runtime at all.
const Supported = true

func openLibrary(path string) (unsafe.Pointer, error) {
	// LOAD_WITH_ALTERED_SEARCH_PATH: the DLLs onnxruntime.dll depends on
	// (onnxruntime_providers_shared.dll, the CUDA provider) load from its own folder.
	h, err := windows.LoadLibraryEx(path, 0, windows.LOAD_WITH_ALTERED_SEARCH_PATH)
	if err != nil {
		return nil, err
	}
	sym, err := windows.GetProcAddress(h, "OrtGetApiBase")
	if err != nil {
		return nil, err
	}
	return toPointer(call(sym)), nil
}

func call(fn uintptr, args ...uintptr) uintptr {
	r, _, _ := syscall.SyscallN(fn, args...)
	return r
}

// osString is a NUL-terminated ORTCHAR_T string: UTF-16 (wchar_t) on Windows.
func osString(s string) []byte {
	u := utf16.Encode([]rune(s + "\x00"))
	return unsafe.Slice((*byte)(unsafe.Pointer(&u[0])), len(u)*2)
}

// Preload loads DLLs, so that ONNX Runtime's providers find them by name
// (Windows matches an already loaded module by its base name). DLLs that fail
// because a dependency is not loaded yet are retried after the others.
func Preload(paths []string) error {
	return preload(paths, func(p string) error {
		_, err := windows.LoadLibraryEx(p, 0, windows.LOAD_WITH_ALTERED_SEARCH_PATH)
		return err
	})
}
