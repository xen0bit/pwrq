//go:build !windows
// +build !windows

package datetime

import (
	"syscall"
	"time"
)

// setSystemTime sets the system time on Unix-like systems
// This requires root privileges
func setSystemTime(t time.Time) error {
	tv := syscall.Timeval{
		Sec:  t.Unix(),
		Usec: int64(t.Nanosecond() / 1000),
	}
	return syscall.Settimeofday(&tv)
}
