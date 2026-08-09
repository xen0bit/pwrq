//go:build viz && embed_web

package cli

import (
	"fmt"
	"io/fs"

	"github.com/xen0bit/pwrq/pkg/web"
)

// launchIDE serves the editor from the copy embedded in this binary, so
// pwrq-viz --ide needs nothing on disk.
func (cli *cli) launchIDE() error {
	dist, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		return fmt.Errorf("the embedded page is not where it should be: %w", err)
	}
	return serveIDE(cli, dist)
}
