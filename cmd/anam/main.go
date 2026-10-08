// anam is the CLI. main is deliberately thin: wire the build identity and signals,
// then hand off to app.Run (AGENTS.md, Structure).
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	// Definitions name the zone their source's clock is read in, and a container
	// image need not carry a zone database. Embedding it makes every build load them.
	_ "time/tzdata"

	"github.com/mattjoyce/anam/internal/app"
)

// Injected by the Makefile via ldflags; "dev" is the honest fallback.
var (
	version   = "dev"
	gitCommit = "none"
	buildDate = "unknown"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	info := app.BuildInfo{Version: version, GitCommit: gitCommit, BuildDate: buildDate}
	if err := app.Run(ctx, info); err != nil {
		fmt.Fprintln(os.Stderr, "anam:", err)
		os.Exit(1)
	}
}
