//go:build unix || windows

package http

import (
	"net"
	"syscall"
)

// reuseAddrConfig returns a ListenConfig that sets SO_REUSEADDR, so a server
// restarted on the same port does not lose to a socket still in TIME_WAIT.
func reuseAddrConfig() net.ListenConfig {
	return net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var err error
			c.Control(func(fd uintptr) {
				err = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
			})
			return err
		},
	}
}
