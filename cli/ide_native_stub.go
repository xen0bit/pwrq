//go:build !viz || !ide_native

package cli

import "errors"

// launchNativeIDE is unavailable without the native IDE build: the server
// evaluates queries on this machine, so the binary that serves it is built
// explicitly with -tags 'viz ide_native', never by accident. The WASM-only
// IDE (--ide) is unaffected and stays a static page with no API.
func (cli *cli) launchNativeIDE() error {
	return errors.New("this build has no native IDE; build with -tags 'viz ide_native' (see 'make build-viz-native-with-ide')")
}
