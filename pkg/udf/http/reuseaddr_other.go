//go:build !unix && !windows

package http

import "net"

// reuseAddrConfig falls back to a plain ListenConfig where SO_REUSEADDR is not
// a thing the platform's syscall package exposes - js/wasm, where there is no
// listening socket to speak of either. Keeping this here is what lets the rest
// of the UDF library build for the browser.
func reuseAddrConfig() net.ListenConfig {
	return net.ListenConfig{}
}
