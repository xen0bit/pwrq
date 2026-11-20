// Command d1assets pins the files pwrq's decision model runs on.
//
//	go run ./internal/tools/d1assets lock   # re-pin pkg/d1/assets/d1.lock.json
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: d1assets lock")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	var err error
	switch os.Args[1] {
	case "lock":
		err = lock(ctx, filepath.Join("pkg", "d1", "assets", "d1.lock.json"))
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "d1assets:", err)
		os.Exit(1)
	}
}
