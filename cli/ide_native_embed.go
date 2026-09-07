//go:build viz && ide_native && embed_web_native

package cli

import (
	"fmt"
	"io/fs"

	"github.com/xen0bit/pwrq/pkg/web"
)

// nativeDist serves the native editor from the copy embedded in this binary,
// so a native pwrq-viz --ide-native needs nothing on disk.
func nativeDist() (fs.FS, error) {
	dist, err := fs.Sub(web.DistNative, "dist-native")
	if err != nil {
		return nil, fmt.Errorf("the embedded native page is not where it should be: %w", err)
	}
	return dist, nil
}
