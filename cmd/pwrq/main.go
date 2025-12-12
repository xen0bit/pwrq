// pwrq - Enhanced Go implementation of jq
package main

import (
	"os"

	"github.com/xen0bit/pwrq/cli"
)

func main() {
	os.Exit(cli.Run())
}
