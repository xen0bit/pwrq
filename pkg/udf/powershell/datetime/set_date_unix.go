//go:build unix

package datetime

import (
	"fmt"
	"syscall"
	"time"
)

// setSystemTime sets the system time on Unix-like systems.
// This requires root privileges.
//
// syscall.Timeval is not one type across platforms: Sec and Usec are int64 on
// 64-bit Linux and int32 on 32-bit (386, arm), so assigning t.Unix() to Sec
// compiles on the one and not the other. NsecToTimeval is the portable
// spelling - the syscall package generates it per platform with the right
// widths - and it is what lets pwrq build for i386, armel and armhf at all.
//
// The conversion goes through nanoseconds, which is the narrower range: an
// int64 nanosecond count only spans 1678-2262, and a 32-bit Sec only reaches
// 2038. Either limit would wrap silently and set the clock to an arbitrary
// time, so the result is checked against the seconds we actually meant rather
// than trusted. One comparison covers both overflows.
func setSystemTime(t time.Time) error {
	tv := syscall.NsecToTimeval(t.UnixNano())
	if int64(tv.Sec) != t.Unix() {
		return fmt.Errorf("date %s is out of range for this platform's settimeofday",
			t.Format(time.RFC3339))
	}
	return syscall.Settimeofday(&tv)
}
