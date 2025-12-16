//go:build embed_web
// +build embed_web

package web

import (
	"embed"
)

//go:embed dist
var Dist embed.FS

