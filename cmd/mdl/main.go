// Command mdl downloads, organises and browses a personal music library.
// See `mdl --help`.
package main

import (
	"context"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/projector22/ubiquitous-sniffle/internal/cli"
)

// Set at build time: -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.NewApp(resolveVersion()).Run(ctx, os.Args[1:])
	stop()
	os.Exit(code)
}

// resolveVersion prefers the -ldflags version and falls back to the module
// version that `go install ...@v1.2.3` records in the binary.
func resolveVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}
