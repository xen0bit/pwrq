//go:build embed_web
// +build embed_web

package webembed

import (
	"embed"
)

//go:embed ../pkg/web/dist
var WebDist embed.FS

