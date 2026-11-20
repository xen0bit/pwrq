//go:build !unix && !windows

package assets

// lockFile has nothing to lock with on this platform (WASM); there is no
// second process to race either.
func lockFile(string) (func(), error) { return func() {}, nil }
