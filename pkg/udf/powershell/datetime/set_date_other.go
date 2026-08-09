//go:build !unix && !windows

package datetime

import (
	"fmt"
	"time"
)

// setSystemTime is unavailable on platforms with no settimeofday syscall -
// notably js/wasm, where the whole idea of setting the host clock is
// meaningless. Set-Date's other modes still work; only writing the system
// clock is refused, and it is refused explicitly rather than by failing to
// build.
func setSystemTime(t time.Time) error {
	return fmt.Errorf("set_date: setting the system time is not supported on this platform")
}
