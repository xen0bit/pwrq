//go:build embed_web_native

package web

import (
	"embed"
)

// DistNative is the native browser IDE: the editor page backed by the
// server's full cmdlet vocabulary. It is embedded separately from Dist (the
// WASM-only page) so a build that embeds one never carries the other by
// accident.
//
//go:embed dist-native
var DistNative embed.FS
