// Command mdl downloads, organises and browses a personal music library.
// See `mdl --help`.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"mdl/internal/cli"
)

// Set at build time: -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.NewApp(version).Run(ctx, os.Args[1:])
	stop()
	os.Exit(code)
}
