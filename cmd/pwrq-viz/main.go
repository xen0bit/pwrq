// pwrq-viz is pwrq with query diagramming and the browser IDE.
//
// It is a separate binary because d2, which renders the diagrams, brings a
// JavaScript engine, a syntax highlighter and a PDF writer with it - roughly
// 35MB that the everyday `pwrq` has no use for.
//
// The build tag is required rather than optional: a pwrq-viz built without it
// would be an ordinary pwrq under a misleading name.
//go:build viz

package main

import (
	"os"

	"github.com/xen0bit/pwrq/cli"
)

func main() {
	os.Exit(cli.Run())
}
