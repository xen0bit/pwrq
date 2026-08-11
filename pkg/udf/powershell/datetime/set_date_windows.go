//go:build windows
// +build windows

package datetime

import (
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	procSetSystemTime = kernel32.NewProc("SetSystemTime")
)

// setSystemTime sets the system time on Windows
// This requires administrator privileges
func setSystemTime(t time.Time) error {
	// Windows FILETIME is 100-nanosecond intervals since January 1, 1601 UTC
	// Convert from Unix time (seconds since January 1, 1970 UTC)
	unixToFiletime := int64(116444736000000000) // Offset between epochs
	filetime := t.UnixNano()/100 + unixToFiletime

	var ft syscall.Filetime
	ft.LowDateTime = uint32(filetime & 0xFFFFFFFF)
	ft.HighDateTime = uint32(filetime >> 32)

	ret, _, err := procSetSystemTime.Call(uintptr(unsafe.Pointer(&ft)))
	if ret == 0 {
		return err
	}
	return nil
}
