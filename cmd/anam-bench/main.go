// anam-bench is Anam's read performance bench. main is thin: it hands off to
// bench.Main (AGENTS.md, Structure).
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	_ "time/tzdata" // the Definitions' zones, in any image

	"github.com/mattjoyce/anam/internal/bench"
)

// Injected by ldflags; "dev" is the honest fallback.
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := bench.Main(ctx, version); err != nil {
		fmt.Fprintln(os.Stderr, "anam-bench:", err)
		os.Exit(1)
	}
}
