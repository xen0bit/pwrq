//go:build !viz

package cli

import "errors"

// launchIDE is unavailable in the default build; the page it serves is the
// diagram tool, which lives in pwrq-viz.
func (cli *cli) launchIDE() error {
	return errors.New("this build has no IDE; use pwrq-viz, or build with -tags viz")
}
